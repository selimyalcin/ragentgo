package retry

import (
	"context"
	"errors"
	"io"
	"net/url"
	"testing"
	"time"
)

func TestDoSucceedsAfterRetry(t *testing.T) {
	attempts := 0
	err := Do(context.Background(), Policy{MaxAttempts: 3, MinBackoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond}, func(context.Context) error {
		attempts++
		if attempts < 3 {
			return &HTTPError{StatusCode: 503, Message: "busy"}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d", attempts)
	}
}

func TestDoDoesNotRetryClientErrors(t *testing.T) {
	attempts := 0
	err := Do(context.Background(), Policy{MaxAttempts: 4, MinBackoff: time.Millisecond, MaxBackoff: time.Millisecond}, func(context.Context) error {
		attempts++
		return &HTTPError{StatusCode: 400, Message: "bad request"}
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d", attempts)
	}
}

func TestDoRetriesEOF(t *testing.T) {
	attempts := 0
	err := Do(context.Background(), Policy{MaxAttempts: 3, MinBackoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond}, func(context.Context) error {
		attempts++
		if attempts < 3 {
			return &url.Error{Op: "Post", URL: "http://x/v1/chat/completions", Err: io.EOF}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d", attempts)
	}
}

func TestIsRetryableEOF(t *testing.T) {
	err := &url.Error{Op: "Post", URL: "http://127.0.0.1:8000/v1/chat/completions", Err: io.EOF}
	if !IsRetryable(err) {
		t.Fatal("EOF from a local gateway must be retried")
	}
	if !IsRetryable(io.ErrUnexpectedEOF) {
		t.Fatal("unexpected EOF must be retried")
	}
	if IsRetryable(context.Canceled) {
		t.Fatal("canceled context must not be retried")
	}
}

func TestDoHonorsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Do(ctx, DefaultPolicy(), func(context.Context) error {
		return errors.New("should not run")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}
