# dBpoweramp Scoring Logic for Logchecker

## Philosophy

The existing checkers start at **100** and deduct for bad settings. dBpoweramp has no embedded
checksum (unlike EAC/XLD/Whipper), so `checksum_status` is always `CHECKSUM_MISSING` — same
treatment as old EAC logs. No additional deduction for this since it's a platform limitation,
not user error.

---

## Settings Block Deductions

Parsed once from the `Drive & Settings` header.

| Check | Condition | Deduction | Rationale |
|---|---|---|---|
| **C2 error detection** | `Using C2: Yes` | **-10** | Same as EAC `c2Pointers` (-10). C2 is unreliable on many drives and can mask errors |
| **FUA Cache Invalidate** | `FUA Cache Invalidate: No` | Notice only | dBpoweramp's equivalent of "Defeat audio cache". Flagged as `[Notice]` pending community consensus — `No` is the default even in good rips |
| **Drive offset = 0 (drive not in DB)** | `Drive offset: 0` and drive unknown | **-5** | Same as EAC/Whipper `readOffset` logic. 0 is almost never correct |
| **Drive offset incorrect (drive found)** | Offset doesn't match known offsets for drive | **-5** | Incorrect offset confirmed via `drives.json` cross-reference |
| **Max re-reads too low** | `Maximum Re-reads:` < 10 | **-5** | Mirrors XLD `maxRetryCount`. dBpoweramp's re-read count is critical for secure ripping |
| **Lossy encoder** | `Encoder:` contains `MP3` or similar lossy format | **-100** (score → 0) | Same as EAC MP3 log handling |

---

## Per-Track Deductions

Parsed from the `Extraction Log` section for each track.

| Track Status | Deduction | Rationale |
|---|---|---|
| `AccurateRip: Accurate` | 0 | Perfect |
| `AccurateRip: Accurate (confidence N)` | 0 | Any AccurateRip Accurate result is perfect regardless of confidence level |
| `Secure (Warning)` (Ultra mode) | **-1/track** | Re-rips occurred, data still intact but not clean. Minor penalty |
| `Secure (Warning)` + re-rip frame count > 16 | **-2/track** | Escalate for significant re-rip counts |
| `AccurateRip: Not in Database` | 0 (notice only) | Cannot verify but not user's fault — same as EAC "Track not present in AccurateRip database" (`badish` display, no deduction) |
| `AccurateRip: Inaccurate` | **-10/track** | Hard failure — data integrity cannot be confirmed. Mirrors EAC treatment of AR-failed tracks |
| No AR result line at all | **-5/track** | AR was supposedly active but no result logged |

---

## Summary Line Cross-Check

Parse the final summary line, e.g. `8 Tracks Ripped: 7 Secure, 1 Inaccurate`.

- If the count of `Inaccurate` tracks here doesn't match per-track parsing → flag as `[Notice] Summary inconsistency`
- If summary says 0 tracks ripped → score 0

---

## Ultra Mode

Ultra mode itself is **not penalized** — it is a quality-improving feature (more passes). The
presence of `Ultra::` in the settings header means:

- Track status changes from `AccurateRip: Accurate [Pass 1]` to `Secure [Pass 1 & 2, Ultra X to Y]`
- Extra passes (`Ultra 1 to 3`) are normal and expected on worn discs
- Only `Secure (Warning)` warrants a deduction (see per-track table above)

---

## Version Check

dBpoweramp logs the version as `dBpoweramp Release 16.3` etc.

- No known minimum safe version to enforce
- Versions < 14 should be flagged as `[Notice]` since older versions had less robust secure ripping
- No score deduction for version

---

## Full Scoring Summary

```
Start:                                      100

Settings deductions:
  C2 used:                                  -10
  FUA Cache Invalidate off:                 [Notice only — no deduction, pending research]
  Drive offset 0 / incorrect:               -5
  Max Re-reads < 10:                        -5
  Lossy encoder (MP3):                      → score 0

Per-track deductions (accumulated):
  AR Accurate (any confidence):              0 (perfect)
  AR Inaccurate:                            -10/track
  Secure (Warning):                         -1/track
  Secure (Warning) + re-rip > 16 frames:    -2/track
  No AR result:                             -5/track
  AR Not in Database:                       [Notice only — no deduction]

Checksum:                                   CHECKSUM_MISSING (always, no penalty)
```

---

## PHP Implementation Notes

### Detection (in `src/Check/Ripper.php`)

Add `DBPOWERAMP` constant and detection before the `UNKNOWN` fallback:

```php
public const DBPOWERAMP = 'dBpoweramp';

// In getRipper():
if (preg_match('/^dBpoweramp Release/i', $log)) {
    return Ripper::DBPOWERAMP;
}
```

### Dispatcher (in `src/Logchecker.php`)

Add dBpoweramp branch in `parse()`:

```php
if ($this->ripper === Ripper::DBPOWERAMP) {
    $this->dbpowerampParse();
} elseif ($this->ripper === Ripper::WHIPPER) {
    $this->whipperParse();
} else {
    $this->legacyParse();
}
```

### Parser structure (`src/Checker/Dbpoweramp.php` or inline method)

The parser is simpler than EAC — no per-track `Test CRC / Copy CRC`, no gap handling, no
accurate stream checks. Key parsing flows:

```php
// Always missing — dBpoweramp has no embedded checksum
$this->checksumStatus = Checksum::CHECKSUM_MISSING;

// Settings (single pass, regex against header block):
// - Drive offset
// - Using C2
// - FUA Cache Invalidate
// - Maximum Re-reads
// - Encoder line

// Track loop (per-track regex):
// - "AccurateRip: Accurate (confidence N)"  → check N < 2
// - "AccurateRip: Inaccurate"
// - "AccurateRip: Not in Database"
// - "Secure (Warning)"                       → extract frame count from "[..., Re-Rip N Frames]"
// - "Secure [Pass 1 & 2, ...]"               → clean pass, no deduction
```

Roughly 40% of the complexity of the EAC parser due to absence of checksum, gap handling,
and mode checks.

---

## Log Format Reference

### Standard mode track (good):
```
Track 1:  Ripped LBA 0 to 26148 (5:48) in 1:20. Filename: ...\01 - Hale Dil._
  AccurateRip: Accurate (confidence 9)     [Pass 1]
  CRC32: E659F4DA     AccurateRip CRC: DE0DB418 (CRCv2)     [DiscID: ...]
  AccurateRip Verified Confidence 9 [CRCv2 de0db418]
```

### Ultra mode track (good):
```
Track 1:  Ripped LBA 0 to 17584 (3:54) in 1:45. Filename: ...\01 - Track._
  Secure  [Pass 1 & 2, Ultra 1 to 1]
  CRC32: 7CE6BC4B     AccurateRip CRC: A501E52C (CRCv2)     [DiscID: ...]
```

### Ultra mode track (warning):
```
Track 7:  Ripped LBA 111230 to 129714 (4:06) in 5:03. Filename: ...\07 - Track._
  Secure (Warning)  [Pass 1 & 2, Ultra 1 to 3, Re-Rip 16 Frames]
  CRC32: 97BF4205     AccurateRip CRC: 594C690E (CRCv2)     [DiscID: ...]
```

### Settings header (standard mode):
```
Ripping with drive 'D:   [HL-DT-ST - DVDRW  GX50N    ]',  Drive offset: 6,  Overread Lead-in/out: No
AccurateRip: Active,  Using C2: No,  Cache: 1024 KB,  FUA Cache Invalidate: No
Pass 1 Drive Speed: Max,  Pass 2 Drive Speed: Max
Bad Sector Re-rip::  Drive Speed: Max,  Maximum Re-reads: 34
```

### Settings header (Ultra mode):
```
Ripping with drive 'H:   [PLDS     - DVD+-RW DS-8A5SH]',  Drive offset: 6,  Overread Lead-in/out: No
AccurateRip: Active,  Using C2: No,  Cache: 1024 KB,  FUA Cache Invalidate: No
Pass 1 Drive Speed: Max,  Pass 2 Drive Speed: Max
Ultra::  Vary Drive Speed: Yes,  Min Passes: 1,  Max Passes: 3,  Finish After Clean Passes: 2
Bad Sector Re-rip::  Drive Speed: Max,  Maximum Re-reads: 34
```

---

## Test Log Coverage

| File | Mode | AR Result | Expected Score |
|---|---|---|---|
| `test_01_perfect_standard.log` | Standard | All Accurate, confidence 21-23 | 100 |
| `test_02_ultra_mixed_results.log` | Ultra | Mix: Secure, Warning, Not in DB, Inaccurate | ~75 |
| `test_03_bad_settings_inaccurate.log` | Standard | 6 Inaccurate, 1 Not in DB, bad settings | ~5 |
| `test_04_ultra_perfect.log` | Ultra | All Secure, AR verified confidence 14-15 | 100 |
| `Murder_2___Rip_LOG.txt` | Standard | All Accurate, mixed confidence (2-10) | ~97 |
| `log-without-ultra.log` | Standard | All Accurate, confidence 4 | 100 |
| `log-with-ultra.log` | Ultra | 14 Secure, 1 Secure (Warning), no AR DB match | ~99 |