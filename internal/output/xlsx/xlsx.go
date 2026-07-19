/*
Package xlsx renders a document.Document into an Excel (.xlsx) spreadsheet
formatted for Greek NT translation practice.

Each Tab in the Document becomes one worksheet. The target file path is
provided via Options.File; the file is created if it does not already exist,
and tabs are appended to it if it does.

No Google authentication is required — the output is a plain local file.
*/
package xlsx

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/nathj07/greeksheet/internal/document"
	"github.com/xuri/excelize/v2"
)

// Options configure where the xlsx Target writes its output.
type Options struct {
	// File is the path to the .xlsx file to write. It is created if it does
	// not exist; tabs are appended to it if it does.
	File string
}

// Target writes documents to a local .xlsx file.
type Target struct {
	opts Options
}

// New returns a Target configured with the given options.
func New(opts Options) *Target {
	return &Target{opts: opts}
}

// Render writes every tab of d into the configured xlsx file and returns its
// path. The file is created if it does not yet exist; subsequent calls append
// new tabs to the same file.
//
// Each tab uses the standard translation-practice layout: grey/blue/orange/
// green row backgrounds, merged O/I/T/C/N cells, bold headings, 12 pt font,
// top-aligned wrapped text, and column widths sized to the widest Greek word.
func (t *Target) Render(_ context.Context, d document.Document) (string, error) {
	if t.opts.File == "" {
		return "", fmt.Errorf("xlsx target: File path must be set")
	}
	path := t.opts.File

	f, isNew, err := openOrCreate(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	sc := &styleCache{}
	if err := sc.build(f); err != nil {
		return "", fmt.Errorf("build styles: %w", err)
	}

	for i, tab := range d.Tabs {
		// Excel sheet names may not contain \ / ? * [ ] : and must be ≤ 31 chars.
		sheetName := sanitiseSheetName(tab.Name)
		sheet, err := ensureSheet(f, sheetName, i == 0 && isNew)
		if err != nil {
			return "", fmt.Errorf("ensure sheet %q: %w", sheetName, err)
		}
		if err := renderTab(f, sheet, tab.Sections, sc); err != nil {
			return "", fmt.Errorf("render tab %q: %w", sheetName, err)
		}
	}

	if err := f.SaveAs(path); err != nil {
		return "", fmt.Errorf("save %s: %w", path, err)
	}

	return path, nil
}

// openOrCreate opens an existing xlsx file or returns a new empty workbook.
// isNew is true when no existing file was found at path.
func openOrCreate(path string) (f *excelize.File, isNew bool, err error) {
	f, err = excelize.OpenFile(path)
	if err == nil {
		return f, false, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return excelize.NewFile(), true, nil
	}
	return nil, false, fmt.Errorf("open %s: %w", path, err)
}

// ensureSheet returns the name of a sheet to write into, creating it when
// needed. On a brand-new workbook the default "Sheet1" is renamed to sheetName
// to avoid leaving an empty spare sheet; on existing workbooks (or after the
// first tab) a fresh sheet is added.
func ensureSheet(f *excelize.File, sheetName string, isFirstSheetOfNewFile bool) (string, error) {
	if isFirstSheetOfNewFile {
		// Rename "Sheet1" rather than adding a new sheet to avoid an empty spare.
		existing := f.GetSheetName(0)
		if err := f.SetSheetName(existing, sheetName); err != nil {
			return "", fmt.Errorf("rename default sheet to %q: %w", sheetName, err)
		}
		return sheetName, nil
	}

	idx, err := f.NewSheet(sheetName)
	if err != nil {
		return "", fmt.Errorf("add sheet %q: %w", sheetName, err)
	}
	// Make the new sheet active so it is the one displayed on open.
	f.SetActiveSheet(idx)
	return sheetName, nil
}

// sanitiseSheetName makes s safe for use as an Excel worksheet name.
// Excel forbids \ / ? * [ ] : in sheet names and limits them to 31 characters.
// Colons are replaced with a period, which keeps verse-range names readable
// (e.g. "1:1-1:14" becomes "1.1-1.14").
func sanitiseSheetName(s string) string {
	replacer := strings.NewReplacer(
		":", ".",
		"\\", "_",
		"/", "_",
		"?", "_",
		"*", "_",
		"[", "_",
		"]", "_",
	)
	name := strings.TrimSpace(replacer.Replace(s))
	if name == "" {
		name = "Sheet"
	}
	// Excel sheet names are limited to 31 characters.
	if len([]rune(name)) > 31 {
		runes := []rune(name)
		name = string(runes[:31])
	}
	return name
}
