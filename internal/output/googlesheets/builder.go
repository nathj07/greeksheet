package googlesheets

import (
	"fmt"
	"strings"

	"github.com/nathj07/greeksheet/internal/document"
	"google.golang.org/api/sheets/v4"
)

// ---------------------------------------------------------------------------
// Colours (RGB floats for the Sheets API)
// ---------------------------------------------------------------------------

type rgbColor struct{ Red, Green, Blue float64 }

func hexToRGB(hex string) rgbColor {
	hex = strings.TrimPrefix(hex, "#")
	parse := func(i int) float64 {
		var v uint8
		fmt.Sscanf(hex[i:i+2], "%02x", &v)
		return float64(v) / 255
	}
	return rgbColor{Red: parse(0), Green: parse(2), Blue: parse(4)}
}

var (
	colGrey   = hexToRGB("d9d9d9") // verse row
	colBlue   = hexToRGB("cfe2f3") // O row — original Greek text
	colOrange = hexToRGB("fce5cd") // I row — interlinear practice
	colGreen  = hexToRGB("b7e1cd") // T row — translation practice
)

func toAPIColor(c rgbColor) *sheets.Color {
	return &sheets.Color{Red: c.Red, Green: c.Green, Blue: c.Blue}
}

// ---------------------------------------------------------------------------
// Sheet layout builder
// ---------------------------------------------------------------------------

type sheetData struct {
	rows          [][]any
	bgRequests    []*sheets.Request
	mergeReqs     []*sheets.Request
	boldRequests  []*sheets.Request
	alignVertReqs []*sheets.Request
	textWrapReqs  []*sheets.Request
	colWidthReqs  []*sheets.Request
}

// buildSheetData converts parsed sections into row data and Sheets API
// formatting requests (backgrounds, merges, bold headings).
func buildSheetData(sections []document.Section) sheetData {
	var d sheetData

	// maxWordRuneLen[i] is the widest word (in runes) seen in 0-based column i.
	// Index 0 corresponds to col A, which is sized separately by narrowColAReq;
	// only word columns starting at index 1 are ever written into this slice.
	var maxWordRuneLen []int

	addVerseBlock := func(v document.Verse) {
		wordCount := len(v.Words)
		firstWordCol := int64(1) // column B (0-indexed)
		lastWordCol := int64(wordCount)

		// Verse row — grey background, verse number in col A then words
		r := int64(len(d.rows))
		row := make([]any, 1+wordCount)
		row[0] = v.Num
		for i, w := range v.Words {
			row[1+i] = w
			col := 1 + i
			for len(maxWordRuneLen) <= col {
				maxWordRuneLen = append(maxWordRuneLen, 0)
			}
			if n := len([]rune(w)); n > maxWordRuneLen[col] {
				maxWordRuneLen[col] = n
			}
		}
		d.rows = append(d.rows, row)
		d.bgRequests = append(d.bgRequests, bgReq(r, 0, r+1, lastWordCol+1, colGrey))

		// Two unlabelled rows for parsing and building word choices — one cell per
		// Greek word so each word's work sits directly beneath it.
		d.rows = append(d.rows, make([]any, 1+wordCount), make([]any, 1+wordCount))

		// O row — single merged cell holding the original Greek text for reference
		r = int64(len(d.rows))
		d.rows = append(d.rows, []any{"O", strings.Join(v.Words, " ")})
		d.bgRequests = append(d.bgRequests, bgReq(r, 0, r+1, lastWordCol+1, colBlue))
		if wordCount > 1 {
			d.mergeReqs = append(d.mergeReqs, mergeReq(r, firstWordCol, r+1, lastWordCol+1))
		}

		// I row — single merged cell for interlinear practice
		r = int64(len(d.rows))
		iRow := make([]any, 1+wordCount)
		iRow[0] = "I"
		d.rows = append(d.rows, iRow)
		d.bgRequests = append(d.bgRequests, bgReq(r, 0, r+1, lastWordCol+1, colOrange))
		if wordCount > 1 {
			d.mergeReqs = append(d.mergeReqs, mergeReq(r, firstWordCol, r+1, lastWordCol+1))
		}

		// T row — single merged cell for full translation practice
		r = int64(len(d.rows))
		tRow := make([]any, 1+wordCount)
		tRow[0] = "T"
		d.rows = append(d.rows, tRow)
		d.bgRequests = append(d.bgRequests, bgReq(r, 0, r+1, lastWordCol+1, colGreen))
		if wordCount > 1 {
			d.mergeReqs = append(d.mergeReqs, mergeReq(r, firstWordCol, r+1, lastWordCol+1))
		}

		// C row — single merged cell for commentary notes
		r = int64(len(d.rows))
		d.rows = append(d.rows, []any{"C"})
		if wordCount > 1 {
			d.mergeReqs = append(d.mergeReqs, mergeReq(r, firstWordCol, r+1, lastWordCol+1))
		}

		// N row — single merged cell for general notes
		r = int64(len(d.rows))
		d.rows = append(d.rows, []any{"N"})
		if wordCount > 1 {
			d.mergeReqs = append(d.mergeReqs, mergeReq(r, firstWordCol, r+1, lastWordCol+1))
		}
	}

	for _, sec := range sections {
		if sec.Heading != "" {
			r := int64(len(d.rows))
			d.rows = append(d.rows, []any{sec.Heading})
			d.boldRequests = append(d.boldRequests, boldReq(r, 0, r+1, 1))
		} else {
			for _, v := range sec.Verses {
				addVerseBlock(v)
			}
		}
	}

	// Set vertical alignment to top for all cells to avoid awkward default centering in taller rows.
	d.alignVertReqs = append(d.alignVertReqs, &sheets.Request{
		RepeatCell: &sheets.RepeatCellRequest{
			Range:  &sheets.GridRange{SheetId: 0},
			Cell:   &sheets.CellData{UserEnteredFormat: &sheets.CellFormat{VerticalAlignment: "TOP"}},
			Fields: "userEnteredFormat.verticalAlignment",
		},
	})

	// Set text wrap for all cells to avoid overflow and ensure row heights adjust to fit content.
	d.textWrapReqs = append(d.textWrapReqs, &sheets.Request{
		RepeatCell: &sheets.RepeatCellRequest{
			Range:  &sheets.GridRange{SheetId: 0},
			Cell:   &sheets.CellData{UserEnteredFormat: &sheets.CellFormat{WrapStrategy: "WRAP"}},
			Fields: "userEnteredFormat.wrapStrategy",
		},
	})

	// Size each word column to fit the widest Greek word it contains without wrapping.
	// The estimate uses 9 px per rune plus 16 px of cell padding at 12 pt font,
	// with a 50 px minimum for very short words.
	for col := 1; col < len(maxWordRuneLen); col++ {
		px := max(int64(maxWordRuneLen[col])*9+16, 50)
		d.colWidthReqs = append(d.colWidthReqs, colWidthReq(0, int64(col), px))
	}

	return d
}

// ---------------------------------------------------------------------------
// Sheets API request helpers
// ---------------------------------------------------------------------------

func gridRange(sr, sc, er, ec int64) *sheets.GridRange {
	return &sheets.GridRange{
		SheetId:          0,
		StartRowIndex:    sr,
		EndRowIndex:      er,
		StartColumnIndex: sc,
		EndColumnIndex:   ec,
	}
}

func bgReq(sr, sc, er, ec int64, color rgbColor) *sheets.Request {
	return &sheets.Request{RepeatCell: &sheets.RepeatCellRequest{
		Range: gridRange(sr, sc, er, ec),
		Cell: &sheets.CellData{UserEnteredFormat: &sheets.CellFormat{
			BackgroundColorStyle: &sheets.ColorStyle{RgbColor: toAPIColor(color)},
		}},
		Fields: "userEnteredFormat.backgroundColorStyle",
	}}
}

func mergeReq(sr, sc, er, ec int64) *sheets.Request {
	return &sheets.Request{MergeCells: &sheets.MergeCellsRequest{
		Range:     gridRange(sr, sc, er, ec),
		MergeType: "MERGE_ALL",
	}}
}

func boldReq(sr, sc, er, ec int64) *sheets.Request {
	return &sheets.Request{RepeatCell: &sheets.RepeatCellRequest{
		Range: gridRange(sr, sc, er, ec),
		Cell: &sheets.CellData{UserEnteredFormat: &sheets.CellFormat{
			TextFormat: &sheets.TextFormat{Bold: true},
		}},
		Fields: "userEnteredFormat.textFormat.bold",
	}}
}

func alignVertReq(sr, sc, er, ec int64, align string) *sheets.Request {
	return &sheets.Request{RepeatCell: &sheets.RepeatCellRequest{
		Range: gridRange(sr, sc, er, ec),
		Cell: &sheets.CellData{UserEnteredFormat: &sheets.CellFormat{
			VerticalAlignment: align,
		}},
		Fields: "userEnteredFormat.verticalAlignment",
	}}
}

func textWrapReq(sr, sc, er, ec int64) *sheets.Request {
	return &sheets.Request{RepeatCell: &sheets.RepeatCellRequest{
		Range: gridRange(sr, sc, er, ec),
		Cell: &sheets.CellData{UserEnteredFormat: &sheets.CellFormat{
			WrapStrategy: "WRAP",
		}},
		Fields: "userEnteredFormat.wrapStrategy",
	}}
}

func narrowColAReq(sheetID int64) *sheets.Request {
	return colWidthReq(sheetID, 0, 40)
}

// colWidthReq sets a single column to the given pixel width.
func colWidthReq(sheetID, colIdx, pixelSize int64) *sheets.Request {
	return &sheets.Request{UpdateDimensionProperties: &sheets.UpdateDimensionPropertiesRequest{
		Range: &sheets.DimensionRange{
			SheetId:    sheetID,
			Dimension:  "COLUMNS",
			StartIndex: colIdx,
			EndIndex:   colIdx + 1,
		},
		Properties: &sheets.DimensionProperties{PixelSize: pixelSize},
		Fields:     "pixelSize",
	}}
}

// fontSizeReq sets the font size for all cells on the sheet in a single pass.
// Using a SheetId-only range (no row/column bounds) covers the entire sheet.
func fontSizeReq(sheetID int64, size int64) *sheets.Request {
	return &sheets.Request{RepeatCell: &sheets.RepeatCellRequest{
		Range: &sheets.GridRange{SheetId: sheetID},
		Cell: &sheets.CellData{UserEnteredFormat: &sheets.CellFormat{
			TextFormat: &sheets.TextFormat{FontSize: size},
		}},
		Fields: "userEnteredFormat.textFormat.fontSize",
	}}
}

func textNumberFormatReq(sheetID int64) *sheets.Request {
	return &sheets.Request{RepeatCell: &sheets.RepeatCellRequest{
		Range: &sheets.GridRange{
			SheetId:          sheetID,
			StartColumnIndex: 1,
		},
		Cell: &sheets.CellData{UserEnteredFormat: &sheets.CellFormat{
			NumberFormat: &sheets.NumberFormat{Type: "TEXT"},
		}},
		Fields: "userEnteredFormat.numberFormat",
	}}
}

// patchSheetID rewrites every GridRange.SheetId in d's formatting requests to
// the given id. This is needed after a tab is created and its real sheetId
// (assigned by Google) becomes known — buildSheetData uses 0 as a placeholder.
func patchSheetID(d *sheetData, id int64) {
	patchReqs := func(reqs []*sheets.Request) {
		for _, req := range reqs {
			if req.RepeatCell != nil && req.RepeatCell.Range != nil {
				req.RepeatCell.Range.SheetId = id
			}
			if req.MergeCells != nil && req.MergeCells.Range != nil {
				req.MergeCells.Range.SheetId = id
			}
			if req.UpdateDimensionProperties != nil && req.UpdateDimensionProperties.Range != nil {
				req.UpdateDimensionProperties.Range.SheetId = id
			}
		}
	}
	patchReqs(d.bgRequests)
	patchReqs(d.mergeReqs)
	patchReqs(d.boldRequests)
	patchReqs(d.alignVertReqs)
	patchReqs(d.textWrapReqs)
	patchReqs(d.colWidthReqs)
}
