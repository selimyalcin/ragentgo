package crawler

import (
	"net/url"
	"testing"
)

func TestCanonicalURLStripsTracking(t *testing.T) {
	got, err := CanonicalURL("https://WWW.Example.com/about/?utm_source=x&id=1#top")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://www.example.com/about/?id=1"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSameSiteTreatsWWWAsEqual(t *testing.T) {
	a, _ := url.Parse("https://acme.com/about")
	b, _ := url.Parse("https://www.acme.com/team")
	if !sameSite(a, b) {
		t.Fatal("www and apex should be the same site")
	}
	c, _ := url.Parse("https://other.com/")
	if sameSite(a, c) {
		t.Fatal("different hosts must not match")
	}
}

func TestShouldSkipAssetsAndSchemes(t *testing.T) {
	cases := map[string]bool{
		"https://acme.com/logo.png":    true,
		"mailto:hi@acme.com":           true,
		"https://acme.com/cdn-cgi/l/x": true,
		"https://acme.com/about":       false,
		"https://acme.com/docs/intro":  false,
	}
	for raw, skip := range cases {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if shouldSkip(u) != skip {
			t.Fatalf("%s skip=%v want %v", raw, shouldSkip(u), skip)
		}
	}
}

func TestLooksLikeChallenge(t *testing.T) {
	if !looksLikeChallenge("Just a moment...", "Enable JavaScript and cookies to continue") {
		t.Fatal("expected challenge")
	}
	if looksLikeChallenge("Acme — About", "We build widgets for industrial customers since 1998.") {
		t.Fatal("real page flagged as challenge")
	}
}
