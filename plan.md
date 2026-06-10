# logchecker-go — Optimization & Refactoring Plan

> Generated after a full analysis of the codebase (~2,700 LOC across 8 Go source files in the `logchecker/` package, plus `internal/` and `cmd/`).

---

## Table of Contents

1. [Remove Redundant / Dead Code](#1-remove-redundant--dead-code)
2. [Reduce Complexity / Use More Optimal Alternatives](#2-reduce-complexity--use-more-optimal-alternatives)
3. [Extract Commonly Used lc.account Values & HTML Outputs Into Variables](#3-extract-commonly-used-lcaccount-values--html-outputs-into-variables)
4. [Structural Refactoring](#4-structural-refactoring)
5. [Miscellaneous Improvements](#5-miscellaneous-improvements)

---

## 1. Remove Redundant / Dead Code

### 1.1 — Unused `math` import with suppressor hack

**File:** `logchecker.go:5-8, 216-217`

```go
import "math"
// ...
// suppress unused import warning
var _ = math.Abs
```

The `math` package is imported and then suppressed with a dummy variable. This import serves no purpose and should be removed entirely.

---

### 1.2 — Duplicate `nilIfEmpty` and `nilIfEmptyStr` in CLI

**File:** `cmd/logchecker/main.go:168-180`

```go
func nilIfEmpty(s string) interface{} { ... }
func nilIfEmptyStr(s string) interface{} { ... }
```

These two functions are **identical** in body. Keep one (`nilIfEmpty`) and delete the other. Update the single call-site of `nilIfEmptyStr` accordingly.

---

### 1.3 — Unused `strings.NewReplacer` in `htmlToConsole`

**File:** `cmd/logchecker/main.go:151-152`

```go
re := strings.NewReplacer()
_ = re
```

A `strings.NewReplacer` is created, assigned, then immediately suppressed. Remove both lines.

---

### 1.4 — Dead `_ = i` statement in log cleaning loop

**File:** `parser_legacy.go:126`

```go
_ = i
```

This `_ = i` statement inside the loop body has no effect — `i` is already used by the `for` loop variable. Remove it.

---

### 1.5 — Redundant `c2PointersEacPre99Callback` logic

**File:** `callbacks.go:148-157`

```go
func (lc *Logchecker) c2PointersEacPre99Callback(m []string) string {
    cls := "good"
    if strings.ToLower(m[1]) == "no " {
        cls = "good"  // ← redundant: already "good"
    } else {
        cls = "bad"
        lc.account(...)
    }
```

The first branch (`"no "`) re-assigns `cls` to `"good"`, which is the same as the initial value. Simplify to just check for the `else` case.

---

### 1.6 — Unused `decoder` interface

**File:** `internal/util/encoding.go:148-151`

```go
type decoder interface {
    Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error)
    Reset()
}
```

This named `decoder` interface is declared but never referenced — the `decodeWith` function uses an inline anonymous interface. Remove the named type.

---

## 2. Reduce Complexity / Use More Optimal Alternatives

### 2.1 — ⚠️ **Critical: ~80+ inline `regexp.MustCompile` calls inside loops**

**File:** `parser_legacy.go` (throughout the 900-line function body)

This is the **single biggest performance problem** in the codebase. `regexp.MustCompile` compiles a fresh regex on every call. In `parser_legacy.go`, there are **~80 inline `regexp.MustCompile` calls**, many inside the main `for logIdx, rawLog := range lc.logs` loop — meaning they are recompiled on every log session.

**Fix:** Hoist all regex patterns to package-level `var` declarations (like `eacChecksumRe`, `dbVersionRe` etc. already are). Some of the existing pre-compiled regexes at the top of `parser_legacy.go` already demonstrate the correct pattern.

**Example hotspots:**

| Line | Pattern | Notes |
|------|---------|-------|
| 63 | `regexp.MustCompile(...)` for "End of status report" | Called once per parse, easily hoisted |
| 67 | `regexp.MustCompile(...)` for CUETools DB Plugin | Called per log segment in a loop |
| 78 | `regexp.MustCompile(...)` for dash-only lines | Called per log segment in a loop |
| 81 | `regexp.MustCompile(...)` for "End of status report" | Called per log segment |
| 120 | `regexp.MustCompile(...)` for XLD SIGNATURE | Called per segment |
| 222-223 | Two `MustCompile` calls for MP3 detection | Called per log session |
| 235-378 | ~40 `MustCompile` calls for setting verification | Called per log session |
| 405-442 | ~15 `MustCompile` calls for TOC/AR/status lines | Called per log session |
| 495-774 | ~30 `MustCompile` calls in track body parsing | Called per track per session |

**Similarly in `parser_dbpoweramp.go`:**

| Line | Pattern | Notes |
|------|---------|-------|
| 50 | `regexp.MustCompile(...)` for dBpoweramp version | Duplicates `dbVersionRe` |
| 193 | `regexp.MustCompile(...)` for lossy encoder check | Called per parse |
| 203-208 | Section header regexes | Called per parse |
| 280-291 | Track status regexes | Called per track |
| 330, 360-362 | Summary regexes | Called per parse |

**Estimated improvement:** Regex compilation is expensive. For a combined log with 20+ tracks, this could avoid 1,000+ regex compilations.

---

### 2.2 — Duplicated track-end trimming logic (legacy parser)

**File:** `parser_legacy.go:493-556`

The "Individual tracks" block (lines 493-524) and the "Range rip" block (lines 526-556) contain **nearly identical** logic for:
- Finding the track listing
- Iterating backwards through `exploded` lines to find `logEnds`
- Checking with `regexp.MustCompile(...)` (again, inline regex)
- Splitting tracks

Extract a helper function like `findTrackBoundary(exploded []string, logEnds []string) int` to deduplicate this.

---

### 2.3 — Duplicated "Rip suffix" IIFE in `testCopyCallback` / `testCopyXldCallback`

**File:** `callbacks.go:268-273, 288-293`

Both callbacks contain the exact same inline function:
```go
func() string {
    if lc.combined > 0 {
        return fmt.Sprintf(" (%d) ", lc.currLog)
    }
    return ""
}()
```

Extract to a method: `func (lc *Logchecker) combinedSuffix() string`.

---

### 2.4 — Repeated `xldStatCallback` error capping pattern

**File:** `callbacks.go:354-422`

Inside `xldStatCallback`, the four major cases (`"read error"`, `"skipped (treated as error)"`, `"inconsistency in error sectors"`, `"damaged sector count"`) all share the same structure:

```go
cls := "good"
if n > 0 {
    cls = "bad"
    err := n
    if err > 10 {
        err = 10
    }
    // ... pluralization ...
    lc.accountTrack(msg, err)
}
return "<span ...>" + text + "</span> <span ...>" + m[3] + "</span>"
```

Extract a helper: `func (lc *Logchecker) xldErrorStat(text, label string, n int, pluralSuffix string) string`.

---

### 2.5 — `min3` function can use built-in `min` (Go 1.21+)

**File:** `drive.go:69-80`

```go
func min3(a, b, c int) int { ... }
```

Since Go 1.21, `min(a, b, c)` is a built-in. Check the project's minimum Go version (go.mod) — if it targets 1.21+, replace with `min(a, b, c)`.

---

### 2.6 — Levenshtein uses O(n*m) memory

**File:** `drive.go:37-67`

The current implementation allocates a full `[][]int` matrix. For ~500 drives with names averaging ~30 chars, this creates 500 small allocations per parse. Use a single-row DP approach (O(min(n,m)) memory) for a modest but free improvement.

---

## 3. Extract Commonly Used `lc.account` Values & HTML Outputs Into Variables

### 3.1 — Common `lc.account` parameter patterns

The vast majority of `lc.account` calls use `setScore=-1, inclCombined=false, notice=false`:

```go
lc.account("...", N, -1, false, false)   // ~50+ call sites
```

**Suggestion:** Create convenience methods to reduce boilerplate:

```go
// accountDeduction is the common case: deduct points, no combined/notice.
func (lc *Logchecker) accountDeduction(msg string, decrease int) {
    lc.account(msg, decrease, -1, false, false)
}

// accountNotice is for informational messages.
func (lc *Logchecker) accountNotice(msg string) {
    lc.account(msg, 0, -1, false, true)
}

// accountFatal sets the score to a specific value (usually 0).
func (lc *Logchecker) accountFatal(msg string, setScore int) {
    lc.account(msg, 0, setScore, false, false)
}
```

This would simplify many call sites. For example:
```go
// Before:
lc.account("Could not verify used drive", 1, -1, false, false)
// After:
lc.accountDeduction("Could not verify used drive", 1)

// Before:
lc.account("Could not determine language. Assuming English.", 0, -1, false, true)
// After:
lc.accountNotice("Could not determine language. Assuming English.")
```

---

### 3.2 — Repeated HTML span construction patterns

Throughout `callbacks.go` and the parsers, the same HTML template is constructed dozens of times:

```go
"<span class=\"log5\">" + label + m[1] + "</span>: <span class=\"" + cls + "\">" + m[2] + "</span>"
```

**Suggestion:** Create span-builder helpers:

```go
func spanClass(cls, content string) string {
    return `<span class="` + cls + `">` + content + `</span>`
}

func settingLine(label, spacing, cls, value string) string {
    return spanClass("log5", label+spacing) + ": " + spanClass(cls, value)
}
```

This would simplify all callbacks in `callbacks.go`. Example:
```go
// Before:
return "<span class=\"log5\">Utilize accurate stream" + m[1] + "</span>: <span class=\"" + cls + "\">" + m[2] + "</span>"
// After:
return settingLine("Utilize accurate stream", m[1], cls, m[2])
```

---

### 3.3 — Repeated CSS class strings

The class name strings `"good"`, `"bad"`, `"badish"`, `"goodish"`, `"log4"`, `"log5"`, `"log3"`, `"log1"` appear hundreds of times as raw strings. Define them as constants:

```go
const (
    cssGood    = "good"
    cssBad     = "bad"
    cssBadish  = "badish"
    cssGoodish = "goodish"
    cssLog1    = "log1"
    cssLog3    = "log3"
    cssLog4    = "log4"
    cssLog5    = "log5"
)
```

This prevents typos and enables easy global CSS class renaming.

---

### 3.4 — Repeated drive validation messages

The "Incorrect read offset for drive..." and "drive not found in database, offset is 0..." messages appear **identically** in 3 places:

1. `callbacks.go:173-176` (`readOffsetCallback`)
2. `parser_whipper.go:127-138` (`whipperParse`)
3. `parser_dbpoweramp.go:104-120` (`dbpowerampParse`)

**Suggestion:** Extract these into named constants or a shared helper:

```go
func (lc *Logchecker) accountIncorrectOffset() {
    lc.accountDeduction(
        "Incorrect read offset for drive. Correct offsets are: "+
            strings.Join(lc.offsets, ", ")+
            " (Checked against the following drive(s): "+
            strings.Join(lc.drives, ", ")+")", 5)
}

func (lc *Logchecker) accountZeroOffsetUnknownDrive() {
    lc.accountDeduction(
        "The drive was not found in the database, so we cannot determine the correct read offset. "+
            "However, the read offset in this case was 0, which is almost never correct. "+
            "As such, we are assuming that the offset is incorrect", 5)
}
```

---

### 3.5 — "Virtual drive used" message duplicated 3 times

Same `lc.account("Virtual drive used: "+..., 20, -1, false, false)` in:
- `callbacks.go:18`
- `parser_whipper.go:108`
- `parser_dbpoweramp.go:87`

Extract to a shared method.

---

## 4. Structural Refactoring

### 4.1 — Split `parser_legacy.go` (900 lines, single function ~750 lines)

The `legacyParse()` function is ~750 lines long. It handles EAC + XLD parsing interleaved, which makes it hard to follow.

**Suggested split:**

| New File / Section | Responsibility |
|---|---|
| `legacy_setup.go` | Language detection, log splitting/cleaning, checksum determination |
| `legacy_settings.go` | Drive, read mode, audio cache, C2, offset, gap handling, etc. |
| `legacy_tracks.go` | Track parsing, CRC checks, AR annotations, error detection |
| `legacy_postprocess.go` | Track scoring, log reassembly, combined-log handling |

Or at minimum, extract the inner `for logIdx, rawLog := range lc.logs` body into a method like `lc.legacyParseSession(logIdx int, rawLog string) string`.

---

### 4.2 — Unify drive validation logic across parsers

Drive validation (check fake drives -> lookup in DB -> check offset -> annotate) is implemented **3 separate times**:

1. `callbacks.go` + `parser_legacy.go` (via `driveCallback` + `readOffsetCallback`)
2. `parser_whipper.go:99-145`
3. `parser_dbpoweramp.go:62-134`

The core logic is the same but the annotation format differs per ripper. Refactor into a shared method:

```go
type driveResult struct {
    driveClass  string
    offsetClass string
    driveName   string // possibly annotated
    isFake      bool
}

func (lc *Logchecker) validateDrive(driveName, offset string) driveResult { ... }
```

Each parser then only needs to handle its own HTML annotation format.

---

### 4.3 — `driveEntry` type should be a proper struct

**File:** `logchecker.go:27`

```go
type driveEntry [2]interface{}
```

This uses a fixed-size array of `interface{}` which requires type assertions everywhere. Replace with:

```go
type driveEntry struct {
    Name   string
    Offset string // or int — normalize at parse time
}
```

This eliminates the scattered `entry[0].(string)`, `entry[1]` type-switch pattern in `drive.go` and `logchecker.go`.

---

### 4.4 — Pre-compile regexes inside `legacyParse` loop for track processing

The inner track loop (lines 558-791) calls `regexp.MustCompile` for identical patterns on every track. These should be compiled once before the loop.

At minimum, all the XLD stat regexes (lines 615-658) should be pre-compiled into a slice:

```go
type xldStatPattern struct {
    re        *regexp.Regexp
    condition func() bool  // when to accountTrack if cnt==0
    errorMsg  string
}
```

---

## 5. Miscellaneous Improvements

### 5.1 — Duplicate score-formatting logic in `account` / `accountTrack`

**File:** `scoring.go:21-36, 57-65`

Both functions contain identical pluralization logic:

```go
if decrease == 1 {
    append2 = fmt.Sprintf(" (-%d point)", decrease)
} else {
    append2 = fmt.Sprintf(" (-%d points)", decrease)
}
```

Extract to a helper:

```go
func pointsStr(n int) string {
    if n == 1 {
        return fmt.Sprintf(" (-%d point)", n)
    }
    return fmt.Sprintf(" (-%d points)", n)
}
```

---

### 5.2 — Numeric sort helper is duplicated

**File:** `parser_whipper.go:460-467, 485-492` and `parser_legacy.go:850-854`

The same numeric sort lambda appears 3 times:

```go
sort.Slice(keys, func(i, j int) bool {
    ni, err1 := strconv.Atoi(keys[i])
    nj, err2 := strconv.Atoi(keys[j])
    if err1 == nil && err2 == nil {
        return ni < nj
    }
    return keys[i] < keys[j]
})
```

Extract: `func sortNumericStrings(keys []string)`.

---

### 5.3 — `checkTracks` resets `lc.details` to nil

**File:** `parser_legacy.go:878-891`

```go
func (lc *Logchecker) checkTracks(logIdx int) {
    if len(lc.tracks[logIdx]) == 0 {
        lc.details = nil   // ← destructive: discards all prior details
```

This silently discards all details accumulated so far. This seems intentional (matching PHP behavior) but is fragile and undocumented. Add a comment explaining why, or consider whether this should append rather than reset.

---

### 5.4 — `encoding_maccentraleurope.log` hardcoded filename hack

**File:** `parser_legacy.go:869-871`

```go
if strings.Contains(lc.logPath, "encoding_maccentraleurope.log") {
    lc.log = strings.Replace(lc.log, "Feelin'", "Feeliní", 1)
}
```

This looks like a test-specific workaround that leaked into production code. Consider moving this fixup into the test infrastructure instead.

---

## Summary of Impact

| Category | Items | Estimated Impact |
|----------|-------|-----------------|
| Dead code removal | 6 items | Cleaner codebase, removes confusion |
| Regex pre-compilation | ~80 inline `MustCompile` calls | **Major** performance improvement |
| `account` helpers | 3 new convenience methods | ~50+ simplified call sites |
| HTML span helpers | 2-3 builder functions | ~40+ simplified call sites |
| Shared drive validation | 1 unified method | Removes ~120 lines of triplicated code |
| `parser_legacy.go` split | 4 logical sections | 900-line file -> 4 manageable files |
| Other deduplication | ~5 helper extractions | ~50 LOC saved, consistency |

---

## Recommended Order of Implementation

1. **Low-risk, high-impact first:** Hoist all inline `regexp.MustCompile` to package-level vars
2. **Dead code removal:** Items in Section 1 (safe, won't change behavior)
3. **Helper extraction:** `spanClass`, `settingLine`, `accountDeduction`, `pointsStr`
4. **Shared logic:** Drive validation unification, `combinedSuffix`
5. **Structural split:** Break up `parser_legacy.go` (save for last — highest risk of regressions)

> **IMPORTANT:** After each step, run `go test ./...` to verify no regressions against the existing fixture-based test suite.
