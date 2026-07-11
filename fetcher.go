/*
Package main — fetcher.go

Fetches Greek NT verses directly from greekbible.com.

Chapter-level fetching: one HTTP request per chapter retrieves all verses in
that chapter using the URL pattern:

	https://www.greekbible.com/{book-slug}/{chapter}

The chapter page contains <sup>N</sup> verse markers followed by
<span class="word …"> word spans as direct siblings inside the passage-output
div. parseChapterHTML groups these into verses.

For verse-range refs like "John 1:1-10" or "John 7:45-8:12", fetchSections
fetches each chapter once and filters verses to the requested range in-process
— no verse-by-verse probing needed.
*/
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// ---------------------------------------------------------------------------
// Reference range
// ---------------------------------------------------------------------------

// refRange holds a parsed reference range, e.g. "John 1:1-10" or "John 1:50-2:10".
type refRange struct {
	book         string // display name, e.g. "John", "1 Corinthians"
	bookSlug     string // URL slug,  e.g. "john", "1-corinthians"
	startChapter int
	startVerse   int
	endChapter   int
	endVerse     int
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

// parseRef parses a reference range string into a refRange.
//
// Accepted formats:
//
//	"John 1:1-10"           same-chapter, verses 1–10
//	"John 1:50-2:10"        cross-chapter within the same book
//	"1 Corinthians 13:1-13" multi-word book name
//
// Cross-book ranges (e.g. "Ephesians 6:1-Philippians 2:6") are not supported.
// Both chapter and verse endpoints must belong to the same book.
func parseRef(s string) (refRange, error) {
	s = strings.TrimSpace(s)
	m := refRangeRE.FindStringSubmatch(s)
	if m == nil {
		return refRange{}, fmt.Errorf("invalid reference %q: expected \"Book ch:v-v\", \"Book ch:v-ch:v\", or \"Book ch-ch\" (whole-chapter fetch)", s)
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

	if startCh > endCh || (startCh == endCh && startV > endV) {
		return refRange{}, fmt.Errorf("invalid reference %q: start must be before end", s)
	}

	return refRange{
		book:         book,
		bookSlug:     bookSlug(book),
		startChapter: startCh,
		startVerse:   startV,
		endChapter:   endCh,
		endVerse:     endV,
	}, nil
}

// bookSlug converts a book display name to its greekbible.com URL slug.
// "1 Corinthians" → "1-corinthians", "John" → "john".
func bookSlug(book string) string {
	return strings.ToLower(strings.ReplaceAll(book, " ", "-"))
}

// tabNameFromRef produces the Google Sheets tab label from a parsed ref range.
// The book name is omitted — it normally appears in the sheet title instead.
// Examples: "1:1 - 1:10", "1:50 - 2:10".
func tabNameFromRef(r refRange) string {
	return fmt.Sprintf("%d:%d-%d:%d", r.startChapter, r.startVerse, r.endChapter, r.endVerse)
}

// ---------------------------------------------------------------------------
// Whole-chapter range (auto-detected from "Book ch-ch" ref format)
// ---------------------------------------------------------------------------

// chapterRange holds a parsed whole-chapter range, e.g. "Ephesians 1-6".
// It is used when the ref is in whole-chapter format; no verse boundaries
// are specified and every chapter is fetched in full.
type chapterRange struct {
	book         string // display name, e.g. "Ephesians"
	bookSlug     string // URL slug, e.g. "ephesians"
	startChapter int
	endChapter   int
}

// chapterRangeRE matches "Book ch-ch", e.g. "Ephesians 1-6".
// The book name may contain spaces and leading digits (e.g. "1 John 1-5").
var chapterRangeRE = regexp.MustCompile(`^(.+?)\s+(\d+)-(\d+)$`)

// parseChapterRange parses a whole-chapter range like "Ephesians 1-6" or
// "1 Corinthians 13-14". Returns an error if the format is wrong or the
// chapter range is inverted.
//
// Cross-book ranges (e.g. "Ephesians 6-Philippians 2") are not supported.
// All chapters must belong to the same book.
func parseChapterRange(s string) (chapterRange, error) {
	s = strings.TrimSpace(s)
	m := chapterRangeRE.FindStringSubmatch(s)
	if m == nil {
		return chapterRange{}, fmt.Errorf("invalid chapter range %q: expected format \"Book ch-ch\"", s)
	}
	startCh, _ := strconv.Atoi(m[2])
	endCh, _ := strconv.Atoi(m[3])
	if startCh > endCh {
		return chapterRange{}, fmt.Errorf("invalid chapter range %q: start chapter must not exceed end chapter", s)
	}
	book := m[1]
	return chapterRange{
		book:         book,
		bookSlug:     bookSlug(book),
		startChapter: startCh,
		endChapter:   endCh,
	}, nil
}

// fetchChapterSections fetches all verses of a single chapter and returns them
// as sections with a heading. tabName is the chapter number as a string (e.g. "3").
//
// This is the building block for whole-chapter fetch: the caller loops over
// chapters and creates one sheet tab per call.
func fetchChapterSections(ctx context.Context, client *http.Client, cr chapterRange, ch int) ([]section, string, error) {
	fmt.Printf("  Fetching %s chapter %d…\n", cr.book, ch)
	verses, ok, err := fetchChapter(ctx, client, cr.bookSlug, ch)
	if err != nil {
		return nil, "", fmt.Errorf("fetching %s %d: %w", cr.book, ch, err)
	}
	if !ok {
		return nil, "", fmt.Errorf("no content returned for %s chapter %d — check the reference is valid", cr.book, ch)
	}

	tabName := strconv.Itoa(ch)
	sections := []section{
		{heading: tabName},
		{verses: verses},
	}
	return sections, tabName, nil
}

// ---------------------------------------------------------------------------
// HTML parsing
// ---------------------------------------------------------------------------

// parsePassageHTML extracts trimmed Greek words from a greekbible.com verse
// page. It looks for <span class="word ..."> elements inside the
// passage-output div.
//
// Returns (words, true) on a valid verse page. Returns (nil, false) when the
// passage-output div contains no word spans — which is how the site signals
// that a verse reference is invalid (the generic "Greek NT Guide" page is
// rendered instead).
func parsePassageHTML(r io.Reader) ([]string, bool) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, false
	}

	passageDiv := findPassageOutputDiv(doc)
	if passageDiv == nil {
		return nil, false
	}

	var words []string
	collectWords(passageDiv, &words)
	if len(words) == 0 {
		return nil, false
	}
	return words, true
}

// parseChapterHTML parses a full greekbible.com chapter page and groups the
// Greek word spans by verse number.
//
// The chapter page's passage-output div contains <sup>N</sup> verse markers
// and <span class="word …"> word spans as direct siblings. This function walks
// those siblings in order, collecting words under each sup marker.
//
// Returns (verses, true) on success, or (nil, false) when the passage-output
// div contains no verses (e.g. the site's guide-page fallback is rendered).
func parseChapterHTML(r io.Reader) ([]verse, bool) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, false
	}

	passageDiv := findPassageOutputDiv(doc)
	if passageDiv == nil {
		return nil, false
	}

	var verses []verse
	var currentVerseNum string
	var currentWords []string

	flushVerse := func() {
		if currentVerseNum != "" && len(currentWords) > 0 {
			verses = append(verses, verse{num: currentVerseNum, words: currentWords})
		}
		currentVerseNum = ""
		currentWords = nil
	}

	for c := passageDiv.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode {
			continue
		}
		switch c.Data {
		case "sup":
			text := strings.TrimSpace(extractText(c))
			// Only treat numeric <sup> elements as verse markers. Non-numeric
			// superscripts (e.g. footnote symbols or asterisks) are ignored so
			// they don't corrupt the verse grouping or filterVerses logic.
			if _, err := strconv.Atoi(text); err == nil {
				flushVerse()
				currentVerseNum = text
			}
		case "span":
			if hasClass(c, "word") {
				if text := strings.TrimSpace(extractText(c)); text != "" {
					currentWords = append(currentWords, text)
				}
			}
		}
	}
	flushVerse()

	if len(verses) == 0 {
		return nil, false
	}
	return verses, true
}

// findPassageOutputDiv traverses the HTML tree and returns the first <div>
// whose class attribute starts with "passage-output".
func findPassageOutputDiv(n *html.Node) *html.Node {
	if n.Type == html.ElementNode && n.Data == "div" {
		for _, a := range n.Attr {
			if a.Key == "class" && strings.HasPrefix(a.Val, "passage-output") {
				return n
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findPassageOutputDiv(c); found != nil {
			return found
		}
	}
	return nil
}

// collectWords walks a subtree and appends the trimmed text of every <span>
// whose class contains "word relative" to words.
func collectWords(n *html.Node, words *[]string) {
	if n.Type == html.ElementNode && n.Data == "span" && hasClass(n, "word") {
		text := strings.TrimSpace(extractText(n))
		if text != "" {
			*words = append(*words, text)
		}
		return // don't recurse into word spans
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectWords(c, words)
	}
}

// hasClass reports whether a node's class attribute contains cls as a
// whitespace-separated token.
func hasClass(n *html.Node, cls string) bool {
	for _, a := range n.Attr {
		if a.Key == "class" {
			for part := range strings.SplitSeq(a.Val, " ") {
				if part == cls {
					return true
				}
			}
		}
	}
	return false
}

// extractText concatenates all text node descendants of n.
func extractText(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			sb.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return sb.String()
}

// ---------------------------------------------------------------------------
// Fetchers
// ---------------------------------------------------------------------------

const (
	greekBibleBase = "https://www.greekbible.com"
	fetchDelay     = 100 * time.Millisecond
)

// fetchChapter retrieves the full chapter page and parses it into verses.
// URL pattern: https://www.greekbible.com/{slug}/{chapter}
//
// Returns (verses, true, nil) on success, or (nil, false, nil) when the site
// returns the generic guide page (no verses). A non-nil error means a network
// or HTTP-level failure.
func fetchChapter(ctx context.Context, client *http.Client, slug string, chapter int) ([]verse, bool, error) {
	url := fmt.Sprintf("%s/%s/%d", greekBibleBase, slug, chapter)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}

	verses, ok := parseChapterHTML(resp.Body)
	return verses, ok, nil
}

// fetchSections fetches Greek verses from greekbible.com for the given
// reference range and returns them as sections ready for buildSheetData.
//
// One HTTP request is made per chapter in the range. Within the first and
// last chapters, verses are filtered to the requested start/end verse
// boundaries; middle chapters are included whole.
//
// A chapter heading section is emitted before each chapter's verses, matching
// what the # heading lines do in the file-based input mode.
func fetchSections(ctx context.Context, refStr string) ([]section, string, error) {
	r, err := parseRef(refStr)
	if err != nil {
		return nil, "", err
	}

	tabName := tabNameFromRef(r)
	client := &http.Client{Timeout: 15 * time.Second}

	var sections []section
	for ch := r.startChapter; ch <= r.endChapter; ch++ {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}

		fmt.Printf("  Fetching %s chapter %d…\n", r.book, ch)
		verses, ok, err := fetchChapter(ctx, client, r.bookSlug, ch)
		if err != nil {
			return nil, "", fmt.Errorf("fetching %s %d: %w", r.book, ch, err)
		}
		if !ok {
			return nil, "", fmt.Errorf("no content returned for %s chapter %d — check the reference is valid", r.book, ch)
		}

		filtered := filterVerses(verses, ch, r)
		sections = append(sections, section{heading: strconv.Itoa(ch)})
		if len(filtered) > 0 {
			sections = append(sections, section{verses: filtered})
		}

		if ch < r.endChapter {
			select {
			case <-time.After(fetchDelay):
			case <-ctx.Done():
				return nil, "", ctx.Err()
			}
		}
	}

	return sections, tabName, nil
}

// filterVerses trims a full chapter's verse list to the subset requested by r.
// For the start chapter, verses before r.startVerse are dropped.
// For the end chapter, verses after r.endVerse are dropped.
// Middle chapters are returned unchanged.
func filterVerses(verses []verse, ch int, r refRange) []verse {
	var out []verse
	for _, v := range verses {
		vn, _ := strconv.Atoi(v.num)
		if ch == r.startChapter && vn < r.startVerse {
			continue
		}
		if ch == r.endChapter && vn > r.endVerse {
			continue
		}
		out = append(out, v)
	}
	return out
}
