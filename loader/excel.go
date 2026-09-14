package loader

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/selimyalcin/ragentgo/document"
	"github.com/xuri/excelize/v2"
)

func parseXLSX(name string, data []byte, opts Options) ([]document.Document, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("xlsx loader: %w", err)
	}
	defer func() { _ = f.Close() }()
	var out []document.Document
	for _, sheet := range f.GetSheetList() {
		rows, err := f.GetRows(sheet)
		if err != nil {
			continue
		}
		out = append(out, tableDocuments(name, sheet, "xlsx", rows, opts)...)
	}
	return out, nil
}

func parseXLS(name string, data []byte, opts Options) ([]document.Document, error) {
	// Structured BIFF parsing is unreliable across encodings; harvest the OLE
	// streams and keep sheet-like line breaks so the chunker can split later.
	text, err := extractOLEText(data)
	if err != nil {
		return nil, fmt.Errorf("xls loader: %w", err)
	}
	meta := fileMeta(name)
	meta["kind"] = "xls"
	return []document.Document{newDoc("", text, name, opts.DefaultNS, meta)}, nil
}

func parseODS(name string, data []byte, opts Options) ([]document.Document, error) {
	body, err := zipNamedFile(data, "content.xml")
	if err != nil {
		return nil, fmt.Errorf("ods loader: %w", err)
	}
	sheets := odsTables(body)
	if len(sheets) == 0 {
		text := xmlRuns(body, map[string]bool{"p": true, "h": true}, map[string]bool{"p": true, "h": true})
		meta := fileMeta(name)
		meta["kind"] = "ods"
		return []document.Document{newDoc("", text, name, opts.DefaultNS, meta)}, nil
	}
	var out []document.Document
	for _, sh := range sheets {
		out = append(out, tableDocuments(name, sh.name, "ods", sh.rows, opts)...)
	}
	return out, nil
}

type odsSheet struct {
	name string
	rows [][]string
}

func tableDocuments(name, sheet, kind string, rows [][]string, opts Options) []document.Document {
	header := []string{}
	prevEmpty := true
	var out []document.Document
	for i, row := range rows {
		if isEmptyRow(row) {
			prevEmpty = true
			continue
		}
		if shouldUseAsHeader(row, header, prevEmpty) {
			header = normalizeHeader(row)
			prevEmpty = false
			continue
		}
		prevEmpty = false
		line := formatTableRow(header, row)
		if line == "" {
			continue
		}
		var b strings.Builder
		if sheet != "" {
			fmt.Fprintf(&b, "Sheet: %s\n", sheet)
		}
		fmt.Fprintf(&b, "Row: %d\n", i+1)
		if rec := compactRowValues(row); rec != "" {
			fmt.Fprintf(&b, "Record: %s\n", rec)
		}
		b.WriteString(line)
		meta := fileMeta(name)
		meta["kind"] = kind
		meta["sheet"] = sheet
		meta["row"] = i + 1
		meta["unit"] = "row"
		out = append(out, newDoc("", strings.TrimSpace(b.String()), name, opts.DefaultNS, meta))
	}
	return out
}

func shouldUseAsHeader(row, current []string, prevEmpty bool) bool {
	if !looksLikeLabelRow(row) {
		return false
	}
	if len(current) == 0 {
		return true
	}
	return prevEmpty
}

func looksLikeLabelRow(row []string) bool {
	if headerScore(row) < 4 || isNumericHeavy(row) {
		return false
	}
	var nonempty, withDigit int
	for _, c := range row {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		nonempty++
		if strings.IndexFunc(c, unicode.IsDigit) >= 0 {
			withDigit++
		}
	}
	return nonempty >= 2 && withDigit*3 < nonempty
}

func normalizeHeader(row []string) []string {
	out := make([]string, len(row))
	seen := map[string]int{}
	for i, c := range row {
		c = strings.TrimSpace(c)
		if c == "" {
			c = fmt.Sprintf("Column %d", i+1)
		}
		n := seen[strings.ToLower(c)]
		seen[strings.ToLower(c)] = n + 1
		if n > 0 {
			c = fmt.Sprintf("%s (%d)", c, n+1)
		}
		out[i] = c
	}
	return out
}

func headerScore(row []string) int {
	var nonempty, numeric int
	for _, c := range row {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		nonempty++
		if isNumericCell(c) {
			numeric++
		}
	}
	if nonempty < 2 {
		return 0
	}
	return nonempty*2 - numeric*3
}

func isNumericHeavy(row []string) bool {
	var nonempty, numeric int
	for _, c := range row {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		nonempty++
		if isNumericCell(c) {
			numeric++
		}
	}
	return nonempty > 0 && numeric*2 >= nonempty
}

func isEmptyRow(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func compactRowValues(row []string) string {
	parts := make([]string, 0, len(row))
	for _, c := range row {
		c = strings.TrimSpace(c)
		if c != "" {
			parts = append(parts, c)
		}
	}
	if len(parts) > 5 {
		parts = parts[:5]
	}
	return strings.Join(parts, " | ")
}

func looksLikeHeader(rows [][]string) bool {
	if len(rows) == 0 {
		return false
	}
	return looksLikeLabelRow(rows[0])
}

func isNumericCell(s string) bool {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "%₺$€ \t")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.Trim(s, "()")
	if s == "" || s == "-" {
		return false
	}
	switch {
	case strings.Contains(s, ",") && strings.Contains(s, "."):
		s = strings.ReplaceAll(s, ",", "")
	case strings.Count(s, ",") == 1 && !strings.Contains(s, "."):
		s = strings.ReplaceAll(s, ",", ".")
	default:
		s = strings.ReplaceAll(s, ",", "")
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}
