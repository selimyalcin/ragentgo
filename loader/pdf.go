package loader

import (
	"bytes"
	"fmt"

	"github.com/ledongthuc/pdf"
	"github.com/selimyalcin/ragentgo/document"
)

func parsePDF(name string, data []byte, opts Options) ([]document.Document, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("pdf loader: %w", err)
	}
	n := r.NumPage()
	out := make([]document.Document, 0, n)
	for i := 1; i <= n; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		text = collapseWS(text)
		if text == "" {
			continue
		}
		meta := fileMeta(name)
		meta["kind"] = "pdf"
		meta["page"] = i
		out = append(out, newDoc("", text, name, opts.DefaultNS, meta))
	}
	return out, nil
}
