package xlsx

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nathj07/greeksheet/internal/document"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	excelize "github.com/xuri/excelize/v2"
)

// ---------------------------------------------------------------------------
// sanitiseSheetName
// ---------------------------------------------------------------------------

func TestSanitiseSheetName(t *testing.T) {
	f := func(input, want string) {
		t.Helper()
		assert.Equal(t, want, sanitiseSheetName(input))
	}

	t.Run("plain_chapter_number", func(t *testing.T) { f("1", "1") })
	t.Run("verse_range_with_colons", func(t *testing.T) { f("1:1-1:14", "1.1-1.14") })
	t.Run("cross_chapter_range", func(t *testing.T) { f("3:36-4:5", "3.36-4.5") })
	t.Run("backslash_replaced", func(t *testing.T) { f("a\\b", "a_b") })
	t.Run("forbidden_chars_replaced", func(t *testing.T) { f("a[b]c?d*e", "a_b_c_d_e") })
	t.Run("empty_falls_back_to_Sheet", func(t *testing.T) { f("", "Sheet") })
	t.Run("truncated_to_31_chars", func(t *testing.T) {
		long := "123456789012345678901234567890XY" // 32 chars
		f(long, "123456789012345678901234567890X") // first 31
	})
}

// ---------------------------------------------------------------------------
// Target.Render — creates a new file when the path does not exist
// ---------------------------------------------------------------------------

func TestRender_createsNewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "John 1.xlsx")
	target := New(Options{File: path})

	doc := document.Document{
		Title: "John 1",
		Tabs: []document.Tab{{
			// Tab names use the verse-range format; colons are replaced with
			// periods in Excel sheet names.
			Name:     "1:1-1:5",
			Sections: []document.Section{{Verses: []document.Verse{{Num: "1", Words: []string{"Ἐν", "ἀρχῇ"}}}}},
		}},
	}

	got, err := target.Render(context.Background(), doc)
	require.NoError(t, err)
	assert.Equal(t, path, got)

	_, statErr := os.Stat(path)
	require.NoError(t, statErr, "file should exist on disk")

	f, err := excelize.OpenFile(path)
	require.NoError(t, err)
	defer f.Close()
	assert.Contains(t, f.GetSheetList(), "1.1-1.5", "colon in tab name should be replaced with period")
}

func TestRender_createsReadableXlsx(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.xlsx")
	target := New(Options{File: path})

	doc := document.Document{
		Title: "Test Sheet",
		Tabs: []document.Tab{{
			Name: "tab1",
			Sections: []document.Section{
				{Heading: "Chapter 1"},
				{Verses: []document.Verse{{Num: "1", Words: []string{"word", "two"}}}},
			},
		}},
	}

	_, err := target.Render(context.Background(), doc)
	require.NoError(t, err)

	// Re-open the file and verify the content is readable.
	f, err := excelize.OpenFile(path)
	require.NoError(t, err)
	defer f.Close()

	heading, err := f.GetCellValue("tab1", "A1")
	require.NoError(t, err)
	assert.Equal(t, "Chapter 1", heading)

	verseNum, err := f.GetCellValue("tab1", "A2")
	require.NoError(t, err)
	assert.Equal(t, "1", verseNum)
}

// ---------------------------------------------------------------------------
// Target.Render — appends a tab to an existing file
// ---------------------------------------------------------------------------

func TestRender_appendsTabToExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "study.xlsx")
	target := New(Options{File: path})

	doc1 := document.Document{
		Title: "Study",
		Tabs: []document.Tab{{
			Name:     "ch1",
			Sections: []document.Section{{Verses: []document.Verse{{Num: "1", Words: []string{"first"}}}}},
		}},
	}
	doc2 := document.Document{
		Title: "Study",
		Tabs: []document.Tab{{
			Name:     "ch2",
			Sections: []document.Section{{Verses: []document.Verse{{Num: "1", Words: []string{"second"}}}}},
		}},
	}

	_, err := target.Render(context.Background(), doc1)
	require.NoError(t, err)

	_, err = target.Render(context.Background(), doc2)
	require.NoError(t, err)

	f, err := excelize.OpenFile(path)
	require.NoError(t, err)
	defer f.Close()

	sheets := f.GetSheetList()
	assert.Contains(t, sheets, "ch1", "first sheet should still be present")
	assert.Contains(t, sheets, "ch2", "second sheet should have been appended")
}

// ---------------------------------------------------------------------------
// Target.Render — multiple tabs in one document
// ---------------------------------------------------------------------------

func TestRender_multipleTabsInDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ephesians.xlsx")
	target := New(Options{File: path})

	doc := document.Document{
		Title: "Ephesians",
		Tabs: []document.Tab{
			{
				Name:     "1",
				Sections: []document.Section{{Verses: []document.Verse{{Num: "1", Words: []string{"alpha"}}}}},
			},
			{
				Name:     "2",
				Sections: []document.Section{{Verses: []document.Verse{{Num: "1", Words: []string{"beta"}}}}},
			},
		},
	}

	_, err := target.Render(context.Background(), doc)
	require.NoError(t, err)

	f, err := excelize.OpenFile(path)
	require.NoError(t, err)
	defer f.Close()

	sheets := f.GetSheetList()
	assert.Contains(t, sheets, "1")
	assert.Contains(t, sheets, "2")
}

// ---------------------------------------------------------------------------
// Target.Render — error when File path is empty
// ---------------------------------------------------------------------------

func TestRender_errorsWithEmptyFilePath(t *testing.T) {
	target := New(Options{})
	doc := document.Document{Title: "t", Tabs: []document.Tab{{Name: "t"}}}
	_, err := target.Render(context.Background(), doc)
	require.ErrorContains(t, err, "File path must be set")
}
