package logchecker

import (
	"fmt"
	"sort"
	"strconv"
)

// pointsStr returns " (-N point(s))" or "" when n == 0.
func pointsStr(n int) string {
	if n == 0 {
		return ""
	}
	if n == 1 {
		return fmt.Sprintf(" (-%d point)", n)
	}
	return fmt.Sprintf(" (-%d points)", n)
}

// account adds a message and applies score changes.
// decrease > 0 subtracts from score. setScore >= 0 sets score to that value.
// Pass decrease=0, setScore=-1 for notice-only.
func (lc *Logchecker) account(msg string, decrease int, setScore int, inclCombined bool, notice bool) {
	append1 := ""
	if inclCombined && lc.combined > 0 {
		append1 = fmt.Sprintf(" (%d)", lc.currLog)
	}
	prepend := ""
	if notice {
		prepend = "[Notice] "
	}
	append2 := ""
	if decrease > 0 {
		append2 = pointsStr(decrease)
	} else if setScore >= 0 {
		d := 100 - setScore
		if d > 0 {
			append2 = pointsStr(d)
		}
	}

	full := prepend + msg + append1 + append2
	for _, existing := range lc.details {
		if existing == full {
			return
		}
	}
	lc.details = append(lc.details, full)
	if decrease > 0 {
		lc.score -= decrease
	} else if setScore >= 0 {
		lc.score = setScore
	}
}

func (lc *Logchecker) accountTrack(msg string, decrease int) {
	tn := lc.trackNumber
	if n, err := strconv.Atoi(tn); err == nil && n < 10 {
		tn = fmt.Sprintf("0%d", n)
	}
	append2 := ""
	if decrease > 0 {
		lc.decreaseTrack += decrease
		append2 = pointsStr(decrease)
	}
	combinedPart := ""
	if lc.combined > 0 {
		combinedPart = fmt.Sprintf(" (%d)", lc.currLog)
	}
	lc.badTrack = append(lc.badTrack, "Track "+tn+combinedPart+": "+msg+append2)
}

// accountDeduction is the common case: deduct points, no combined suffix, no notice.
func (lc *Logchecker) accountDeduction(msg string, decrease int) {
	lc.account(msg, decrease, -1, false, false)
}

// accountNotice records an informational message with no score deduction.
func (lc *Logchecker) accountNotice(msg string) {
	lc.account(msg, 0, -1, false, true)
}

// accountFatal sets the score to a specific absolute value (usually 0).
func (lc *Logchecker) accountFatal(msg string, setScore int) {
	lc.account(msg, 0, setScore, false, false)
}

// sortNumericStrings sorts a slice of numeric strings in ascending numeric order.
// Non-numeric strings sort lexicographically after all numeric strings.
func sortNumericStrings(ss []string) {
	sort.Slice(ss, func(i, j int) bool {
		ni, erri := strconv.Atoi(ss[i])
		nj, errj := strconv.Atoi(ss[j])
		if erri == nil && errj == nil {
			return ni < nj
		}
		if erri == nil {
			return true
		}
		if errj == nil {
			return false
		}
		return ss[i] < ss[j]
	})
}
