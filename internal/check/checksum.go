// Package check provides checksum validation for CD-rip log files.
package check

import (
	"crypto/sha256"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/Nirzak/eac-logchecker/eaclogchecker"
	"github.com/Nirzak/xld-logchecker/xldlogchecker"
)

const (
	ChecksumOK      = "checksum_ok"
	ChecksumInvalid = "checksum_invalid"
	ChecksumMissing = "checksum_missing"
)

var whipperHashRe = regexp.MustCompile(`(?i)SHA-256 hash: ([A-Z0-9]+)$`)

// Validate checks the integrity of a log file for the given ripper.
// logPath may be empty for whipper (content is used directly), but in practice
// it is always populated. Returns one of the Checksum* constants.
func Validate(logPath, ripper string) string {
	switch ripper {
	case DBpoweramp:
		return ChecksumMissing

	case Whipper:
		return validateWhipper(logPath)

	case EAC:
		results := eaclogchecker.CheckChecksum(logPath)
		if len(results) == 0 {
			return ChecksumOK // no entries — assume ok (logchecker not applicable)
		}
		// If any entry is BAD, the whole log is invalid.
		// If any entry has NO checksum, mark missing.
		// Only if all are OK do we return ok.
		hasMissing := false
		for _, r := range results {
			switch r.Status {
			case "BAD":
				return ChecksumInvalid
			case "NO":
				hasMissing = true
			}
		}
		if hasMissing {
			return ChecksumMissing
		}
		return ChecksumOK

	case XLD:
		r := xldlogchecker.ParseLog(logPath)
		switch r.Status {
		case "BAD":
			return ChecksumInvalid
		case "ERROR":
			return ChecksumMissing
		default:
			return ChecksumOK
		}

	default:
		return ChecksumOK
	}
}

// validateWhipper computes the SHA-256 of all lines except the last hash line
// and compares it to the embedded hash.
func validateWhipper(logPath string) string {
	raw, err := os.ReadFile(logPath)
	if err != nil {
		return ChecksumMissing
	}
	content := strings.TrimRight(string(raw), "\r\n ")

	m := whipperHashRe.FindStringSubmatch(content)
	if m == nil {
		return ChecksumMissing
	}
	embedded := strings.ToLower(m[1])

	lines := strings.Split(content, "\n")
	// All lines except the last one (the SHA-256 line).
	body := strings.Join(lines[:len(lines)-1], "\n")
	computed := fmt.Sprintf("%x", sha256.Sum256([]byte(body)))

	if computed == embedded {
		return ChecksumOK
	}
	return ChecksumInvalid
}
