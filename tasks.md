# AccurateRip Implementation Tasks

- [ ] Add AR disc ID computation to `internal/toc/toc.go`
- [ ] Add AR computation tests to `internal/toc/toc_test.go`
- [ ] Create `accuraterip/accuraterip.go` — HTTP lookup + binary response parsing
- [ ] Create `accuraterip/accuraterip_test.go` — unit tests
- [ ] Add `GetAccurateRipID()` getter to `logchecker/logchecker.go`
- [ ] Extract AR DiscID from dBpoweramp logs in `parser_dbpoweramp.go`
- [ ] Add `TestAccurateRipIDExtraction` to `logchecker_test.go`
- [ ] Run full test suite, verify no regressions