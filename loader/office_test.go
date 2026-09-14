package loader

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestFromBytesXLSX(t *testing.T) {
	f := excelize.NewFile()
	_ = f.SetSheetName("Sheet1", "Ledger")
	_ = f.SetCellValue("Ledger", "A1", "Account")
	_ = f.SetCellValue("Ledger", "B1", "Amount")
	_ = f.SetCellValue("Ledger", "A2", "ABC Ltd")
	_ = f.SetCellValue("Ledger", "B2", "15000")
	_ = f.SetCellValue("Ledger", "A3", "Office rent")
	_ = f.SetCellValue("Ledger", "B3", "4200")
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	docs, err := FromBytes("ledger.xlsx", buf.Bytes(), Options{DefaultNS: "acc"})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) == 0 {
		t.Fatal("expected rows")
	}
	joined := ""
	for _, d := range docs {
		joined += d.Content
		if d.Namespace != "acc" {
			t.Fatalf("namespace %q", d.Namespace)
		}
		if d.Metadata["kind"] != "xlsx" {
			t.Fatalf("kind %v", d.Metadata["kind"])
		}
	}
	if !strings.Contains(joined, "ABC Ltd") || !strings.Contains(joined, "15000") {
		t.Fatalf("missing cells: %s", joined)
	}
}

func TestXLSXTitleThenHeaderRows(t *testing.T) {
	f := excelize.NewFile()
	_ = f.SetSheetName("Sheet1", "Personel")
	_ = f.SetCellValue("Personel", "A1", "2026 Personel Ana Listesi")
	_ = f.SetCellValue("Personel", "A3", "Ad Soyad")
	_ = f.SetCellValue("Personel", "B3", "Grup Sirketi")
	_ = f.SetCellValue("Personel", "C3", "Departman")
	_ = f.SetCellValue("Personel", "D3", "Pozisyon")
	_ = f.SetCellValue("Personel", "A4", "Elif Koc")
	_ = f.SetCellValue("Personel", "B4", "MB Lojistik")
	_ = f.SetCellValue("Personel", "C4", "Operasyon")
	_ = f.SetCellValue("Personel", "D4", "Operasyon Muduru")
	_ = f.SetCellValue("Personel", "A5", "Ahmet Yilmaz")
	_ = f.SetCellValue("Personel", "B5", "MB Enerji")
	_ = f.SetCellValue("Personel", "C5", "Proje")
	_ = f.SetCellValue("Personel", "D5", "Proje Uzmani")
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	docs, err := FromBytes("holding.xlsx", buf.Bytes(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	var elif string
	rowDocs := 0
	for _, d := range docs {
		if d.Metadata["unit"] != "row" {
			t.Fatalf("expected unit=row, got %v in %q", d.Metadata["unit"], d.Content)
		}
		if strings.Contains(d.Content, "Elif Koc") {
			elif = d.Content
		}
		if strings.Contains(d.Content, "Ad Soyad:") {
			rowDocs++
		}
	}
	if rowDocs < 2 {
		t.Fatalf("expected one document per data row, got %d docs: %#v", len(docs), docs)
	}
	for _, need := range []string{
		"Sheet: Personel",
		"Ad Soyad: Elif Koc",
		"Grup Sirketi: MB Lojistik",
		"Pozisyon: Operasyon Muduru",
	} {
		if !strings.Contains(elif, need) {
			t.Fatalf("missing %q in:\n%s", need, elif)
		}
	}
}

func TestFromBytesDOCX(t *testing.T) {
	data := mustZip(t, map[string]string{
		"word/document.xml": `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body><w:p><w:r><w:t>Invoice ABC Ltd paid 15000 TL in April.</w:t></w:r></w:p></w:body>
</w:document>`,
	})
	docs, err := FromBytes("invoice.docx", data, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || !strings.Contains(docs[0].Content, "ABC Ltd") {
		t.Fatalf("%#v", docs)
	}
}

func TestFromBytesRTF(t *testing.T) {
	docs, err := FromBytes("note.rtf", []byte(`{\rtf1\ansi Hello accounting note\par}`), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(docs[0].Content, "Hello accounting note") {
		t.Fatalf("%q", docs[0].Content)
	}
}

func TestFromBytesCSVStillRows(t *testing.T) {
	docs, err := FromBytes("a.csv", []byte("name,amount\nABC,10\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || !strings.Contains(docs[0].Content, "ABC") {
		t.Fatalf("%#v", docs)
	}
}

func TestFromBytesRejectsBinary(t *testing.T) {
	_, err := FromBytes("x.bin", []byte{0x00, 0x01, 0x02, 0x03, 0xff, 0xfe}, Options{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDirectoryLoadsOffice(t *testing.T) {
	dir := t.TempDir()
	xlsx, err := FromBytes("t.xlsx", mustXLSX(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	_ = xlsx
	if err := os.WriteFile(filepath.Join(dir, "t.xlsx"), mustXLSX(t), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skip.png"), []byte{137, 80, 78, 71}, 0o644); err != nil {
		t.Fatal(err)
	}
	docs, err := Directory(Options{}).Load(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) == 0 {
		t.Fatal("expected xlsx docs")
	}
}

func mustXLSX(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	_ = f.SetCellValue("Sheet1", "A1", "Hello")
	_ = f.SetCellValue("Sheet1", "A2", "World")
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func mustZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
