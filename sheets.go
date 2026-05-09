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
// identified by sheetID inside the given spreadsheet. When renameTab is true,
// the default "Sheet1" tab created by Drive.Files.Create is renamed to tabName
// as part of the first BatchUpdate — avoiding a separate API call.
func populateTab(ctx context.Context, sheetsSvc *sheets.Service, spreadsheetID, tabName string, renameTab bool, sheetID int64, d sheetData) error {
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
	dataReqs := []*sheets.Request{}
	if renameTab {
		dataReqs = append(dataReqs, &sheets.Request{
			UpdateSheetProperties: &sheets.UpdateSheetPropertiesRequest{
				Properties: &sheets.SheetProperties{
					SheetId: sheetID,
					Title:   tabName,
				},
				Fields: "title",
			},
		})
	}
	dataReqs = append(dataReqs,
		&sheets.Request{UpdateSheetProperties: &sheets.UpdateSheetPropertiesRequest{
			Properties: &sheets.SheetProperties{
				SheetId: sheetID,
				GridProperties: &sheets.GridProperties{
					ColumnCount: int64(maxCols),
				},
			},
			Fields: "gridProperties.columnCount",
		}},
		&sheets.Request{UpdateCells: &sheets.UpdateCellsRequest{
			Start:  &sheets.GridCoordinate{SheetId: sheetID},
			Rows:   vr,
			Fields: "userEnteredValue",
		}},
	)
	updateReq := &sheets.BatchUpdateSpreadsheetRequest{Requests: dataReqs}
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
//
// If folderID is non-empty, the spreadsheet is created directly inside that Drive
// folder using the Drive Files.Create API (which accepts a parents field). When
// folderID is empty the sheet lands in the authenticated user's Drive root.
func createSpreadsheet(ctx context.Context, client *http.Client, title, tabName, folderID string, d sheetData) (string, error) {
	sheetsSvc, err := sheets.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return "", fmt.Errorf("creating Sheets service: %w", err)
	}
	driveSvc, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return "", fmt.Errorf("creating Drive service: %w", err)
	}

	var spreadsheetID string
	var initialSheetID int64
	if folderID != "" {
		// Create directly in the target folder via the Drive API. This avoids the
		// sheet ever appearing in the Drive root, which would happen if we created
		// it via the Sheets API and then moved it.
		f := &drive.File{
			Name:     title,
			MimeType: "application/vnd.google-apps.spreadsheet",
			Parents:  []string{folderID},
		}
		created, err := driveSvc.Files.Create(f).Context(ctx).Do()
		if err != nil {
			return "", fmt.Errorf("creating spreadsheet in folder: %w", err)
		}
		if created.Id == "" {
			return "", fmt.Errorf("creating spreadsheet in folder: Drive returned empty file ID")
		}
		spreadsheetID = created.Id

		// Read back the sheet to get the real sheetId — Drive.Files.Create does
		// not return sheet metadata, and we need the ID for BatchUpdate calls.
		ss, err := sheetsSvc.Spreadsheets.Get(spreadsheetID).Context(ctx).Do()
		if err != nil {
			return "", fmt.Errorf("reading new spreadsheet: %w", err)
		}
		if len(ss.Sheets) == 0 {
			return "", fmt.Errorf("spreadsheet created but no sheets returned")
		}
		initialSheetID = ss.Sheets[0].Properties.SheetId
	} else {
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
		spreadsheetID = ss.SpreadsheetId
		initialSheetID = ss.Sheets[0].Properties.SheetId
	}
	fmt.Printf("Created: https://docs.google.com/spreadsheets/d/%s\n", spreadsheetID)

	if err := populateTab(ctx, sheetsSvc, spreadsheetID, tabName, folderID != "", initialSheetID, d); err != nil {
		return "", err
	}

	// Make the sheet accessible via link.
	if _, err = driveSvc.Permissions.Create(spreadsheetID, &drive.Permission{
		Type: "anyone",
		Role: "writer",
	}).Context(ctx).Do(); err != nil {
		// Non-fatal — sheet is still usable by the owner.
		fmt.Fprintf(os.Stderr, "Warning: could not set sharing permissions: %v\n", err)
	}

	return fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s", spreadsheetID), nil
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

	if err := populateTab(ctx, sheetsSvc, spreadsheetID, tabName, false, newSheetID, d); err != nil {
		return "", err
	}

	return fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s", spreadsheetID), nil
}
