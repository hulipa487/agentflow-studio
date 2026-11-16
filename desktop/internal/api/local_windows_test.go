//go:build windows

package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
)

func TestNamedPipeTransport(t *testing.T) {
	short := fmt.Sprintf("agentflow-studio-test-%d-%d", os.Getpid(), time.Now().UnixNano())
	full := `\\.\pipe\` + short
	l, err := winio.ListenPipe(full, &winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;OW)",
	})
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

	c := New("npipe://"+short, "token")
	if !c.Local() || c.DisplayURL != "npipe://"+short {
		t.Fatalf("client = %+v", c)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Ping(ctx); err != nil {
		t.Fatal(err)
	}
}
