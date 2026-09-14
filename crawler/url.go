package crawler

import (
	"net/url"
	"path"
	"strings"
)

var skipExt = map[string]struct{}{
	".jpg": {}, ".jpeg": {}, ".png": {}, ".gif": {}, ".webp": {}, ".svg": {}, ".ico": {},
	".mp4": {}, ".webm": {}, ".mp3": {}, ".wav": {}, ".pdf": {}, ".zip": {}, ".gz": {},
	".css": {}, ".js": {}, ".map": {}, ".woff": {}, ".woff2": {}, ".ttf": {}, ".eot": {},
	".xml": {}, ".rss": {},
}

// CanonicalURL strips fragments and tracking params so the same page is visited once.
func CanonicalURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" {
		u.Scheme = "https"
	}
	u.Fragment = ""
	u.Host = strings.ToLower(u.Host)
	q := u.Query()
	for _, key := range []string{"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content", "gclid", "fbclid", "mc_cid", "mc_eid"} {
		q.Del(key)
	}
	u.RawQuery = q.Encode()
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String(), nil
}

func sameSite(seed, candidate *url.URL) bool {
	a := siteKey(seed.Hostname())
	b := siteKey(candidate.Hostname())
	return a != "" && a == b
}

func siteKey(host string) string {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	return strings.TrimPrefix(host, "www.")
}

func shouldSkip(u *url.URL) bool {
	if u == nil {
		return true
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return true
	}
	p := strings.ToLower(u.Path)
	if strings.Contains(p, "/cdn-cgi/") || strings.Contains(p, "/wp-login") || strings.Contains(p, "/wp-admin") {
		return true
	}
	ext := strings.ToLower(path.Ext(u.Path))
	_, skip := skipExt[ext]
	return skip
}

func looksLikeChallenge(title, text string) bool {
	blob := strings.ToLower(title + "\n" + text)
	markers := []string{
		"just a moment",
		"attention required! | cloudflare",
		"checking your browser",
		"please wait while we verify",
		"verify you are human",
		"enable javascript and cookies to continue",
		"sorry, you have been blocked",
	}
	for _, m := range markers {
		if strings.Contains(blob, m) {
			return true
		}
	}
	if strings.Contains(strings.ToLower(title), "cloudflare") && len(strings.TrimSpace(text)) < 400 {
		return true
	}
	return false
}
