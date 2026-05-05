package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
