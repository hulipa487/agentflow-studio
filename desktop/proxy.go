package main

// The local proxy, in Go. It is the same contract as web/server/proxy.mjs,
// which serves it beside the Vite dev server:
//
//   webview  ──  /__af/admin/api/...   X-AF-Target: unix:///run/agentflow/admin.sock
//   proxy    ──  GET /admin/api/... on that socket
//
// Wails hands this handler every request the embedded assets cannot answer.
// It is not a network listener: only the app's own webview can reach it, so
// the Node proxy's loopback Host check has nothing to guard here. The other
// guards are kept: only /admin/ is forwarded, the target must be named per
// request in X-AF-Target, cross-site requests and CORS preflights are refused,
// and the bearer token passes through without being logged.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const proxyPrefix = "/__af"

var (
	forwardReq = []string{"Authorization", "Content-Type", "Accept"}
	forwardRes = []string{"Content-Type", "Cache-Control", "Content-Disposition"}
)

// target is a parsed X-AF-Target.
type target struct {
	kind    string // tcp, unix, npipe
	url     *url.URL
	path    string // socket or pipe path
	display string
}

// parseTarget turns the address the operator typed into a target, accepting
// what the web app's login form accepts.
func parseTarget(raw string) (target, error) {
	t := strings.TrimSpace(raw)
	switch {
	case t == "":
		return target{}, errors.New("instance URL is required")
	case strings.HasPrefix(t, "unix://"):
		p := strings.TrimPrefix(t, "unix://")
		if p == "" {
			return target{}, errors.New("a Unix socket URL needs a path, e.g. unix:///run/agentflow/admin.sock")
		}
		return target{kind: "unix", path: p, display: t}, nil
	case strings.HasPrefix(t, "npipe://"), strings.HasPrefix(t, "pipe://"), strings.HasPrefix(t, `\\.\pipe\`):
		name := strings.TrimPrefix(strings.TrimPrefix(t, "npipe://"), "pipe://")
		if !strings.HasPrefix(name, `\\.\pipe\`) {
			name = `\\.\pipe\` + strings.TrimLeft(name, "/")
		}
		return target{kind: "npipe", path: name, display: t}, nil
	}
	if !strings.Contains(t, "://") {
		t = "http://" + t
	}
	u, err := url.Parse(t)
	if err != nil {
		return target{}, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return target{}, fmt.Errorf("unknown scheme %s://; use http, https, unix or npipe", u.Scheme)
	}
	if u.Host == "" {
		return target{}, errors.New("instance URL has no host")
	}
	return target{kind: "tcp", url: u, display: u.Scheme + "://" + u.Host}, nil
}

type proxy struct {
	tcp     *http.Transport
	desktop *desktop
}

func newProxy() *proxy {
	return &proxy{desktop: newDesktop(), tcp: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 0, // streams (log tail, playground) wait as long as they need
		IdleConnTimeout:       90 * time.Second,
	}}
}

func refuse(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": msg})
}

// platform names the OS the way the web app's Node proxy does.
func platform() string {
	if runtime.GOOS == "windows" {
		return "win32"
	}
	return runtime.GOOS
}

func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if !strings.HasPrefix(path, proxyPrefix+"/") {
		http.NotFound(w, r)
		return
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		refuse(w, http.StatusForbidden, "cross-site requests are refused")
		return
	}
	if r.Method == http.MethodOptions {
		refuse(w, http.StatusForbidden, "preflight refused: this proxy is same-origin only")
		return
	}
	rest := strings.TrimPrefix(path, proxyPrefix)
	if rest == "/__ping" {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true, "proxy": true, "platform": platform(), "shell": "wails", "frameless": p.desktop.frameless, "version": version,
		})
		return
	}
	if action, ok := strings.CutPrefix(rest, "/__desktop/"); ok {
		p.desktop.serve(w, r, action)
		return
	}
	if !strings.HasPrefix(rest, "/admin/") {
		refuse(w, http.StatusNotFound, "only /admin/ is proxied")
		return
	}
	raw := r.Header.Values("X-AF-Target")
	if len(raw) != 1 || raw[0] == "" {
		refuse(w, http.StatusBadRequest, "X-AF-Target header is required")
		return
	}
	tg, err := parseTarget(raw[0])
	if err != nil {
		refuse(w, http.StatusBadRequest, err.Error())
		return
	}

	upURL := url.URL{Scheme: "http", Host: "localhost", Path: rest, RawQuery: r.URL.RawQuery}
	transport := p.tcp
	if tg.kind == "tcp" {
		upURL.Scheme, upURL.Host = tg.url.Scheme, tg.url.Host
	} else {
		transport = localTransport(tg.kind, tg.path)
		defer transport.CloseIdleConnections()
	}

	// r.Context() ends when the webview drops the request (navigation, Stop,
	// a closed stream), which ends the upstream call too.
	req, err := http.NewRequestWithContext(r.Context(), r.Method, upURL.String(), r.Body)
	if err != nil {
		refuse(w, http.StatusBadRequest, err.Error())
		return
	}
	req.ContentLength = r.ContentLength
	for _, h := range forwardReq {
		if v := r.Header.Get(h); v != "" {
			req.Header.Set(h, v)
		}
	}
	res, err := transport.RoundTrip(req)
	if err != nil {
		refuse(w, http.StatusBadGateway, fmt.Sprintf("cannot reach %s: %s", tg.display, explain(err, tg.kind)))
		return
	}
	defer res.Body.Close()

	for _, h := range forwardRes {
		if v := res.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(res.StatusCode)
	streamBody(w, res.Body)
}

// streamBody copies the upstream body, flushing each read so a stream (the
// log tail, a playground turn) reaches the page as it is written.
func streamBody(w http.ResponseWriter, body io.Reader) {
	flusher, _ := w.(http.Flusher)
	if flusher != nil {
		flusher.Flush()
	}
	buf := make([]byte, 32*1024)
	for {
		n, err := body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}

// localTransport dials a Unix socket or a Windows named pipe; the request URL
// stays a synthetic http://localhost.
func localTransport(kind, addr string) *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialLocal(ctx, kind, addr)
		},
	}
}

// explain turns a dial error into the wording the Node proxy uses.
func explain(err error, kind string) string {
	switch {
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection refused"
	case errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOENT):
		return "no such socket"
	case errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EACCES):
		return "permission denied on the socket"
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return "host not found"
	}
	if kind != "tcp" && errors.Is(err, syscall.EINVAL) {
		return "not a usable socket path (too long, or not a socket)"
	}
	return err.Error()
}
