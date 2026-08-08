package main

import (
	"context"
	"errors"
	"testing"

	"fyne.io/fyne/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock Runner
// ---------------------------------------------------------------------------

type mockRunner struct {
	mock.Mock
}

func (m *mockRunner) RunXLSX(ctx context.Context, opts XLSXOptions) (string, error) {
	args := m.Called(ctx, opts)
	return args.String(0), args.Error(1)
}

func (m *mockRunner) RunSheets(ctx context.Context, opts SheetsOptions) (string, error) {
	args := m.Called(ctx, opts)
	return args.String(0), args.Error(1)
}

// ---------------------------------------------------------------------------
// validateXLSXInputs
// ---------------------------------------------------------------------------

func TestValidateXLSXInputs(t *testing.T) {
	f := func(opts XLSXOptions, wantErrContains string) {
		t.Helper()
		err := validateXLSXInputs(opts)
		if wantErrContains == "" {
			require.NoError(t, err)
			return
		}
		require.Error(t, err)
		assert.ErrorContains(t, err, wantErrContains)
	}

	valid := XLSXOptions{Ref: "John 1:1-14", XLSXFile: "/tmp/out.xlsx"}

	t.Run("valid_ref_and_file", func(t *testing.T) { f(valid, "") })
	t.Run("valid_input_file_and_xlsx", func(t *testing.T) {
		f(XLSXOptions{InputFile: "/tmp/verses.txt", XLSXFile: "/tmp/out.xlsx"}, "")
	})
	t.Run("ref_and_input_file_mutually_exclusive", func(t *testing.T) {
		f(XLSXOptions{Ref: "John 1:1-14", InputFile: "/tmp/f.txt", XLSXFile: "/tmp/out.xlsx"}, "mutually exclusive")
	})
	t.Run("no_source_provided", func(t *testing.T) {
		f(XLSXOptions{XLSXFile: "/tmp/out.xlsx"}, "scripture reference or an input file is required")
	})
	t.Run("no_xlsx_path", func(t *testing.T) {
		f(XLSXOptions{Ref: "John 1:1-14"}, "output .xlsx file path is required")
	})
	t.Run("xlsx_wrong_extension", func(t *testing.T) {
		f(XLSXOptions{Ref: "John 1:1-14", XLSXFile: "/tmp/out.csv"}, ".xlsx extension")
	})
	t.Run("xlsx_extension_case_insensitive", func(t *testing.T) {
		f(XLSXOptions{Ref: "John 1:1-14", XLSXFile: "/tmp/out.XLSX"}, "")
	})
}

// ---------------------------------------------------------------------------
// validateSheetsInputs
// ---------------------------------------------------------------------------

func TestValidateSheetsInputs(t *testing.T) {
	f := func(opts SheetsOptions, wantErrContains string) {
		t.Helper()
		err := validateSheetsInputs(opts)
		if wantErrContains == "" {
			require.NoError(t, err)
			return
		}
		require.Error(t, err)
		assert.ErrorContains(t, err, wantErrContains)
	}

	t.Run("valid_ref_only", func(t *testing.T) {
		f(SheetsOptions{Ref: "John 1:1-14"}, "")
	})
	t.Run("valid_input_file_only", func(t *testing.T) {
		f(SheetsOptions{InputFile: "/tmp/verses.txt"}, "")
	})
	t.Run("valid_ref_with_sheet_id", func(t *testing.T) {
		f(SheetsOptions{Ref: "John 1:1-14", SheetID: "abc123"}, "")
	})
	t.Run("valid_ref_with_folder_id", func(t *testing.T) {
		f(SheetsOptions{Ref: "John 1:1-14", FolderID: "folder456"}, "")
	})
	t.Run("ref_and_input_file_mutually_exclusive", func(t *testing.T) {
		f(SheetsOptions{Ref: "John 1:1-14", InputFile: "/tmp/f.txt"}, "mutually exclusive")
	})
	t.Run("no_source_provided", func(t *testing.T) {
		f(SheetsOptions{SheetID: "abc123"}, "scripture reference or an input file is required")
	})
	t.Run("sheet_id_and_folder_id_mutually_exclusive", func(t *testing.T) {
		f(SheetsOptions{Ref: "John 1:1-14", SheetID: "abc", FolderID: "def"}, "mutually exclusive")
	})
}

// ---------------------------------------------------------------------------
// uriPath helper
// ---------------------------------------------------------------------------

func TestUriPath(t *testing.T) {
	f := func(uri fyne.URI, want string) {
		t.Helper()
		assert.Equal(t, want, uriPath(uri))
	}

	t.Run("nil_returns_empty_string", func(t *testing.T) { f(nil, "") })
	t.Run("uri_returns_its_path", func(t *testing.T) { f(fakeURI{path: "/tmp/verses.txt"}, "/tmp/verses.txt") })
}

// ---------------------------------------------------------------------------
// ui.xlsxFilePath
// ---------------------------------------------------------------------------

// fakeURI is a minimal fyne.URI implementation for tests, avoiding a real
// Fyne display environment.
type fakeURI struct{ path string }

func (f fakeURI) Extension() string { return ".xlsx" }
func (f fakeURI) Name() string      { return "file.xlsx" }
func (f fakeURI) MimeType() string {
	return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
}
func (f fakeURI) Scheme() string    { return "file" }
func (f fakeURI) Authority() string { return "" }
func (f fakeURI) Path() string      { return f.path }
func (f fakeURI) Query() string     { return "" }
func (f fakeURI) Fragment() string  { return "" }
func (f fakeURI) String() string    { return "file://" + f.path }

// fakeListableURI additionally implements fyne.ListableURI.
type fakeListableURI struct{ fakeURI }

func (f fakeListableURI) List() ([]fyne.URI, error) { return nil, nil }

func TestXlsxFilePath(t *testing.T) {
	f := func(fileURI fyne.URI, folderURI fyne.ListableURI, title, want string) {
		t.Helper()
		u := &ui{xlsxFileURI: fileURI, xlsxFolderURI: folderURI}
		assert.Equal(t, want, u.xlsxFilePath(title))
	}

	t.Run("file_uri_set_returns_its_path", func(t *testing.T) {
		f(fakeURI{path: "/home/user/practice.xlsx"}, nil, "", "/home/user/practice.xlsx")
	})
	t.Run("folder_uri_uses_title_as_filename", func(t *testing.T) {
		f(nil, fakeListableURI{fakeURI{path: "/home/user/sheets"}}, "John 1", "/home/user/sheets/John 1.xlsx")
	})
	t.Run("folder_uri_falls_back_to_greeksheet_when_title_empty", func(t *testing.T) {
		f(nil, fakeListableURI{fakeURI{path: "/home/user/sheets"}}, "", "/home/user/sheets/greeksheet.xlsx")
	})
	t.Run("neither_set_returns_empty_string", func(t *testing.T) {
		f(nil, nil, "", "")
	})
	// file takes priority over folder — guards the internal ordering assumption
	// in case both fields are somehow non-nil (defensive test, not a normal flow).
	t.Run("file_uri_takes_precedence_over_folder", func(t *testing.T) {
		f(
			fakeURI{path: "/home/user/existing.xlsx"},
			fakeListableURI{fakeURI{path: "/home/user/sheets"}},
			"ignored",
			"/home/user/existing.xlsx",
		)
	})
}

// ---------------------------------------------------------------------------
// sanitiseFilename
// ---------------------------------------------------------------------------

func TestSanitiseFilename(t *testing.T) {
	f := func(title, want string) {
		t.Helper()
		assert.Equal(t, want, sanitiseFilename(title))
	}

	t.Run("plain_title_unchanged", func(t *testing.T) { f("John 1", "John 1") })
	t.Run("empty_returns_greeksheet", func(t *testing.T) { f("", "greeksheet") })
	t.Run("whitespace_only_returns_greeksheet", func(t *testing.T) { f("   ", "greeksheet") })
	t.Run("slashes_replaced", func(t *testing.T) { f("a/b\\c", "a_b_c") })
	t.Run("colon_replaced", func(t *testing.T) { f("John 1:1-14", "John 1_1-14") })
}

// ---------------------------------------------------------------------------
// Source mutual exclusion — OnChanged clearing
// ---------------------------------------------------------------------------
// These tests exercise the OnChanged callbacks that keep ref and input file
// mutually exclusive. The callbacks are wired in excelTab()/sheetsTab(), so we
// call those methods on a headless ui to register them before firing OnChanged.

func TestExcelTabTypingRefClearsInputFile(t *testing.T) {
	u := newUIForTest(&mockRunner{})

	// Simulate a file having been picked first.
	u.xlsxInputFileURI = fakeURI{path: "/tmp/verses.txt"}
	u.xlsxInputFileLabel.SetText("/tmp/verses.txt")

	// Typing in the ref field must clear the file selection.
	u.xlsxRef.OnChanged("John 1:1-14")

	assert.Nil(t, u.xlsxInputFileURI)
	assert.Equal(t, "No file selected", u.xlsxInputFileLabel.Text)
}

// TestExcelTabClearingRefPreservesInputFile confirms that blanking the ref
// entry (e.g. when showInputFilePicker calls ref.SetText("")) does not clear
// a file that was just picked — the two inputs are mutually exclusive, but the
// guard only fires when the user is actively typing a non-empty ref.
func TestExcelTabClearingRefPreservesInputFile(t *testing.T) {
	u := newUIForTest(&mockRunner{})

	u.xlsxInputFileURI = fakeURI{path: "/tmp/verses.txt"}
	u.xlsxInputFileLabel.SetText("/tmp/verses.txt")

	// Clearing the ref entry (e.g. after picking a file) must NOT clear the URI.
	u.xlsxRef.OnChanged("")

	assert.Equal(t, fakeURI{path: "/tmp/verses.txt"}, u.xlsxInputFileURI)
	assert.Equal(t, "/tmp/verses.txt", u.xlsxInputFileLabel.Text)
}

func TestSheetsTabTypingRefClearsInputFile(t *testing.T) {
	u := newUIForTest(&mockRunner{})

	u.sheetsInputFileURI = fakeURI{path: "/tmp/verses.txt"}
	u.sheetsInputFileLabel.SetText("/tmp/verses.txt")

	u.sheetsRef.OnChanged("Ephesians 1-6")

	assert.Nil(t, u.sheetsInputFileURI)
	assert.Equal(t, "No file selected", u.sheetsInputFileLabel.Text)
}

// TestSheetsTabClearingRefPreservesInputFile mirrors the Excel tab test for
// the Sheets tab's wireRefClearsFile wiring.
func TestSheetsTabClearingRefPreservesInputFile(t *testing.T) {
	u := newUIForTest(&mockRunner{})

	u.sheetsInputFileURI = fakeURI{path: "/tmp/verses.txt"}
	u.sheetsInputFileLabel.SetText("/tmp/verses.txt")

	u.sheetsRef.OnChanged("")

	assert.Equal(t, fakeURI{path: "/tmp/verses.txt"}, u.sheetsInputFileURI)
	assert.Equal(t, "/tmp/verses.txt", u.sheetsInputFileLabel.Text)
}

// ---------------------------------------------------------------------------
// buildSource
// ---------------------------------------------------------------------------

func TestBuildSource(t *testing.T) {
	f := func(ref, inputFile, wantErrContains string) {
		t.Helper()
		src, err := buildSource(ref, inputFile)
		if wantErrContains != "" {
			require.Error(t, err)
			assert.ErrorContains(t, err, wantErrContains)
			return
		}
		require.NoError(t, err)
		assert.NotNil(t, src)
	}

	t.Run("ref_returns_non_nil_source", func(t *testing.T) { f("John 1:1-14", "", "") })
	t.Run("input_file_returns_non_nil_source", func(t *testing.T) { f("", "/tmp/verses.txt", "") })
	t.Run("both_empty_returns_error", func(t *testing.T) {
		f("", "", "either a scripture reference or an input file must be provided")
	})
	t.Run("both_provided_returns_error", func(t *testing.T) {
		f("John 1:1-14", "/tmp/verses.txt", "mutually exclusive")
	})
}

// ---------------------------------------------------------------------------
// Submit handler validation gate — tested via the pure validation functions
// ---------------------------------------------------------------------------
// onXLSXSubmit and onSheetsSubmit call dialog.ShowError on validation failure,
// which requires a live Fyne display and cannot run headlessly. The validation
// logic itself is tested exhaustively through validateXLSXInputs and
// validateSheetsInputs above. These tests confirm that the validation functions
// return errors for the same empty-input scenarios the submit handlers would
// encounter, providing the same coverage without touching the display layer.

func TestXLSXSubmitValidationGateEmptyInputsReturnsError(t *testing.T) {
	err := validateXLSXInputs(XLSXOptions{})
	require.Error(t, err)
}

func TestSheetsSubmitValidationGateEmptyInputsReturnsError(t *testing.T) {
	err := validateSheetsInputs(SheetsOptions{})
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// Runner interface — mock contract tests
// ---------------------------------------------------------------------------
// These tests verify the Runner interface contract using a mock. The submit
// handlers call the Runner in a goroutine with a live Fyne display, which
// cannot run headlessly; these tests exercise the Runner directly.

func TestMockRunnerRunXLSXReturnsPath(t *testing.T) {
	runner := &mockRunner{}
	opts := XLSXOptions{
		Ref:      "John 1:1-14",
		XLSXFile: "/tmp/test.xlsx",
	}
	runner.On("RunXLSX", mock.Anything, opts).Return("/tmp/test.xlsx", nil)

	u := newUIForTest(runner)
	path, err := u.runner.RunXLSX(t.Context(), opts)

	require.NoError(t, err)
	assert.Equal(t, "/tmp/test.xlsx", path)
	runner.AssertExpectations(t)
}

func TestMockRunnerRunSheetsReturnsURL(t *testing.T) {
	runner := &mockRunner{}
	opts := SheetsOptions{
		Ref:   "Ephesians 1-6",
		Title: "Ephesians",
	}
	runner.On("RunSheets", mock.Anything, opts).
		Return("https://docs.google.com/spreadsheets/d/abc", nil)

	u := newUIForTest(runner)
	url, err := u.runner.RunSheets(t.Context(), opts)

	require.NoError(t, err)
	assert.Equal(t, "https://docs.google.com/spreadsheets/d/abc", url)
	runner.AssertExpectations(t)
}

func TestMockRunnerRunXLSXPropagatesError(t *testing.T) {
	runner := &mockRunner{}
	opts := XLSXOptions{
		Ref:      "John 1:1-14",
		XLSXFile: "/tmp/test.xlsx",
	}
	runner.On("RunXLSX", mock.Anything, opts).Return("", errors.New("network failure"))

	u := newUIForTest(runner)
	_, err := u.runner.RunXLSX(t.Context(), opts)

	require.Error(t, err)
	assert.ErrorContains(t, err, "network failure")
	runner.AssertExpectations(t)
}

// newUIForTest constructs a ui with stub widgets suitable for headless tests.
// Fyne widgets can be created without a running display in test code.
func newUIForTest(runner Runner) *ui {
	return newUI(nil, runner)
}
