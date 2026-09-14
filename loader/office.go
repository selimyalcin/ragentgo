package loader

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strings"
	"unicode"

	"github.com/selimyalcin/ragentgo/document"
	"golang.org/x/net/html/charset"
)

func parseDOCX(name string, data []byte, opts Options) ([]document.Document, error) {
	text, err := zipXMLText(data, func(name string) bool {
		base := path.Base(name)
		dir := path.Dir(name)
		if dir != "word" {
			return false
		}
		switch {
		case base == "document.xml", base == "footnotes.xml", base == "endnotes.xml", base == "comments.xml":
			return true
		case strings.HasPrefix(base, "header") && strings.HasSuffix(base, ".xml"):
			return true
		case strings.HasPrefix(base, "footer") && strings.HasSuffix(base, ".xml"):
			return true
		default:
			return false
		}
	}, map[string]bool{"t": true}, map[string]bool{"p": true, "br": true, "tab": true})
	if err != nil {
		return nil, fmt.Errorf("docx loader: %w", err)
	}
	meta := fileMeta(name)
	meta["kind"] = "docx"
	return []document.Document{newDoc("", text, name, opts.DefaultNS, meta)}, nil
}

func parsePPTX(name string, data []byte, opts Options) ([]document.Document, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("pptx loader: %w", err)
	}
	var out []document.Document
	slide := 0
	for _, f := range zr.File {
		base := path.Base(f.Name)
		if path.Dir(f.Name) != "ppt/slides" || !strings.HasPrefix(base, "slide") || !strings.HasSuffix(base, ".xml") {
			continue
		}
		body, err := readZipFile(f)
		if err != nil {
			return nil, err
		}
		text := xmlRuns(body, map[string]bool{"t": true}, map[string]bool{"p": true, "br": true})
		if strings.TrimSpace(text) == "" {
			continue
		}
		slide++
		meta := fileMeta(name)
		meta["kind"] = "pptx"
		meta["slide"] = slide
		out = append(out, newDoc("", text, name, opts.DefaultNS, meta))
	}
	return out, nil
}

func parseODT(name string, data []byte, opts Options) ([]document.Document, error) {
	body, err := zipNamedFile(data, "content.xml")
	if err != nil {
		return nil, fmt.Errorf("odt loader: %w", err)
	}
	text := xmlRuns(body, map[string]bool{"p": true, "h": true, "span": true}, map[string]bool{"p": true, "h": true})
	meta := fileMeta(name)
	meta["kind"] = "odt"
	return []document.Document{newDoc("", text, name, opts.DefaultNS, meta)}, nil
}

func parseDOC(name string, data []byte, opts Options) ([]document.Document, error) {
	text, err := extractOLEText(data)
	if err != nil {
		return nil, fmt.Errorf("doc loader: %w", err)
	}
	meta := fileMeta(name)
	meta["kind"] = "doc"
	return []document.Document{newDoc("", text, name, opts.DefaultNS, meta)}, nil
}

func parsePPT(name string, data []byte, opts Options) ([]document.Document, error) {
	text, err := extractOLEText(data)
	if err != nil {
		return nil, fmt.Errorf("ppt loader: %w", err)
	}
	meta := fileMeta(name)
	meta["kind"] = "ppt"
	return []document.Document{newDoc("", text, name, opts.DefaultNS, meta)}, nil
}

func parseRTF(name string, data []byte, opts Options) ([]document.Document, error) {
	text := rtfText(string(data))
	meta := fileMeta(name)
	meta["kind"] = "rtf"
	return []document.Document{newDoc("", text, name, opts.DefaultNS, meta)}, nil
}

func zipXMLText(data []byte, match func(string) bool, take, breaks map[string]bool) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, f := range zr.File {
		if !match(f.Name) {
			continue
		}
		body, err := readZipFile(f)
		if err != nil {
			return "", err
		}
		s := xmlRuns(body, take, breaks)
		if strings.TrimSpace(s) == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(s)
	}
	return strings.TrimSpace(b.String()), nil
}

func zipNamedFile(data []byte, name string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if f.Name == name {
			return readZipFile(f)
		}
	}
	return nil, fmt.Errorf("zip: %s not found", name)
}

func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, 32<<20))
}

func xmlRuns(data []byte, take, breaks map[string]bool) string {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = charset.NewReaderLabel
	dec.Strict = false
	var (
		b    strings.Builder
		want int
	)
	flushBreak := func(local string) {
		if local == "p" || local == "h" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			return
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			local := t.Name.Local
			if take[local] {
				want++
			}
			if breaks[local] && (local == "br" || local == "tab") {
				flushBreak(local)
			}
		case xml.EndElement:
			local := t.Name.Local
			if take[local] && want > 0 {
				want--
			}
			if breaks[local] && (local == "p" || local == "h") {
				flushBreak(local)
			}
		case xml.CharData:
			if want > 0 {
				s := strings.ReplaceAll(string(t), "\u00a0", " ")
				b.WriteString(s)
			}
		}
	}
	return strings.TrimSpace(collapseWS(b.String()))
}

func odsTables(data []byte) []odsSheet {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = charset.NewReaderLabel
	dec.Strict = false
	var (
		sheets []odsSheet
		cur    = -1
		row    []string
		cell   strings.Builder
		inCell bool
		inText bool
		repeat int
	)
	attr := func(t xml.StartElement, local string) string {
		for _, a := range t.Attr {
			if a.Name.Local == local {
				return a.Value
			}
		}
		return ""
	}
	flushCell := func() {
		val := strings.TrimSpace(cell.String())
		n := repeat
		if n <= 0 {
			n = 1
		}
		if n > 32 {
			n = 32
		}
		for i := 0; i < n; i++ {
			row = append(row, val)
		}
		cell.Reset()
		repeat = 0
		inCell = false
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "table":
				sh := odsSheet{name: attr(t, "name")}
				if sh.name == "" {
					sh.name = fmt.Sprintf("Sheet%d", len(sheets)+1)
				}
				sheets = append(sheets, sh)
				cur = len(sheets) - 1
			case "table-row":
				row = nil
			case "table-cell", "covered-table-cell":
				inCell = true
				repeat = atoiDefault(attr(t, "number-columns-repeated"), 1)
			case "p", "h", "span":
				if inCell {
					inText = true
					if cell.Len() > 0 {
						cell.WriteByte(' ')
					}
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "table-cell", "covered-table-cell":
				if inCell {
					flushCell()
				}
			case "table-row":
				if cur >= 0 {
					empty := true
					for _, c := range row {
						if strings.TrimSpace(c) != "" {
							empty = false
							break
						}
					}
					if !empty {
						sheets[cur].rows = append(sheets[cur].rows, append([]string(nil), row...))
					}
				}
				row = nil
			case "p", "h", "span":
				inText = false
			}
		case xml.CharData:
			if inText {
				cell.WriteString(string(t))
			}
		}
	}
	return sheets
}

func atoiDefault(s string, def int) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return def
		}
		n = n*10 + int(r-'0')
	}
	if n == 0 {
		return def
	}
	return n
}

func collapseWS(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	nl := false
	space := false
	for _, r := range s {
		switch {
		case r == '\n':
			if !nl {
				b.WriteByte('\n')
				nl = true
			}
			space = false
		case unicode.IsSpace(r):
			if !nl && !space && b.Len() > 0 {
				b.WriteByte(' ')
				space = true
			}
		default:
			b.WriteRune(r)
			nl = false
			space = false
		}
	}
	return strings.TrimSpace(b.String())
}

func rtfText(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{\\rtf") {
		return strings.TrimSpace(s)
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			if i+1 >= len(s) {
				break
			}
			n := s[i+1]
			if n == '\\' || n == '{' || n == '}' {
				b.WriteByte(n)
				i++
				continue
			}
			if n == '\'' && i+3 < len(s) {
				i += 3
				continue
			}
			if n == 'u' {
				j := i + 2
				sign := 1
				if j < len(s) && s[j] == '-' {
					sign = -1
					j++
				}
				num := 0
				for j < len(s) && s[j] >= '0' && s[j] <= '9' {
					num = num*10 + int(s[j]-'0')
					j++
				}
				if num > 0 {
					b.WriteRune(rune(sign * num))
				}
				i = j - 1
				continue
			}
			j := i + 1
			for j < len(s) && ((s[j] >= 'a' && s[j] <= 'z') || (s[j] >= 'A' && s[j] <= 'Z')) {
				j++
			}
			word := s[i+1 : j]
			for j < len(s) && (s[j] == '-' || (s[j] >= '0' && s[j] <= '9')) {
				j++
			}
			if j < len(s) && s[j] == ' ' {
				j++
			}
			if word == "par" || word == "line" || word == "tab" {
				b.WriteByte('\n')
			}
			i = j - 1
		case '{', '}':
			continue
		case '\r':
			continue
		default:
			b.WriteByte(c)
		}
	}
	return strings.TrimSpace(collapseWS(b.String()))
}
