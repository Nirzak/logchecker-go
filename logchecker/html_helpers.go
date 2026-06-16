package logchecker

import "strings"

// spanClass wraps content in a <span> with the given CSS class.
func spanClass(cls, content string) string {
	return `<span class="` + cls + `">` + content + `</span>`
}

// settingLine renders a "label: value" line with log5/cls spans.
// spacing is the whitespace captured between the label and the colon.
func settingLine(label, spacing, cls, value string) string {
	return spanClass(cssLog5, label+spacing) + ": " + spanClass(cls, value)
}

// accountVirtualDrive records a virtual-drive penalty.
// Used by driveCallback (EAC/XLD), whipperParse, and dbpowerampParse.
func (lc *Logchecker) accountVirtualDrive(driveName string) {
	lc.accountDeduction("Virtual drive used: "+driveName, 20)
}

// accountIncorrectOffset records an incorrect-offset deduction.
// Used by readOffsetCallback, whipperParse, and dbpowerampParse.
func (lc *Logchecker) accountIncorrectOffset() {
	lc.accountDeduction(
		"Incorrect read offset for drive. Correct offsets are: "+
			strings.Join(lc.offsets, ", ")+
			" (Checked against the following drive(s): "+
			strings.Join(lc.drives, ", ")+")", 5)
}

// accountZeroOffsetUnknownDrive records the "drive not found + offset is 0" deduction.
// Used by readOffsetCallback, whipperParse, and dbpowerampParse.
func (lc *Logchecker) accountZeroOffsetUnknownDrive() {
	lc.accountDeduction(
		"The drive was not found in the database, so we cannot determine the correct read offset. "+
			"However, the read offset in this case was 0, which is almost never correct. "+
			"As such, we are assuming that the offset is incorrect", 5)
}

// xldErrorStat renders one XLD status line (label + count) and, when n > 0,
// records a per-track deduction capped at 10 points. text is the label span
// content (m[1]+m[2]), valueStr is the raw count string (m[3]), and msg is the
// pre-built deduction message. Used by the error cases of xldStatCallback.
func (lc *Logchecker) xldErrorStat(text, valueStr, msg string, n int) string {
	cls := cssGood
	if n > 0 {
		cls = cssBad
		errCount := n
		if errCount > 10 {
			errCount = 10
		}
		lc.accountTrack(msg, errCount)
	}
	return spanClass(cssLog4, text) + " " + spanClass(cls, valueStr)
}
