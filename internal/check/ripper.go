// Package check provides ripper detection for CD-rip log files.
package check

import (
	"errors"
	"regexp"
	"strings"
)

const (
	Unknown    = "unknown"
	Whipper    = "whipper"
	XLD        = "XLD"
	EAC        = "EAC"
	DBpoweramp = "dBpoweramp"
)

// ErrUnknownRipper is returned when the log cannot be attributed to a known ripper.
var ErrUnknownRipper = errors.New("could not determine ripper")

var dbpowerampRe = regexp.MustCompile(`(?im)^dBpoweramp Release`)

// GetRipper inspects log text and returns one of the ripper constants.
func GetRipper(log string) (string, error) {
	if strings.Contains(log, "Log created by: whipper") {
		return Whipper, nil
	}
	if strings.Contains(log, "X Lossless Decoder version") {
		return XLD, nil
	}
	if strings.Contains(log, "Exact Audio Copy") {
		return EAC, nil
	}
	if dbpowerampRe.MatchString(log) {
		return DBpoweramp, nil
	}
	// Fallback: check first line for "EAC"
	if nl := strings.Index(log, "\n"); nl != -1 {
		if strings.Contains(log[:nl], "EAC") {
			return EAC, nil
		}
	}
	return Unknown, ErrUnknownRipper
}
