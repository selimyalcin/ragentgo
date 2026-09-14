package loader

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/selimyalcin/ragentgo/document"
)

// Loader turns a source path or URL into documents.
type Loader interface {
	Load(ctx context.Context, source string) ([]document.Document, error)
}

// Options apply to file and remote loaders.
type Options struct {
	MaxBytes     int64
	AllowPrivate bool
	AllowedHosts []string
	DefaultNS    string
	MIME         string
}

func (o Options) maxBytes() int64 {
	if o.MaxBytes <= 0 {
		return 8 << 20
	}
	return o.MaxBytes
}

// Detect returns a loader that inspects the file bytes (extension, MIME, magic).
func Detect(path string, opts Options) Loader {
	return fileLoader{opts: opts, extract: func(name string, data []byte) ([]document.Document, error) {
		return FromBytes(name, data, opts)
	}}
}

// FromPath loads a single file or walks a directory.
func FromPath(ctx context.Context, path string, opts Options) ([]document.Document, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return Directory(opts).Load(ctx, path)
	}
	return Detect(path, opts).Load(ctx, path)
}

func readLimited(r io.Reader, max int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("loader: source exceeds %d bytes", max)
	}
	return data, nil
}

func fileMeta(path string) map[string]any {
	base := filepath.Base(path)
	return map[string]any{
		"filename":  base,
		"source":    path,
		"extension": strings.ToLower(filepath.Ext(path)),
	}
}

func newDoc(id, content, source, ns string, meta map[string]any) document.Document {
	return document.Document{
		ID:        id,
		Content:   content,
		Source:    source,
		Namespace: ns,
		Metadata:  meta,
	}.Normalize()
}

// ValidateRemoteURL parses a remote HTTP(S) URL and rejects private hosts unless allowed.
func ValidateRemoteURL(ctx context.Context, source string, opts Options) (*url.URL, error) {
	u, err := url.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("url loader: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("url loader: unsupported scheme %q", u.Scheme)
	}
	if err := validateHost(ctx, u, opts); err != nil {
		return nil, err
	}
	return u, nil
}
