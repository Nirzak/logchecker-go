package logchecker_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Nirzak/logchecker-go/logchecker"
)

func TestDebugEn3(t *testing.T) {
	lc := logchecker.New()
	if err := lc.NewFile("tests/logs/eac/originals/en_3.log"); err != nil {
		t.Fatal(err)
	}
	lc.Parse()
	out := lc.GetLog()
	lines := strings.Split(out, "\n")
	fmt.Printf("en_3 Total lines: %d\n", len(lines))
	if len(lines) >= 265 {
		for i := 262; i < len(lines) && i < 268; i++ {
			fmt.Printf("line %d: %q\n", i+1, lines[i])
		}
	}
}

func TestDebugEn2(t *testing.T) {
	lc := logchecker.New()
	if err := lc.NewFile("tests/logs/eac/originals/en_2.log"); err != nil {
		t.Fatal(err)
	}
	lc.Parse()
	out := lc.GetLog()
	lines := strings.Split(out, "\n")
	fmt.Printf("en_2 Total lines: %d\n", len(lines))
	if len(lines) >= 244 {
		for i := 242; i < len(lines) && i < 248; i++ {
			fmt.Printf("line %d: %q\n", i+1, lines[i])
		}
	}
}
