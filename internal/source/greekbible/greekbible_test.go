package greekbible

import (
	"strings"
	"testing"

	"github.com/nathj07/greeksheet/internal/document"
	"github.com/nathj07/greeksheet/internal/reference"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		assert.Equal(t, "1", verses[0].Num)
		assert.Equal(t, []string{"Ἰησοῦς", "δὲ"}, verses[0].Words)
		assert.Equal(t, "2", verses[1].Num)
		assert.Equal(t, []string{"Ὄρθρου", "πάλιν", "παρεγένετο"}, verses[1].Words)
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
		assert.Equal(t, "1", verses[0].Num)
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
		assert.Equal(t, "1", verses[0].Num)
		assert.Equal(t, []string{"Ἰησοῦς", "δὲ"}, verses[0].Words)
		assert.Equal(t, "2", verses[1].Num)
		assert.Equal(t, []string{"Ὄρθρου"}, verses[1].Words)
	})
}

// ---------------------------------------------------------------------------
// filterVerses tests
// ---------------------------------------------------------------------------

func TestFilterVerses(t *testing.T) {
	allVerses := []document.Verse{
		{Num: "1"}, {Num: "2"}, {Num: "3"}, {Num: "4"}, {Num: "5"},
	}

	f := func(ch int, r reference.RefRange, wantNums []string) {
		t.Helper()
		got := filterVerses(allVerses, ch, r)
		var nums []string
		for _, v := range got {
			nums = append(nums, v.Num)
		}
		assert.Equal(t, wantNums, nums)
	}

	r := reference.RefRange{StartChapter: 2, StartVerse: 2, EndChapter: 4, EndVerse: 4}
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
		same := reference.RefRange{StartChapter: 1, StartVerse: 2, EndChapter: 1, EndVerse: 4}
		f(1, same, []string{"2", "3", "4"})
	})
}
