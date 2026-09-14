package loader

import (
	"context"
	"fmt"
	"github.com/selimyalcin/ragentgo/document"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// Web fetches a remote URL and extracts text. Private networks are blocked by default.
func Web(opts Options) Loader {
	return webLoader{opts: opts}
}

type webLoader struct{ opts Options }

func (w webLoader) Load(ctx context.Context, source string) ([]document.Document, error) {
	u, err := url.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("url loader: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("url loader: unsupported scheme %q", u.Scheme)
	}
	if err := validateHost(ctx, u, w.opts); err != nil {
		return nil, err
	}
	client := &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("url loader: too many redirects")
			}
			return validateHost(req.Context(), req.URL, w.opts)
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ragentgo/0.1")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("url loader: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("url loader: status %d", resp.StatusCode)
	}
	data, err := readLimited(resp.Body, w.opts.maxBytes())
	if err != nil {
		return nil, err
	}
	ctype := resp.Header.Get("Content-Type")
	name := u.Path
	if name == "" || strings.HasSuffix(name, "/") {
		name = "download"
	}
	opts := w.opts
	opts.MIME = ctype
	if kindFromExt(path.Ext(name)) == "" && kindFromMIME(ctype) == "" {
		if strings.Contains(ctype, "html") {
			name += ".html"
		}
	}
	docs, err := FromBytes(name, data, opts)
	if err != nil {
		return nil, err
	}
	for i := range docs {
		if docs[i].Metadata == nil {
			docs[i].Metadata = map[string]any{}
		}
		docs[i].Metadata["url"] = source
		docs[i].Metadata["mime"] = ctype
		if docs[i].Source == name {
			docs[i].Source = source
		}
	}
	return docs, nil
}

func validateHost(ctx context.Context, u *url.URL, opts Options) error {
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("url loader: missing host")
	}
	if len(opts.AllowedHosts) > 0 {
		ok := false
		for _, allowed := range opts.AllowedHosts {
			if strings.EqualFold(host, allowed) {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("url loader: host %q is not allowlisted", host)
		}
	}
	if opts.AllowPrivate {
		return nil
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("url loader: resolve %s: %w", host, err)
	}
	for _, ip := range ips {
		if isPrivate(ip.IP) {
			return fmt.Errorf("url loader: host %q resolves to a private address", host)
		}
	}
	return nil
}

func isPrivate(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
	}
	return false
}
