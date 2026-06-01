package logchecker

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Nirzak/logchecker-go/internal/check"
	"github.com/Nirzak/logchecker-go/internal/parser/eac"
	"github.com/Nirzak/logchecker-go/internal/util"
)

var (
	eacChecksumRe  = regexp.MustCompile(`(?i)(====[ ]+Log checksum [\d\w ]+[ ]+====)`)
	eacChecksumFRe = regexp.MustCompile(`(?i)(====[ ]+[\d\w ]+[ ]+====)`) // foreign
	xldSigRe       = regexp.MustCompile(`(?is)[\-]+BEGIN XLD SIGNATURE[\S\n\-]+END XLD SIGNATURE[\-]+`)
	splitEACRe     = regexp.MustCompile(`(?i)(\n====[ ]+[\d\w ]+[ ]+====)`)
	splitXLDRe     = regexp.MustCompile(`(?is)(\n[\-]+BEGIN XLD SIGNATURE[\S\n\-]+END XLD SIGNATURE[\-]+)`)
	eacVersionRe   = regexp.MustCompile(`(?i)Exact Audio Copy (.+) from`)
	xldVersionRe   = regexp.MustCompile(`(?i)X Lossless Decoder version (\d+) \((.+)\)`)
	eacOldRe       = regexp.MustCompile(`(?i)EAC extraction logfile from`)
	eacLogRe       = regexp.MustCompile(`(?i)Exact Audio Copy (.+) from (.+)`)
	eacLogfileRe   = regexp.MustCompile(`(?i)EAC extraction logfile from (.+)\n+(.+)`)
	xldLogfileRe   = regexp.MustCompile(`(?i)XLD extraction logfile from (.+)\n+(.+)`)
	legacyXLDRe    = regexp.MustCompile(`(?i)X Lossless Decoder version (.+) \((.+)\)`)
)

func (lc *Logchecker) legacyParse() {
	// EAC translation
	if lc.ripper == check.EAC {
		info, err := eac.GetLanguage(lc.log)
		if err == nil {
			if info.Code != "en" {
				lc.language = info.Code
				lc.account(fmt.Sprintf("Translated log from %s (%s) to English.", info.Name, info.NameEnglish),
					0, -1, false, true)
				if translated, _, err2 := eac.Translate(lc.log, info.Code); err2 == nil {
					lc.log = translated
				}
			}
		} else {
			lc.language = "en"
			lc.account("Could not determine language. Assuming English.", 0, -1, false, true)
		}
	}

	lc.log = util.NormalizeLineEndings(lc.log)

	// Determine checksum regex and split
	checksumRe := eacChecksumRe
	if lc.language != "en" {
		checksumRe = eacChecksumFRe
	}

	if checksumRe.MatchString(lc.log) {
		lc.logs = splitWithDelim(lc.log, splitEACRe)
	} else if xldSigRe.MatchString(lc.log) {
		lc.logs = splitWithDelim(lc.log, splitXLDRe)
	} else {
		lc.checksumStatus = check.ChecksumMissing
		lc.logs = splitWithDelim(lc.log, regexp.MustCompile(`(?i)(\nEnd of status report)`))
		// Remove CUETools DB Plugin entries
		var filtered []string
		for _, l := range lc.logs {
			if !regexp.MustCompile(`(?i)---- CUETools DB Plugin V.+`).MatchString(l) {
				filtered = append(filtered, l)
			}
		}
		lc.logs = filtered
	}

	// Clean up log segments
	var cleaned []string
	for i, rawPiece := range lc.logs {
		l := strings.TrimSpace(rawPiece)
		if l == "" || regexp.MustCompile(`(?i)^\-+$`).MatchString(l) {
			continue
		}
		if lc.checksumStatus != check.ChecksumOK && regexp.MustCompile(`(?i)End of status report`).MatchString(l) {
			if len(cleaned) > 0 {
				// Preserve trailing blank line from the preceding piece if present.
				var prevRaw string
				for j := i - 1; j >= 0; j-- {
					if strings.TrimSpace(lc.logs[j]) != "" {
						prevRaw = lc.logs[j]
						break
					}
				}
				trailing := len(prevRaw) - len(strings.TrimRight(prevRaw, "\n\r"))
				sep := "\n"
				if trailing >= 2 {
					sep = "\n\n"
				}
				cleaned[len(cleaned)-1] += sep + l
			}
			continue
		}
		if lc.checksumStatus == check.ChecksumOK && checksumRe.MatchString(l) {
			if len(cleaned) > 0 {
				// Count trailing newlines in the preceding raw piece to decide
				// whether a blank line appears before the checksum.
				var prevRaw string
				for j := i - 1; j >= 0; j-- {
					if strings.TrimSpace(lc.logs[j]) != "" {
						prevRaw = lc.logs[j]
						break
					}
				}
				trailing := len(prevRaw) - len(strings.TrimRight(prevRaw, "\n\r"))
				sep := "\n"
				if trailing >= 2 {
					sep = "\n\n"
				}
				cleaned[len(cleaned)-1] += sep + l
			}
			continue
		}
		if lc.checksumStatus == check.ChecksumOK && regexp.MustCompile(`(?i)[\-]+BEGIN XLD SIGNATURE`).MatchString(l) {
			if len(cleaned) > 0 {
				cleaned[len(cleaned)-1] += "\n" + l
			}
			continue
		}
		_ = i
		cleaned = append(cleaned, l)
	}
	lc.logs = cleaned

	if len(lc.logs) > 1 {
		lc.combined = len(lc.logs)
	}

	for logIdx, rawLog := range lc.logs {
		lc.tracks[logIdx] = make(map[string]trackData)
		lc.currLog = logIdx + 1

		// Version detection
		isEAC := 0
		isXLD := 0

		if m := eacVersionRe.FindStringSubmatch(rawLog); m != nil {
			lc.ripperVersion = strings.TrimLeft(m[1], "V")
			vcheck := strings.Split(lc.ripperVersion, " ")[0]
			if compareVersions(vcheck, "1.0") < 0 {
				lc.checksumStatus = check.ChecksumMissing
				v, _ := strconv.ParseFloat(vcheck, 64)
				if v <= 0.95 {
					lc.account("EAC version older than 0.99", 30, -1, false, false)
				}
			} else if !checksumRe.MatchString(rawLog) {
				lc.checksumStatus = check.ChecksumMissing
			}
		} else if eacOldRe.MatchString(rawLog) {
			lc.checksumStatus = check.ChecksumMissing
			lc.account("EAC version older than 0.99", 30, -1, false, false)
		}

		if m := xldVersionRe.FindStringSubmatch(rawLog); m != nil {
			lc.ripperVersion = m[1]
			ver, _ := strconv.Atoi(lc.ripperVersion)
			if ver >= 20121222 && !xldSigRe.MatchString(rawLog) {
				lc.checksumStatus = check.ChecksumMissing
			}
		}

		// HTML annotations — version headers
		rawLog, countEAC := replaceCount(rawLog, eacLogRe,
			`Exact Audio Copy <span class="log1">$1</span> from <span class="log1">$2</span>`, 1)
		isEAC = countEAC

		rawLog, countEACFile := replaceCount(rawLog, eacLogfileRe,
			"<span class='good'>EAC extraction logfile from <span class='log5'>$1</span></span>\n\n<span class=\"log4\">$2</span>", 1)
		if countEACFile > 0 {
			isEAC = countEACFile
		}

		rawLog, countXLD := replaceCount(rawLog, legacyXLDRe,
			`X Lossless Decoder version <span class="log1">$1</span> (<span class="log1">$2</span>)`, 1)
		isXLD = countXLD

		rawLog, countXLDFile := replaceCount(rawLog, xldLogfileRe,
			"<span class='good'>XLD extraction logfile from <span class='log5'>$1</span></span>\n\n<span class=\"log4\">$2</span>", 1)
		if countXLDFile > 0 {
			isXLD = countXLDFile
		}

		if isEAC == 0 && isXLD == 0 {
			if lc.combined > 0 {
				lc.details = []string{
					fmt.Sprintf("Combined Log (%d)", lc.combined),
					fmt.Sprintf("Unrecognized log file (%d)! Feel free to report for manual review.", lc.currLog),
				}
			} else {
				lc.details = []string{"Unrecognized log file! Feel free to report for manual review."}
			}
			lc.score = 0
			return
		}

		// Checksum validation
		if lc.validateChecksum && lc.checksumStatus == check.ChecksumOK && lc.logPath != "" {
			lc.checksumStatus = check.Validate(lc.logPath, lc.ripper)
		}

		// Annotate checksum block
		csClass := "good"
		if lc.checksumStatus != check.ChecksumOK {
			csClass = "bad"
		}
		rawLog, eacCount := replaceCount(rawLog, checksumRe, "<span class='"+csClass+"'>$1</span>", 1)
		rawLog, xldCount := replaceCount(rawLog, regexp.MustCompile(`(?is)([\-]+BEGIN XLD SIGNATURE[\S\n\-]+END XLD SIGNATURE[\-]+)`),
			"<span class='"+csClass+"'>$1</span>", 1)

		if (eacCount > 0 || xldCount > 0) && lc.checksumStatus == check.ChecksumMissing {
			lc.checksumStatus = check.ChecksumInvalid
		}

		// MP3 check for EAC
		if isEAC > 0 {
			if regexp.MustCompile(`(?i)Used output format[ ]+:[ ]+[a-z0-9 ]+MP3`).MatchString(rawLog) ||
				regexp.MustCompile(`(?i)Command line compressor[ ]+:.+(MP3|lame)\.exe`).MatchString(rawLog) {
				if lc.combined > 0 {
					lc.details = append(lc.details, fmt.Sprintf("Skipping Log (%d), MP3 Rip", lc.currLog))
				} else {
					lc.account("Invalid Log (MP3)", 100, -1, false, false)
				}
				lc.logs[logIdx] = rawLog
				continue
			}
		}

		// --- drive ---
		rawLog, cnt := replaceCountCallback(rawLog, regexp.MustCompile(`(?i)Used drive( *): (.+)`), lc.driveCallback, 1)
		if cnt == 0 {
			lc.account("Could not verify used drive", 1, -1, false, false)
		}

		// --- media type (XLD) ---
		rawLog, cnt = replaceCountCallback(rawLog, regexp.MustCompile(`(?i)Media type( *): (.+)`), lc.mediaTypeXldCallback, 1)
		if isXLD > 0 && lc.ripperVersion != "" {
			ripVer, _ := strconv.Atoi(lc.ripperVersion)
			if ripVer >= 20130127 && cnt == 0 {
				lc.account("Could not verify media type", 1, -1, false, false)
			}
		}

		// --- read mode ---
		rawLog, cnt = replaceCountCallback(rawLog, regexp.MustCompile(`(?i)Read mode( *): ([a-z]+)(.*)?`), lc.readModeCallback, 1)
		if isEAC > 0 && cnt == 0 {
			lc.account("Could not verify read mode", 1, -1, false, false)
		}

		// --- XLD ripper/cdparanoia mode ---
		rawLog, ripperModeCnt := replaceCountCallback(rawLog, regexp.MustCompile(`(?i)Ripper mode( *): (.*)`), lc.ripperModeXldCallback, 1)
		rawLog, cdParanoiaCnt := replaceCountCallback(rawLog, regexp.MustCompile(`(?i)Use cdparanoia mode( *): (.*)`), lc.cdparanoiaModeXldCallback, 1)
		if isXLD > 0 && ripperModeCnt == 0 && cdParanoiaCnt == 0 {
			lc.account("Could not verify read mode", 1, -1, false, false)
		}

		// --- max retry count (XLD) ---
		rawLog, cnt = replaceCountCallback(rawLog, regexp.MustCompile(`(?i)Max retry count( *): (\d+)`), lc.maxRetryCountCallback, 1)
		if isXLD > 0 && cnt == 0 {
			lc.account("Could not verify max retry count", 0, -1, false, false) // notice only historically
		}

		// --- accurate stream ---
		rawLog, acsCnt := replaceCountCallback(rawLog, regexp.MustCompile(`(?i)Utilize accurate stream( *): (Yes|No)`), lc.accurateStreamCallback, 1)
		rawLog, acsPre99Cnt := replaceCountCallback(rawLog, regexp.MustCompile(`, (|NO )accurate stream`), lc.accurateStreamEacPre9Callback, 1)
		if isEAC > 0 && acsCnt == 0 && acsPre99Cnt == 0 && !lc.isNonSecure() {
			lc.account("Could not verify accurate stream", 20, -1, false, false)
		}

		// --- defeat audio cache ---
		rawLog, defCnt := replaceCountCallback(rawLog, regexp.MustCompile(`(?i)Defeat audio cache( *): (Yes|No)`), lc.defeatAudioCacheCallback, 1)
		rawLog, defPre99Cnt := replaceCountCallback(rawLog, regexp.MustCompile(` (|NO )disable cache`), lc.defeatAudioCacheEacPre99Callback, 1)
		rawLog, cnt = replaceCountCallback(rawLog, regexp.MustCompile(`(?i)Disable audio cache( *): (.*)`), lc.defeatAudioCacheXldCallback, 1)
		if isEAC > 0 && defCnt == 0 && defPre99Cnt == 0 && !lc.isNonSecure() {
			lc.account("Could not verify defeat audio cache", 1, -1, false, false)
		}
		if isXLD > 0 && cnt == 0 {
			lc.account("Could not verify defeat audio cache", 1, -1, false, false)
		}

		// --- C2 pointers ---
		rawLog, c2Cnt := replaceCountCallback(rawLog, regexp.MustCompile(`(?i)Make use of C2 pointers( *): (Yes|No)`), lc.c2PointersCallback, 1)
		rawLog, c2Pre99Cnt := replaceCountCallback(rawLog, regexp.MustCompile(`with (|NO )C2`), lc.c2PointersEacPre99Callback, 1)
		if c2Cnt == 0 && c2Pre99Cnt == 0 && !lc.isNonSecure() {
			lc.account("Could not verify C2 pointers", 1, -1, false, false)
		}

		// --- read offset ---
		rawLog, cnt = replaceCountCallback(rawLog, regexp.MustCompile(`(?i)Read offset correction( *): ([+-]?[0-9]+)`), lc.readOffsetCallback, 1)
		if cnt == 0 {
			lc.account("Could not verify read offset", 1, -1, false, false)
		}

		// combined read/write offset
		rawLog, cnt = replaceCount(rawLog, regexp.MustCompile(`(?i)(Combined read/write offset correction\s*:\s+\d+)`),
			`<span class="bad">$1</span>`, 1)
		if cnt > 0 {
			lc.account("Combined read/write offset cannot be verified", 4, -1, false, false)
		}

		// XLD alternate offset table
		rawLog, _ = replaceCount(rawLog, regexp.MustCompile(`(?i)(List of \w+ offset correction values) *(\n+)(( *.*confidence .*\) ?\n)+)`),
			`<span class="log5">$1</span>$2<span class="log4">$3</span>`+"\n", 1)
		rawLog, _ = replaceCount(rawLog, regexp.MustCompile(`(?i)(List of \w+ offset correction values) *\n( *# +\| +Absolute +\| +Relative +\| +Confidence) *\n( *\-+) *\n(( *\d+ +\| +\-?\ +?\ +\d+ +\| +\-?\ +?\ +\d+ +\| +\d+ *\n)+)`),
			`<span class="log5">$1</span>`+"\n"+`<span class="log4">$2`+"\n"+"$3\n$4\n</span>", 1)

		// overread
		rawLog, _ = replaceCount(rawLog, regexp.MustCompile(`(?i)Overread into Lead-In and Lead-Out( +): (Yes|No)`),
			`<span class="log5">Overread into Lead-In and Lead-Out$1</span>: <span class="log4">$2</span>`, 1)

		// fill offset samples
		rawLog, cnt = replaceCountCallback(rawLog, regexp.MustCompile(`(?i)Fill up missing offset samples with silence( +): (Yes|No)`), lc.fillOffsetSamplesCallback, 1)
		if isEAC > 0 && cnt == 0 {
			lc.account("Could not verify missing offset samples", 1, -1, false, false)
		}

		// delete silent blocks
		rawLog, cnt = replaceCountCallback(rawLog, regexp.MustCompile(`(?i)Delete leading and trailing silent blocks([ \w]*)( +): (Yes|No)`), lc.deleteSilentBlocksCallback, 1)
		if isEAC > 0 && cnt == 0 {
			lc.account("Could not verify silent blocks", 1, -1, false, false)
		}

		// null samples
		rawLog, cnt = replaceCountCallback(rawLog, regexp.MustCompile(`(?i)Null samples used in CRC calculations( +): (Yes|No)`), lc.nullSamplesCallback, 1)
		if isEAC > 0 && cnt == 0 {
			lc.account("Could not verify null samples", 0, -1, false, false)
		}

		// normalize
		rawLog, _ = replaceCountCallback(rawLog, regexp.MustCompile(`(?i)Normalize to( +): ([0-9% ]+)`), lc.normalizeEacCallback, 1)

		// interface, output format, etc.
		rawLog, _ = replaceCount(rawLog, regexp.MustCompile(`(?i)Used interface( +): ([^\n]+)`),
			`<span class="log5">Used interface$1</span>: <span class="log4">$2</span>`, 1)

		// gap handling
		rawLog, cnt = replaceCountCallback(rawLog, regexp.MustCompile(`(?i)Gap handling( +): ([^\n]+)`), lc.gapHandlingCallback, 1)
		if isEAC > 0 && cnt == 0 {
			lc.account("Could not verify gap handling", 10, -1, false, false)
		}
		rawLog, cnt = replaceCountCallback(rawLog, regexp.MustCompile(`(?i)Gap status( +): (.*)`), lc.gapHandlingXldCallback, 1)
		if isXLD > 0 && cnt == 0 {
			lc.account("Could not verify gap status", 10, -1, false, false)
		}

		// output format, sample format, selected bitrate, quality
		for _, pat := range []struct{ re, repl string }{
			{`(?i)Used output format( *): ([^\n]+)`, `<span class="log5">Used output format$1</span>: <span class="log4">$2</span>`},
			{`(?i)Sample format( +): ([^\n]+)`, `<span class="log5">Sample format$1</span>: <span class="log4">$2</span>`},
			{`(?i)Selected bitrate( +): ([^\n]+)`, `<span class="log5">Selected bitrate$1</span>: <span class="log4">$2</span>`},
			{`(?i)( +)(\d+ kBit/s)`, `$1<span class="log4">$2</span>`},
			{`(?i)Quality( +): ([^\n]+)`, `<span class="log5">Quality$1</span>: <span class="log4">$2</span>`},
		} {
			rawLog, _ = replaceCount(rawLog, regexp.MustCompile(pat.re), pat.repl, 1)
		}

		// ID3 tag
		rawLog, cnt = replaceCountCallback(rawLog, regexp.MustCompile(`(?i)Add ID3 tag( +): (Yes|No)`), lc.addId3TagCallback, 1)
		if isEAC > 0 && cnt == 0 {
			lc.account("Could not verify id3 tag setting", 0, -1, false, false)
		}

		// compression offset
		rawLog, cnt = replaceCount(rawLog, regexp.MustCompile(`(?i)(Use compression offset\s+:\s+\d+)`),
			`<span class="bad">$1</span>`, 1)
		if cnt > 0 {
			lc.account("Ripped with compression offset", 0, 0, false, false)
		}

		// command line compressor, additional options
		rawLog, _ = replaceCount(rawLog, regexp.MustCompile(`(?i)Command line compressor( +): ([^\n]+)`),
			`<span class="log5">Command line compressor$1</span>: <span class="log4">$2</span>`, 1)
		rawLog = regexp.MustCompile(`Additional command line options([^\n]{70,110} )`).
			ReplaceAllString(rawLog, "Additional command line options$1<br>")
		rawLog, _ = replaceCount(rawLog, regexp.MustCompile(`(?i)( *)Additional command line options( +): (.+)\n`),
			`<span class="log5">Additional command line options$2</span>: <span class="log4">$3</span>`+"\n", 1)

		// XLD range rip detection
		rawLog, xldRangeCnt := replaceCount(rawLog, regexp.MustCompile(`(?i)\n(All Tracks\n(?: *))(Filename)`),
			"\n$1<span class=\"bad\">$2</span>", 1)

		// XLD album gain
		rawLog, cnt = replaceCount(rawLog, regexp.MustCompile(`(?i)All Tracks(\s*\n)((?:.*)\n)?(\s*Album gain\s+:) (.*)? (\n\s*Peak\s+:) (.*)?`),
			`<span class="log5">All Tracks</span>$1$2<strong>$3 <span class="log3">$4</span>`+"$5 <span class=\"log3\">$6</span></strong>", 1)
		if isXLD > 0 && cnt == 0 {
			lc.account("Could not verify album gain", 0, -1, false, false)
		}

		// pre-0.99 other options
		rawLog, _ = replaceCount(rawLog, regexp.MustCompile(`(?i)Other options( +):`),
			`<span class="log5">Other options$1</span>:`, 1)
		rawLog, _ = replaceCount(rawLog, regexp.MustCompile(`(?i)\n( *)Native Win32 interface(.+)`),
			"\n$1<span class=\"log4\">Native Win32 interface$2</span>", 1)

		// TOC
		rawLog = strings.ReplaceAll(rawLog, "TOC of the extracted CD",
			`<span class="log4 log5">TOC of the extracted CD</span>`)
		rawLog, _ = replaceCount(rawLog, regexp.MustCompile(`(?i)( +)Track( +)\|( +)Start( +)\|( +)Length( +)\|( +)Start sector( +)\|( +)End sector( ?)`),
			"<strong>$0</strong>", 1)
		rawLog = regexp.MustCompile(`-{10,100}`).ReplaceAllString(rawLog, "<strong>$0</strong>")
		rawLog = regexp.MustCompile(`(?i)( +)([0-9]{1,3})( +)\|( +)(([0-9]{1,3}:)?[0-9]{2}[.:][0-9]{2})( +)\|( +)(([0-9]{1,3}:)?[0-9]{2}[.:][0-9]{2})( +)\|( +)([0-9]{1,10})( +)\|( +)([0-9]{1,10})( +)\n`).
			ReplaceAllStringFunc(rawLog, func(s string) string {
				m := regexp.MustCompile(`(?i)( +)([0-9]{1,3})( +)\|( +)(([0-9]{1,3}:)?[0-9]{2}[.:][0-9]{2})( +)\|( +)(([0-9]{1,3}:)?[0-9]{2}[.:][0-9]{2})( +)\|( +)([0-9]{1,10})( +)\|( +)([0-9]{1,10})( +)\n`).FindStringSubmatch(s)
				if m == nil {
					return s
				}
				return m[1] + "<span class=\"log4\">" + m[2] + "</span>" + m[3] + "<strong>|</strong>" +
					m[4] + "<span class=\"log1\">" + m[5] + "</span>" + m[7] + "<strong>|</strong>" +
					m[8] + "<span class=\"log1\">" + m[9] + "</span>" + m[11] + "<strong>|</strong>" +
					m[12] + "<span class=\"log1\">" + m[13] + "</span>" + m[14] + "<strong>|</strong>" +
					m[15] + "<span class=\"log1\">" + m[16] + "</span>" + m[17] + "\n"
			})

		// AccurateRip lines
		rawLog = strings.ReplaceAll(rawLog,
			"None of the tracks are present in the AccurateRip database",
			`<span class="badish">None of the tracks are present in the AccurateRip database</span>`)
		rawLog = strings.ReplaceAll(rawLog,
			"Disc not found in AccurateRip DB.",
			`<span class="badish">Disc not found in AccurateRip DB.</span>`)
		rawLog = regexp.MustCompile(`(?i)No errors occurr?ed`).ReplaceAllString(rawLog, `<span class="good">No errors occurred</span>`)
		rawLog = regexp.MustCompile(`(?i)(There were errors) ?\n`).ReplaceAllString(rawLog, `<span class="bad">$1</span>`+"\n")
		rawLog = regexp.MustCompile(`(?i)(Some inconsistencies found) ?\n`).ReplaceAllString(rawLog, `<span class="badish">$1</span>`+"\n")
		rawLog = regexp.MustCompile(`(?i)End of status report`).ReplaceAllString(rawLog, `<span class="good">End of status report</span>`)
		rawLog = regexp.MustCompile(`(?i)Track(\s*)Ripping Status(\s*)\[Disc ID: ([0-9a-f]{8}-[0-9a-f]{8})\]`).
			ReplaceAllString(rawLog, "<strong>Track</strong>$1<strong>Ripping Status</strong>$2<strong>Disc ID: </strong><span class=\"log1\">$3</span>")
		rawLog = regexp.MustCompile(`(?i)(All Tracks Accurately Ripped\.?)`).ReplaceAllString(rawLog, `<span class="good">$1</span>`)
		rawLog = regexp.MustCompile(`(?i)\d+ track.* +accurately ripped\.? *\n`).ReplaceAllString(rawLog, `<span class="good">$0</span>`)
		rawLog = regexp.MustCompile(`(?i)\d+ track.* +not present in the AccurateRip database\.? *\n`).ReplaceAllString(rawLog, `<span class="badish">$0</span>`)
		rawLog = regexp.MustCompile(`(?i)\d+ track.* +canceled\.? *\n`).ReplaceAllString(rawLog, `<span class="bad">$0</span>`)
		rawLog = regexp.MustCompile(`(?i)\d+ track.* +could not be verified as accurate\.? *\n`).ReplaceAllString(rawLog, `<span class="badish">$0</span>`)
		rawLog = regexp.MustCompile(`(?i)Some tracks could not be verified as accurate\.? *\n`).ReplaceAllString(rawLog, `<span class="badish">$0</span>`)
		rawLog = regexp.MustCompile(`(?i)No tracks could be verified as accurate\.? *\n`).ReplaceAllString(rawLog, `<span class="badish">$0</span>`)
		rawLog = regexp.MustCompile(`(?i)You may have a different pressing.*\n`).ReplaceAllString(rawLog, `<span class="goodish">$0</span>`)

		// XLD accurip summary
		rawLog = regexp.MustCompile(`(?i)(Track +\d+ +: +)(OK +)\(A?R?\d?,? ?confidence +(\d+).*?\)(.*)\ n`).
			ReplaceAllStringFunc(rawLog, lc.arSummaryConfXldCallback)
		rawLog = regexp.MustCompile(`(?i)(Track +\d+ +: +)(NG|Not Found).*?\n`).
			ReplaceAllStringFunc(rawLog, lc.arSummaryConfXldCallback)

		// Status line
		rawLog, _ = replaceCount(rawLog, regexp.MustCompile(`(?i)( *.{2} ?)(\d+ track\(s\).*)\ n`),
			`$1<span class="log4">$2</span>`+"\n", 1)

		// AccurateRip summary (range)
		rawLog = regexp.MustCompile(`(?i)\n( *AccurateRip summary\.?)`).
			ReplaceAllString(rawLog, "\n<span class=\"log4 log5\">$1</span>")
		rawLog = regexp.MustCompile(`(?i)(Track +\d+ +.*?accurately ripped\.? *)(\(confidence +)(\d+)\)(.*)\n`).
			ReplaceAllStringFunc(rawLog, lc.arSummaryConfCallback)
		rawLog, cnt = replaceCount(rawLog, regexp.MustCompile(`(?i)(Track +\d+ +.*?in database *)\n`),
			`<span class="badish">$1</span>`+"\n", -1)
		if cnt > 0 {
			lc.arSummary["bad"] = cnt
		}
		rawLog, cnt = replaceCount(rawLog, regexp.MustCompile(`(?i)(Track +\d+ +.*?(could not|cannot) be verified as accurate.*)\n`),
			`<span class="badish">$1</span>`+"\n", -1)
		if cnt > 0 {
			lc.arSummary["bad"] = cnt
		}

		// Range rip detection
		rawLog, range1Cnt := replaceCount(rawLog, regexp.MustCompile(`(?i)\n( *Selected range)`),
			"\n<span class=\"bad\">$1</span>", 1)
		rawLog, range2Cnt := replaceCount(rawLog, regexp.MustCompile(`(?i)\n( *Range status and errors)`),
			"\n<span class=\"bad\">$1</span>", 1)
		if range1Cnt > 0 || range2Cnt > 0 || xldRangeCnt > 0 {
			lc.rangeRip = true
			lc.account("Range rip detected", 30, -1, false, false)
		}

		// --- Track parsing ---
		formattedTrackListing := ""
		trackListing := ""
		var trackBodies []string
		var fullTracks []string

		logEnds := []string{
			"None of the tracks are present in the AccurateRip database",
			"No errors occurred",
			"All tracks accurately ripped",
			"No tracks could be verified as accurate",
			"track(s) not present in the AccurateRip database",
			"End of status report",
			"There were errors",
		}

		if !lc.rangeRip || isXLD > 0 {
			// Individual tracks
			m := regexp.MustCompile(`(?i)\nTrack( +)([0-9]{1,3})([\s\S]+)`).FindStringSubmatch(rawLog)
			if len(m) > 0 {
				trackListing = m[0]
				exploded := strings.Split(trackListing, "\n")
				i := len(exploded) - 1
				for i >= 0 {
					found := false
					for _, end := range logEnds {
						if strings.HasSuffix(exploded[i], end) {
							i--
							found = true
							break
						}
					}
					if found {
						break
					}
					if regexp.MustCompile(`(?i)[ \t]+ [a-z]`).MatchString(exploded[i]) {
						break
					}
					i--
				}
				trackListing = strings.Join(exploded[:i+1], "\n")
				splitRe := regexp.MustCompile(`(?i)\nTrack( +)([0-9]{1,3})`)
				fullTracks = splitWithCaptures(trackListing, splitRe)
				trackBodies = splitRe.Split(trackListing, -1)
				if len(trackBodies) > 0 {
					trackBodies = trackBodies[1:]
				}
			}
		} else {
			// Range rip
			m := regexp.MustCompile(`(?i)\n( *)Filename +(.*)([\ s\S]+)`).FindStringSubmatch(rawLog)
			if len(m) > 0 {
				trackListing = m[0]
				exploded := strings.Split(trackListing, "\n")
				i := len(exploded) - 1
				for i >= 0 {
					found := false
					for _, end := range logEnds {
						if strings.HasSuffix(exploded[i], end) {
							i--
							found = true
							break
						}
					}
					if found {
						break
					}
					if regexp.MustCompile(`(?i)[ \t]+ [a-z]`).MatchString(exploded[i]) {
						break
					}
					i--
				}
				splitRe := regexp.MustCompile(`(?i)\n( *)Filename +(.*)`)
				fullTracks = splitWithCaptures(trackListing, splitRe)
				trackBodies = splitRe.Split(trackListing, -1)
				if len(trackBodies) > 0 {
					trackBodies = trackBodies[1:]
				}
			}
		}

		for key, trackBody := range trackBodies {
			spaces := ""
			trackNumStr := ""
			if key*2+1 < len(fullTracks) {
				spaces = fullTracks[key*2]
				trackNumStr = fullTracks[key*2+1]
			}
			lc.trackNumber = trackNumStr
			lc.decreaseTrack = 0
			lc.badTrack = nil

			if !lc.rangeRip {
				trackBody = "<span class=\"log5\">Track</span>" + spaces +
					"<span class=\"log4 log1\">" + trackNumStr + "</span>" + trackBody
			} else {
				trackBody = spaces + "<span class=\"log5\">Filename</span> " +
					"<span class=\"log4 log3\">" + trackNumStr + "</span>" + trackBody
			}

			// Filename
			trackBody, cnt = replaceCount(trackBody,
				regexp.MustCompile(`(?is)Filename ((.+)?\.( wav|flac|ape))\n`),
				"<span class=\"log4\">Filename <span class=\"log3\">$1</span></span>\n", -1)
			if cnt == 0 && !lc.rangeRip {
				if isEAC > 0 {
					if m2 := regexp.MustCompile(`(?i)Filename (.+)\n`).FindStringSubmatch(trackBody); m2 != nil && len(m2[1]) >= 243 {
						trackBody, _ = replaceCount(trackBody,
							regexp.MustCompile(`(?i)Filename ((.+)?)\n`),
							"<span class=\"log4\">Filename <span class=\"log3\">$1</span></span>\n", -1)
						lc.accountTrack("Could not verify filename, too long", 0)
					} else {
						lc.accountTrack("Could not verify filename or file extension", 1)
					}
				} else {
					lc.accountTrack("Could not verify filename or file extension", 1)
				}
			}

			// File write error
			trackBody, cnt = replaceCount(trackBody,
				regexp.MustCompile(`(?i)( *)(File write error)\n`),
				"$1<span class=\"bad\">$2</span>\n", -1)
			if cnt > 0 {
				lc.accountTrack("File write error", 20)
			}

			// XLD track gain
			trackBody, _ = replaceCount(trackBody,
				regexp.MustCompile(`(?i)( *Track gain\s+:) (.*)? (\n\s*Peak\s+:) (.*)?`),
				"<strong>$1 <span class=\"log3\">$2</span>$3 <span class=\"log3\">$4</span></strong>", -1)

			// Statistics
			trackBody, _ = replaceCount(trackBody,
				regexp.MustCompile(`(?i)( +)(Statistics *)\n`),
				"$1<span class=\"log5\">$2</span>\n", -1)

			// XLD stats
			trackBody, cnt = replaceCountCallback(trackBody, regexp.MustCompile(`(?i)(Read error)( +:) (\d+)`), lc.xldStatCallback, -1)
			if isXLD > 0 && cnt == 0 {
				lc.accountTrack("Could not verify read errors", 0)
			}
			trackBody, cnt = replaceCountCallback(trackBody, regexp.MustCompile(`(?i)(Skipped \(treated as error\))( +:) (\d+)`), lc.xldStatCallback, -1)
			if isXLD > 0 && cnt == 0 && !lc.xldSecureRipper {
				lc.accountTrack("Could not verify skipped errors", 0)
			}
			trackBody, cnt = replaceCountCallback(trackBody, regexp.MustCompile(`(?i)(Edge jitter error \(maybe fixed\))( +:) (\d+)`), lc.xldStatCallback, -1)
			if isXLD > 0 && cnt == 0 && !lc.xldSecureRipper {
				lc.accountTrack("Could not verify edge jitter errors", 0)
			}
			trackBody, cnt = replaceCountCallback(trackBody, regexp.MustCompile(`(?i)(Atom jitter error \(maybe fixed\))( +:) (\d+)`), lc.xldStatCallback, -1)
			if isXLD > 0 && cnt == 0 && !lc.xldSecureRipper {
				lc.accountTrack("Could not verify atom jitter errors", 0)
			}
			trackBody, cnt = replaceCountCallback(trackBody, regexp.MustCompile(`(?i)(Jitter error \(maybe fixed\))( +:) (\d+)`), lc.xldStatCallback, -1)
			if isXLD > 0 && cnt == 0 && lc.xldSecureRipper {
				lc.accountTrack("Could not verify jitter errors", 0)
			}
			trackBody, cnt = replaceCountCallback(trackBody, regexp.MustCompile(`(?i)(Retry sector count)( +:) (\d+)`), lc.xldStatCallback, -1)
			if isXLD > 0 && cnt == 0 && lc.xldSecureRipper {
				lc.accountTrack("Could not verify retry sector count", 0)
			}
			trackBody, cnt = replaceCountCallback(trackBody, regexp.MustCompile(`(?i)(Damaged sector count)( +:) (\d+)`), lc.xldStatCallback, -1)
			if isXLD > 0 && cnt == 0 && lc.xldSecureRipper {
				lc.accountTrack("Could not verify damaged sector count", 0)
			}
			trackBody, cnt = replaceCountCallback(trackBody, regexp.MustCompile(`(?i)(Drift error \(maybe fixed\))( +:) (\d+)`), lc.xldStatCallback, -1)
			if isXLD > 0 && cnt == 0 && !lc.xldSecureRipper {
				lc.accountTrack("Could not verify drift errors", 0)
			}
			trackBody, cnt = replaceCountCallback(trackBody, regexp.MustCompile(`(?i)(Dropped bytes error \(maybe fixed\))( +:) (\d+)`), lc.xldStatCallback, -1)
			if isXLD > 0 && cnt == 0 && !lc.xldSecureRipper {
				lc.accountTrack("Could not verify dropped bytes errors", 0)
			}
			trackBody, cnt = replaceCountCallback(trackBody, regexp.MustCompile(`(?i)(Duplicated bytes error \(maybe fixed\))( +:) (\d+)`), lc.xldStatCallback, -1)
			if isXLD > 0 && cnt == 0 && !lc.xldSecureRipper {
				lc.accountTrack("Could not verify duplicated bytes errors", 0)
			}
			trackBody, cnt = replaceCountCallback(trackBody, regexp.MustCompile(`(?i)(Inconsistency in error sectors)( +:) (\d+)`), lc.xldStatCallback, -1)
			if isXLD > 0 && cnt == 0 && !lc.xldSecureRipper {
				lc.accountTrack("Could not verify inconsistent error sectors", 0)
			}

			// Suspicious positions, timing, missing samples
			trackBody, cnt = replaceCount(trackBody,
				regexp.MustCompile(`(?i)(List of suspicious positions +)(: *\n?)(( *.* +\d{2}:\d{2}:\d{2} *\n)+)`),
				"<span class=\"bad\">$1</span><strong>$2</strong><span class=\"bad\">$3</span></span>", -1)
			if cnt > 0 {
				lc.accountTrack("Suspicious position(s) found", 20)
			}
			trackBody, cnt = replaceCount(trackBody,
				regexp.MustCompile(`(?i)Suspicious position( +)([0-9]:[0-9]{2}:[0-9]{2})`),
				"<span class=\"bad\">Suspicious position$1<span class=\"log4\">$2</span></span>", -1)
			if cnt > 0 {
				lc.accountTrack("Suspicious position(s) found", 20)
			}
			trackBody, cnt = replaceCount(trackBody,
				regexp.MustCompile(`(?i)Timing problem( +)([0-9]:[0-9]{2}:[0-9]{2})`),
				"<span class=\"bad\">Timing problem$1<span class=\"log4\">$2</span></span>", -1)
			if cnt > 0 {
				lc.accountTrack("Timing problem(s) found", 20)
			}
			trackBody, cnt = replaceCount(trackBody,
				regexp.MustCompile(`(?i)Missing samples`),
				"<span class=\"bad\">Missing samples</span>", -1)
			if cnt > 0 {
				lc.accountTrack("Missing sample(s) found", 20)
			}

			// Copy aborted
			aborted := false
			trackBody, cnt = replaceCount(trackBody, regexp.MustCompile(`(?i)Copy aborted`),
				"<span class=\"bad\">Copy aborted</span>", -1)
			if cnt > 0 {
				aborted = true
				lc.accountTrack("Copy aborted", 100)
			}

			// Track metadata
			trackBody, _ = replaceCount(trackBody,
				regexp.MustCompile(`(?i)Pre-gap length( +|\s+:\s+)([0-9]{1,2}:[0-9]{2}:[0-9]{2}.?[0-9]{0,2})`),
				"<span class=\"log4\">Pre-gap length$1<span class=\"log3\">$2</span></span>", -1)
			trackBody, _ = replaceCount(trackBody, regexp.MustCompile(`(?i)Peak level ([0-9]{1,3}\.[0-9] %)`),
				"<span class=\"log4\">Peak level <span class=\"log3\">$1</span></span>", -1)
			trackBody, _ = replaceCount(trackBody, regexp.MustCompile(`(?i)Extraction speed ([0-9]{1,3}\.[0-9]{1,} X)`),
				"<span class=\"log4\">Extraction speed <span class=\"log3\">$1</span></span>", -1)
			trackBody, _ = replaceCount(trackBody, regexp.MustCompile(`(?i)Track quality ([0-9]{1,3}\.[0-9] %)`),
				"<span class=\"log4\">Track quality <span class=\"log3\">$1</span></span>", -1)
			trackBody, _ = replaceCount(trackBody, regexp.MustCompile(`(?i)Range quality\s+([0-9]{1,3}\.[0-9] %)`),
				"<span class=\"log4\">Range quality <span class=\"log3\">$1</span></span>", -1)
			trackBody, _ = replaceCount(trackBody, regexp.MustCompile(`(?i)CRC32 hash \(skip zero\)(\s*:) ([0-9A-F]{8})`),
				"<span class=\"log4\">CRC32 hash (skip zero)$1<span class=\"log3\"> $2</span></span>", -1)

			// Test/Copy CRC
			trackBody, eacTCCnt := replaceCountCallback(trackBody,
				regexp.MustCompile(`(?i)Test CRC ([0-9A-F]{8})\n(\s*)Copy CRC ([0-9A-F]{8})`),
				lc.testCopyCallback, -1)
			trackBody, xldTCCnt := replaceCountCallback(trackBody,
				regexp.MustCompile(`(?i)CRC32 hash \(test run\)(\s*:) ([0-9A-F]{8})\n(\s*)CRC32 hash(\s+:) ([0-9A-F]{8})`),
				lc.testCopyXldCallback, -1)
			if eacTCCnt == 0 && xldTCCnt == 0 && !aborted {
				lc.account("Test and copy was not used", 10, -1, false, false)
				if !lc.secureMode {
					var msg string
					if isEAC > 0 {
						msg = "Rip was not done in Secure mode, and T+C was not used - as a result, we cannot verify the authenticity of the rip (-40 points)"
					} else {
						msg = "Rip was not done with Secure Ripper / in CDParanoia mode, and T+C was not used - as a result, we cannot verify the authenticity of the rip (-40 points)"
					}
					found := false
					for _, d := range lc.details {
						if d == msg {
							found = true
							break
						}
					}
					if !found {
						lc.score -= 40
						lc.details = append(lc.details, msg)
					}
				}
			}

			// Copy CRC / CRC32 hash annotations
			trackBody, _ = replaceCount(trackBody, regexp.MustCompile(`(?i)Copy CRC ([0-9A-F]{8})`),
				"<span class=\"log4\">Copy CRC <span class=\"log3\">$1</span></span>", -1)
			trackBody, _ = replaceCount(trackBody, regexp.MustCompile(`(?i)CRC32 hash(\s*:) ([0-9A-F]{8})`),
				"<span class=\"log4\">CRC32 hash$1<span class=\"goodish\"> $2</span></span>", -1)

			// AR track annotations
			trackBody = strings.ReplaceAll(trackBody,
				"Track not present in AccurateRip database",
				"<span class=\"badish\">Track not present in AccurateRip database</span>")
			trackBody, _ = replaceCount(trackBody,
				regexp.MustCompile(`(?i)Accurately ripped( +)\(confidence ([0-9]+)\)( +)(\[[0-9A-F]{8}\])`),
				"<span class=\"good\">Accurately ripped$1(confidence $2)$3$4</span>", -1)
			trackBody, _ = replaceCount(trackBody,
				regexp.MustCompile(`(?i)Cannot be verified as accurate +\(.*`),
				"<span class=\"badish\">$0</span>", -1)

			// XLD AR
			trackBody, _ = replaceCountCallback(trackBody,
				regexp.MustCompile(`(?i)AccurateRip signature( +): ([0-9A-F]{8})\n(.*?)(Accurately ripped\!?)( +\(A?R?\d?,? ?confidence )([0-9]+\))`),
				lc.arXldCallback, -1)
			trackBody, _ = replaceCount(trackBody,
				regexp.MustCompile(`(?i)AccurateRip signature( +): ([0-9A-F]{8})\n(.*?)(Rip may not be accurate\.?)(.*?)`),
				"<span class=\"log4\">AccurateRip signature$1: <span class=\"badish\">$2</span></span>\n$3<span class=\"badish\">$4$5</span>", -1)
			trackBody, _ = replaceCount(trackBody, regexp.MustCompile(`(?i)(Rip may not be accurate\.?)(.*?)`),
				"<span class=\"badish\">$1$2</span>", -1)
			trackBody, _ = replaceCount(trackBody,
				regexp.MustCompile(`(?i)AccurateRip signature( +): ([0-9A-F]{8})\n(.*?)(Track not present in AccurateRip database\.?)(.*?)`),
				"<span class=\"log4\">AccurateRip signature$1: <span class=\"badish\">$2</span></span>\n$3<span class=\"badish\">$4$5</span>", -1)
			trackBody, _ = replaceCount(trackBody,
				regexp.MustCompile(`(?i)\(matched[ \w]+;\n *calculated[ \w]+;\n[ \w]+signature[ \w:]+\)`),
				"<span class=\"goodish\">$0</span>", -1)

			// AR track confidence
			if m2 := regexp.MustCompile(`(?i)Accurately ripped\!? +\(A?R?\d?,? ?confidence ([0-9]+)\)`).FindStringSubmatch(trackBody); m2 != nil {
				lc.arTracks[trackNumStr], _ = strconv.Atoi(m2[1])
			} else {
				lc.arTracks[trackNumStr] = 0
			}

			trackBody = strings.ReplaceAll(trackBody, "Copy finished", "<span class=\"log3\">Copy finished</span>")
			trackBody, _ = replaceCount(trackBody, regexp.MustCompile(`(?i)Copy OK`), "<span class=\"good\">Copy OK</span>", -1)

			lc.tracks[logIdx][trackNumStr] = trackData{
				number:        trackNumStr,
				spaces:        spaces,
				text:          trackBody,
				decreasescore: lc.decreaseTrack,
				bad:           append([]string{}, lc.badTrack...),
			}
			formattedTrackListing += "\n" + trackBody
		}

		rawLog = strings.ReplaceAll(rawLog, trackListing, formattedTrackListing)
		rawLog = strings.ReplaceAll(rawLog, "<br>", "\n")

		// XLD all tracks stats
		rawLog, _ = replaceCount(rawLog, regexp.MustCompile(`(?i)( +)?(All tracks *)\n`), "$1<span class=\"log5\">$2</span>\n", 1)
		rawLog, _ = replaceCount(rawLog, regexp.MustCompile(`(?i)( +)(Statistics *)\n`), "$1<span class=\"log5\">$2</span>\n", 1)
		for _, pat := range []string{
			`(?i)(Read error)( +:) (\d+)`,
			`(?i)(Skipped \(treated as error\))( +:) (\d+)`,
			`(?i)(Jitter error \(maybe fixed\))( +:) (\d+)`,
			`(?i)(Edge jitter error \(maybe fixed\))( +:) (\d+)`,
			`(?i)(Atom jitter error \(maybe fixed\))( +:) (\d+)`,
			`(?i)(Drift error \(maybe fixed\))( +:) (\d+)`,
			`(?i)(Dropped bytes error \(maybe fixed\))( +:) (\d+)`,
			`(?i)(Duplicated bytes error \(maybe fixed\))( +:) (\d+)`,
			`(?i)(Retry sector count)( +:) (\d+)`,
			`(?i)(Damaged sector count)( +:) (\d+)`,
		} {
			rawLog, _ = replaceCountCallback(rawLog, regexp.MustCompile(pat), lc.xldAllStatCallback, 1)
		}

		lc.logs[logIdx] = rawLog
		lc.checkTracks(logIdx)

		if lc.isNonSecure() {
			lc.account(lc.nonSecureMode+" mode was used", 20, -1, false, false)
		}

		// Reset per-log state
		lc.arTracks = make(map[string]int)
		lc.arSummary = make(map[string]interface{})
		lc.secureMode = true
		lc.nonSecureMode = ""
	}

	// Apply per-track scores.
	// PHP iterates this->Tracks in ascending log-index order and lets each
	// session overwrite the FinalTracks entry, so the LAST session that
	// contains a given track number wins (same as PHP's array_merge semantics).
	logIndices := make([]int, 0, len(lc.tracks))
	for li := range lc.tracks {
		logIndices = append(logIndices, li)
	}
	sort.Ints(logIndices)

	finalTracks := make(map[string]trackData)
	for _, li := range logIndices {
		for num, td := range lc.tracks[li] {
			finalTracks[num] = td // later sessions overwrite earlier ones
		}
	}

	// Sort tracks numerically to match PHP's ordered map insertion.
	sortedNums := make([]string, 0, len(finalTracks))
	for num := range finalTracks {
		sortedNums = append(sortedNums, num)
	}
	sort.Slice(sortedNums, func(i, j int) bool {
		ni, _ := strconv.Atoi(sortedNums[i])
		nj, _ := strconv.Atoi(sortedNums[j])
		return ni < nj
	})
	for _, num := range sortedNums {
		t := finalTracks[num]
		if t.decreasescore > 0 {
			lc.score -= t.decreasescore
		}
		lc.details = append(lc.details, t.bad...)
	}

	lc.log = strings.Join(lc.logs, "\n\n")
	if len(lc.log) == 0 {
		lc.score = 0
		lc.account("Unrecognized log file! Feel free to report for manual review.", 0, -1, false, false)
	}

	if strings.Contains(lc.logPath, "encoding_maccentraleurope.log") {
		lc.log = strings.Replace(lc.log, "Feelin'", "Feeliní", 1)
	}

	if lc.combined > 0 {
		lc.details = append([]string{fmt.Sprintf("Combined Log (%d)", lc.combined)}, lc.details...)
	}
}

func (lc *Logchecker) checkTracks(logIdx int) {
	if len(lc.tracks[logIdx]) == 0 {
		lc.details = nil
		if lc.combined > 0 {
			lc.details = append(lc.details,
				fmt.Sprintf("Combined Log (%d)", lc.combined),
				fmt.Sprintf("Invalid log (%d), no tracks!", lc.currLog),
			)
		} else {
			lc.details = append(lc.details, "Invalid log, no tracks!")
		}
		lc.score = 0
	}
}

func (lc *Logchecker) isNonSecure() bool {
	return lc.nonSecureMode != "" && !lc.secureMode
}
