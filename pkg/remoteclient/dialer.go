package remoteclient

import (
	"context"
	"fmt"
	"net"
)

type dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

func createDialContext(d dialer, apiServerIPOverride string) func(ctx context.Context, network, address string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" {
			return nil, fmt.Errorf("unimplemented network %q", network)
		}

		_, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}

		return d.DialContext(ctx, network, apiServerIPOverride+":"+port)
	}
}
