package logchecker

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Nirzak/logchecker-go/internal/check"
	"gopkg.in/yaml.v3"
)

var (
	whipperVersionRe = regexp.MustCompile(`whipper ([0-9]+\.[0-9]+\.[0-9]+)`)
	crcRe            = regexp.MustCompile(`CRC: ([A-Z0-9]+)`)
	logCreatedByRe   = regexp.MustCompile(`(?i)^(whipper)\s+([^\s]+)`)
)

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

	var parsedRaw interface{}
	if err := yaml.Unmarshal([]byte(fixed), &parsedRaw); err != nil {
		lc.account("Could not parse whipper log.", 100, -1, false, false)
		return
	}
	parsed, ok := sanitizeYAMLMap(parsedRaw).(map[string]interface{})
	if !ok {
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

func sanitizeYAMLMap(m interface{}) interface{} {
	switch m := m.(type) {
	case map[interface{}]interface{}:
		res := make(map[string]interface{})
		for k, v := range m {
			res[fmt.Sprintf("%v", k)] = sanitizeYAMLMap(v)
		}
		return res
	case map[string]interface{}:
		res := make(map[string]interface{})
		for k, v := range m {
			res[k] = sanitizeYAMLMap(v)
		}
		return res
	case []interface{}:
		res := make([]interface{}, len(m))
		for i, v := range m {
			res[i] = sanitizeYAMLMap(v)
		}
		return res
	default:
		return m
	}
}

// rpiFieldOrder defines the canonical output order of Ripping phase information fields.
var rpiFieldOrder = []string{
	"Drive", "Extraction engine", "Defeat audio cache", "Read offset correction",
	"Overread into lead-out", "Gap detection", "CD-R detected",
}

// cdMetaFieldOrder defines the canonical output order of CD metadata fields.
var cdMetaFieldOrder = []string{
	"Release", "Album", "CDDB Disc ID", "MusicBrainz Disc ID",
	"MusicBrainz Disc Id", "MusicBrainz lookup URL", "MusicBrainz lookup url",
	"MusicBrainz Release URL",
}

// tocFieldOrder defines the canonical output order of TOC track fields.
var tocFieldOrder = []string{"Start", "Length", "Start sector", "End sector"}

// trackFieldOrder defines the canonical output order of per-track fields.
var trackFieldOrder = []string{
	"Filename", "Pre-gap length", "Peak level", "Pre-emphasis",
	"Extraction speed", "Extraction quality",
	"Test CRC", "Copy CRC",
	"AccurateRip v1", "AccurateRip v2",
	"Status",
}

// arFieldOrder defines the canonical output order of AccurateRip sub-fields.
var arFieldOrder = []string{"Result", "Confidence", "Local CRC", "Remote CRC"}

// csrFieldOrder defines the canonical output order of Conclusive status report fields.
var csrFieldOrder = []string{"AccurateRip summary", "Health Status", "Health status", "EOF"}

func writeOrderedFields(sb *strings.Builder, indent string, m map[string]interface{}, order []string) {
	// Write known keys in order, then any remaining keys alphabetically.
	seen := make(map[string]bool)
	for _, k := range order {
		if v, ok := m[k]; ok {
			seen[k] = true
			writeWhipperKV(sb, indent, k, v)
		}
	}
	// Any extra keys not in the order list — sort for determinism.
	remaining := make([]string, 0)
	for k := range m {
		if !seen[k] {
			remaining = append(remaining, k)
		}
	}
	sort.Strings(remaining)
	for _, k := range remaining {
		writeWhipperKV(sb, indent, k, m[k])
	}
}

func writeWhipperKV(sb *strings.Builder, indent, k string, v interface{}) {
	if v == nil {
		sb.WriteString(indent + k + ": \n")
		return
	}
	switch val := v.(type) {
	case bool:
		if val {
			sb.WriteString(indent + k + ": true\n")
		} else {
			sb.WriteString(indent + k + ": false\n")
		}
	case map[string]interface{}:
		sb.WriteString(indent + k + ":\n")
		writeOrderedFields(sb, indent+"  ", val, arFieldOrder)
	default:
		sb.WriteString(indent + k + ": " + fmt.Sprintf("%v", val) + "\n")
	}
}

func renderWhipperLog(parsed map[string]interface{}) string {
	var sb strings.Builder

	if lcb, ok := parsed["Log created by"]; ok {
		sb.WriteString("Log created by: " + fmt.Sprintf("%v", lcb) + "\n")
	}
	if lcd, ok := parsed["Log creation date"]; ok {
		var lcdStr string
		if t, ok := lcd.(time.Time); ok {
			lcdStr = t.UTC().Format(time.RFC3339)
		} else {
			lcdStr = fmt.Sprintf("%v", lcd)
		}
		sb.WriteString("Log creation date: " + lcdStr + "\n")
	}
	sb.WriteString("\n")

	if rpi, ok := parsed["Ripping phase information"].(map[string]interface{}); ok {
		sb.WriteString("Ripping phase information:\n")
		writeOrderedFields(&sb, "  ", rpi, rpiFieldOrder)
		sb.WriteString("\n")
	}

	if cd, ok := parsed["CD metadata"].(map[string]interface{}); ok {
		sb.WriteString("CD metadata:\n")
		writeOrderedFields(&sb, "  ", cd, cdMetaFieldOrder)
		sb.WriteString("\n")
	}

	if toc, ok := parsed["TOC"].(map[string]interface{}); ok {
		sb.WriteString("TOC:\n")
		keys := make([]string, 0, len(toc))
		for k := range toc {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			ni, err1 := strconv.Atoi(keys[i])
			nj, err2 := strconv.Atoi(keys[j])
			if err1 == nil && err2 == nil {
				return ni < nj
			}
			return keys[i] < keys[j]
		})
		for _, k := range keys {
			sb.WriteString("  " + k + ":\n")
			if t, ok := toc[k].(map[string]interface{}); ok {
				writeOrderedFields(&sb, "    ", t, tocFieldOrder)
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
		sort.Slice(keys, func(i, j int) bool {
			ni, err1 := strconv.Atoi(keys[i])
			nj, err2 := strconv.Atoi(keys[j])
			if err1 == nil && err2 == nil {
				return ni < nj
			}
			return keys[i] < keys[j]
		})
		for _, k := range keys {
			sb.WriteString("  " + k + ":\n")
			if t, ok := tracks[k].(map[string]interface{}); ok {
				for _, fk := range trackFieldOrder {
					vv, ok := t[fk]
					if !ok {
						continue
					}
					switch val := vv.(type) {
					case map[string]interface{}:
						sb.WriteString("    " + fk + ":\n")
						writeOrderedFields(&sb, "      ", val, arFieldOrder)
					case bool:
						if val {
							sb.WriteString("    " + fk + ": Yes\n")
						} else {
							sb.WriteString("    " + fk + ": No\n")
						}
					default:
						if vv == nil {
							sb.WriteString("    " + fk + ": \n")
						} else {
							sb.WriteString("    " + fk + ": " + fmt.Sprintf("%v", vv) + "\n")
						}
					}
				}
			}
			sb.WriteString("\n")
		}
	}

	if csr, ok := parsed["Conclusive status report"].(map[string]interface{}); ok {
		sb.WriteString("Conclusive status report:\n")
		writeOrderedFields(&sb, "  ", csr, csrFieldOrder)
		sb.WriteString("\n")
	}

	if hash, ok := parsed["SHA-256 hash"]; ok {
		sb.WriteString("SHA-256 hash: " + fmt.Sprintf("%v", hash) + "\n")
	}

	return sb.String()
}
