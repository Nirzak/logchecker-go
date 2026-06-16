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

	// Splitting / cleaning regexes
	splitEndStatusRe = regexp.MustCompile(`(?i)(\nEnd of status report)`)
	cueToolsPluginRe = regexp.MustCompile(`(?i)---- CUETools DB Plugin V.+`)
	dashOnlyRe       = regexp.MustCompile(`(?i)^\-+$`)
	endStatusRe      = regexp.MustCompile(`(?i)End of status report`)
	xldBeginSigRe    = regexp.MustCompile(`(?i)[\-]+BEGIN XLD SIGNATURE`)
	xldSigAnnotateRe = regexp.MustCompile(`(?is)([\-]+BEGIN XLD SIGNATURE[\S\n\-]+END XLD SIGNATURE[\-]+)`)

	// Per-session regexes
	mp3OutputRe       = regexp.MustCompile(`(?i)Used output format[ ]+:[ ]+[a-z0-9 ]+MP3`)
	mp3CompressorRe   = regexp.MustCompile(`(?i)Command line compressor[ ]+:.+(MP3|lame)\.exe`)
	driveRe           = regexp.MustCompile(`(?i)Used drive( *): (.+)`)
	mediaTypeRe       = regexp.MustCompile(`(?i)Media type( *): (.+)`)
	readModeRe        = regexp.MustCompile(`(?i)Read mode( *): ([a-z]+)(.*)?`)
	ripperModeRe      = regexp.MustCompile(`(?i)Ripper mode( *): (.*)`)
	cdParanoiaModeRe  = regexp.MustCompile(`(?i)Use cdparanoia mode( *): (.*)`)
	maxRetryCountRe   = regexp.MustCompile(`(?i)Max retry count( *): (\d+)`)
	accStreamRe       = regexp.MustCompile(`(?i)Utilize accurate stream( *): (Yes|No)`)
	accStreamPre99Re  = regexp.MustCompile(`, (|NO )accurate stream`)
	defAudioCacheRe   = regexp.MustCompile(`(?i)Defeat audio cache( *): (Yes|No)`)
	defAudioPre99Re   = regexp.MustCompile(` (|NO )disable cache`)
	defAudioXldRe     = regexp.MustCompile(`(?i)Disable audio cache( *): (.*)`)
	c2PointersRe      = regexp.MustCompile(`(?i)Make use of C2 pointers( *): (Yes|No)`)
	c2PointersPre99Re = regexp.MustCompile(`with (|NO )C2`)
	readOffsetRe      = regexp.MustCompile(`(?i)Read offset correction( *): ([+-]?[0-9]+)`)
	combinedOffsetRe  = regexp.MustCompile(`(?i)(Combined read/write offset correction\s*:\s+\d+)`)
	xldAltOffsetRe1   = regexp.MustCompile(`(?i)(List of \w+ offset correction values) *(\n+)(( *.*confidence .*\) ?\n)+)`)
	xldAltOffsetRe2   = regexp.MustCompile(`(?i)(List of \w+ offset correction values) *\n( *# +\| +Absolute +\| +Relative +\| +Confidence) *\n( *\-+) *\n(( *\d+ +\| +\-?\+?\d+ +\| +\-?\+?\d+ +\| +\d+ *\n)+)`)
	overreadRe        = regexp.MustCompile(`(?i)Overread into Lead-In and Lead-Out( +): (Yes|No)`)
	fillOffsetRe      = regexp.MustCompile(`(?i)Fill up missing offset samples with silence( +): (Yes|No)`)
	deleteSilentRe    = regexp.MustCompile(`(?i)Delete leading and trailing silent blocks([ \w]*)( +): (Yes|No)`)
	nullSamplesRe     = regexp.MustCompile(`(?i)Null samples used in CRC calculations( +): (Yes|No)`)
	normalizeRe       = regexp.MustCompile(`(?i)Normalize to( +): ([0-9% ]+)`)
	usedInterfaceRe   = regexp.MustCompile(`(?i)Used interface( +): ([^\n]+)`)
	gapHandlingRe     = regexp.MustCompile(`(?i)Gap handling( +): ([^\n]+)`)
	gapStatusRe       = regexp.MustCompile(`(?i)Gap status( +): (.*)`)
	outputFormatRe    = regexp.MustCompile(`(?i)Used output format( *): ([^\n]+)`)
	sampleFormatRe    = regexp.MustCompile(`(?i)Sample format( +): ([^\n]+)`)
	selBitrateRe      = regexp.MustCompile(`(?i)Selected bitrate( +): ([^\n]+)`)
	kbitRe            = regexp.MustCompile(`(?i)( +)(\d+ kBit/s)`)
	qualityRe         = regexp.MustCompile(`(?i)Quality( +): ([^\n]+)`)
	addId3Re          = regexp.MustCompile(`(?i)Add ID3 tag( +): (Yes|No)`)
	compressOffsetRe  = regexp.MustCompile(`(?i)(Use compression offset\s+:\s+\d+)`)
	cmdCompressorRe   = regexp.MustCompile(`(?i)Command line compressor( +): ([^\n]+)`)
	addlCmdLineWrapRe = regexp.MustCompile(`Additional command line options([^\n]{70,110} )`)
	addlCmdLineRe     = regexp.MustCompile(`(?i)( *)Additional command line options( +): (.+)\n`)
	xldRangeRe        = regexp.MustCompile(`(?i)\n(All Tracks\n(?: *))(Filename)`)
	xldAlbumGainRe    = regexp.MustCompile(`(?i)All Tracks(\s*\n)((?:.*)\n)?(\s*Album gain\s+:) (.*)?(\n\s*Peak\s+:) (.*)?`)
	otherOptionsRe    = regexp.MustCompile(`(?i)Other options( +):`)
	win32InterfaceRe  = regexp.MustCompile(`(?i)\n( *)Native Win32 interface(.+)`)
	tocHeaderRe       = regexp.MustCompile(`(?i)( +)Track( +)\|( +)Start( +)\|( +)Length( +)\|( +)Start sector( +)\|( +)End sector( ?)`)
	tocDashRe         = regexp.MustCompile(`-{10,100}`)
	tocRowRe          = regexp.MustCompile(`(?i)( +)([0-9]{1,3})( +)\|( +)(([0-9]{1,3}:)?[0-9]{2}[.:][0-9]{2})( +)\|( +)(([0-9]{1,3}:)?[0-9]{2}[.:][0-9]{2})( +)\|( +)([0-9]{1,10})( +)\|( +)([0-9]{1,10})( +)\n`)
	noErrorsRe        = regexp.MustCompile(`(?i)No errors occurr?ed`)
	thereWereErrorsRe = regexp.MustCompile(`(?i)(There were errors) ?\n`)
	someInconsistRe   = regexp.MustCompile(`(?i)(Some inconsistencies found) ?\n`)
	endStatusAnnotRe  = regexp.MustCompile(`(?i)End of status report`)
	rippingStatusRe   = regexp.MustCompile(`(?i)Track(\s*)Ripping Status(\s*)\[Disc ID: ([0-9a-f]{8}-[0-9a-f]{8})\]`)
	allAccRe          = regexp.MustCompile(`(?i)(All Tracks Accurately Ripped\.?)`)
	nTracksAccRe      = regexp.MustCompile(`(?i)\d+ track.* +accurately ripped\.? *\n`)
	nTracksNotInDBRe  = regexp.MustCompile(`(?i)\d+ track.* +not present in the AccurateRip database\.? *\n`)
	nTracksCanceledRe = regexp.MustCompile(`(?i)\d+ track.* +canceled\.? *\n`)
	nTracksUnverRe    = regexp.MustCompile(`(?i)\d+ track.* +could not be verified as accurate\.? *\n`)
	someUnverRe       = regexp.MustCompile(`(?i)Some tracks could not be verified as accurate\.? *\n`)
	noTracksVerRe     = regexp.MustCompile(`(?i)No tracks could be verified as accurate\.? *\n`)
	diffPressingRe    = regexp.MustCompile(`(?i)You may have a different pressing.*\n`)
	xldSumOKRe        = regexp.MustCompile(`(?i)(Track +\d+ +: +)(OK +)\(A?R?\d?,? ?confidence +(\d+).*?\)(.*)\n`)
	xldSumNGRe        = regexp.MustCompile(`(?i)(Track +\d+ +: +)(NG|Not Found).*?\n`)
	statusLineRe      = regexp.MustCompile(`(?i)( *.{2} ?)(\d+ track\(s\).*)\n`)
	arSummaryHdrRe    = regexp.MustCompile(`(?i)\n( *AccurateRip summary\.?)`)
	arSummaryConfRe   = regexp.MustCompile(`(?i)(Track +\d+ +.*?accurately ripped\.? *)(\(confidence +)(\d+)\)(.*)\n`)
	arNotInDBRe       = regexp.MustCompile(`(?i)(Track +\d+ +.*?in database *)\n`)
	arCannotVerRe     = regexp.MustCompile(`(?i)(Track +\d+ +.*?(could not|cannot) be verified as accurate.*)\n`)
	selRangeRe        = regexp.MustCompile(`(?i)\n( *Selected range)`)
	rangeStatusRe     = regexp.MustCompile(`(?i)\n( *Range status and errors)`)
	xldAllStatHdrRe   = regexp.MustCompile(`(?i)( +)?(All tracks *)\n`)
	xldStatsHdrRe     = regexp.MustCompile(`(?i)( +)(Statistics *)\n`)

	// Track-body regexes
	trackHeaderRe   = regexp.MustCompile(`(?i)\nTrack( +)([0-9]{1,3})([\s\S]+)`)
	trackSplitRe    = regexp.MustCompile(`(?i)\nTrack( +)([0-9]{1,3})`)
	rangeHeaderRe   = regexp.MustCompile(`(?i)\n( *)Filename +(.*)([\s\S]+)`)
	rangeSplitRe    = regexp.MustCompile(`(?i)\n( *)Filename +(.*)`)
	trackEndRe      = regexp.MustCompile(`(?i)[ \t]+ [a-z]`)
	filenameRe      = regexp.MustCompile(`(?is)Filename ((.+)?\.(wav|flac|ape))\n`)
	filenameLongRe  = regexp.MustCompile(`(?i)Filename (.+)\n`)
	filenameAnyRe   = regexp.MustCompile(`(?i)Filename ((.+)?)\n`)
	fileWriteErrRe  = regexp.MustCompile(`(?i)( *)(File write error)\n`)
	trackGainRe     = regexp.MustCompile(`(?i)( *Track gain\s+:) (.*)?(\n\s*Peak\s+:) (.*)?`)
	statisticsRe    = regexp.MustCompile(`(?i)( +)(Statistics *)\n`)
	xldReadErrRe    = regexp.MustCompile(`(?i)(Read error)( +:) (\d+)`)
	xldSkippedRe    = regexp.MustCompile(`(?i)(Skipped \(treated as error\))( +:) (\d+)`)
	xldEdgeJitterRe = regexp.MustCompile(`(?i)(Edge jitter error \(maybe fixed\))( +:) (\d+)`)
	xldAtomJitterRe = regexp.MustCompile(`(?i)(Atom jitter error \(maybe fixed\))( +:) (\d+)`)
	xldJitterRe     = regexp.MustCompile(`(?i)(Jitter error \(maybe fixed\))( +:) (\d+)`)
	xldRetryRe      = regexp.MustCompile(`(?i)(Retry sector count)( +:) (\d+)`)
	xldDamagedRe    = regexp.MustCompile(`(?i)(Damaged sector count)( +:) (\d+)`)
	xldDriftRe      = regexp.MustCompile(`(?i)(Drift error \(maybe fixed\))( +:) (\d+)`)
	xldDroppedRe    = regexp.MustCompile(`(?i)(Dropped bytes error \(maybe fixed\))( +:) (\d+)`)
	xldDuplicatedRe = regexp.MustCompile(`(?i)(Duplicated bytes error \(maybe fixed\))( +:) (\d+)`)
	xldInconsistRe  = regexp.MustCompile(`(?i)(Inconsistency in error sectors)( +:) (\d+)`)
	suspListRe      = regexp.MustCompile(`(?i)(List of suspicious positions +)(: *\n?)(( *.* +\d{2}:\d{2}:\d{2} *\n)+)`)
	suspPosRe       = regexp.MustCompile(`(?i)Suspicious position( +)([0-9]:[0-9]{2}:[0-9]{2})`)
	timingProbRe    = regexp.MustCompile(`(?i)Timing problem( +)([0-9]:[0-9]{2}:[0-9]{2})`)
	missingSampRe   = regexp.MustCompile(`(?i)Missing samples`)
	copyAbortedRe   = regexp.MustCompile(`(?i)Copy aborted`)
	preGapRe        = regexp.MustCompile(`(?i)Pre-gap length( +|\s+:\s+)([0-9]{1,2}:[0-9]{2}:[0-9]{2}.?[0-9]{0,2})`)
	peakLevelRe     = regexp.MustCompile(`(?i)Peak level ([0-9]{1,3}\.[0-9] %)`)
	extrSpeedRe     = regexp.MustCompile(`(?i)Extraction speed ([0-9]{1,3}\.[0-9]{1,} X)`)
	trackQualRe     = regexp.MustCompile(`(?i)Track quality ([0-9]{1,3}\.[0-9] %)`)
	rangeQualRe     = regexp.MustCompile(`(?i)Range quality\s+([0-9]{1,3}\.[0-9] %)`)
	crc32SkipRe     = regexp.MustCompile(`(?i)CRC32 hash \(skip zero\)(\s*:) ([0-9A-F]{8})`)
	testCopyRe      = regexp.MustCompile(`(?i)Test CRC ([0-9A-F]{8})\n(\s*)Copy CRC ([0-9A-F]{8})`)
	testCopyXldRe   = regexp.MustCompile(`(?i)CRC32 hash \(test run\)(\s*:) ([0-9A-F]{8})\n(\s*)CRC32 hash(\s+:) ([0-9A-F]{8})`)
	copyCRCRe       = regexp.MustCompile(`(?i)Copy CRC ([0-9A-F]{8})`)
	crc32HashRe     = regexp.MustCompile(`(?i)CRC32 hash(\s*:) ([0-9A-F]{8})`)
	arAccRippedRe   = regexp.MustCompile(`(?i)Accurately ripped( +)\(confidence ([0-9]+)\)( +)(\[[0-9A-F]{8}\])`)
	arCannotRe      = regexp.MustCompile(`(?i)Cannot be verified as accurate +\(.*`)
	arXldRe         = regexp.MustCompile(`(?i)AccurateRip signature( +): ([0-9A-F]{8})\n(.*?)(Accurately ripped\!?)( +\(A?R?\d?,? ?confidence )([0-9]+\))`)
	arXldInaccRe    = regexp.MustCompile(`(?i)AccurateRip signature( +): ([0-9A-F]{8})\n(.*?)(Rip may not be accurate\.?)(.*?)`)
	ripInaccRe      = regexp.MustCompile(`(?i)(Rip may not be accurate\.?)(.*?)`)
	arXldNotInDBRe  = regexp.MustCompile(`(?i)AccurateRip signature( +): ([0-9A-F]{8})\n(.*?)(Track not present in AccurateRip database\.?)(.*?)`)
	arXldMatchedRe  = regexp.MustCompile(`(?i)\(matched[ \w]+;\n *calculated[ \w]+;\n[ \w]+signature[ \w:]+\)`)
	arConfRe        = regexp.MustCompile(`(?i)Accurately ripped\!? +\(A?R?\d?,? ?confidence ([0-9]+)\)`)
	copyOKRe        = regexp.MustCompile(`(?i)Copy OK`)
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
		lc.logs = splitWithDelim(lc.log, splitEndStatusRe)
		// Remove CUETools DB Plugin entries
		var filtered []string
		for _, l := range lc.logs {
			if !cueToolsPluginRe.MatchString(l) {
				filtered = append(filtered, l)
			}
		}
		lc.logs = filtered
	}

	// Clean up log segments
	var cleaned []string
	for i, rawPiece := range lc.logs {
		l := strings.TrimSpace(rawPiece)
		if l == "" || dashOnlyRe.MatchString(l) {
			continue
		}
		if lc.checksumStatus != check.ChecksumOK && endStatusRe.MatchString(l) {
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
		if lc.checksumStatus == check.ChecksumOK && xldBeginSigRe.MatchString(l) {
			if len(cleaned) > 0 {
				cleaned[len(cleaned)-1] += "\n" + l
			}
			continue
		}
		cleaned = append(cleaned, l)
	}
	lc.logs = cleaned

	if len(lc.logs) > 1 {
		lc.combined = len(lc.logs)
	}

	for logIdx, rawLog := range lc.logs {
		lc.legacyParseSession(logIdx, rawLog)
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
	sortNumericStrings(sortedNums)
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
		lc.accountDeduction("Unrecognized log file! Feel free to report for manual review.", 0)
	}

	if lc.combined > 0 {
		lc.details = append([]string{fmt.Sprintf("Combined Log (%d)", lc.combined)}, lc.details...)
	}
}

// legacyParseSession parses a single rip session (one segment of a combined
// log). It annotates rawLog with HTML spans, records per-track scoring into lc,
// and stores the annotated result back into lc.logs[logIdx]. Called once per
// segment by legacyParse.
func (lc *Logchecker) legacyParseSession(logIdx int, rawLog string) {
	lc.tracks[logIdx] = make(map[string]trackData)
	lc.currLog = logIdx + 1

	checksumRe := eacChecksumRe
	if lc.language != "en" {
		checksumRe = eacChecksumFRe
	}

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
	rawLog, xldCount := replaceCount(rawLog, xldSigAnnotateRe,
		"<span class='"+csClass+"'>$1</span>", 1)

	if (eacCount > 0 || xldCount > 0) && lc.checksumStatus == check.ChecksumMissing {
		lc.checksumStatus = check.ChecksumInvalid
	}

	// MP3 check for EAC
	if isEAC > 0 {
		if mp3OutputRe.MatchString(rawLog) || mp3CompressorRe.MatchString(rawLog) {
			if lc.combined > 0 {
				lc.details = append(lc.details, fmt.Sprintf("Skipping Log (%d), MP3 Rip", lc.currLog))
			} else {
				lc.account("Invalid Log (MP3)", 100, -1, false, false)
			}
			lc.logs[logIdx] = rawLog
			return
		}
	}

	// --- drive ---
	rawLog, cnt := replaceCountCallback(rawLog, driveRe, lc.driveCallback, 1)
	if cnt == 0 {
		lc.account("Could not verify used drive", 1, -1, false, false)
	}

	// --- media type (XLD) ---
	rawLog, cnt = replaceCountCallback(rawLog, mediaTypeRe, lc.mediaTypeXldCallback, 1)
	if isXLD > 0 && lc.ripperVersion != "" {
		ripVer, _ := strconv.Atoi(lc.ripperVersion)
		if ripVer >= 20130127 && cnt == 0 {
			lc.account("Could not verify media type", 1, -1, false, false)
		}
	}

	// --- read mode ---
	rawLog, cnt = replaceCountCallback(rawLog, readModeRe, lc.readModeCallback, 1)
	if isEAC > 0 && cnt == 0 {
		lc.account("Could not verify read mode", 1, -1, false, false)
	}

	// --- XLD ripper/cdparanoia mode ---
	rawLog, ripperModeCnt := replaceCountCallback(rawLog, ripperModeRe, lc.ripperModeXldCallback, 1)
	rawLog, cdParanoiaCnt := replaceCountCallback(rawLog, cdParanoiaModeRe, lc.cdparanoiaModeXldCallback, 1)
	if isXLD > 0 && ripperModeCnt == 0 && cdParanoiaCnt == 0 {
		lc.account("Could not verify read mode", 1, -1, false, false)
	}

	// --- max retry count (XLD) ---
	rawLog, cnt = replaceCountCallback(rawLog, maxRetryCountRe, lc.maxRetryCountCallback, 1)
	if isXLD > 0 && cnt == 0 {
		lc.account("Could not verify max retry count", 0, -1, false, false) // notice only historically
	}

	// --- accurate stream ---
	rawLog, acsCnt := replaceCountCallback(rawLog, accStreamRe, lc.accurateStreamCallback, 1)
	rawLog, acsPre99Cnt := replaceCountCallback(rawLog, accStreamPre99Re, lc.accurateStreamEacPre9Callback, 1)
	if isEAC > 0 && acsCnt == 0 && acsPre99Cnt == 0 && !lc.isNonSecure() {
		lc.account("Could not verify accurate stream", 20, -1, false, false)
	}

	// --- defeat audio cache ---
	rawLog, defCnt := replaceCountCallback(rawLog, defAudioCacheRe, lc.defeatAudioCacheCallback, 1)
	rawLog, defPre99Cnt := replaceCountCallback(rawLog, defAudioPre99Re, lc.defeatAudioCacheEacPre99Callback, 1)
	rawLog, cnt = replaceCountCallback(rawLog, defAudioXldRe, lc.defeatAudioCacheXldCallback, 1)
	if isEAC > 0 && defCnt == 0 && defPre99Cnt == 0 && !lc.isNonSecure() {
		lc.account("Could not verify defeat audio cache", 1, -1, false, false)
	}
	if isXLD > 0 && cnt == 0 {
		lc.account("Could not verify defeat audio cache", 1, -1, false, false)
	}

	// --- C2 pointers ---
	rawLog, c2Cnt := replaceCountCallback(rawLog, c2PointersRe, lc.c2PointersCallback, 1)
	rawLog, c2Pre99Cnt := replaceCountCallback(rawLog, c2PointersPre99Re, lc.c2PointersEacPre99Callback, 1)
	if c2Cnt == 0 && c2Pre99Cnt == 0 && !lc.isNonSecure() {
		lc.account("Could not verify C2 pointers", 1, -1, false, false)
	}

	// --- read offset ---
	rawLog, cnt = replaceCountCallback(rawLog, readOffsetRe, lc.readOffsetCallback, 1)
	if cnt == 0 {
		lc.account("Could not verify read offset", 1, -1, false, false)
	}

	// combined read/write offset
	rawLog, cnt = replaceCount(rawLog, combinedOffsetRe,
		`<span class="bad">$1</span>`, 1)
	if cnt > 0 {
		lc.account("Combined read/write offset cannot be verified", 4, -1, false, false)
	}

	// XLD alternate offset table
	rawLog, _ = replaceCount(rawLog, xldAltOffsetRe1,
		`<span class="log5">$1</span>$2<span class="log4">$3</span>`+"\n", 1)
	rawLog, _ = replaceCount(rawLog, xldAltOffsetRe2,
		`<span class="log5">$1</span>`+"\n"+`<span class="log4">$2`+"\n"+"$3\n$4\n</span>", 1)

	// overread
	rawLog, _ = replaceCount(rawLog, overreadRe,
		`<span class="log5">Overread into Lead-In and Lead-Out$1</span>: <span class="log4">$2</span>`, 1)

	// fill offset samples
	rawLog, cnt = replaceCountCallback(rawLog, fillOffsetRe, lc.fillOffsetSamplesCallback, 1)
	if isEAC > 0 && cnt == 0 {
		lc.account("Could not verify missing offset samples", 1, -1, false, false)
	}

	// delete silent blocks
	rawLog, cnt = replaceCountCallback(rawLog, deleteSilentRe, lc.deleteSilentBlocksCallback, 1)
	if isEAC > 0 && cnt == 0 {
		lc.account("Could not verify silent blocks", 1, -1, false, false)
	}

	// null samples
	rawLog, cnt = replaceCountCallback(rawLog, nullSamplesRe, lc.nullSamplesCallback, 1)
	if isEAC > 0 && cnt == 0 {
		lc.accountDeduction("Could not verify null samples", 0)
	}

	// normalize
	rawLog, _ = replaceCountCallback(rawLog, normalizeRe, lc.normalizeEacCallback, 1)

	// interface, output format, etc.
	rawLog, _ = replaceCount(rawLog, usedInterfaceRe,
		`<span class="log5">Used interface$1</span>: <span class="log4">$2</span>`, 1)

	// gap handling
	rawLog, cnt = replaceCountCallback(rawLog, gapHandlingRe, lc.gapHandlingCallback, 1)
	if isEAC > 0 && cnt == 0 {
		lc.accountDeduction("Could not verify gap handling", 10)
	}
	rawLog, cnt = replaceCountCallback(rawLog, gapStatusRe, lc.gapHandlingXldCallback, 1)
	if isXLD > 0 && cnt == 0 {
		lc.accountDeduction("Could not verify gap status", 10)
	}

	// output format, sample format, selected bitrate, quality
	for _, pat := range []struct {
		re   *regexp.Regexp
		repl string
	}{
		{outputFormatRe, `<span class="log5">Used output format$1</span>: <span class="log4">$2</span>`},
		{sampleFormatRe, `<span class="log5">Sample format$1</span>: <span class="log4">$2</span>`},
		{selBitrateRe, `<span class="log5">Selected bitrate$1</span>: <span class="log4">$2</span>`},
		{kbitRe, `$1<span class="log4">$2</span>`},
		{qualityRe, `<span class="log5">Quality$1</span>: <span class="log4">$2</span>`},
	} {
		rawLog, _ = replaceCount(rawLog, pat.re, pat.repl, 1)
	}

	// ID3 tag
	rawLog, cnt = replaceCountCallback(rawLog, addId3Re, lc.addId3TagCallback, 1)
	if isEAC > 0 && cnt == 0 {
		lc.accountDeduction("Could not verify id3 tag setting", 0)
	}

	// compression offset
	rawLog, cnt = replaceCount(rawLog, compressOffsetRe,
		`<span class="bad">$1</span>`, 1)
	if cnt > 0 {
		lc.accountFatal("Ripped with compression offset", 0)
	}

	// command line compressor, additional options
	rawLog, _ = replaceCount(rawLog, cmdCompressorRe,
		`<span class="log5">Command line compressor$1</span>: <span class="log4">$2</span>`, 1)
	rawLog = addlCmdLineWrapRe.ReplaceAllString(rawLog, "Additional command line options$1<br>")
	rawLog, _ = replaceCount(rawLog, addlCmdLineRe,
		`<span class="log5">Additional command line options$2</span>: <span class="log4">$3</span>`+"\n", 1)

	// XLD range rip detection
	rawLog, xldRangeCnt := replaceCount(rawLog, xldRangeRe,
		"\n$1<span class=\"bad\">$2</span>", 1)

	// XLD album gain
	rawLog, cnt = replaceCount(rawLog, xldAlbumGainRe,
		`<span class="log5">All Tracks</span>$1$2<strong>$3 <span class="log3">$4</span>`+"$5 <span class=\"log3\">$6</span></strong>", 1)
	if isXLD > 0 && cnt == 0 {
		lc.accountDeduction("Could not verify album gain", 0)
	}

	// pre-0.99 other options
	rawLog, _ = replaceCount(rawLog, otherOptionsRe,
		`<span class="log5">Other options$1</span>:`, 1)
	rawLog, _ = replaceCount(rawLog, win32InterfaceRe,
		"\n$1<span class=\"log4\">Native Win32 interface$2</span>", 1)

	// TOC
	rawLog = strings.ReplaceAll(rawLog, "TOC of the extracted CD",
		`<span class="log4 log5">TOC of the extracted CD</span>`)
	rawLog, _ = replaceCount(rawLog, tocHeaderRe,
		"<strong>$0</strong>", 1)
	rawLog = tocDashRe.ReplaceAllString(rawLog, "<strong>$0</strong>")
	rawLog = tocRowRe.
		ReplaceAllStringFunc(rawLog, func(s string) string {
			m := tocRowRe.FindStringSubmatch(s)
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
	rawLog = noErrorsRe.ReplaceAllString(rawLog, `<span class="good">No errors occurred</span>`)
	rawLog = thereWereErrorsRe.ReplaceAllString(rawLog, `<span class="bad">$1</span>`+"\n")
	rawLog = someInconsistRe.ReplaceAllString(rawLog, `<span class="badish">$1</span>`+"\n")
	rawLog = endStatusAnnotRe.ReplaceAllString(rawLog, `<span class="good">End of status report</span>`)
	rawLog = rippingStatusRe.
		ReplaceAllString(rawLog, "<strong>Track</strong>$1<strong>Ripping Status</strong>$2<strong>Disc ID: </strong><span class=\"log1\">$3</span>")
	rawLog = allAccRe.ReplaceAllString(rawLog, `<span class="good">$1</span>`)
	rawLog = nTracksAccRe.ReplaceAllString(rawLog, `<span class="good">$0</span>`)
	rawLog = nTracksNotInDBRe.ReplaceAllString(rawLog, `<span class="badish">$0</span>`)
	rawLog = nTracksCanceledRe.ReplaceAllString(rawLog, `<span class="bad">$0</span>`)
	rawLog = nTracksUnverRe.ReplaceAllString(rawLog, `<span class="badish">$0</span>`)
	rawLog = someUnverRe.ReplaceAllString(rawLog, `<span class="badish">$0</span>`)
	rawLog = noTracksVerRe.ReplaceAllString(rawLog, `<span class="badish">$0</span>`)
	rawLog = diffPressingRe.ReplaceAllString(rawLog, `<span class="goodish">$0</span>`)

	// XLD accurip summary
	rawLog = xldSumOKRe.
		ReplaceAllStringFunc(rawLog, lc.arSummaryConfXldCallback)
	rawLog = xldSumNGRe.
		ReplaceAllStringFunc(rawLog, lc.arSummaryConfXldCallback)

	// Status line
	rawLog, _ = replaceCount(rawLog, statusLineRe,
		`$1<span class="log4">$2</span>`+"\n", 1)

	// AccurateRip summary (range)
	rawLog = arSummaryHdrRe.
		ReplaceAllString(rawLog, "\n<span class=\"log4 log5\">$1</span>")
	rawLog = arSummaryConfRe.
		ReplaceAllStringFunc(rawLog, lc.arSummaryConfCallback)
	rawLog, cnt = replaceCount(rawLog, arNotInDBRe,
		`<span class="badish">$1</span>`+"\n", -1)
	if cnt > 0 {
		lc.arSummary["bad"] = cnt
	}
	rawLog, cnt = replaceCount(rawLog, arCannotVerRe,
		`<span class="badish">$1</span>`+"\n", -1)
	if cnt > 0 {
		lc.arSummary["bad"] = cnt
	}

	// Range rip detection
	rawLog, range1Cnt := replaceCount(rawLog, selRangeRe,
		"\n<span class=\"bad\">$1</span>", 1)
	rawLog, range2Cnt := replaceCount(rawLog, rangeStatusRe,
		"\n<span class=\"bad\">$1</span>", 1)
	if range1Cnt > 0 || range2Cnt > 0 || xldRangeCnt > 0 {
		lc.rangeRip = true
		lc.accountDeduction("Range rip detected", 30)
	}

	rawLog = lc.legacyParseTracks(logIdx, rawLog, isEAC, isXLD)

	lc.logs[logIdx] = rawLog
	lc.checkTracks(logIdx)

	if lc.nonSecureMode != "" {
		lc.accountDeduction(lc.nonSecureMode+" mode was used", 20)
	}

	// Reset per-log state
	lc.arTracks = make(map[string]int)
	lc.arSummary = make(map[string]interface{})
	lc.secureMode = true
	lc.nonSecureMode = ""
}

// legacyParseTracks parses the track-listing region of one rip session: it
// splits the listing into per-track bodies, annotates each (filename, CRC, AR
// status, copy result), records per-track scoring into lc.tracks[logIdx], and
// returns rawLog with the formatted track listing and XLD all-track stats
// substituted in. Called by legacyParseSession.
func (lc *Logchecker) legacyParseTracks(logIdx int, rawLog string, isEAC, isXLD int) string {
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
		m := trackHeaderRe.FindStringSubmatch(rawLog)
		if len(m) > 0 {
			trackListing = m[0]
			exploded := strings.Split(trackListing, "\n")
			i := findTrackBoundary(exploded, logEnds)
			trackListing = strings.Join(exploded[:i+1], "\n")
			fullTracks = splitWithCaptures(trackListing, trackSplitRe)
			trackBodies = trackSplitRe.Split(trackListing, -1)
			if len(trackBodies) > 0 {
				trackBodies = trackBodies[1:]
			}
		}
	} else {
		// Range rip
		m := rangeHeaderRe.FindStringSubmatch(rawLog)
		if len(m) > 0 {
			trackListing = m[0]
			// Use original m[0] (full match) for split — not trimmed exploded.
			fullTracks = splitWithCaptures(trackListing, rangeSplitRe)
			trackBodies = rangeSplitRe.Split(trackListing, -1)
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
		trackBody, cnt := replaceCount(trackBody,
			filenameRe,
			"<span class=\"log4\">Filename <span class=\"log3\">$1</span></span>\n", -1)
		if cnt == 0 && !lc.rangeRip {
			if isEAC > 0 {
				if m2 := filenameLongRe.FindStringSubmatch(trackBody); m2 != nil && len(m2[1]) >= 243 {
					trackBody, _ = replaceCount(trackBody,
						filenameAnyRe,
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
			fileWriteErrRe,
			"$1<span class=\"bad\">$2</span>\n", -1)
		if cnt > 0 {
			lc.accountTrack("File write error", 20)
		}

		// XLD track gain
		trackBody, _ = replaceCount(trackBody,
			trackGainRe,
			"<strong>$1 <span class=\"log3\">$2</span>$3 <span class=\"log3\">$4</span></strong>", -1)

		// Statistics
		trackBody, _ = replaceCount(trackBody,
			statisticsRe,
			"$1<span class=\"log5\">$2</span>\n", -1)

		// XLD stats
		trackBody, cnt = replaceCountCallback(trackBody, xldReadErrRe, lc.xldStatCallback, -1)
		if isXLD > 0 && cnt == 0 {
			lc.accountTrack("Could not verify read errors", 0)
		}
		trackBody, cnt = replaceCountCallback(trackBody, xldSkippedRe, lc.xldStatCallback, -1)
		if isXLD > 0 && cnt == 0 && !lc.xldSecureRipper {
			lc.accountTrack("Could not verify skipped errors", 0)
		}
		trackBody, cnt = replaceCountCallback(trackBody, xldEdgeJitterRe, lc.xldStatCallback, -1)
		if isXLD > 0 && cnt == 0 && !lc.xldSecureRipper {
			lc.accountTrack("Could not verify edge jitter errors", 0)
		}
		trackBody, cnt = replaceCountCallback(trackBody, xldAtomJitterRe, lc.xldStatCallback, -1)
		if isXLD > 0 && cnt == 0 && !lc.xldSecureRipper {
			lc.accountTrack("Could not verify atom jitter errors", 0)
		}
		trackBody, cnt = replaceCountCallback(trackBody, xldJitterRe, lc.xldStatCallback, -1)
		if isXLD > 0 && cnt == 0 && lc.xldSecureRipper {
			lc.accountTrack("Could not verify jitter errors", 0)
		}
		trackBody, cnt = replaceCountCallback(trackBody, xldRetryRe, lc.xldStatCallback, -1)
		if isXLD > 0 && cnt == 0 && lc.xldSecureRipper {
			lc.accountTrack("Could not verify retry sector count", 0)
		}
		trackBody, cnt = replaceCountCallback(trackBody, xldDamagedRe, lc.xldStatCallback, -1)
		if isXLD > 0 && cnt == 0 && lc.xldSecureRipper {
			lc.accountTrack("Could not verify damaged sector count", 0)
		}
		trackBody, cnt = replaceCountCallback(trackBody, xldDriftRe, lc.xldStatCallback, -1)
		if isXLD > 0 && cnt == 0 && !lc.xldSecureRipper {
			lc.accountTrack("Could not verify drift errors", 0)
		}
		trackBody, cnt = replaceCountCallback(trackBody, xldDroppedRe, lc.xldStatCallback, -1)
		if isXLD > 0 && cnt == 0 && !lc.xldSecureRipper {
			lc.accountTrack("Could not verify dropped bytes errors", 0)
		}
		trackBody, cnt = replaceCountCallback(trackBody, xldDuplicatedRe, lc.xldStatCallback, -1)
		if isXLD > 0 && cnt == 0 && !lc.xldSecureRipper {
			lc.accountTrack("Could not verify duplicated bytes errors", 0)
		}
		trackBody, cnt = replaceCountCallback(trackBody, xldInconsistRe, lc.xldStatCallback, -1)
		if isXLD > 0 && cnt == 0 && !lc.xldSecureRipper {
			lc.accountTrack("Could not verify inconsistent error sectors", 0)
		}

		// Suspicious positions, timing, missing samples
		trackBody, cnt = replaceCount(trackBody,
			suspListRe,
			"<span class=\"bad\">$1</span><strong>$2</strong><span class=\"bad\">$3</span></span>", -1)
		if cnt > 0 {
			lc.accountTrack("Suspicious position(s) found", 20)
		}
		trackBody, cnt = replaceCount(trackBody,
			suspPosRe,
			"<span class=\"bad\">Suspicious position$1<span class=\"log4\">$2</span></span>", -1)
		if cnt > 0 {
			lc.accountTrack("Suspicious position(s) found", 20)
		}
		trackBody, cnt = replaceCount(trackBody,
			timingProbRe,
			"<span class=\"bad\">Timing problem$1<span class=\"log4\">$2</span></span>", -1)
		if cnt > 0 {
			lc.accountTrack("Timing problem(s) found", 20)
		}
		trackBody, cnt = replaceCount(trackBody,
			missingSampRe,
			"<span class=\"bad\">Missing samples</span>", -1)
		if cnt > 0 {
			lc.accountTrack("Missing sample(s) found", 20)
		}

		// Copy aborted
		aborted := false
		trackBody, cnt = replaceCount(trackBody, copyAbortedRe,
			"<span class=\"bad\">Copy aborted</span>", -1)
		if cnt > 0 {
			aborted = true
			lc.accountTrack("Copy aborted", 100)
		}

		// Track metadata
		trackBody, _ = replaceCount(trackBody,
			preGapRe,
			"<span class=\"log4\">Pre-gap length$1<span class=\"log3\">$2</span></span>", -1)
		trackBody, _ = replaceCount(trackBody, peakLevelRe,
			"<span class=\"log4\">Peak level <span class=\"log3\">$1</span></span>", -1)
		trackBody, _ = replaceCount(trackBody, extrSpeedRe,
			"<span class=\"log4\">Extraction speed <span class=\"log3\">$1</span></span>", -1)
		trackBody, _ = replaceCount(trackBody, trackQualRe,
			"<span class=\"log4\">Track quality <span class=\"log3\">$1</span></span>", -1)
		trackBody, _ = replaceCount(trackBody, rangeQualRe,
			"<span class=\"log4\">Range quality <span class=\"log3\">$1</span></span>", -1)
		trackBody, _ = replaceCount(trackBody, crc32SkipRe,
			"<span class=\"log4\">CRC32 hash (skip zero)$1<span class=\"log3\"> $2</span></span>", -1)

		// Test/Copy CRC
		trackBody, eacTCCnt := replaceCountCallback(trackBody,
			testCopyRe,
			lc.testCopyCallback, -1)
		trackBody, xldTCCnt := replaceCountCallback(trackBody,
			testCopyXldRe,
			lc.testCopyXldCallback, -1)
		if eacTCCnt == 0 && xldTCCnt == 0 && !aborted {
			lc.accountDeduction("Test and copy was not used", 10)
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
		trackBody, _ = replaceCount(trackBody, copyCRCRe,
			"<span class=\"log4\">Copy CRC <span class=\"log3\">$1</span></span>", -1)
		trackBody, _ = replaceCount(trackBody, crc32HashRe,
			"<span class=\"log4\">CRC32 hash$1<span class=\"goodish\"> $2</span></span>", -1)

		// AR track annotations
		trackBody = strings.ReplaceAll(trackBody,
			"Track not present in AccurateRip database",
			"<span class=\"badish\">Track not present in AccurateRip database</span>")
		trackBody, _ = replaceCount(trackBody,
			arAccRippedRe,
			"<span class=\"good\">Accurately ripped$1(confidence $2)$3$4</span>", -1)
		trackBody, _ = replaceCount(trackBody,
			arCannotRe,
			"<span class=\"badish\">$0</span>", -1)

		// XLD AR
		trackBody, _ = replaceCountCallback(trackBody,
			arXldRe,
			lc.arXldCallback, -1)
		trackBody, _ = replaceCount(trackBody,
			arXldInaccRe,
			"<span class=\"log4\">AccurateRip signature$1: <span class=\"badish\">$2</span></span>\n$3<span class=\"badish\">$4$5</span>", -1)
		trackBody, _ = replaceCount(trackBody, ripInaccRe,
			"<span class=\"badish\">$1$2</span>", -1)
		trackBody, _ = replaceCount(trackBody,
			arXldNotInDBRe,
			"<span class=\"log4\">AccurateRip signature$1: <span class=\"badish\">$2</span></span>\n$3<span class=\"badish\">$4$5</span>", -1)
		trackBody, _ = replaceCount(trackBody,
			arXldMatchedRe,
			"<span class=\"goodish\">$0</span>", -1)

		// AR track confidence
		if m2 := arConfRe.FindStringSubmatch(trackBody); m2 != nil {
			lc.arTracks[trackNumStr], _ = strconv.Atoi(m2[1])
		} else {
			lc.arTracks[trackNumStr] = 0
		}

		trackBody = strings.ReplaceAll(trackBody, "Copy finished", "<span class=\"log3\">Copy finished</span>")
		trackBody, _ = replaceCount(trackBody, copyOKRe, "<span class=\"good\">Copy OK</span>", -1)

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
	rawLog, _ = replaceCount(rawLog, xldAllStatHdrRe, "$1<span class=\"log5\">$2</span>\n", 1)
	rawLog, _ = replaceCount(rawLog, xldStatsHdrRe, "$1<span class=\"log5\">$2</span>\n", 1)
	for _, pat := range []*regexp.Regexp{
		xldReadErrRe, xldSkippedRe, xldJitterRe, xldEdgeJitterRe, xldAtomJitterRe,
		xldDriftRe, xldDroppedRe, xldDuplicatedRe, xldRetryRe, xldDamagedRe,
	} {
		rawLog, _ = replaceCountCallback(rawLog, pat, lc.xldAllStatCallback, 1)
	}

	return rawLog
}

// findTrackBoundary scans exploded lines backwards and returns the index of the
// last line belonging to the track listing. Lines ending in any logEnds marker
// (and an immediately preceding line) are trimmed off; iteration also stops at a
// trackEndRe match. Returns -1 if every line is trimmed.
func findTrackBoundary(exploded []string, logEnds []string) int {
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
		if trackEndRe.MatchString(exploded[i]) {
			break
		}
		i--
	}
	return i
}

func (lc *Logchecker) checkTracks(logIdx int) {
	if len(lc.tracks[logIdx]) == 0 {
		// nil-reset clears all previously accumulated details so the "no tracks"
		// message is the only one reported; the score is set to 0 unconditionally.
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

// ─────────────────────────────────────────────────────────────────────────────
// callback helpers — ported from PHP callback methods
// ─────────────────────────────────────────────────────────────────────────────
