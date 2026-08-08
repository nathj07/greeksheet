/*
greeksheetui is a desktop GUI front-end for the greeksheet tool. It lets users
generate Greek NT translation-practice spreadsheets without needing to use the
command line.

The two tabs mirror the two output modes of the CLI:

  - Excel (.xlsx): writes to a local file; no Google account needed.
  - Google Sheets: writes to a Google Sheet; a browser opens for OAuth on first
    run and the token is cached at ~/.config/greeksheet/token.json.

In both tabs the user provides a scripture reference (e.g. "John 1:1-14") or
picks a local plain-text verse file, then clicks Submit.
*/
package main

import (
	"context"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
)

func main() {
	w := makeUI(AppRunner{})
	w.ShowAndRun()
}

// makeUI builds the complete window. It accepts a Runner so tests can inject a
// stub without spawning a real Fyne application.
func makeUI(runner Runner) fyne.Window {
	uiApp := app.NewWithID("greeksheet")
	w := uiApp.NewWindow("Greek Sheet UI")
	w.Resize(fyne.NewSize(480, 400))

	u := newUI(w, runner)

	tabs := container.NewAppTabs(
		u.excelTab(),
		u.sheetsTab(),
	)
	tabs.SetTabLocation(container.TabLocationTop)
	w.SetContent(tabs)

	return w
}

// ui owns all mutable widget state for the application window. Using a struct
// receiver instead of package-level globals makes the state explicit and
// removes the need for global variables that would interfere across tests.
type ui struct {
	win    fyne.Window
	runner Runner

	// Excel tab widgets
	xlsxFileLabel      *widget.Label
	xlsxFolderLabel    *widget.Label
	xlsxFileURI        fyne.URI
	xlsxFolderURI      fyne.ListableURI
	xlsxInputFileLabel *widget.Label
	xlsxInputFileURI   fyne.URI
	xlsxRef            *widget.Entry
	xlsxTitle          *widget.Entry
	xlsxResult         *widget.Label

	// Sheets tab widgets
	sheetsInputFileLabel *widget.Label
	sheetsInputFileURI   fyne.URI
	sheetsRef            *widget.Entry
	sheetsSheetID        *widget.Entry
	sheetsFolderID       *widget.Entry
	sheetsTitle          *widget.Entry
	sheetsResult         *widget.Label
}

func newUI(w fyne.Window, runner Runner) *ui {
	u := &ui{
		win:    w,
		runner: runner,

		xlsxFileLabel:      widget.NewLabel("No file selected"),
		xlsxFolderLabel:    widget.NewLabel("No folder selected"),
		xlsxInputFileLabel: widget.NewLabel("No file selected"),
		xlsxRef:            widget.NewEntry(),
		xlsxTitle:          widget.NewEntry(),
		xlsxResult:         widget.NewLabel(""),

		sheetsInputFileLabel: widget.NewLabel("No file selected"),
		sheetsRef:            widget.NewEntry(),
		sheetsSheetID:        widget.NewEntry(),
		sheetsFolderID:       widget.NewEntry(),
		sheetsTitle:          widget.NewEntry(),
		sheetsResult:         widget.NewLabel(""),
	}

	// Typing a ref clears any picked input file so only one source is active.
	// Wired here rather than in the tab builders so the callbacks are
	// available without needing a live Fyne display (important for tests).
	u.xlsxRef.OnChanged = func(_ string) {
		u.xlsxInputFileURI = nil
		u.xlsxInputFileLabel.SetText("No file selected")
	}
	u.sheetsRef.OnChanged = func(_ string) {
		u.sheetsInputFileURI = nil
		u.sheetsInputFileLabel.SetText("No file selected")
	}

	return u
}

// ---------------------------------------------------------------------------
// Excel tab
// ---------------------------------------------------------------------------

func (u *ui) excelTab() *container.TabItem {
	u.xlsxRef.SetPlaceHolder(`e.g. "John 1:1-14" or "Ephesians 1-6"`)
	u.xlsxTitle.SetPlaceHolder("optional title override")

	inputFilePickerBtn := widget.NewButton("Pick input .txt file", func() {
		u.showXLSXInputFilePicker()
	})
	filePickerBtn := widget.NewButton("Pick existing .xlsx file", func() {
		u.showXLSXFilePicker()
	})
	folderPickerBtn := widget.NewButton("Pick folder for new file", func() {
		u.showXLSXFolderPicker()
	})

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Ref", Widget: u.xlsxRef, HintText: "Scripture reference to fetch"},
			{Text: "Input file", Widget: inputFilePickerBtn, HintText: "Local .txt verse file (alternative to ref)"},
			{Text: "Selected input", Widget: u.xlsxInputFileLabel},
			{Text: "Title", Widget: u.xlsxTitle},
			{Text: "Existing .xlsx", Widget: filePickerBtn, HintText: "Append to an existing file"},
			{Text: "Selected file", Widget: u.xlsxFileLabel},
			{Text: "New file folder", Widget: folderPickerBtn, HintText: "Create a new file in this folder"},
			{Text: "Selected folder", Widget: u.xlsxFolderLabel},
			{Text: "Result", Widget: u.xlsxResult},
		},
		OnSubmit: u.onXLSXSubmit,
	}

	return container.NewTabItem("Excel (.xlsx)", form)
}

func (u *ui) showXLSXInputFilePicker() {
	fd := dialog.NewFileOpen(func(uc fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, u.win)
			return
		}
		if uc == nil {
			return
		}
		u.xlsxInputFileURI = uc.URI()
		u.xlsxInputFileLabel.SetText(uc.URI().Path())
		// Clear the ref field so only one source is active.
		u.xlsxRef.SetText("")
	}, u.win)
	fd.SetFilter(storage.NewExtensionFileFilter([]string{".txt"}))
	fd.Show()
}

func (u *ui) showXLSXFilePicker() {
	fd := dialog.NewFileOpen(func(uc fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, u.win)
			return
		}
		if uc == nil {
			return
		}
		u.xlsxFileURI = uc.URI()
		u.xlsxFileLabel.SetText(uc.URI().Path())
		// Clear any folder selection so only one destination is set.
		u.xlsxFolderURI = nil
		u.xlsxFolderLabel.SetText("No folder selected")
	}, u.win)
	fd.SetFilter(storage.NewExtensionFileFilter([]string{".xlsx"}))
	fd.Show()
}

func (u *ui) showXLSXFolderPicker() {
	fd := dialog.NewFolderOpen(func(list fyne.ListableURI, err error) {
		if err != nil {
			dialog.ShowError(err, u.win)
			return
		}
		if list == nil {
			return
		}
		u.xlsxFolderURI = list
		u.xlsxFolderLabel.SetText(list.Path())
		// Clear any file selection so only one destination is set.
		u.xlsxFileURI = nil
		u.xlsxFileLabel.SetText("No file selected")
	}, u.win)
	fd.Show()
}

// xlsxFilePath returns the resolved output .xlsx file path from whichever
// picker was used, or an empty string if neither was set. When a folder was
// picked, title is used as the filename (falling back to "greeksheet" when
// title is empty).
func (u *ui) xlsxFilePath(title string) string {
	if u.xlsxFileURI != nil {
		return u.xlsxFileURI.Path()
	}
	if u.xlsxFolderURI != nil {
		name := sanitiseFilename(title)
		return filepath.Join(u.xlsxFolderURI.Path(), name+".xlsx")
	}
	return ""
}

// sanitiseFilename converts a title string into a safe filename component by
// replacing characters that are problematic on common filesystems. Returns
// "greeksheet" when the result would otherwise be empty.
func sanitiseFilename(title string) string {
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	name := strings.TrimSpace(replacer.Replace(title))
	if name == "" {
		return "greeksheet"
	}
	return name
}

// uriPath safely extracts the filesystem path from a fyne.URI, returning an
// empty string when the URI is nil (i.e. the user has not yet picked a file).
func uriPath(u fyne.URI) string {
	if u == nil {
		return ""
	}
	return u.Path()
}

func (u *ui) onXLSXSubmit() {
	opts := XLSXOptions{
		XLSXFile:  u.xlsxFilePath(u.xlsxTitle.Text),
		Ref:       u.xlsxRef.Text,
		InputFile: uriPath(u.xlsxInputFileURI),
		Title:     u.xlsxTitle.Text,
	}
	if err := validateXLSXInputs(opts); err != nil {
		dialog.ShowError(err, u.win)
		return
	}

	prog := dialog.NewInformation("Running…", "Generating your spreadsheet, please wait.", u.win)
	prog.Show()

	go func() {
		path, err := u.runner.RunXLSX(context.Background(), opts)
		fyne.Do(func() {
			prog.Hide()
			if err != nil {
				dialog.ShowError(err, u.win)
				return
			}
			u.xlsxResult.SetText(path)
			dialog.ShowInformation("Done!", "Spreadsheet written to:\n"+path, u.win)
		})
	}()
}

// ---------------------------------------------------------------------------
// Google Sheets tab
// ---------------------------------------------------------------------------

func (u *ui) sheetsTab() *container.TabItem {
	u.sheetsRef.SetPlaceHolder(`e.g. "John 1:1-14" or "Ephesians 1-6"`)
	u.sheetsSheetID.SetPlaceHolder("ID of existing spreadsheet (optional)")
	u.sheetsFolderID.SetPlaceHolder("Drive folder ID for new sheet (optional)")
	u.sheetsTitle.SetPlaceHolder("optional spreadsheet title")

	inputFilePickerBtn := widget.NewButton("Pick input .txt file", func() {
		u.showSheetsInputFilePicker()
	})

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Ref", Widget: u.sheetsRef, HintText: "Scripture reference to fetch"},
			{Text: "Input file", Widget: inputFilePickerBtn, HintText: "Local .txt verse file (alternative to ref)"},
			{Text: "Selected input", Widget: u.sheetsInputFileLabel},
			{Text: "Sheet ID", Widget: u.sheetsSheetID, HintText: "Append to an existing sheet"},
			{Text: "Folder ID", Widget: u.sheetsFolderID, HintText: "Create a new sheet in this folder"},
			{Text: "Title", Widget: u.sheetsTitle},
			{Text: "Result URL", Widget: u.sheetsResult},
		},
		OnSubmit: u.onSheetsSubmit,
	}

	return container.NewTabItem("Google Sheets", form)
}

func (u *ui) showSheetsInputFilePicker() {
	fd := dialog.NewFileOpen(func(uc fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, u.win)
			return
		}
		if uc == nil {
			return
		}
		u.sheetsInputFileURI = uc.URI()
		u.sheetsInputFileLabel.SetText(uc.URI().Path())
		// Clear the ref field so only one source is active.
		u.sheetsRef.SetText("")
	}, u.win)
	fd.SetFilter(storage.NewExtensionFileFilter([]string{".txt"}))
	fd.Show()
}

func (u *ui) onSheetsSubmit() {
	opts := SheetsOptions{
		Ref:       u.sheetsRef.Text,
		InputFile: uriPath(u.sheetsInputFileURI),
		SheetID:   u.sheetsSheetID.Text,
		FolderID:  u.sheetsFolderID.Text,
		Title:     u.sheetsTitle.Text,
	}
	if err := validateSheetsInputs(opts); err != nil {
		dialog.ShowError(err, u.win)
		return
	}

	prog := dialog.NewInformation("Authenticating…", "Opening browser for Google sign-in, then generating your sheet.", u.win)
	prog.Show()

	go func() {
		url, err := u.runner.RunSheets(context.Background(), opts)
		fyne.Do(func() {
			prog.Hide()
			if err != nil {
				dialog.ShowError(err, u.win)
				return
			}
			u.sheetsResult.SetText(url)
			dialog.ShowInformation("Done!", "Your sheet is ready:\n"+url, u.win)
		})
	}()
}
