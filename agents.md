# AGENTS.md — logchecker-go

> Guideline for AI agents working in this repository.

---

## 1. Project Context

**Logchecker** is a Go library (and CLI tool) that parses and scores CD-rip log files produced by four rippers:

| Ripper | Constant | Log signature |
|---|---|---|
| Exact Audio Copy (EAC) | `check.EAC` | Contains `"Exact Audio Copy"` |
| X Lossless Decoder (XLD) | `check.XLD` | Contains `"X Lossless Decoder version"` |
| whipper | `check.Whipper` | Contains `"Log created by: whipper"` |
| dBpoweramp | `check.DBpoweramp` | First line matches `^dBpoweramp Release` |

A score starts at **100** and decreases based on problems found (bad settings, checksum failures, CRC mismatches, etc.). The final score and an array of human-readable detail messages are the primary output.

**Stack**
- Language: Go 1.25+ (no CGO, no runtime deps)
- Key deps: `github.com/Nirzak/eac-logchecker`, `github.com/Nirzak/xld-logchecker` (native Go checksum validators), `golang.org/x/text` (encoding detection), `gopkg.in/yaml.v3`
- No web framework, no database, no external services at runtime

**Key architectural decisions**
| Decision | Why |
|---|---|
| `internal/` packages | `check`, `util`, `parser/eac` are not part of the public API; prevents accidental import by consumers |
| `//go:embed` for `drives.json` + language JSONs | Single-binary distribution — no external data files needed at runtime |
| `Logchecker` struct with `reset()` on `NewFile()` | Safe re-use of a single instance across multiple files without allocation overhead |
| `account()` / `accountTrack()` centralize scoring (with `accountDeduction`/`accountNotice`/`accountFatal` convenience wrappers in `scoring.go`) | All score mutations go through one place; prevents duplicate detail messages |
| Ripper-specific parse entry points (`legacyParse`, `whipperParse`, `dbpowerampParse`) | EAC/XLD share legacy logic; whipper and dBpoweramp have divergent formats |
| Fixture-driven tests (`tests/logs/*/details/*.json` + `html/*.log`) | Regression tests against real-world logs with known-good outputs |

---

## 2. Development Commands

```bash
# Setup — requires Go 1.25+
go mod download

# Build CLI binary
go build -o logchecker ./cmd/logchecker/main.go

# Run all tests
go test ./...

# Run tests with verbose output (shows per-fixture subtests)
go test -v ./...

# Run tests for a specific ripper
go test -v -run TestLogchecker/eac ./...

# Run only HTML output regression tests
go test -v -run TestHTMLOutput ./...

# Lint (install golangci-lint first)
golangci-lint run ./...

# Vet
go vet ./...

# Format check
gofmt -l .

# Format in place
gofmt -w .

# CLI usage
logchecker analyze path/to/file.log
logchecker analyze --html path/to/file.log out.html details.json
logchecker decode  path/to/file.log   # detect + dump UTF-8
logchecker translate -l de path/to/file.log
logchecker version
```

**Non-obvious flags**
- `--no_text`: suppress log body output (prints only score summary)
- `--html`: print raw HTML annotation to stdout instead of stripping tags
- Third positional arg to `analyze` writes a `details.json` — this is what the test fixtures are generated from

---

## 3. Code Style & Conventions

### Naming
- Files: `snake_case.go`, prefixed by concern — `parser_legacy.go`, `parser_whipper.go`, `parser_dbpoweramp.go`
- Exported types/functions: `PascalCase` — `Logchecker`, `NewFile`, `GetScore`
- Unexported functions/fields: `camelCase` — `legacyParse`, `driveCallback`, `checksumStatus`
- Constants in `internal/check`: ALL_CAPS-free, plain string constants — `ChecksumOK`, `EAC`, `Whipper`
- Regex vars: named with `Re` suffix, declared at package level as `var` — `eacChecksumRe`, `dbpowerampRe`

### Import order (standard Go goimports grouping)
```go
import (
    // stdlib
    "fmt"
    "regexp"

    // internal
    "github.com/Nirzak/logchecker-go/internal/check"
    "github.com/Nirzak/logchecker-go/internal/util"

    // third-party
    "golang.org/x/text/encoding"
)
```

### Error handling
- Library code (`logchecker/`, `internal/`) **never** calls `os.Exit` or `panic` — return errors up the call stack
- `Parse()` is intentionally error-free at the signature level: on fatal decode/ripper failure it sets `lc.score = 0` and appends to `lc.details` via `account()`
- CLI (`cmd/logchecker/main.go`) is the **only** place that calls `os.Exit(1)` — after printing to `os.Stderr`
- Use sentinel errors for expected failure modes: `check.ErrUnknownRipper`, `eac.ErrUnknownLanguage`, `eac.ErrInvalidFile`
- Error strings: lowercase, no trailing punctuation (`"could not detect log encoding"` not `"Could not detect log encoding."`)

### Logging / output standards
- The library emits **zero** output to stdout/stderr — all feedback goes into `lc.details` via `account()`
- `[Notice]` prefix (via `notice=true` on `account()`) marks informational-only details that do NOT deduct points
- Score detail format: `"Message text (-N point(s))"` — always produced by `account()`, never hand-formatted

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

Stricly maintain the above classes. do not invent new classes

---

## 4. Architecture & Structure

```
logchecker-go/
├── cmd/logchecker/main.go        # CLI only — thin wrapper, no business logic
├── internal/
│   ├── check/
│   │   ├── ripper.go             # GetRipper() — detects ripper from raw text
│   │   └── checksum.go           # Validate() — returns Checksum* constants
│   ├── parser/eac/
│   │   ├── translator.go         # GetLanguage(), Translate() — EAC i18n
│   │   └── languages/            # master.json + per-lang JSON translation maps
│   └── util/encoding.go          # DecodeEncoding() — UTF-16/Latin-1 → UTF-8
├── logchecker/
│   ├── logchecker.go             # Public API: New(), NewFile(), Parse(), Get*()
│   ├── scoring.go                # account*/pointsStr/sortNumericStrings — score mutation + helpers
│   ├── callbacks.go              # HTML annotation callbacks (one per log field)
│   ├── html_helpers.go           # spanClass/settingLine + shared account* helpers (xldErrorStat, drive/offset)
│   ├── css_classes.go            # css* class-name constants (good/bad/log4/log5/…)
│   ├── regex_util.go             # Pure regex helpers: splitWithDelim, splitWithCaptures, replaceCount*, compareVersions
│   ├── parser_legacy.go          # EAC + XLD parse logic (legacyParse → legacyParseSession → legacyParseTracks)
│   ├── parser_whipper.go         # whipper parse logic (YAML unmarshal + re-render)
│   ├── parser_dbpoweramp.go      # dBpoweramp parse logic
│   ├── drive.go                  # getDrives()/validateDrive() — Levenshtein drive lookup
│   └── resources/drives.json     # Embedded drive DB (~6000 entries)
├── scripts/update_drives.go      # Maintenance: regenerate drives.json (not part of the library)
├── logchecker_test.go            # All tests — fixture-driven
└── tests/logs/{eac,xld,whipper,dbpoweramp}/
    ├── originals/                # Input log files (source of truth)
    ├── details/                  # Expected JSON output (score, details, checksum)
    ├── html/                     # Expected HTML-annotated output
    └── utf8/                     # UTF-8 decoded versions (EAC only)
```

### Parse flow (`Parse()`)

```
Parse()
 ├── util.DecodeEncoding(scoringFn)     UTF-16/Latin-1 → UTF-8; scoring callback
 │    └── scoringFn tries eac.GetLanguage() + eac.Translate() per encoding candidate
 │         to pick the best charset — happens before ripper detection
 ├── check.GetRipper()                  Detect ripper; score=0 + return if unknown
 ├── switch ripper
 │    ├── DBpoweramp → dbpowerampParse()   Regex-only; no checksum validation
 │    ├── Whipper    → whipperParse()      YAML unmarshal + field checks
 │    └── default    → legacyParse()       EAC + XLD
 │         ├── [EAC only] eac.GetLanguage() + eac.Translate() → rewrite lc.log to English
 │         ├── util.NormalizeLineEndings()
 │         ├── Split lc.log into lc.logs[] — 3-way branch:
 │         │    ├── EAC checksum present  → split on `==== Log checksum ... ====`
 │         │    ├── XLD signature present → split on `--- BEGIN/END XLD SIGNATURE ---`
 │         │    └── neither (checksumStatus=Missing) → split on `End of status report`
 │         │         └── strip CUETools DB Plugin segments
 │         ├── Per-section loop → legacyParseSession(logIdx, rawLog) (one rip session each):
 │         │    ├── version/ripper detection; [EAC] MP3-rip early-out
 │         │    ├── settings: replaceCountCallback() over ~40 regex patterns
 │         │    │    Each callback: annotates log text with HTML spans
 │         │    │    AND calls account()/accountTrack() on problems found
 │         │    ├── Checksum validation (check.Validate()) if checksumStatus==OK
 │         │    ├── legacyParseTracks(logIdx, rawLog, isEAC, isXLD):
 │         │    │    split track listing (findTrackBoundary), annotate each track
 │         │    │    (filename/CRC/AR/copy), then XLD all-track stats
 │         │    ├── checkTracks(logIdx) — sets score=0 if zero tracks found
 │         │    └── Reset per-session state (arTracks, secureMode)
 │         └── Merge per-session track scores into lc.details + lc.score
```

### Where to add new features
| Feature type | Location |
|---|---|
| New ripper support | Add constant in `internal/check/ripper.go` → new `parser_<ripper>.go` in `logchecker/` → `switch` case in `logchecker.go:Parse()` |
| New scoring rule for existing ripper | Add callback in `callbacks.go` (if HTML annotation needed) or inline in the parser file; call `account()` / `accountTrack()` |
| New CLI subcommand | Add `case` to `main()` switch + `cmdFoo()` function in `cmd/logchecker/main.go` |
| New language support | Add JSON file to `internal/parser/eac/languages/` + entry in `master.json`; `//go:embed` picks it up automatically |
| Encoding edge case | `internal/util/encoding.go` only |

---

## 5. Critical Rules (Non-Negotiable)

1. **Never mutate score outside `account()` / `accountTrack()`** — direct assignment to `lc.score` is only allowed in `reset()` and on unrecoverable parse failure in `Parse()`.

2. **Never add `os.Exit` or `panic` inside `logchecker/` or `internal/`** — these packages are imported as a library.

3. **Never break the public API of `logchecker/` without a major version bump** — `New()`, `NewFile()`, `Parse()`, and all `Get*()` methods are the stable contract.

4. **Never hardcode drive names, score values, or HTML class strings as bare literals in parser files** — drives live in `resources/drives.json`; HTML classes are `good`/`bad`/`badish`/`log4`/`log5`; penalty values must be traceable to the PHP reference implementation.

5. **Never modify test fixtures (`tests/logs/*/details/*.json`, `html/*.log`) to make a failing test pass** — fix the code. Fixtures are the ground truth derived from the PHP reference implementation.

6. **Never add external HTTP calls or filesystem side effects to the library** — the library is pure CPU/memory; `NewFile()` is the only I/O entry point.

7. **`LevenshteinDistance` is a package-level var for testing only** — do not increase it as a workaround for a failing drive lookup.

---

## 6. Testing Requirements

### Structure
- All tests live in `logchecker_test.go` (package `logchecker_test`) — black-box testing of the public API only
- `TestLogchecker`: validates `ripper`, `version`, `language`, `combined`, `score`, `checksum`, `details` against `tests/logs/*/details/*.json`
- `TestHTMLOutput`: validates full annotated HTML output against `tests/logs/*/html/*.log`



### Adding tests for a new log
```bash
# 1. Place the original log
cp new.log tests/logs/eac/originals/new.log

# 2. Generate the expected fixture
logchecker analyze tests/logs/eac/originals/new.log \
  tests/logs/eac/html/new.log \
  tests/logs/eac/details/new.json

# 3. Verify the fixture looks correct, then commit both files
```

### Running targeted tests
```bash
go test -v -run "TestLogchecker/eac/combined_1.log" ./...
go test -v -run "TestHTMLOutput/whipper" ./...
```

### Coverage expectations
- Every new ripper rule (`account()` call) **must** have at least one fixture that exercises it
- All code paths in `internal/` packages must be covered by the fixture suite or explicit unit tests
- `checksum_invalid` fixtures may report `checksum_ok` in environments without the external validator — this is explicitly tolerated in the test harness; do not work around it differently

---

## 7. Agent Behavior Guidelines

### Edits vs new files
- **Prefer editing existing files.** A new scoring rule for EAC goes into `parser_legacy.go` and a callback in `callbacks.go` — not a new file.
- Create a new file only for a genuinely new ripper parser (`parser_<name>.go`) or a new `internal/` utility package.

### Before writing any code
1. Check `regex_util.go` for existing helpers (`splitWithDelim`, `splitWithCaptures`, `replaceCount`, `replaceCountCallback`, `compareVersions`) before implementing string/regex operations.
2. Check `callbacks.go` for an existing callback pattern before writing a new HTML annotation.
3. Check `html_helpers.go` (`spanClass`, `settingLine`) and `css_classes.go` (`css*` constants) before emitting raw `<span class="...">` literals.
4. Check `scoring.go` for the right account helper before calling the low-level `account()`: use `accountDeduction(msg, n)` for a normal penalty, `accountNotice(msg)` for an informational `[Notice]`, `accountFatal(msg, setScore)` to force a score, and `accountTrack(msg, n)` for per-track penalties.
5. Check `internal/check/ripper.go` constants before referencing ripper names as string literals.

### Assumptions
- Never assume a log file is valid UTF-8 — always route raw bytes through `util.DecodeEncoding()`.
- Never assume `lc.ripper` is populated before `Parse()` returns.
- When in doubt about expected score behavior, check `tests/logs/*/details/*.json` for the nearest similar fixture.

### Commit message format
```
<type>(<scope>): <short imperative summary>

Types : feat | fix | refactor | test | docs | chore
Scope : logchecker | cli | eac | xld | whipper | dbpoweramp | internal | tests

Examples:
  feat(eac): penalize logs with null drive offset
  fix(whipper): handle SHA-256 hash on final line without trailing newline
  test(xld): add fixture for XLD combined log with bad CRC
  refactor(internal): extract encoding scorer into named function
```

- Subject line ≤ 72 characters, imperative mood, no trailing period
- Never commit fixture JSON/HTML changes without an accompanying code change that motivated them