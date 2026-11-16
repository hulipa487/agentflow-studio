//go:build windows

package api

import (
	"context"
	"net"

	"github.com/Microsoft/go-winio"
)

func dialLocal(ctx context.Context, kind, addr string) (net.Conn, error) {
	switch kind {
	case "npipe":
		return winio.DialPipeContext(ctx, addr)
	case "unix":
		var d net.Dialer
		return d.DialContext(ctx, "unix", addr)
	}
	return nil, localUnsupported(kind)
}
