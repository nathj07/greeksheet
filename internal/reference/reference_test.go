package reference

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRef(t *testing.T) {
	f := func(input string, want RefRange, wantErr bool) {
		t.Helper()
		got, err := ParseRef(input)
		if wantErr {
			require.Error(t, err)
			return
		}
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}

	t.Run("same_chapter_shorthand", func(t *testing.T) {
		f("John 1:1-10", RefRange{
			Book: "John", BookSlug: "john",
			StartChapter: 1, StartVerse: 1,
			EndChapter: 1, EndVerse: 10,
		}, false)
	})
	t.Run("cross_chapter", func(t *testing.T) {
		f("John 1:50-2:10", RefRange{
			Book: "John", BookSlug: "john",
			StartChapter: 1, StartVerse: 50,
			EndChapter: 2, EndVerse: 10,
		}, false)
	})
	t.Run("multi_word_book", func(t *testing.T) {
		f("1 Corinthians 13:1-13", RefRange{
			Book: "1 Corinthians", BookSlug: "1-corinthians",
			StartChapter: 13, StartVerse: 1,
			EndChapter: 13, EndVerse: 13,
		}, false)
	})
	t.Run("multi_word_book_cross_chapter", func(t *testing.T) {
		f("1 Thessalonians 4:15-5:3", RefRange{
			Book: "1 Thessalonians", BookSlug: "1-thessalonians",
			StartChapter: 4, StartVerse: 15,
			EndChapter: 5, EndVerse: 3,
		}, false)
	})
	t.Run("single_verse", func(t *testing.T) {
		// A single verse is expressed as ch:v-v with the same verse number
		f("John 3:16-16", RefRange{
			Book: "John", BookSlug: "john",
			StartChapter: 3, StartVerse: 16,
			EndChapter: 3, EndVerse: 16,
		}, false)
	})
	t.Run("missing_chapter_verse", func(t *testing.T) { f("John", RefRange{}, true) })
	// Single-chapter format ("Book ch") is handled upstream by ParseSingleChapter
	// before ParseRef is ever called, so "John 1" is still invalid here.
	t.Run("malformed_ref", func(t *testing.T) { f("John 1", RefRange{}, true) })
	t.Run("empty_string", func(t *testing.T) { f("", RefRange{}, true) })
	t.Run("no_book", func(t *testing.T) { f("1:1-10", RefRange{}, true) })
	t.Run("inverted_range_same_chapter", func(t *testing.T) { f("John 1:10-1", RefRange{}, true) })
	t.Run("inverted_range_cross_chapter", func(t *testing.T) { f("John 2:1-1:10", RefRange{}, true) })
	// Whole-chapter format ("Book ch-ch") is handled upstream by
	// ParseChapterRange before ParseRef is ever called, so it will never parse
	// successfully here. The error message still mentions "Book ch-ch" as a hint
	// about the tool's supported formats, even though this function cannot accept it.
	t.Run("whole_chapter_format_rejected", func(t *testing.T) {
		_, err := ParseRef("Ephesians 1-6")
		require.Error(t, err)
	})
}

func TestBookSlug(t *testing.T) {
	f := func(book, want string) {
		t.Helper()
		assert.Equal(t, want, BookSlug(book))
	}

	t.Run("single_word", func(t *testing.T) { f("John", "john") })
	t.Run("numbered_book", func(t *testing.T) { f("1 Corinthians", "1-corinthians") })
	t.Run("two_words", func(t *testing.T) { f("1 Thessalonians", "1-thessalonians") })
	t.Run("already_lowercase", func(t *testing.T) { f("matthew", "matthew") })
}

func TestTabName(t *testing.T) {
	f := func(r RefRange, want string) {
		t.Helper()
		assert.Equal(t, want, TabName(r))
	}

	t.Run("same_chapter", func(t *testing.T) {
		f(RefRange{StartChapter: 1, StartVerse: 1, EndChapter: 1, EndVerse: 10}, "1:1-1:10")
	})
	t.Run("cross_chapter", func(t *testing.T) {
		f(RefRange{StartChapter: 1, StartVerse: 50, EndChapter: 2, EndVerse: 10}, "1:50-2:10")
	})
	t.Run("single_verse", func(t *testing.T) {
		f(RefRange{StartChapter: 3, StartVerse: 16, EndChapter: 3, EndVerse: 16}, "3:16-3:16")
	})
}

func TestParseChapterRange(t *testing.T) {
	f := func(input string, want ChapterRange, wantErr bool) {
		t.Helper()
		got, err := ParseChapterRange(input)
		if wantErr {
			require.Error(t, err)
			return
		}
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}

	t.Run("simple_range", func(t *testing.T) {
		f("Ephesians 1-6", ChapterRange{
			Book: "Ephesians", BookSlug: "ephesians",
			StartChapter: 1, EndChapter: 6,
		}, false)
	})
	t.Run("single_chapter", func(t *testing.T) {
		f("John 1-1", ChapterRange{
			Book: "John", BookSlug: "john",
			StartChapter: 1, EndChapter: 1,
		}, false)
	})
	t.Run("multi_word_book", func(t *testing.T) {
		f("1 Corinthians 13-14", ChapterRange{
			Book: "1 Corinthians", BookSlug: "1-corinthians",
			StartChapter: 13, EndChapter: 14,
		}, false)
	})
	t.Run("inverted_range", func(t *testing.T) { f("John 5-1", ChapterRange{}, true) })
	// Verse-range format must be rejected — it does not match "Book ch-ch"
	t.Run("verse_range_rejected", func(t *testing.T) { f("John 1:1-10", ChapterRange{}, true) })
	t.Run("empty_string", func(t *testing.T) { f("", ChapterRange{}, true) })
}

func TestParseSingleChapter(t *testing.T) {
	f := func(input string, want ChapterRange, wantErr bool) {
		t.Helper()
		got, err := ParseSingleChapter(input)
		if wantErr {
			require.Error(t, err)
			return
		}
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}

	t.Run("simple", func(t *testing.T) {
		f("Ephesians 1", ChapterRange{
			Book: "Ephesians", BookSlug: "ephesians",
			StartChapter: 1, EndChapter: 1,
		}, false)
	})
	t.Run("multi_word_book", func(t *testing.T) {
		f("1 Corinthians 13", ChapterRange{
			Book: "1 Corinthians", BookSlug: "1-corinthians",
			StartChapter: 13, EndChapter: 13,
		}, false)
	})
	t.Run("invalid_book", func(t *testing.T) { f("Hezekiah 1", ChapterRange{}, true) })
	t.Run("chapter_out_of_range", func(t *testing.T) { f("John 30", ChapterRange{}, true) })
	t.Run("empty_string", func(t *testing.T) { f("", ChapterRange{}, true) })
}
