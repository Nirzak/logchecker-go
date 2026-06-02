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
  logchecker analyze  [--html] [--no_text] <file> [out_file] [details_json]
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

lc.GetRipper()          // string: "EAC" | "XLD" | "whipper" | "unknown"
lc.GetRipperVersion()   // string
lc.GetScore()           // int 0–100
lc.GetChecksumState()   // "checksum_ok" | "checksum_invalid" | "checksum_missing"
lc.GetDetails()         // []string — human-readable list of deductions / notices
lc.GetLanguage()        // string language code, e.g. "en", "ru"
lc.GetLog()             // string — HTML-annotated log text (span-tagged)
lc.IsCombinedLog()      // bool — true when the file holds multiple rip sessions

// Control
lc.ValidateChecksum(false) // Disable external checksum validation
```

## Testing

To run the test suite:

```bash
go test -v ./...
```
