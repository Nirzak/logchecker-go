// Package gnudb resolves a CDDB/freeDB disc ID against the gnudb.org database
// via its CDDB CGI protocol, returning the authoritative gnudb disc ID when the
// TOC matches an existing entry.
//
// Network I/O lives in this dedicated package, kept out of the pure logchecker
// core and the pure internal/toc disc-ID math. Consumers opt in by passing a
// *toc.TOC (e.g. from logchecker.GetTOC()) to Resolve.
package gnudb

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Nirzak/logchecker-go/internal/toc"
)

// helloString identifies this client to gnudb (user host clientname version).
const helloString = "nirzak+nirzak.win+logchecker+1.14.8"

// baseURL is the gnudb CDDB CGI endpoint. The CGI lives on the gnudb.gnudb.org
// host over plain HTTP; the bare gnudb.org host serves only the website.
const baseURL = "http://gnudb.gnudb.org/~cddb/cddb.cgi"

var defaultClient = &http.Client{Timeout: 15 * time.Second}

// Result holds the outcome of a gnudb query.
type Result struct {
	DiscID  string // the gnudb disc ID to use (matched id, or the calculated one)
	URL     string // https://gnudb.org/cd/<DiscID>
	Matched bool   // true if gnudb returned an exact entry
	Title   string // "Artist / Album" from the match, if any
}

// Resolve queries gnudb for the given TOC. On an exact match it returns the
// gnudb-supplied disc ID; otherwise it falls back to the calculated FreeDB ID.
// A query error is returned, but Result is still populated with the fallback.
func Resolve(t *toc.TOC) (*Result, error) {
	return ResolveWithContext(context.Background(), t)
}

// ResolveWithContext is Resolve with caller-supplied cancellation.
func ResolveWithContext(ctx context.Context, t *toc.TOC) (*Result, error) {
	calcID := t.FreeDBDiscID()
	res := &Result{
		DiscID: calcID,
		URL:    "https://gnudb.org/cd/" + calcID,
	}
	if calcID == "" {
		return res, fmt.Errorf("gnudb: empty disc id")
	}

	query := buildQuery(t, calcID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, query, nil)
	if err != nil {
		return res, err
	}
	req.Header.Set("User-Agent", "logchecker/1.14.8")
	resp, err := defaultClient.Do(req)
	if err != nil {
		return res, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return res, fmt.Errorf("gnudb: http %d", resp.StatusCode)
	}

	id, title := parseResponse(resp.Body)
	if id != "" {
		res.DiscID = id
		res.URL = "https://gnudb.org/cd/" + id
		res.Matched = true
		res.Title = title
	}
	return res, nil
}

// buildQuery assembles the cddb query URL. Tokens are space-separated, encoded
// as '+'; the query is built raw (not url.Values) because url.Values would
// percent-encode '+' to %2B, which the CDDB protocol does not accept.
func buildQuery(t *toc.TOC, discID string) string {
	var sb strings.Builder
	sb.WriteString(baseURL)
	sb.WriteString("?cmd=cddb+query+")
	sb.WriteString(discID)
	fmt.Fprintf(&sb, "+%d", t.LastTrack)
	for _, off := range t.Offsets {
		fmt.Fprintf(&sb, "+%d", off+150) // frame offset = LBA + 150 lead-in
	}
	totalSecs := (t.Leadout + 150) / 75
	fmt.Fprintf(&sb, "+%d", totalSecs)
	sb.WriteString("&hello=")
	sb.WriteString(helloString)
	sb.WriteString("&proto=6")
	return sb.String()
}

// parseResponse extracts the disc ID and title from a CDDB query reply.
//
// Exact single match (code 200):
//
//	200 <category> <discid> <Artist> / <Album>
//
// Match list (code 210 exact, or 211 inexact) — first candidate line:
//
//	<category> <discid> <Artist> / <Album>
func parseResponse(r io.Reader) (id, title string) {
	sc := bufio.NewScanner(r)
	var firstCode string
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "" || line == "." {
			continue
		}
		if firstCode == "" {
			fields := strings.SplitN(line, " ", 2)
			firstCode = fields[0]
			switch firstCode {
			case "200": // exact single: "200 <cat> <id> <title>"
				if len(fields) < 2 {
					return "", ""
				}
				return parseEntry(strings.TrimSpace(fields[1]))
			case "210", "211": // list: next non-code line is the first candidate
				continue
			default: // 202 no match, 403 corrupt, or error code
				return "", ""
			}
		}
		// 210/211 candidate line: "<cat> <id> <title>"
		return parseEntry(line)
	}
	return "", ""
}

// parseEntry parses "<category> <discid> <Artist> / <Album>".
func parseEntry(s string) (id, title string) {
	fields := strings.SplitN(s, " ", 3)
	if len(fields) < 2 {
		return "", ""
	}
	id = fields[1]
	if len(fields) == 3 {
		title = fields[2]
	}
	return id, title
}
