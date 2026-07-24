package shortener

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const abreGenerateURL = "https://abre.ai/_/generate"

var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:125.0) Gecko/20100101 Firefox/125.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
}

type Client struct {
	HTTP    *http.Client
	Retries int
}

func New() *Client {
	return &Client{
		HTTP: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        32,
				MaxIdleConnsPerHost: 16,
				IdleConnTimeout:     60 * time.Second,
			},
		},
		Retries: 3,
	}
}

type generateResp struct {
	Data struct {
		Attributes struct {
			ShortenedURL string `json:"shortenedUrl"`
		} `json:"attributes"`
	} `json:"data"`
}

// Shorten creates an abre.ai short link pointing at destination.
func (c *Client) Shorten(destination string) (string, error) {
	destination = strings.TrimSpace(destination)
	if destination == "" {
		return "", fmt.Errorf("empty destination")
	}
	if c.HTTP == nil {
		c = New()
	}
	retries := c.Retries
	if retries < 0 {
		retries = 0
	}

	payload, _ := json.Marshal(map[string]any{
		"url_translation": map[string]string{"url": destination, "token": ""},
	})

	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		req, err := http.NewRequest(http.MethodPost, abreGenerateURL, bytes.NewReader(payload))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "https://abre.ai")
		req.Header.Set("Referer", "https://abre.ai/")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.Header.Set("Accept", "application/json, text/plain, */*")
		req.Header.Set("User-Agent", userAgents[attempt%len(userAgents)])

		resp, err := c.HTTP.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(300*(attempt+1)) * time.Millisecond)
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()

		if resp.StatusCode == 429 || resp.StatusCode == 403 || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("abre.ai http %d", resp.StatusCode)
			time.Sleep(time.Duration(800*(attempt+1)) * time.Millisecond)
			continue
		}
		if resp.StatusCode != 200 && resp.StatusCode != 201 {
			return "", fmt.Errorf("abre.ai http %d: %s", resp.StatusCode, truncate(string(body), 120))
		}

		var out generateResp
		if err := json.Unmarshal(body, &out); err != nil {
			lastErr = err
			continue
		}
		link := strings.TrimSpace(out.Data.Attributes.ShortenedURL)
		if link == "" || !strings.Contains(link, "abre.ai/") {
			lastErr = fmt.Errorf("abre.ai bad response")
			continue
		}
		return link, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("abre.ai failed")
	}
	return "", lastErr
}

// UniqueDestination appends a random query param so each short maps to a unique target.
func UniqueDestination(base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return ""
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		sep := "?"
		if strings.Contains(base, "?") {
			sep = "&"
		}
		return base + sep + "_=" + randHex(8)
	}
	q := u.Query()
	q.Set("_", randHex(8))
	u.RawQuery = q.Encode()
	return u.String()
}

// ShortenBatch shortens up to count destinations derived from bases (round-robin).
func (c *Client) ShortenBatch(bases []string, count, concurrency int) []string {
	bases = cleanBases(bases)
	if len(bases) == 0 || count <= 0 {
		return nil
	}
	if concurrency <= 0 {
		concurrency = 6
	}
	if concurrency > count {
		concurrency = count
	}

	type job struct{ i int }
	jobs := make(chan job, count)
	for i := 0; i < count; i++ {
		jobs <- job{i: i}
	}
	close(jobs)

	out := make([]string, count)
	var sem = make(chan struct{}, concurrency)
	done := make(chan struct{}, count)

	for j := range jobs {
		sem <- struct{}{}
		go func(idx int) {
			defer func() { <-sem; done <- struct{}{} }()
			base := bases[idx%len(bases)]
			dest := UniqueDestination(base)
			link, err := c.Shorten(dest)
			if err == nil {
				out[idx] = link
			}
		}(j.i)
	}
	for i := 0; i < count; i++ {
		<-done
	}

	var ok []string
	for _, s := range out {
		if s != "" {
			ok = append(ok, s)
		}
	}
	return ok
}

func cleanBases(in []string) []string {
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		out = append(out, s)
	}
	return out
}

func randHex(n int) string {
	b := make([]byte, (n+1)/2)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:n]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
