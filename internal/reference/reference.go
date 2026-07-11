/*
Package reference parses New Testament scripture reference strings into typed
ranges before any network request is made.

It recognises four surface formats:

	"John 1:1-10"      verse range within one chapter   → RefRange
	"John 1:50-2:10"   cross-chapter verse range        → RefRange
	"Ephesians 1-6"    whole chapters                   → ChapterRange
	"Ephesians 1"      single whole chapter             → ChapterRange

All formats are limited to a single book; cross-book ranges are not supported.
Every parse validates the book, chapter, and verse numbers against
package datasource, so an out-of-range reference is rejected here rather than
surfacing later as a confusing fetch failure.
*/
package reference

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/nathj07/greeksheet/internal/datasource"
)

// RefRange is a parsed verse-range reference, e.g. "John 1:1-10" or
// "John 1:50-2:10".
type RefRange struct {
	Book         string // display name, e.g. "John", "1 Corinthians"
	BookSlug     string // URL slug,  e.g. "john", "1-corinthians"
	StartChapter int
	StartVerse   int
	EndChapter   int
	EndVerse     int
}

// ChapterRange is a parsed whole-chapter range, e.g. "Ephesians 1-6". No verse
// boundaries are specified; every chapter is fetched in full. A single-chapter
// ref ("Ephesians 1") yields a ChapterRange with StartChapter == EndChapter.
type ChapterRange struct {
	Book         string // display name, e.g. "Ephesians"
	BookSlug     string // URL slug, e.g. "ephesians"
	StartChapter int
	EndChapter   int
}

// refRangeRE matches:
//
//	<book>  <ch>:<v>-<v>           (same-chapter shorthand)
//	<book>  <ch>:<v>-<ch>:<v>      (cross-chapter)
//
// The book name may contain spaces and leading digits (e.g. "1 Corinthians").
var refRangeRE = regexp.MustCompile(
	`^(.+?)\s+(\d+):(\d+)-(?:(\d+):)?(\d+)$`,
)

// chapterRangeRE matches "Book ch-ch", e.g. "Ephesians 1-6".
// The book name may contain spaces and leading digits (e.g. "1 John 1-5").
var chapterRangeRE = regexp.MustCompile(`^(.+?)\s+(\d+)-(\d+)$`)

// singleChapterRE matches "Book ch", e.g. "Ephesians 1".
// It is checked after chapterRangeRE so that "Book ch-ch" is never ambiguous.
var singleChapterRE = regexp.MustCompile(`^(.+?)\s+(\d+)$`)

// IsChapterRange reports whether s structurally looks like a whole-chapter
// range ("Book ch-ch"). Callers use this to choose the chapter-range path
// before falling back to verse-range parsing, so that an inverted range such
// as "Ephesians 6-1" surfaces a specific error rather than a confusing
// "invalid reference" one.
func IsChapterRange(s string) bool {
	return chapterRangeRE.MatchString(strings.TrimSpace(s))
}

// IsSingleChapter reports whether s structurally looks like a single whole
// chapter ("Book ch"). It must be checked after IsChapterRange.
func IsSingleChapter(s string) bool {
	return singleChapterRE.MatchString(strings.TrimSpace(s))
}

// ParseRef parses a verse-range reference string into a RefRange.
//
// Accepted formats:
//
//	"John 1:1-10"           same-chapter, verses 1–10
//	"John 1:50-2:10"        cross-chapter within the same book
//	"1 Corinthians 13:1-13" multi-word book name
//
// Cross-book ranges (e.g. "Ephesians 6:1-Philippians 2:6") are not supported.
func ParseRef(s string) (RefRange, error) {
	s = strings.TrimSpace(s)
	m := refRangeRE.FindStringSubmatch(s)
	if m == nil {
		return RefRange{}, fmt.Errorf("invalid reference %q: expected \"Book ch:v-v\", \"Book ch:v-ch:v\", or \"Book ch-ch\" (whole-chapter)", s)
	}
	// m[1]=book  m[2]=startCh  m[3]=startV  m[4]=endCh(optional)  m[5]=endV
	book := m[1]
	startCh, _ := strconv.Atoi(m[2])
	startV, _ := strconv.Atoi(m[3])
	endCh := startCh // same-chapter shorthand
	if m[4] != "" {
		endCh, _ = strconv.Atoi(m[4])
	}
	endV, _ := strconv.Atoi(m[5])

	if err := datasource.ValidateRef(book, startCh, startV, endCh, endV); err != nil {
		return RefRange{}, fmt.Errorf("invalid reference %q: %w", s, err)
	}

	return RefRange{
		Book:         book,
		BookSlug:     BookSlug(book),
		StartChapter: startCh,
		StartVerse:   startV,
		EndChapter:   endCh,
		EndVerse:     endV,
	}, nil
}

// ParseChapterRange parses a whole-chapter range like "Ephesians 1-6" or
// "1 Corinthians 13-14". Returns an error if the format is wrong or the
// chapter range is inverted.
//
// Cross-book ranges (e.g. "Ephesians 6-Philippians 2") are not supported.
func ParseChapterRange(s string) (ChapterRange, error) {
	s = strings.TrimSpace(s)
	m := chapterRangeRE.FindStringSubmatch(s)
	if m == nil {
		return ChapterRange{}, fmt.Errorf("invalid chapter range %q: expected format \"Book ch-ch\"", s)
	}
	startCh, _ := strconv.Atoi(m[2])
	endCh, _ := strconv.Atoi(m[3])
	book := m[1]
	if err := datasource.ValidateChapterRange(book, startCh, endCh); err != nil {
		return ChapterRange{}, fmt.Errorf("invalid chapter range %q: %w", s, err)
	}
	return ChapterRange{
		Book:         book,
		BookSlug:     BookSlug(book),
		StartChapter: startCh,
		EndChapter:   endCh,
	}, nil
}

// ParseSingleChapter parses a single-chapter ref like "Ephesians 1" or
// "1 Corinthians 13". It is syntactic sugar for the whole-chapter path — the
// result is a ChapterRange with StartChapter == EndChapter so a caller looping
// over chapters creates exactly one tab.
func ParseSingleChapter(s string) (ChapterRange, error) {
	s = strings.TrimSpace(s)
	m := singleChapterRE.FindStringSubmatch(s)
	if m == nil {
		return ChapterRange{}, fmt.Errorf("invalid single-chapter ref %q: expected format \"Book ch\"", s)
	}
	ch, _ := strconv.Atoi(m[2])
	book := m[1]
	if err := datasource.ValidateChapterRange(book, ch, ch); err != nil {
		return ChapterRange{}, fmt.Errorf("invalid single-chapter ref %q: %w", s, err)
	}
	return ChapterRange{
		Book:         book,
		BookSlug:     BookSlug(book),
		StartChapter: ch,
		EndChapter:   ch,
	}, nil
}

// BookSlug converts a book display name to its greekbible.com URL slug.
// "1 Corinthians" → "1-corinthians", "John" → "john".
func BookSlug(book string) string {
	return strings.ToLower(strings.ReplaceAll(book, " ", "-"))
}

// TabName produces the Google Sheets tab label from a parsed verse range.
// The book name is omitted — it normally appears in the sheet title instead.
// Examples: "1:1-1:10", "1:50-2:10".
func TabName(r RefRange) string {
	return fmt.Sprintf("%d:%d-%d:%d", r.StartChapter, r.StartVerse, r.EndChapter, r.EndVerse)
}
