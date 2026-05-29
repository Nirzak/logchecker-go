// Package eac provides EAC log language detection and translation.
package eac

import (
	_ "embed"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
)

// ErrUnknownLanguage is returned when the log language cannot be determined.
var ErrUnknownLanguage = errors.New("could not determine language of EAC log")

// ErrInvalidFile is returned when a language file cannot be loaded or parsed.
var ErrInvalidFile = errors.New("invalid translation file")

//go:embed languages/master.json
var masterJSON []byte

//go:embed languages/en.json
var englishJSON []byte

// languageEntry represents one entry in master.json.
type languageEntry struct {
	Name        string   `json:"name"`
	NameEnglish string   `json:"name_english"`
	EACStrings  []string `json:"eac_strings"`
}

// LangInfo holds the detected language metadata.
type LangInfo struct {
	Code        string
	Name        string
	NameEnglish string
}

// GetLanguage scans the log text against master.json EAC signature strings
// and returns the detected language. Falls back to ErrUnknownLanguage.
func GetLanguage(log string) (LangInfo, error) {
	var langs map[string]languageEntry
	if err := json.Unmarshal(masterJSON, &langs); err != nil {
		return LangInfo{}, ErrInvalidFile
	}
	for code, lang := range langs {
		for _, sig := range lang.EACStrings {
			pattern := `(?i)` + regexp.QuoteMeta(sig)
			if matched, _ := regexp.MatchString(pattern, log); matched {
				return LangInfo{
					Code:        code,
					Name:        lang.Name,
					NameEnglish: lang.NameEnglish,
				}, nil
			}
		}
	}
	return LangInfo{}, ErrUnknownLanguage
}

// IsKnownLanguage returns true if the log language can be determined.
func IsKnownLanguage(log string) bool {
	_, err := GetLanguage(log)
	return err == nil
}

// langFiles maps language codes to their embedded JSON content.
// We embed all language files at compile time.
//
//go:embed languages/bg.json
var bgJSON []byte

//go:embed languages/cs.json
var csJSON []byte

//go:embed languages/de.json
var deJSON []byte

//go:embed languages/es.json
var esJSON []byte

//go:embed languages/fr.json
var frJSON []byte

//go:embed languages/it.json
var itJSON []byte

//go:embed languages/jp.json
var jpJSON []byte

//go:embed languages/ko.json
var koJSON []byte

//go:embed languages/nl.json
var nlJSON []byte

//go:embed languages/pl.json
var plJSON []byte

//go:embed languages/ru.json
var ruJSON []byte

//go:embed languages/se.json
var seJSON []byte

//go:embed languages/sk.json
var skJSON []byte

//go:embed languages/sr.json
var srJSON []byte

//go:embed languages/zh.json
var zhJSON []byte

var langData = map[string][]byte{
	"bg": bgJSON,
	"cs": csJSON,
	"de": deJSON,
	"es": esJSON,
	"fr": frJSON,
	"it": itJSON,
	"jp": jpJSON,
	"ko": koJSON,
	"nl": nlJSON,
	"pl": plJSON,
	"ru": ruJSON,
	"se": seJSON,
	"sk": skJSON,
	"sr": srJSON,
	"zh": zhJSON,
}

// Translate replaces foreign-language phrases in log with their English equivalents.
// Returns the translated string, the number of replacements made, and any error.
// If langCode is "en", the log is returned unchanged with 0 replacements.
func Translate(log, langCode string) (string, int, error) {
	if langCode == "en" {
		return log, 0, nil
	}

	var english map[string]interface{}
	if err := json.Unmarshal(englishJSON, &english); err != nil {
		return log, 0, ErrInvalidFile
	}

	data, ok := langData[langCode]
	if !ok {
		return log, 0, ErrInvalidFile
	}
	var translation map[string]interface{}
	if err := json.Unmarshal(data, &translation); err != nil {
		return log, 0, ErrInvalidFile
	}

	// Build a list of (phrase, englishEquivalent, caseInsensitive) triples.
	// We sort by phrase length descending so that longer phrases are replaced
	// first, preventing shorter sub-phrases from corrupting them.
	type entry struct {
		phrase          string
		english         string
		caseInsensitive bool
	}
	var entries []entry

	for keyStr, val := range translation {
		enVal, ok := english[keyStr]
		if !ok {
			continue
		}
		enStr, ok := enVal.(string)
		if !ok || enStr == "" {
			continue
		}

		// Determine case sensitivity: keys > 16 use case-insensitive matching.
		keyInt := 0
		for _, ch := range keyStr {
			if ch >= '0' && ch <= '9' {
				keyInt = keyInt*10 + int(ch-'0')
			}
		}
		caseInsensitive := keyInt > 16

		// val may be a string or an array of strings.
		switch v := val.(type) {
		case string:
			if v != "" {
				entries = append(entries, entry{v, enStr, caseInsensitive})
			}
		case []interface{}:
			for _, item := range v {
				if s, ok := item.(string); ok && s != "" {
					entries = append(entries, entry{s, enStr, caseInsensitive})
				}
			}
		}
	}

	// Sort by phrase length descending to avoid shorter phrases corrupting longer ones.
	sort.Slice(entries, func(i, j int) bool {
		return len(entries[i].phrase) > len(entries[j].phrase)
	})

	replacements := 0
	for _, e := range entries {
		flags := ""
		if e.caseInsensitive {
			flags = "(?i)"
		}
		pattern := flags + regexp.QuoteMeta(e.phrase)
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		count := len(re.FindAllStringIndex(log, -1))
		if count > 0 {
			log = re.ReplaceAllString(log, strings.ReplaceAll(e.english, "$", "$$"))
			replacements += count
		}
	}

	return log, replacements, nil
}
