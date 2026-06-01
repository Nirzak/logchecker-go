package logchecker

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

func (lc *Logchecker) driveCallback(m []string) string {
	driveName := strings.TrimSpace(m[2])
	if driveName == "(null) (null) (revision (null))" {
		lc.account("Null drive used", 20, -1, false, false)
		return "<span class=\"log5\">Used Drive" + m[1] + "</span>: <span class=\"bad\">" + m[2] + "</span>"
	}
	for _, f := range lc.fakeDrives {
		if driveName == f {
			lc.account("Virtual drive used: "+m[2], 20, -1, false, false)
			return "<span class=\"log5\">Used Drive" + m[1] + "</span>: <span class=\"bad\">" + m[2] + "</span>"
		}
	}
	lc.getDrives(driveName)
	cls := "badish"
	displayName := m[2]
	if len(lc.drives) > 0 {
		cls = "good"
		lc.driveFound = true
	} else {
		displayName += " (not found in database)"
		lc.driveFound = false
	}
	return "<span class=\"log5\">Used Drive" + m[1] + "</span>: <span class=\"" + cls + "\">" + displayName + "</span>"
}

func (lc *Logchecker) mediaTypeXldCallback(m []string) string {
	cls := "badish"
	if strings.TrimSpace(m[2]) == "Pressed CD" {
		cls = "good"
	} else {
		lc.account("Not a pressed cd", 0, -1, true, true)
	}
	return "<span class=\"log5\">Media type" + m[1] + "</span>: <span class=\"" + cls + "\">" + m[2] + "</span>"
}

func (lc *Logchecker) readModeCallback(m []string) string {
	cls := "bad"
	if m[2] == "Secure" {
		cls = "good"
	} else {
		lc.secureMode = false
		lc.nonSecureMode = m[2]
	}
	s := "<span class=\"log5\">Read mode" + m[1] + "</span>: <span class=\"" + cls + "\">" + m[2] + "</span>"
	if len(m) > 3 && m[3] != "" {
		s += "<span class=\"log4\">" + m[3] + "</span>"
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
	return "<span class=\"log5\">Ripper mode" + m[1] + "</span>: <span class=\"" + cls + "\">" + m[2] + "</span>"
}

func (lc *Logchecker) cdparanoiaModeXldCallback(m []string) string {
	cls := "bad"
	if strings.HasPrefix(m[2], "YES") {
		cls = "good"
	} else {
		lc.secureMode = false
	}
	return "<span class=\"log5\">Use cdparanoia mode" + m[1] + "</span>: <span class=\"" + cls + "\">" + m[2] + "</span>"
}

func (lc *Logchecker) maxRetryCountCallback(m []string) string {
	n, _ := strconv.Atoi(m[2])
	cls := "goodish"
	if n < 10 {
		cls = "badish"
		lc.account("Low \"max retry count\" (potentially bad setting)", 0, -1, false, false)
	}
	return "<span class=\"log5\">Max retry count" + m[1] + "</span>: <span class=\"" + cls + "\">" + m[2] + "</span>"
}

func (lc *Logchecker) accurateStreamCallback(m []string) string {
	cls := "good"
	if m[2] != "Yes" {
		cls = "bad"
		lc.account(`"Utilize accurate stream" should be yes`, 20, -1, false, false)
	}
	return "<span class=\"log5\">Utilize accurate stream" + m[1] + "</span>: <span class=\"" + cls + "\">" + m[2] + "</span>"
}

func (lc *Logchecker) accurateStreamEacPre9Callback(m []string) string {
	cls := "good"
	if strings.ToLower(m[1]) == "no " {
		cls = "bad"
		lc.account(`"accurate stream" should be yes`, 20, -1, false, false)
	}
	return ", <span class=\"" + cls + "\">" + m[1] + "accurate stream</span>"
}

func (lc *Logchecker) defeatAudioCacheCallback(m []string) string {
	cls := "good"
	if m[2] != "Yes" {
		cls = "bad"
		lc.account(`"Defeat audio cache" should be yes`, 10, -1, false, false)
	}
	return "<span class=\"log5\">Defeat audio cache" + m[1] + "</span>: <span class=\"" + cls + "\">" + m[2] + "</span>"
}

func (lc *Logchecker) defeatAudioCacheEacPre99Callback(m []string) string {
	cls := "good"
	if strings.ToLower(m[1]) == "no " {
		cls = "bad"
		lc.account("Audio cache not disabled", 10, -1, false, false)
	}
	return " <span class=\"" + cls + "\">" + m[1] + "disable cache</span>"
}

func (lc *Logchecker) defeatAudioCacheXldCallback(m []string) string {
	cls := "bad"
	if strings.HasPrefix(m[2], "OK") || strings.HasPrefix(m[2], "YES") {
		cls = "good"
	} else {
		lc.account(`"Disable audio cache" should be yes/ok`, 10, -1, false, false)
	}
	return "<span class=\"log5\">Disable audio cache" + m[1] + "</span>: <span class=\"" + cls + "\">" + m[2] + "</span>"
}

func (lc *Logchecker) c2PointersCallback(m []string) string {
	cls := "good"
	if strings.ToLower(m[2]) == "yes" {
		cls = "bad"
		lc.account("C2 pointers were used", 10, -1, false, false)
	}
	return "<span class=\"log5\">Make use of C2 pointers" + m[1] + "</span>: <span class=\"" + cls + "\">" + m[2] + "</span>"
}

func (lc *Logchecker) c2PointersEacPre99Callback(m []string) string {
	cls := "good"
	if strings.ToLower(m[1]) == "no " {
		cls = "good"
	} else {
		cls = "bad"
		lc.account("C2 pointers were used", 10, -1, false, false)
	}
	return "with <span class=\"" + cls + "\">" + m[1] + "C2</span>"
}

func (lc *Logchecker) readOffsetCallback(m []string) string {
	cls := "badish"
	if lc.driveFound {
		found := false
		for _, o := range lc.offsets {
			if o == m[2] {
				found = true
				break
			}
		}
		if found {
			cls = "good"
		} else {
			cls = "bad"
			msg := "Incorrect read offset for drive. Correct offsets are: " +
				strings.Join(lc.offsets, ", ") +
				" (Checked against the following drive(s): " + strings.Join(lc.drives, ", ") + ")"
			lc.account(msg, 5, -1, false, false)
		}
	} else if m[2] == "0" {
		cls = "bad"
		lc.account("The drive was not found in the database, so we cannot determine the correct read offset. "+
			"However, the read offset in this case was 0, which is almost never correct. As such, we are "+
			"assuming that the offset is incorrect", 5, -1, false, false)
	}
	return "<span class=\"log5\">Read offset correction" + m[1] + "</span>: <span class=\"" + cls + "\">" + m[2] + "</span>"
}

func (lc *Logchecker) fillOffsetSamplesCallback(m []string) string {
	cls := "good"
	if m[2] != "Yes" {
		cls = "bad"
		lc.account("Does not fill up missing offset samples with silence", 5, -1, false, false)
	}
	return "<span class=\"log5\">Fill up missing offset samples with silence" + m[1] + "</span>: <span class=\"" + cls + "\">" + m[2] + "</span>"
}

func (lc *Logchecker) deleteSilentBlocksCallback(m []string) string {
	cls := "good"
	if m[3] == "Yes" {
		cls = "bad"
		lc.account("Deletes leading and trailing silent blocks", 5, -1, false, false)
	}
	return "<span class=\"log5\">Delete leading and trailing silent blocks" + m[1] + m[2] + "</span>: <span class=\"" + cls + "\">" + m[3] + "</span>"
}

func (lc *Logchecker) nullSamplesCallback(m []string) string {
	cls := "good"
	if m[2] != "Yes" {
		cls = "bad"
		lc.account("Null samples should be used in CRC calculations", 5, -1, false, false)
	}
	return "<span class=\"log5\">Null samples used in CRC calculations" + m[1] + "</span>: <span class=\"" + cls + "\">" + m[2] + "</span>"
}

func (lc *Logchecker) normalizeEacCallback(m []string) string {
	lc.account("Normalization should be not be active", 100, -1, false, false)
	return "<span class=\"log5\">Normalize to" + m[1] + "</span>: <span class=\"bad\">" + m[2] + "</span>"
}

func (lc *Logchecker) gapHandlingCallback(m []string) string {
	cls := "goodish"
	switch {
	case strings.Contains(m[2], "Not detected"):
		cls = "bad"
		lc.account("Gap handling was not detected", 10, -1, false, false)
	case strings.Contains(m[2], "Appended to next track"):
		cls = "bad"
		lc.account("Gap handling should be appended to previous track", 10, -1, false, false)
	case strings.Contains(m[2], "Left out"):
		cls = "bad"
		lc.account("Gap handling should be appended to previous track", 10, -1, false, false)
	case strings.Contains(strings.ToLower(m[2]), "appended to previous track"):
		cls = "good"
		m[2] = strings.ReplaceAll(m[2], "Track", "track")
	}
	return "<span class=\"log5\">Gap handling" + m[1] + "</span>: <span class=\"" + cls + "\">" + m[2] + "</span>"
}

func (lc *Logchecker) gapHandlingXldCallback(m []string) string {
	lower := strings.ToLower(m[2])
	cls := "badish"
	if strings.Contains(lower, "not") {
		cls = "bad"
		lc.account("Incorrect gap handling", 10, -1, false, false)
	} else if strings.Contains(lower, "analyzed") && strings.Contains(lower, "appended") {
		cls = "good"
	} else {
		lc.account("Incomplete gap handling", 10, -1, false, false)
	}
	return "<span class=\"log5\">Gap status" + m[1] + "</span>: <span class=\"" + cls + "\">" + m[2] + "</span>"
}

func (lc *Logchecker) addId3TagCallback(m []string) string {
	cls := "good"
	if m[2] == "Yes" {
		cls = "badish"
		lc.account("ID3 tags should not be added to FLAC files - they are mainly for MP3 files. FLACs should have vorbis comments for tags instead.", 1, -1, false, false)
	}
	return "<span class=\"log5\">Add ID3 tag" + m[1] + "</span>: <span class=\"" + cls + "\">" + m[2] + "</span>"
}

func (lc *Logchecker) testCopyCallback(m []string) string {
	cls := "good"
	if m[1] != m[3] {
		cls = "bad"
		lc.accountTrack("CRC mismatch: "+m[1]+" and "+m[3], 30)
		if !lc.secureMode {
			lc.decreaseTrack += 20
			lc.badTrack = append(lc.badTrack, "Rip "+func() string {
				if lc.combined > 0 {
					return fmt.Sprintf(" (%d) ", lc.currLog)
				}
				return ""
			}()+"was not done in Secure mode, and experienced CRC mismatches (-20 points)")
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
			lc.badTrack = append(lc.badTrack, "Rip "+func() string {
				if lc.combined > 0 {
					return fmt.Sprintf(" (%d) ", lc.currLog)
				}
				return ""
			}()+"was not done with Secure Ripper / in CDParanoia mode, and experienced CRC mismatches (-20 points)")
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
	re := regexp.MustCompile(`(?i)(Track +\d+ +: +)(OK +)\(A?R?\d?,? ?confidence +(\d+).*?\)(.*)\n`)
	re2 := regexp.MustCompile(`(?i)(Track +\d+ +: +)(NG|Not Found).*?\n`)
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
	re := regexp.MustCompile(`(?i)(Track +\d+ +.*?accurately ripped\.? *)(\(confidence +)(\d+)\)(.*)\n`)
	m := re.FindStringSubmatch(s)
	if m == nil {
		return s
	}
	n, _ := strconv.Atoi(m[3])
	cls := "good"
	if n < 2 {
		cls = "goodish"
		if _, ok := lc.arSummary["goodish"]; !ok {
			lc.arSummary["goodish"] = []int{}
		}
	} else {
		if _, ok := lc.arSummary["good"]; !ok {
			lc.arSummary["good"] = []int{}
		}
	}
	return "<span class =\"" + cls + "\">" + s + "</span>"
}

func (lc *Logchecker) xldStatCallback(m []string) string {
	text := m[1] + m[2]
	lower := strings.ToLower(m[1])
	n, _ := strconv.Atoi(m[3])

	switch lower {
	case "read error":
		cls := "good"
		if n > 0 {
			cls = "bad"
			err := n
			if err > 10 {
				err = 10
			}
			suffix := ""
			if n != 1 {
				suffix = "s"
			}
			lc.accountTrack("Read error"+suffix+" detected", err)
		}
		return "<span class=\"log4\">" + text + "</span> <span class=\"" + cls + "\">" + m[3] + "</span>"
	case "skipped (treated as error)":
		cls := "good"
		if n > 0 {
			cls = "bad"
			err := n
			if err > 10 {
				err = 10
			}
			suffix := ""
			if n != 1 {
				suffix = "s"
			}
			lc.accountTrack("Skipped error"+suffix+" detected", err)
		}
		return "<span class=\"log4\">" + text + "</span> <span class=\"" + cls + "\">" + m[3] + "</span>"
	case "inconsistency in error sectors":
		cls := "good"
		if n > 0 {
			cls = "bad"
			err := n
			if err > 10 {
				err = 10
			}
			suffix := "y"
			if n != 1 {
				suffix = "ies"
			}
			lc.accountTrack("Inconsistenc"+suffix+" in error sectors detected", err)
		}
		return "<span class=\"log4\">" + text + "</span> <span class=\"" + cls + "\">" + m[3] + "</span>"
	case "damaged sector count":
		cls := "good"
		if n > 0 {
			cls = "bad"
			err := n
			if err > 10 {
				err = 10
			}
			lc.accountTrack("Damaged sector count of "+m[3], err)
		}
		return "<span class=\"log4\">" + text + "</span> <span class=\"" + cls + "\">" + m[3] + "</span>"
	default:
		cls := "goodish"
		if n > 0 {
			cls = "badish"
		}
		return "<span class=\"log4\">" + text + "</span> <span class=\"" + cls + "\">" + m[3] + "</span>"
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
