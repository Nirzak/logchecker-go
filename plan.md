# logchecker-go — Code Health Plan (Round 2)

> Fresh analysis after the Round 1 optimization/refactor pass (regex hoisting,
> helper extraction, `legacyParse` split into `legacyParse` →
> `legacyParseSession` → `legacyParseTracks`, `driveEntry` struct).
>
> Baseline at time of writing: Go 1.26, `go vet ./...` clean, `gofmt -l .` clean,
> full fixture suite passing. ~3,900 LOC across the non-test sources.
>
> **Overall:** the codebase is in good shape. The items below are modest,
> targeted improvements — there is no critical defect outstanding. Each item
> notes its real-world impact so low-value work can be skipped.

---

## Table of Contents

1. [Dead Code Removal](#1-dead-code-removal)
2. [Performance Optimization](#2-performance-optimization)
3. [Coding Issues](#3-coding-issues)
4. [Security](#4-security)
5. [Verification](#5-verification)

---

## 1. Dead Code Removal

### 1.1 — `lc.arSummary` is write-only state  ⚠️ main finding

**Files:** `logchecker.go:57,127`, `callbacks.go:322-327`, `parser_legacy.go:640,645,669`

The `arSummary map[string]interface{}` field is written but **never read**:

- `callbacks.go:322-327` — sets `arSummary["goodish"]` / `arSummary["good"]` to empty `[]int{}`.
- `parser_legacy.go:640,645` — sets `arSummary["bad"] = cnt`.
- `logchecker.go:127` + `parser_legacy.go:669` — re-initialize it each reset/session.

No code ever reads these values back (verified by grep: the only `arSummary` *read* is an unrelated local string in `parser_whipper.go:255`). The mixed value types (`[]int{}` vs `int`) are a further smell — the map could never be consumed coherently anyway.

**Action:** Remove the `arSummary` field and all writes to it. The surrounding callback logic that sets the CSS class (`cls`) must stay — only the `lc.arSummary[...] = ...` book-keeping lines are dead.

**Risk:** Low. Pure dead-state removal; no behavior change. Fixture suite will confirm. Verify there is no PHP-reference feature that was *intended* to consume this (the PHP origin may have read it) — if so, the correct fix may be to *implement the read* rather than delete. Decide before removing.

**Impact:** Removes confusing write-only state and one `interface{}` map.

---

## 2. Performance Optimization

### 2.1 — `fixFieldRe` recompiled per whipper parse

**File:** `parser_whipper.go:30`

```go
fixFieldRe := regexp.MustCompile(`  (Release|Album): (.+)`)
```

This is the **only** remaining inline `regexp.MustCompile` in the codebase (Round 1 hoisted the rest). It compiles on every `whipperParse()` call. It is called once per parse — *not* per track or per line — so the cost is small. But hoisting it to a package-level `var` (next to `whipperVersionRe`, `crcRe`, etc.) is trivial and matches the established convention.

**Action:** Promote to package-level `var fixFieldRe = regexp.MustCompile(...)`.

**Risk:** Trivial. **Impact:** Minor; mostly consistency.

> Note: the rest of the hot paths are clean. No `regexp.MustCompile` inside loops
> anywhere. No `strings.Replace` quadratic patterns spotted. Levenshtein is
> already single-row DP (Round 1). No further perf items found worth doing.

---

## 3. Coding Issues

### 3.1 — `arSummary` typing (folds into 1.1)

If 1.1 is resolved by deletion, this is moot. If the map is kept for a future
read, it should be a concrete type (e.g. `map[string][]int` or a small struct)
instead of `map[string]interface{}` so writers can't store incompatible shapes.

### 3.2 — CLI file-existence check misses non-ENOENT stat errors

**File:** `cmd/logchecker/main.go:71`

```go
if _, err := os.Stat(file); os.IsNotExist(err) {
    fmt.Fprintln(os.Stderr, "Invalid file")
    os.Exit(1)
}
```

Only `IsNotExist` is handled. A permission error or a path that is a directory
passes this guard and fails later with a less clear message. Low priority — the
subsequent `os.ReadFile` still errors out — but the early check could report the
real error.

**Action (optional):** Replace with a check that surfaces any `err != nil`, or
drop the pre-check and rely on the `ReadFile` error path with a clear message.

**Risk:** Trivial. **Impact:** Slightly better CLI error messages. CLI-only.

---

## 4. Security

> Context: this is a local CLI/library that parses user-supplied log files. No
> network, no untrusted remote input, no auth surface. Threat model is narrow.

### 4.1 — No upper bound on input file size  (hardening, low priority)

**Files:** `logchecker.go:98`, `cmd/logchecker/main.go:188,217`, `internal/check/checksum.go:75`

`os.ReadFile` loads the entire file into memory with no size cap. A pathologically
large file could exhaust memory. For a user running the tool on their own logs
this is near-zero risk; it only matters if logchecker is ever embedded in a
service that accepts untrusted uploads.

**Action (only if used server-side):** Add a size guard before reading — e.g.
`os.Stat` + reject above a sane limit (a CD-rip log is realistically < ~1 MB), or
use `io.LimitReader`. If the library may run in a server context, do this in the
library (`NewFile`) and document the limit.

**Risk:** Low. **Impact:** DoS hardening for the embedded-service case only.

### 4.2 — Positive findings (no action needed)

- **No ReDoS risk.** Go's `regexp` uses RE2 (linear-time, no catastrophic
  backtracking), so even adversarial input cannot blow up the many regexes.
- **No `panic` / `os.Exit` in `logchecker/` or `internal/`** — library stays
  embeddable; `os.Exit` is confined to the CLI.
- **No command execution, no path traversal surface, no template/HTML injection
  into a live page** — the HTML output is annotation text, not served.
- `validateWhipper`'s `lines[:len(lines)-1]` is safe: it is only reached after a
  successful hash-regex match, which guarantees non-empty content.

---

## 5. Verification

After each change:

```bash
go vet ./...
gofmt -l .            # must print nothing
go test ./...         # full fixture suite must stay green
```

Do not modify fixtures to pass — fixtures are ground truth (see AGENTS.md §5).

---

## Summary of Impact

| Item | Category | Risk | Value |
|---|---|---|---|
| 1.1 `arSummary` write-only state | Dead code | Low | Medium — removes confusing dead state |
| 2.1 Hoist `fixFieldRe` | Perf | Trivial | Low — consistency |
| 3.2 CLI stat-error handling | Coding | Trivial | Low — better CLI messages |
| 4.1 Input size guard | Security | Low | Conditional — only if embedded server-side |

**Recommended order:** 1.1 (decide delete vs implement-read first) → 2.1 → 3.2.
4.1 only if there is a plan to run logchecker against untrusted input.
