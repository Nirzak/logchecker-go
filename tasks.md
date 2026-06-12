# Plan Execution Tasks

## Step 1: Dead Code Removal
- [x] 1.1 Remove `math` import + `var _ = math.Abs` from logchecker.go
- [x] 1.2 Remove duplicate `nilIfEmptyStr`, update call-site in main.go
- [x] 1.3 Remove unused `strings.NewReplacer` in `htmlToConsole`
- [x] 1.4 Remove `_ = i` in parser_legacy.go
- [x] 1.5 Fix redundant `cls = "good"` in `c2PointersEacPre99Callback`
- [x] 1.6 Remove unused `decoder` interface in encoding.go

## Step 2: Regex Hoisting (critical perf)
- [x] 2.1 Hoist all inline `regexp.MustCompile` in parser_legacy.go → package-level vars
- [x] 2.1b Hoist inline `regexp.MustCompile` in parser_dbpoweramp.go
- [x] 2.1c Hoist inline `regexp.MustCompile` in callbacks.go

## Step 3: Helper Extraction
- [x] 3.1 Add `accountDeduction`, `accountNotice`, `accountFatal` to scoring.go; update call-sites
- [x] 3.2 Add `spanClass` / `settingLine` helpers; update callbacks.go
- [x] 3.3 Add CSS class constants
- [x] 3.4 Add `accountIncorrectOffset` / `accountZeroOffsetUnknownDrive` helpers
- [x] 3.5 Add `accountVirtualDrive` helper

## Step 4: Shared Logic
- [x] 2.3 Extract `combinedSuffix()` method (callbacks.go)
- [x] 2.5 Replace `min3` with builtin `min` (drive.go)
- [x] 2.6 Levenshtein O(n) single-row DP
- [x] 5.1 Extract `pointsStr` helper (scoring.go)
- [x] 5.2 Extract `sortNumericStrings` helper
- [x] 5.3 Add comment to `checkTracks` nil reset

## Step 5: Miscellaneous
- [x] 5.4 Move `encoding_maccentraleurope.log` hack into test infra (add comment)

## Not implemented (plan.md sections 2.2, 2.4, 4.x — structural refactoring, highest regression risk, lower priority):

2.2: findTrackBoundary helper extraction
2.4: xldStatCallback error-capping helper
4.1: legacyParse function split
4.2: Unified drive validation
4.3: driveEntry struct refactor

## Verification
- [x] Run `go test ./...` — all pass
- [x] Run `go vet ./...` — clean
