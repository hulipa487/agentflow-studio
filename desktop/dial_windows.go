//go:build windows

package main

import (
	"context"
	"net"

	"github.com/Microsoft/go-winio"
)

func dialLocal(ctx context.Context, kind, addr string) (net.Conn, error) {
	if kind == "npipe" {
		return winio.DialPipeContext(ctx, addr)
	}
	var d net.Dialer
	return d.DialContext(ctx, "unix", addr)
}
