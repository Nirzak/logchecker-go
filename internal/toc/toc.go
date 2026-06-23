// Package toc provides CD Table of Contents representation and disc ID
// calculation for MusicBrainz, FreeDB/CDDB, and CUETools Database (CTDB).
package toc

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"strings"
)

// TOC holds the parsed Table of Contents from a CD rip log.
// Offsets are 0-based LBA sector addresses (no lead-in offset added).
type TOC struct {
	FirstTrack int   // always 1 for standard audio CDs
	LastTrack  int   // number of audio tracks
	Offsets    []int // 0-based LBA start sector per track (index 0 = track 1)
	Leadout    int   // 0-based LBA sector immediately after last track
}

// mbBase64Encoding is the custom Base64 encoding used by MusicBrainz.
// Standard Base64 uses +, /, = but MusicBrainz replaces them with ., _, -
var mbBase64Encoding = base64.NewEncoding(
	"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._",
).WithPadding('-')

// ctdbBase64Encoding is the URL-safe Base64 encoding used by CUETools.
var ctdbBase64Encoding = base64.URLEncoding.WithPadding(base64.NoPadding)

// MusicBrainzDiscID computes the MusicBrainz Disc ID from the TOC.
// See https://musicbrainz.org/doc/Disc_ID_Calculation
func (t *TOC) MusicBrainzDiscID() string {
	if t == nil || len(t.Offsets) == 0 {
		return ""
	}

	h := sha1.New()

	// First track number
	fmt.Fprintf(h, "%02X", t.FirstTrack)
	// Last track number
	fmt.Fprintf(h, "%02X", t.LastTrack)

	// Lead-out offset at index 0, then up to 99 track offsets.
	// All offsets need +150 for the standard lead-in.
	offsets := make([]int, 100)
	offsets[0] = t.Leadout + 150
	for i, off := range t.Offsets {
		if i < 99 {
			offsets[i+1] = off + 150
		}
	}
	for _, off := range offsets {
		fmt.Fprintf(h, "%08X", off)
	}

	digest := h.Sum(nil)
	return mbBase64Encoding.EncodeToString(digest)
}

// MusicBrainzTOCString returns the TOC string used in MusicBrainz lookup URLs.
// Format: "1 N leadout+150 off1+150 off2+150 ..."
func (t *TOC) MusicBrainzTOCString() string {
	if t == nil || len(t.Offsets) == 0 {
		return ""
	}
	parts := make([]string, 0, len(t.Offsets)+3)
	parts = append(parts, fmt.Sprintf("%d", t.FirstTrack))
	parts = append(parts, fmt.Sprintf("%d", t.LastTrack))
	parts = append(parts, fmt.Sprintf("%d", t.Leadout+150))
	for _, off := range t.Offsets {
		parts = append(parts, fmt.Sprintf("%d", off+150))
	}
	return strings.Join(parts, "+")
}

// MusicBrainzLookupURL returns the URL to look up or attach this disc on MusicBrainz.
func (t *TOC) MusicBrainzLookupURL() string {
	if t == nil || len(t.Offsets) == 0 {
		return ""
	}
	discID := t.MusicBrainzDiscID()
	tocStr := t.MusicBrainzTOCString()
	return fmt.Sprintf("https://musicbrainz.org/cdtoc/attach?toc=%s&tracks=%d&id=%s",
		tocStr, t.LastTrack, discID)
}

// FreeDBDiscID computes the FreeDB/CDDB disc ID (8-char hex).
func (t *TOC) FreeDBDiscID() string {
	if t == nil || len(t.Offsets) == 0 {
		return ""
	}

	// All offsets need +150 lead-in adjustment.
	checksum := 0
	for _, off := range t.Offsets {
		checksum += digitSum((off + 150) / 75)
	}

	totalSeconds := (t.Leadout+150)/75 - (t.Offsets[0]+150)/75

	discID := ((checksum % 255) << 24) | (totalSeconds << 8) | t.LastTrack
	return fmt.Sprintf("%08x", discID)
}

// FreeDBLookupURL returns the GnuDB lookup URL for this disc.
func (t *TOC) FreeDBLookupURL() string {
	id := t.FreeDBDiscID()
	if id == "" {
		return ""
	}
	return fmt.Sprintf("https://gnudb.com/cd/%s", id)
}

// CTDBDiscID computes a best-effort CUETools Database TOC ID.
// The algorithm hashes track lengths (in sectors) using SHA-1,
// then encodes with URL-safe Base64 (no padding).
func (t *TOC) CTDBDiscID() string {
	if t == nil || len(t.Offsets) == 0 {
		return ""
	}

	// Build string of track lengths separated by spaces.
	var sb strings.Builder
	for i := 0; i < len(t.Offsets); i++ {
		if i > 0 {
			sb.WriteByte(' ')
		}
		var length int
		if i+1 < len(t.Offsets) {
			length = t.Offsets[i+1] - t.Offsets[i]
		} else {
			length = t.Leadout - t.Offsets[i]
		}
		fmt.Fprintf(&sb, "%d", length)
	}

	h := sha1.New()
	h.Write([]byte(sb.String()))
	digest := h.Sum(nil)
	return ctdbBase64Encoding.EncodeToString(digest)
}

// CTDBLookupURL returns the CUETools Database lookup URL.
func (t *TOC) CTDBLookupURL() string {
	id := t.CTDBDiscID()
	if id == "" {
		return ""
	}
	return fmt.Sprintf("https://db.cuetools.net/ui/cd/%s", id)
}

// AccurateRipDiscID1 computes the first AccurateRip disc identifier.
// It is the sum of all 0-based LBA offsets (tracks + lead-out), masked to 32 bits.
func (t *TOC) AccurateRipDiscID1() uint32 {
	if t == nil || len(t.Offsets) == 0 {
		return 0
	}
	var id1 uint32
	for _, off := range t.Offsets {
		id1 += uint32(off)
	}
	id1 += uint32(t.Leadout)
	return id1
}

// AccurateRipDiscID2 computes the second AccurateRip disc identifier.
// It is the sum of max(offset, 1) * (i+1) for each offset and lead-out, masked to 32 bits.
func (t *TOC) AccurateRipDiscID2() uint32 {
	if t == nil || len(t.Offsets) == 0 {
		return 0
	}
	var id2 uint32
	for i, off := range t.Offsets {
		v := uint32(off)
		if v == 0 {
			v = 1
		}
		id2 += v * uint32(i+1)
	}
	// Lead-out is at position len(Offsets)+1
	lo := uint32(t.Leadout)
	if lo == 0 {
		lo = 1
	}
	id2 += lo * uint32(len(t.Offsets)+1)
	return id2
}

// AccurateRipID returns the full AccurateRip disc identifier string.
// Format: "NNN-IIIIIIII-JJJJJJJJ-KKKKKKKK" where NNN is track count,
// I is ID1 (hex), J is ID2 (hex), K is FreeDB/CDDB ID (hex).
func (t *TOC) AccurateRipID() string {
	if t == nil || len(t.Offsets) == 0 {
		return ""
	}
	cddb := t.FreeDBDiscID()
	if cddb == "" {
		return ""
	}
	return fmt.Sprintf("%03d-%08x-%08x-%s",
		t.LastTrack, t.AccurateRipDiscID1(), t.AccurateRipDiscID2(), cddb)
}

// AccurateRipURL returns the AccurateRip database lookup URL for this disc.
// URL: http://www.accuraterip.com/accuraterip/X/Y/Z/dBAR-NNN-ID1-ID2-CDDB.bin
// where X, Y, Z are derived from the hex representation of ID1.
func (t *TOC) AccurateRipURL() string {
	id := t.AccurateRipID()
	if id == "" {
		return ""
	}
	id1hex := fmt.Sprintf("%08x", t.AccurateRipDiscID1())
	// Directory path uses last 3 hex chars of ID1 (positions 7, 6, 5)
	return fmt.Sprintf("http://www.accuraterip.com/accuraterip/%c/%c/%c/dBAR-%s.bin",
		id1hex[7], id1hex[6], id1hex[5], id)
}

// digitSum returns the sum of the decimal digits of n.
func digitSum(n int) int {
	sum := 0
	for n > 0 {
		sum += n % 10
		n /= 10
	}
	return sum
}
