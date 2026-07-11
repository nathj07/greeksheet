/*
Package greekbible fetches Greek NT verses directly from greekbible.com and
assembles them into a document.Document.

Chapter-level fetching: one HTTP request per chapter retrieves all verses in
that chapter using the URL pattern:

	https://www.greekbible.com/{book-slug}/{chapter}

The chapter page contains <sup>N</sup> verse markers followed by
<span class="word …"> word spans as direct siblings inside the passage-output
div. parseChapterHTML groups these into verses.

Ref formats are dispatched by Load:

  - "Book ch-ch" / "Book ch": whole chapters, one Tab per chapter.
  - "Book ch:v-v" / "Book ch:v-ch:v": a verse range within a single Tab. One
    HTTP request is made per chapter; verses are filtered in-process to the
    requested range — no verse-by-verse probing needed.
*/
package greekbible

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/nathj07/greeksheet/internal/document"
	"github.com/nathj07/greeksheet/internal/reference"
)

const (
	greekBibleBase = "https://www.greekbible.com"
	fetchDelay     = 100 * time.Millisecond
	httpTimeout    = 15 * time.Second
)

// Source fetches Greek text for a single reference from greekbible.com.
type Source struct {
	ref string
}

// New returns a Source that fetches the given reference. The ref is not parsed
// until Load runs, so construction never fails.
func New(ref string) *Source {
	return &Source{ref: ref}
}

// Load fetches the configured reference and assembles it into a Document.
//
// Whole-chapter formats ("Book ch-ch", "Book ch") produce one Tab per chapter.
// The structural chapter-range check runs before parsing so that an inverted
// range such as "Ephesians 6-1" surfaces the specific chapter-range error
// rather than falling through to verse-range parsing and returning a confusing
// "invalid reference" error.
func (s *Source) Load(ctx context.Context) (document.Document, error) {
	if reference.IsChapterRange(s.ref) {
		cr, err := reference.ParseChapterRange(s.ref)
		if err != nil {
			return document.Document{}, err
		}
		return s.loadChapters(ctx, cr)
	}
	if reference.IsSingleChapter(s.ref) {
		cr, err := reference.ParseSingleChapter(s.ref)
		if err != nil {
			return document.Document{}, err
		}
		return s.loadChapters(ctx, cr)
	}
	return s.loadVerseRange(ctx)
}

// loadChapters fetches every chapter in cr and returns a Document with one Tab
// per chapter, each named by chapter number ("1", "2", …).
func (s *Source) loadChapters(ctx context.Context, cr reference.ChapterRange) (document.Document, error) {
	fmt.Printf("Fetching %s chapters %d–%d from greekbible.com…\n", cr.Book, cr.StartChapter, cr.EndChapter)

	client := &http.Client{Timeout: httpTimeout}
	d := document.Document{Title: s.ref}

	for ch := cr.StartChapter; ch <= cr.EndChapter; ch++ {
		if ctx.Err() != nil {
			return document.Document{}, ctx.Err()
		}

		sections, tabName, err := fetchChapterSections(ctx, client, cr, ch)
		if err != nil {
			return document.Document{}, err
		}

		var totalVerses int
		for _, sec := range sections {
			totalVerses += len(sec.Verses)
		}
		fmt.Printf("  Chapter %d: %d verses\n", ch, totalVerses)

		d.Tabs = append(d.Tabs, document.Tab{Name: tabName, Sections: sections})

		if ch < cr.EndChapter {
			select {
			case <-time.After(fetchDelay):
			case <-ctx.Done():
				return document.Document{}, ctx.Err()
			}
		}
	}

	return d, nil
}

// loadVerseRange fetches a verse-range reference into a single Tab.
func (s *Source) loadVerseRange(ctx context.Context) (document.Document, error) {
	fmt.Printf("Fetching Greek text for %q from greekbible.com…\n", s.ref)

	sections, tabName, err := fetchSections(ctx, s.ref)
	if err != nil {
		return document.Document{}, fmt.Errorf("fetching verses: %w", err)
	}
	return document.Document{
		Title: s.ref,
		Tabs:  []document.Tab{{Name: tabName, Sections: sections}},
	}, nil
}

// fetchChapterSections fetches all verses of a single chapter and returns them
// as sections with a heading. tabName is the chapter number as a string (e.g. "3").
//
// This is the building block for whole-chapter fetch: the caller loops over
// chapters and creates one sheet tab per call.
func fetchChapterSections(ctx context.Context, client *http.Client, cr reference.ChapterRange, ch int) ([]document.Section, string, error) {
	fmt.Printf("  Fetching %s chapter %d…\n", cr.Book, ch)
	verses, ok, err := fetchChapter(ctx, client, cr.BookSlug, ch)
	if err != nil {
		return nil, "", fmt.Errorf("fetching %s %d: %w", cr.Book, ch, err)
	}
	if !ok {
		return nil, "", fmt.Errorf("no content returned for %s chapter %d — check the reference is valid", cr.Book, ch)
	}

	tabName := strconv.Itoa(ch)
	sections := []document.Section{
		{Heading: tabName},
		{Verses: verses},
	}
	return sections, tabName, nil
}

// fetchChapter retrieves the full chapter page and parses it into verses.
// URL pattern: https://www.greekbible.com/{slug}/{chapter}
//
// Returns (verses, true, nil) on success, or (nil, false, nil) when the site
// returns the generic guide page (no verses). A non-nil error means a network
// or HTTP-level failure.
func fetchChapter(ctx context.Context, client *http.Client, slug string, chapter int) ([]document.Verse, bool, error) {
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
// verse-range reference and returns them as sections ready for the output
// target.
//
// One HTTP request is made per chapter in the range. Within the first and
// last chapters, verses are filtered to the requested start/end verse
// boundaries; middle chapters are included whole.
//
// A chapter heading section is emitted before each chapter's verses, matching
// what the # heading lines do in the file-based input mode.
func fetchSections(ctx context.Context, refStr string) ([]document.Section, string, error) {
	r, err := reference.ParseRef(refStr)
	if err != nil {
		return nil, "", err
	}

	tabName := reference.TabName(r)
	client := &http.Client{Timeout: httpTimeout}

	var sections []document.Section
	for ch := r.StartChapter; ch <= r.EndChapter; ch++ {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}

		fmt.Printf("  Fetching %s chapter %d…\n", r.Book, ch)
		verses, ok, err := fetchChapter(ctx, client, r.BookSlug, ch)
		if err != nil {
			return nil, "", fmt.Errorf("fetching %s %d: %w", r.Book, ch, err)
		}
		if !ok {
			return nil, "", fmt.Errorf("no content returned for %s chapter %d — check the reference is valid", r.Book, ch)
		}

		filtered := filterVerses(verses, ch, r)
		sections = append(sections, document.Section{Heading: strconv.Itoa(ch)})
		if len(filtered) > 0 {
			sections = append(sections, document.Section{Verses: filtered})
		}

		if ch < r.EndChapter {
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
// For the start chapter, verses before r.StartVerse are dropped.
// For the end chapter, verses after r.EndVerse are dropped.
// Middle chapters are returned unchanged.
func filterVerses(verses []document.Verse, ch int, r reference.RefRange) []document.Verse {
	var out []document.Verse
	for _, v := range verses {
		vn, _ := strconv.Atoi(v.Num)
		if ch == r.StartChapter && vn < r.StartVerse {
			continue
		}
		if ch == r.EndChapter && vn > r.EndVerse {
			continue
		}
		out = append(out, v)
	}
	return out
}
