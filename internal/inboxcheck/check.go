package inboxcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	apiBase    = "https://spamchecker-api.mailreach.co/api/v1"
	reportBase = "https://spamchecker.mailreach.co"
	pageURL    = "https://www.mailreach.co/email-spam-test"
)

// Seed is kept for YAML/bindings backward compatibility; unused by MailReach flow.
type Seed struct {
	Provider string `yaml:"provider" json:"provider"`
	Email    string `yaml:"email" json:"email"`
	Password string `yaml:"password" json:"password"`
}

type Options struct {
	Headless   bool   `yaml:"headless" json:"headless"`
	WaitSec    int    `yaml:"wait_sec" json:"wait_sec"`
	TimeoutSec int    `yaml:"timeout_sec" json:"timeout_sec"`
	ScriptDir  string `yaml:"-" json:"-"`
}

type PlacementDetail struct {
	Email     string `json:"email"`
	Provider  string `json:"provider"`
	Placement string `json:"placement"` // inbox | spam | other | missing | error
	Error     string `json:"error,omitempty"`
}

type PlacementSummary struct {
	Score     float64           `json:"score"`
	Inbox     int               `json:"inbox"`
	Spam      int               `json:"spam"`
	Other     int               `json:"other"`
	Missing   int               `json:"missing"`
	Total     int               `json:"total"`
	InboxRate float64           `json:"inbox_rate"`
	Label     string            `json:"label"`
	Summary   string            `json:"summary"`
	PublicID  string            `json:"public_id"`
	ReportURL string            `json:"report_url,omitempty"`
	Details   []PlacementDetail `json:"details"`
}

type TestResult struct {
	ID         int     `json:"id"`
	Email      string  `json:"email"`
	Desc       string  `json:"desc"`
	Provider   string  `json:"provider"`
	ReceivedIn *string `json:"received_in"`
}

type Test struct {
	PublicID     string       `json:"public_id"`
	PublicFullID string       `json:"public_full_id"`
	Completed    bool         `json:"completed"`
	Score        *float64     `json:"score"`
	BtoBScore    *float64     `json:"btob_score"`
	BtoCScore    *float64     `json:"btoc_score"`
	Results      []TestResult `json:"results"`
}

type Client struct {
	HTTP *http.Client
}

func NewClient() *Client {
	return &Client{
		HTTP: &http.Client{Timeout: 45 * time.Second},
	}
}

func (c *Client) do(ctx context.Context, method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	req.Header.Set("Origin", "https://www.mailreach.co")
	req.Header.Set("Referer", pageURL)
	return c.HTTP.Do(req)
}

// CreateTest starts a free MailReach spamchecker session (same API as the website SPA).
func (c *Client) CreateTest(ctx context.Context) (*Test, error) {
	resp, err := c.do(ctx, http.MethodPost, apiBase+"/tests?", bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, fmt.Errorf("mailreach create: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("mailreach rate limit/forbidden (%d): %s", resp.StatusCode, truncate(string(data), 200))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("mailreach create HTTP %d: %s", resp.StatusCode, truncate(string(data), 300))
	}
	var t Test
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("mailreach create parse: %w", err)
	}
	if t.PublicID == "" || t.PublicFullID == "" {
		return nil, fmt.Errorf("mailreach create: missing public_id")
	}
	if len(t.Results) == 0 {
		return nil, fmt.Errorf("mailreach create: empty seed list")
	}
	return &t, nil
}

// GetTest polls placement results by short public_id.
func (c *Client) GetTest(ctx context.Context, publicID string) (*Test, error) {
	resp, err := c.do(ctx, http.MethodGet, apiBase+"/tests/"+publicID, nil)
	if err != nil {
		return nil, fmt.Errorf("mailreach get: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("mailreach get HTTP %d: %s", resp.StatusCode, truncate(string(data), 300))
	}
	var t Test
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("mailreach get parse: %w", err)
	}
	return &t, nil
}

// PollUntilDone waits until completed or deadline. Returns last snapshot even if incomplete.
// onProgress receives (placed, total) after each GET when provided.
func (c *Client) PollUntilDone(ctx context.Context, publicID string, interval time.Duration, onProgress func(placed, total int)) (*Test, error) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	var last *Test
	for {
		t, err := c.GetTest(ctx, publicID)
		if err != nil {
			return last, err
		}
		last = t
		placed := 0
		for _, r := range t.Results {
			if r.ReceivedIn != nil && strings.TrimSpace(*r.ReceivedIn) != "" {
				placed++
			}
		}
		if onProgress != nil {
			onProgress(placed, len(t.Results))
		}
		if t.Completed {
			return t, nil
		}

		select {
		case <-ctx.Done():
			if last != nil {
				return last, nil
			}
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}

// CreateTestWithPlaywright fallback when direct API is blocked.
func CreateTestWithPlaywright(ctx context.Context, opt Options) (*Test, error) {
	scriptDir := opt.ScriptDir
	if scriptDir == "" {
		scriptDir = defaultScriptDir()
	}
	script := filepath.Join(scriptDir, "mailreach.mjs")
	if _, err := os.Stat(script); err != nil {
		return nil, fmt.Errorf("script Playwright não encontrado: %s", script)
	}
	timeoutSec := opt.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 180
	}
	in, _ := json.Marshal(map[string]any{
		"action":     "create",
		"headless":   opt.Headless,
		"timeout_ms": timeoutSec * 1000,
	})
	out, err := runNode(ctx, scriptDir, script, in)
	if err != nil {
		return nil, err
	}
	var t Test
	if err := json.Unmarshal(out, &t); err != nil {
		return nil, fmt.Errorf("playwright create parse: %w — %s", err, truncate(string(out), 200))
	}
	if t.PublicID == "" {
		return nil, fmt.Errorf("playwright create: missing public_id")
	}
	return &t, nil
}

func runNode(ctx context.Context, dir, script string, stdin []byte) ([]byte, error) {
	node := "node"
	if runtime.GOOS == "windows" {
		node = "node.exe"
	}
	cmd := exec.CommandContext(ctx, node, script)
	cmd.Dir = dir
	cmd.Stdin = bytes.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("playwright: %s", truncate(msg, 500))
	}
	return stdout.Bytes(), nil
}

func mapPlacement(receivedIn *string) string {
	if receivedIn == nil {
		return "missing"
	}
	v := strings.ToUpper(strings.TrimSpace(*receivedIn))
	switch {
	case v == "" || v == "NULL":
		return "missing"
	case strings.Contains(v, "INBOX"):
		return "inbox"
	case strings.Contains(v, "SPAM") || strings.Contains(v, "JUNK"):
		return "spam"
	default:
		// PROMOTIONS, CATEGORY_*, etc.
		return "other"
	}
}

func SummarizeTest(t *Test) PlacementSummary {
	return SummarizeTestFiltered(t, nil)
}

// SummarizeTestFiltered scores only results whose email is in allow (case-insensitive).
// If allow is empty/nil, all results are used. When filtered, MailReach global score is ignored
// and inbox_rate of the subset is used instead.
func SummarizeTestFiltered(t *Test, allow map[string]struct{}) PlacementSummary {
	filtered := allow != nil && len(allow) > 0
	s := PlacementSummary{
		PublicID:  t.PublicFullID,
		ReportURL: reportBase + "/" + t.PublicFullID,
		Details:   make([]PlacementDetail, 0, len(t.Results)),
	}
	for _, r := range t.Results {
		email := strings.TrimSpace(strings.ToLower(r.Email))
		if filtered {
			if _, ok := allow[email]; !ok {
				continue
			}
		}
		p := mapPlacement(r.ReceivedIn)
		s.Details = append(s.Details, PlacementDetail{
			Email:     r.Email,
			Provider:  r.Provider,
			Placement: p,
		})
		switch p {
		case "inbox":
			s.Inbox++
		case "spam":
			s.Spam++
		case "other":
			s.Other++
		default:
			s.Missing++
		}
	}
	s.Total = len(s.Details)
	known := s.Inbox + s.Spam + s.Other
	if known > 0 {
		s.InboxRate = float64(s.Inbox) / float64(known) * 100
	}
	// Prefer MailReach score only for the full (unfiltered) test.
	if !filtered && t.Score != nil {
		s.Score = *t.Score * 10
	} else {
		s.Score = s.InboxRate
	}
	switch {
	case known == 0:
		s.Label = "unknown"
	case s.Spam == 0 && s.Inbox > 0 && s.Other == 0:
		s.Label = "inbox"
	case s.Inbox == 0 && s.Spam > 0:
		s.Label = "spam"
	default:
		s.Label = "mixed"
	}
	scoreStr := "n/a"
	if !filtered && t.Score != nil {
		scoreStr = fmt.Sprintf("%.1f/10", *t.Score)
	} else {
		scoreStr = fmt.Sprintf("%.0f%%", s.InboxRate)
	}
	s.Summary = fmt.Sprintf("mailreach %s · inbox=%d spam=%d other=%d missing=%d · %s",
		scoreStr, s.Inbox, s.Spam, s.Other, s.Missing, s.ReportURL)
	return s
}

// SeedEmails returns recipient addresses from a created test.
func SeedEmails(t *Test) []string {
	out := make([]string, 0, len(t.Results))
	for _, r := range t.Results {
		if e := strings.TrimSpace(r.Email); e != "" {
			out = append(out, e)
		}
	}
	return out
}

func EmbedCode(html, fullID string) string {
	code := strings.TrimSpace(fullID)
	if code == "" {
		return html
	}
	if strings.Contains(html, code) {
		return html
	}
	return html + "\n<!-- " + code + " -->\n<p style=\"font-size:1px;color:transparent\">" + code + "</p>\n"
}

func defaultScriptDir() string {
	candidates := []string{
		filepath.Join("scripts", "inbox-check"),
		filepath.Join("..", "scripts", "inbox-check"),
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append([]string{filepath.Join(filepath.Dir(exe), "scripts", "inbox-check")}, candidates...)
	}
	for _, c := range candidates {
		if st, err := os.Stat(filepath.Join(c, "mailreach.mjs")); err == nil && !st.IsDir() {
			abs, _ := filepath.Abs(c)
			return abs
		}
		if st, err := os.Stat(filepath.Join(c, "check.mjs")); err == nil && !st.IsDir() {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	abs, _ := filepath.Abs(filepath.Join("scripts", "inbox-check"))
	return abs
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func Wait(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
