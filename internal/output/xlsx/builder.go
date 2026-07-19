package xlsx

import (
	"fmt"
	"strings"

	"github.com/nathj07/greeksheet/internal/document"
	excelize "github.com/xuri/excelize/v2"
)

// ---------------------------------------------------------------------------
// Colours (ARGB hex strings for excelize)
// ---------------------------------------------------------------------------

// Standard row colours for the translation-practice layout.
const (
	colGrey   = "FFD9D9D9" // verse row
	colBlue   = "FFCFE2F3" // O row — original Greek text
	colOrange = "FFFCE5CD" // I row — interlinear practice
	colGreen  = "FFB7E1CD" // T row — translation practice
)

// ---------------------------------------------------------------------------
// Style cache — excelize requires a unique integer ID per distinct style
// ---------------------------------------------------------------------------

// styleCache holds pre-built excelize style IDs so we create each distinct
// style only once per file.
type styleCache struct {
	f *excelize.File

	// Pre-built IDs are populated by build().
	plainID    int
	greyBgID   int
	blueBgID   int
	orangeBgID int
	greenBgID  int
	boldID     int
}

// build creates all required styles in f and stores their IDs in the cache.
func (sc *styleCache) build(f *excelize.File) error {
	sc.f = f

	var err error

	sc.plainID, err = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 12},
		Alignment: &excelize.Alignment{Vertical: "top", WrapText: true},
	})
	if err != nil {
		return fmt.Errorf("create plain style: %w", err)
	}

	sc.greyBgID, err = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 12},
		Alignment: &excelize.Alignment{Vertical: "top", WrapText: true},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{colGrey}},
	})
	if err != nil {
		return fmt.Errorf("create grey style: %w", err)
	}

	sc.blueBgID, err = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 12},
		Alignment: &excelize.Alignment{Vertical: "top", WrapText: true},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{colBlue}},
	})
	if err != nil {
		return fmt.Errorf("create blue style: %w", err)
	}

	sc.orangeBgID, err = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 12},
		Alignment: &excelize.Alignment{Vertical: "top", WrapText: true},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{colOrange}},
	})
	if err != nil {
		return fmt.Errorf("create orange style: %w", err)
	}

	sc.greenBgID, err = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 12},
		Alignment: &excelize.Alignment{Vertical: "top", WrapText: true},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{colGreen}},
	})
	if err != nil {
		return fmt.Errorf("create green style: %w", err)
	}

	sc.boldID, err = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 12, Bold: true},
		Alignment: &excelize.Alignment{Vertical: "top", WrapText: true},
	})
	if err != nil {
		return fmt.Errorf("create bold style: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Sheet layout builder
// ---------------------------------------------------------------------------

// renderTab writes all verse and heading data from sections into the named
// sheet of f, applying the standard translation-practice layout:
//
//   - Column A: 40 px (label / verse number column)
//   - Word columns: sized to the widest Greek word they contain
//   - Per-verse block: 8 rows (verse, 2 parsing, O, I, T, C, N)
//   - Background colours: grey / blue / orange / green on the appropriate rows
//   - O/I/T/C/N rows: word columns merged into a single cell
//   - Heading rows: bold, no background
func renderTab(f *excelize.File, sheet string, sections []document.Section, sc *styleCache) error {
	// maxWordRuneLen[i] is the widest word (in runes) seen in 0-based column i.
	// Index 0 is col A (sized separately); only word columns from index 1 onward
	// are written into this slice.
	var maxWordRuneLen []int

	row := 1 // excelize uses 1-based row indices

	for _, sec := range sections {
		if sec.Heading != "" {
			cell, err := excelize.CoordinatesToCellName(1, row)
			if err != nil {
				return fmt.Errorf("heading cell name: %w", err)
			}
			if err := f.SetCellStr(sheet, cell, sec.Heading); err != nil {
				return fmt.Errorf("set heading value: %w", err)
			}
			if err := f.SetCellStyle(sheet, cell, cell, sc.boldID); err != nil {
				return fmt.Errorf("set heading style: %w", err)
			}
			row++
			continue
		}

		for _, v := range sec.Verses {
			if err := renderVerseBlock(f, sheet, v, &row, &maxWordRuneLen, sc); err != nil {
				return err
			}
		}
	}

	// Size column A to the narrow label width (40 px ≈ 5.71 character units in
	// Excel's default column width measure where 1 unit ≈ 7 pixels).
	if err := f.SetColWidth(sheet, "A", "A", 5.71); err != nil {
		return fmt.Errorf("set col A width: %w", err)
	}

	// Size each word column to the widest Greek word it holds.
	// The formula uses 9 px per rune plus 16 px of cell padding at 12 pt font,
	// with a 50 px minimum for very short words, converted to Excel character-
	// width units (÷ 7 px per unit).
	for col := 1; col < len(maxWordRuneLen); col++ {
		px := max(int(maxWordRuneLen[col])*9+16, 50)
		colName, err := excelize.ColumnNumberToName(col + 1) // col is 0-based; +1 for col A
		if err != nil {
			return fmt.Errorf("column name for index %d: %w", col+1, err)
		}
		if err := f.SetColWidth(sheet, colName, colName, float64(px)/7.0); err != nil {
			return fmt.Errorf("set col %s width: %w", colName, err)
		}
	}

	return nil
}

// renderVerseBlock writes the 8-row block for a single verse:
//
//  1. Verse row  — verse number in col A, one Greek word per following cell  (grey bg)
//  2. Parse row  — empty cells matching word columns                          (plain)
//  3. Parse row  — empty cells matching word columns                          (plain)
//  4. O row      — "O" label + original text merged across word cols          (blue bg)
//  5. I row      — "I" label + merged empty cell for interlinear practice     (orange bg)
//  6. T row      — "T" label + merged empty cell for final translation        (green bg)
//  7. C row      — "C" label + merged empty cell for commentary               (plain)
//  8. N row      — "N" label + merged empty cell for general notes            (plain)
func renderVerseBlock(
	f *excelize.File,
	sheet string,
	v document.Verse,
	row *int,
	maxWordRuneLen *[]int,
	sc *styleCache,
) error {
	wordCount := len(v.Words)
	firstWordCol := 2                  // column B (1-based)
	lastWordCol := 1 + wordCount       // 1-based; col A=1 so words span B..lastWordCol

	// ── Verse row ──────────────────────────────────────────────────────────
	// Col A: verse number; cols B onward: one Greek word each; grey background.
	verseRow := *row
	if err := setCellWithStyle(f, sheet, 1, verseRow, v.Num, sc.greyBgID); err != nil {
		return err
	}
	for i, w := range v.Words {
		col := firstWordCol + i
		trackWordWidth(maxWordRuneLen, col, w)
		if err := setCellWithStyle(f, sheet, col, verseRow, w, sc.greyBgID); err != nil {
			return err
		}
	}
	*row++

	// ── Two unlabelled parsing rows ─────────────────────────────────────────
	// Per-word parsing work; cells match the word columns above them.
	for range 2 {
		parseRow := *row
		if err := applyRowStyleRange(f, sheet, 1, lastWordCol, parseRow, sc.plainID); err != nil {
			return err
		}
		*row++
	}

	// ── O row ───────────────────────────────────────────────────────────────
	oRow := *row
	if err := setCellWithStyle(f, sheet, 1, oRow, "O", sc.blueBgID); err != nil {
		return err
	}
	if err := setCellWithStyle(f, sheet, firstWordCol, oRow, strings.Join(v.Words, " "), sc.blueBgID); err != nil {
		return err
	}
	if err := applyRowStyleRange(f, sheet, firstWordCol+1, lastWordCol, oRow, sc.blueBgID); err != nil {
		return err
	}
	if wordCount > 1 {
		if err := mergeWordCols(f, sheet, firstWordCol, lastWordCol, oRow); err != nil {
			return err
		}
	}
	*row++

	// ── I row ───────────────────────────────────────────────────────────────
	iRow := *row
	if err := setCellWithStyle(f, sheet, 1, iRow, "I", sc.orangeBgID); err != nil {
		return err
	}
	if err := applyRowStyleRange(f, sheet, firstWordCol, lastWordCol, iRow, sc.orangeBgID); err != nil {
		return err
	}
	if wordCount > 1 {
		if err := mergeWordCols(f, sheet, firstWordCol, lastWordCol, iRow); err != nil {
			return err
		}
	}
	*row++

	// ── T row ───────────────────────────────────────────────────────────────
	tRow := *row
	if err := setCellWithStyle(f, sheet, 1, tRow, "T", sc.greenBgID); err != nil {
		return err
	}
	if err := applyRowStyleRange(f, sheet, firstWordCol, lastWordCol, tRow, sc.greenBgID); err != nil {
		return err
	}
	if wordCount > 1 {
		if err := mergeWordCols(f, sheet, firstWordCol, lastWordCol, tRow); err != nil {
			return err
		}
	}
	*row++

	// ── C row ───────────────────────────────────────────────────────────────
	cRow := *row
	if err := setCellWithStyle(f, sheet, 1, cRow, "C", sc.plainID); err != nil {
		return err
	}
	if err := applyRowStyleRange(f, sheet, firstWordCol, lastWordCol, cRow, sc.plainID); err != nil {
		return err
	}
	if wordCount > 1 {
		if err := mergeWordCols(f, sheet, firstWordCol, lastWordCol, cRow); err != nil {
			return err
		}
	}
	*row++

	// ── N row ───────────────────────────────────────────────────────────────
	nRow := *row
	if err := setCellWithStyle(f, sheet, 1, nRow, "N", sc.plainID); err != nil {
		return err
	}
	if err := applyRowStyleRange(f, sheet, firstWordCol, lastWordCol, nRow, sc.plainID); err != nil {
		return err
	}
	if wordCount > 1 {
		if err := mergeWordCols(f, sheet, firstWordCol, lastWordCol, nRow); err != nil {
			return err
		}
	}
	*row++

	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// setCellWithStyle sets a string value and applies a pre-built style to one cell.
func setCellWithStyle(f *excelize.File, sheet string, col, row int, value string, styleID int) error {
	cell, err := excelize.CoordinatesToCellName(col, row)
	if err != nil {
		return fmt.Errorf("cell name (%d,%d): %w", col, row, err)
	}
	if err := f.SetCellStr(sheet, cell, value); err != nil {
		return fmt.Errorf("set cell value %s: %w", cell, err)
	}
	if err := f.SetCellStyle(sheet, cell, cell, styleID); err != nil {
		return fmt.Errorf("set cell style %s: %w", cell, err)
	}
	return nil
}

// applyRowStyleRange applies styleID to columns firstCol..lastCol on rowNum.
// If firstCol > lastCol the call is a no-op.
func applyRowStyleRange(f *excelize.File, sheet string, firstCol, lastCol, rowNum, styleID int) error {
	if firstCol > lastCol {
		return nil
	}
	top, err := excelize.CoordinatesToCellName(firstCol, rowNum)
	if err != nil {
		return fmt.Errorf("style range top (%d,%d): %w", firstCol, rowNum, err)
	}
	bot, err := excelize.CoordinatesToCellName(lastCol, rowNum)
	if err != nil {
		return fmt.Errorf("style range bot (%d,%d): %w", lastCol, rowNum, err)
	}
	if err := f.SetCellStyle(sheet, top, bot, styleID); err != nil {
		return fmt.Errorf("set style %s:%s: %w", top, bot, err)
	}
	return nil
}

// mergeWordCols merges columns firstWordCol..lastWordCol on rowNum into a
// single cell so the O/I/T/C/N label rows span the full word area.
func mergeWordCols(f *excelize.File, sheet string, firstWordCol, lastWordCol, rowNum int) error {
	top, err := excelize.CoordinatesToCellName(firstWordCol, rowNum)
	if err != nil {
		return fmt.Errorf("merge top (%d,%d): %w", firstWordCol, rowNum, err)
	}
	bot, err := excelize.CoordinatesToCellName(lastWordCol, rowNum)
	if err != nil {
		return fmt.Errorf("merge bot (%d,%d): %w", lastWordCol, rowNum, err)
	}
	if err := f.MergeCell(sheet, top, bot); err != nil {
		return fmt.Errorf("merge %s:%s: %w", top, bot, err)
	}
	return nil
}

// trackWordWidth updates maxWordRuneLen so each word column records the longest
// word (in runes) seen across all verses. col is 1-based (col A = 1, col B = 2);
// it is stored at index col-1. Index 0 (col A) is therefore allocated but never
// written — word columns start at col B — and the sizing loop skips it by
// iterating from index 1 and mapping i → ColumnNumberToName(i+1).
func trackWordWidth(maxWordRuneLen *[]int, col int, word string) {
	idx := col - 1
	for len(*maxWordRuneLen) <= idx {
		*maxWordRuneLen = append(*maxWordRuneLen, 0)
	}
	if n := len([]rune(word)); n > (*maxWordRuneLen)[idx] {
		(*maxWordRuneLen)[idx] = n
	}
}
