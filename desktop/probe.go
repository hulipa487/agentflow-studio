//go:build probe

package main

// A test aid, compiled in only with -tags probe: a control socket on
// 127.0.0.1 that runs JavaScript in the window and resizes it, so resize and
// screen checks can be scripted. It is never part of a normal build.

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func init() {
	onReadyHooks = append(onReadyHooks, startProbe)
}

func startProbe(ctx context.Context) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return
	}
	if f := os.Getenv("AF_STUDIO_PROBE_ADDR"); f != "" {
		_ = os.WriteFile(f, []byte(ln.Addr().String()), 0o600)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/js", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		runtime.WindowExecJS(ctx, string(b))
	})
	mux.HandleFunc("/size", func(w http.ResponseWriter, r *http.Request) {
		wd, _ := strconv.Atoi(r.URL.Query().Get("w"))
		ht, _ := strconv.Atoi(r.URL.Query().Get("h"))
		if wd > 0 && ht > 0 {
			runtime.WindowUnmaximise(ctx)
			runtime.WindowSetSize(ctx, wd, ht)
		}
		gw, gh := runtime.WindowGetSize(ctx)
		io.WriteString(w, strconv.Itoa(gw)+"x"+strconv.Itoa(gh))
	})
	go http.Serve(ln, mux)
}
