package gnudb

import (
	"strings"
	"testing"

	"github.com/Nirzak/logchecker-go/internal/toc"
)

func testTOC() *toc.TOC {
	return &toc.TOC{
		FirstTrack: 1, LastTrack: 6,
		Offsets: []int{0, 21191, 46706, 73452, 95617, 121242},
		Leadout: 131263,
	}
}

func TestBuildQuery(t *testing.T) {
	tc := testTOC()
	q := buildQuery(tc, "bb0c2306")

	// space-separated tokens encoded as '+', never %2B
	if strings.Contains(q, "%2B") || strings.Contains(q, " ") {
		t.Fatalf("query has raw space or %%2B: %s", q)
	}
	for _, want := range []string{
		"cmd=cddb+query+bb0c2306+6+",
		"+150+", // first frame offset = 0 + 150
		"&hello=nirzak+nirzak.win+logchecker+1.14.8",
		"&proto=6",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("query missing %q in %s", want, q)
		}
	}
	// total secs = (131263+150)/75 = 1752
	if !strings.Contains(q, "+1752&hello=") {
		t.Errorf("query missing total secs 1752: %s", q)
	}
}

func TestParseResponse_Exact(t *testing.T) {
	body := "200 rock 9d0bf78f Arijit Singh / Aashiqui 2\n"
	id, title := parseResponse(strings.NewReader(body))
	if id != "9d0bf78f" {
		t.Errorf("id = %q, want 9d0bf78f", id)
	}
	if title != "Arijit Singh / Aashiqui 2" {
		t.Errorf("title = %q", title)
	}
}

func TestParseResponse_Multiple(t *testing.T) {
	body := "211 close matches found\n" +
		"rock 9d0bf78f Arijit Singh / Aashiqui 2\n" +
		"misc 9d0bf70b VA / Aashiqui 2\n" +
		".\n"
	id, title := parseResponse(strings.NewReader(body))
	if id != "9d0bf78f" {
		t.Errorf("id = %q, want first candidate 9d0bf78f", id)
	}
	if title != "Arijit Singh / Aashiqui 2" {
		t.Errorf("title = %q", title)
	}
}

func TestParseResponse_ExactList(t *testing.T) {
	// code 210: exact matches list (real gnudb reply for Aashiqui 2)
	body := "210 Found exact matches, list follows (until terminating `.')\n" +
		"data 9d0bf789 Various Artists / Aashiqui 2\n" +
		"data 9d0bf78f Arijit Singh / Aashiqui 2\n" +
		".\n"
	id, title := parseResponse(strings.NewReader(body))
	if id != "9d0bf789" {
		t.Errorf("id = %q, want first candidate 9d0bf789", id)
	}
	if title != "Various Artists / Aashiqui 2" {
		t.Errorf("title = %q", title)
	}
}

func TestParseResponse_NoMatch(t *testing.T) {
	id, title := parseResponse(strings.NewReader("202 No match found\n"))
	if id != "" || title != "" {
		t.Errorf("expected empty, got id=%q title=%q", id, title)
	}
}

func TestResolve_EmptyID(t *testing.T) {
	res, err := Resolve(&toc.TOC{})
	if err == nil {
		t.Error("expected error for empty disc id")
	}
	if res.Matched {
		t.Error("expected no match for empty toc")
	}
}

// TestResolve_Live hits the real gnudb CDDB endpoint. Skipped by default.
// Run manually: go test -run TestResolve_Live ./gnudb/
//
// TOC is the "Various Artists / Aashiqui 2" disc (11 tracks); gnudb returns a
// 210 exact-match list whose first entry is 9d0bf789. Calculated CDDB ID is
// 9d0bf70b — the track-count byte differs, which is exactly what the fuzzy
// lookup resolves.
func TestResolve_Live(t *testing.T) {
	t.Skip("network test; run manually")
	tc := &toc.TOC{
		FirstTrack: 1, LastTrack: 11,
		Offsets: []int{0, 19799, 49216, 72234, 95373, 115479, 137089, 155294, 171530, 195283, 217618},
		Leadout: 229796,
	}
	res, err := Resolve(tc)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	t.Logf("calc=%s matched=%v id=%s title=%q url=%s",
		tc.FreeDBDiscID(), res.Matched, res.DiscID, res.Title, res.URL)
	if !res.Matched {
		t.Error("expected a gnudb match for Aashiqui 2 TOC")
	}
}
