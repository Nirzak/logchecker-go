// Package logchecker parses and scores CD-rip log files produced by EAC, XLD,
// whipper, and dBpoweramp.
package logchecker

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Nirzak/logchecker-go/internal/check"
	"github.com/Nirzak/logchecker-go/internal/parser/eac"
	"github.com/Nirzak/logchecker-go/internal/util"
	"gopkg.in/yaml.v3"
)

//go:embed resources/drives.json
var drivesJSON []byte

// LevenshteinDistance controls fuzzy drive matching (0 = exact match only).
var LevenshteinDistance = 0

// Version of the logchecker library.
const Version = "1.14.4"

// drive entry from drives.json: [name, offset]
type driveEntry [2]interface{}

// Logchecker holds all state for a single parse run.
type Logchecker struct {
	log              string
	logPath          string
	logs             []string
	language         string
	tracks           map[int]map[string]trackData
	checksumStatus   string
	score            int
	details          []string
	offsets          []string
	driveFound       bool
	allDrives        []driveEntry
	drives           []string
	secureMode       bool
	nonSecureMode    string
	badTrack         []string
	decreaseTrack    int
	ripper           string
	ripperVersion    string
	trackNumber      string
	arTracks         map[string]int
	combined         int // 0 = not combined, >0 = number of sessions
	currLog          int
	rangeRip         bool
	arSummary        map[string]interface{}
	xldSecureRipper  bool
	validateChecksum bool
	fakeDrives       []string
}

type trackData struct {
	number        string
	spaces        string
	text          string
	decreasescore int
	bad           []string
}

// New creates a ready-to-use Logchecker instance.
func New() *Logchecker {
	lc := &Logchecker{
		validateChecksum: true,
		fakeDrives: []string{
			"Generic DVD-ROM",
			"Generic DVD-ROM SCSI CdRom Device",
		},
	}
	var raw [][]interface{}
	if err := json.Unmarshal(drivesJSON, &raw); err == nil {
		for _, entry := range raw {
			if len(entry) >= 2 {
				name, ok1 := entry[0].(string)
				offset := entry[1]
				if ok1 {
					lc.allDrives = append(lc.allDrives, driveEntry{name, offset})
				}
			}
		}
	}
	return lc
}

// NewFile resets state and loads a log file.
func (lc *Logchecker) NewFile(path string) error {
	lc.reset()
	lc.logPath = path
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lc.log = string(data)
	return nil
}

func (lc *Logchecker) reset() {
	lc.logPath = ""
	lc.logs = nil
	lc.tracks = make(map[int]map[string]trackData)
	lc.checksumStatus = check.ChecksumOK
	lc.score = 100
	lc.details = nil
	lc.offsets = nil
	lc.driveFound = false
	lc.drives = nil
	lc.secureMode = true
	lc.nonSecureMode = ""
	lc.badTrack = nil
	lc.decreaseTrack = 0
	lc.ripper = ""
	lc.ripperVersion = ""
	lc.trackNumber = ""
	lc.arTracks = make(map[string]int)
	lc.combined = 0
	lc.currLog = 0
	lc.rangeRip = false
	lc.arSummary = make(map[string]interface{})
	lc.xldSecureRipper = false
	lc.language = "en"
}

// ValidateChecksum enables or disables external checksum validation.
func (lc *Logchecker) ValidateChecksum(b bool) { lc.validateChecksum = b }

// GetLog returns the HTML-annotated log text.
func (lc *Logchecker) GetLog() string { return lc.log }

// GetRipper returns the detected ripper name.
func (lc *Logchecker) GetRipper() string { return lc.ripper }

// GetRipperVersion returns the detected ripper version string.
func (lc *Logchecker) GetRipperVersion() string { return lc.ripperVersion }

// GetScore returns the current score (0–100).
func (lc *Logchecker) GetScore() int { return lc.score }

// GetDetails returns the list of detail messages accumulated during parse.
func (lc *Logchecker) GetDetails() []string { return lc.details }

// GetChecksumState returns one of check.Checksum* constants.
func (lc *Logchecker) GetChecksumState() string { return lc.checksumStatus }

// GetLanguage returns the detected log language code (e.g. "en", "ru").
func (lc *Logchecker) GetLanguage() string { return lc.language }

// IsCombinedLog returns true when the file contains multiple rip sessions.
func (lc *Logchecker) IsCombinedLog() bool { return lc.combined > 0 }

// GetAcceptValues returns the accepted file extensions string.
func GetAcceptValues() string { return ".txt,.TXT,.log,.LOG" }

// GetVersion returns the logchecker version.
func GetVersion() string { return Version }

// Parse runs the full analysis pipeline.
func (lc *Logchecker) Parse() {
	isWhipper := strings.Contains(lc.log, "Log created by: whipper")
	decoded, err := util.DecodeEncoding([]byte(lc.log), isWhipper, func(s string) int {
		score := 0
		if strings.Contains(s, "Exact Audio Copy") {
			score += 10
		}
		if strings.Contains(s, "X Lossless Decoder") {
			score += 10
		}
		if strings.Contains(s, "whipper") {
			score += 10
		}
		if strings.Contains(s, "dBpoweramp Release") {
			score += 10
		}
		info, lErr := eac.GetLanguage(s)
		if lErr == nil {
			score += 50
			if info.Code != "en" {
				_, count, _ := eac.Translate(s, info.Code)
				score += count * 5
			}
		}
		return score
	})
	if err != nil {
		lc.score = 0
		lc.account("Could not detect log encoding, log is corrupt.", 0, 0, false, false)
		return
	}
	lc.log = decoded

	ripper, err := check.GetRipper(lc.log)
	if err != nil {
		lc.score = 0
		lc.account("Unknown log file, could not determine ripper.", 0, 0, false, false)
		lc.ripper = check.Unknown
		return
	}
	lc.ripper = ripper

	switch ripper {
	case check.DBpoweramp:
		lc.dbpowerampParse()
	case check.Whipper:
		lc.whipperParse()
	default:
		lc.legacyParse()
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// scoring helpers
// ─────────────────────────────────────────────────────────────────────────────

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

// ─────────────────────────────────────────────────────────────────────────────
// drive matching
// ─────────────────────────────────────────────────────────────────────────────

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
	d := make([][]int, ls+1)
	for i := range d {
		d[i] = make([]int, lt+1)
	}
	for i := 0; i <= ls; i++ {
		d[i][0] = i
	}
	for j := 0; j <= lt; j++ {
		d[0][j] = j
	}
	for i := 1; i <= ls; i++ {
		for j := 1; j <= lt; j++ {
			cost := 1
			if sr[i-1] == tr[j-1] {
				cost = 0
			}
			d[i][j] = min3(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
		}
	}
	return d[ls][lt]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
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

// ─────────────────────────────────────────────────────────────────────────────
// whipper parser
// ─────────────────────────────────────────────────────────────────────────────

var whipperVersionRe = regexp.MustCompile(`whipper ([0-9]+\.[0-9]+\.[0-9]+)`)
var crcRe = regexp.MustCompile(`CRC: ([A-Z0-9]+)`)
var logCreatedByRe = regexp.MustCompile(`(?i)^(whipper)\s+([^\s]+)`)

func (lc *Logchecker) whipperParse() {
	if m := whipperVersionRe.FindStringSubmatch(lc.log); m != nil {
		if compareVersions(m[1], "0.7.3") < 0 {
			lc.account("Logs must be produced by whipper 0.7.3+.", 100, -1, false, false)
			return
		}
	}

	// Fix un-escaped YAML strings for Release/Album fields.
	fixFieldRe := regexp.MustCompile(`  (Release|Album): (.+)`)
	fixed := fixFieldRe.ReplaceAllStringFunc(lc.log, func(s string) string {
		m := fixFieldRe.FindStringSubmatch(s)
		if m == nil {
			return s
		}
		val := m[2]
		// Minimal YAML-safe quoting: wrap in double quotes if not already.
		if !strings.HasPrefix(val, `"`) {
			val = `"` + strings.ReplaceAll(val, `"`, `\"`) + `"`
		}
		return "  " + m[1] + ": " + val
	})
	// Wrap CRC values in quotes to prevent octal interpretation.
	fixed = crcRe.ReplaceAllString(fixed, `CRC: "$1"`)

	var parsed map[string]interface{}
	if err := yaml.Unmarshal([]byte(fixed), &parsed); err != nil {
		lc.account("Could not parse whipper log.", 100, -1, false, false)
		return
	}

	// Version
	if lcb, ok := parsed["Log created by"].(string); ok {
		parts := strings.Fields(lcb)
		if len(parts) >= 2 {
			lc.ripperVersion = parts[1]
		}
		if lc.ripperVersion == "" || compareVersions(lc.ripperVersion, "0.7.3") < 0 {
			lc.account("Logs must be produced by whipper 0.7.3+", 100, -1, false, false)
			return
		}
		// Annotate
		parsed["Log created by"] = logCreatedByRe.ReplaceAllString(lcb,
			"<span class='good'>$1</span> <span class='log1'>$2</span>")
	}

	// Checksum
	if hash, ok := parsed["SHA-256 hash"].(string); ok && hash != "" {
		lc.checksumStatus = check.Validate(lc.logPath, check.Whipper)
		cls := "good"
		if lc.checksumStatus != check.ChecksumOK {
			cls = "bad"
		}
		parsed["SHA-256 hash"] = fmt.Sprintf("<span class='%s'>%s</span>", cls, hash)
	} else {
		lc.checksumStatus = check.ChecksumMissing
	}

	// Ripping phase
	key := "Ripping phase information"
	if rpi, ok := parsed[key].(map[string]interface{}); ok {
		drive, _ := rpi["Drive"].(string)
		offsetRaw := rpi["Read offset correction"]
		var offsetStr string
		switch v := offsetRaw.(type) {
		case int:
			offsetStr = strconv.Itoa(v)
		case float64:
			offsetStr = strconv.Itoa(int(v))
		case string:
			offsetStr = v
		}

		isFake := false
		for _, f := range lc.fakeDrives {
			if strings.TrimSpace(drive) == f {
				isFake = true
				break
			}
		}

		if isFake {
			lc.account("Virtual drive used: "+drive, 20, -1, false, false)
			rpi["Drive"] = "<span class='bad'>" + drive + "</span>"
		} else {
			lc.getDrives(drive)
			driveClass := "badish"
			offsetClass := "badish"
			if len(lc.drives) > 0 {
				driveClass = "good"
				found := false
				for _, o := range lc.offsets {
					if o == offsetStr {
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
			} else {
				drive += " (not found in database)"
				if offsetStr == "0" {
					offsetClass = "bad"
					lc.account("The drive was not found in the database, so we cannot determine the correct read "+
						"offset. However, the read offset in this case was 0, which is almost never correct. "+
						"As such, we are assuming that the offset is incorrect", 5, -1, false, false)
				}
			}
			rpi["Drive"] = fmt.Sprintf("<span class='%s'>%s</span>", driveClass, drive)
			if n, err := strconv.Atoi(offsetStr); err == nil && n > 0 {
				offsetStr = "+" + offsetStr
			}
			rpi["Read offset correction"] = fmt.Sprintf("<span class='%s'>%s</span>", offsetClass, offsetStr)
		}

		// Defeat audio cache
		defeatRaw := rpi["Defeat audio cache"]
		var defeatStr string
		var defeatClass string
		switch v := defeatRaw.(type) {
		case bool:
			if v {
				defeatStr, defeatClass = "true", "good"
			} else {
				defeatStr, defeatClass = "false", "bad"
			}
		case string:
			if strings.ToLower(v) == "yes" {
				defeatStr, defeatClass = "Yes", "good"
			} else {
				defeatStr, defeatClass = "No", "bad"
			}
		}
		if defeatClass == "bad" {
			lc.account(`"Defeat audio cache" should be Yes/true`, 10, -1, false, false)
		}
		rpi["Defeat audio cache"] = fmt.Sprintf("<span class='%s'>%s</span>", defeatClass, defeatStr)

		// Overread
		if orl, ok := rpi["Overread into lead-out"]; ok {
			var s string
			switch v := orl.(type) {
			case bool:
				if v {
					s = "true"
				} else {
					s = "false"
				}
			default:
				s = fmt.Sprintf("%v", v)
			}
			rpi["Overread into lead-out"] = "<span class='log4'>" + s + "</span>"
		}
		parsed[key] = rpi
	}

	// CD metadata
	if cdMeta, ok := parsed["CD metadata"].(map[string]interface{}); ok {
		releaseKey := "Release"
		if _, ok := cdMeta["Release"]; !ok {
			releaseKey = "Album"
		}
		switch v := cdMeta[releaseKey].(type) {
		case string:
			cdMeta[releaseKey] = "<span class='log4'>" + v + "</span>"
		case map[string]interface{}:
			if a, ok := v["Artist"].(string); ok {
				v["Artist"] = "<span class='log4'>" + a + "</span>"
			}
			if t, ok := v["Title"].(string); ok {
				v["Title"] = "<span class='log4'>" + t + "</span>"
			}
			cdMeta[releaseKey] = v
		}
		parsed["CD metadata"] = cdMeta
	}

	// TOC
	if toc, ok := parsed["TOC"].(map[string]interface{}); ok {
		for k, trackRaw := range toc {
			if t, ok := trackRaw.(map[string]interface{}); ok {
				for _, field := range []string{"Start", "Length", "Start sector", "End sector"} {
					if v, ok := t[field]; ok {
						t[field] = fmt.Sprintf("<span class='log1'>%v</span>", v)
					}
				}
				toc[k] = t
			}
		}
		parsed["TOC"] = toc
	}

	// Tracks
	if tracks, ok := parsed["Tracks"].(map[string]interface{}); ok {
		for k, trackRaw := range tracks {
			t, ok := trackRaw.(map[string]interface{})
			if !ok {
				continue
			}
			// Peak level
			if pl, ok := t["Peak level"].(float64); ok {
				t["Peak level"] = fmt.Sprintf("%.6f", pl)
			}
			// CRC comparison
			testCRC, _ := t["Test CRC"].(string)
			copyCRC, _ := t["Copy CRC"].(string)
			crcClass := "good"
			if testCRC != copyCRC {
				crcClass = "bad"
				lc.account(fmt.Sprintf("CRC mismatch: %s and %s", testCRC, copyCRC), 30, -1, false, false)
			}
			t["Test CRC"] = fmt.Sprintf("<span class='%s'>%s</span>", crcClass, testCRC)
			t["Copy CRC"] = fmt.Sprintf("<span class='%s'>%s</span>", crcClass, copyCRC)
			// Status
			status, _ := t["Status"].(string)
			statusClass := "bad"
			if status == "Copy OK" {
				statusClass = "good"
			}
			t["Status"] = fmt.Sprintf("<span class='%s'>%s</span>", statusClass, status)
			// Other fields
			for _, field := range []string{"Filename", "Pre-gap length", "Peak level", "Extraction speed", "Extraction quality"} {
				if v, ok := t[field]; ok {
					t[field] = fmt.Sprintf("<span class='log3'>%v</span>", v)
				}
			}
			// AccurateRip
			for _, ver := range []string{"v1", "v2"} {
				arKey := "AccurateRip " + ver
				if ar, ok := t[arKey].(map[string]interface{}); ok {
					result, _ := ar["Result"].(string)
					arClass := "badish"
					if result == "Found, exact match" {
						arClass = "good"
					}
					ar["Result"] = fmt.Sprintf("<span class='%s'>%s</span>", arClass, result)
					if lCRC, ok := ar["Local CRC"].(string); ok {
						if rCRC, ok := ar["Remote CRC"].(string); ok {
							cls := "badish"
							if lCRC == rCRC {
								cls = "goodish"
							}
							ar["Local CRC"] = fmt.Sprintf("<span class='%s'>%s</span>", cls, lCRC)
							ar["Remote CRC"] = fmt.Sprintf("<span class='%s'>%s</span>", cls, rCRC)
						}
					}
					t[arKey] = ar
				}
			}
			tracks[k] = t
		}
		parsed["Tracks"] = tracks
	}

	// Conclusive status report
	if csr, ok := parsed["Conclusive status report"].(map[string]interface{}); ok {
		arSummary, _ := csr["AccurateRip summary"].(string)
		arClass := "badish"
		if arSummary == "All tracks accurately ripped" {
			arClass = "good"
		}
		csr["AccurateRip summary"] = fmt.Sprintf("<span class='%s'>%s</span>", arClass, arSummary)
		healthKey := "Health Status"
		if _, ok := csr["Health status"]; ok {
			healthKey = "Health status"
		}
		health, _ := csr[healthKey].(string)
		healthClass := "bad"
		if health == "No errors occurred" {
			healthClass = "good"
		}
		csr[healthKey] = fmt.Sprintf("<span class='%s'>%s</span>", healthClass, health)
		parsed["Conclusive status report"] = csr
	}

	// Re-render log as annotated text.
	lc.log = renderWhipperLog(parsed)
}

func renderWhipperLog(parsed map[string]interface{}) string {
	var sb strings.Builder

	writeKV := func(indent, k string, v interface{}) {
		switch val := v.(type) {
		case bool:
			if val {
				sb.WriteString(indent + k + ": true\n")
			} else {
				sb.WriteString(indent + k + ": false\n")
			}
		case map[string]interface{}:
			sb.WriteString(indent + k + ":\n")
			for kk, vv := range val {
				sb.WriteString(indent + "  " + kk + ": " + fmt.Sprintf("%v", vv) + "\n")
			}
		default:
			sb.WriteString(indent + k + ": " + fmt.Sprintf("%v", val) + "\n")
		}
	}

	if lcb, ok := parsed["Log created by"]; ok {
		sb.WriteString("Log created by: " + fmt.Sprintf("%v", lcb) + "\n")
	}
	if lcd, ok := parsed["Log creation date"]; ok {
		sb.WriteString("Log creation date: " + fmt.Sprintf("%v", lcd) + "\n")
	}
	sb.WriteString("\n")

	if rpi, ok := parsed["Ripping phase information"].(map[string]interface{}); ok {
		sb.WriteString("Ripping phase information:\n")
		for k, v := range rpi {
			writeKV("  ", k, v)
		}
		sb.WriteString("\n")
	}

	if cd, ok := parsed["CD metadata"].(map[string]interface{}); ok {
		sb.WriteString("CD metadata:\n")
		for k, v := range cd {
			writeKV("  ", k, v)
		}
		sb.WriteString("\n")
	}

	if toc, ok := parsed["TOC"].(map[string]interface{}); ok {
		sb.WriteString("TOC:\n")
		// Sort track keys
		keys := make([]string, 0, len(toc))
		for k := range toc {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sb.WriteString("  " + k + ":\n")
			if t, ok := toc[k].(map[string]interface{}); ok {
				for kk, vv := range t {
					sb.WriteString("    " + kk + ": " + fmt.Sprintf("%v", vv) + "\n")
				}
			}
			sb.WriteString("\n")
		}
	}

	if tracks, ok := parsed["Tracks"].(map[string]interface{}); ok {
		sb.WriteString("Tracks:\n")
		keys := make([]string, 0, len(tracks))
		for k := range tracks {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sb.WriteString("  " + k + ":\n")
			if t, ok := tracks[k].(map[string]interface{}); ok {
				for kk, vv := range t {
					switch val := vv.(type) {
					case map[string]interface{}:
						sb.WriteString("    " + kk + ":\n")
						for kkk, vvv := range val {
							sb.WriteString("      " + kkk + ": " + fmt.Sprintf("%v", vvv) + "\n")
						}
					case bool:
						if val {
							sb.WriteString("    " + kk + ": Yes\n")
						} else {
							sb.WriteString("    " + kk + ": No\n")
						}
					default:
						sb.WriteString("    " + kk + ": " + fmt.Sprintf("%v", vv) + "\n")
					}
				}
			}
			sb.WriteString("\n")
		}
	}

	if csr, ok := parsed["Conclusive status report"].(map[string]interface{}); ok {
		sb.WriteString("Conclusive status report:\n")
		for k, v := range csr {
			sb.WriteString("  " + k + ": " + fmt.Sprintf("%v", v) + "\n")
		}
		sb.WriteString("\n")
	}

	if hash, ok := parsed["SHA-256 hash"]; ok {
		sb.WriteString("SHA-256 hash: " + fmt.Sprintf("%v", hash) + "\n")
	}

	return sb.String()
}

// ─────────────────────────────────────────────────────────────────────────────
// dBpoweramp parser
// ─────────────────────────────────────────────────────────────────────────────

var (
	dbVersionRe      = regexp.MustCompile(`(?im)^dBpoweramp Release ([^\s]+)( Digital Audio Extraction Log from )(.+)`)
	dbDriveRe        = regexp.MustCompile(`(?i)Ripping with drive '([^']+)',\s*Drive offset:\s*([+-]?\d+)`)
	dbDriveModelRe   = regexp.MustCompile(`^[A-Z]:\s+\[(.+)\]\s*$`)
	dbC2Re           = regexp.MustCompile(`(?i)Using C2:\s*(Yes|No)`)
	dbFUARe          = regexp.MustCompile(`(?i)FUA Cache Invalidate:\s*(Yes|No)`)
	dbMaxReReadsRe   = regexp.MustCompile(`(?i)Maximum Re-reads:\s*(\d+)`)
	dbUltraRe        = regexp.MustCompile(`(?im)^Ultra::`)
	dbAccurateRipRe  = regexp.MustCompile(`(?i)(AccurateRip:\s*)(Active)`)
	dbEncoderRe      = regexp.MustCompile(`(?im)^Encoder:\s*(.+)`)
	dbTrackHeaderRe  = regexp.MustCompile(`(?im)\nTrack (\d+):([ \t]+(?:Ripped|ERROR)[^\n]+)`)
	dbTracksRippedRe = regexp.MustCompile(`(?i)(\d+) Tracks Ripped:\s*(.+)`)
	dbTracksAccRe    = regexp.MustCompile(`(?i)(\d+ Tracks Ripped Accurately)`)
	dbUserStopRe     = regexp.MustCompile(`(?i)User Stopped Ripping`)
	dbLBARe          = regexp.MustCompile(`(?i)(Ripped LBA\s+)([^ \t(]+(?:\s+to\s+[^ \t(]+)?)(` + "`" + `\s+\()([^)]+)(\)\s+in\s+)([^\s.]+)`)
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
		lc.log = regexp.MustCompile(`(?im)^(dBpoweramp Release )([^\s]+)( Digital Audio Extraction Log from )(.+)`).
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
		// Strip drive letter prefix [VENDOR - MODEL]
		driveModel := driveName
		if mm := dbDriveModelRe.FindStringSubmatch(driveName); mm != nil {
			driveModel = mm[1]
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
		if isFake {
			lc.account("Virtual drive used: "+driveName, 20, -1, false, false)
			driveClass, offsetClass = "bad", "bad"
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
		} else {
			driveClass = "badish"
			driveName += " (not found in database)"
			if driveOffset == "0" {
				offsetClass = "bad"
				lc.account("The drive was not found in the database, so we cannot determine the correct read offset. "+
					"However, the read offset in this case was 0, which is almost never correct. "+
					"As such, we are assuming that the offset is incorrect", 5, -1, false, false)
			} else {
				offsetClass = "badish"
			}
		}
		lc.log = dbDriveRe.ReplaceAllString(lc.log,
			"Ripping with drive '<span class='"+driveClass+"'>"+driveName+"</span>',  Drive offset: <span class='"+offsetClass+"'>"+driveOffset+"</span>")
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

	// Ultra mode
	lc.log = dbUltraRe.ReplaceAllString(lc.log, "<span class='good'>Ultra::</span>")

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
		headerRest = regexp.MustCompile(`(?i)(Ripped LBA\s+)([^ \t(]+(?:\s+to\s+[^ \t(]+)?)(\s+\()([^)]+)(\)\s+in\s+)([^\s.]+)`).
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
	}

	// Rebuild log with annotated tracks
	extractStart := strings.Index(lc.log, "\n<span class='log4 log5'>Extraction Log</span>")
	summaryStart := strings.LastIndex(lc.log, "\n<strong>--------------")
	if extractStart >= 0 && summaryStart > extractStart {
		before := lc.log[:extractStart]
		afterSep := lc.log[summaryStart:]
		var trackBlock strings.Builder
		for _, t := range formattedTracks {
			trackBlock.WriteString(t.text)
		}
		lc.log = before + "\n<span class='log4 log5'>Extraction Log</span>\n<strong>--------------</strong>" +
			trackBlock.String() + "\n" + afterSep
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// legacy parser (EAC + XLD)
// ─────────────────────────────────────────────────────────────────────────────

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
	for i, l := range lc.logs {
		l = strings.TrimSpace(l)
		if l == "" || regexp.MustCompile(`(?i)^\-+$`).MatchString(l) {
			continue
		}
		if lc.checksumStatus != check.ChecksumOK && regexp.MustCompile(`(?i)End of status report`).MatchString(l) {
			if len(cleaned) > 0 {
				cleaned[len(cleaned)-1] += l
			}
			continue
		}
		if lc.checksumStatus == check.ChecksumOK && checksumRe.MatchString(l) {
			if len(cleaned) > 0 {
				cleaned[len(cleaned)-1] += l
			}
			continue
		}
		if lc.checksumStatus == check.ChecksumOK && regexp.MustCompile(`(?i)[\-]+BEGIN XLD SIGNATURE`).MatchString(l) {
			if len(cleaned) > 0 {
				cleaned[len(cleaned)-1] += l
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
		rawLog, _ = replaceCount(rawLog, regexp.MustCompile(`(?i)(List of \w+ offset correction values) *\n( *# +\| +Absolute +\| +Relative +\| +Confidence) *\n( *\-+) *\n(( *\d+ +\| +\-?\+?\d+ +\| +\-?\+?\d+ +\| +\d+ *\n)+)`),
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
		rawLog, cnt = replaceCount(rawLog, regexp.MustCompile(`(?i)All Tracks(\s*\n)((?:.*)\n)?(\s*Album gain\s+:) (.*)?(\n\s*Peak\s+:) (.*)?`),
			`<span class="log5">All Tracks</span>$1$2<strong>$3 <span class="log3">$4</span>`+"\n"+"$5 <span class=\"log3\">$6</span></strong>", 1)
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
		rawLog = regexp.MustCompile(`(?i)(Track +\d+ +: +)(OK +)\(A?R?\d?,? ?confidence +(\d+).*?\)(.*)\n`).
			ReplaceAllStringFunc(rawLog, lc.arSummaryConfXldCallback)
		rawLog = regexp.MustCompile(`(?i)(Track +\d+ +: +)(NG|Not Found).*?\n`).
			ReplaceAllStringFunc(rawLog, lc.arSummaryConfXldCallback)

		// Status line
		rawLog, _ = replaceCount(rawLog, regexp.MustCompile(`(?i)( *.{2} ?)(\d+ track\(s\).*)\n`),
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
			m := regexp.MustCompile(`(?i)\n( *)Filename +(.*)([\s\S]+)`).FindStringSubmatch(rawLog)
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
				regexp.MustCompile(`(?is)Filename ((.+)?\.(wav|flac|ape))\n`),
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
				regexp.MustCompile(`(?i)( *Track gain\s+:) (.*)?(\n\s*Peak\s+:) (.*)?`),
				"<strong>$1 <span class=\"log3\">$2</span>\n$3 <span class=\"log3\">$4</span></strong>", -1)

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

	lc.log = strings.Join(lc.logs, "")
	if len(lc.log) == 0 {
		lc.score = 0
		lc.account("Unrecognized log file! Feel free to report for manual review.", 0, -1, false, false)
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

// ─────────────────────────────────────────────────────────────────────────────
// callback helpers — ported from PHP callback methods
// ─────────────────────────────────────────────────────────────────────────────

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
	case strings.Contains(m[2], "Appended to previous track"):
		cls = "good"
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

// ─────────────────────────────────────────────────────────────────────────────
// regex helper utilities
// ─────────────────────────────────────────────────────────────────────────────

// splitWithDelim splits s by re but includes captured delimiters inline.
func splitWithDelim(s string, re *regexp.Regexp) []string {
	// Equivalent to PHP preg_split with PREG_SPLIT_DELIM_CAPTURE
	var result []string
	last := 0
	for _, loc := range re.FindAllStringIndex(s, -1) {
		result = append(result, s[last:loc[0]])
		result = append(result, s[loc[0]:loc[1]])
		last = loc[1]
	}
	result = append(result, s[last:])
	return result
}

// splitWithCaptures splits s by re returning alternating: [cap1, cap2, body, cap1, cap2, body, ...]
func splitWithCaptures(s string, re *regexp.Regexp) []string {
	var captures []string
	matches := re.FindAllStringSubmatchIndex(s, -1)
	for _, m := range matches {
		// Each match: m[0],m[1] = full; m[2],m[3] = cap1; m[4],m[5] = cap2
		for i := 2; i < len(m)-1; i += 2 {
			if m[i] >= 0 {
				captures = append(captures, s[m[i]:m[i+1]])
			} else {
				captures = append(captures, "")
			}
		}
	}
	return captures
}

// replaceCount replaces up to n occurrences (n=-1 for all) and returns the count.
func replaceCount(s string, re *regexp.Regexp, repl string, n int) (string, int) {
	count := 0
	result := re.ReplaceAllStringFunc(s, func(match string) string {
		if n >= 0 && count >= n {
			return match
		}
		count++
		return re.ReplaceAllString(match, repl)
	})
	return result, count
}

// replaceCountCallback replaces up to n occurrences using a callback.
func replaceCountCallback(s string, re *regexp.Regexp, fn func([]string) string, n int) (string, int) {
	count := 0
	result := re.ReplaceAllStringFunc(s, func(match string) string {
		if n >= 0 && count >= n {
			return match
		}
		m := re.FindStringSubmatch(match)
		if m == nil {
			return match
		}
		count++
		return fn(m)
	})
	return result, count
}

// ─────────────────────────────────────────────────────────────────────────────
// version comparison
// ─────────────────────────────────────────────────────────────────────────────

// compareVersions compares two semver-ish version strings.
// Returns -1 if a < b, 0 if equal, 1 if a > b.
func compareVersions(a, b string) int {
	partsA := strings.Split(a, ".")
	partsB := strings.Split(b, ".")
	n := len(partsA)
	if len(partsB) > n {
		n = len(partsB)
	}
	for i := 0; i < n; i++ {
		var na, nb int
		if i < len(partsA) {
			na, _ = strconv.Atoi(partsA[i])
		}
		if i < len(partsB) {
			nb, _ = strconv.Atoi(partsB[i])
		}
		if na < nb {
			return -1
		}
		if na > nb {
			return 1
		}
	}
	return 0
}

// suppress unused import warning
var _ = math.Abs
