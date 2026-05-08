package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// parseRef tests
// ---------------------------------------------------------------------------

func TestParseRef(t *testing.T) {
	f := func(input string, want refRange, wantErr bool) {
		t.Helper()
		got, err := parseRef(input)
		if wantErr {
			require.Error(t, err)
			return
		}
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}

	t.Run("same_chapter_shorthand", func(t *testing.T) {
		f("John 1:1-10", refRange{
			book: "John", bookSlug: "john",
			startChapter: 1, startVerse: 1,
			endChapter: 1, endVerse: 10,
		}, false)
	})
	t.Run("cross_chapter", func(t *testing.T) {
		f("John 1:50-2:10", refRange{
			book: "John", bookSlug: "john",
			startChapter: 1, startVerse: 50,
			endChapter: 2, endVerse: 10,
		}, false)
	})
	t.Run("multi_word_book", func(t *testing.T) {
		f("1 Corinthians 13:1-13", refRange{
			book: "1 Corinthians", bookSlug: "1-corinthians",
			startChapter: 13, startVerse: 1,
			endChapter: 13, endVerse: 13,
		}, false)
	})
	t.Run("multi_word_book_cross_chapter", func(t *testing.T) {
		f("1 Thessalonians 4:15-5:3", refRange{
			book: "1 Thessalonians", bookSlug: "1-thessalonians",
			startChapter: 4, startVerse: 15,
			endChapter: 5, endVerse: 3,
		}, false)
	})
	t.Run("single_verse", func(t *testing.T) {
		// A single verse is expressed as ch:v-v with the same verse number
		f("John 3:16-16", refRange{
			book: "John", bookSlug: "john",
			startChapter: 3, startVerse: 16,
			endChapter: 3, endVerse: 16,
		}, false)
	})
	t.Run("missing_chapter_verse", func(t *testing.T) { f("John", refRange{}, true) })
	t.Run("malformed_ref", func(t *testing.T) { f("John 1", refRange{}, true) })
	t.Run("empty_string", func(t *testing.T) { f("", refRange{}, true) })
	t.Run("no_book", func(t *testing.T) { f("1:1-10", refRange{}, true) })
	t.Run("inverted_range_same_chapter", func(t *testing.T) { f("John 1:10-1", refRange{}, true) })
	t.Run("inverted_range_cross_chapter", func(t *testing.T) { f("John 2:1-1:10", refRange{}, true) })
}

// ---------------------------------------------------------------------------
// bookSlug tests
// ---------------------------------------------------------------------------

func TestBookSlug(t *testing.T) {
	f := func(book, want string) {
		t.Helper()
		assert.Equal(t, want, bookSlug(book))
	}

	t.Run("single_word", func(t *testing.T) { f("John", "john") })
	t.Run("numbered_book", func(t *testing.T) { f("1 Corinthians", "1-corinthians") })
	t.Run("two_words", func(t *testing.T) { f("1 Thessalonians", "1-thessalonians") })
	t.Run("already_lowercase", func(t *testing.T) { f("matthew", "matthew") })
}

// ---------------------------------------------------------------------------
// tabNameFromRef tests
// ---------------------------------------------------------------------------

func TestTabNameFromRef(t *testing.T) {
	f := func(r refRange, want string) {
		t.Helper()
		assert.Equal(t, want, tabNameFromRef(r))
	}

	t.Run("same_chapter", func(t *testing.T) {
		f(refRange{startChapter: 1, startVerse: 1, endChapter: 1, endVerse: 10}, "1:1 - 1:10")
	})
	t.Run("cross_chapter", func(t *testing.T) {
		f(refRange{startChapter: 1, startVerse: 50, endChapter: 2, endVerse: 10}, "1:50 - 2:10")
	})
	t.Run("single_verse", func(t *testing.T) {
		f(refRange{startChapter: 3, startVerse: 16, endChapter: 3, endVerse: 16}, "3:16 - 3:16")
	})
}

// ---------------------------------------------------------------------------
// parsePassageHTML tests
// ---------------------------------------------------------------------------

// validVerseHTML mirrors the real greekbible.com response for a single verse
// inside the passage-output div.
const validVerseHTML = `<html><body>
<div class="passage-output bg-white shadow-lg border border-stone-300 rounded-sm p-4 lg:p-12 lg:pe-16">
<h2 class="text-xl lg:text-2xl font-semibold mb-4">John 1:1</h2>
<sup>1</sup>
<span data-over-tt="in, on, among" data-tt-placement="bottom" class="word relative word-1">Ἐν </span>
<span data-over-tt="ruler, beginning" data-tt-placement="bottom" class="word relative word-2">ἀρχῇ </span>
<span data-over-tt="I am, exist" data-tt-placement="bottom" class="word relative word-3">ἦν </span>
<span data-over-tt="the" data-tt-placement="bottom" class="word relative word-4">ὁ </span>
<span data-over-tt="a word, speech, divine utterance, analogy" data-tt-placement="bottom" class="word relative word-5">λόγος. </span>
<hr class="my-4">
</div>
</body></html>`

// invalidVerseHTML is what greekbible.com returns for a non-existent verse:
// the passage-output div falls through to the site's guide section instead of
// showing a <sup> + word spans.
const invalidVerseHTML = `<html><body>
<div class="passage-output bg-white shadow-lg border border-stone-300 rounded-sm p-4 lg:p-12 lg:pe-16">
<section class="gh-content gh-canvas is-body p-4 text-stone-600 pb-16" data-svelte-h="svelte-172rub9">
<h2 class="font-semibold text-2xl">Greek NT Guide</h2>
<p class="mb-8 mt-1">How to browse and search the Online Greek New Testament:</p>
</section>
</div>
</body></html>`

func TestParsePassageHTML(t *testing.T) {
	t.Run("valid_verse_extracts_words", func(t *testing.T) {
		words, ok := parsePassageHTML(strings.NewReader(validVerseHTML))
		require.True(t, ok)
		assert.Equal(t, []string{"Ἐν", "ἀρχῇ", "ἦν", "ὁ", "λόγος."}, words)
	})
	t.Run("invalid_verse_returns_false", func(t *testing.T) {
		words, ok := parsePassageHTML(strings.NewReader(invalidVerseHTML))
		assert.False(t, ok)
		assert.Nil(t, words)
	})
}
