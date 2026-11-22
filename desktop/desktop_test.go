package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func readyProxy(t *testing.T) *proxy {
	t.Helper()
	p := newProxy()
	p.desktop.setContext(context.Background())
	return p
}

func decodeBody(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("not JSON: %s", b)
	}
	return m
}

func TestDesktopSaveWritesWhereTheDialogSays(t *testing.T) {
	p := readyProxy(t)
	dest := filepath.Join(t.TempDir(), "chosen.json")
	var offered string
	p.desktop.saveDialog = func(_ context.Context, name string) (string, error) {
		offered = name
		return dest, nil
	}
	rec := call(t, p, "POST", "/__af/__desktop/save", "", `{"name":"../../user-u1.json","content":"{\"a\":1}"}`, nil)
	got := decodeBody(t, rec.Body.Bytes())
	if got["ok"] != true || got["path"] != dest {
		t.Fatalf("save = %+v", got)
	}
	if offered != "user-u1.json" {
		t.Fatalf("the dialog was offered %q; a suggested name must not carry a path", offered)
	}
	if b, _ := os.ReadFile(dest); string(b) != `{"a":1}` {
		t.Fatalf("file = %q", b)
	}

	// Cancelling the dialog writes nothing.
	p.desktop.saveDialog = func(context.Context, string) (string, error) { return "", nil }
	got = decodeBody(t, call(t, p, "POST", "/__af/__desktop/save", "", `{"name":"x","content":"y"}`, nil).Body.Bytes())
	if got["ok"] != false || got["cancelled"] != true {
		t.Fatalf("cancelled save = %+v", got)
	}
}

func TestDesktopOpenFolder(t *testing.T) {
	p := readyProxy(t)
	var opened string
	p.desktop.openDir = func(dir string) error { opened = dir; return nil }

	dir := t.TempDir()
	cfg := filepath.Join(dir, "agentflow.yaml")
	os.WriteFile(cfg, []byte("version: 1\n"), 0o600)

	got := decodeBody(t, call(t, p, "POST", "/__af/__desktop/open-folder", "", `{"path":`+jsonString(cfg)+`}`, nil).Body.Bytes())
	if got["ok"] != true || opened != dir {
		t.Fatalf("a config file opens its folder: %+v, opened %q", got, opened)
	}
	opened = ""
	got = decodeBody(t, call(t, p, "POST", "/__af/__desktop/open-folder", "", `{"path":"/no/such/place/agentflow.yaml"}`, nil).Body.Bytes())
	if got["ok"] != false || got["problem"] != "missing" || opened != "" {
		t.Fatalf("a path that is not on this machine is reported, not opened: %+v", got)
	}
}

func TestDesktopReadsTheFyneConfig(t *testing.T) {
	p := readyProxy(t)
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("AGENTFLOW_STUDIO_CONFIG", path)

	got := decodeBody(t, call(t, p, "GET", "/__af/__desktop/fyne-config", "", "", nil).Body.Bytes())
	if got["found"] != false {
		t.Fatalf("no Fyne config = %+v", got)
	}

	os.WriteFile(path, []byte(`{"url":"http://a:8080","token":"t","language":"zh-Hant","active":"a:8080",
		"profiles":[{"name":"a:8080","url":"http://a:8080","token":"t"},{"name":"b","url":"unix:///run/b.sock","token":"secret","forget_token":true}]}`), 0o600)
	got = decodeBody(t, call(t, p, "GET", "/__af/__desktop/fyne-config", "", "", nil).Body.Bytes())
	profiles, _ := got["profiles"].([]any)
	if got["found"] != true || len(profiles) != 2 || got["language"] != "zh-Hant" || got["active"] != "a:8080" {
		t.Fatalf("fyne config = %+v", got)
	}
	if b := profiles[1].(map[string]any); b["token"] != nil && b["token"] != "" {
		t.Fatalf("a profile set to forget its token must not hand one over: %+v", b)
	}

	// A config from before profiles existed is one profile.
	os.WriteFile(path, []byte(`{"url":"http://old:8080","token":"t"}`), 0o600)
	got = decodeBody(t, call(t, p, "GET", "/__af/__desktop/fyne-config", "", "", nil).Body.Bytes())
	if profiles, _ := got["profiles"].([]any); len(profiles) != 1 {
		t.Fatalf("legacy config = %+v", got)
	}
}

func TestDesktopBackgroundAndUnknownActions(t *testing.T) {
	p := readyProxy(t)
	var rgb [3]uint8
	p.desktop.setBg = func(_ context.Context, r, g, b uint8) { rgb = [3]uint8{r, g, b} }
	call(t, p, "POST", "/__af/__desktop/background", "", `{"r":15,"g":17,"b":300}`, nil)
	if rgb != [3]uint8{15, 17, 255} {
		t.Fatalf("background = %v", rgb)
	}
	if rec := call(t, p, "GET", "/__af/__desktop/save", "", "", nil); rec.Code != 404 {
		t.Fatalf("GET save = %d", rec.Code)
	}
	// Before the window exists, the calls that need it say so.
	if rec := call(t, newProxy(), "POST", "/__af/__desktop/save", "", `{}`, nil); rec.Code != 503 {
		t.Fatalf("save before startup = %d", rec.Code)
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestPingReportsFramelessAndCloseRuns(t *testing.T) {
	p := readyProxy(t)
	p.desktop.frameless = true
	closed := make(chan bool, 1)
	p.desktop.close = func(context.Context) { closed <- true }
	ping := decodeBody(t, call(t, p, "GET", "/__af/__ping", "", "", nil).Body.Bytes())
	if ping["frameless"] != true || ping["shell"] != "wails" || ping["version"] != version {
		t.Fatalf("ping = %+v", ping)
	}
	if got := decodeBody(t, call(t, p, "POST", "/__af/__desktop/close", "", "", nil).Body.Bytes()); got["ok"] != true {
		t.Fatalf("close = %+v", got)
	}
	if !<-closed {
		t.Fatal("close did not run")
	}
}

func TestDesktopUpdateStatusAndOpenURL(t *testing.T) {
	p := readyProxy(t)
	st := decodeBody(t, call(t, p, "GET", "/__af/__desktop/update/status", "", "", nil).Body.Bytes())
	if st["current_version"] != version || st["repo"] == "" {
		t.Fatalf("update/status = %+v", st)
	}

	var opened string
	p.desktop.openURL = func(_ context.Context, target string) { opened = target }
	if rec := call(t, p, "POST", "/__af/__desktop/open-url", "", `{"url":"file:///etc/passwd"}`, nil); rec.Code != 400 || opened != "" {
		t.Fatalf("non-http URL should be rejected: code=%d opened=%q", rec.Code, opened)
	}
	if rec := call(t, p, "POST", "/__af/__desktop/open-url", "", `{"url":"https://github.com/hulipa487/agentflow-studio/releases"}`, nil); rec.Code != 200 || opened == "" {
		t.Fatalf("https URL should open: code=%d opened=%q", rec.Code, opened)
	}
}
