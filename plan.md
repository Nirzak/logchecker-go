# TOC Parser & Disc ID Calculation

Parse CD Table of Contents from rip logs → calculate MusicBrainz, FreeDB, and CTDB identifiers → expose via public API with lookup/attach URLs.

## TOC Format per Ripper

### EAC / XLD (legacy parser)
Both share identical tabular TOC format:
```
TOC of the extracted CD

     Track |   Start  |  Length  | Start sector | End sector
    ---------------------------------------------------------
        1  |  0:00.00 |  3:45.27 |         0    |    16901
        2  |  3:45.27 |  2:45.73 |     16902    |    29349
```
- Start sector = LBA offset (0-based, no +150 lead-in)
- End sector = last sector of track
- Lead-out = end sector of last track + 1

> [!IMPORTANT]
> EAC older logs (e.g. `en_1.log`) have **no TOC section**. TOC extraction must be optional — if absent, `GetTOC()` returns nil and all disc IDs return empty.

### Whipper
YAML structure:
```yaml
TOC:
  1:
    Start: 00:00:00
    Length: 00:50:25
    Start sector: 0
    End sector: 3774
```
Also provides `CDDB Disc ID`, `MusicBrainz Disc ID`, and `MusicBrainz lookup URL` in the `CD metadata` section. We will **compute these ourselves** from the TOC for consistency + expose the pre-existing whipper values when present.

### dBpoweramp
No explicit TOC table. Track LBAs embedded in each track line:
```
Track 1:  Ripped LBA 0 to 19832 (4:24) in 1:08.
Track 2:  Ripped LBA 19832 to 41274 (4:46) in 1:14.
```
- Start LBA and end LBA per track (0-based)
- Lead-out = end LBA of last track + 1

---

## Disc ID Algorithms

### 1. MusicBrainz Disc ID
**Input**: first track (1), last track, lead-out offset (+150), track offsets (+150)
**Algorithm**:
1. Build string: `sprintf("%02X", first)` + `sprintf("%02X", last)` + `sprintf("%08X", leadout+150)`
2. For i = 1..99: `sprintf("%08X", offset[i]+150)` (0 for nonexistent tracks)
3. SHA-1 hash → custom Base64 (`+/=` → `._ -`)

**TOC string** (for attach URL): `1+N+leadout+150 offset1+150 offset2+150 ...`

**URLs**:
- Lookup: `https://musicbrainz.org/cdtoc/attach?toc=TOC_STRING&tracks=N&id=DISC_ID`
- If whipper has release URL: expose that too

### 2. FreeDB / CDDB Disc ID
**Input**: track offsets (+150), lead-out offset (+150), track count
**Algorithm**:
1. For each track: `digitSum(offset / 75)` (where offset includes +150)
2. `totalSeconds = (leadout+150) / 75 - (firstOffset+150) / 75`
3. `discID = (checksum % 255) << 24 | totalSeconds << 8 | trackCount`
4. Format as 8-char lowercase hex

**URL**: `https://gnudb.com/cd/FREEDB_ID`

### 3. CTDB TOC ID
**Input**: track count, audio lengths (in sectors) for each track
**Algorithm** (from CUETools source):
1. String = `trackCount` + joined track lengths separated by spaces
2. SHA-1 hash → URL-safe Base64 (`+/=` → `-_ `)

**URL**: `https://db.cuetools.net/ui/cd/CTDB_TOCID` (if we can construct it)

> [!WARNING]
> CTDB TOCID algorithm is less well-documented than MB/FreeDB. The implementation below follows the CUETools source (CDImageLayout.TOCID). We need to verify against known whipper logs that contain CTDB data. If we cannot verify the algorithm, we should expose only the TOC and let users look it up manually.

---

## Proposed Changes

### Internal TOC Package

#### [NEW] [toc.go](file:///Users/nirjas/study/logchecker-go/internal/toc/toc.go)
Core `TOC` struct + disc ID calculation:
```go
package toc

type TOC struct {
    FirstTrack int      // always 1
    LastTrack  int      // number of audio tracks
    Offsets    []int    // 0-based LBA start sector per track (index 0 = track 1)
    Leadout    int      // 0-based LBA sector after last track
}

// MusicBrainzDiscID() string — SHA-1 + custom base64
// MusicBrainzTOCString() string — "1 N leadout+150 off1+150 off2+150..."
// MusicBrainzLookupURL() string — attach URL
// FreeDBDiscID() string — 8-char hex
// FreeDBLookupURL() string — gnudb URL
// CTDBTOCString() string — track lengths space-separated
// CTDBDiscID() string — SHA-1 + URL-safe base64 (best-effort)
// CTDBLookupURL() string — db.cuetools.net URL
```

No external deps — only `crypto/sha1`, `encoding/base64`, `fmt`.

---

### Logchecker Package

#### [MODIFY] [logchecker.go](file:///Users/nirjas/study/logchecker-go/logchecker/logchecker.go)
- Add `toc *toc.TOC` field to `Logchecker` struct
- Add `GetTOC() *toc.TOC` public getter
- Reset `toc` in `reset()`

#### [MODIFY] [parser_legacy.go](file:///Users/nirjas/study/logchecker-go/logchecker/parser_legacy.go)
- In `legacyParseSession()`, after TOC annotation, extract start sectors + end sectors from `tocRowRe` matches
- Build `toc.TOC` from extracted data
- Set `lc.toc` (first session only for combined logs)

#### [MODIFY] [parser_whipper.go](file:///Users/nirjas/study/logchecker-go/logchecker/parser_whipper.go)
- After YAML parse of `TOC` section, extract `Start sector` and `End sector` for each track
- Build `toc.TOC` from extracted data
- Set `lc.toc`

#### [MODIFY] [parser_dbpoweramp.go](file:///Users/nirjas/study/logchecker-go/logchecker/parser_dbpoweramp.go)
- Extract LBA start/end from track lines (`Ripped LBA (\d+) to (\d+)`)
- Build `toc.TOC` from extracted data
- Set `lc.toc`

---

### Tests

#### [NEW] [internal/toc/toc_test.go](file:///Users/nirjas/study/logchecker-go/internal/toc/toc_test.go)
Unit tests for disc ID calculations using known TOC data:
- Verify MusicBrainz disc ID against whipper log fixture (`wXcMD4BGh8KcpBCxKY.mfAfc_EY-`)
- Verify FreeDB disc ID against whipper log fixture (`c2058d11`)
- Verify against the MB documentation example (`49HHV7Eb8UKF3aQiNmu1GR8vKTY-`)

#### [MODIFY] [logchecker_test.go](file:///Users/nirjas/study/logchecker-go/logchecker_test.go)
Add `TestTOCExtraction` that:
- Parses selected fixtures from each ripper
- Verifies `GetTOC()` returns non-nil
- Spot-checks track count and sector values
- For whipper `1.log`: verifies computed MB disc ID matches log's `MusicBrainz Disc ID` field

---

## Open Questions

> [!IMPORTANT]
> **CTDB TOCID**: The exact algorithm is embedded in CUETools C# source. Do you want me to implement a best-effort version, or skip CTDB disc ID for now and only provide the TOC string + a manual lookup URL pattern?

> [!IMPORTANT]
> **Combined logs**: For EAC/XLD combined logs with multiple sessions (different CDs), each session may have a different TOC. Should `GetTOC()` return only the first session's TOC, return a slice, or return nil for combined logs?

> [!IMPORTANT]
> **Whipper pre-computed values**: Whipper logs already contain `CDDB Disc ID`, `MusicBrainz Disc ID`, and lookup URLs in the `CD metadata` section. Should we prefer the whipper-provided values or always use our computed values? (I suggest: compute our own, but also expose whipper's raw values via the existing `GetLog()` output.)

---

## Verification Plan

### Automated Tests
```bash
go test -v -run TestTOC ./...
go test -v -run TestTOCExtraction ./...
go test ./...  # ensure no regressions
```

### Manual Verification
- Cross-check computed MusicBrainz disc ID from whipper `1.log` against value in log: `wXcMD4BGh8KcpBCxKY.mfAfc_EY-`
- Cross-check computed FreeDB ID from whipper `1.log` against value in log: `c2058d11`
- Cross-check MB disc ID example from official docs: CD with 6 tracks → `49HHV7Eb8UKF3aQiNmu1GR8vKTY-`
- Verify generated URLs open correctly in browser
