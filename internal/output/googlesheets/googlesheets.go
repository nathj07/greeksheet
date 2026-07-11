/*
Package googlesheets renders a document.Document into a Google Sheets
spreadsheet formatted for Greek NT translation practice.

Each Tab in the Document becomes one spreadsheet tab. When no existing
spreadsheet is targeted, the first tab creates a new spreadsheet and subsequent
tabs are appended to it; when an existing SheetID is provided, every tab is
appended to that spreadsheet.
*/
package googlesheets

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/nathj07/greeksheet/internal/document"
)

// Options configure how a Target writes to Google Sheets.
type Options struct {
	// SheetID, when non-empty, is an existing spreadsheet to append tabs to.
	// When empty, a new spreadsheet is created.
	SheetID string
	// FolderID, when non-empty, is the Drive folder the new spreadsheet is
	// created in. It only applies when creating a new spreadsheet.
	FolderID string
	// Verbose logs Sheets API retry attempts to stderr.
	Verbose bool
}

// Target writes documents to Google Sheets using an authenticated HTTP client.
type Target struct {
	client *http.Client
	opts   Options
}

// New returns a Target that writes with the given authenticated client. The
// client must be authorised for the Sheets and Drive scopes (see internal/auth).
func New(client *http.Client, opts Options) *Target {
	return &Target{client: client, opts: opts}
}

// Render writes every tab of d to Google Sheets and returns the spreadsheet URL.
//
// With no configured SheetID the first tab creates a new spreadsheet (titled
// d.Title) and later tabs are appended to it. With a SheetID set, every tab is
// appended to that existing spreadsheet.
func (t *Target) Render(ctx context.Context, d document.Document) (string, error) {
	spreadsheetID := t.opts.SheetID
	var sheetURL string

	for _, tab := range d.Tabs {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}

		data := buildSheetData(tab.Sections)

		var err error
		if spreadsheetID == "" {
			// First tab and no existing target — create the spreadsheet;
			// subsequent tabs append to it.
			sheetURL, err = t.createSpreadsheet(ctx, d.Title, tab.Name, data)
			if err != nil {
				return "", err
			}
			spreadsheetID, err = extractSpreadsheetID(sheetURL)
			if err != nil {
				return "", err
			}
		} else {
			sheetURL, err = t.addTabToSpreadsheet(ctx, spreadsheetID, tab.Name, data)
			if err != nil {
				return "", err
			}
		}
	}

	return sheetURL, nil
}

// extractSpreadsheetID strips the Google Sheets URL prefix and returns just
// the spreadsheet ID. Used when we create a new spreadsheet and then need to
// add further tabs to it by ID.
func extractSpreadsheetID(sheetURL string) (string, error) {
	const prefix = "https://docs.google.com/spreadsheets/d/"
	if !strings.HasPrefix(sheetURL, prefix) {
		return "", fmt.Errorf("unexpected spreadsheet URL format: %q", sheetURL)
	}
	return strings.TrimPrefix(sheetURL, prefix), nil
}
