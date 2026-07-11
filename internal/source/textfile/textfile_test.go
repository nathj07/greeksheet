package textfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nathj07/greeksheet/internal/document"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseVerses(t *testing.T) {
	f := func(input string, expected []document.Verse) {
		t.Helper()
		assert.Equal(t, expected, parseVerses(input))
	}

	t.Run("single_verse", func(t *testing.T) {
		f("1 Ἐὰν ταῖς γλώσσαις", []document.Verse{
			{Num: "1", Words: []string{"Ἐὰν", "ταῖς", "γλώσσαις"}},
		})
	})

	t.Run("multiple_verses", func(t *testing.T) {
		f("1 word one. 2 word two", []document.Verse{
			{Num: "1", Words: []string{"word", "one."}},
			{Num: "2", Words: []string{"word", "two"}},
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

	assert.Equal(t, "1 Corinthians 13", sections[0].Heading)
	assert.Equal(t, "1 Corinthians 14", sections[2].Heading)

	require.Len(t, sections[1].Verses, 2)
	assert.Equal(t, "1", sections[1].Verses[0].Num)
	assert.Equal(t, "2", sections[1].Verses[1].Num)

	require.Len(t, sections[3].Verses, 1)
	assert.Equal(t, "1", sections[3].Verses[0].Num)
}

func TestDeriveTabName(t *testing.T) {
	f := func(sections []document.Section, expected string) {
		t.Helper()
		assert.Equal(t, expected, deriveTabName(sections))
	}

	t.Run("single_chapter", func(t *testing.T) {
		f([]document.Section{
			{Heading: "1 Corinthians 13"},
			{Verses: []document.Verse{{Num: "1"}, {Num: "13"}}},
		}, "13:1 - 13:13")
	})

	t.Run("multi_chapter", func(t *testing.T) {
		f([]document.Section{
			{Heading: "1 Corinthians 13"},
			{Verses: []document.Verse{{Num: "1"}, {Num: "13"}}},
			{Heading: "1 Corinthians 14"},
			{Verses: []document.Verse{{Num: "1"}, {Num: "40"}}},
		}, "13:1 - 14:40")
	})

	t.Run("no_heading", func(t *testing.T) {
		f([]document.Section{
			{Verses: []document.Verse{{Num: "3"}, {Num: "7"}}},
		}, "1:3 - 1:7")
	})

	t.Run("no_verses", func(t *testing.T) {
		f([]document.Section{{Heading: "Romans 1"}}, "sheet")
	})

	t.Run("single_verse", func(t *testing.T) {
		f([]document.Section{
			{Heading: "John 3"},
			{Verses: []document.Verse{{Num: "16"}}},
		}, "3:16")
	})

	t.Run("heading_without_trailing_number", func(t *testing.T) {
		// Headings like "Romans" have no trailing chapter digit; chapter defaults to "1".
		f([]document.Section{
			{Heading: "Romans"},
			{Verses: []document.Verse{{Num: "1"}, {Num: "32"}}},
		}, "1:1 - 1:32")
	})
}
