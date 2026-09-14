package loader

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf16"

	"github.com/richardlehane/mscfb"
)

func extractOLEText(data []byte) (string, error) {
	var b strings.Builder
	rs := bytes.NewReader(data)
	r, err := mscfb.New(rs)
	if err != nil {
		text := pickHarvest(data)
		if text == "" {
			return "", fmt.Errorf("ole: %w", err)
		}
		return text, nil
	}
	for {
		entry, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		chunk, err := io.ReadAll(io.LimitReader(entry, 8<<20))
		if err != nil && len(chunk) == 0 {
			continue
		}
		if t := pickHarvest(chunk); t != "" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(t)
		}
	}
	text := strings.TrimSpace(collapseWS(b.String()))
	if text == "" {
		text = pickHarvest(data)
	}
	if text == "" {
		return "", fmt.Errorf("ole: no extractable text")
	}
	return text, nil
}

func pickHarvest(b []byte) string {
	u := harvestUTF16LE(b)
	a := harvestASCII(b)
	if len([]rune(u)) >= len([]rune(a)) {
		return u
	}
	return a
}

func harvestUTF16LE(b []byte) string {
	var (
		parts []string
		run   []rune
	)
	flush := func() {
		if letterCount(run) >= 4 && len(run) >= 6 {
			parts = append(parts, strings.TrimSpace(string(run)))
		}
		run = run[:0]
	}
	for i := 0; i+1 < len(b); i += 2 {
		u := binary.LittleEndian.Uint16(b[i:])
		var r rune
		switch {
		case utf16.IsSurrogate(rune(u)):
			if i+3 >= len(b) {
				flush()
				continue
			}
			u2 := binary.LittleEndian.Uint16(b[i+2:])
			r = utf16.DecodeRune(rune(u), rune(u2))
			i += 2
		default:
			r = rune(u)
		}
		if isHarvestRune(r) || (r == ' ' && len(run) > 0) {
			run = append(run, r)
			continue
		}
		flush()
	}
	flush()
	return strings.Join(parts, "\n")
}

func harvestASCII(b []byte) string {
	var (
		parts []string
		run   []byte
	)
	flush := func() {
		if letterCount([]rune(string(run))) >= 4 && len(run) >= 6 {
			parts = append(parts, strings.TrimSpace(string(run)))
		}
		run = run[:0]
	}
	for _, c := range b {
		if c >= 0x20 && c < 0x7f {
			run = append(run, c)
			continue
		}
		if c == '\n' || c == '\r' || c == '\t' {
			flush()
			continue
		}
		flush()
	}
	flush()
	return strings.Join(parts, "\n")
}

func isHarvestRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsPunct(r)
}

func letterCount(run []rune) int {
	n := 0
	for _, r := range run {
		if unicode.IsLetter(r) {
			n++
		}
	}
	return n
}
