package proxyclient

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/wiz/sendsmtp/internal/store"
	"golang.org/x/net/proxy"
)

// DialerFor returns a context dialer that egresses via the server's SOCKS5 proxy.
func DialerFor(srv store.Server, dialTimeout time.Duration) (func(ctx context.Context, network, address string) (net.Conn, error), error) {
	if srv.ProxyPort <= 0 {
		return nil, fmt.Errorf("server %s has no proxy port (deploy first)", srv.Host)
	}
	addr := fmt.Sprintf("%s:%d", srv.Host, srv.ProxyPort)
	var auth *proxy.Auth
	if srv.ProxyUser != "" || srv.ProxyPassword != "" {
		auth = &proxy.Auth{User: srv.ProxyUser, Password: srv.ProxyPassword}
	}
	base := &net.Dialer{Timeout: dialTimeout}
	dialer, err := proxy.SOCKS5("tcp", addr, auth, base)
	if err != nil {
		return nil, fmt.Errorf("socks5 dialer: %w", err)
	}
	if cd, ok := dialer.(proxy.ContextDialer); ok {
		return cd.DialContext, nil
	}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		return dialer.Dial(network, address)
	}, nil
}

// Dial opens a TCP connection to address through the SOCKS5 server.
func Dial(srv store.Server, network, address string, dialTimeout time.Duration) (net.Conn, error) {
	d, err := DialerFor(srv, dialTimeout)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()
	return d(ctx, network, address)
}
