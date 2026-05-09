# Chapter-per-fetch & Chapter-per-tab Implementation Plan

## Overview

Two related improvements:

1. **Chapter-per-fetch optimisation** — instead of one HTTP request per verse,
   fetch an entire chapter in a single request (`greekbible.com/{book}/{chapter}`)
   and split the response into verses by reading `<sup>N</sup>` markers. This
   dramatically reduces network traffic for any `-ref` invocation.

2. **`--chapter-per-tab` mode** — a new flag that accepts a whole-chapter range
   (`"Ephesians 1-6"`) and creates one Google Sheets tab per chapter, each
   named by chapter number only (`"1"`, `"2"`, …).

---

## Current State Analysis

### Fetching (fetcher.go)

`fetchSections` loops **verse-by-verse** using
`https://www.greekbible.com/{slug}/{chapter}/{verse}`.  Chapter-end detection
relies on the site returning a "guide page" (no word spans) for a non-existent
verse — which costs an extra HTTP round-trip per chapter.

`fetchVerse` (`fetcher.go:290`) performs one HTTP GET per call and hands the
response to `parsePassageHTML`, which only knows how to extract word spans
(it ignores `<sup>` tags entirely).

### HTML structure (verified by live fetch)

`https://www.greekbible.com/john/8` returns the whole chapter inside the
`passage-output` div as:

```html
<sup>1</sup>
<span class="word relative word-1">Ἰησοῦς </span>
...
<sup>2</sup>
<span class="word relative word-2">Ὄρθρου </span>
...
```

`<sup>` elements and `<span class="word …">` elements are **direct siblings**
inside the passage-output div.  A new chapter-level HTML parser can walk these
siblings in order, grouping word spans between consecutive `<sup>` boundaries.

### Reference parsing (fetcher.go)

`parseRef` accepts:
- `"John 1:1-10"` (same-chapter, verse range)
- `"John 1:50-2:10"` (cross-chapter, verse range)

It does **not** accept whole-chapter ranges (`"Ephesians 1-6"`). This will be a
**separate parsing path** (`parseChapterRange`) to avoid complicating the
existing regex.

### Sheets multi-tab flow (main.go / sheets.go)

`run()` currently calls `fetchSections` → `buildSheetData` → one tab.
`addTabToSpreadsheet` and `createSpreadsheet` are already capable of writing
multiple tabs; we just need to drive them in a loop.

---

## Desired End State

### Optimised fetch

- Any `-ref` invocation fetches **one page per chapter**, not one per verse.
- A ref spanning chapters fetches exactly N pages (one per chapter involved).
- Within-chapter verse filtering (start/end verse) is applied in-process.
- No behaviour change visible to the user other than speed.

### `--chapter-per-tab`

```
go run . -ref "Ephesians 1-6" --chapter-per-tab -sheet-id <ID>
```

- Accepts whole-chapter range `"Book ch-ch"` (no verse numbers) when
  `--chapter-per-tab` is active.
- Creates one tab per chapter, named `"1"`, `"2"`, … `"6"`.
- Each tab contains all verses of that chapter.
- All tabs land in the same spreadsheet (new or existing via `-sheet-id`).

### Verification

- `go test ./...` passes (updated + new tests).
- Manual run of `go run . -ref "John 8:1-12"` produces identical output to the
  current verse-by-verse approach but with only 1 HTTP request instead of 12.
- Manual run of `go run . -ref "Ephesians 1-6" --chapter-per-tab` creates a
  spreadsheet with 6 tabs labelled 1–6.

---

## What We're NOT Doing

- No changes to the `-input` file mode.
- No change to the existing `parseRef` regex/behaviour for verse-range refs.
- No parallel fetching (sequential per-chapter fetch is sufficient).
- No support for `--chapter-per-tab` with a verse-range ref (it only makes
  sense for whole-chapter refs).
- No caching of fetched pages.

---

## Implementation Approach

Three phases: (1) new chapter-level HTML parser, (2) rewrite `fetchSections`
to use it, (3) add `parseChapterRange` + `--chapter-per-tab` flag.

---

## Phase 1: Chapter-level HTML parser (`fetcher.go`)

### Overview

Add `parseChapterHTML` which, given the `passage-output` div, walks its
**direct children** in order, grouping word spans under each `<sup>` verse
number it encounters.  Returns `[]verse` (the same type already used
throughout).

### Changes Required

#### 1. New function `parseChapterHTML` in `fetcher.go`

```go
// parseChapterHTML parses a full greekbible.com chapter page and returns
// the verses it contains.  It walks the direct children of the
// passage-output div, collecting <span class="word …"> elements under each
// <sup>N</sup> verse-number marker.
//
// Returns (verses, true) on success. Returns (nil, false) when the
// passage-output div contains no verses (guide page fallback).
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
        if c.Type == html.ElementNode && c.Data == "sup" {
            flushVerse()
            currentVerseNum = strings.TrimSpace(extractText(c))
        } else if c.Type == html.ElementNode && c.Data == "span" && hasClass(c, "word") {
            text := strings.TrimSpace(extractText(c))
            if text != "" {
                currentWords = append(currentWords, text)
            }
        }
    }
    flushVerse()

    if len(verses) == 0 {
        return nil, false
    }
    return verses, true
}
```

#### 2. New function `fetchChapter` in `fetcher.go`

```go
// fetchChapter retrieves a full chapter page and parses it into verses.
// URL pattern: https://www.greekbible.com/{slug}/{chapter}
// Returns (verses, true, nil) on success; (nil, false, nil) for guide page.
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
```

### Success Criteria

#### Automated Verification:
- [ ] `go test ./...` passes (existing tests unchanged, new tests added — see Phase 1 tests below)

#### New tests for `parseChapterHTML` in `fetcher_test.go`:

```go
const validChapterHTML = `<html><body>
<div class="passage-output bg-white ...">
<h2>John 8</h2>
<sup>1</sup>
<span class="word relative word-1">Ἰησοῦς </span>
<span class="word relative word-2">δὲ </span>
<sup>2</sup>
<span class="word relative word-3">Ὄρθρου </span>
</div>
</body></html>`

func TestParseChapterHTML(t *testing.T) {
    t.Run("extracts_verses_by_sup", func(t *testing.T) {
        verses, ok := parseChapterHTML(strings.NewReader(validChapterHTML))
        require.True(t, ok)
        require.Len(t, verses, 2)
        assert.Equal(t, "1", verses[0].num)
        assert.Equal(t, []string{"Ἰησοῦς", "δὲ"}, verses[0].words)
        assert.Equal(t, "2", verses[1].num)
        assert.Equal(t, []string{"Ὄρθρου"}, verses[1].words)
    })
    t.Run("guide_page_returns_false", func(t *testing.T) {
        verses, ok := parseChapterHTML(strings.NewReader(invalidVerseHTML))
        assert.False(t, ok)
        assert.Nil(t, verses)
    })
}
```

---

## Phase 2: Rewrite `fetchSections` to use chapter-level fetches (`fetcher.go`)

### Overview

Replace the verse-by-verse loop with a chapter-by-chapter loop.  For each
chapter in the range, call `fetchChapter`, then filter the returned verses
to only those within `[startVerse, endVerse]` for the first/last chapters.
Middle chapters are included whole.

### Changes Required

#### 1. Rewrite `fetchSections` in `fetcher.go`

```go
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

        fmt.Printf("  Fetching chapter %d…\n", ch)
        verses, ok, err := fetchChapter(ctx, client, r.bookSlug, ch)
        if err != nil {
            return nil, "", fmt.Errorf("fetching %s %d: %w", r.book, ch, err)
        }
        if !ok {
            return nil, "", fmt.Errorf("no content returned for %s %d", r.book, ch)
        }

        // Filter verses to the requested range.
        var filtered []verse
        for _, v := range verses {
            vn, _ := strconv.Atoi(v.num)
            if ch == r.startChapter && vn < r.startVerse {
                continue
            }
            if ch == r.endChapter && vn > r.endVerse {
                continue
            }
            filtered = append(filtered, v)
        }

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
```

**Key points:**
- `fetchDelay` is only applied *between* chapters, not after the last.
- The old `fetchVerse` function is kept (it may still be useful for single-verse
  refs if desired), but `fetchSections` no longer calls it.
- The old chapter-end detection loop (with invalid-verse sentinel) is removed.

### Success Criteria

#### Automated Verification:
- [ ] `go test ./...` passes
- [ ] Manual: `go run . -ref "John 8:1-12"` produces 12 correctly parsed verses
  and makes only **1** HTTP request (add a `fmt.Printf` counter temporarily if
  needed during testing)

#### Manual Verification:
- [ ] Cross-chapter ref `go run . -ref "John 7:45-8:12"` makes exactly 2 HTTP
  requests and the sheet contains the correct verses from both chapters.

---

## Phase 3: `parseChapterRange` + `--chapter-per-tab` flag

### Overview

Add a new reference format `"Book ch-ch"` (whole-chapter range) and a
`--chapter-per-tab` flag.  When the flag is set, `run()` loops over chapters
and creates one tab per chapter using the existing `addTabToSpreadsheet` or
`createSpreadsheet` API.

### Changes Required

#### 1. New type and parser in `fetcher.go`

```go
// chapterRange holds a parsed whole-chapter range, e.g. "Ephesians 1-6".
type chapterRange struct {
    book         string
    bookSlug     string
    startChapter int
    endChapter   int
}

// chapterRangeRE matches "Book ch-ch", e.g. "Ephesians 1-6".
var chapterRangeRE = regexp.MustCompile(`^(.+?)\s+(\d+)-(\d+)$`)

// parseChapterRange parses a whole-chapter range like "Ephesians 1-6".
// Returns an error if the format is wrong or the range is inverted.
func parseChapterRange(s string) (chapterRange, error) {
    s = strings.TrimSpace(s)
    m := chapterRangeRE.FindStringSubmatch(s)
    if m == nil {
        return chapterRange{}, fmt.Errorf("invalid chapter range %q: expected \"Book ch-ch\"", s)
    }
    startCh, _ := strconv.Atoi(m[2])
    endCh, _ := strconv.Atoi(m[3])
    if startCh > endCh {
        return chapterRange{}, fmt.Errorf("invalid chapter range %q: start chapter must be <= end chapter", s)
    }
    book := m[1]
    return chapterRange{
        book:         book,
        bookSlug:     bookSlug(book),
        startChapter: startCh,
        endChapter:   endCh,
    }, nil
}
```

#### 2. New `fetchChapterSections` in `fetcher.go`

Fetches a single whole chapter and returns `([]section, tabName, error)` —
same signature shape as `fetchSections` but for one chapter at a time, to
drive the per-tab loop.

```go
// fetchChapterSections fetches all verses of a single chapter from
// greekbible.com and returns them as sections with a chapter heading.
// tabName is the chapter number as a string (e.g. "3").
func fetchChapterSections(ctx context.Context, client *http.Client, cr chapterRange, ch int) ([]section, string, error) {
    fmt.Printf("  Fetching %s chapter %d…\n", cr.book, ch)
    verses, ok, err := fetchChapter(ctx, client, cr.bookSlug, ch)
    if err != nil {
        return nil, "", fmt.Errorf("fetching %s %d: %w", cr.book, ch, err)
    }
    if !ok {
        return nil, "", fmt.Errorf("no content returned for %s %d", cr.book, ch)
    }
    tabName := strconv.Itoa(ch)
    sections := []section{
        {heading: tabName},
        {verses: verses},
    }
    return sections, tabName, nil
}
```

#### 3. New flag and config field in `main.go`

```go
// In config struct:
type config struct {
    InputFile      string
    Ref            string
    Title          string
    SecretsFile    string
    SheetID        string
    ChapterPerTab  bool   // new
}

// In start():
chapterPerTab := flags.Bool("chapter-per-tab", false, "Create one tab per chapter (use with whole-chapter ref like \"Ephesians 1-6\")")
```

Add validation: `--chapter-per-tab` requires `-ref` and `-sheet-id` (or creates
a new spreadsheet for the first chapter, then adds tabs for the rest).

#### 4. New `runChapterPerTab` path in `main.go`

`run()` branches on `conf.ChapterPerTab`:

```go
func (a *app) run(ctx context.Context) error {
    // ... auth setup unchanged ...

    if a.conf.ChapterPerTab {
        return a.runChapterPerTab(ctx, httpClient)
    }
    // ... existing single-tab path ...
}

func (a *app) runChapterPerTab(ctx context.Context, httpClient *http.Client) error {
    cr, err := parseChapterRange(a.conf.Ref)
    if err != nil {
        return fmt.Errorf("parsing chapter range: %w", err)
    }

    client := &http.Client{Timeout: 15 * time.Second}
    spreadsheetID := a.conf.SheetID
    var sheetURL string

    for ch := cr.startChapter; ch <= cr.endChapter; ch++ {
        if ctx.Err() != nil {
            return ctx.Err()
        }

        sections, tabName, err := fetchChapterSections(ctx, client, cr, ch)
        if err != nil {
            return err
        }

        var totalVerses int
        for _, s := range sections {
            totalVerses += len(s.verses)
        }
        fmt.Printf("Chapter %d: %d verses\n", ch, totalVerses)

        d := buildSheetData(sections)

        if spreadsheetID == "" {
            // First chapter — create the spreadsheet.
            sheetURL, err = createSpreadsheet(ctx, httpClient, a.conf.Title, tabName, d)
            if err != nil {
                return err
            }
            // Extract the spreadsheet ID from the URL for subsequent tabs.
            spreadsheetID = extractSpreadsheetID(sheetURL)
        } else {
            sheetURL, err = addTabToSpreadsheet(ctx, httpClient, spreadsheetID, tabName, d)
            if err != nil {
                return err
            }
        }

        if ch < cr.endChapter {
            select {
            case <-time.After(fetchDelay):
            case <-ctx.Done():
                return ctx.Err()
            }
        }
    }

    fmt.Printf("\nDone! Open your sheet at:\n  %s\n", sheetURL)
    return nil
}
```

Add a small helper `extractSpreadsheetID(url string) string` that strips the
`https://docs.google.com/spreadsheets/d/` prefix.

#### 5. Validation logic in `start()` (`main.go`)

```go
if *chapterPerTab && *refFlag == "" {
    return fmt.Errorf("--chapter-per-tab requires -ref")
}
if *chapterPerTab && *inputFile != "" {
    return fmt.Errorf("--chapter-per-tab cannot be used with -input")
}
```

When `--chapter-per-tab` is set and `-sheet-id` is omitted, the first chapter
creates the spreadsheet; subsequent chapters add tabs to it (handled in
`runChapterPerTab` above).

### Success Criteria

#### Automated Verification:
- [ ] `go test ./...` passes with new tests for `parseChapterRange`
- [ ] `go build .` compiles cleanly

#### Manual Verification:
- [ ] `go run . -ref "Ephesians 1-6" --chapter-per-tab` creates a new spreadsheet with 6 tabs named `1`, `2`, `3`, `4`, `5`, `6`
- [ ] `go run . -ref "Ephesians 1-6" --chapter-per-tab -sheet-id <ID>` adds 6 tabs to the existing sheet
- [ ] Each tab contains correct Greek verses for that chapter
- [ ] `go run . -ref "Ephesians 1:1-6:24"` (verse-range, no flag) still works correctly via the optimised chapter-fetch path

---

## Testing Strategy

### New unit tests (`fetcher_test.go`)

| Test | What it covers |
|------|----------------|
| `TestParseChapterHTML/extracts_verses_by_sup` | Happy path: `<sup>` → word span grouping |
| `TestParseChapterHTML/guide_page_returns_false` | Invalid chapter returns `false` |
| `TestParseChapterHTML/no_words_after_sup` | `<sup>` with no following spans |
| `TestParseChapterRange/valid` | `"Ephesians 1-6"` parses correctly |
| `TestParseChapterRange/single_chapter` | `"John 1-1"` — start == end |
| `TestParseChapterRange/inverted_range` | Returns error |
| `TestParseChapterRange/verse_range_rejected` | `"John 1:1-10"` is rejected (wrong format) |
| `TestParseChapterRange/multi_word_book` | `"1 Corinthians 13-14"` |

### Existing tests

All existing tests in `fetcher_test.go`, `parse_test.go`, `builder_test.go`
must continue to pass unchanged.

---

## Migration Notes

- The `fetchVerse` function can be **kept** (it is tested via
  `TestParsePassageHTML` indirectly) but is no longer called by
  `fetchSections`. It may be removed in a future cleanup or kept for potential
  single-verse use.
- No Google Sheets schema changes; no migration needed.
- The `fetchDelay` constant (`100ms`) is reused — now applied between chapters
  rather than between verses, which is actually more polite.

---

## References

- `fetcher.go:220` — `fetchSections` (current verse-by-verse implementation)
- `fetcher.go:290` — `fetchVerse` (single verse HTTP fetch)
- `fetcher.go:117` — `parsePassageHTML` (verse-level HTML parser)
- `fetcher.go:136` — `findPassageOutputDiv`
- `main.go:118` — `run()` entry point
- `sheets.go:130` — `addTabToSpreadsheet`
- Live HTML structure verified: `https://www.greekbible.com/john/8`
