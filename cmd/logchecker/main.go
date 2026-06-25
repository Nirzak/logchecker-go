// Command logchecker analyzes CD-rip log files produced by EAC, XLD, whipper, and dBpoweramp.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Nirzak/logchecker-go/gnudb"
	"github.com/Nirzak/logchecker-go/internal/parser/eac"
	"github.com/Nirzak/logchecker-go/internal/util"
	"github.com/Nirzak/logchecker-go/logchecker"
)

func usage() {
	fmt.Fprintf(os.Stderr, `Usage:
  logchecker analyze  [--html] [--no_text] [--ids] <file> [out_file] [details_json]
  logchecker analyse  (alias of analyze)
  logchecker decode   <file>
  logchecker translate [-l lang] <file>
  logchecker version

`)
	os.Exit(1)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}

	switch os.Args[1] {
	case "analyze", "analyse":
		cmdAnalyze(os.Args[2:])
	case "decode":
		cmdDecode(os.Args[2:])
	case "translate":
		cmdTranslate(os.Args[2:])
	case "version":
		fmt.Println(logchecker.Version)
	default:
		usage()
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// analyze command
// ─────────────────────────────────────────────────────────────────────────────

func cmdAnalyze(args []string) {
	fs := flag.NewFlagSet("analyze", flag.ExitOnError)
	htmlFlag := fs.Bool("html", false, "print the HTML version of the log")
	noText := fs.Bool("no_text", false, "do not print log text to console")
	idsFlag := fs.Bool("ids", false, "print disc IDs (AccurateRip, MusicBrainz, CTDB, FreeDB) and exit")
	fs.Parse(args)

	positional := fs.Args()
	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "analyze: <file> argument required")
		os.Exit(1)
	}

	file := positional[0]
	outFile := ""
	detailsFile := ""
	if len(positional) >= 2 {
		outFile = positional[1]
	}
	if len(positional) >= 3 {
		detailsFile = positional[2]
	}

	if _, err := os.Stat(file); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid file: %v\n", err)
		os.Exit(1)
	}

	lc := logchecker.New()
	if err := lc.NewFile(file); err != nil {
		fmt.Fprintln(os.Stderr, "Error reading file:", err)
		os.Exit(1)
	}
	lc.Parse()

	// Print disc IDs and exit, if requested.
	if *idsFlag {
		printDiscIDs(lc)
		return
	}

	// Write details JSON if requested.
	if detailsFile != "" {
		details := struct {
			Ripper   string      `json:"ripper"`
			Version  interface{} `json:"version"`
			Language interface{} `json:"language"`
			Combined bool        `json:"combined"`
			Score    int         `json:"score"`
			Checksum string      `json:"checksum"`
			Details  []string    `json:"details"`
		}{
			Ripper:   lc.GetRipper(),
			Version:  nilIfEmpty(lc.GetRipperVersion()),
			Language: nilIfEmpty(lc.GetLanguage()),
			Combined: lc.IsCombinedLog(),
			Score:    lc.GetScore(),
			Checksum: lc.GetChecksumState(),
			Details:  lc.GetDetails(),
		}
		data, _ := json.MarshalIndent(details, "", "    ")
		if err := os.WriteFile(detailsFile, data, 0644); err != nil {
			fmt.Fprintln(os.Stderr, "Error writing details:", err)
		}
	}

	// Write HTML log to file if requested.
	if outFile != "" {
		if err := os.WriteFile(outFile, []byte(lc.GetLog()), 0644); err != nil {
			fmt.Fprintln(os.Stderr, "Error writing output:", err)
		}
		return
	}

	// Print summary to stdout.
	fmt.Println("Ripper  :", lc.GetRipper())
	fmt.Println("Version :", lc.GetRipperVersion())
	fmt.Println("Language:", lc.GetLanguage())
	fmt.Println("Score   :", lc.GetScore())
	fmt.Println("Checksum:", lc.GetChecksumState())
	if details := lc.GetDetails(); len(details) > 0 {
		fmt.Println("Details :")
		for _, d := range details {
			fmt.Println("   ", d)
		}
	}

	if *noText {
		return
	}

	fmt.Println()
	fmt.Println("Log Text:")
	fmt.Println()

	logText := lc.GetLog()
	if !*htmlFlag {
		logText = htmlToConsole(logText)
	}
	fmt.Print(logText)
}

// printDiscIDs prints the computed disc identifiers and their lookup URLs.
// AccurateRip prefers the log-embedded ID (dBpoweramp); the rest derive from
// the parsed TOC. Lines are omitted when their source data is unavailable.
func printDiscIDs(lc *logchecker.Logchecker) {
	fmt.Println("Ripper  :", lc.GetRipper())

	if ar := lc.GetAccurateRipID(); ar != "" {
		fmt.Println("AccurateRip :", ar)
	} else {
		fmt.Println("AccurateRip : (unavailable)")
	}

	t := lc.GetTOC()
	if t == nil {
		fmt.Println("(no TOC in log — MusicBrainz/CTDB/FreeDB unavailable)")
		return
	}

	if t.AccurateRipURL() != "" {
		fmt.Println("  AR URL    :", t.AccurateRipURL())
	}
	fmt.Println("MusicBrainz :", t.MusicBrainzDiscID())
	fmt.Println("  MB URL    :", t.MusicBrainzLookupURL())
	fmt.Println("CTDB        :", t.CTDBDiscID())
	fmt.Println("  CTDB URL  :", t.CTDBLookupURL())
	fmt.Println("FreeDB/CDDB :", t.FreeDBDiscID())
	if g, err := gnudb.Resolve(t); err != nil {
		fmt.Println("  gnudb     : lookup failed:", err)
		fmt.Println("  FreeDB URL:", t.FreeDBLookupURL())
	} else if g.Matched {
		fmt.Println("  gnudb     : matched", g.DiscID, "—", g.Title)
		fmt.Println("  FreeDB URL:", g.URL)
	} else {
		fmt.Println("  gnudb     : no match; using calculated ID")
		fmt.Println("  FreeDB URL:", g.URL)
	}
}

// htmlToConsole strips HTML span/strong tags (console output, no colors).
func htmlToConsole(s string) string {
	s = strings.ReplaceAll(s, "</span>", "")
	s = strings.ReplaceAll(s, "</strong>", "")
	s = strings.ReplaceAll(s, "<strong>", "")
	// Strip all <span ...> opening tags.
	for {
		start := strings.Index(s, "<span")
		if start < 0 {
			break
		}
		end := strings.Index(s[start:], ">")
		if end < 0 {
			break
		}
		s = s[:start] + s[start+end+1:]
	}
	return s
}

func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// ─────────────────────────────────────────────────────────────────────────────
// decode command
// ─────────────────────────────────────────────────────────────────────────────

func cmdDecode(args []string) {
	fs := flag.NewFlagSet("decode", flag.ExitOnError)
	fs.Parse(args)

	positional := fs.Args()
	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "decode: <file> argument required")
		os.Exit(1)
	}

	file := positional[0]
	data, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error reading file:", err)
		os.Exit(1)
	}

	decoded, err := util.DecodeEncoding(data, false, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Warning: encoding detection failed:", err)
	}
	fmt.Print(decoded)
}

// ─────────────────────────────────────────────────────────────────────────────
// translate command
// ─────────────────────────────────────────────────────────────────────────────

func cmdTranslate(args []string) {
	fs := flag.NewFlagSet("translate", flag.ExitOnError)
	langFlag := fs.String("l", "", "force language code (e.g. de, fr, ru)")
	fs.Parse(args)

	positional := fs.Args()
	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "translate: <file> argument required")
		os.Exit(1)
	}

	file := positional[0]
	data, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error reading file:", err)
		os.Exit(1)
	}

	content, err := util.DecodeEncoding(data, false, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Warning: encoding detection failed:", err)
	}

	langCode := *langFlag
	if langCode == "" {
		info, err2 := eac.GetLanguage(content)
		if err2 != nil {
			fmt.Fprintln(os.Stderr, "Could not determine language:", err2)
			os.Exit(1)
		}
		langCode = info.Code
		fmt.Fprintln(os.Stderr, "Detected language:", info.Name, "("+info.Code+")")
	}

	translated, _, err := eac.Translate(content, langCode)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Translation error:", err)
		os.Exit(1)
	}
	fmt.Print(translated)
}
