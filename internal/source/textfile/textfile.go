/*
Package textfile parses a plain text file copied from greekbible.com into a
document.Document.

Lines beginning with '#' become bold chapter-heading sections (the '#' is
stripped). All other non-blank lines are joined and parsed as a block of verses
in the greekbible.com inline format:

	1 word word word. 2 word word...
*/
package textfile

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nathj07/greeksheet/internal/document"
)

// trailingDigitRE matches a heading token that is a pure chapter number,
// e.g. the "13" in "1 Corinthians 13".
var trailingDigitRE = regexp.MustCompile(`^\d+$`)

// verseRE splits greekbible.com copy-paste text: "1 word word. 2 word..."
// We split on a verse number that is not preceded by a word character.
var verseRE = regexp.MustCompile(`(?:^|(?:\W))(\d+)\s`)

// Source reads Greek verses from a local text file.
type Source struct {
	path string
}

// New returns a Source that reads the given file path when Load runs.
func New(path string) *Source {
	return &Source{path: path}
}

// Load parses the file into a single-tab Document. The Document title defaults
// to the file's base name (without extension) and the tab is named from the
// chapter:verse range found in the content.
func (s *Source) Load(ctx context.Context) (document.Document, error) {
	sections, err := parseInputFile(s.path)
	if err != nil {
		return document.Document{}, fmt.Errorf("reading input: %w", err)
	}

	base := filepath.Base(s.path)
	title := strings.TrimSuffix(base, filepath.Ext(base))
	return document.Document{
		Title: title,
		Tabs:  []document.Tab{{Name: deriveTabName(sections), Sections: sections}},
	}, nil
}

// parseInputFile reads the input file and returns a flat list of sections.
// Heading sections carry a text label; verse sections carry parsed verse data.
func parseInputFile(path string) ([]document.Section, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var sections []document.Section
	var pending []string

	flushPending := func() {
		if len(pending) == 0 {
			return
		}
		raw := strings.Join(pending, " ")
		pending = pending[:0]
		vs := parseVerses(raw)
		if len(vs) > 0 {
			sections = append(sections, document.Section{Verses: vs})
		}
	}

	scanner := bufio.NewScanner(f)
	// Default scanner buffer (64 KB) can overflow for long chapters pasted as a single line;
	// 1 MB handles even the largest NT books comfortably.
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			flushPending()
			text := strings.TrimSpace(strings.TrimLeft(line, "# "))
			sections = append(sections, document.Section{Heading: text})
		} else if strings.TrimSpace(line) != "" {
			pending = append(pending, strings.TrimSpace(line))
		}
	}
	flushPending()
	return sections, scanner.Err()
}

// parseVerses parses the inline verse format into (number, words) pairs.
func parseVerses(text string) []document.Verse {
	// FindAllStringSubmatchIndex gives us the locations of each verse-number match.
	matches := verseRE.FindAllStringSubmatchIndex(strings.TrimSpace(text), -1)
	if len(matches) == 0 {
		return nil
	}

	var verses []document.Verse
	for i, m := range matches {
		numStart, numEnd := m[2], m[3]
		num := text[numStart:numEnd]

		// The verse body starts after the number+space and ends at the next match (or EOF).
		bodyStart := m[1] // end of the full match (past the trailing space)
		bodyEnd := len(text)
		if i+1 < len(matches) {
			bodyEnd = matches[i+1][0] // start of next match's leading non-word char
		}

		words := strings.Fields(text[bodyStart:bodyEnd])
		if len(words) > 0 {
			verses = append(verses, document.Verse{Num: num, Words: words})
		}
	}
	return verses
}

// deriveTabName produces a "ch:v - ch:v" label from the parsed sections,
// suitable for use as a Google Sheets tab title.
//
// Chapter numbers are extracted from the trailing integer of each heading line
// (e.g. "1 Corinthians 13" → chapter 13). If no heading precedes the first
// verse block, chapter 1 is assumed. The range collapses to "ch:v" when the
// input contains exactly one verse. Falls back to "sheet" when no verses are
// found.
func deriveTabName(sections []document.Section) string {
	type ref struct{ ch, v string }
	var first, last ref
	currentChapter := "1"

	for _, sec := range sections {
		if sec.Heading != "" {
			// Extract the trailing digit sequence, e.g. "1 Corinthians 13" → "13"
			parts := strings.Fields(sec.Heading)
			if len(parts) > 0 {
				candidate := parts[len(parts)-1]
				if trailingDigitRE.MatchString(candidate) {
					currentChapter = candidate
				}
			}
			continue
		}
		for _, v := range sec.Verses {
			if first.ch == "" {
				first = ref{ch: currentChapter, v: v.Num}
			}
			last = ref{ch: currentChapter, v: v.Num}
		}
	}

	if first.ch == "" {
		return "sheet"
	}
	if first.ch == last.ch && first.v == last.v {
		return fmt.Sprintf("%s:%s", first.ch, first.v)
	}
	return fmt.Sprintf("%s:%s - %s:%s", first.ch, first.v, last.ch, last.v)
}
