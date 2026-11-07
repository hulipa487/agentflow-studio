package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
)

// localTransport dials a unix socket or Windows named pipe for the admin API.
// The URL stays synthetic (http://unix or http://npipe); only the transport's
// dialer knows the real address.
func localTransport(kind, addr string) *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialLocal(ctx, kind, addr)
		},
	}
}

// normalizePipeName accepts "agentflow-admin", "npipe://agentflow-admin" or
// "\\\\.\\pipe\\agentflow-admin" and returns the full Win32 path.
func normalizePipeName(name string) string {
	name = strings.TrimPrefix(name, `./pipe/`)
	name = strings.TrimPrefix(name, `/pipe/`)
	name = strings.TrimPrefix(name, `\\.\pipe\`)
	return `\\.\pipe\` + name
}

// isLocalURL reports whether raw names a local socket or pipe.
func isLocalURL(raw string) bool {
	return strings.HasPrefix(raw, "unix://") ||
		strings.HasPrefix(raw, "npipe://") ||
		strings.HasPrefix(raw, "pipe://") ||
		strings.HasPrefix(raw, `\\.\pipe\`)
}

// parseLocal splits a local transport URL into its kind and address.
func parseLocal(raw string) (kind, addr string, ok bool) {
	switch {
	case strings.HasPrefix(raw, "unix://"):
		return "unix", strings.TrimPrefix(raw, "unix://"), true
	case strings.HasPrefix(raw, "npipe://"):
		return "npipe", normalizePipeName(strings.TrimPrefix(raw, "npipe://")), true
	case strings.HasPrefix(raw, "pipe://"):
		return "npipe", normalizePipeName(strings.TrimPrefix(raw, "pipe://")), true
	case strings.HasPrefix(raw, `\\.\pipe\`):
		return "npipe", raw, true
	}
	return "", "", false
}

// localUnsupported is returned when a transport is unavailable on this OS.
func localUnsupported(kind string) error {
	return fmt.Errorf("%s transport is not supported on this platform", kind)
}
