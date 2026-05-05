package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// section represents either a chapter heading or a block of parsed verses.
// Exactly one of heading or verses will be populated.
type section struct {
	heading string
	verses  []verse // non-nil only when heading == ""
}

// verse holds a single parsed verse: its number and the individual Greek words.
type verse struct {
	num   string
	words []string
}

// trailingDigitRE matches a heading token that is a pure chapter number,
// e.g. the "13" in "1 Corinthians 13".
var trailingDigitRE = regexp.MustCompile(`^\d+$`)

// verseRE splits greekbible.com copy-paste text: "1 word word. 2 word..."
// We split on a verse number that is not preceded by a word character.
var verseRE = regexp.MustCompile(`(?:^|(?:\W))(\d+)\s`)

// parseInputFile reads the input file and returns a flat list of sections.
// Heading sections carry a text label; verse sections carry parsed verse data.
func parseInputFile(path string) ([]section, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var sections []section
	var pending []string

	flushPending := func() {
		if len(pending) == 0 {
			return
		}
		raw := strings.Join(pending, " ")
		pending = pending[:0]
		vs := parseVerses(raw)
		if len(vs) > 0 {
			sections = append(sections, section{verses: vs})
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
			sections = append(sections, section{heading: text})
		} else if strings.TrimSpace(line) != "" {
			pending = append(pending, strings.TrimSpace(line))
		}
	}
	flushPending()
	return sections, scanner.Err()
}

// parseVerses parses the inline verse format into (number, words) pairs.
func parseVerses(text string) []verse {
	// FindAllStringSubmatchIndex gives us the locations of each verse-number match.
	matches := verseRE.FindAllStringSubmatchIndex(strings.TrimSpace(text), -1)
	if len(matches) == 0 {
		return nil
	}

	var verses []verse
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
			verses = append(verses, verse{num: num, words: words})
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
func deriveTabName(sections []section) string {
	type ref struct{ ch, v string }
	var first, last ref
	currentChapter := "1"

	for _, sec := range sections {
		if sec.heading != "" {
			// Extract the trailing digit sequence, e.g. "1 Corinthians 13" → "13"
			parts := strings.Fields(sec.heading)
			if len(parts) > 0 {
				candidate := parts[len(parts)-1]
				if trailingDigitRE.MatchString(candidate) {
					currentChapter = candidate
				}
			}
			continue
		}
		for _, v := range sec.verses {
			if first.ch == "" {
				first = ref{ch: currentChapter, v: v.num}
			}
			last = ref{ch: currentChapter, v: v.num}
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
