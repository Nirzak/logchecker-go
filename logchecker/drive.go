package logchecker

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	reTSSTcorp    = regexp.MustCompile(`TSSTcorp(BD|CD|DVD)`)
	reHLDTST      = regexp.MustCompile(`HL-DT-ST(BD|CD|DVD)`)
	rePanasonic   = regexp.MustCompile(`Panasonic(BD|CD|DVD)`)
	reLeadingDash = regexp.MustCompile(`^[ _-]+`)
	reDashSpace   = regexp.MustCompile(`\s+-\s`)
	reSpaces      = regexp.MustCompile(`\s+`)
	reRevision    = regexp.MustCompile(`\(revision [a-zA-Z0-9.,\-]*\)`)
	reAdapter     = regexp.MustCompile(` Adapter.*$`)
)

func normalizeDriveName(name string) string {
	name = strings.ReplaceAll(name, "JLMS", "Lite-ON")
	name = reTSSTcorp.ReplaceAllString(name, "TSSTcorp $1")
	name = reHLDTST.ReplaceAllString(name, "HL-DT-ST $1")
	name = strings.ReplaceAll(name, "HL-DT-ST", "LG Electronics")
	name = strings.ReplaceAll(name, "Matshita", "Panasonic")
	name = strings.ReplaceAll(name, "MATSHITA", "Panasonic")
	name = rePanasonic.ReplaceAllString(name, "Panasonic $1")
	name = reLeadingDash.ReplaceAllString(name, "")
	name = reDashSpace.ReplaceAllString(name, " ")
	name = reSpaces.ReplaceAllString(name, " ")
	name = reRevision.ReplaceAllString(name, "")
	name = reAdapter.ReplaceAllString(name, "")
	return strings.ToLower(strings.TrimSpace(name))
}

func levenshtein(s, t string) int {
	sr := []rune(s)
	tr := []rune(t)
	ls, lt := len(sr), len(tr)
	if ls == 0 {
		return lt
	}
	if lt == 0 {
		return ls
	}
	d := make([][]int, ls+1)
	for i := range d {
		d[i] = make([]int, lt+1)
	}
	for i := 0; i <= ls; i++ {
		d[i][0] = i
	}
	for j := 0; j <= lt; j++ {
		d[0][j] = j
	}
	for i := 1; i <= ls; i++ {
		for j := 1; j <= lt; j++ {
			cost := 1
			if sr[i-1] == tr[j-1] {
				cost = 0
			}
			d[i][j] = min3(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
		}
	}
	return d[ls][lt]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

func (lc *Logchecker) getDrives(driveName string) {
	normalized := normalizeDriveName(driveName)
	maxDist := LevenshteinDistance + 1

	type bucket struct {
		drives  []string
		offsets []string
	}
	buckets := make([]bucket, maxDist)

	for _, entry := range lc.allDrives {
		name, _ := entry[0].(string)
		offset := entry[1]
		dist := levenshtein(name, normalized)
		if dist < maxDist {
			buckets[dist].drives = append(buckets[dist].drives, name)
			var offStr string
			switch v := offset.(type) {
			case float64:
				offStr = strconv.Itoa(int(v))
			case int:
				offStr = strconv.Itoa(v)
			case string:
				offStr = v
			default:
				offStr = fmt.Sprintf("%v", v)
			}
			buckets[dist].offsets = append(buckets[dist].offsets, offStr)
		}
	}

	lc.drives = nil
	lc.offsets = nil
	for _, b := range buckets {
		if len(b.drives) > 0 {
			lc.drives = b.drives
			lc.offsets = b.offsets
			break
		}
	}
}
