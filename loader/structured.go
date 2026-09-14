package loader

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/selimyalcin/ragentgo/document"
	"golang.org/x/net/html"
)

// JSON parses objects, arrays, or raw JSON as documents.
func JSON(opts Options) Loader {
	return fileLoader{opts: opts, extract: func(name string, data []byte) ([]document.Document, error) {
		return parseJSON(name, data, opts)
	}}
}

func parseJSON(name string, data []byte, opts Options) ([]document.Document, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("json loader: %w", err)
	}
	meta := fileMeta(name)
	switch t := v.(type) {
	case []any:
		out := make([]document.Document, 0, len(t))
		for i, item := range t {
			content, extra := jsonItem(item)
			m := document.CloneMetadata(meta)
			for k, val := range extra {
				m[k] = val
			}
			m["index"] = i
			out = append(out, newDoc("", content, name, opts.DefaultNS, m))
		}
		return out, nil
	default:
		content, extra := jsonItem(v)
		for k, val := range extra {
			meta[k] = val
		}
		return []document.Document{newDoc("", content, name, opts.DefaultNS, meta)}, nil
	}
}

func jsonItem(v any) (string, map[string]any) {
	if s, ok := v.(string); ok {
		return s, nil
	}
	if m, ok := v.(map[string]any); ok {
		for _, key := range []string{"content", "text", "body"} {
			if s, ok := m[key].(string); ok && s != "" {
				extra := map[string]any{}
				if id, ok := m["id"].(string); ok {
					extra["id"] = id
				}
				return s, extra
			}
		}
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b), nil
}

// CSV turns each row into a document, joining columns as "header: value".
func CSV(opts Options) Loader {
	return fileLoader{opts: opts, extract: func(name string, data []byte) ([]document.Document, error) {
		return parseCSV(name, data, opts, ',')
	}}
}

func parseCSV(name string, data []byte, opts Options, comma rune) ([]document.Document, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = comma
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	header, err := r.Read()
	if err != nil {
		return nil, err
	}
	var out []document.Document
	idx := 0
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		line := formatTableRow(header, row)
		if line == "" {
			continue
		}
		var b strings.Builder
		if rec := compactRowValues(row); rec != "" {
			fmt.Fprintf(&b, "Record: %s\n", rec)
		}
		b.WriteString(line)
		meta := fileMeta(name)
		meta["index"] = idx
		meta["kind"] = "csv"
		meta["unit"] = "row"
		out = append(out, newDoc("", strings.TrimSpace(b.String()), name, opts.DefaultNS, meta))
		idx++
	}
	return out, nil
}

func formatTableRow(header, row []string) string {
	var b strings.Builder
	if len(header) == 0 {
		parts := make([]string, 0, len(row))
		for _, cell := range row {
			cell = strings.TrimSpace(cell)
			if cell != "" {
				parts = append(parts, cell)
			}
		}
		return strings.Join(parts, " | ")
	}
	for i, col := range header {
		if i >= len(row) {
			break
		}
		val := strings.TrimSpace(row[i])
		if val == "" {
			continue
		}
		col = strings.TrimSpace(col)
		if col == "" {
			fmt.Fprintf(&b, "%s\n", val)
			continue
		}
		fmt.Fprintf(&b, "%s: %s\n", col, val)
	}
	return strings.TrimSpace(b.String())
}

// HTML extracts visible text from an HTML file.
func HTML(opts Options) Loader {
	return fileLoader{opts: opts, extract: func(name string, data []byte) ([]document.Document, error) {
		text, err := extractHTML(data)
		if err != nil {
			return nil, err
		}
		return []document.Document{newDoc("", text, name, opts.DefaultNS, fileMeta(name))}, nil
	}}
}

func extractHTML(data []byte) (string, error) {
	node, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "noscript":
				return
			}
		}
		if n.Type == html.TextNode {
			t := strings.TrimSpace(n.Data)
			if t != "" {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.WriteString(t)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(node)
	return b.String(), nil
}
