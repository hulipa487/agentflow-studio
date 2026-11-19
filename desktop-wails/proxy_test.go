package main

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// upstream is a stand-in runtime: it echoes what it was sent, and streams
// two lines with a pause between them on /admin/api/stream.
func upstream() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/admin/api/stream" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(200)
			io.WriteString(w, "data: one\n\n")
			w.(http.Flusher).Flush()
			time.Sleep(300 * time.Millisecond)
			io.WriteString(w, "data: two\n\n")
			return
		}
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "never=forwarded")
		json.NewEncoder(w).Encode(map[string]string{
			"method": r.Method, "path": r.URL.RequestURI(), "auth": r.Header.Get("Authorization"),
			"cookie": r.Header.Get("Cookie"), "body": string(body),
		})
	})
}

func call(t *testing.T, h http.Handler, method, path, tgt, body string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if tgt != "" {
		req.Header.Set("X-AF-Target", tgt)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestProxyForwardsToTCPAndUnixTargets(t *testing.T) {
	tcp := httptest.NewServer(upstream())
	defer tcp.Close()
	targets := []string{tcp.URL, strings.TrimPrefix(tcp.URL, "http://")}

	if runtime.GOOS != "windows" {
		sock := filepath.Join(t.TempDir(), "admin.sock")
		ln, err := net.Listen("unix", sock)
		if err != nil {
			t.Fatal(err)
		}
		srv := &http.Server{Handler: upstream()}
		go srv.Serve(ln)
		defer srv.Close()
		targets = append(targets, "unix://"+sock)
	}

	p := newProxy()
	for _, tgt := range targets {
		rec := call(t, p, "POST", "/__af/admin/api/agents?x=1", tgt, `{"a":1}`, map[string]string{
			"Authorization": "Bearer t0k", "Content-Type": "application/json", "Cookie": "c=1",
		})
		if rec.Code != 200 {
			t.Fatalf("%s: status %d: %s", tgt, rec.Code, rec.Body)
		}
		var got map[string]string
		json.Unmarshal(rec.Body.Bytes(), &got)
		if got["method"] != "POST" || got["path"] != "/admin/api/agents?x=1" || got["auth"] != "Bearer t0k" || got["body"] != `{"a":1}` {
			t.Fatalf("%s: upstream saw %+v", tgt, got)
		}
		if got["cookie"] != "" || rec.Header().Get("Set-Cookie") != "" {
			t.Fatalf("%s: cookies must not pass either way", tgt)
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s: responses must not be cached", tgt)
		}
	}
}

func TestProxyRefusals(t *testing.T) {
	p := newProxy()
	cases := []struct {
		name, method, path, target string
		hdr                        map[string]string
		want                       int
	}{
		{"no target", "GET", "/__af/admin/api/x", "", nil, 400},
		{"not admin", "GET", "/__af/metrics", "http://127.0.0.1:1", nil, 404},
		{"preflight", "OPTIONS", "/__af/admin/api/x", "http://127.0.0.1:1", nil, 403},
		{"cross-site", "GET", "/__af/admin/api/x", "http://127.0.0.1:1", map[string]string{"Sec-Fetch-Site": "cross-site"}, 403},
		{"bad scheme", "GET", "/__af/admin/api/x", "ftp://host", nil, 400},
		{"unreachable", "GET", "/__af/admin/api/x", "http://127.0.0.1:1", nil, 502},
	}
	for _, c := range cases {
		if rec := call(t, p, c.method, c.path, c.target, "", c.hdr); rec.Code != c.want {
			t.Errorf("%s: status %d, want %d (%s)", c.name, rec.Code, c.want, rec.Body)
		}
	}
	rec := call(t, p, "GET", "/__af/__ping", "", "", nil)
	var ping map[string]any
	json.Unmarshal(rec.Body.Bytes(), &ping)
	if ping["proxy"] != true || ping["platform"] == "" {
		t.Fatalf("ping = %+v", ping)
	}
}

// A stream reaches the page line by line, not when the upstream finishes.
func TestProxyStreamsWithoutBuffering(t *testing.T) {
	up := httptest.NewServer(upstream())
	defer up.Close()
	front := httptest.NewServer(newProxy())
	defer front.Close()

	req, _ := http.NewRequest("GET", front.URL+"/__af/admin/api/stream", nil)
	req.Header.Set("X-AF-Target", up.URL)
	start := time.Now()
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	line, err := bufio.NewReader(res.Body).ReadString('\n')
	if err != nil || line != "data: one\n" {
		t.Fatalf("first line = %q, %v", line, err)
	}
	if waited := time.Since(start); waited > 250*time.Millisecond {
		t.Fatalf("the first event took %v: the stream was buffered", waited)
	}
}
