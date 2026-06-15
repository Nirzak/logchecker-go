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

// levenshtein computes the edit distance between s and t using O(n) space.
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
	prev := make([]int, lt+1)
	curr := make([]int, lt+1)
	for j := 0; j <= lt; j++ {
		prev[j] = j
	}
	for i := 1; i <= ls; i++ {
		curr[0] = i
		for j := 1; j <= lt; j++ {
			cost := 1
			if sr[i-1] == tr[j-1] {
				cost = 0
			}
			curr[j] = min(prev[j]+1, min(curr[j-1]+1, prev[j-1]+cost))
		}
		prev, curr = curr, prev
	}
	return prev[lt]
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

// driveResult carries the class/state outcomes of validateDrive so each
// parser can build its own HTML annotation format.
type driveResult struct {
	DriveClass  string
	OffsetClass string
	IsFake      bool
	InDB        bool
}

// validateDrive performs the canonical drive validation flow shared by all
// three parsers (EAC/XLD callbacks, whipper, dBpoweramp):
//
//  1. Fake-drive guard — sets DriveClass/OffsetClass to "bad", fires accountVirtualDrive.
//  2. DB lookup via getDrives.
//  3. Offset match when drive is found (skipped when offsetStr == "").
//  4. Zero-offset guard when drive is not found (skipped when offsetStr == "").
//
// driveName is the display name used in accountVirtualDrive messages.
// lookupName is the normalised model string passed to getDrives (may equal driveName).
// offsetStr is the rip-offset value to compare against lc.offsets.
// Pass offsetStr == "" to skip offset validation (EAC/XLD, where readOffsetCallback
// handles the offset check separately after getDrives has populated lc.drives/offsets).
func (lc *Logchecker) validateDrive(driveName, lookupName, offsetStr string) driveResult {
	for _, f := range lc.fakeDrives {
		if strings.TrimSpace(driveName) == f {
			lc.accountVirtualDrive(driveName)
			return driveResult{DriveClass: cssBad, OffsetClass: cssBad, IsFake: true}
		}
	}

	lc.getDrives(lookupName)

	if len(lc.drives) > 0 {
		lc.driveFound = true
		if offsetStr == "" {
			return driveResult{DriveClass: cssGood, InDB: true}
		}
		found := false
		for _, o := range lc.offsets {
			if o == offsetStr {
				found = true
				break
			}
		}
		if found {
			return driveResult{DriveClass: cssGood, OffsetClass: cssGood, InDB: true}
		}
		lc.accountIncorrectOffset()
		return driveResult{DriveClass: cssGood, OffsetClass: cssBad, InDB: true}
	}

	// Drive not in DB.
	lc.driveFound = false
	if offsetStr == "" {
		return driveResult{DriveClass: cssBadish}
	}
	if offsetStr == "0" {
		lc.accountZeroOffsetUnknownDrive()
		return driveResult{DriveClass: cssBadish, OffsetClass: cssBad}
	}
	return driveResult{DriveClass: cssBadish, OffsetClass: cssBadish}
}
