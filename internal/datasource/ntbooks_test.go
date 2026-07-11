package datasource_test

import (
	"testing"

	"github.com/nathj07/greeksheet/internal/datasource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRef(t *testing.T) {
	f := func(book string, startCh, startV, endCh, endV int, wantErr string) {
		t.Helper()
		err := datasource.ValidateRef(book, startCh, startV, endCh, endV)
		if wantErr == "" {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
			assert.Contains(t, err.Error(), wantErr)
		}
	}

	// --- valid references ---
	t.Run("john_1:1-14", func(t *testing.T) { f("John", 1, 1, 1, 14, "") })
	t.Run("john_cross_chapter", func(t *testing.T) { f("John", 1, 50, 2, 10, "") })
	t.Run("ephesians_full_chapter", func(t *testing.T) { f("Ephesians", 1, 1, 6, 24, "") })
	t.Run("philemon_single_chapter", func(t *testing.T) { f("Philemon", 1, 1, 1, 25, "") })
	t.Run("revelation_last_verse", func(t *testing.T) { f("Revelation", 22, 21, 22, 21, "") })
	t.Run("case_insensitive_lowercase", func(t *testing.T) { f("john", 3, 16, 3, 16, "") })
	t.Run("case_insensitive_uppercase", func(t *testing.T) { f("JOHN", 3, 16, 3, 16, "") })
	t.Run("numbered_book", func(t *testing.T) { f("1 Corinthians", 13, 1, 13, 13, "") })

	// --- unknown book ---
	t.Run("unknown_book", func(t *testing.T) { f("Genesis", 1, 1, 1, 10, "not a recognised New Testament book") })
	t.Run("misspelled_book", func(t *testing.T) { f("Johnathan", 1, 1, 1, 10, "not a recognised New Testament book") })

	// --- inverted range ---
	t.Run("inverted_chapters", func(t *testing.T) { f("John", 3, 1, 2, 10, "start 3:1 must not exceed end 2:10") })
	t.Run("inverted_verses_same_chapter", func(t *testing.T) { f("John", 1, 10, 1, 5, "start 1:10 must not exceed end 1:5") })

	// --- chapter out of range ---
	t.Run("start_chapter_too_high", func(t *testing.T) { f("Ephesians", 7, 1, 7, 1, "start chapter 7 is out of range") })
	t.Run("start_chapter_zero", func(t *testing.T) { f("Ephesians", 0, 1, 1, 1, "start chapter 0 is out of range") })
	t.Run("end_chapter_too_high", func(t *testing.T) { f("Ephesians", 1, 1, 7, 1, "end chapter 7 is out of range") })

	// --- verse out of range ---
	t.Run("start_verse_too_high", func(t *testing.T) { f("John", 1, 52, 1, 52, "start verse 52 is out of range") })
	t.Run("start_verse_zero", func(t *testing.T) { f("John", 1, 0, 1, 10, "start verse 0 is out of range") })
	t.Run("end_verse_zero", func(t *testing.T) { f("John", 1, 1, 1, 0, "start 1:1 must not exceed end 1:0") })
	t.Run("end_verse_too_high", func(t *testing.T) { f("John", 1, 1, 1, 52, "end verse 52 is out of range") })
	t.Run("philemon_verse_too_high", func(t *testing.T) { f("Philemon", 1, 1, 1, 26, "end verse 26 is out of range") })
}

func TestValidateChapterRange(t *testing.T) {
	f := func(book string, startCh, endCh int, wantErr string) {
		t.Helper()
		err := datasource.ValidateChapterRange(book, startCh, endCh)
		if wantErr == "" {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
			assert.Contains(t, err.Error(), wantErr)
		}
	}

	// --- valid ranges ---
	t.Run("ephesians_1-6", func(t *testing.T) { f("Ephesians", 1, 6, "") })
	t.Run("ephesians_single_chapter", func(t *testing.T) { f("Ephesians", 3, 3, "") })
	t.Run("revelation_all", func(t *testing.T) { f("Revelation", 1, 22, "") })
	t.Run("case_insensitive", func(t *testing.T) { f("ephesians", 1, 6, "") })

	// --- unknown book ---
	t.Run("unknown_book", func(t *testing.T) { f("Genesis", 1, 5, "not a recognised New Testament book") })

	// --- inverted range ---
	t.Run("inverted_range", func(t *testing.T) { f("Ephesians", 5, 2, "start chapter 5 must not exceed end chapter 2") })

	// --- chapter out of range ---
	t.Run("start_chapter_zero", func(t *testing.T) { f("Ephesians", 0, 1, "start chapter 0 is out of range") })
	t.Run("start_chapter_too_high", func(t *testing.T) { f("Ephesians", 7, 7, "start chapter 7 is out of range") })
	t.Run("end_chapter_too_high", func(t *testing.T) { f("Ephesians", 1, 7, "end chapter 7 is out of range") })
	t.Run("philemon_chapter_2", func(t *testing.T) { f("Philemon", 1, 2, "end chapter 2 is out of range") })
}

// TestNTBooksCompleteness verifies all 27 NT books are present and each has at
// least one chapter with at least one verse, catching any accidental omissions.
func TestNTBooksCompleteness(t *testing.T) {
	books := []string{
		"Matthew", "Mark", "Luke", "John", "Acts",
		"Romans", "1 Corinthians", "2 Corinthians", "Galatians", "Ephesians",
		"Philippians", "Colossians", "1 Thessalonians", "2 Thessalonians",
		"1 Timothy", "2 Timothy", "Titus", "Philemon",
		"Hebrews", "James", "1 Peter", "2 Peter",
		"1 John", "2 John", "3 John", "Jude", "Revelation",
	}
	assert.Len(t, books, 27, "NT has 27 books")

	for _, book := range books {
		t.Run(book, func(t *testing.T) {
			// chapter 1, verse 1 must always be valid for every NT book
			err := datasource.ValidateRef(book, 1, 1, 1, 1)
			require.NoError(t, err, "expected chapter 1 verse 1 to be valid for %s", book)
		})
	}
}
