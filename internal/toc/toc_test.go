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
	if toc.AccurateRipID() != "" {
		t.Error("expected empty for nil TOC")
	}
	if toc.AccurateRipURL() != "" {
		t.Error("expected empty for nil TOC")
	}
}

// TestAccurateRipDiscID verifies ID1/ID2 against known-good vectors from the
// Rust `cdtoc` crate's t_accuraterip test. Offsets here are 0-based LBA
// (sector-150), matching this package's TOC convention; cdtoc subtracts 150
// from raw sectors internally, so the arithmetic is identical.
func TestAccurateRipDiscID(t *testing.T) {
	cases := []struct {
		name    string
		toc     *TOC
		wantID1 uint32
		wantID2 uint32
	}{
		{
			// cdtoc: 4+96+2D2B+6256+B327+D84A (sectors); -150 → LBA below.
			name: "4 tracks",
			toc: &TOC{
				FirstTrack: 1, LastTrack: 4,
				Offsets: []int{0, 0x2D2B - 150, 0x6256 - 150, 0xB327 - 150},
				Leadout: 0xD84A - 150,
			},
			wantID1: 0x0002189a,
			wantID2: 0x00087f33,
		},
		{
			// cdtoc: 10(0x10=16)+B6+5352+62AC+99D6+E218+12AC0+135E7+142E9+
			//        178B0+19D22+1B0D0+1E7FA+22882+247DB+27074+2A1BD+2C0FB
			name: "16 tracks",
			toc: &TOC{
				FirstTrack: 1, LastTrack: 16,
				Offsets: []int{
					0xB6 - 150, 0x5352 - 150, 0x62AC - 150, 0x99D6 - 150,
					0xE218 - 150, 0x12AC0 - 150, 0x135E7 - 150, 0x142E9 - 150,
					0x178B0 - 150, 0x19D22 - 150, 0x1B0D0 - 150, 0x1E7FA - 150,
					0x22882 - 150, 0x247DB - 150, 0x27074 - 150, 0x2A1BD - 150,
				},
				Leadout: 0x2C0FB - 150,
			},
			wantID1: 0x0018be61,
			wantID2: 0x012232a8,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.toc.AccurateRipDiscID1(); got != tc.wantID1 {
				t.Errorf("AccurateRipDiscID1() = %08x, want %08x", got, tc.wantID1)
			}
			if got := tc.toc.AccurateRipDiscID2(); got != tc.wantID2 {
				t.Errorf("AccurateRipDiscID2() = %08x, want %08x", got, tc.wantID2)
			}
		})
	}
}

// TestAccurateRipURL verifies URL format + directory digits from ID1 hex.
func TestAccurateRipURL(t *testing.T) {
	toc := &TOC{
		FirstTrack: 1, LastTrack: 4,
		Offsets: []int{0, 0x2D2B - 150, 0x6256 - 150, 0xB327 - 150},
		Leadout: 0xD84A - 150,
	}
	// ID1 = 0002189a → directory digits [7]=a [6]=9 [5]=8.
	url := toc.AccurateRipURL()
	want := "http://www.accuraterip.com/accuraterip/a/9/8/dBAR-" + toc.AccurateRipID() + ".bin"
	if url != want {
		t.Errorf("AccurateRipURL() = %q, want %q", url, want)
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
