package logchecker

import (
	"regexp"
	"strconv"
	"strings"
)

// splitWithDelim splits s by re but includes captured delimiters inline.
// Equivalent to PHP preg_split with PREG_SPLIT_DELIM_CAPTURE.
func splitWithDelim(s string, re *regexp.Regexp) []string {
	var result []string
	last := 0
	for _, loc := range re.FindAllStringIndex(s, -1) {
		result = append(result, s[last:loc[0]])
		result = append(result, s[loc[0]:loc[1]])
		last = loc[1]
	}
	result = append(result, s[last:])
	return result
}

// splitWithCaptures splits s by re returning alternating: [cap1, cap2, body, cap1, cap2, body, ...]
func splitWithCaptures(s string, re *regexp.Regexp) []string {
	var captures []string
	matches := re.FindAllStringSubmatchIndex(s, -1)
	for _, m := range matches {
		// Each match: m[0],m[1] = full; m[2],m[3] = cap1; m[4],m[5] = cap2
		for i := 2; i < len(m)-1; i += 2 {
			if m[i] >= 0 {
				captures = append(captures, s[m[i]:m[i+1]])
			} else {
				captures = append(captures, "")
			}
		}
	}
	return captures
}

// replaceCount replaces up to n occurrences (n=-1 for all) and returns the count.
func replaceCount(s string, re *regexp.Regexp, repl string, n int) (string, int) {
	count := 0
	result := re.ReplaceAllStringFunc(s, func(match string) string {
		if n >= 0 && count >= n {
			return match
		}
		count++
		return re.ReplaceAllString(match, repl)
	})
	return result, count
}

// replaceCountCallback replaces up to n occurrences using a callback.
func replaceCountCallback(s string, re *regexp.Regexp, fn func([]string) string, n int) (string, int) {
	count := 0
	result := re.ReplaceAllStringFunc(s, func(match string) string {
		if n >= 0 && count >= n {
			return match
		}
		m := re.FindStringSubmatch(match)
		if m == nil {
			return match
		}
		count++
		return fn(m)
	})
	return result, count
}

// compareVersions compares two semver-ish version strings.
// Returns -1 if a < b, 0 if equal, 1 if a > b.
func compareVersions(a, b string) int {
	partsA := strings.Split(a, ".")
	partsB := strings.Split(b, ".")
	n := len(partsA)
	if len(partsB) > n {
		n = len(partsB)
	}
	for i := 0; i < n; i++ {
		var na, nb int
		if i < len(partsA) {
			na, _ = strconv.Atoi(partsA[i])
		}
		if i < len(partsB) {
			nb, _ = strconv.Atoi(partsB[i])
		}
		if na < nb {
			return -1
		}
		if na > nb {
			return 1
		}
	}
	return 0
}
