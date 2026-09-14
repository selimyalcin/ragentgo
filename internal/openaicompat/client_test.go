package openaicompat

import (
	"net/http"
	"testing"
	"time"
)

func TestHTTPSClientAllowsHTTP2(t *testing.T) {
	c := New(Options{BaseURL: "https://router.huggingface.co/v1", Timeout: time.Second})
	tr, ok := c.HTTP().Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected *http.Transport")
	}
	if tr.DisableKeepAlives {
		t.Fatal("https should keep-alive and speak HTTP/2")
	}
	if tr.ForceAttemptHTTP2 == false {
		t.Fatal("https must not force HTTP/1-only")
	}
}

func TestPlainHTTPDisablesHTTP2(t *testing.T) {
	c := New(Options{BaseURL: "http://localhost:8000/v1", Timeout: time.Second})
	tr, ok := c.HTTP().Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected *http.Transport")
	}
	if !tr.DisableKeepAlives || tr.ForceAttemptHTTP2 {
		t.Fatal("local http should use HTTP/1 without keep-alive")
	}
}
