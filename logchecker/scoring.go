package logchecker

import (
	"fmt"
	"strconv"
)

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
		if decrease == 1 {
			append2 = fmt.Sprintf(" (-%d point)", decrease)
		} else {
			append2 = fmt.Sprintf(" (-%d points)", decrease)
		}
	} else if setScore >= 0 {
		d := 100 - setScore
		if d > 0 {
			if d == 1 {
				append2 = fmt.Sprintf(" (-%d point)", d)
			} else {
				append2 = fmt.Sprintf(" (-%d points)", d)
			}
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
		if decrease == 1 {
			append2 = fmt.Sprintf(" (-%d point)", decrease)
		} else {
			append2 = fmt.Sprintf(" (-%d points)", decrease)
		}
	}
	combinedPart := ""
	if lc.combined > 0 {
		combinedPart = fmt.Sprintf(" (%d)", lc.currLog)
	}
	lc.badTrack = append(lc.badTrack, "Track "+tn+combinedPart+": "+msg+append2)
}
