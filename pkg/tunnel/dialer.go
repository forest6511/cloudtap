package tunnel

import (
	"context"
	"errors"
	"net"
	"time"
)

// TCPDialer dials real TCP endpoints.
type TCPDialer struct {
	Timeout time.Duration
}

func (d TCPDialer) Connect(ctx context.Context, target Target) error {
	network, addr, err := ResolveNetworkAddr(target)
	if err != nil {
		return err
	}
	nd := &net.Dialer{}
	if d.Timeout > 0 {
		nd.Timeout = d.Timeout
	}
	conn, err := nd.DialContext(ctx, network, addr)
	if err != nil {
		return err
	}
	return conn.Close()
}

// SimulatedDialer pretends to establish tunnels without touching the network.
type SimulatedDialer struct{}

func (SimulatedDialer) Connect(ctx context.Context, target Target) error {
	select {
	case <-time.After(20 * time.Millisecond):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ErrUnsupportedProtocol is returned when the target protocol cannot be dialed.
var ErrUnsupportedProtocol = errors.New("unsupported protocol")
