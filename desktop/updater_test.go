package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v0.1.0", "v0.1.0", 0},
		{"0.1.0", "v0.1.0", 0},
		{"v0.2.0", "v0.1.0", 1},
		{"v0.10.0", "v0.2.0", 1},
		{"v1.0.0", "v1.0.0-rc.1", 1},
		{"v1.0.0-rc.2", "v1.0.0-rc.1", 1},
		{"v0.1.0", "v0.2.0", -1},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
	if isSemver("dev") || isSemver("7f51afb") {
		t.Fatal("dev/sha strings must not parse as semver")
	}
	if !isSemver("v1.2.3-beta.1") {
		t.Fatal("v1.2.3-beta.1 should parse as semver")
	}
}

func TestAssetNameAndSHA256Sums(t *testing.T) {
	if got := assetNameFor("windows", "arm64"); got != "agentflow-studio-windows-arm64.exe" {
		t.Fatalf("windows/arm64 asset = %q", got)
	}
	if got := assetNameFor("darwin", "arm64"); got != "agentflow-studio-darwin-arm64.tar.gz" {
		t.Fatalf("darwin/arm64 asset = %q", got)
	}
	if got := assetNameFor("linux", "amd64"); got != "agentflow-studio-linux-amd64.tar.gz" {
		t.Fatalf("linux/amd64 asset = %q", got)
	}
	if got := assetNameFor("freebsd", "amd64"); got != "" {
		t.Fatalf("unexpected freebsd asset = %q", got)
	}

	sums := parseSHA256Sums(strings.Repeat("a", 64) + "  ./agentflow-studio-linux-amd64.tar.gz\n" +
		strings.Repeat("b", 64) + " *agentflow-studio-windows-amd64.exe\n")
	if sums["agentflow-studio-linux-amd64.tar.gz"] != strings.Repeat("a", 64) {
		t.Fatalf("linux sum = %+v", sums)
	}
	if sums["agentflow-studio-windows-amd64.exe"] != strings.Repeat("b", 64) {
		t.Fatalf("windows sum = %+v", sums)
	}
}

func makeTarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		hdr := &tar.Header{
			Name:     name,
			Mode:     0o755,
			Size:     int64(len(body)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func shaHex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func TestUpdaterCheckAndApply(t *testing.T) {
	payload := makeTarGz(t, map[string][]byte{
		"./agentflow-studio": []byte("new-binary-payload-v0.2.0"),
		"./install.sh":       []byte("#!/bin/sh\n"),
	})
	sumLine := shaHex(payload) + "  agentflow-studio-linux-amd64.tar.gz\n"

	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/hulipa487/agentflow-studio/releases/latest":
			_ = json.NewEncoder(w).Encode(githubRelease{
				TagName: "v0.2.0",
				HTMLURL: "https://github.com/hulipa487/agentflow-studio/releases/tag/v0.2.0",
				Body:    "Release notes for v0.2.0",
				Assets: []githubReleaseAsset{
					{Name: "agentflow-studio-linux-amd64.tar.gz", BrowserDownloadURL: srvURL + "/dl/linux"},
					{Name: "SHA256SUMS.txt", BrowserDownloadURL: srvURL + "/dl/sums"},
				},
			})
		case "/dl/sums":
			_, _ = w.Write([]byte(sumLine))
		case "/dl/linux":
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	srvURL = srv.URL

	dir := t.TempDir()
	exe := filepath.Join(dir, "agentflow-studio")
	if err := os.WriteFile(exe, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	u := &updater{
		repo:           defaultUpdateRepo,
		apiBase:        srv.URL,
		currentVersion: "v0.1.0",
		goos:           "linux",
		goarch:         "amd64",
		exePath:        exe,
		httpClient:     srv.Client(),
		downloadClient: srv.Client(),
	}
	u.st = u.baseStatus()

	relaunched := make(chan string, 1)
	quitCalled := make(chan bool, 1)
	u.relaunch = func(exePath, _ string, _ string) error {
		relaunched <- exePath
		return nil
	}

	st, err := u.check(context.Background())
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !st.UpdateAvailable || st.LatestVersion != "v0.2.0" || !st.CanSelfUpdate {
		t.Fatalf("unexpected check status: %+v", st)
	}

	if err := u.apply(context.Background(), func() { quitCalled <- true }); err != nil {
		t.Fatalf("apply: %v", err)
	}
	gotExe := <-relaunched
	<-quitCalled
	if gotExe != exe {
		t.Fatalf("relaunch exe = %q, want %q", gotExe, exe)
	}
	gotBytes, err := os.ReadFile(exe)
	if err != nil || string(gotBytes) != "new-binary-payload-v0.2.0" {
		t.Fatalf("updated binary = %q (%v)", gotBytes, err)
	}
}

func TestUpdaterRejectsChecksumMismatchAndTarTraversal(t *testing.T) {
	payload := []byte("corrupted-exe")
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/hulipa487/agentflow-studio/releases/latest":
			_ = json.NewEncoder(w).Encode(githubRelease{
				TagName: "v0.3.0",
				Assets: []githubReleaseAsset{
					{Name: "agentflow-studio-windows-amd64.exe", BrowserDownloadURL: srvURL + "/dl/win"},
					{Name: "SHA256SUMS.txt", BrowserDownloadURL: srvURL + "/dl/sums"},
				},
			})
		case "/dl/sums":
			_, _ = w.Write([]byte(strings.Repeat("0", 64) + "  agentflow-studio-windows-amd64.exe\n"))
		case "/dl/win":
			_, _ = w.Write(payload)
		}
	}))
	defer srv.Close()
	srvURL = srv.URL

	dir := t.TempDir()
	exe := filepath.Join(dir, "agentflow-studio.exe")
	_ = os.WriteFile(exe, []byte("original"), 0o600)

	u := &updater{
		repo:           defaultUpdateRepo,
		apiBase:        srv.URL,
		currentVersion: "v0.1.0",
		goos:           "windows",
		goarch:         "amd64",
		exePath:        exe,
		httpClient:     srv.Client(),
		downloadClient: srv.Client(),
	}
	u.st = u.baseStatus()
	if _, err := u.check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := u.apply(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch error, got %v", err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "original" {
		t.Fatalf("original binary was modified on checksum failure: %q", b)
	}

	// Archive with path traversal must fail cleanly.
	evilArchive := filepath.Join(t.TempDir(), "evil.tar.gz")
	_ = os.WriteFile(evilArchive, makeTarGz(t, map[string][]byte{"../../evil": []byte("x")}), 0o600)
	if err := extractTarGzSafe(evilArchive, filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("expected tar path traversal error")
	}
}

func TestUpdaterFallsBackToLatestReleasePageOnRateLimit(t *testing.T) {
	payload := makeTarGz(t, map[string][]byte{
		"./agentflow-studio": []byte("new-binary-from-web-fallback-v0.2.0"),
	})
	sumLine := shaHex(payload) + "  agentflow-studio-linux-amd64.tar.gz\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/hulipa487/agentflow-studio/releases/latest":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
		case "/hulipa487/agentflow-studio/releases/latest":
			http.Redirect(w, r, "/hulipa487/agentflow-studio/releases/tag/v0.2.0", http.StatusFound)
		case "/hulipa487/agentflow-studio/releases/tag/v0.2.0":
			t.Fatal("fallback must not download the full HTML release page")
		case "/hulipa487/agentflow-studio/releases/download/v0.2.0/SHA256SUMS.txt":
			_, _ = w.Write([]byte(sumLine))
		case "/hulipa487/agentflow-studio/releases/download/v0.2.0/agentflow-studio-linux-amd64.tar.gz":
			if r.Method == http.MethodHead {
				w.WriteHeader(http.StatusOK)
				return
			}
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	exe := filepath.Join(dir, "agentflow-studio")
	if err := os.WriteFile(exe, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	u := &updater{
		repo:           defaultUpdateRepo,
		apiBase:        srv.URL,
		webBase:        srv.URL,
		currentVersion: "v0.1.0",
		goos:           "linux",
		goarch:         "amd64",
		exePath:        exe,
		httpClient:     srv.Client(),
		downloadClient: srv.Client(),
	}
	u.st = u.baseStatus()
	relaunched := make(chan string, 1)
	quitCalled := make(chan bool, 1)
	u.relaunch = func(exePath, _ string, _ string) error {
		relaunched <- exePath
		return nil
	}

	st, err := u.check(context.Background())
	if err != nil {
		t.Fatalf("check via web fallback: %v", err)
	}
	wantReleaseURL := srv.URL + "/hulipa487/agentflow-studio/releases/tag/v0.2.0"
	if !st.UpdateAvailable || st.LatestVersion != "v0.2.0" || !st.CanSelfUpdate || st.ReleaseURL != wantReleaseURL {
		t.Fatalf("unexpected fallback status: %+v", st)
	}

	if err := u.apply(context.Background(), func() { quitCalled <- true }); err != nil {
		t.Fatalf("apply via web fallback: %v", err)
	}
	if <-relaunched != exe {
		t.Fatal("expected relaunch")
	}
	<-quitCalled
	gotBytes, err := os.ReadFile(exe)
	if err != nil || string(gotBytes) != "new-binary-from-web-fallback-v0.2.0" {
		t.Fatalf("updated binary = %q (%v)", gotBytes, err)
	}
}
