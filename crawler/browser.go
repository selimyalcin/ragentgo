package crawler

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

const desktopUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

type pageSnapshot struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Canonical   string   `json:"canonical"`
	Text        string   `json:"text"`
	Links       []string `json:"links"`
	JSONLD      []string `json:"jsonld"`
	Href        string   `json:"href"`
}

func allocatorOptions(cfg Config) []chromedp.ExecAllocatorOption {
	opts := []chromedp.ExecAllocatorOption{
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-popup-blocking", true),
		chromedp.Flag("disable-hang-monitor", true),
		chromedp.Flag("disable-prompt-on-repost", true),
		chromedp.Flag("disable-client-side-phishing-detection", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("metrics-recording-only", true),
		chromedp.Flag("safebrowsing-disable-auto-update", true),
		chromedp.Flag("password-store", "basic"),
		chromedp.Flag("use-mock-keychain", true),
		chromedp.Flag("enable-automation", false),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.WindowSize(1920, 1080),
		chromedp.UserAgent(desktopUA),
		chromedp.WSURLReadTimeout(90 * time.Second),
	}
	if cfg.Headless {
		opts = append(opts, chromedp.Flag("headless", "new"))
	}
	if p := resolveChromePath(cfg.ChromePath); p != "" {
		opts = append(opts, chromedp.ExecPath(p))
	}
	return opts
}

func resolveChromePath(configured string) string {
	if configured != "" {
		return configured
	}
	if p := os.Getenv("CHROME_PATH"); p != "" {
		return p
	}
	if p := os.Getenv("CHROMIUM_PATH"); p != "" {
		return p
	}
	for _, p := range []string{
		"/usr/bin/chromium-browser",
		"/usr/bin/chromium",
		"/usr/bin/google-chrome",
		"/usr/bin/google-chrome-stable",
	} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func fetchPage(parent context.Context, pageURL string, cfg Config) (pageSnapshot, error) {
	var snap pageSnapshot
	timeout := cfg.PageTimeout
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(stealthJS).Do(ctx)
			return err
		}),
		chromedp.Navigate(pageURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(800*time.Millisecond),
		chromedp.Evaluate(dismissConsentJS+";true", new(bool)),
		waitUntilReadable(cfg.ChallengeWait),
		chromedp.Evaluate(extractJS, &snap),
	)
	if err != nil {
		return snap, fmt.Errorf("render %s: %w", pageURL, err)
	}
	if looksLikeChallenge(snap.Title, snap.Text) {
		return snap, fmt.Errorf("render %s: page is still a bot interstitial", pageURL)
	}
	return snap, nil
}

func waitUntilReadable(budget time.Duration) chromedp.Action {
	if budget <= 0 {
		budget = 20 * time.Second
	}
	return chromedp.ActionFunc(func(ctx context.Context) error {
		deadline := time.Now().Add(budget)
		var snap pageSnapshot
		for {
			if err := chromedp.Evaluate(extractJS, &snap).Do(ctx); err != nil {
				return err
			}
			if !looksLikeChallenge(snap.Title, snap.Text) && strings.TrimSpace(snap.Text) != "" {
				return nil
			}
			if time.Now().After(deadline) {
				if looksLikeChallenge(snap.Title, snap.Text) {
					return fmt.Errorf("bot interstitial did not finish in %s", budget)
				}
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(800 * time.Millisecond):
			}
		}
	})
}
