package retry

import (
	"context"
	"errors"
	"io"
	"math"
	"math/rand"
	"net"
	"strings"
	"syscall"
	"time"
)

// Policy controls how provider calls are retried.
type Policy struct {
	MaxAttempts int
	MinBackoff  time.Duration
	MaxBackoff  time.Duration
}

// DefaultPolicy retries a few times with jittered exponential backoff.
func DefaultPolicy() Policy {
	return Policy{
		MaxAttempts: 4,
		MinBackoff:  200 * time.Millisecond,
		MaxBackoff:  4 * time.Second,
	}
}

// Do runs fn until it succeeds, returns a non-retryable error, or the context ends.
func Do(ctx context.Context, p Policy, fn func(context.Context) error) error {
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = 1
	}
	if p.MinBackoff <= 0 {
		p.MinBackoff = 100 * time.Millisecond
	}
	if p.MaxBackoff < p.MinBackoff {
		p.MaxBackoff = p.MinBackoff
	}

	var last error
	for attempt := 1; attempt <= p.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		last = fn(ctx)
		if last == nil {
			return nil
		}
		if attempt == p.MaxAttempts || !IsRetryable(last) {
			return last
		}
		wait := backoff(p, attempt)
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return last
}

// IsRetryable reports whether the error is worth another attempt.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case syscall.ECONNRESET, syscall.ECONNREFUSED, syscall.EPIPE:
			return true
		}
	}
	var re *HTTPError
	if errors.As(err, &re) {
		switch re.StatusCode {
		case 429, 500, 502, 503, 504:
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{
		"eof",
		"connection reset",
		"connection refused",
		"broken pipe",
		"i/o timeout",
		"server closed idle connection",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

// HTTPError is a retry-aware HTTP failure.
type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	if e.Message == "" {
		return "http error"
	}
	return e.Message
}

func backoff(p Policy, attempt int) time.Duration {
	exp := float64(p.MinBackoff) * math.Pow(2, float64(attempt-1))
	if exp > float64(p.MaxBackoff) {
		exp = float64(p.MaxBackoff)
	}
	jitter := 0.5 + rand.Float64()
	return time.Duration(exp * jitter)
}
