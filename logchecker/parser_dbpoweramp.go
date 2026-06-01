package logchecker

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Nirzak/logchecker-go/internal/check"
	"github.com/Nirzak/logchecker-go/internal/util"
)

var (
	dbVersionRe      = regexp.MustCompile(`(?im)^(dBpoweramp Release )([^\s]+)( Digital Audio Extraction Log from )(.+)`)
	dbDriveRe        = regexp.MustCompile(`(?i)Ripping with drive '([^']+)',\s*Drive offset:\s*([+-]?\d+)`)
	dbDriveModelRe   = regexp.MustCompile(`^([A-Z]:\s+)(\[.+\])\s*$`)
	dbC2Re           = regexp.MustCompile(`(?i)Using C2:\s*(Yes|No)`)
	dbFUARe          = regexp.MustCompile(`(?i)FUA Cache Invalidate:\s*(Yes|No)`)
	dbCacheRe        = regexp.MustCompile(`(?i)(Cache:\s*)(\d+\s*KB)`)
	dbPassSpeedRe    = regexp.MustCompile(`(?i)(Pass 1 Drive Speed:\s*)(\S+)(,\s*Pass 2 Drive Speed:\s*)(\S+)`)
	dbMaxReReadsRe   = regexp.MustCompile(`(?i)Maximum Re-reads:\s*(\d+)`)
	dbUltraRe        = regexp.MustCompile(`(?im)^Ultra::(\s*Vary Drive Speed:\s*)(\S+)(,\s*Min Passes:\s*)(\d+)(,\s*Max Passes:\s*)(\d+)(,\s*Finish After Clean Passes:\s*)(\d+)`)
	dbBadSectorRe    = regexp.MustCompile(`(?i)(Bad Sector Re-rip:)(:\s*Drive Speed:\s*[^,\s]+)`)
	dbOverreadRe     = regexp.MustCompile(`(?i)(Overread Lead-in/out:\s*)(\S+)`)
	dbAccurateRipRe  = regexp.MustCompile(`(?i)(AccurateRip:\s*)(Active)`)
	dbEncoderRe      = regexp.MustCompile(`(?im)^Encoder:\s*(.+)`)
	dbTrackHeaderRe  = regexp.MustCompile(`(?im)\nTrack (\d+):([ \t]+(?:Ripped|ERROR)[^\n]+)`)
	dbTracksRippedRe = regexp.MustCompile(`(?i)(\d+) Tracks Ripped:\s*(.+)`)
	dbTracksAccRe    = regexp.MustCompile(`(?i)(\d+ Tracks Ripped Accurately)`)
	dbUserStopRe     = regexp.MustCompile(`(?im)^(User Stopped Ripping)`)
	dbLBARe          = regexp.MustCompile(`(?i)(Ripped LBA\s+)([^ \t(]+(?:\s+to\s+[^ \t(]+)?)(\s+\()([^)]+)(\)\s+in\s+)([^\s.]+)`)
	dbFileRe         = regexp.MustCompile(`(?i)(Filename:\s*)(.+)`)
	dbSecureWarnRe   = regexp.MustCompile(`(?im)^(\s*)(Secure \(Warning\))([ \t]+\[([^\]]+)\])`)
	dbSecureRe       = regexp.MustCompile(`(?im)^(\s*)Secure([ \t]+\[[^\]]+\])`)
	dbARAccurateRe   = regexp.MustCompile(`(?im)^(\s*)AccurateRip:\s*Accurate\s*\(confidence\s*(\d+)\)`)
	dbARInaccurateRe = regexp.MustCompile(`(?im)^(\s*)AccurateRip:\s*Inaccurate`)
	dbARNotInDBRe    = regexp.MustCompile(`(?im)^(\s*)AccurateRip:\s*Not in Database`)
	dbCRC32Re        = regexp.MustCompile(`(?i)(CRC32:\s*)([0-9A-F]{8})`)
	dbARCRCRe        = regexp.MustCompile(`(?i)(AccurateRip CRC:\s*)([0-9A-F]{8})`)
	dbARVerConfRe    = regexp.MustCompile(`(?i)(AccurateRip Verified Confidence\s*)(\d+)(\s*\[[^\]\s]+\s+)([0-9A-F]{8})(\])`)
	dbDiscIDRe       = regexp.MustCompile(`(?i)(\[DiscID:\s*)([^\]]+)(\])`)
	dbReRipFramesRe  = regexp.MustCompile(`(?i)Re-Rip (\d+) Frames`)
)

func (lc *Logchecker) dbpowerampParse() {
	lc.checksumStatus = check.ChecksumMissing
	lc.log = util.NormalizeLineEndings(lc.log)

	// Version
	if m := regexp.MustCompile(`(?im)^dBpoweramp Release ([^\s]+)`).FindStringSubmatch(lc.log); m != nil {
		lc.ripperVersion = m[1]
		verNum, _ := strconv.ParseFloat(m[1], 64)
		lc.log = dbVersionRe.
			ReplaceAllString(lc.log, "<span class='good'>$1<span class='log1'>$2</span>$3<span class='log1'>$4</span></span>")
		if verNum < 14 {
			lc.account("[Notice] dBpoweramp version older than 14 — older versions had less robust secure ripping.",
				0, -1, false, false)
		}
	}

	// Drive & offset
	if m := dbDriveRe.FindStringSubmatch(lc.log); m != nil {
		driveName := strings.TrimSpace(m[1])
		driveOffset := m[2]
		// Extract drive letter prefix and bracket model part
		drivePrefix := driveName // fallback: whole name if no bracket found
		driveBracket := ""
		driveModel := driveName
		if mm := dbDriveModelRe.FindStringSubmatch(driveName); mm != nil {
			drivePrefix = mm[1]
			driveBracket = mm[2]
			driveModel = strings.Trim(driveBracket, "[]")
		}
		lc.getDrives(driveModel)

		isFake := false
		for _, f := range lc.fakeDrives {
			if strings.TrimSpace(driveName) == f {
				isFake = true
				break
			}
		}

		var driveClass, offsetClass string
		var driveAnnotated string
		if isFake {
			lc.account("Virtual drive used: "+driveName, 20, -1, false, false)
			driveClass, offsetClass = "bad", "bad"
			driveAnnotated = driveName
		} else if len(lc.drives) > 0 {
			lc.driveFound = true
			driveClass = "good"
			found := false
			for _, o := range lc.offsets {
				if o == driveOffset {
					found = true
					break
				}
			}
			if found {
				offsetClass = "good"
			} else {
				offsetClass = "bad"
				lc.account("Incorrect read offset for drive. Correct offsets are: "+
					strings.Join(lc.offsets, ", ")+" (Checked against the following drive(s): "+
					strings.Join(lc.drives, ", ")+")", 5, -1, false, false)
			}
			if driveBracket != "" {
				driveAnnotated = drivePrefix + "<span class='log4'>" + driveBracket + "</span>"
			} else {
				driveAnnotated = driveName
			}
		} else {
			driveClass = "badish"
			notInDB := " (not found in database)"
			if driveOffset == "0" {
				offsetClass = "bad"
				lc.account("The drive was not found in the database, so we cannot determine the correct read offset. "+
					"However, the read offset in this case was 0, which is almost never correct. "+
					"As such, we are assuming that the offset is incorrect", 5, -1, false, false)
			} else {
				offsetClass = "badish"
			}
			if driveBracket != "" {
				driveAnnotated = drivePrefix + "<span class='log4'>" + driveBracket + notInDB + "</span>"
			} else {
				driveAnnotated = driveName + notInDB
			}
		}
		lc.log = dbDriveRe.ReplaceAllString(lc.log,
			"Ripping with drive '<span class='"+driveClass+"'>"+driveAnnotated+"</span>',  Drive offset: <span class='"+offsetClass+"'>"+driveOffset+"</span>")
	} else {
		lc.account("Could not verify used drive", 1, -1, false, false)
	}

	// C2
	if m := dbC2Re.FindStringSubmatch(lc.log); m != nil {
		c2Class := "good"
		if strings.ToLower(m[1]) == "yes" {
			c2Class = "bad"
			lc.account("C2 pointers were used", 10, -1, false, false)
		}
		lc.log = dbC2Re.ReplaceAllString(lc.log, "Using C2: <span class='"+c2Class+"'>$1</span>")
	}

	// FUA Cache Invalidate
	if m := dbFUARe.FindStringSubmatch(lc.log); m != nil {
		fuaClass := "good"
		if strings.ToLower(m[1]) == "no" {
			fuaClass = "badish"
			lc.account("[Notice] FUA Cache Invalidate is disabled (audio cache may not be fully defeated).",
				0, -1, false, false)
		}
		lc.log = dbFUARe.ReplaceAllString(lc.log, "FUA Cache Invalidate: <span class='"+fuaClass+"'>$1</span>")
	}

	// Cache size
	lc.log = dbCacheRe.ReplaceAllString(lc.log, "${1}<span class='log4'>${2}</span>")

	// Pass 1/2 Drive Speed
	lc.log = dbPassSpeedRe.ReplaceAllString(lc.log,
		"Pass 1 Drive Speed: <span class='log4'>$2</span>,  Pass 2 Drive Speed: <span class='log4'>$4</span>")

	// Max Re-reads
	if m := dbMaxReReadsRe.FindStringSubmatch(lc.log); m != nil {
		reReads, _ := strconv.Atoi(m[1])
		reClass := "good"
		if reReads < 10 {
			reClass = "bad"
			lc.account("Maximum re-reads is too low (< 10), which may reduce rip quality", 5, -1, false, false)
		}
		lc.log = dbMaxReReadsRe.ReplaceAllString(lc.log, "Maximum Re-reads: <span class='"+reClass+"'>$1</span>")
	}

	// Bad Sector Re-rip drive speed
	lc.log = dbBadSectorRe.ReplaceAllString(lc.log,
		"$1<span class='log4'>$2</span>")

	// Ultra mode with value annotations
	lc.log = dbUltraRe.ReplaceAllString(lc.log,
		"<span class='good'>Ultra:<span class='log4'>:</span></span>$1<span class='log4'>$2</span>$3<span class='log4'>$4</span>$5<span class='log4'>$6</span>$7<span class='log4'>$8</span>")

	// Overread Lead-in/out
	lc.log = dbOverreadRe.ReplaceAllString(lc.log, "${1}<span class='log4'>${2}</span>")

	// AccurateRip active
	lc.log = dbAccurateRipRe.ReplaceAllString(lc.log, "${1}<span class='good'>${2}</span>")

	// Encoder
	if m := dbEncoderRe.FindStringSubmatch(lc.log); m != nil {
		enc := strings.TrimSpace(m[1])
		encClass := "good"
		if regexp.MustCompile(`(?i)mp3|aac|ogg|wma|opus`).MatchString(enc) {
			encClass = "bad"
			lc.account("Lossy encoder detected — rip is not lossless", 0, 0, false, false)
		} else if strings.Contains(strings.ToLower(enc), `wave`) {
			encClass = "badish"
		}
		lc.log = dbEncoderRe.ReplaceAllString(lc.log, "Encoder: <span class='"+encClass+"'>"+enc+"</span>")
	}

	// Section headers
	lc.log = regexp.MustCompile(`(?im)^(Drive & Settings)\s*$`).
		ReplaceAllString(lc.log, "<span class='log4 log5'>$1</span>")
	lc.log = regexp.MustCompile(`(?im)^(Extraction Log)\s*$`).
		ReplaceAllString(lc.log, "<span class='log4 log5'>$1</span>")
	lc.log = regexp.MustCompile(`(?im)^-{3,}\s*$`).
		ReplaceAllString(lc.log, "<strong>$0</strong>")

	// Track parsing
	trackHeaders := dbTrackHeaderRe.FindAllStringSubmatchIndex(lc.log, -1)
	if len(trackHeaders) == 0 {
		lc.account("No tracks found in log", 0, 0, false, false)
		return
	}

	type formattedTrack struct {
		number        string
		text          string
		decreasescore int
		bad           []string
	}
	var formattedTracks []formattedTrack
	inaccurateCount := 0

	for i, hIdx := range trackHeaders {
		trackNum := lc.log[hIdx[2]:hIdx[3]]
		headerRest := lc.log[hIdx[4]:hIdx[5]]

		startPos := hIdx[1]
		var endPos int
		if i+1 < len(trackHeaders) {
			endPos = trackHeaders[i+1][0]
		} else {
			if idx := strings.Index(lc.log[startPos:], "\n<strong>--------------"); idx >= 0 {
				endPos = startPos + idx
			} else {
				endPos = len(lc.log)
			}
		}
		trackBody := lc.log[startPos:endPos]

		lc.trackNumber = trackNum
		lc.decreaseTrack = 0
		lc.badTrack = nil

		isError := strings.Contains(strings.ToLower(headerRest), "error")
		if isError {
			lc.accountTrack("Track ripping error — could not complete rip", 20)
		}

		// Annotate LBA
		headerRest = dbLBARe.
			ReplaceAllString(headerRest, "$1<span class='log1'>$2</span>$3<span class='log1'>$4</span>$5<span class='log1'>$6</span>")
		headerRest = dbFileRe.ReplaceAllString(headerRest, "$1<span class='log3'>$2</span>")
		if isError {
			headerRest = "<span class='bad'>" + headerRest + "</span>"
		}

		statusAnnotated := false

		if m := dbSecureWarnRe.FindStringSubmatch(trackBody); m != nil {
			reRipFrames := 0
			if rf := dbReRipFramesRe.FindStringSubmatch(m[4]); rf != nil {
				reRipFrames, _ = strconv.Atoi(rf[1])
			}
			if reRipFrames > 16 {
				lc.accountTrack("Secure (Warning) with high re-rip frame count ("+strconv.Itoa(reRipFrames)+" frames)", 2)
			} else {
				lc.accountTrack("Secure (Warning) — re-rip occurred during extraction", 1)
			}
			trackBody = dbSecureWarnRe.ReplaceAllString(trackBody,
				"$1<span class='badish'>$2</span><span class='log4'>$3</span>")
			statusAnnotated = true
		} else if dbSecureRe.MatchString(trackBody) {
			trackBody = dbSecureRe.ReplaceAllString(trackBody,
				"$1<span class='good'>Secure$2</span>")
			statusAnnotated = true
		} else if dbARAccurateRe.MatchString(trackBody) {
			trackBody = regexp.MustCompile(`(?im)^(\s*)(AccurateRip:\s*Accurate\s*\(confidence\s*\d+\).*)`).
				ReplaceAllString(trackBody, "$1<span class='good'>$2</span>")
			statusAnnotated = true
		} else if dbARInaccurateRe.MatchString(trackBody) {
			inaccurateCount++
			lc.accountTrack("AccurateRip: Inaccurate — data integrity cannot be confirmed", 10)
			trackBody = regexp.MustCompile(`(?im)^(\s*)(AccurateRip:\s*Inaccurate.*)`).
				ReplaceAllString(trackBody, "$1<span class='bad'>$2</span>")
			statusAnnotated = true
		} else if dbARNotInDBRe.MatchString(trackBody) {
			trackBody = regexp.MustCompile(`(?im)^(\s*)(AccurateRip:\s*Not in Database.*)`).
				ReplaceAllString(trackBody, "$1<span class='badish'>$2</span>")
			statusAnnotated = true
		}

		if !statusAnnotated && !isError {
			lc.accountTrack("Could not determine track rip status", 5)
		}

		// Annotate CRCs
		trackBody = dbCRC32Re.ReplaceAllString(trackBody, "$1<span class='log3'>$2</span>")
		trackBody = dbARCRCRe.ReplaceAllString(trackBody, "$1<span class='log3'>$2</span>")
		trackBody = dbARVerConfRe.ReplaceAllString(trackBody,
			"<span class='good'>$1<span class='log4'>$2</span>$3<span class='log3'>$4</span>$5</span>")
		trackBody = dbDiscIDRe.ReplaceAllString(trackBody, "$1<span class='log3'>$2</span>$3")

		formHeader := "\n<span class='log5'>Track</span> <span class='log4 log1'>" + trackNum + "</span>:" + headerRest
		formattedTracks = append(formattedTracks, formattedTrack{
			number:        trackNum,
			text:          formHeader + trackBody,
			decreasescore: lc.decreaseTrack,
			bad:           lc.badTrack,
		})
	}

	// Apply per-track deductions
	for _, t := range formattedTracks {
		if t.decreasescore > 0 {
			lc.score -= t.decreasescore
		}
		lc.details = append(lc.details, t.bad...)
	}

	// Summary line
	if m := dbTracksRippedRe.FindStringSubmatch(lc.log); m != nil {
		total, _ := strconv.Atoi(m[1])
		if total == 0 {
			lc.account("No tracks were ripped successfully", 0, 0, false, false)
		}
		summaryInaccurate := 0
		if si := regexp.MustCompile(`(?i)(\d+)\s*Inaccurate`).FindStringSubmatch(m[2]); si != nil {
			summaryInaccurate, _ = strconv.Atoi(si[1])
		}
		if summaryInaccurate != inaccurateCount {
			lc.account(fmt.Sprintf("[Notice] Summary inaccurate count (%d) does not match per-track count (%d)",
				summaryInaccurate, inaccurateCount), 0, -1, false, false)
		}
	} else if dbUserStopRe.MatchString(lc.log) {
		lc.account("Ripping was aborted by user — log is incomplete", 0, 0, false, false)
		lc.log = dbUserStopRe.ReplaceAllString(lc.log, "<span class='bad'>$1</span>")
	}

	// Rebuild log with annotated tracks
	extractStart := strings.Index(lc.log, "\n<span class='log4 log5'>Extraction Log</span>")
	summaryStart := strings.LastIndex(lc.log, "\n<strong>--------------")
	if extractStart >= 0 && summaryStart > extractStart {
		before := lc.log[:extractStart]
		afterSep := lc.log[summaryStart:]

		// Annotate "N Tracks Ripped Accurately" summary
		afterSep = dbTracksAccRe.ReplaceAllString(afterSep, "<span class='good'>$1</span>")

		// Annotate summary line: "N Tracks Ripped: X Secure, Y Secure (Warning)"
		afterSep = dbTracksRippedRe.ReplaceAllStringFunc(afterSep, func(s string) string {
			m := dbTracksRippedRe.FindStringSubmatch(s)
			if m == nil {
				return s
			}
			total := m[1]
			detailStr := m[2]
			reWarn := regexp.MustCompile(`(?i)^\d+\s+Secure\s*\(Warning\)$`)
			reSecure := regexp.MustCompile(`(?i)^\d+\s+Secure$`)
			reInaccurate := regexp.MustCompile(`(?i)^\d+\s+Inaccurate`)
			tokens := strings.Split(detailStr, ",")
			for i, tok := range tokens {
				trimmed := strings.TrimSpace(tok)
				if reWarn.MatchString(trimmed) {
					tokens[i] = "<span class='badish'>" + trimmed + "</span>"
				} else if reInaccurate.MatchString(trimmed) {
					tokens[i] = "<span class='badish'>" + trimmed + "</span>"
				} else if reSecure.MatchString(trimmed) {
					tokens[i] = "<span class='good'>" + trimmed + "</span>"
				} else {
					tokens[i] = trimmed
				}
			}
			return "<span class='log4'>" + total + " Tracks Ripped:</span> " + strings.Join(tokens, ", ")
		})

		var trackBlock strings.Builder
		for _, t := range formattedTracks {
			trackBlock.WriteString(t.text)
		}
		lc.log = before + "\n<span class='log4 log5'>Extraction Log</span>\n<strong>--------------</strong>" +
			trackBlock.String() + "\n" + afterSep
	}
}
