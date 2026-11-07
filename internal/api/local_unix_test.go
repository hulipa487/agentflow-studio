//go:build !windows

package api

import (
	"context"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

func TestUnixSocketTransport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admin.sock")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/api/state" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})}
	go func() { _ = srv.Serve(l) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	c := New("unix://"+path, "token")
	if !c.Local() || c.DisplayURL != "unix://"+path {
		t.Fatalf("client = %+v", c)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Ping(ctx); err != nil {
		t.Fatal(err)
	}
}
