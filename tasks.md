# AccurateRip Implementation Tasks

- [x] Add AR disc ID computation to `internal/toc/toc.go`
- [x] Add AR computation tests to `internal/toc/toc_test.go` (cdtoc real vectors)
- [x] Create `accuraterip/accuraterip.go` — HTTP lookup + binary response parsing
- [x] Create `accuraterip/accuraterip_test.go` — unit tests
- [x] Add `GetAccurateRipID()` getter to `logchecker/logchecker.go`
- [x] Extract AR DiscID from dBpoweramp logs in `parser_dbpoweramp.go`
- [x] Add `TestAccurateRipIDExtraction` to `logchecker_test.go`
- [x] Run full test suite, verify no regressions — all green

## Resolution of cutoff blocker
Handoff "formula failure" was a **bad-data problem, not a math problem**.
- dBpoweramp fixtures are **synthetic**: sequential hand-typed CRC32s
  (`A1F2B3C4`, `C3D4E5F6`...) and a non-computed `[DiscID:]`. The "expected"
  IDs derived from them are unreachable by any correct formula.
- Validated `toc.go` AR math against the Rust **cdtoc** crate's `t_accuraterip`
  vectors instead: 4-track + 16-track both **exact**.
- Algorithm confirmed: `off = sector - 150` per track + leadout;
  `id1 = Σoff`, `id2 = Σ max(off,1)*idx`. toc.go's 0-based LBA == `sector-150`,
  so existing code was already correct.
- Extraction path uses the embedded `[DiscID:]` when present (authoritative),
  TOC computation as fallback.
