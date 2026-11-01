package engine

import (
	"context"
	"net"
	"strconv"
	"time"
)

// PortOpen reports whether something listens on a port of this Mac, so the
// preview (GUI-020) can say when nothing does instead of showing a blank
// frame.
func (e *Engine) PortOpen(ctx context.Context, port int) bool {
	if port < 1 || port > 65535 {
		return false
	}
	for _, host := range []string{"127.0.0.1", "::1"} {
		d := net.Dialer{Timeout: 300 * time.Millisecond}
		if c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port))); err == nil {
			c.Close()
			return true
		}
	}
	return false
}
