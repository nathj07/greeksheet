/*
Package main — fetcher.go

Fetches Greek NT verses directly from greekbible.com using the URL pattern:

	https://www.greekbible.com/{book-slug}/{chapter}/{verse}

Given a reference range like "John 1:1-10" or "John 1:50-2:10", fetchSections
iterates verse-by-verse, parses the Greek words from the passage-output div,
and returns the same []section format that the file-based parser produces.

Chapter-end detection relies on the site's behaviour: a non-existent verse
returns HTTP 200 with the generic "Greek NT Guide" section inside the
passage-output div (no <sup> or word spans), rather than a 404.
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
//	"John 1:50-2:10"        cross-chapter
//	"1 Corinthians 13:1-13" multi-word book name
func parseRef(s string) (refRange, error) {
	s = strings.TrimSpace(s)
	m := refRangeRE.FindStringSubmatch(s)
	if m == nil {
		return refRange{}, fmt.Errorf("invalid reference %q: expected format \"Book ch:v-v\" or \"Book ch:v-ch:v\"", s)
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
	return fmt.Sprintf("%d:%d - %d:%d", r.startChapter, r.startVerse, r.endChapter, r.endVerse)
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
// Verse fetcher
// ---------------------------------------------------------------------------

const (
	greekBibleBase  = "https://www.greekbible.com"
	fetchDelay      = 100 * time.Millisecond
)

// fetchSections fetches Greek verses from greekbible.com for the given
// reference range and returns them as sections ready for buildSheetData.
//
// A chapter heading section (containing just the chapter number as a bold
// label) is emitted before the first verse of each chapter, matching what the
// # heading lines do in the file-based input mode.
//
// Chapter-end detection: when a verse URL returns the generic guide page
// (no word spans in the passage-output div), the fetcher increments the
// chapter, resets the verse to 1, and continues — unless the chapter has
// already reached the end chapter, in which case it stops.
func fetchSections(ctx context.Context, refStr string) ([]section, string, error) {
	r, err := parseRef(refStr)
	if err != nil {
		return nil, "", err
	}

	tabName := tabNameFromRef(r)
	client := &http.Client{Timeout: 15 * time.Second}

	var sections []section
	ch := r.startChapter
	v := r.startVerse

	// Emit the first chapter heading before fetching anything.
	sections = append(sections, section{heading: strconv.Itoa(ch)})

	for {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}

		words, valid, err := fetchVerse(ctx, client, r.bookSlug, ch, v)
		if err != nil {
			return nil, "", fmt.Errorf("fetching %s %d:%d: %w", r.book, ch, v, err)
		}

		if !valid {
			// The site returned the guide page — this verse doesn't exist,
			// meaning we've hit the end of the chapter.
			if ch >= r.endChapter {
				// Already at or past the target end chapter; nothing more to fetch.
				break
			}
			ch++
			if ch > r.endChapter {
				// Sanity guard: don't overshoot the end chapter (e.g. if the
				// site returns unexpected invalid responses back-to-back).
				break
			}
			v = 1
			sections = append(sections, section{heading: strconv.Itoa(ch)})
			// No delay needed — we didn't get real content on the last request.
			continue
		}

		sections = append(sections, section{
			verses: []verse{{num: strconv.Itoa(v), words: words}},
		})

		if ch == r.endChapter && v == r.endVerse {
			break
		}

		v++
		// Use a context-aware sleep so the tool exits promptly on cancellation
		// rather than blocking for fetchDelay after each verse.
		select {
		case <-time.After(fetchDelay):
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}
	}

	return sections, tabName, nil
}

// fetchVerse retrieves a single verse page and parses the Greek words.
// Returns (words, true, nil) on success or (nil, false, nil) when the site
// indicates the verse doesn't exist. A non-nil error means a network or
// HTTP-level failure.
func fetchVerse(ctx context.Context, client *http.Client, slug string, chapter, verse int) ([]string, bool, error) {
	url := fmt.Sprintf("%s/%s/%d/%d", greekBibleBase, slug, chapter, verse)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	// A true 404 (if the site ever starts returning them) means the verse
	// does not exist — treat it the same as the guide-page fallback.
	if resp.StatusCode == http.StatusNotFound {
		return nil, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}

	words, ok := parsePassageHTML(resp.Body)
	return words, ok, nil
}
