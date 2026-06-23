package accuraterip

import (
	"encoding/binary"
	"testing"

	"github.com/Nirzak/logchecker-go/internal/toc"
)

// craftPressing builds a binary AR pressing block for testing.
func craftPressing(id1, id2, cddb uint32, tracks []TrackResult) []byte {
	b := make([]byte, 0, 13+len(tracks)*9)
	b = append(b, byte(len(tracks)))
	var dw [4]byte
	for _, v := range []uint32{id1, id2, cddb} {
		binary.LittleEndian.PutUint32(dw[:], v)
		b = append(b, dw[:]...)
	}
	for _, tr := range tracks {
		b = append(b, byte(tr.Confidence))
		binary.LittleEndian.PutUint32(dw[:], tr.CRCv1)
		b = append(b, dw[:]...)
		binary.LittleEndian.PutUint32(dw[:], tr.CRCv2)
		b = append(b, dw[:]...)
	}
	return b
}

func TestParseBinary_SinglePressing(t *testing.T) {
	want := []TrackResult{
		{Confidence: 14, CRCv1: 0xB2C3D4E5, CRCv2: 0x11112222},
		{Confidence: 15, CRCv1: 0xDEADBEEF, CRCv2: 0x33334444},
	}
	data := craftPressing(0x0002189a, 0x00087f33, 0x1f02e004, want)

	pressings, err := parseBinary(data)
	if err != nil {
		t.Fatalf("parseBinary: %v", err)
	}
	if len(pressings) != 1 {
		t.Fatalf("pressings = %d, want 1", len(pressings))
	}
	got := pressings[0].Tracks
	if len(got) != len(want) {
		t.Fatalf("tracks = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("track %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseBinary_MultiPressing(t *testing.T) {
	p1 := craftPressing(1, 2, 3, []TrackResult{{Confidence: 5, CRCv1: 0xAAAA, CRCv2: 0xBBBB}})
	p2 := craftPressing(4, 5, 6, []TrackResult{{Confidence: 9, CRCv1: 0xCCCC, CRCv2: 0xDDDD}})
	data := append(p1, p2...)

	pressings, err := parseBinary(data)
	if err != nil {
		t.Fatalf("parseBinary: %v", err)
	}
	if len(pressings) != 2 {
		t.Fatalf("pressings = %d, want 2", len(pressings))
	}
	if pressings[1].Tracks[0].Confidence != 9 {
		t.Errorf("p2 track0 confidence = %d, want 9", pressings[1].Tracks[0].Confidence)
	}
}

func TestParseBinary_Truncated(t *testing.T) {
	data := craftPressing(1, 2, 3, []TrackResult{{Confidence: 5}})
	if _, err := parseBinary(data[:len(data)-3]); err == nil {
		t.Error("expected error on truncated track data")
	}
	if _, err := parseBinary(data[:7]); err == nil {
		t.Error("expected error on truncated header")
	}
}

func TestParseBinary_Empty(t *testing.T) {
	pressings, err := parseBinary(nil)
	if err != nil {
		t.Fatalf("parseBinary(nil): %v", err)
	}
	if len(pressings) != 0 {
		t.Errorf("pressings = %d, want 0", len(pressings))
	}
}

func TestLookup_NilAndEmpty(t *testing.T) {
	if _, err := Lookup(nil); err == nil {
		t.Error("expected error for nil toc")
	}
	if _, err := Lookup(&toc.TOC{}); err == nil {
		t.Error("expected error for empty toc (no disc id)")
	}
}

// TestLookup_Live hits the real AccurateRip database. Skipped by default.
// Run with: go test -run TestLookup_Live -tags=live ./accuraterip/
func TestLookup_Live(t *testing.T) {
	t.Skip("network test; run manually")
	tc := &toc.TOC{
		FirstTrack: 1, LastTrack: 4,
		Offsets: []int{0, 0x2D2B - 150, 0x6256 - 150, 0xB327 - 150},
		Leadout: 0xD84A - 150,
	}
	res, err := Lookup(tc)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	t.Logf("status=%s url=%s pressings=%d", res.Status, res.URL, len(res.Pressings))
}
