package loader

import (
	"context"
	"os"
	"path/filepath"

	"github.com/selimyalcin/ragentgo/document"
)

type fileLoader struct {
	opts    Options
	extract func(name string, data []byte) ([]document.Document, error)
}

func (f fileLoader) Load(ctx context.Context, source string) ([]document.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	fh, err := os.Open(source)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	data, err := readLimited(fh, f.opts.maxBytes())
	if err != nil {
		return nil, err
	}
	if f.extract != nil {
		return f.extract(source, data)
	}
	meta := fileMeta(source)
	return []document.Document{newDoc("", string(data), source, f.opts.DefaultNS, meta)}, nil
}

// Text reads a file as UTF-8 text.
func Text(opts Options) Loader { return fileLoader{opts: opts} }

// Markdown reads a Markdown file. Parsing is left to the chunker.
func Markdown(opts Options) Loader { return fileLoader{opts: opts} }

type dirLoader struct{ opts Options }

// Directory walks a folder and loads recognized files.
func Directory(opts Options) Loader { return dirLoader{opts: opts} }

func (d dirLoader) Load(ctx context.Context, source string) ([]document.Document, error) {
	var out []document.Document
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			base := entry.Name()
			if base == ".git" || base == "node_modules" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if kindFromExt(filepath.Ext(path)) == "" {
			return nil
		}
		docs, err := Detect(path, d.opts).Load(ctx, path)
		if err != nil {
			return err
		}
		out = append(out, docs...)
		return nil
	})
	return out, err
}
