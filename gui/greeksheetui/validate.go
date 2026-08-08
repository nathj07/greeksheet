package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// validateXLSXInputs checks that the xlsx-tab form values are self-consistent
// before the pipeline is invoked. It mirrors the validation in the CLI's run()
// function so the two code paths enforce the same rules.
func validateXLSXInputs(opts XLSXOptions) error {
	if opts.Ref != "" && opts.InputFile != "" {
		return fmt.Errorf("ref and input file are mutually exclusive: provide one or the other, not both")
	}
	if opts.Ref == "" && opts.InputFile == "" {
		return fmt.Errorf("a scripture reference or an input file is required")
	}
	if opts.XLSXFile == "" {
		return fmt.Errorf("an output .xlsx file path is required")
	}
	if !strings.EqualFold(filepath.Ext(opts.XLSXFile), ".xlsx") {
		return fmt.Errorf("output file %q must have a .xlsx extension", opts.XLSXFile)
	}
	return nil
}

// validateSheetsInputs checks that the Google Sheets-tab form values are
// self-consistent before the pipeline is invoked.
func validateSheetsInputs(opts SheetsOptions) error {
	if opts.Ref != "" && opts.InputFile != "" {
		return fmt.Errorf("ref and input file are mutually exclusive: provide one or the other, not both")
	}
	if opts.Ref == "" && opts.InputFile == "" {
		return fmt.Errorf("a scripture reference or an input file is required")
	}
	if opts.SheetID != "" && opts.FolderID != "" {
		return fmt.Errorf("sheet ID and folder ID are mutually exclusive: provide one or the other, not both")
	}
	return nil
}
