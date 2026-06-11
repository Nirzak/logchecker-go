// Package util provides encoding detection and conversion utilities.
package util

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// DecodeEncoding converts the given raw log bytes to a UTF-8 string.
// It handles:
//   - UTF-16 LE/BE BOM
//   - UTF-8 BOM (stripped)
//   - Whipper logs (already UTF-8, returned as-is)
//   - Heuristic fallback for common Windows/Mac codepages via golang.org/x/text
//
// On error (encoding conversion failure), the original content is returned
// along with the error.
// func DecodeEncoding takes a score func instead of isValid boolean
func DecodeEncoding(data []byte, isWhipper bool, scoreFunc func(string) int) (string, error) {
	if isWhipper {
		// Whipper always writes UTF-8; bypass all detection.
		return string(data), nil
	}

	// UTF-16 LE BOM: FF FE
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE {
		dec := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM)
		s, err := decodeWith(dec.NewDecoder(), data[2:])
		if err != nil {
			return string(data), err
		}
		return s, nil
	}
	// UTF-16 BE BOM: FE FF
	if len(data) >= 2 && data[0] == 0xFE && data[1] == 0xFF {
		dec := unicode.UTF16(unicode.BigEndian, unicode.UseBOM)
		s, err := decodeWith(dec.NewDecoder(), data[2:])
		if err != nil {
			return string(data), err
		}
		return s, nil
	}
	// UTF-8 BOM: EF BB BF
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		return string(data[3:]), nil
	}

	// Try to detect common non-UTF-8 encodings heuristically.
	if !isValidUTF8(data) {
		if s, ok := tryDecode(data, scoreFunc); ok {
			return s, nil
		}
	}

	return string(data), nil
}

// isValidUTF8 checks whether data is valid UTF-8.
func isValidUTF8(data []byte) bool {
	return utf8.Valid(data)
}

// tryDecode attempts to decode data using common codepages in order of
// likelihood for EAC logs encountered in the wild.
func tryDecode(data []byte, scoreFunc func(string) int) (string, bool) {
	candidates := []struct {
		enc encoding.Encoding
	}{
		{charmap.Windows1252}, // Western European (most common EAC)
		{charmap.Windows1250}, // Central European
		{charmap.Windows1251}, // Cyrillic (rutracker logs)
		{charmap.Windows1253}, // Greek
		{charmap.Windows1254}, // Turkish
		{charmap.Windows1255}, // Hebrew
		{charmap.Windows1256}, // Arabic
		{charmap.Windows1257}, // Baltic
		{charmap.Windows1258}, // Vietnamese
		{charmap.ISO8859_1},   // Latin-1
		{charmap.ISO8859_2},   // Latin-2
		{charmap.MacintoshCyrillic},
	}
	var bestString string
	var bestScore int = -1

	evaluate := func(s string) {
		if !isCleanText(s) {
			return
		}
		score := 0
		if scoreFunc != nil {
			score = scoreFunc(s)
		}
		if score > bestScore {
			bestScore = score
			bestString = s
		}
	}

	for _, c := range candidates {
		s, err := decodeWith(c.enc.NewDecoder(), data)
		if err == nil {
			evaluate(s)
		}
	}

	// Japanese / Korean / Chinese are less common but possible.
	if s, err := decodeWith(japanese.ShiftJIS.NewDecoder(), data); err == nil {
		evaluate(s)
	}
	if s, err := decodeWith(korean.EUCKR.NewDecoder(), data); err == nil {
		evaluate(s)
	}
	if s, err := decodeWith(simplifiedchinese.GBK.NewDecoder(), data); err == nil {
		evaluate(s)
	}

	// Fallback: treat as ISO-8859-1 (lossless for 0x00-0xFF).
	if s, err := decodeWith(charmap.ISO8859_1.NewDecoder(), data); err == nil {
		evaluate(s)
	}

	if bestScore >= 0 {
		return bestString, true
	}
	return "", false
}

// isCleanText returns true if s looks like human-readable text.
// We use a simple heuristic: no control characters except CR/LF/Tab.
func isCleanText(s string) bool {
	for _, r := range s {
		if r < 0x20 && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}

func decodeWith(d interface {
	Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error)
	Reset()
}, data []byte) (string, error) {
	t := transform.NewReader(bytes.NewReader(data), d)
	var buf bytes.Buffer
	_, err := buf.ReadFrom(t)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// NormalizeLineEndings replaces \r\n and lone \r with \n.
func NormalizeLineEndings(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}
