package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

var replacements = [][2]string{
	{"16X DVD- - ROM", "16X DVD-ROM"},
	{"HL)DP-ST", "HL-DP-ST"},
	{"FREECOM_", "FREECOM"},
	{"Generic_", "GENERIC"},
}

var (
	reDash  = regexp.MustCompile(` +- +`)
	reSpace = regexp.MustCompile(` +`)
)

func main() {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", "https://www.accuraterip.com/driveoffsets.htm", nil)
	if err != nil {
		log.Fatalf("request build failed: %v", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; logchecker-go/1.0)")
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("fetch failed: %v", err)
	}
	defer resp.Body.Close()

	doc, err := html.Parse(resp.Body)
	if err != nil {
		log.Fatalf("parse failed: %v", err)
	}
	fmt.Println("Loaded AccurateRip Drive Website")

	tables := findAll(doc, "table")
	if len(tables) < 2 {
		log.Fatal("expected at least 2 tables")
	}
	table := tables[1]

	var drives [][4]any
	for _, tr := range findAll(table, "tr") {
		if entry, ok := parseRow(tr); ok {
			drives = append(drives, entry)
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("getwd failed: %v", err)
	}
	outPath := filepath.Join(cwd, "logchecker", "resources", "drives.json")
	data, _ := json.Marshal(drives)
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		log.Fatalf("write failed: %v", err)
	}
	fmt.Printf("Updating %d drives\n", len(drives))
}

// parseRow parses one <tr> from the AccurateRip table.
// Returns the [4]any entry and true on success, zero value and false otherwise.
func parseRow(tr *html.Node) ([4]any, bool) {
	cells := findAll(tr, "td")
	if len(cells) != 4 {
		return [4]any{}, false
	}

	parts := make([]string, 4)
	for i, td := range cells {
		parts[i] = strings.TrimSpace(nodeText(td))
	}

	// col 0: drive name (strip leading "- ")
	name := strings.TrimLeft(parts[0], "- ")
	if name == "" {
		return [4]any{}, false
	}

	// col 1: offset e.g. "+6" or "-12"
	if parts[1] == "[Purged]" {
		return [4]any{}, false
	}
	offset, err := strconv.Atoi(strings.TrimPrefix(parts[1], "+"))
	if err != nil {
		return [4]any{}, false
	}

	// col 2: submitted by (kept as string per original format)
	submittedBy := parts[2]
	if submittedBy == "" {
		return [4]any{}, false
	}

	// col 3: percentage agree e.g. "100%" — PHP casts to int → strips "%"
	pct, err := strconv.Atoi(strings.TrimSuffix(parts[3], "%"))
	if err != nil || pct <= 50 {
		return [4]any{}, false
	}

	for _, r := range replacements {
		name = strings.ReplaceAll(name, r[0], r[1])
	}
	name = strings.ToLower(name)
	name = reDash.ReplaceAllString(name, " ")
	name = reSpace.ReplaceAllString(name, " ")

	return [4]any{name, offset, submittedBy, pct}, true
}

func findAll(n *html.Node, tag string) []*html.Node {
	var results []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == tag {
			results = append(results, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return results
}

func nodeText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		sb.WriteString(nodeText(c))
	}
	return sb.String()
}
