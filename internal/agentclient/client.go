package agentclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/wiz/sendsmtp/internal/agentapi"
	"github.com/wiz/sendsmtp/internal/mailer"
	"github.com/wiz/sendsmtp/internal/store"
)

var httpClient = &http.Client{
	Timeout: 90 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        128,
		MaxIdleConnsPerHost: 32,
		IdleConnTimeout:     90 * time.Second,
	},
}

// Send posts a message to the remote agent; SMTP dial happens on the VPS.
func Send(srv store.Server, acc store.SMTP, msg mailer.Message, dialTO, sendTO time.Duration) error {
	if srv.ProxyPort <= 0 {
		return agentapi.Transportf("server %s has no agent port (deploy first)", srv.Host)
	}
	token := srv.ProxyPassword
	if token == "" {
		token = srv.SSHPassword
	}

	reqBody := agentapi.SendRequest{
		SMTP: agentapi.SMTPCreds{
			Host:       acc.Host,
			Port:       acc.Port,
			Encryption: acc.Encryption,
			User:       acc.User,
			Password:   acc.Password,
			FromAddr:   acc.FromAddr,
		},
		Message:        msg,
		DialTimeoutSec: int(dialTO.Seconds()),
		SendTimeoutSec: int(sendTO.Seconds()),
	}
	raw, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("http://%s:%d/v1/send", srv.Host, srv.ProxyPort)
	ctx, cancel := context.WithTimeout(context.Background(), sendTO+dialTO+15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return agentapi.Transportf("agent request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return agentapi.Transportf("agent dial %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode == http.StatusUnauthorized {
		return agentapi.Transportf("agent unauthorized")
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return agentapi.Transportf("agent busy")
	}
	if resp.StatusCode >= 500 {
		return agentapi.Transportf("agent http %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var out agentapi.SendResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return agentapi.Transportf("agent bad response: %s", truncate(string(body), 200))
	}
	if !out.OK {
		if out.Error == "" {
			return fmt.Errorf("agent send failed")
		}
		return fmt.Errorf("%s", out.Error)
	}
	return nil
}

// Health checks GET /health on the agent.
func Health(srv store.Server, timeout time.Duration) error {
	if srv.ProxyPort <= 0 {
		return agentapi.Transportf("no agent port")
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	url := fmt.Sprintf("http://%s:%d/health", srv.Host, srv.ProxyPort)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return agentapi.Transportf("health: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return agentapi.Transportf("health http %d", resp.StatusCode)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
