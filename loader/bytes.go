package loader

import (
	"archive/zip"
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/selimyalcin/ragentgo/document"
)

const (
	zipMagic = "PK\x03\x04"
	oleMagic = "\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1"
	pdfMagic = "%PDF"
)

// FromBytes inspects filename, MIME, and magic bytes, then extracts text.
func FromBytes(name string, data []byte, opts Options) ([]document.Document, error) {
	if int64(len(data)) > opts.maxBytes() {
		return nil, fmt.Errorf("loader: source exceeds %d bytes", opts.maxBytes())
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("loader: empty file")
	}
	kind := detectKind(name, opts.MIME, data)
	var (
		docs []document.Document
		err  error
	)
	switch kind {
	case "json":
		docs, err = parseJSON(name, data, opts)
	case "csv":
		docs, err = parseCSV(name, data, opts, ',')
	case "tsv":
		docs, err = parseCSV(name, data, opts, '\t')
	case "html":
		text, herr := extractHTML(data)
		if herr != nil {
			return nil, herr
		}
		meta := fileMeta(name)
		meta["kind"] = "html"
		docs = []document.Document{newDoc("", text, name, opts.DefaultNS, meta)}
	case "xlsx":
		docs, err = parseXLSX(name, data, opts)
	case "xls":
		docs, err = parseXLS(name, data, opts)
	case "ole":
		docs, err = parseDOC(name, data, opts)
	case "ods":
		docs, err = parseODS(name, data, opts)
	case "docx":
		docs, err = parseDOCX(name, data, opts)
	case "doc":
		docs, err = parseDOC(name, data, opts)
	case "pptx":
		docs, err = parsePPTX(name, data, opts)
	case "ppt":
		docs, err = parsePPT(name, data, opts)
	case "odt":
		docs, err = parseODT(name, data, opts)
	case "pdf":
		docs, err = parsePDF(name, data, opts)
	case "rtf":
		docs, err = parseRTF(name, data, opts)
	case "text":
		docs, err = parsePlain(name, data, opts)
	default:
		return nil, fmt.Errorf("loader: unsupported file type %q", displayType(name, kind))
	}
	if err != nil {
		return nil, err
	}
	docs = dropEmptyDocs(docs)
	if len(docs) == 0 {
		return nil, fmt.Errorf("loader: no extractable text in %s", filepath.Base(name))
	}
	return docs, nil
}

func parsePlain(name string, data []byte, opts Options) ([]document.Document, error) {
	if looksBinary(data) {
		return nil, fmt.Errorf("loader: unsupported binary file %q", filepath.Base(name))
	}
	text := string(data)
	if !utf8.ValidString(text) {
		text = strings.ToValidUTF8(text, "")
	}
	meta := fileMeta(name)
	meta["kind"] = "text"
	return []document.Document{newDoc("", text, name, opts.DefaultNS, meta)}, nil
}

func dropEmptyDocs(in []document.Document) []document.Document {
	out := in[:0]
	for _, d := range in {
		if strings.TrimSpace(d.Content) == "" {
			continue
		}
		out = append(out, d)
	}
	return out
}

func detectKind(name, mime string, data []byte) string {
	if k := kindFromExt(filepath.Ext(name)); k != "" {
		return k
	}
	if k := kindFromMIME(mime); k != "" {
		return k
	}
	return sniffKind(data)
}

func kindFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".md", ".markdown", ".txt", ".text", ".go", ".yml", ".yaml", ".log", ".rst", ".xml", ".toml":
		return "text"
	case ".html", ".htm":
		return "html"
	case ".json":
		return "json"
	case ".csv":
		return "csv"
	case ".tsv":
		return "tsv"
	case ".xlsx", ".xlsm", ".xltx", ".xltm":
		return "xlsx"
	case ".xls":
		return "xls"
	case ".ods":
		return "ods"
	case ".docx", ".dotx":
		return "docx"
	case ".doc":
		return "doc"
	case ".pptx", ".potx", ".ppsx":
		return "pptx"
	case ".ppt":
		return "ppt"
	case ".odt":
		return "odt"
	case ".pdf":
		return "pdf"
	case ".rtf":
		return "rtf"
	default:
		return ""
	}
}

func kindFromMIME(mime string) string {
	mime = strings.ToLower(strings.TrimSpace(strings.Split(mime, ";")[0]))
	switch mime {
	case "application/pdf":
		return "pdf"
	case "text/csv":
		return "csv"
	case "text/tab-separated-values":
		return "tsv"
	case "application/json", "text/json":
		return "json"
	case "text/html", "application/xhtml+xml":
		return "html"
	case "application/rtf", "text/rtf":
		return "rtf"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/vnd.ms-excel.sheet.macroenabled.12":
		return "xlsx"
	case "application/vnd.ms-excel", "application/msexcel":
		return "xls"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return "docx"
	case "application/msword":
		return "doc"
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation":
		return "pptx"
	case "application/vnd.ms-powerpoint":
		return "ppt"
	case "application/vnd.oasis.opendocument.text":
		return "odt"
	case "application/vnd.oasis.opendocument.spreadsheet":
		return "ods"
	case "text/plain":
		return "text"
	default:
		return ""
	}
}

func sniffKind(data []byte) string {
	if len(data) >= 4 && string(data[:4]) == pdfMagic {
		return "pdf"
	}
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("{\\rtf")) {
		return "rtf"
	}
	if len(data) >= 8 && string(data[:8]) == oleMagic {
		return "ole"
	}
	if len(data) >= 4 && string(data[:4]) == zipMagic {
		return sniffZip(data)
	}
	if looksBinary(data) {
		return ""
	}
	trim := strings.TrimSpace(string(data[:min(len(data), 256)]))
	if strings.HasPrefix(trim, "{") || strings.HasPrefix(trim, "[") {
		return "json"
	}
	if strings.HasPrefix(strings.ToLower(trim), "<!doctype html") || strings.HasPrefix(strings.ToLower(trim), "<html") {
		return "html"
	}
	return "text"
}

func sniffZip(data []byte) string {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return ""
	}
	var mime string
	for _, f := range zr.File {
		switch {
		case f.Name == "mimetype":
			body, err := readZipFile(f)
			if err == nil {
				mime = strings.TrimSpace(string(body))
			}
		case strings.HasPrefix(f.Name, "word/"):
			return "docx"
		case strings.HasPrefix(f.Name, "xl/"):
			return "xlsx"
		case strings.HasPrefix(f.Name, "ppt/"):
			return "pptx"
		}
	}
	switch mime {
	case "application/vnd.oasis.opendocument.text":
		return "odt"
	case "application/vnd.oasis.opendocument.spreadsheet":
		return "ods"
	}
	return ""
}

func looksBinary(data []byte) bool {
	n := len(data)
	if n > 800 {
		n = 800
	}
	if n == 0 {
		return false
	}
	var nul, ctrl int
	for _, b := range data[:n] {
		if b == 0 {
			nul++
		}
		if b < 0x09 || (b > 0x0d && b < 0x20) {
			ctrl++
		}
	}
	return nul > 0 || ctrl*20 > n
}

func displayType(name, kind string) string {
	if kind != "" {
		return kind
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext != "" {
		return ext
	}
	return "unknown"
}
