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
	// Single-chapter format ("Book ch") is handled upstream by parseSingleChapter
	// before parseRef is ever called, so "John 1" is still invalid here.
	t.Run("malformed_ref", func(t *testing.T) { f("John 1", refRange{}, true) })
	t.Run("empty_string", func(t *testing.T) { f("", refRange{}, true) })
	t.Run("no_book", func(t *testing.T) { f("1:1-10", refRange{}, true) })
	t.Run("inverted_range_same_chapter", func(t *testing.T) { f("John 1:10-1", refRange{}, true) })
	t.Run("inverted_range_cross_chapter", func(t *testing.T) { f("John 2:1-1:10", refRange{}, true) })
	// Whole-chapter format ("Book ch-ch") is handled upstream by parseChapterRange
	// before parseRef is ever called, so it will never parse successfully here.
	// The error message still mentions "Book ch-ch" as a hint about the tool's
	// supported formats, even though this function cannot accept it.
	t.Run("whole_chapter_format_rejected", func(t *testing.T) {
		_, err := parseRef("Ephesians 1-6")
		require.Error(t, err)
	})
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
		f(refRange{startChapter: 1, startVerse: 1, endChapter: 1, endVerse: 10}, "1:1-1:10")
	})
	t.Run("cross_chapter", func(t *testing.T) {
		f(refRange{startChapter: 1, startVerse: 50, endChapter: 2, endVerse: 10}, "1:50-2:10")
	})
	t.Run("single_verse", func(t *testing.T) {
		f(refRange{startChapter: 3, startVerse: 16, endChapter: 3, endVerse: 16}, "3:16-3:16")
	})
}

// ---------------------------------------------------------------------------
// parseChapterRange tests
// ---------------------------------------------------------------------------

func TestParseChapterRange(t *testing.T) {
	f := func(input string, want chapterRange, wantErr bool) {
		t.Helper()
		got, err := parseChapterRange(input)
		if wantErr {
			require.Error(t, err)
			return
		}
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}

	t.Run("simple_range", func(t *testing.T) {
		f("Ephesians 1-6", chapterRange{
			book: "Ephesians", bookSlug: "ephesians",
			startChapter: 1, endChapter: 6,
		}, false)
	})
	t.Run("single_chapter", func(t *testing.T) {
		f("John 1-1", chapterRange{
			book: "John", bookSlug: "john",
			startChapter: 1, endChapter: 1,
		}, false)
	})
	t.Run("multi_word_book", func(t *testing.T) {
		f("1 Corinthians 13-14", chapterRange{
			book: "1 Corinthians", bookSlug: "1-corinthians",
			startChapter: 13, endChapter: 14,
		}, false)
	})
	t.Run("inverted_range", func(t *testing.T) { f("John 5-1", chapterRange{}, true) })
	// Verse-range format must be rejected — it does not match "Book ch-ch"
	t.Run("verse_range_rejected", func(t *testing.T) { f("John 1:1-10", chapterRange{}, true) })
	t.Run("empty_string", func(t *testing.T) { f("", chapterRange{}, true) })
}

// ---------------------------------------------------------------------------
// parseSingleChapter tests
// ---------------------------------------------------------------------------

func TestParseSingleChapter(t *testing.T) {
	f := func(input string, want chapterRange, wantErr bool) {
		t.Helper()
		got, err := parseSingleChapter(input)
		if wantErr {
			require.Error(t, err)
			return
		}
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}

	t.Run("simple", func(t *testing.T) {
		f("Ephesians 1", chapterRange{
			book: "Ephesians", bookSlug: "ephesians",
			startChapter: 1, endChapter: 1,
		}, false)
	})
	t.Run("multi_word_book", func(t *testing.T) {
		f("1 Corinthians 13", chapterRange{
			book: "1 Corinthians", bookSlug: "1-corinthians",
			startChapter: 13, endChapter: 13,
		}, false)
	})
	t.Run("invalid_book", func(t *testing.T) { f("Hezekiah 1", chapterRange{}, true) })
	t.Run("chapter_out_of_range", func(t *testing.T) { f("John 30", chapterRange{}, true) })
	t.Run("empty_string", func(t *testing.T) { f("", chapterRange{}, true) })
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

// ---------------------------------------------------------------------------
// parseChapterHTML tests
// ---------------------------------------------------------------------------

// validChapterHTML mirrors a real greekbible.com chapter page: <sup> verse
// markers and word spans are direct siblings inside the passage-output div.
const validChapterHTML = `<html><body>
<div class="passage-output bg-white shadow-lg border border-stone-300 rounded-sm p-4 lg:p-12 lg:pe-16">
<h2 class="text-xl lg:text-2xl font-semibold mb-4">John 8</h2>
<sup>1</sup>
<span data-over-tt="Jesus" data-tt-placement="bottom" class="word relative word-1">Ἰησοῦς </span>
<span data-over-tt="but" data-tt-placement="bottom" class="word relative word-2">δὲ </span>
<sup>2</sup>
<span data-over-tt="early" data-tt-placement="bottom" class="word relative word-3">Ὄρθρου </span>
<span data-over-tt="again" data-tt-placement="bottom" class="word relative word-4">πάλιν </span>
<span data-over-tt="he came" data-tt-placement="bottom" class="word relative word-5">παρεγένετο </span>
</div>
</body></html>`

func TestParseChapterHTML(t *testing.T) {
	t.Run("groups_words_by_sup_verse_number", func(t *testing.T) {
		verses, ok := parseChapterHTML(strings.NewReader(validChapterHTML))
		require.True(t, ok)
		require.Len(t, verses, 2)
		assert.Equal(t, "1", verses[0].num)
		assert.Equal(t, []string{"Ἰησοῦς", "δὲ"}, verses[0].words)
		assert.Equal(t, "2", verses[1].num)
		assert.Equal(t, []string{"Ὄρθρου", "πάλιν", "παρεγένετο"}, verses[1].words)
	})
	t.Run("guide_page_returns_false", func(t *testing.T) {
		verses, ok := parseChapterHTML(strings.NewReader(invalidVerseHTML))
		assert.False(t, ok)
		assert.Nil(t, verses)
	})
	t.Run("sup_with_no_following_words_is_dropped", func(t *testing.T) {
		// A <sup> at the very end of the div with no word spans should not produce
		// an empty verse entry.
		html := `<html><body>
<div class="passage-output">
<sup>1</sup>
<span class="word relative">Ἰησοῦς </span>
<sup>2</sup>
</div></body></html>`
		verses, ok := parseChapterHTML(strings.NewReader(html))
		require.True(t, ok)
		require.Len(t, verses, 1)
		assert.Equal(t, "1", verses[0].num)
	})
	t.Run("non_numeric_sup_is_ignored", func(t *testing.T) {
		// Footnote markers like '*' or 'a' must not be treated as verse numbers,
		// which would corrupt the verse grouping and filterVerses logic.
		html := `<html><body>
<div class="passage-output">
<sup>1</sup>
<span class="word relative">Ἰησοῦς </span>
<sup>*</sup>
<span class="word relative">δὲ </span>
<sup>2</sup>
<span class="word relative">Ὄρθρου </span>
</div></body></html>`
		verses, ok := parseChapterHTML(strings.NewReader(html))
		require.True(t, ok)
		require.Len(t, verses, 2)
		// The '*' sup is ignored; its following word span is attached to verse 1.
		assert.Equal(t, "1", verses[0].num)
		assert.Equal(t, []string{"Ἰησοῦς", "δὲ"}, verses[0].words)
		assert.Equal(t, "2", verses[1].num)
		assert.Equal(t, []string{"Ὄρθρου"}, verses[1].words)
	})
}

// ---------------------------------------------------------------------------
// filterVerses tests
// ---------------------------------------------------------------------------

func TestFilterVerses(t *testing.T) {
	allVerses := []verse{
		{num: "1"}, {num: "2"}, {num: "3"}, {num: "4"}, {num: "5"},
	}

	f := func(ch int, r refRange, wantNums []string) {
		t.Helper()
		got := filterVerses(allVerses, ch, r)
		var nums []string
		for _, v := range got {
			nums = append(nums, v.num)
		}
		assert.Equal(t, wantNums, nums)
	}

	r := refRange{startChapter: 2, startVerse: 2, endChapter: 4, endVerse: 4}
	t.Run("start_chapter_trims_leading_verses", func(t *testing.T) {
		f(2, r, []string{"2", "3", "4", "5"})
	})
	t.Run("middle_chapter_unchanged", func(t *testing.T) {
		f(3, r, []string{"1", "2", "3", "4", "5"})
	})
	t.Run("end_chapter_trims_trailing_verses", func(t *testing.T) {
		f(4, r, []string{"1", "2", "3", "4"})
	})
	t.Run("same_chapter_trims_both_ends", func(t *testing.T) {
		same := refRange{startChapter: 1, startVerse: 2, endChapter: 1, endVerse: 4}
		f(1, same, []string{"2", "3", "4"})
	})
}
