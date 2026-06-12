// Package logchecker parses and scores CD-rip log files produced by EAC, XLD,
// whipper, and dBpoweramp.
package logchecker

import (
	_ "embed"
	"encoding/json"
	"os"
	"strings"

	"github.com/Nirzak/logchecker-go/internal/check"
	"github.com/Nirzak/logchecker-go/internal/parser/eac"
	"github.com/Nirzak/logchecker-go/internal/util"
)

//go:embed resources/drives.json
var drivesJSON []byte

// LevenshteinDistance controls fuzzy drive matching (0 = exact match only).
var LevenshteinDistance = 0

// Version of the logchecker library.
const Version = "1.14.6"

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
		lc.accountFatal("Could not detect log encoding, log is corrupt.", 0)
		return
	}
	lc.log = decoded

	ripper, err := check.GetRipper(lc.log)
	if err != nil {
		lc.score = 0
		lc.accountFatal("Unknown log file, could not determine ripper.", 0)
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
