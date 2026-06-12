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
