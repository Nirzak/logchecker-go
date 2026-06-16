package logchecker

import (
	"fmt"
	"strconv"
	"strings"
)

func (lc *Logchecker) driveCallback(m []string) string {
	driveName := strings.TrimSpace(m[2])
	if driveName == "(null) (null) (revision (null))" {
		lc.accountDeduction("Null drive used", 20)
		return settingLine("Used Drive", m[1], cssBad, m[2])
	}
	res := lc.validateDrive(driveName, driveName, "")
	displayName := m[2]
	if !res.InDB && !res.IsFake {
		displayName += " (not found in database)"
	}
	return settingLine("Used Drive", m[1], res.DriveClass, displayName)
}

func (lc *Logchecker) mediaTypeXldCallback(m []string) string {
	cls := "badish"
	if strings.TrimSpace(m[2]) == "Pressed CD" {
		cls = "good"
	} else {
		lc.account("Not a pressed cd", 0, -1, true, true)
	}
	return settingLine("Media type", m[1], cls, m[2])
}

func (lc *Logchecker) readModeCallback(m []string) string {
	cls := "bad"
	if m[2] == "Secure" {
		cls = "good"
	} else {
		lc.secureMode = false
		lc.nonSecureMode = m[2]
	}
	s := settingLine("Read mode", m[1], cls, m[2])
	if len(m) > 3 && m[3] != "" {
		s += spanClass(cssLog4, m[3])
	}
	return s
}

func (lc *Logchecker) ripperModeXldCallback(m []string) string {
	cls := "bad"
	if strings.HasPrefix(m[2], "CDParanoia") {
		cls = "good"
	} else if m[2] == "XLD Secure Ripper" {
		cls = "good"
		lc.xldSecureRipper = true
	} else {
		lc.secureMode = false
	}
	return settingLine("Ripper mode", m[1], cls, m[2])
}

func (lc *Logchecker) cdparanoiaModeXldCallback(m []string) string {
	cls := "bad"
	if strings.HasPrefix(m[2], "YES") {
		cls = "good"
	} else {
		lc.secureMode = false
	}
	return settingLine("Use cdparanoia mode", m[1], cls, m[2])
}

func (lc *Logchecker) maxRetryCountCallback(m []string) string {
	n, _ := strconv.Atoi(m[2])
	cls := cssGoodish
	if n < 10 {
		cls = cssBadish
		lc.accountDeduction(`Low "max retry count" (potentially bad setting)`, 0)
	}
	return settingLine("Max retry count", m[1], cls, m[2])
}

func (lc *Logchecker) accurateStreamCallback(m []string) string {
	cls := cssGood
	if m[2] != "Yes" {
		cls = cssBad
		lc.accountDeduction(`"Utilize accurate stream" should be yes`, 20)
	}
	return settingLine("Utilize accurate stream", m[1], cls, m[2])
}

func (lc *Logchecker) accurateStreamEacPre9Callback(m []string) string {
	cls := cssGood
	if strings.ToLower(m[1]) == "no " {
		cls = cssBad
		lc.accountDeduction(`"accurate stream" should be yes`, 20)
	}
	return ", " + spanClass(cls, m[1]+"accurate stream")
}

func (lc *Logchecker) defeatAudioCacheCallback(m []string) string {
	cls := cssGood
	if m[2] != "Yes" {
		cls = cssBad
		lc.accountDeduction(`"Defeat audio cache" should be yes`, 10)
	}
	return settingLine("Defeat audio cache", m[1], cls, m[2])
}

func (lc *Logchecker) defeatAudioCacheEacPre99Callback(m []string) string {
	cls := cssGood
	if strings.ToLower(m[1]) == "no " {
		cls = cssBad
		lc.accountDeduction("Audio cache not disabled", 10)
	}
	return " " + spanClass(cls, m[1]+"disable cache")
}

func (lc *Logchecker) defeatAudioCacheXldCallback(m []string) string {
	cls := cssBad
	if strings.HasPrefix(m[2], "OK") || strings.HasPrefix(m[2], "YES") {
		cls = cssGood
	} else {
		lc.accountDeduction(`"Disable audio cache" should be yes/ok`, 10)
	}
	return settingLine("Disable audio cache", m[1], cls, m[2])
}

func (lc *Logchecker) c2PointersCallback(m []string) string {
	cls := cssGood
	if strings.ToLower(m[2]) == "yes" {
		cls = cssBad
		lc.accountDeduction("C2 pointers were used", 10)
	}
	return settingLine("Make use of C2 pointers", m[1], cls, m[2])
}

func (lc *Logchecker) c2PointersEacPre99Callback(m []string) string {
	cls := cssGood
	if strings.ToLower(m[1]) != "no " {
		cls = cssBad
		lc.accountDeduction("C2 pointers were used", 10)
	}
	return "with " + spanClass(cls, m[1]+"C2")
}

func (lc *Logchecker) readOffsetCallback(m []string) string {
	cls := cssBadish
	if lc.driveFound {
		found := false
		for _, o := range lc.offsets {
			if o == m[2] {
				found = true
				break
			}
		}
		if found {
			cls = cssGood
		} else {
			cls = cssBad
			lc.accountIncorrectOffset()
		}
	} else if m[2] == "0" {
		cls = cssBad
		lc.accountZeroOffsetUnknownDrive()
	}
	return settingLine("Read offset correction", m[1], cls, m[2])
}

func (lc *Logchecker) fillOffsetSamplesCallback(m []string) string {
	cls := cssGood
	if m[2] != "Yes" {
		cls = cssBad
		lc.accountDeduction("Does not fill up missing offset samples with silence", 5)
	}
	return settingLine("Fill up missing offset samples with silence", m[1], cls, m[2])
}

func (lc *Logchecker) deleteSilentBlocksCallback(m []string) string {
	cls := cssGood
	if m[3] == "Yes" {
		cls = cssBad
		lc.accountDeduction("Deletes leading and trailing silent blocks", 5)
	}
	return settingLine("Delete leading and trailing silent blocks", m[1]+m[2], cls, m[3])
}

func (lc *Logchecker) nullSamplesCallback(m []string) string {
	cls := cssGood
	if m[2] != "Yes" {
		cls = cssBad
		lc.accountDeduction("Null samples should be used in CRC calculations", 5)
	}
	return settingLine("Null samples used in CRC calculations", m[1], cls, m[2])
}

func (lc *Logchecker) normalizeEacCallback(m []string) string {
	lc.accountDeduction("Normalization should be not be active", 100)
	return settingLine("Normalize to", m[1], cssBad, m[2])
}

func (lc *Logchecker) gapHandlingCallback(m []string) string {
	cls := cssGoodish
	switch {
	case strings.Contains(m[2], "Not detected"):
		cls = cssBad
		lc.accountDeduction("Gap handling was not detected", 10)
	case strings.Contains(m[2], "Appended to next track"):
		cls = cssBad
		lc.accountDeduction("Gap handling should be appended to previous track", 10)
	case strings.Contains(m[2], "Left out"):
		cls = cssBad
		lc.accountDeduction("Gap handling should be appended to previous track", 10)
	case strings.Contains(strings.ToLower(m[2]), "appended to previous track"):
		cls = cssGood
		m[2] = strings.ReplaceAll(m[2], "Track", "track")
	}
	return settingLine("Gap handling", m[1], cls, m[2])
}

func (lc *Logchecker) gapHandlingXldCallback(m []string) string {
	lower := strings.ToLower(m[2])
	cls := cssBadish
	if strings.Contains(lower, "not") {
		cls = cssBad
		lc.accountDeduction("Incorrect gap handling", 10)
	} else if strings.Contains(lower, "analyzed") && strings.Contains(lower, "appended") {
		cls = cssGood
	} else {
		lc.accountDeduction("Incomplete gap handling", 10)
	}
	return settingLine("Gap status", m[1], cls, m[2])
}

func (lc *Logchecker) addId3TagCallback(m []string) string {
	cls := cssGood
	if m[2] == "Yes" {
		cls = cssBadish
		lc.accountDeduction("ID3 tags should not be added to FLAC files - they are mainly for MP3 files. FLACs should have vorbis comments for tags instead.", 1)
	}
	return settingLine("Add ID3 tag", m[1], cls, m[2])
}

// combinedSuffix returns " (N) " when this is a combined log, otherwise "".
func (lc *Logchecker) combinedSuffix() string {
	if lc.combined > 0 {
		return fmt.Sprintf(" (%d) ", lc.currLog)
	}
	return ""
}

func (lc *Logchecker) testCopyCallback(m []string) string {
	cls := "good"
	if m[1] != m[3] {
		cls = "bad"
		lc.accountTrack("CRC mismatch: "+m[1]+" and "+m[3], 30)
		if !lc.secureMode {
			lc.decreaseTrack += 20
			lc.badTrack = append(lc.badTrack, "Rip "+lc.combinedSuffix()+"was not done in Secure mode, and experienced CRC mismatches (-20 points)")
			lc.secureMode = true
		}
	}
	return "<span class=\"log4\">Test CRC <span class=\"" + cls + "\">" + m[1] + "</span></span>\n" +
		m[2] + "<span class=\"log4\">Copy CRC <span class=\"" + cls + "\">" + m[3] + "</span></span>"
}

func (lc *Logchecker) testCopyXldCallback(m []string) string {
	cls := "good"
	if m[2] != m[5] {
		cls = "bad"
		lc.accountTrack("CRC mismatch: "+m[2]+" and "+m[5], 30)
		if !lc.secureMode {
			lc.decreaseTrack += 20
			lc.badTrack = append(lc.badTrack, "Rip "+lc.combinedSuffix()+"was not done with Secure Ripper / in CDParanoia mode, and experienced CRC mismatches (-20 points)")
			lc.secureMode = true
		}
	}
	return "<span class=\"log4\">CRC32 hash (test run)" + m[1] + " <span class=\"" + cls + "\">" + m[2] + "</span></span>\n" +
		m[3] + "<span class=\"log4\">CRC32 hash" + m[4] + " <span class=\"" + cls + "\">" + m[5] + "</span></span>"
}

func (lc *Logchecker) arXldCallback(m []string) string {
	cls := "badish"
	if strings.Contains(strings.ToLower(m[4]), "accurately ripped") {
		conf := strings.TrimSuffix(m[6], ")")
		n, _ := strconv.Atoi(conf)
		if n < 2 {
			cls = "goodish"
		} else {
			cls = "good"
		}
	}
	return "<span class=\"log4\">AccurateRip signature" + m[1] + ": <span class=\"" + cls + "\">" + m[2] +
		"</span></span>\n" + m[3] + "<span class=\"" + cls + "\">" + m[4] + m[5] + m[6] + "</span>"
}

func (lc *Logchecker) arSummaryConfXldCallback(s string) string {
	re := xldSumOKRe
	re2 := xldSumNGRe
	if m := re.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[3])
		cls := "good"
		if n < 2 {
			cls = "goodish"
		}
		return m[1] + "<span class =\"" + cls + "\">" + s[len(m[1]):] + "</span>"
	}
	if m := re2.FindStringSubmatch(s); m != nil {
		return m[1] + "<span class =\"badish\">" + s[len(m[1]):] + "</span>"
	}
	return s
}

func (lc *Logchecker) arSummaryConfCallback(s string) string {
	re := arSummaryConfRe
	m := re.FindStringSubmatch(s)
	if m == nil {
		return s
	}
	n, _ := strconv.Atoi(m[3])
	cls := "good"
	if n < 2 {
		cls = "goodish"
	}
	return "<span class =\"" + cls + "\">" + s + "</span>"
}

func (lc *Logchecker) xldStatCallback(m []string) string {
	text := m[1] + m[2]
	lower := strings.ToLower(m[1])
	n, _ := strconv.Atoi(m[3])

	switch lower {
	case "read error":
		suffix := ""
		if n != 1 {
			suffix = "s"
		}
		return lc.xldErrorStat(text, m[3], "Read error"+suffix+" detected", n)
	case "skipped (treated as error)":
		suffix := ""
		if n != 1 {
			suffix = "s"
		}
		return lc.xldErrorStat(text, m[3], "Skipped error"+suffix+" detected", n)
	case "inconsistency in error sectors":
		suffix := "y"
		if n != 1 {
			suffix = "ies"
		}
		return lc.xldErrorStat(text, m[3], "Inconsistenc"+suffix+" in error sectors detected", n)
	case "damaged sector count":
		return lc.xldErrorStat(text, m[3], "Damaged sector count of "+m[3], n)
	default:
		cls := cssGoodish
		if n > 0 {
			cls = cssBadish
		}
		return spanClass(cssLog4, text) + " " + spanClass(cls, m[3])
	}
}

func (lc *Logchecker) xldAllStatCallback(m []string) string {
	text := m[1] + m[2]
	lower := strings.ToLower(m[1])
	n, _ := strconv.Atoi(m[3])

	isBad := lower == "read error" || lower == "skipped (treated as error)" ||
		lower == "inconsistency in error sectors" || lower == "damaged sector count"
	if isBad {
		cls := "good"
		if n > 0 {
			cls = "bad"
		}
		return "<span class=\"log4\">" + text + "</span> <span class=\"" + cls + "\">" + m[3] + "</span>"
	}
	cls := "goodish"
	if n > 0 {
		cls = "badish"
	}
	return "<span class=\"log4\">" + text + "</span> <span class=\"" + cls + "\">" + m[3] + "</span>"
}
