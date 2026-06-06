package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/sheets/v4"
)

func TestBuildSheetData_headingRow(t *testing.T) {
	sections := []section{{heading: "1 Corinthians 13"}}
	d := buildSheetData(sections)

	require.Len(t, d.rows, 1)
	assert.Equal(t, []any{"1 Corinthians 13"}, d.rows[0])
	require.Len(t, d.boldRequests, 1)
	assert.Len(t, d.alignVertReqs, 1)
	assert.Len(t, d.textWrapReqs, 1)
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
	assert.Len(t, d.alignVertReqs, 1)
	assert.Len(t, d.textWrapReqs, 1)
}

// TestBuildSheetData_wholeSheetFormattingRequests checks that buildSheetData always emits exactly
// one whole-sheet vert-align and one whole-sheet wrap request, regardless of content. The range
// must have no row or column bounds set — leaving all index fields at zero causes the Sheets API
// to treat them as unbounded (they are omitted from the JSON payload).
func TestBuildSheetData_wholeSheetFormattingRequests(t *testing.T) {
	sections := []section{{heading: "Romans 8"}}
	d := buildSheetData(sections)

	require.Len(t, d.alignVertReqs, 1)
	ar := d.alignVertReqs[0].RepeatCell
	require.NotNil(t, ar)
	assert.Equal(t, "TOP", ar.Cell.UserEnteredFormat.VerticalAlignment)
	assert.Equal(t, "userEnteredFormat.verticalAlignment", ar.Fields)
	// SheetId: 0 is the placeholder rewritten by patchSheetID; all bound fields zero = whole sheet.
	assert.Equal(t, &sheets.GridRange{SheetId: 0}, ar.Range)

	require.Len(t, d.textWrapReqs, 1)
	wr := d.textWrapReqs[0].RepeatCell
	require.NotNil(t, wr)
	assert.Equal(t, "WRAP", wr.Cell.UserEnteredFormat.WrapStrategy)
	assert.Equal(t, "userEnteredFormat.wrapStrategy", wr.Fields)
	assert.Equal(t, &sheets.GridRange{SheetId: 0}, wr.Range)
}

// TestTextNumberFormatReq verifies that the text number format request targets columns B onwards
// only. StartColumnIndex: 1 skips column A (row labels / verse numbers); leaving EndColumnIndex
// at zero means the Sheets API treats it as unbounded when serialised to JSON.
func TestTextNumberFormatReq(t *testing.T) {
	req := textNumberFormatReq(42)

	require.NotNil(t, req.RepeatCell)
	assert.Equal(t, &sheets.GridRange{SheetId: 42, StartColumnIndex: 1}, req.RepeatCell.Range)

	nf := req.RepeatCell.Cell.UserEnteredFormat.NumberFormat
	require.NotNil(t, nf)
	assert.Equal(t, "TEXT", nf.Type)
	assert.Equal(t, "userEnteredFormat.numberFormat", req.RepeatCell.Fields)
}

// TestPatchSheetID_patchesAllSlices confirms that patchSheetID rewrites the placeholder SheetId
// in every formatting slice, including the newer alignVertReqs and textWrapReqs additions.
func TestPatchSheetID_patchesAllSlices(t *testing.T) {
	d := buildSheetData([]section{{heading: "test"}})

	// All formatting requests use SheetId: 0 as a placeholder before the real id is known.
	require.Equal(t, int64(0), d.alignVertReqs[0].RepeatCell.Range.SheetId)
	require.Equal(t, int64(0), d.textWrapReqs[0].RepeatCell.Range.SheetId)

	patchSheetID(&d, 99)

	assert.Equal(t, int64(99), d.alignVertReqs[0].RepeatCell.Range.SheetId)
	assert.Equal(t, int64(99), d.textWrapReqs[0].RepeatCell.Range.SheetId)
}
