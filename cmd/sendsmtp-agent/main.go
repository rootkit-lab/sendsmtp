package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/wiz/sendsmtp/internal/agentapi"
)

func main() {
	addr := flag.String("addr", ":18080", "listen address")
	token := flag.String("token", "", "Bearer token (or AGENT_TOKEN env)")
	conc := flag.Int("conc", 64, "max concurrent SMTP sends")
	flag.Parse()
	if *token == "" {
		*token = os.Getenv("AGENT_TOKEN")
	}
	if *token == "" {
		log.Fatal("-token or AGENT_TOKEN required")
	}

	h := agentapi.NewHandler(*token, "0.1.3", *conc)
	mux := http.NewServeMux()
	h.Register(mux)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 16,
	}
	log.Printf("sendsmtp-agent listening on %s (conc=%d)", *addr, *conc)
	log.Fatal(srv.ListenAndServe())
}
