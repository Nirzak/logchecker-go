// Package accuraterip queries the AccurateRip database over HTTP to verify
// whether a disc is present and to retrieve per-track confidence/CRC data.
//
// It is intentionally kept separate from the core logchecker library, which is
// pure CPU/memory. Disc ID *computation* lives in internal/toc; this package
// performs the network I/O a consumer opts into explicitly.
package accuraterip

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Nirzak/logchecker-go/internal/toc"
)

// Status is the outcome of a database lookup.
type Status string

const (
	StatusFound    Status = "found"
	StatusNotFound Status = "not_found"
	StatusError    Status = "error"
)

// TrackResult holds per-track verification data from one pressing.
type TrackResult struct {
	Confidence int
	CRCv1      uint32
	CRCv2      uint32
}

// Pressing holds the per-track data for one disc pressing.
type Pressing struct {
	Tracks []TrackResult
}

// Result is the outcome of a lookup.
type Result struct {
	DiscID    string // full AR disc ID (NNN-ID1-ID2-CDDB)
	URL       string // URL queried
	Status    Status
	Pressings []Pressing
}

// defaultClient bounds every request; AR returns small payloads.
var defaultClient = &http.Client{Timeout: 15 * time.Second}

// Lookup queries the AccurateRip database for the given TOC.
func Lookup(t *toc.TOC) (*Result, error) {
	return LookupWithContext(context.Background(), t)
}

// LookupWithContext is Lookup with caller-supplied cancellation.
func LookupWithContext(ctx context.Context, t *toc.TOC) (*Result, error) {
	if t == nil {
		return nil, fmt.Errorf("accuraterip: nil toc")
	}
	url := t.AccurateRipURL()
	if url == "" {
		return nil, fmt.Errorf("accuraterip: empty disc id")
	}
	res := &Result{DiscID: t.AccurateRipID(), URL: url, Status: StatusError}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return res, err
	}
	resp, err := defaultClient.Do(req)
	if err != nil {
		return res, err
	}
	defer resp.Body.Close()

	// AR returns 404 for discs not in the database.
	if resp.StatusCode == http.StatusNotFound {
		res.Status = StatusNotFound
		return res, nil
	}
	if resp.StatusCode != http.StatusOK {
		return res, fmt.Errorf("accuraterip: http %d", resp.StatusCode)
	}

	// Cap the body: real responses are a few KB; guard against a runaway stream.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return res, err
	}

	pressings, err := parseBinary(body)
	if err != nil {
		return res, err
	}
	res.Pressings = pressings
	if len(pressings) == 0 {
		res.Status = StatusNotFound
	} else {
		res.Status = StatusFound
	}
	return res, nil
}

// parseBinary decodes the AccurateRip .bin payload.
//
// Layout, repeated per pressing:
//
//	header: 1-byte track count + 3×uint32 LE (ID1, ID2, CDDB) = 13 bytes
//	track:  1-byte confidence + uint32 LE CRCv1 + uint32 LE CRCv2 = 9 bytes
//	        repeated trackCount times
func parseBinary(b []byte) ([]Pressing, error) {
	var pressings []Pressing
	pos := 0
	for pos < len(b) {
		if pos+13 > len(b) {
			return nil, fmt.Errorf("accuraterip: truncated header at %d", pos)
		}
		trackCount := int(b[pos])
		pos += 13 // skip count byte + ID1 + ID2 + CDDB
		if trackCount <= 0 || trackCount > 99 {
			return nil, fmt.Errorf("accuraterip: bad track count %d", trackCount)
		}
		if pos+trackCount*9 > len(b) {
			return nil, fmt.Errorf("accuraterip: truncated track data")
		}
		p := Pressing{Tracks: make([]TrackResult, trackCount)}
		for i := 0; i < trackCount; i++ {
			p.Tracks[i] = TrackResult{
				Confidence: int(b[pos]),
				CRCv1:      binary.LittleEndian.Uint32(b[pos+1 : pos+5]),
				CRCv2:      binary.LittleEndian.Uint32(b[pos+5 : pos+9]),
			}
			pos += 9
		}
		pressings = append(pressings, p)
	}
	return pressings, nil
}
