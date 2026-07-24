package main

import (
	"flag"
	"log"
	"os"

	"github.com/wiz/sendsmtp/internal/socks5d"
)

func main() {
	addr := flag.String("addr", ":10808", "listen address")
	user := flag.String("user", "sendsmtp", "SOCKS5 username")
	pass := flag.String("pass", "", "SOCKS5 password")
	flag.Parse()
	if *pass == "" {
		*pass = os.Getenv("SOCKS5_PASS")
	}
	if *pass == "" {
		log.Fatal("-pass or SOCKS5_PASS required")
	}
	if err := socks5d.Run(*addr, *user, *pass); err != nil {
		log.Fatal(err)
	}
}
