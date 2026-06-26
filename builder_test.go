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

	// verse row + 2 parsing rows + O + I + T + C + N = 8 rows
	assert.Len(t, d.rows, 8)

	// Verse row: num then words
	assert.Equal(t, []any{"1", "word", "two"}, d.rows[0])

	// Parsing rows are unlabelled (empty first cell) and span all word columns
	assert.Equal(t, any(nil), d.rows[1][0])
	assert.Equal(t, any(nil), d.rows[2][0])

	// O row: label then joined original Greek text
	assert.Equal(t, "O", d.rows[3][0])
	assert.Equal(t, "word two", d.rows[3][1])

	// I row label
	assert.Equal(t, "I", d.rows[4][0])

	// T row label
	assert.Equal(t, "T", d.rows[5][0])

	// O, I, T, C, N rows all get merged cells (2 words → merge requested)
	assert.Len(t, d.mergeReqs, 5)
	// verse + O + I + T rows each have a background colour; C and N do not
	assert.Len(t, d.bgRequests, 4)
	assert.Len(t, d.alignVertReqs, 1)
	assert.Len(t, d.textWrapReqs, 1)
}

func TestBuildSheetData_verseBlock_singleWord(t *testing.T) {
	sections := []section{{verses: []verse{{num: "1", words: []string{"λόγος"}}}}}
	d := buildSheetData(sections)

	assert.Len(t, d.rows, 8)

	// O row holds the single word with no merge needed
	assert.Equal(t, "O", d.rows[3][0])
	assert.Equal(t, "λόγος", d.rows[3][1])

	// No merges when there is only one word column
	assert.Len(t, d.mergeReqs, 0)
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

// TestPatchSheetID_patchesColWidthReqs confirms that patchSheetID also rewrites SheetId in
// colWidthReqs, which use UpdateDimensionProperties rather than RepeatCell.
func TestPatchSheetID_patchesColWidthReqs(t *testing.T) {
	d := buildSheetData([]section{{verses: []verse{{num: "1", words: []string{"word"}}}}})
	require.NotEmpty(t, d.colWidthReqs)
	require.Equal(t, int64(0), d.colWidthReqs[0].UpdateDimensionProperties.Range.SheetId)

	patchSheetID(&d, 77)

	for _, req := range d.colWidthReqs {
		assert.Equal(t, int64(77), req.UpdateDimensionProperties.Range.SheetId)
	}
}

// TestColWidthReq verifies the structure of the UpdateDimensionProperties request it produces.
func TestColWidthReq(t *testing.T) {
	req := colWidthReq(42, 3, 80)

	require.NotNil(t, req.UpdateDimensionProperties)
	udp := req.UpdateDimensionProperties
	assert.Equal(t, "COLUMNS", udp.Range.Dimension)
	assert.Equal(t, int64(42), udp.Range.SheetId)
	assert.Equal(t, int64(3), udp.Range.StartIndex)
	assert.Equal(t, int64(4), udp.Range.EndIndex)
	assert.Equal(t, int64(80), udp.Properties.PixelSize)
	assert.Equal(t, "pixelSize", udp.Fields)
}

// TestBuildSheetData_colWidthReqs_singleVerse checks that each word column gets a width request
// sized to the widest word in that column position.
//
// "word" = 4 runes → max(4×9+16, 50) = 52 px
// "two"  = 3 runes → max(3×9+16, 50) = 50 px
func TestBuildSheetData_colWidthReqs_singleVerse(t *testing.T) {
	sections := []section{{verses: []verse{{num: "1", words: []string{"word", "two"}}}}}
	d := buildSheetData(sections)

	require.Len(t, d.colWidthReqs, 2)

	col1 := d.colWidthReqs[0].UpdateDimensionProperties
	require.NotNil(t, col1)
	assert.Equal(t, int64(1), col1.Range.StartIndex)
	assert.Equal(t, int64(2), col1.Range.EndIndex)
	assert.Equal(t, int64(52), col1.Properties.PixelSize)

	col2 := d.colWidthReqs[1].UpdateDimensionProperties
	require.NotNil(t, col2)
	assert.Equal(t, int64(2), col2.Range.StartIndex)
	assert.Equal(t, int64(3), col2.Range.EndIndex)
	assert.Equal(t, int64(50), col2.Properties.PixelSize)
}

// TestBuildSheetData_colWidthReqs_maxAcrossVerses verifies that the widest word across all
// verses wins for each column position.
//
// verse 1: col 1="hello"(5), col 2="hi"(2)
// verse 2: col 1="ok"(2),    col 2="world"(5)
// Both columns: max = 5 runes → max(5×9+16, 50) = 61 px
func TestBuildSheetData_colWidthReqs_maxAcrossVerses(t *testing.T) {
	sections := []section{{verses: []verse{
		{num: "1", words: []string{"hello", "hi"}},
		{num: "2", words: []string{"ok", "world"}},
	}}}
	d := buildSheetData(sections)

	require.Len(t, d.colWidthReqs, 2)
	assert.Equal(t, int64(61), d.colWidthReqs[0].UpdateDimensionProperties.Properties.PixelSize)
	assert.Equal(t, int64(61), d.colWidthReqs[1].UpdateDimensionProperties.Properties.PixelSize)
}

// TestBuildSheetData_colWidthReqs_headingOnly confirms no width requests are generated when
// there are no verse blocks (and therefore no word columns).
func TestBuildSheetData_colWidthReqs_headingOnly(t *testing.T) {
	sections := []section{{heading: "Chapter 1"}}
	d := buildSheetData(sections)

	assert.Empty(t, d.colWidthReqs)
}

// TestBuildSheetData_colWidthReqs_greekWords confirms that the rune count is measured correctly
// for actual Greek Unicode text.
//
// "λόγος" = 5 runes → max(5×9+16, 50) = 61 px
// "ἦν"    = 2 runes → max(2×9+16, 50) = 50 px
func TestBuildSheetData_colWidthReqs_greekWords(t *testing.T) {
	sections := []section{{verses: []verse{{num: "1", words: []string{"λόγος", "ἦν"}}}}}
	d := buildSheetData(sections)

	require.Len(t, d.colWidthReqs, 2)
	assert.Equal(t, int64(61), d.colWidthReqs[0].UpdateDimensionProperties.Properties.PixelSize)
	assert.Equal(t, int64(50), d.colWidthReqs[1].UpdateDimensionProperties.Properties.PixelSize)
}

// TestBuildSheetData_colWidthReqs_raggedVerses confirms that when verses have unequal word
// counts, all columns seen across any verse get a width request, and each uses the max width
// across only the verses that contain that column.
//
// verse 1: col 1="alpha"(5), col 2="bee"(3), col 3="ca"(2)
// verse 2: col 1="do"(2),    col 2="elephant"(8)
// col 1: max(5,2)=5 → 61 px;  col 2: max(3,8)=8 → 88 px;  col 3: max(2)=2 → 50 px
func TestBuildSheetData_colWidthReqs_raggedVerses(t *testing.T) {
	sections := []section{{verses: []verse{
		{num: "1", words: []string{"alpha", "bee", "ca"}},
		{num: "2", words: []string{"do", "elephant"}},
	}}}
	d := buildSheetData(sections)

	require.Len(t, d.colWidthReqs, 3)
	assert.Equal(t, int64(61), d.colWidthReqs[0].UpdateDimensionProperties.Properties.PixelSize) // col 1
	assert.Equal(t, int64(88), d.colWidthReqs[1].UpdateDimensionProperties.Properties.PixelSize) // col 2
	assert.Equal(t, int64(50), d.colWidthReqs[2].UpdateDimensionProperties.Properties.PixelSize) // col 3
}
