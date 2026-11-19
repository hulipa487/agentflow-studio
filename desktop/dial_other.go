//go:build !windows

package main

import (
	"context"
	"errors"
	"net"
)

func dialLocal(ctx context.Context, kind, addr string) (net.Conn, error) {
	if kind == "unix" {
		var d net.Dialer
		return d.DialContext(ctx, "unix", addr)
	}
	return nil, errors.New("named pipes exist only on Windows")
}
