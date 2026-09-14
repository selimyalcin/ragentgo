package crawler

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/chromedp/chromedp"
	"github.com/google/uuid"
	"github.com/selimyalcin/ragentgo/document"
	"github.com/selimyalcin/ragentgo/loader"
)

// Config controls a same-site browser crawl.
type Config struct {
	MaxPages      int
	MaxDepth      int
	Delay         time.Duration
	PageTimeout   time.Duration
	ChallengeWait time.Duration
	Headless      bool
	ChromePath    string
	Namespace     string
	MaxBytes      int64
	AllowPrivate  bool
	AllowedHosts  []string
}

// Result is what a crawl produced before indexing.
type Result struct {
	StartURL     string              `json:"start_url"`
	PagesVisited int                 `json:"pages_visited"`
	PagesSkipped int                 `json:"pages_skipped"`
	FailedURLs   []string            `json:"failed_urls,omitempty"`
	Documents    []document.Document `json:"-"`
}

type queueItem struct {
	URL   string
	Depth int
}

// Crawl visits public pages on the seed host with a real Chrome session.
func Crawl(ctx context.Context, start string, cfg Config) (*Result, error) {
	cfg = cfg.withDefaults()
	start, err := CanonicalURL(start)
	if err != nil {
		return nil, fmt.Errorf("crawl: %w", err)
	}
	seed, err := loader.ValidateRemoteURL(ctx, start, loader.Options{
		AllowPrivate: cfg.AllowPrivate,
		AllowedHosts: cfg.AllowedHosts,
		MaxBytes:     cfg.MaxBytes,
	})
	if err != nil {
		return nil, err
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, allocatorOptions(cfg)...)
	defer allocCancel()
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()
	if err := chromedp.Run(browserCtx); err != nil {
		return nil, fmt.Errorf("start chrome: %w", err)
	}

	res := &Result{StartURL: seed.String()}
	seen := map[string]struct{}{}
	queue := []queueItem{{URL: seed.String(), Depth: 0}}

	for len(queue) > 0 && res.PagesVisited < cfg.MaxPages {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		item := queue[0]
		queue = queue[1:]
		if _, ok := seen[item.URL]; ok {
			continue
		}
		seen[item.URL] = struct{}{}

		u, err := url.Parse(item.URL)
		if err != nil || shouldSkip(u) || !sameSite(seed, u) {
			res.PagesSkipped++
			continue
		}

		snap, err := fetchPage(browserCtx, item.URL, cfg)
		if err != nil {
			res.FailedURLs = append(res.FailedURLs, item.URL+": "+err.Error())
			continue
		}
		doc, ok := documentFromSnapshot(snap, cfg)
		if !ok {
			res.PagesSkipped++
		} else {
			res.Documents = append(res.Documents, doc)
			res.PagesVisited++
		}

		if item.Depth < cfg.MaxDepth {
			for _, href := range snap.Links {
				canon, err := CanonicalURL(href)
				if err != nil {
					continue
				}
				lu, err := url.Parse(canon)
				if err != nil || shouldSkip(lu) || !sameSite(seed, lu) {
					continue
				}
				if _, ok := seen[canon]; ok {
					continue
				}
				queue = append(queue, queueItem{URL: canon, Depth: item.Depth + 1})
			}
		}
		if cfg.Delay > 0 {
			select {
			case <-ctx.Done():
				return res, ctx.Err()
			case <-time.After(cfg.Delay):
			}
		}
	}
	if len(res.Documents) == 0 {
		if len(res.FailedURLs) > 0 {
			return res, fmt.Errorf("crawl: no readable pages (%s)", res.FailedURLs[0])
		}
		return res, fmt.Errorf("crawl: no readable pages at %s", start)
	}
	return res, nil
}

// Fetch loads a single URL in Chrome. Used by the one-shot URL ingest endpoint.
func Fetch(ctx context.Context, raw string, cfg Config) (document.Document, error) {
	cfg = cfg.withDefaults()
	cfg.MaxPages = 1
	cfg.MaxDepth = 0
	res, err := Crawl(ctx, raw, cfg)
	if err != nil {
		return document.Document{}, err
	}
	if len(res.Documents) == 0 {
		return document.Document{}, fmt.Errorf("fetch: empty page")
	}
	return res.Documents[0], nil
}

func (c Config) withDefaults() Config {
	if c.MaxPages <= 0 {
		c.MaxPages = 40
	}
	if c.MaxDepth < 0 {
		c.MaxDepth = 3
	}
	if c.MaxPages > 1 && c.MaxDepth == 0 {
		c.MaxDepth = 3
	}
	if c.Delay <= 0 {
		c.Delay = 400 * time.Millisecond
	}
	if c.PageTimeout <= 0 {
		c.PageTimeout = 45 * time.Second
	}
	if c.ChallengeWait <= 0 {
		c.ChallengeWait = 20 * time.Second
	}
	if c.MaxBytes <= 0 {
		c.MaxBytes = 8 << 20
	}
	if c.Namespace == "" {
		c.Namespace = document.DefaultNamespace
	}
	c.Headless = true
	return c
}

func documentFromSnapshot(snap pageSnapshot, cfg Config) (document.Document, bool) {
	body := strings.TrimSpace(snap.Text)
	if utf8.RuneCountInString(body) < 80 && len(snap.JSONLD) == 0 && strings.TrimSpace(snap.Description) == "" {
		return document.Document{}, false
	}
	source := snap.Canonical
	if source == "" {
		source = snap.Href
	}
	var b strings.Builder
	if snap.Title != "" {
		fmt.Fprintf(&b, "Title: %s\n", snap.Title)
	}
	fmt.Fprintf(&b, "URL: %s\n", source)
	if snap.Description != "" {
		fmt.Fprintf(&b, "Description: %s\n", snap.Description)
	}
	b.WriteByte('\n')
	b.WriteString(body)
	if len(snap.JSONLD) > 0 {
		b.WriteString("\n\nStructured data:\n")
		b.WriteString(strings.Join(snap.JSONLD, "\n"))
	}
	content := b.String()
	if int64(len(content)) > cfg.MaxBytes {
		content = content[:cfg.MaxBytes]
	}
	id := uuid.NewSHA1(uuid.NameSpaceURL, []byte(source)).String()
	doc := document.Document{
		ID:        id,
		Content:   content,
		Source:    source,
		Namespace: cfg.Namespace,
		Metadata: map[string]any{
			"source":   source,
			"url":      source,
			"title":    snap.Title,
			"filename": source,
			"kind":     "web_crawl",
		},
	}.Normalize()
	doc.ID = id
	return doc, true
}
