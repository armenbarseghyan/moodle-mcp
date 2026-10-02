// Package moodle is a read-only client for the Moodle REST web service API.
//
// It knows nothing about MCP, markdown or caching: it sends requests, retries
// transient failures, recognises Moodle's HTTP-200 exception responses and
// decodes the payload into wire types.
package moodle

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	restPath         = "/webservice/rest/server.php"
	defaultTimeout   = 20 * time.Second
	defaultDLTimeout = 5 * time.Minute
	defaultInFlight  = 5
	maxBodyBytes     = 32 << 20
	maxAttempts      = 2 // one retry
	userAgent        = "moodle-mcp"
)

// Config configures a Client. BaseURL and Token are required.
type Config struct {
	BaseURL string
	Token   string

	// HTTPClient is used for REST calls. Default: 20s timeout.
	HTTPClient *http.Client
	// DownloadClient is used for file downloads. Default: no overall timeout
	// (DownloadTimeout bounds the whole transfer), 20s for response headers.
	DownloadClient  *http.Client
	DownloadTimeout time.Duration

	// Logger receives one record per HTTP attempt. Token values are redacted.
	// Default: discard.
	Logger *slog.Logger

	// Backoff returns the delay before the given retry attempt (2, 3, ...).
	// Default: 300ms ± 30%.
	Backoff func(attempt int) time.Duration

	// MaxInFlight bounds concurrent HTTP requests across the whole client.
	// Default: 5.
	MaxInFlight int
}

// Client is safe for concurrent use.
type Client struct {
	base      *url.URL
	token     string
	redactor  *Redactor
	http      *http.Client
	dl        *http.Client
	dlTimeout time.Duration
	log       *slog.Logger
	backoff   func(int) time.Duration
	sem       chan struct{}
}

// New validates cfg and returns a Client. It does not contact Moodle.
func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, errors.New("moodle: token is empty")
	}
	base, err := url.Parse(strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"))
	if err != nil {
		return nil, fmt.Errorf("moodle: invalid base URL: %w", err)
	}
	if base.Scheme != "https" && base.Scheme != "http" || base.Host == "" {
		return nil, fmt.Errorf("moodle: base URL must be http(s)://host[/path], got %q", cfg.BaseURL)
	}
	if base.RawQuery != "" || base.Fragment != "" {
		return nil, fmt.Errorf("moodle: base URL must not contain query or fragment")
	}

	c := &Client{
		base:      base,
		token:     cfg.Token,
		redactor:  NewRedactor(cfg.Token),
		http:      cfg.HTTPClient,
		dl:        cfg.DownloadClient,
		dlTimeout: cfg.DownloadTimeout,
		backoff:   cfg.Backoff,
	}
	if c.http == nil {
		c.http = &http.Client{Timeout: defaultTimeout}
	}
	if c.dl == nil {
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.ResponseHeaderTimeout = defaultTimeout
		c.dl = &http.Client{Transport: tr}
	}
	// Downloads carry the token in the query string: never follow a redirect
	// to another host. Copy the client so a caller-supplied one is not mutated.
	dl := *c.dl
	dl.CheckRedirect = c.checkRedirect
	c.dl = &dl
	if c.dlTimeout <= 0 {
		c.dlTimeout = defaultDLTimeout
	}
	if c.backoff == nil {
		c.backoff = defaultBackoff
	}
	n := cfg.MaxInFlight
	if n <= 0 {
		n = defaultInFlight
	}
	c.sem = make(chan struct{}, n)

	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	c.log = slog.New(NewRedactingHandler(logger.Handler(), cfg.Token))
	return c, nil
}

// BaseURL returns a copy of the configured site URL.
func (c *Client) BaseURL() *url.URL {
	u := *c.base
	return &u
}

// Redact removes the client's token from s.
func (c *Client) Redact(s string) string { return c.redactor.String(s) }

func defaultBackoff(int) time.Duration {
	const base = 300 * time.Millisecond
	jitter := time.Duration((rand.Float64()*0.6 - 0.3) * float64(base)) //nolint:gosec // retry jitter, not security
	return base + jitter
}

// Call invokes a Moodle web service function and decodes the result into out
// (which may be nil). Only allowlisted read-only functions can be called.
func (c *Client) Call(ctx context.Context, fn string, params url.Values, out any) error {
	if !Allowed(fn) {
		return fmt.Errorf("%w: %s", ErrNotAllowed, fn)
	}
	form := url.Values{}
	for k, v := range params {
		form[k] = v
	}
	form.Set("wstoken", c.token)
	form.Set("wsfunction", fn)
	form.Set("moodlewsrestformat", "json")

	body, err := c.post(ctx, fn, form.Encode())
	if err != nil {
		return err
	}
	return c.decode(fn, body, out)
}

func (c *Client) post(ctx context.Context, fn, payload string) ([]byte, error) {
	endpoint := c.base.JoinPath(restPath).String()
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			if err := sleep(ctx, c.backoff(attempt)); err != nil {
				return nil, err
			}
		}
		start := time.Now()
		body, status, err := c.postOnce(ctx, endpoint, payload)
		dur := time.Since(start)

		if err == nil && status >= 200 && status < 300 {
			c.log.Debug("moodle call", "fn", fn, "attempt", attempt, "status", status,
				"dur_ms", dur.Milliseconds(), "bytes", len(body))
			return body, nil
		}

		retry := c.retryable(ctx, status, err)
		if err != nil {
			lastErr = fmt.Errorf("moodle: %s: %w", fn, err)
		} else {
			// A Moodle exception can arrive with a non-2xx status too.
			if mErr := c.parseException(fn, body); mErr != nil {
				lastErr = mErr
				retry = false
			} else {
				lastErr = &HTTPError{Function: fn, Status: status, Snippet: c.snippet(body)}
			}
		}
		c.log.Warn("moodle call failed", "fn", fn, "attempt", attempt, "status", status,
			"dur_ms", dur.Milliseconds(), "retry", retry && attempt < maxAttempts, "err", lastErr)
		if !retry {
			break
		}
	}
	return nil, lastErr
}

func (c *Client) postOnce(ctx context.Context, endpoint, payload string) ([]byte, int, error) {
	if err := c.acquire(ctx); err != nil {
		return nil, 0, err
	}
	defer c.release()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(payload))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if len(body) > maxBodyBytes {
		return nil, resp.StatusCode, fmt.Errorf("response larger than %d bytes", maxBodyBytes)
	}
	return body, resp.StatusCode, nil
}

// retryable: network errors and 5xx get one more attempt, unless the caller's
// context is done.
func (c *Client) retryable(ctx context.Context, status int, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	if err != nil {
		return !errors.Is(err, context.Canceled)
	}
	return status >= 500
}

func (c *Client) acquire(ctx context.Context) error {
	select {
	case c.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) release() { <-c.sem }

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) snippet(body []byte) string {
	const n = 200
	s := string(bytes.TrimSpace(body))
	if len([]rune(s)) > n {
		s = string([]rune(s)[:n]) + "…"
	}
	return c.Redact(s)
}
