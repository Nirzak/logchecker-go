# AccurateRip Disc ID Extraction & Database Verification

Compute or extract AccurateRip disc ID from rip logs, then optionally verify disc presence in the AccurateRip database via HTTP lookup.

## User Review Required

> [!IMPORTANT]
> **Library purity vs HTTP calls**: The project's AGENTS.md states _"Never add external HTTP calls or filesystem side effects to the library."_ The AccurateRip database lookup requires an HTTP GET. I propose placing the HTTP verification in a **new public package `accuraterip/`** (not inside `logchecker/` or `internal/`), keeping the core library pure. The `internal/toc/` package gets pure-computation disc ID methods, while the new `accuraterip/` package handles network I/O. Does this structure work for you?

> [!IMPORTANT]
> **dBpoweramp DiscID extraction**: dBpoweramp logs already embed the AccurateRip DiscID in each track line (e.g., `[DiscID: 009-000f105c-006e4f61-8a0a4209-1]`). Should we extract this from the log text as a first-class data source, or always prefer computing from TOC? I recommend: **extract if present, compute as fallback**, since the log-embedded ID is authoritative.

## Open Questions

> [!IMPORTANT]
> **AccurateRip database access policy**: AccurateRip is technically a proprietary database maintained by Illustrate Ltd. Open-source tools (CUETools, ARver, whipper) query it via the documented HTTP URL format. Are you comfortable with this approach, or should the lookup be opt-in / behind a flag?

## AccurateRip Disc ID Algorithm

From the official dBpoweramp C++ source (forum.dbpoweramp.com):

```
TrackCount = number of audio tracks
Offsets[] = [track1_LBA+150, track2_LBA+150, ..., leadout_LBA+150]

TrackOffsetsAdded   = sum(Offsets[i]) for all i (tracks + leadout)
TrackOffsetsMultiplied = sum(max(Offsets[i], 1) * (i+1)) for all i
FreedBIdent         = CDDB disc ID (already computed in toc.TOC)

Full ID string: "%03d-%08x-%08x-%08x" % (TrackCount, ID1, ID2, CDDB)
```

**URL format**:
```
http://www.accuraterip.com/accuraterip/X/Y/Z/dBAR-NNN-IIIIIIII-JJJJJJJJ-KKKKKKKK.bin
```
Where X, Y, Z are the last 3 hex chars of ID1 (positions [7], [6], [5] in `%08x`).

**Binary response** (per pressing):
- 1-byte track count + 4-byte ID1 + 4-byte ID2 + 4-byte CDDB (13 bytes header)
- Per track: 1-byte confidence + 4-byte CRCv1 + 4-byte CRCv2 (9 bytes each)
- Multiple pressings concatenated

---

## Proposed Changes

### Internal TOC Package — Pure Computation

#### [MODIFY] [toc.go](file:///Users/nirjas/study/logchecker-go/internal/toc/toc.go)
Add AccurateRip disc ID computation methods to `TOC`:
- `AccurateRipDiscID1() uint32` — sum of all offsets (+150 lead-in)
- `AccurateRipDiscID2() uint32` — sum of `max(offset+150, 1) * (i+1)`
- `AccurateRipID() string` — full `"NNN-ID1-ID2-CDDB"` string
- `AccurateRipURL() string` — database lookup URL

These are pure CPU computations, no I/O — consistent with the existing `MusicBrainzDiscID()` pattern.

---

### New AccurateRip Package — HTTP Verification

#### [NEW] [accuraterip/accuraterip.go](file:///Users/nirjas/study/logchecker-go/accuraterip/accuraterip.go)
Public package for AccurateRip database interaction:

```go
package accuraterip

// Status represents the result of an AccurateRip database lookup.
type Status string

const (
    StatusFound    Status = "found"
    StatusNotFound Status = "not_found"
    StatusError    Status = "error"
)

// Result holds the outcome of an AccurateRip lookup.
type Result struct {
    DiscID string // full AR disc ID string (NNN-ID1-ID2-CDDB)
    URL    string // the URL queried
    Status Status
    // Per-pressing data (if found)
    Pressings []Pressing
}

// Pressing holds data for one disc pressing from the AR database.
type Pressing struct {
    Tracks []TrackResult
}

// TrackResult holds per-track AR verification data.
type TrackResult struct {
    Confidence int
    CRCv1      uint32
    CRCv2      uint32
}

// Lookup queries the AccurateRip database for the given TOC.
// Returns the disc ID and whether the disc was found.
func Lookup(t *toc.TOC) (*Result, error)

// LookupWithContext is like Lookup but accepts a context for cancellation.
func LookupWithContext(ctx context.Context, t *toc.TOC) (*Result, error)
```

This package:
- Constructs the AR URL from `toc.AccurateRipURL()`
- Performs HTTP GET
- Parses the binary response
- Returns structured results

---

### Logchecker Package — Integration

#### [MODIFY] [logchecker.go](file:///Users/nirjas/study/logchecker-go/logchecker/logchecker.go)
- Add `GetAccurateRipID() string` public getter — returns AR disc ID from log extraction or TOC computation
- Add `accurateRipID string` field to struct, reset in `reset()`

#### [MODIFY] [parser_dbpoweramp.go](file:///Users/nirjas/study/logchecker-go/logchecker/parser_dbpoweramp.go)
- Extract AR DiscID from existing `[DiscID: NNN-ID1-ID2-CDDB-tracknum]` regex matches
- Set `lc.accurateRipID` (strip trailing `-tracknum`)

#### [MODIFY] [logchecker.go](file:///Users/nirjas/study/logchecker-go/logchecker/logchecker.go) (in `GetAccurateRipID`)
- If `lc.accurateRipID` was extracted from log → return it
- Else if `lc.cdToc != nil` → compute via `lc.cdToc.AccurateRipID()`
- Else → return `""`

---

### Tests

#### [MODIFY] [internal/toc/toc_test.go](file:///Users/nirjas/study/logchecker-go/internal/toc/toc_test.go)
- Add `TestAccurateRipDiscID` with the sample TOC from the dBpoweramp forum
- Verify against known dBpoweramp log fixture (`009-001c3d4e-01f2a3b4-7c08d509` from Ultra Perfect Rip.log)
- Verify URL format

#### [NEW] [accuraterip/accuraterip_test.go](file:///Users/nirjas/study/logchecker-go/accuraterip/accuraterip_test.go)
- Test binary response parsing with crafted test data
- Test URL construction
- Integration test (skipped by default) that hits real AR database

#### [MODIFY] [logchecker_test.go](file:///Users/nirjas/study/logchecker-go/logchecker_test.go)
- Add `TestAccurateRipIDExtraction` verifying `GetAccurateRipID()`:
  - dBpoweramp log: extracted from DiscID field
  - EAC/XLD/whipper: computed from TOC
  - EAC without TOC: returns empty

---

## Verification Plan

### Automated Tests
```bash
go test -v -run TestAccurateRip ./internal/toc/...
go test -v -run TestAccurateRip ./accuraterip/...
go test -v -run TestAccurateRipID ./...
go test ./...  # full regression
```

### Manual Verification
- Cross-check computed AR disc ID from `Ultra Perfect Rip.log` dBpoweramp fixture against log's DiscID field: `009-001c3d4e-01f2a3b4-7c08d509`
- Cross-check computed AR disc ID from `Standard Accurate Rip Ultra Disabled 2.log`: `009-000f105c-006e4f61-8a0a4209`
- Verify generated URL resolves to valid AccurateRip path structure
