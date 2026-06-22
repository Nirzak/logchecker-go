package toc

import (
	"testing"
)

// TestMusicBrainzDiscID_DocumentationExample verifies against the example
// from https://musicbrainz.org/doc/Disc_ID_Calculation
// CD with 6 tracks, expected disc ID: 49HHV7Eb8UKF3aQiNmu1GR8vKTY-
func TestMusicBrainzDiscID_DocumentationExample(t *testing.T) {
	// From the MB docs: 0-based LBA offsets (before +150 adjustment).
	toc := &TOC{
		FirstTrack: 1,
		LastTrack:  6,
		Offsets:    []int{0, 15213, 32164, 46442, 63264, 80339},
		Leadout:    95312,
	}

	got := toc.MusicBrainzDiscID()
	want := "49HHV7Eb8UKF3aQiNmu1GR8vKTY-"
	if got != want {
		t.Errorf("MusicBrainzDiscID() = %q, want %q", got, want)
	}
}

// TestMusicBrainzTOCString verifies the TOC string format.
func TestMusicBrainzTOCString(t *testing.T) {
	toc := &TOC{
		FirstTrack: 1,
		LastTrack:  6,
		Offsets:    []int{0, 15213, 32164, 46442, 63264, 80339},
		Leadout:    95312,
	}

	got := toc.MusicBrainzTOCString()
	want := "1+6+95462+150+15363+32314+46592+63414+80489"
	if got != want {
		t.Errorf("MusicBrainzTOCString() = %q, want %q", got, want)
	}
}

// TestMusicBrainzDiscID_WhipperLog verifies against the whipper 1.log fixture.
// whipper log has: MusicBrainz Disc ID: wXcMD4BGh8KcpBCxKY.mfAfc_EY-
// TOC from log: 17 tracks, offsets from Start sector fields.
func TestMusicBrainzDiscID_WhipperLog(t *testing.T) {
	// From whipper 1.log TOC section (Start sector values)
	toc := &TOC{
		FirstTrack: 1,
		LastTrack:  17,
		Offsets: []int{
			0, 3775, 9127, 16237, 21634, 25436, 29828,
			38491, 46082, 53567, 61145, 69428, 74137,
			81696, 87254, 92894, 99627,
		},
		Leadout: 106578, // end sector of last track + 1 = 106577 + 1
	}

	got := toc.MusicBrainzDiscID()
	want := "wXcMD4BGh8KcpBCxKY.mfAfc_EY-"
	if got != want {
		t.Errorf("MusicBrainzDiscID() = %q, want %q", got, want)
	}
}

// TestFreeDBDiscID_WhipperLog verifies against the whipper 1.log fixture.
// whipper log has: CDDB Disc ID: c2058d11
func TestFreeDBDiscID_WhipperLog(t *testing.T) {
	toc := &TOC{
		FirstTrack: 1,
		LastTrack:  17,
		Offsets: []int{
			0, 3775, 9127, 16237, 21634, 25436, 29828,
			38491, 46082, 53567, 61145, 69428, 74137,
			81696, 87254, 92894, 99627,
		},
		Leadout: 106578,
	}

	got := toc.FreeDBDiscID()
	want := "c2058d11"
	if got != want {
		t.Errorf("FreeDBDiscID() = %q, want %q", got, want)
	}
}

// TestFreeDBLookupURL verifies the GnuDB URL format.
func TestFreeDBLookupURL(t *testing.T) {
	toc := &TOC{
		FirstTrack: 1,
		LastTrack:  6,
		Offsets:    []int{0, 15213, 32164, 46442, 63264, 80339},
		Leadout:    95312,
	}

	got := toc.FreeDBLookupURL()
	want := "https://gnudb.com/cd/" + toc.FreeDBDiscID()
	if got != want {
		t.Errorf("FreeDBLookupURL() = %q, want %q", got, want)
	}
}

// TestMusicBrainzLookupURL verifies URL contains all components.
func TestMusicBrainzLookupURL(t *testing.T) {
	toc := &TOC{
		FirstTrack: 1,
		LastTrack:  6,
		Offsets:    []int{0, 15213, 32164, 46442, 63264, 80339},
		Leadout:    95312,
	}

	url := toc.MusicBrainzLookupURL()
	if url == "" {
		t.Fatal("MusicBrainzLookupURL() returned empty")
	}
	if !contains(url, "musicbrainz.org") {
		t.Errorf("URL missing musicbrainz.org: %s", url)
	}
	if !contains(url, toc.MusicBrainzDiscID()) {
		t.Errorf("URL missing disc ID: %s", url)
	}
	if !contains(url, "tracks=6") {
		t.Errorf("URL missing tracks=6: %s", url)
	}
}

// TestCTDBDiscID verifies CTDB disc ID is non-empty for valid TOC.
func TestCTDBDiscID(t *testing.T) {
	toc := &TOC{
		FirstTrack: 1,
		LastTrack:  6,
		Offsets:    []int{0, 15213, 32164, 46442, 63264, 80339},
		Leadout:    95312,
	}

	got := toc.CTDBDiscID()
	if got == "" {
		t.Error("CTDBDiscID() returned empty for valid TOC")
	}

	url := toc.CTDBLookupURL()
	if !contains(url, "db.cuetools.net") {
		t.Errorf("CTDB URL missing db.cuetools.net: %s", url)
	}
}

// TestNilTOC verifies all methods handle nil TOC gracefully.
func TestNilTOC(t *testing.T) {
	var toc *TOC
	if toc.MusicBrainzDiscID() != "" {
		t.Error("expected empty for nil TOC")
	}
	if toc.FreeDBDiscID() != "" {
		t.Error("expected empty for nil TOC")
	}
	if toc.CTDBDiscID() != "" {
		t.Error("expected empty for nil TOC")
	}
	if toc.MusicBrainzLookupURL() != "" {
		t.Error("expected empty for nil TOC")
	}
	if toc.FreeDBLookupURL() != "" {
		t.Error("expected empty for nil TOC")
	}
	if toc.CTDBLookupURL() != "" {
		t.Error("expected empty for nil TOC")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
