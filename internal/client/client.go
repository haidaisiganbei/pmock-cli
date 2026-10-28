// Package client talks to the PMOCK open API.
//
// Every request is signed with the team AK/SK; responses use the unified
// envelope {traceId, code, msg, data, status}. Business failures are returned
// as *APIError so the command layer can render the envelope and choose an exit
// code without string matching.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/pmock/pmock-cli/internal/auth"
)

// Envelope is the unified PMOCK API response.
type Envelope struct {
	TraceID string          `json:"traceId"`
	Code    int             `json:"code"`
	Msg     string          `json:"msg"`
	Data    json.RawMessage `json:"data"`
	Status  string          `json:"status"`
}

// OK reports whether the business code indicates success.
func (e *Envelope) OK() bool { return e != nil && e.Code == 0 }

// APIError is a non-zero business code returned by the API.
type APIError struct {
	Code    int
	Msg     string
	TraceID string
}

func (e *APIError) Error() string {
	if e.TraceID != "" {
		return fmt.Sprintf("%s (code=%d, traceId=%s)", e.Msg, e.Code, e.TraceID)
	}
	return fmt.Sprintf("%s (code=%d)", e.Msg, e.Code)
}

// HTTPError is a transport-level failure that did not carry a usable envelope.
type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	body := strings.TrimSpace(e.Body)
	if len(body) > 300 {
		body = body[:300] + "..."
	}
	if body == "" {
		return fmt.Sprintf("HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, body)
}

// Request describes one API call.
type Request struct {
	Method string
	Path   string // spec path, starting with /open/api/
	Query  url.Values
	Body   any
}

// Client is safe for concurrent use.
type Client struct {
	BaseURL    string
	AccessKey  string
	SecretKey  string
	HTTP       *http.Client
	UserAgent  string
	MaxRetries int
	// RetryNonIdempotent also retries POSTs on 5xx/network errors. Off by
	// default because create/delete must not be duplicated.
	RetryNonIdempotent bool
}

// New builds a client with sane defaults.
func New(baseURL, accessKey, secretKey string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		AccessKey:  accessKey,
		SecretKey:  secretKey,
		HTTP:       &http.Client{Timeout: timeout},
		UserAgent:  "pmock-cli/dev",
		MaxRetries: 2,
	}
}

// Do executes a request and returns the parsed envelope. A non-zero business
// code is returned as *APIError together with the envelope, so callers can
// still print it.
func (c *Client) Do(ctx context.Context, req Request) (*Envelope, error) {
	var bodyBytes []byte
	if req.Body != nil {
		b, err := json.Marshal(req.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to encode request body: %w", err)
		}
		bodyBytes = b
	} else if !strings.EqualFold(req.Method, http.MethodGet) {
		bodyBytes = []byte("{}")
	}

	endpoint := c.endpoint(req.Path, req.Query)
	method := strings.ToUpper(req.Method)

	var lastErr error
	attempts := c.MaxRetries + 1
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			delay := backoff(attempt)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		env, retryAfter, err := c.once(ctx, method, endpoint, bodyBytes)
		if err == nil {
			if !env.OK() {
				return env, &APIError{Code: env.Code, Msg: env.Msg, TraceID: env.TraceID}
			}
			return env, nil
		}
		lastErr = err

		if !c.shouldRetry(err, method) || attempt == attempts-1 {
			break
		}
		if retryAfter > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryAfter):
			}
		}
	}
	return nil, lastErr
}

func (c *Client) once(ctx context.Context, method, endpoint string, body []byte) (*Envelope, time.Duration, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, 0, err
	}

	_, token := auth.Sign(c.AccessKey, c.SecretKey, time.Now())
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("pmock-access-key", c.AccessKey)
	httpReq.Header.Set("pmock-access-token", token)
	httpReq.Header.Set("User-Agent", c.UserAgent)
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read response: %w", err)
	}

	retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, retryAfter, &HTTPError{StatusCode: resp.StatusCode, Body: string(raw)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// PMOCK returns business failures with a 4xx/5xx status in some
		// deployments (invalid developer key is an HTTP 500 + code 10123).
		// A valid envelope is authoritative; only fall back to HTTPError when
		// the body is not one.
		var probe Envelope
		if json.Unmarshal(raw, &probe) == nil && probe.Code != 0 {
			return &probe, 0, nil
		}
		return nil, retryAfter, &HTTPError{StatusCode: resp.StatusCode, Body: string(raw)}
	}

	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		preview := string(raw)
		if len(preview) > 300 {
			preview = preview[:300] + "..."
		}
		return nil, 0, fmt.Errorf("failed to parse API response: %w (body: %s)", err, preview)
	}
	return &env, retryAfter, nil
}

func (c *Client) shouldRetry(err error, method string) bool {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.StatusCode {
		case http.StatusTooManyRequests:
			return true
		case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return method == http.MethodGet || c.RetryNonIdempotent
		default:
			return false
		}
	}
	// Network errors are retried for reads; writes only when explicitly asked.
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return false
	}
	return method == http.MethodGet || c.RetryNonIdempotent
}

func (c *Client) endpoint(path string, query url.Values) string {
	return BuildURL(c.BaseURL, path, query)
}

// BuildURL joins the configured base with a spec path. It accepts either
// https://host/api (spec paths kept) or the documented
// https://host/api/open/api (prefix stripped from the path).
func BuildURL(baseURL, path string, query url.Values) string {
	base := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(base, "/open/api") {
		base = strings.TrimSuffix(base, "/open/api")
		path = strings.TrimPrefix(path, "/open/api")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	u := base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	return u
}

func backoff(attempt int) time.Duration {
	base := time.Duration(250*(1<<uint(attempt-1))) * time.Millisecond
	jitter := time.Duration(rand.Int63n(int64(100 * time.Millisecond)))
	return base + jitter
}

func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

// IsAuthError reports whether a business code means bad/disabled credentials.
func IsAuthError(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Code == 10123
}
