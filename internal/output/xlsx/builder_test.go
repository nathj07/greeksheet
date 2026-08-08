package xlsx

import (
	"testing"

	"github.com/nathj07/greeksheet/internal/document"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	excelize "github.com/xuri/excelize/v2"
)

// ---------------------------------------------------------------------------
// trackWordWidth
// ---------------------------------------------------------------------------

func TestTrackWordWidthGrowsSlice(t *testing.T) {
	var widths []int
	trackWordWidth(&widths, 2, "hi") // col 2 (1-based) = index 1, 2 runes

	require.Len(t, widths, 2) // slice must reach index 1
	assert.Equal(t, 2, widths[1])
}

func TestTrackWordWidthKeepsMax(t *testing.T) {
	var widths []int
	trackWordWidth(&widths, 2, "ab")    // col B: 2 runes
	trackWordWidth(&widths, 2, "abcde") // col B: 5 runes — wider
	trackWordWidth(&widths, 2, "xyz")   // col B: 3 runes — not wider

	assert.Equal(t, 5, widths[1]) // stored at index col-1 = 1
}

func TestTrackWordWidthGreekRunes(t *testing.T) {
	// "λόγος" is 5 Unicode code points, not 5 bytes.
	var widths []int
	trackWordWidth(&widths, 2, "λόγος") // col B → index 1

	assert.Equal(t, 5, widths[1])
}

// ---------------------------------------------------------------------------
// styleCache
// ---------------------------------------------------------------------------

func TestStyleCacheBuildsWithoutError(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	sc := &styleCache{}
	err := sc.build(f)
	require.NoError(t, err)

	// Style IDs are non-negative integers; 0 is a valid style ID in excelize.
	// Just check they were assigned — each NewStyle call returns an incrementing ID.
	assert.GreaterOrEqual(t, sc.plainID, 0)
	assert.GreaterOrEqual(t, sc.greyBgID, 0)
	assert.GreaterOrEqual(t, sc.blueBgID, 0)
	assert.GreaterOrEqual(t, sc.orangeBgID, 0)
	assert.GreaterOrEqual(t, sc.greenBgID, 0)
	assert.GreaterOrEqual(t, sc.boldID, 0)
}

// ---------------------------------------------------------------------------
// applyRowStyleRange
// ---------------------------------------------------------------------------

func TestApplyRowStyleRangeNoOpWhenFirstColExceedsLast(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	sc := &styleCache{}
	require.NoError(t, sc.build(f))

	// firstCol > lastCol: should be a no-op, not an error.
	err := applyRowStyleRange(f, "Sheet1", 5, 3, 1, sc.greyBgID)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// setCellWithStyle
// ---------------------------------------------------------------------------

func TestSetCellWithStyleSetsValueAndStyle(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	sc := &styleCache{}
	require.NoError(t, sc.build(f))

	require.NoError(t, setCellWithStyle(f, "Sheet1", 1, 1, "hello", sc.boldID))

	val, err := f.GetCellValue("Sheet1", "A1")
	require.NoError(t, err)
	assert.Equal(t, "hello", val)

	styleID, err := f.GetCellStyle("Sheet1", "A1")
	require.NoError(t, err)
	assert.Equal(t, sc.boldID, styleID)
}

// ---------------------------------------------------------------------------
// mergeWordCols
// ---------------------------------------------------------------------------

func TestMergeWordColsRegistersMerge(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	require.NoError(t, mergeWordCols(f, "Sheet1", 2, 4, 3))

	// Retrieve the merge list and confirm B3:D3 is present.
	merges, err := f.GetMergeCells("Sheet1")
	require.NoError(t, err)
	require.NotEmpty(t, merges, "at least one merge should be registered")

	found := false
	for _, m := range merges {
		if m.GetStartAxis() == "B3" && m.GetEndAxis() == "D3" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected merge B3:D3 to be present, got %v", merges)
}

// ---------------------------------------------------------------------------
// renderTab — column A width
// ---------------------------------------------------------------------------

func TestRenderTabColAWidth(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	sc := &styleCache{}
	require.NoError(t, sc.build(f))

	sections := []document.Section{{Verses: []document.Verse{{Num: "1", Words: []string{"word"}}}}}
	require.NoError(t, renderTab(f, "Sheet1", sections, sc))

	cols, err := f.GetColWidth("Sheet1", "A")
	require.NoError(t, err)
	// 40 px ÷ 7 ≈ 5.71; excelize may round to a nearby value, so check approximately.
	assert.InDelta(t, 5.71, cols, 0.1)
}

// ---------------------------------------------------------------------------
// renderTab — word column widths
// ---------------------------------------------------------------------------

// TestRenderTab_wordColWidths verifies that word columns are sized by the
// widest word seen in each column position.
//
// "word"  = 4 runes → max(4×9+16, 50) = 52 px → 52/7 ≈ 7.43 units
// "λόγος" = 5 runes → max(5×9+16, 50) = 61 px → 61/7 ≈ 8.71 units
func TestRenderTabWordColWidths(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	sc := &styleCache{}
	require.NoError(t, sc.build(f))

	sections := []document.Section{{Verses: []document.Verse{
		{Num: "1", Words: []string{"word", "λόγος"}},
	}}}
	require.NoError(t, renderTab(f, "Sheet1", sections, sc))

	colBWidth, err := f.GetColWidth("Sheet1", "B")
	require.NoError(t, err)
	assert.InDelta(t, 52.0/7.0, colBWidth, 0.15, "col B width for 4-rune word")

	colCWidth, err := f.GetColWidth("Sheet1", "C")
	require.NoError(t, err)
	assert.InDelta(t, 61.0/7.0, colCWidth, 0.15, "col C width for 5-rune word")
}

// TestRenderTab_wordColWidths_maxAcrossVerses confirms the widest word across
// all verses wins for a given column position.
//
// verse 1: col B="hi"(2), col C="elephant"(8)
// verse 2: col B="longer"(6)
// col B: max(2,6)=6 → max(6×9+16,50)=70 px;  col C: 8 → max(8×9+16,50)=88 px
func TestRenderTabWordColWidthsMaxAcrossVerses(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	sc := &styleCache{}
	require.NoError(t, sc.build(f))

	sections := []document.Section{{Verses: []document.Verse{
		{Num: "1", Words: []string{"hi", "elephant"}},
		{Num: "2", Words: []string{"longer"}},
	}}}
	require.NoError(t, renderTab(f, "Sheet1", sections, sc))

	colBWidth, err := f.GetColWidth("Sheet1", "B")
	require.NoError(t, err)
	assert.InDelta(t, 70.0/7.0, colBWidth, 0.1, "col B width should use widest word across verses")

	colCWidth, err := f.GetColWidth("Sheet1", "C")
	require.NoError(t, err)
	assert.InDelta(t, 88.0/7.0, colCWidth, 0.1, "col C width")
}

// TestRenderTab_wordColWidths_minimumWidth confirms the 50 px minimum applies
// for very short words.
//
// "a" = 1 rune → 1×9+16 = 25 px → clamped to 50 px → 50/7 ≈ 7.14 units
func TestRenderTabWordColWidthsMinimumWidth(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	sc := &styleCache{}
	require.NoError(t, sc.build(f))

	sections := []document.Section{{Verses: []document.Verse{{Num: "1", Words: []string{"a"}}}}}
	require.NoError(t, renderTab(f, "Sheet1", sections, sc))

	colBWidth, err := f.GetColWidth("Sheet1", "B")
	require.NoError(t, err)
	assert.InDelta(t, 50.0/7.0, colBWidth, 0.1, "minimum 50 px should apply for short words")
}

// ---------------------------------------------------------------------------
// renderTab — heading bold style
// ---------------------------------------------------------------------------

func TestRenderTabHeadingUsesBoldStyle(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	sc := &styleCache{}
	require.NoError(t, sc.build(f))

	sections := []document.Section{{Heading: "Romans 8"}}
	require.NoError(t, renderTab(f, "Sheet1", sections, sc))

	styleID, err := f.GetCellStyle("Sheet1", "A1")
	require.NoError(t, err)
	assert.Equal(t, sc.boldID, styleID)
}

// ---------------------------------------------------------------------------
// renderTab — verse row colours
// ---------------------------------------------------------------------------

// TestRenderTab_verseRowUsesGreyStyle checks that the verse row (row 1 for the
// first verse) has the grey background style on all word cells.
func TestRenderTabVerseRowUsesGreyStyle(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	sc := &styleCache{}
	require.NoError(t, sc.build(f))

	sections := []document.Section{{Verses: []document.Verse{{Num: "1", Words: []string{"alpha", "beta"}}}}}
	require.NoError(t, renderTab(f, "Sheet1", sections, sc))

	for _, cell := range []string{"A1", "B1", "C1"} {
		styleID, err := f.GetCellStyle("Sheet1", cell)
		require.NoError(t, err)
		assert.Equal(t, sc.greyBgID, styleID, "cell %s should have grey style", cell)
	}
}

// TestRenderTab_ORowUsesBlueStyle confirms the O row (row 4) has the blue background.
func TestRenderTabORowUsesBlueStyle(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	sc := &styleCache{}
	require.NoError(t, sc.build(f))

	sections := []document.Section{{Verses: []document.Verse{{Num: "1", Words: []string{"word", "two"}}}}}
	require.NoError(t, renderTab(f, "Sheet1", sections, sc))

	// O row is row 4: verse(1) + parse(2) + parse(3) = 3 rows above it.
	styleID, err := f.GetCellStyle("Sheet1", "A4")
	require.NoError(t, err)
	assert.Equal(t, sc.blueBgID, styleID, "O row col A should have blue style")

	styleID, err = f.GetCellStyle("Sheet1", "B4")
	require.NoError(t, err)
	assert.Equal(t, sc.blueBgID, styleID, "O row col B should have blue style")
}

// TestRenderTab_IRowUsesOrangeStyle confirms the I row uses the orange background.
func TestRenderTabIRowUsesOrangeStyle(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	sc := &styleCache{}
	require.NoError(t, sc.build(f))

	sections := []document.Section{{Verses: []document.Verse{{Num: "1", Words: []string{"word"}}}}}
	require.NoError(t, renderTab(f, "Sheet1", sections, sc))

	styleID, err := f.GetCellStyle("Sheet1", "A5")
	require.NoError(t, err)
	assert.Equal(t, sc.orangeBgID, styleID, "I row should have orange style")
}

// TestRenderTab_TRowUsesGreenStyle confirms the T row uses the green background.
func TestRenderTabTRowUsesGreenStyle(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	sc := &styleCache{}
	require.NoError(t, sc.build(f))

	sections := []document.Section{{Verses: []document.Verse{{Num: "1", Words: []string{"word"}}}}}
	require.NoError(t, renderTab(f, "Sheet1", sections, sc))

	styleID, err := f.GetCellStyle("Sheet1", "A6")
	require.NoError(t, err)
	assert.Equal(t, sc.greenBgID, styleID, "T row should have green style")
}

// TestRenderTab_CNRowsUsePlainStyle confirms C and N rows have no background colour.
func TestRenderTabCNRowsUsePlainStyle(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	sc := &styleCache{}
	require.NoError(t, sc.build(f))

	sections := []document.Section{{Verses: []document.Verse{{Num: "1", Words: []string{"word"}}}}}
	require.NoError(t, renderTab(f, "Sheet1", sections, sc))

	for _, cell := range []string{"A7", "A8"} {
		styleID, err := f.GetCellStyle("Sheet1", cell)
		require.NoError(t, err)
		assert.Equal(t, sc.plainID, styleID, "cell %s (C/N row) should have plain style", cell)
	}
}

// ---------------------------------------------------------------------------
// renderTab — merge behaviour
// ---------------------------------------------------------------------------

// TestRenderTab_multiWordVerse_mergesOITCNRows verifies that all five label
// rows have their word columns merged when a verse has more than one word.
func TestRenderTabMultiWordVerseMergesOITCNRows(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	sc := &styleCache{}
	require.NoError(t, sc.build(f))

	sections := []document.Section{{Verses: []document.Verse{{Num: "1", Words: []string{"alpha", "beta"}}}}}
	require.NoError(t, renderTab(f, "Sheet1", sections, sc))

	merges, err := f.GetMergeCells("Sheet1")
	require.NoError(t, err)

	// Expect exactly 5 merges: one each for O(row4), I(row5), T(row6), C(row7), N(row8).
	assert.Len(t, merges, 5, "expected one merge per label row for a two-word verse")

	// Build a set of start+end axes for quick lookup.
	mergeSet := make(map[string]bool, len(merges))
	for _, m := range merges {
		mergeSet[m.GetStartAxis()+":"+m.GetEndAxis()] = true
	}
	for _, want := range []string{"B4:C4", "B5:C5", "B6:C6", "B7:C7", "B8:C8"} {
		assert.True(t, mergeSet[want], "expected merge %s", want)
	}
}

// TestRenderTab_singleWordVerse_noMerges confirms no merges when a verse has
// only one word — there is nothing to merge.
func TestRenderTabSingleWordVerseNoMerges(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	sc := &styleCache{}
	require.NoError(t, sc.build(f))

	sections := []document.Section{{Verses: []document.Verse{{Num: "1", Words: []string{"λόγος"}}}}}
	require.NoError(t, renderTab(f, "Sheet1", sections, sc))

	merges, err := f.GetMergeCells("Sheet1")
	require.NoError(t, err)
	assert.Empty(t, merges, "single-word verse should produce no merges")
}

// ---------------------------------------------------------------------------
// renderTab — heading-only tab produces no merges or word column widths
// ---------------------------------------------------------------------------

func TestRenderTabHeadingOnlyNoMergesNoWordCols(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	sc := &styleCache{}
	require.NoError(t, sc.build(f))

	sections := []document.Section{{Heading: "Chapter 1"}}
	require.NoError(t, renderTab(f, "Sheet1", sections, sc))

	merges, err := f.GetMergeCells("Sheet1")
	require.NoError(t, err)
	assert.Empty(t, merges)

	// Column B should still have the default width (no word columns were placed).
	colBWidth, err := f.GetColWidth("Sheet1", "B")
	require.NoError(t, err)
	// excelize returns the default width (around 9–10 units) when no width has been set.
	assert.Greater(t, colBWidth, 7.0, "col B default width should be wider than the narrow col A")
}

// ---------------------------------------------------------------------------
// renderTab — cell values
// ---------------------------------------------------------------------------

func TestRenderTabHeadingRowValue(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	sc := &styleCache{}
	require.NoError(t, sc.build(f))

	sections := []document.Section{{Heading: "1 Corinthians 13"}}
	require.NoError(t, renderTab(f, "Sheet1", sections, sc))

	val, err := f.GetCellValue("Sheet1", "A1")
	require.NoError(t, err)
	assert.Equal(t, "1 Corinthians 13", val)
}

// TestRenderTab_verseBlock_rowCount confirms the verse block is exactly 8 rows
// by checking that the cell immediately after is empty.
func TestRenderTabVerseBlockRowCount(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	sc := &styleCache{}
	require.NoError(t, sc.build(f))

	sections := []document.Section{{Verses: []document.Verse{{Num: "1", Words: []string{"word", "two"}}}}}
	require.NoError(t, renderTab(f, "Sheet1", sections, sc))

	// verse row + 2 parsing rows + O + I + T + C + N = 8 rows; row 9 should be empty.
	val, err := f.GetCellValue("Sheet1", "A9")
	require.NoError(t, err)
	assert.Empty(t, val, "row 9 should be empty — verse block is exactly 8 rows")
}

func TestRenderTabVerseBlockVerseRow(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	sc := &styleCache{}
	require.NoError(t, sc.build(f))

	sections := []document.Section{{Verses: []document.Verse{{Num: "3", Words: []string{"λόγος", "ἦν"}}}}}
	require.NoError(t, renderTab(f, "Sheet1", sections, sc))

	numCell, err := f.GetCellValue("Sheet1", "A1")
	require.NoError(t, err)
	assert.Equal(t, "3", numCell)

	word1, err := f.GetCellValue("Sheet1", "B1")
	require.NoError(t, err)
	assert.Equal(t, "λόγος", word1)

	word2, err := f.GetCellValue("Sheet1", "C1")
	require.NoError(t, err)
	assert.Equal(t, "ἦν", word2)
}

// TestRenderTab_verseBlock_ORow confirms the O row holds the label in col A
// and the words joined with spaces starting at col B.
func TestRenderTabVerseBlockORow(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	sc := &styleCache{}
	require.NoError(t, sc.build(f))

	sections := []document.Section{{Verses: []document.Verse{{Num: "1", Words: []string{"word", "two"}}}}}
	require.NoError(t, renderTab(f, "Sheet1", sections, sc))

	// O row is row 4 (1-based): verse(1) + parse(2) + parse(3) above it.
	label, err := f.GetCellValue("Sheet1", "A4")
	require.NoError(t, err)
	assert.Equal(t, "O", label)

	joined, err := f.GetCellValue("Sheet1", "B4")
	require.NoError(t, err)
	assert.Equal(t, "word two", joined)
}

func TestRenderTabVerseBlockRowLabels(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	sc := &styleCache{}
	require.NoError(t, sc.build(f))

	sections := []document.Section{{Verses: []document.Verse{{Num: "1", Words: []string{"x"}}}}}
	require.NoError(t, renderTab(f, "Sheet1", sections, sc))

	// Row 4=O, 5=I, 6=T, 7=C, 8=N
	for _, tt := range []struct{ cell, label string }{
		{"A4", "O"}, {"A5", "I"}, {"A6", "T"}, {"A7", "C"}, {"A8", "N"},
	} {
		val, err := f.GetCellValue("Sheet1", tt.cell)
		require.NoError(t, err)
		assert.Equal(t, tt.label, val, "cell %s", tt.cell)
	}
}

// TestRenderTab_singleWordVerse confirms the O row contains the word when the
// verse has only one word (no merge is performed in this case).
func TestRenderTabSingleWordVerseORowValue(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	sc := &styleCache{}
	require.NoError(t, sc.build(f))

	sections := []document.Section{{Verses: []document.Verse{{Num: "1", Words: []string{"λόγος"}}}}}
	require.NoError(t, renderTab(f, "Sheet1", sections, sc))

	val, err := f.GetCellValue("Sheet1", "B4")
	require.NoError(t, err)
	assert.Equal(t, "λόγος", val)
}

// TestRenderTab_multipleVerses confirms each verse block starts at the correct
// row — the second verse must begin immediately after the first 8-row block.
func TestRenderTabMultipleVersesRowPositions(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	sc := &styleCache{}
	require.NoError(t, sc.build(f))

	sections := []document.Section{{Verses: []document.Verse{
		{Num: "1", Words: []string{"alpha"}},
		{Num: "2", Words: []string{"beta"}},
	}}}
	require.NoError(t, renderTab(f, "Sheet1", sections, sc))

	// Verse 1 at row 1; verse 2 at row 9 (8 rows per block, 1-based).
	v1, err := f.GetCellValue("Sheet1", "A1")
	require.NoError(t, err)
	assert.Equal(t, "1", v1)

	v2, err := f.GetCellValue("Sheet1", "A9")
	require.NoError(t, err)
	assert.Equal(t, "2", v2)
}
