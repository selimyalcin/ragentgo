package openaicompat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/selimyalcin/ragentgo/internal/retry"
	"golang.org/x/sync/semaphore"
)

// Client talks to any OpenAI-compatible HTTP API (vLLM, LocalAI, llama.cpp,
// LM Studio, or a local gateway such as http://localhost:8000/v1).
type Client struct {
	baseURL  string
	apiKey   string
	http     *http.Client
	retry    retry.Policy
	sem      *semaphore.Weighted
	maxBatch int
	h1Close  bool
}

// Options configure an OpenAI-compatible client.
type Options struct {
	BaseURL        string
	APIKey         string
	Model          string
	Dimension      int
	Timeout        time.Duration
	MaxConcurrency int
	MaxBatch       int
	Retry          retry.Policy
	HTTPClient     *http.Client
}

// New returns a client pointed at an OpenAI-compatible /v1 endpoint.
func New(opts Options) *Client {
	if opts.BaseURL == "" {
		opts.BaseURL = "http://localhost:8000/v1"
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 60 * time.Second
	}
	if opts.MaxConcurrency <= 0 {
		opts.MaxConcurrency = 4
	}
	if opts.MaxBatch <= 0 {
		opts.MaxBatch = 64
	}
	if opts.Retry.MaxAttempts == 0 {
		opts.Retry = retry.DefaultPolicy()
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = newHTTPClient(opts.Timeout, isPlainHTTP(opts.BaseURL))
	}
	return &Client{
		baseURL:  strings.TrimRight(opts.BaseURL, "/"),
		apiKey:   opts.APIKey,
		http:     httpClient,
		retry:    opts.Retry,
		sem:      semaphore.NewWeighted(int64(opts.MaxConcurrency)),
		maxBatch: opts.MaxBatch,
		h1Close:  isPlainHTTP(opts.BaseURL),
	}
}

// BaseURL is the /v1 root, without a trailing slash.
func (c *Client) BaseURL() string { return c.baseURL }

// APIKey is sent as a Bearer token when non-empty.
func (c *Client) APIKey() string { return c.apiKey }

// HTTP returns the underlying HTTP client.
func (c *Client) HTTP() *http.Client { return c.http }

// MaxBatch is the preferred embedding batch size.
func (c *Client) MaxBatch() int { return c.maxBatch }

// Acquire takes a concurrency slot.
func (c *Client) Acquire(ctx context.Context) error {
	return c.sem.Acquire(ctx, 1)
}

// Release frees a concurrency slot.
func (c *Client) Release() {
	c.sem.Release(1)
}

// Post JSON-encodes payload and POSTs it to path (for example "/embeddings").
func (c *Client) Post(ctx context.Context, path string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var out []byte
	err = retry.Do(ctx, c.retry, func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		c.writeAuth(req)
		if c.h1Close {
			req.Header.Set("Connection", "close")
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if resp.StatusCode >= 400 {
			return &retry.HTTPError{
				StatusCode: resp.StatusCode,
				Message:    fmt.Sprintf("openai-compat %s: status %d: %s", path, resp.StatusCode, strings.TrimSpace(string(raw))),
			}
		}
		out = raw
		return nil
	})
	return out, err
}

// WriteHeaders sets auth and, for local HTTP/1 gateways, Connection: close.
func (c *Client) WriteHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	c.writeAuth(req)
	if c.h1Close {
		req.Header.Set("Connection", "close")
	}
}

func (c *Client) writeAuth(req *http.Request) {
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
}

func isPlainHTTP(base string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(base)), "http://")
}

func newHTTPClient(timeout time.Duration, localHTTP1 bool) *http.Client {
	var tr *http.Transport
	if base, ok := http.DefaultTransport.(*http.Transport); ok {
		tr = base.Clone()
	} else {
		tr = &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout: 10 * time.Second,
		}
	}
	tr.ExpectContinueTimeout = 0
	tr.IdleConnTimeout = 30 * time.Second
	if localHTTP1 {
		// Local llama.cpp / Jan gateways often EOF on reused HTTP/2 or keep-alive.
		tr.ForceAttemptHTTP2 = false
		tr.DisableKeepAlives = true
	} else {
		tr.ForceAttemptHTTP2 = true
		tr.DisableKeepAlives = false
	}
	return &http.Client{Timeout: timeout, Transport: tr}
}
