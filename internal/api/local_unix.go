//go:build !windows

package api

import (
	"context"
	"net"
)

func dialLocal(ctx context.Context, kind, addr string) (net.Conn, error) {
	switch kind {
	case "unix":
		var d net.Dialer
		return d.DialContext(ctx, "unix", addr)
	case "npipe":
		return nil, localUnsupported("named pipe")
	}
	return nil, localUnsupported(kind)
}
