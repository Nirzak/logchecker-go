# Logchecker (Go Edition)

A CD rip logchecker, used for analyzing the generated logs for any problems that would potentially
indicate a non-perfect rip was produced. Of course, just because a log doesn't score a perfect 100%
does not mean that the produced rip isn't bit perfect, it's just less likely. 

This project is a pure Go rewrite of the original PHP Logchecker.

Unlike the original PHP version which required external Python scripts to validate EAC and XLD checksums, this Go version has all checksum verification built-in via native Go libraries (`github.com/Nirzak/eac-logchecker` and `github.com/Nirzak/xld-logchecker`).

[![Go Report Card](https://goreportcard.com/badge/github.com/Nirzak/logchecker-go)](https://goreportcard.com/report/github.com/Nirzak/logchecker-go)  [![Go Reference](https://pkg.go.dev/badge/github.com/Nirzak/logchecker-go.svg)](https://pkg.go.dev/github.com/Nirzak/logchecker-go)

## Requirements

* Go 1.25.0+ (Only for build and development. No runtime required for prebuilt binaries)

## Standalone CLI

### Installation

Download from github releases : https://github.com/Nirzak/logchecker-go/releases

Install via `go install`:

```bash
go install github.com/Nirzak/logchecker-go/cmd/logchecker@latest
```

Alternatively, you can build it from source:

```bash
git clone https://github.com/Nirzak/logchecker-go.git
cd logchecker-go
go build -o logchecker cmd/logchecker/main.go
```

### Usage

```text
$ logchecker version
Logchecker 1.14.4

Usage:
  logchecker analyze  [--html] [--no_text] [--ids] <file> [out_file] [details_json]
  logchecker analyse  (alias of analyze)
  logchecker decode   <file>
  logchecker translate [-l lang] <file>
  logchecker version
```

Main usage is through the `analyze` command, e.g.:

```text
$ logchecker analyze path/to/file.log
Ripper  : EAC
Version : 1.0 beta 3
Language: en
Score   : 59
Checksum: checksum_ok
Details :
    Could not verify gap handling (-10 points)
    Could not verify id3 tag setting (-1 point)
    Range rip detected (-30 points)
```

### Disc IDs (`--ids`)

`analyze --ids` prints the disc identifiers (AccurateRip, MusicBrainz, CTDB,
FreeDB/CDDB) with their lookup URLs, then exits without dumping the log text.
The AccurateRip ID is taken from the log when embedded (dBpoweramp); the rest
are computed from the parsed TOC. Lines are omitted when the log has no TOC.

```text
$ logchecker analyze --ids path/to/file.log
Ripper  : whipper
AccurateRip : 017-000dfdc8-00b4ca27-c2058d11
  AR URL    : http://www.accuraterip.com/accuraterip/8/c/d/dBAR-017-000dfdc8-00b4ca27-c2058d11.bin
MusicBrainz : wXcMD4BGh8KcpBCxKY.mfAfc_EY-
  MB URL    : https://musicbrainz.org/cdtoc/attach?toc=...&tracks=17&id=...
CTDB        : tbuVo3k57JgCiLhxX1jqNvn2hME
  CTDB URL  : https://db.cuetools.net/ui/cd/tbuVo3k57JgCiLhxX1jqNvn2hME
FreeDB/CDDB : c2058d11
  FreeDB URL: https://gnudb.com/cd/c2058d11
```

## Library Usage

### Installation

```bash
go get github.com/Nirzak/logchecker-go
```

### Usage

```go
package main

import (
    "fmt"
    "github.com/Nirzak/logchecker-go/logchecker"
)

func main() {
    lc := logchecker.New()
    
    // Load and parse the file
    err := lc.NewFile("/path/to/log/file.log")
    if err != nil {
        panic(err)
    }
    lc.Parse()
    
    // Output results
    fmt.Printf("Ripper   : %s\n", lc.GetRipper())
    fmt.Printf("Version  : %s\n", lc.GetRipperVersion())
    fmt.Printf("Score    : %d\n", lc.GetScore())
    fmt.Printf("Checksum : %s\n", lc.GetChecksumState())
    fmt.Printf("\nDetails:\n")
    for _, detail := range lc.GetDetails() {
        fmt.Printf("  %s\n", detail)
    }
}
```

### Public API Reference

```go

lc.Parse()              // Run analysis

lc.GetRipper()          // string: "EAC" | "XLD" | "whipper" | "dBpoweramp" | "unknown"
lc.GetRipperVersion()   // string
lc.GetScore()           // int 0–100
lc.GetChecksumState()   // "checksum_ok" | "checksum_invalid" | "checksum_missing"
lc.GetDetails()         // []string — human-readable list of deductions / notices
lc.GetLanguage()        // string language code, e.g. "en", "ru"
lc.GetLog()             // string — HTML-annotated log text (span-tagged)
lc.IsCombinedLog()      // bool — true when the file holds multiple rip sessions

// Disc identifiers
lc.GetTOC()             // *toc.TOC — parsed Table of Contents, or nil if absent
lc.GetAccurateRipID()   // string — AccurateRip ID (embedded if present, else computed)

// Control
lc.ValidateChecksum(false) // Disable external checksum validation
```

The value returned by `GetTOC()` exposes the disc-ID computations (all pure, no
network I/O):

```go
t := lc.GetTOC()
if t != nil {
    t.MusicBrainzDiscID()   // string
    t.MusicBrainzLookupURL()
    t.FreeDBDiscID()        // string (8-char hex CDDB ID)
    t.FreeDBLookupURL()
    t.CTDBDiscID()          // string (CUETools Database TOC ID)
    t.CTDBLookupURL()
    t.AccurateRipID()       // string "NNN-ID1-ID2-CDDB"
    t.AccurateRipURL()      // string — AccurateRip .bin lookup URL
}
```

### AccurateRip database verification (`accuraterip`)

The core `logchecker` library performs no network I/O. To verify a disc against
the AccurateRip database, use the separate `accuraterip` package, passing the
TOC from `GetTOC()`:

```go
import "github.com/Nirzak/logchecker-go/accuraterip"

res, err := accuraterip.Lookup(lc.GetTOC())   // or LookupWithContext(ctx, toc)
if err != nil {
    // network / parse error
}
switch res.Status {
case accuraterip.StatusFound:    // res.Pressings holds per-track Confidence/CRCv1/CRCv2
case accuraterip.StatusNotFound: // disc absent from the database
case accuraterip.StatusError:
}
```


## Testing

To run the test suite:

```bash
go test -v ./...
```
