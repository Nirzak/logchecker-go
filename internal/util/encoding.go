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
		{MacCentralEurope},    // Mac Central European
		{charmap.Macintosh},   // MacRoman
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

// MacCentralEurope is the Macintosh Central European (CP10029) encoding.
var MacCentralEurope encoding.Encoding = macCentralEurope{}

type macCentralEurope struct{}

func (macCentralEurope) NewDecoder() *encoding.Decoder {
	return &encoding.Decoder{
		Transformer: &macCentralEuropeDecoder{},
	}
}

func (macCentralEurope) NewEncoder() *encoding.Encoder {
	return nil
}

func (macCentralEurope) String() string {
	return "MacCentralEurope"
}

type macCentralEuropeDecoder struct{}

// this is required by transform.Transformer
func (macCentralEuropeDecoder) Reset() {}

var cp10029Table = [128]rune{
	0x00C4, 0x0100, 0x0101, 0x00C9, 0x0104, 0x00D6, 0x00DC, 0x00E1,
	0x0105, 0x010C, 0x00E4, 0x010D, 0x0106, 0x0107, 0x00E9, 0x0179,
	0x017A, 0x010E, 0x00ED, 0x010F, 0x0112, 0x0113, 0x0116, 0x00F3,
	0x0117, 0x00F4, 0x00F6, 0x00F5, 0x00FA, 0x011A, 0x011B, 0x00FC,
	0x2020, 0x00B0, 0x0118, 0x00A3, 0x00A7, 0x2022, 0x00B6, 0x00DF,
	0x00AE, 0x00A9, 0x2122, 0x0119, 0x00A8, 0x2260, 0x0123, 0x012E,
	0x012F, 0x012A, 0x2264, 0x2265, 0x012B, 0x0136, 0x2202, 0x2211,
	0x0142, 0x013B, 0x013C, 0x013D, 0x013E, 0x0139, 0x013A, 0x0145,
	0x0146, 0x0143, 0x00AC, 0x221A, 0x0144, 0x0147, 0x2206, 0x00AB,
	0x00BB, 0x2026, 0x00A0, 0x0148, 0x0150, 0x00D5, 0x0151, 0x014C,
	0x2013, 0x2014, 0x201C, 0x201D, 0x2018, 0x2019, 0x00F7, 0x25CA,
	0x014D, 0x0154, 0x0155, 0x0158, 0x2039, 0x203A, 0x0159, 0x0156,
	0x0157, 0x0160, 0x201A, 0x201E, 0x0161, 0x015A, 0x015B, 0x00C1,
	0x0164, 0x0165, 0x00CD, 0x017D, 0x017E, 0x016A, 0x00D3, 0x00D4,
	0x016B, 0x016E, 0x00DA, 0x016F, 0x0170, 0x0171, 0x0172, 0x0173,
	0x00DD, 0x00FD, 0x0137, 0x017B, 0x0141, 0x017C, 0x0122, 0x02C7,
}

func (macCentralEuropeDecoder) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for nSrc < len(src) {
		b := src[nSrc]
		var r rune
		if b < 128 {
			r = rune(b)
		} else {
			r = cp10029Table[b-128]
		}

		size := utf8.RuneLen(r)
		if nDst+size > len(dst) {
			err = transform.ErrShortDst
			break
		}

		utf8.EncodeRune(dst[nDst:], r)
		nDst += size
		nSrc++
	}
	return nDst, nSrc, err
}
