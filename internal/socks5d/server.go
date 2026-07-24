package socks5d

import (
	"fmt"
	"log"
	"net"
	"os"

	"github.com/things-go/go-socks5"
)

// Run starts a SOCKS5 proxy with username/password auth on listenAddr (e.g. ":10808").
func Run(listenAddr, user, password string) error {
	if user == "" || password == "" {
		return fmt.Errorf("user and password required")
	}
	cred := socks5.StaticCredentials{user: password}
	auth := socks5.UserPassAuthenticator{Credentials: cred}
	server := socks5.NewServer(
		socks5.WithAuthMethods([]socks5.Authenticator{auth}),
		socks5.WithLogger(socks5.NewLogger(log.New(os.Stdout, "socks5d: ", log.LstdFlags))),
	)
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}
	log.Printf("listening on %s (user=%s)", listenAddr, user)
	return server.Serve(ln)
}
