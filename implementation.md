# Refactor `logchecker/logchecker.go`

## Background

`logchecker/logchecker.go` is a 2 718-line, ~99 KB single file that holds
every concern of the package:

- Core type definitions and lifecycle (`New`, `NewFile`, `reset`, getters)
- Scoring helpers (`account`, `accountTrack`)
- Drive-matching logic (normalization + Levenshtein)
- Three independent ripper parsers:
  - **whipper** – YAML-based, `whipperParse()` + render helpers
  - **dBpoweramp** – regex-based, `dbpowerampParse()`
  - **EAC / XLD (legacy)** – line-by-line regex, `legacyParse()` + many callbacks
- Regex utility functions (`replaceCount`, `replaceCountCallback`, etc.)
- Version-comparison helper

## Strategy

Split the file into focused files **within the same `logchecker` package**.
This keeps the public API identical (zero impact on callers, zero import
changes) while making each concern navigable and testable in isolation.

All new files live in `logchecker/` alongside the existing `resources/` dir.

---

## Proposed File Split

### [MODIFY] logchecker.go (shrink from 2 718 → ~160 lines)
Keep only:
- Package doc comment
- `import` block (trimmed to what's still needed here)
- `//go:embed resources/drives.json`
- Package-level `var LevenshteinDistance` and `const Version`
- Type declarations: `driveEntry`, `Logchecker`, `trackData`
- `New()`, `NewFile()`, `reset()`
- Public getter / setter methods
- `Parse()` dispatcher
- `var _ = math.Abs` suppress line

---

### [NEW] logchecker/scoring.go (~70 lines)
Move:
- `account()`
- `accountTrack()`

---

### [NEW] logchecker/drive.go (~120 lines)
Move:
- All drive-matching `var` regexes (`reTSSTcorp`, `reHLDTST`, …)
- `normalizeDriveName()`
- `levenshtein()`
- `min3()`
- `getDrives()`

---

### [NEW] logchecker/parser_whipper.go (~300 lines)
Move:
- `var whipperVersionRe`, `crcRe`, `logCreatedByRe`
- `whipperParse()`
- `sanitizeYAMLMap()`
- All field-order `var` slices (`rpiFieldOrder`, `cdMetaFieldOrder`, …)
- `writeOrderedFields()`
- `writeWhipperKV()`
- `renderWhipperLog()`

---

### [NEW] logchecker/parser_dbpoweramp.go (~350 lines)
Move:
- All `db*Re` compiled regex `var` block
- `dbpowerampParse()`

---

### [NEW] logchecker/parser_legacy.go (~900 lines)
Move:
- All legacy regex `var` block (`eacChecksumRe`, `splitEACRe`, …)
- `legacyParse()`
- `checkTracks()`
- `isNonSecure()`

---

### [NEW] logchecker/callbacks.go (~450 lines)
Move all `*Callback` receiver methods:
- `driveCallback`, `mediaTypeXldCallback`, `readModeCallback`
- `ripperModeXldCallback`, `cdparanoiaModeXldCallback`
- `maxRetryCountCallback`, `accurateStreamCallback`
- `accurateStreamEacPre9Callback`, `defeatAudioCacheCallback`
- `defeatAudioCacheEacPre99Callback`, `defeatAudioCacheXldCallback`
- `c2PointersCallback`, `c2PointersEacPre99Callback`
- `readOffsetCallback`, `fillOffsetSamplesCallback`
- `deleteSilentBlocksCallback`, `nullSamplesCallback`
- `normalizeEacCallback`, `gapHandlingCallback`, `gapHandlingXldCallback`
- `addId3TagCallback`, `testCopyCallback`, `testCopyXldCallback`
- `arXldCallback`, `arSummaryConfXldCallback`, `arSummaryConfCallback`
- `xldStatCallback`, `xldAllStatCallback`

---

### [NEW] logchecker/regex_util.go (~100 lines)
Move:
- `splitWithDelim()`
- `splitWithCaptures()`
- `replaceCount()`
- `replaceCountCallback()`
- `compareVersions()`

---

## Verification Plan

### Automated Tests
- `go build ./...` — must succeed with zero errors
- `go test ./...` — all existing tests must pass unchanged

### Manual Verification
- Confirm line counts: original 2 718 lines distributed across 7 files with
  no net additions of logic
- The public API (`New`, `NewFile`, `Parse`, all getters) remains unchanged
