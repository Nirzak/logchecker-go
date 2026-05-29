# Logchecker — Agent Reference

> **Purpose**: This file gives AI agents a fast, accurate orientation to the `Nirzak/logchecker-go` codebase so that they can reason, modify, and extend it without having to re-read the whole source tree.

---

## 1. What the project is

**Logchecker** is a Go library (and CLI tool) that parses and scores CD-rip log files produced by four rippers:

| Ripper | Constant | Log signature |
|---|---|---|
| Exact Audio Copy (EAC) | `check.EAC` | Contains `"Exact Audio Copy"` |
| X Lossless Decoder (XLD) | `check.XLD` | Contains `"X Lossless Decoder version"` |
| whipper | `check.WHIPPER` | Contains `"Log created by: whipper"` |
| dBpoweramp | `check.DBPOWERAMP` | First line matches `^dBpoweramp Release` |

A score starts at **100** and decreases based on problems found (bad settings, checksum failures, CRC mismatches, etc.). The final score and an array of human-readable detail messages are the primary output.

---

## 2. Repository layout

```
logchecker-go/
├── cmd/
│   └── logchecker/
│       └── main.go             # CLI wrapper for the logchecker tool
├── internal/
│   ├── check/
│   │   ├── checksum.go         # Checksum state constants + validation logic
│   │   └── ripper.go           # Detects ripper from raw log text
│   ├── parser/
│   │   └── eac/
│   │       ├── translator.go   # EAC-only: detects log language, translates to English
│   │       └── languages/      # JSON mappings for non-English to English translations
│   └── util/
│       └── encoding.go         # Encoding detection and conversion to UTF-8
├── logchecker/
│   ├── logchecker.go           # Core package — all parsing and scoring logic
│   └── resources/
│       └── drives.json         # ~6 000 drive entries mapped by Exact Audio Copy
├── logchecker_test.go          # Data-provider test against real log fixtures
├── tests/
│   └── logs/
│       ├── eac/                # originals/ details/ html/ utf8/
│       ├── xld/                # originals/ details/ html/
│       ├── whipper/            # originals/ details/ html/
│       └── dbpoweramp/         # (reserved / extra fixtures)
├── go.mod                      # Go module definition and dependencies
└── README.md
```

---

## 3. Packages & Imports

| Package | Purpose |
|---|---|
| `github.com/Nirzak/logchecker-go/logchecker` | Core API to be imported by users |
| `github.com/Nirzak/logchecker-go/internal/check` | Internal checks (ripper detection, checksums) |
| `github.com/Nirzak/logchecker-go/internal/util` | Internal encoding and utility functions |
| `github.com/Nirzak/logchecker-go/internal/parser/eac` | EAC specific language parsing |

---

## 4. Core Struct: `Logchecker`

**File**: [`logchecker/logchecker.go`](logchecker/logchecker.go)

### Public API

```go
lc := logchecker.New()

// Load a file (resets all internal state)
err := lc.NewFile("/path/to/file.log")

// Run analysis
lc.Parse()

// Results
lc.GetRipper()          // string: "EAC" | "XLD" | "whipper" | "unknown"
lc.GetRipperVersion()   // string
lc.GetScore()           // int 0–100
lc.GetChecksumState()   // "checksum_ok" | "checksum_invalid" | "checksum_missing"
lc.GetDetails()         // []string — human-readable list of deductions / notices
lc.GetLanguage()        // string language code, e.g. "en", "ru"
lc.GetLog()             // string — HTML-annotated log text (span-tagged)
lc.IsCombinedLog()      // bool — true when the file holds multiple rip sessions

// Control
lc.ValidateChecksum(false) // Disable external checksum validation
```

### Parse flow (`Parse()`)

```
Parse()
 ├── util.DecodeEncoding()           Converts log to UTF-8 (BOM detection + charmap fallback)
 ├── check.GetRipper()               Determines ripper type
 ├── if DBPOWERAMP → dbpowerampParse()  Settings + per-track regex parsing
 ├── if WHIPPER → whipperParse()     YAML-based parsing
 └── else → legacyParse()
          ├── if EAC → eac.Translate Auto-detect + translate non-English to English
          ├── Split log into sections by checksum delimiter or "End of status report"
          ├── Per-section: regexp loops over line items
          │    Each callback annotates the log text with HTML spans
          │    AND calls account() or accountTrack() if a problem is detected
          └── checkTracks() — fails score to 0 if no tracks found
```

### Scoring mechanics

- `account(msg, decrease, setScore, inclCombined, notice)` — **global** deduction.  
  - `decrease` → `lc.score -= decrease`  
  - `setScore` → `lc.score = setScore` (absolute override)  
  - Deduplication: same message string is never added twice.
- `accountTrack(msg, decrease)` — **per-track** deduction.  
  - Accumulated per track (applied at end of loop).  
  - Track-level messages carry `"Track NN: …"` prefix.

### HTML annotation classes

| Class | Meaning |
|---|---|
| `good` | Green — expected/correct value |
| `goodish` | Cyan — acceptable but not ideal |
| `badish` | Yellow — suspicious / minor issue |
| `bad` | Red — definite problem |
| `log1` | Underline — version/date info |
| `log3` | Blue — file / technical data |
| `log4` | Bold — field values |
| `log5` | Underline — field labels |

---

## 5. Checksum validation

**File**: [`internal/check/checksum.go`](internal/check/checksum.go)

Unlike the original PHP version which required Python tools, the Go version incorporates checksum verification directly using native Go ports.

| Ripper | Method | Dependency |
|---|---|---|
| whipper | SHA-256 hash computed in Go, compared to last line of log | none |
| dBpoweramp | Always `CHECKSUM_MISSING` — no embedded checksum | none |
| EAC | Uses native Go package | `github.com/Nirzak/eac-logchecker` |
| XLD | Uses native Go package | `github.com/Nirzak/xld-logchecker` |

---

## 6. Drive offset matching

- `logchecker/resources/drives.json` — JSON array of `[drive_name_lowercase, offset_int]` pairs.
- Loaded via `go:embed` when the package is initialized.
- Matching uses `normalizeDriveName()` to strip alias substitutions (e.g. `HL-DT-ST` → `LG Electronics`), whitespace collapse, and revision suffixes before exact matching.

---

## 7. EAC multi-language support

- **Detection**: `eac.GetLanguage()` scans `master.json` for EAC-specific marker strings.
- **Translation**: `eac.Translate()` does regex-replacement of foreign phrases. Keys are integers; case-sensitivity differs by key range (`> 16` uses case-insensitive matching). Translation phrases are sorted by descending length to prevent shorter phrases corrupting longer translations.
- **Supported languages (16)**: bg, cs, de, en, es, fr, it, jp, ko, nl, pl, ru, se, sk, sr, zh.

---

## 8. whipper parsing specifics

- Log format is YAML — parsed via `gopkg.in/yaml.v3`.
- Pre-parse fixups handle two known whipper bugs:
  1. Un-escaped YAML strings in `Release`/`Album` fields.
  2. CRCs starting with `0` that `yaml.v3` would interpret as octal.
- Minimum supported whipper version: **0.7.3** (earlier versions have octal track number bugs).

---

## 9. CLI interface

The CLI is located in `cmd/logchecker/main.go`.

| Command | Key options |
|---|---|
| `analyze` / `analyse` | `--html`, `--no_text`, optional `out_file`, optional `details` (JSON) |
| `decode` | Converts log encoding to UTF-8; prints to stdout or file |
| `translate` | `--language (-l)` to force language code |

---

## 10. Testing

```bash
go test -v ./...       # runs the standard Go test framework
```

### Test fixture layout (`tests/logs/<ripper>/`)

```
originals/   ← raw .log files fed to Logchecker
details/     ← expected JSON output (ripper, version, language, combined, score, checksum, details[])
html/        ← expected getLog() HTML output
utf8/        ← UTF-8 re-encoded versions (for decode command tests)
```

`logchecker_test.go` iterates all files in `tests/logs/*/originals/`, parses them, and asserts equality with the corresponding `details/*.json` and `html/*.log` fixtures.

---

## 11. Dependencies

### `go.mod` Requirements

| Package | Used for |
|---|---|
| `go 1.25.0` | Language runtime |
| `golang.org/x/text` | Encoding detection and codepage fallbacks |
| `gopkg.in/yaml.v3` | Parsing whipper YAML logs |
| `github.com/Nirzak/eac-logchecker` | Native Go port of EAC logchecker |
| `github.com/Nirzak/xld-logchecker` | Native Go port of XLD logchecker |

---

## 12. Important constants and tunables

| Constant / key | Default | Location | Effect |
|---|---|---|---|
| `validateChecksum` | `true` | instance | Set via `lc.ValidateChecksum(false)` |
| `check.ChecksumOk` | `"checksum_ok"` | `internal/check/checksum.go` | Passed checksum or tool missing |
| `check.ChecksumInvalid` | `"checksum_invalid"` | `internal/check/checksum.go` | Failed/Tampered log |
| `check.ChecksumMissing` | `"checksum_missing"` | `internal/check/checksum.go` | No checksum present |

---

## 13. Key scoring deductions (representative list)

| Condition | Points |
|---|---|
| Unknown log / corrupt encoding | −100 (score → 0) |
| EAC version older than 0.99 | −30 |
| Range rip detected | −30 |
| CRC mismatch per track | −30 |
| No test-and-copy used | −10 |
| Rip not in Secure mode AND no T+C | −40 additional |
| Accurate stream not Yes | −20 |
| C2 pointers used | −10 |
| Defeat audio cache not Yes | −10 |
| Incorrect gap handling | −10 |
| Normalization active | −100 (score → 0) |
| Virtual / fake drive used | −20 |
| Incorrect read offset | −5 |
| ID3 tags added to FLAC | −1 |
| Suspicious/timing position per track | −20 |
| Read error per track (capped 10) | −1 to −10 |

---

## 14. Patterns to follow when modifying

1. **Adding a new check**: Add or modify the matching regex in `logchecker.go`. Extract substrings and call `lc.account()` or `lc.accountTrack()` when issues are detected.
2. **Adding a new language**: Update `internal/parser/eac/languages/<code>.json` using the integer-keyed schema, and add the detection string to `master.json`.
3. **Updating test fixtures**: When scoring logic changes, the `details` arrays in `logchecker_test.go` will complain. Adjust them to properly reflect the new scores.
