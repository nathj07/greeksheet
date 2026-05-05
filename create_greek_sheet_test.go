package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseVerses(t *testing.T) {
	f := func(input string, expected []verse) {
		t.Helper()
		assert.Equal(t, expected, parseVerses(input))
	}

	t.Run("single_verse", func(t *testing.T) {
		f("1 Ἐὰν ταῖς γλώσσαις", []verse{
			{num: "1", words: []string{"Ἐὰν", "ταῖς", "γλώσσαις"}},
		})
	})

	t.Run("multiple_verses", func(t *testing.T) {
		f("1 word one. 2 word two", []verse{
			{num: "1", words: []string{"word", "one."}},
			{num: "2", words: []string{"word", "two"}},
		})
	})

	t.Run("empty_input", func(t *testing.T) {
		f("", nil)
	})

	t.Run("no_verse_numbers", func(t *testing.T) {
		f("just some words with no numbers", nil)
	})
}

func TestParseInputFile(t *testing.T) {
	content := "# 1 Corinthians 13\n1 word one two. 2 word three.\n# 1 Corinthians 14\n1 other words here\n"
	tmp := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(tmp, []byte(content), 0o600))

	sections, err := parseInputFile(tmp)
	require.NoError(t, err)
	require.Len(t, sections, 4)

	assert.Equal(t, "1 Corinthians 13", sections[0].heading)
	assert.Equal(t, "1 Corinthians 14", sections[2].heading)

	require.Len(t, sections[1].verses, 2)
	assert.Equal(t, "1", sections[1].verses[0].num)
	assert.Equal(t, "2", sections[1].verses[1].num)

	require.Len(t, sections[3].verses, 1)
	assert.Equal(t, "1", sections[3].verses[0].num)
}

func TestDeriveTabName(t *testing.T) {
	f := func(sections []section, expected string) {
		t.Helper()
		assert.Equal(t, expected, deriveTabName(sections))
	}

	t.Run("single_chapter", func(t *testing.T) {
		f([]section{
			{heading: "1 Corinthians 13"},
			{verses: []verse{{num: "1"}, {num: "13"}}},
		}, "13:1 - 13:13")
	})

	t.Run("multi_chapter", func(t *testing.T) {
		f([]section{
			{heading: "1 Corinthians 13"},
			{verses: []verse{{num: "1"}, {num: "13"}}},
			{heading: "1 Corinthians 14"},
			{verses: []verse{{num: "1"}, {num: "40"}}},
		}, "13:1 - 14:40")
	})

	t.Run("no_heading", func(t *testing.T) {
		f([]section{
			{verses: []verse{{num: "3"}, {num: "7"}}},
		}, "1:3 - 1:7")
	})

	t.Run("no_verses", func(t *testing.T) {
		f([]section{{heading: "Romans 1"}}, "sheet")
	})

	t.Run("single_verse", func(t *testing.T) {
		f([]section{
			{heading: "John 3"},
			{verses: []verse{{num: "16"}}},
		}, "3:16")
	})

	t.Run("heading_without_trailing_number", func(t *testing.T) {
		// Headings like "Romans" have no trailing chapter digit; chapter defaults to "1".
		f([]section{
			{heading: "Romans"},
			{verses: []verse{{num: "1"}, {num: "32"}}},
		}, "1:1 - 1:32")
	})
}

func TestBuildSheetData_headingRow(t *testing.T) {
	sections := []section{{heading: "1 Corinthians 13"}}
	d := buildSheetData(sections)

	require.Len(t, d.rows, 1)
	assert.Equal(t, []any{"1 Corinthians 13"}, d.rows[0])
	require.Len(t, d.boldRequests, 1)
}

func TestBuildSheetData_verseBlock(t *testing.T) {
	sections := []section{{verses: []verse{{num: "1", words: []string{"word", "two"}}}}}
	d := buildSheetData(sections)

	// verse row + 2 parsing rows + I + T + C + N = 7 rows
	assert.Len(t, d.rows, 7)

	// Verse row: num then words
	assert.Equal(t, []any{"1", "word", "two"}, d.rows[0])

	// Parsing rows are unlabelled (empty first cell) and span all word columns
	assert.Equal(t, any(nil), d.rows[1][0])
	assert.Equal(t, any(nil), d.rows[2][0])

	// I row label
	assert.Equal(t, "I", d.rows[3][0])

	// T row label
	assert.Equal(t, "T", d.rows[4][0])

	// C and N are merged (2 words → merge requested)
	assert.Len(t, d.mergeReqs, 4) // I, T, C, N rows all get merged cells
}
