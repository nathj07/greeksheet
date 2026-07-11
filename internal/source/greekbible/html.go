package greekbible

import (
	"io"
	"strconv"
	"strings"

	"github.com/nathj07/greeksheet/internal/document"
	"golang.org/x/net/html"
)

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
func parseChapterHTML(r io.Reader) ([]document.Verse, bool) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, false
	}

	passageDiv := findPassageOutputDiv(doc)
	if passageDiv == nil {
		return nil, false
	}

	var verses []document.Verse
	var currentVerseNum string
	var currentWords []string

	flushVerse := func() {
		if currentVerseNum != "" && len(currentWords) > 0 {
			verses = append(verses, document.Verse{Num: currentVerseNum, Words: currentWords})
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
// whose class contains "word" to words.
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
