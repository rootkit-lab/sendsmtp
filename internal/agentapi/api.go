package agentapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/wiz/sendsmtp/internal/mailer"
)

// SendRequest is POSTed by the desktop app to a remote agent.
type SendRequest struct {
	SMTP           SMTPCreds      `json:"smtp"`
	Message        mailer.Message `json:"message"`
	DialTimeoutSec int            `json:"dial_timeout_sec"`
	SendTimeoutSec int            `json:"send_timeout_sec"`
}

type SMTPCreds struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Encryption string `json:"encryption"`
	User       string `json:"user"`
	Password   string `json:"password"`
	FromAddr   string `json:"from_addr"`
}

type SendResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type HealthResponse struct {
	OK      bool   `json:"ok"`
	Version string `json:"version"`
	Sent    int64  `json:"sent"`
	Failed  int64  `json:"failed"`
	Inflight int64 `json:"inflight"`
}

// Handler serves /health and /v1/send with Bearer token auth.
type Handler struct {
	Token   string
	Version string
	MaxConc int64

	sent     atomic.Int64
	failed   atomic.Int64
	inflight atomic.Int64
	sem      chan struct{}
}

func NewHandler(token, version string, maxConc int) *Handler {
	if maxConc <= 0 {
		maxConc = 64
	}
	h := &Handler{
		Token:   token,
		Version: version,
		MaxConc: int64(maxConc),
		sem:     make(chan struct{}, maxConc),
	}
	return h
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/health", h.handleHealth)
	mux.HandleFunc("/v1/send", h.handleSend)
}

func (h *Handler) authOK(r *http.Request) bool {
	if h.Token == "" {
		return true
	}
	got := r.Header.Get("Authorization")
	return got == "Bearer "+h.Token || got == h.Token
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, HealthResponse{
		OK:       true,
		Version:  h.Version,
		Sent:     h.sent.Load(),
		Failed:   h.failed.Load(),
		Inflight: h.inflight.Load(),
	})
}

func (h *Handler) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	if !h.authOK(r) {
		writeJSON(w, http.StatusUnauthorized, SendResponse{OK: false, Error: "unauthorized"})
		return
	}

	var req SendRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20))
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, SendResponse{OK: false, Error: "bad json: " + err.Error()})
		return
	}
	if req.SMTP.Host == "" || req.SMTP.Port <= 0 || req.Message.To == "" {
		writeJSON(w, http.StatusBadRequest, SendResponse{OK: false, Error: "smtp.host, smtp.port, message.to required"})
		return
	}
	if req.Message.From == "" {
		req.Message.From = req.SMTP.FromAddr
	}
	if req.Message.From == "" {
		req.Message.From = req.SMTP.User
	}

	select {
	case h.sem <- struct{}{}:
		defer func() { <-h.sem }()
	default:
		writeJSON(w, http.StatusTooManyRequests, SendResponse{OK: false, Error: "agent busy"})
		return
	}

	dialTO := time.Duration(req.DialTimeoutSec) * time.Second
	sendTO := time.Duration(req.SendTimeoutSec) * time.Second
	if dialTO <= 0 {
		dialTO = 15 * time.Second
	}
	if sendTO <= 0 {
		sendTO = 45 * time.Second
	}

	acc := mailer.Account{
		Host:       req.SMTP.Host,
		Port:       req.SMTP.Port,
		Encryption: req.SMTP.Encryption,
		User:       req.SMTP.User,
		Password:   req.SMTP.Password,
		FromAddr:   req.SMTP.FromAddr,
	}

	h.inflight.Add(1)
	err := mailer.Send(acc, req.Message, dialTO, sendTO)
	h.inflight.Add(-1)
	if err != nil {
		h.failed.Add(1)
		writeJSON(w, http.StatusOK, SendResponse{OK: false, Error: err.Error()})
		return
	}
	h.sent.Add(1)
	writeJSON(w, http.StatusOK, SendResponse{OK: true})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// ErrTransport wraps agent HTTP/connectivity failures (not SMTP).
type ErrTransport struct {
	Msg string
}

func (e *ErrTransport) Error() string { return e.Msg }

func IsTransport(err error) bool {
	_, ok := err.(*ErrTransport)
	return ok
}

func Transportf(format string, args ...any) error {
	return &ErrTransport{Msg: fmt.Sprintf(format, args...)}
}
