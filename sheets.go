package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

// populateTab writes row data and applies all formatting to the sheet
// identified by sheetID inside the given spreadsheet.
func populateTab(ctx context.Context, sheetsSvc *sheets.Service, spreadsheetID string, sheetID int64, d sheetData) error {
	maxCols := 1
	for _, row := range d.rows {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}

	vr := make([]*sheets.RowData, len(d.rows))
	for i, row := range d.rows {
		cells := make([]*sheets.CellData, maxCols)
		for j := range maxCols {
			cd := &sheets.CellData{}
			if j < len(row) {
				if s, ok := row[j].(string); ok {
					cd.UserEnteredValue = &sheets.ExtendedValue{StringValue: &s}
				}
			}
			cells[j] = cd
		}
		vr[i] = &sheets.RowData{Values: cells}
	}

	// Expand column count first — the default 26-column limit would reject
	// writes beyond column Z for longer verses. Then write all cell values.
	updateReq := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{UpdateSheetProperties: &sheets.UpdateSheetPropertiesRequest{
				Properties: &sheets.SheetProperties{
					SheetId: sheetID,
					GridProperties: &sheets.GridProperties{
						ColumnCount: int64(maxCols),
					},
				},
				Fields: "gridProperties.columnCount",
			}},
			{UpdateCells: &sheets.UpdateCellsRequest{
				Start:  &sheets.GridCoordinate{SheetId: sheetID},
				Rows:   vr,
				Fields: "userEnteredValue",
			}},
		},
	}
	if _, err := sheetsSvc.Spreadsheets.BatchUpdate(spreadsheetID, updateReq).Context(ctx).Do(); err != nil {
		return fmt.Errorf("writing cell values: %w", err)
	}
	fmt.Printf("Written %d rows × %d cols\n", len(d.rows), maxCols)

	// Rewrite all placeholder SheetId=0 values to the real id, then apply
	// formatting in a single batch.
	patchSheetID(&d, sheetID)
	var allReqs []*sheets.Request
	allReqs = append(allReqs, d.mergeReqs...)
	allReqs = append(allReqs, d.bgRequests...)
	allReqs = append(allReqs, d.boldRequests...)
	allReqs = append(allReqs, narrowColAReq(sheetID))
	allReqs = append(allReqs, fontSizeReq(sheetID, 12))

	fmtReq := &sheets.BatchUpdateSpreadsheetRequest{Requests: allReqs}
	if _, err := sheetsSvc.Spreadsheets.BatchUpdate(spreadsheetID, fmtReq).Context(ctx).Do(); err != nil {
		return fmt.Errorf("applying formatting: %w", err)
	}
	fmt.Println("Formatting applied.")
	return nil
}

// createSpreadsheet creates a new Google Sheet with a single content tab named
// tabName, populates it with d, and makes it accessible via link. The spreadsheet
// title is used as the document name in Google Drive.
func createSpreadsheet(ctx context.Context, client *http.Client, title, tabName string, d sheetData) (string, error) {
	sheetsSvc, err := sheets.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return "", fmt.Errorf("creating Sheets service: %w", err)
	}
	driveSvc, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return "", fmt.Errorf("creating Drive service: %w", err)
	}

	// Create the spreadsheet with the content tab already named correctly —
	// this avoids needing to rename or delete a default "Sheet1" afterwards.
	ss, err := sheetsSvc.Spreadsheets.Create(&sheets.Spreadsheet{
		Properties: &sheets.SpreadsheetProperties{Title: title},
		Sheets: []*sheets.Sheet{{
			Properties: &sheets.SheetProperties{Title: tabName},
		}},
	}).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("creating spreadsheet: %w", err)
	}
	if len(ss.Sheets) == 0 {
		return "", fmt.Errorf("spreadsheet created but no sheets returned")
	}
	sheetID := ss.Sheets[0].Properties.SheetId
	fmt.Printf("Created: https://docs.google.com/spreadsheets/d/%s\n", ss.SpreadsheetId)

	if err := populateTab(ctx, sheetsSvc, ss.SpreadsheetId, sheetID, d); err != nil {
		return "", err
	}

	// Make the sheet accessible via link.
	if _, err = driveSvc.Permissions.Create(ss.SpreadsheetId, &drive.Permission{
		Type: "anyone",
		Role: "writer",
	}).Context(ctx).Do(); err != nil {
		// Non-fatal — sheet is still usable by the owner.
		fmt.Fprintf(os.Stderr, "Warning: could not set sharing permissions: %v\n", err)
	}

	return fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s", ss.SpreadsheetId), nil
}

// addTabToSpreadsheet adds a new tab named tabName to an existing spreadsheet
// and populates it with d.
func addTabToSpreadsheet(ctx context.Context, client *http.Client, spreadsheetID, tabName string, d sheetData) (string, error) {
	sheetsSvc, err := sheets.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return "", fmt.Errorf("creating Sheets service: %w", err)
	}

	resp, err := sheetsSvc.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{{
			AddSheet: &sheets.AddSheetRequest{
				Properties: &sheets.SheetProperties{Title: tabName},
			},
		}},
	}).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("adding tab: %w", err)
	}
	if len(resp.Replies) == 0 || resp.Replies[0].AddSheet == nil {
		return "", fmt.Errorf("unexpected empty reply when adding tab '%s'", tabName)
	}
	newSheetID := resp.Replies[0].AddSheet.Properties.SheetId
	fmt.Printf("Added tab '%s' to https://docs.google.com/spreadsheets/d/%s\n", tabName, spreadsheetID)

	if err := populateTab(ctx, sheetsSvc, spreadsheetID, newSheetID, d); err != nil {
		return "", err
	}

	return fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s", spreadsheetID), nil
}
