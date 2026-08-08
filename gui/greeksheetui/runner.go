package main

import (
	"context"
	"fmt"

	"github.com/nathj07/greeksheet/internal/app"
	"github.com/nathj07/greeksheet/internal/auth"
	"github.com/nathj07/greeksheet/internal/output/googlesheets"
	"github.com/nathj07/greeksheet/internal/output/xlsx"
	"github.com/nathj07/greeksheet/internal/source"
	"github.com/nathj07/greeksheet/internal/source/greekbible"
	"github.com/nathj07/greeksheet/internal/source/textfile"
)

// Runner is the seam between the UI and the core generation pipeline. It lets
// tests replace the real pipeline with a stub so no network or filesystem calls
// are needed during testing.
type Runner interface {
	RunXLSX(ctx context.Context, opts XLSXOptions) (string, error)
	RunSheets(ctx context.Context, opts SheetsOptions) (string, error)
}

// XLSXOptions mirrors the xlsx-specific CLI flags.
type XLSXOptions struct {
	// XLSXFile is the full path to the .xlsx output file.
	XLSXFile string
	// Ref is a scripture reference string (e.g. "John 1:1-14"). Mutually
	// exclusive with InputFile.
	Ref string
	// InputFile is a path to a plain-text verse file. Mutually exclusive with Ref.
	InputFile string
	// Title is an optional spreadsheet title override.
	Title string
}

// SheetsOptions mirrors the Google Sheets-specific CLI flags.
type SheetsOptions struct {
	// SheetID is an existing Google Spreadsheet ID to append to. Mutually
	// exclusive with FolderID.
	SheetID string
	// FolderID is the Drive folder ID for a new spreadsheet.
	FolderID string
	// Ref is a scripture reference string. Mutually exclusive with InputFile.
	Ref string
	// InputFile is a path to a plain-text verse file. Mutually exclusive with Ref.
	InputFile string
	// Title is an optional spreadsheet title override.
	Title string
}

// AppRunner is the production Runner that delegates to the real app pipeline.
type AppRunner struct{}

// RunXLSX runs the full generation pipeline writing output to a local .xlsx file.
func (AppRunner) RunXLSX(ctx context.Context, opts XLSXOptions) (string, error) {
	src, err := buildSource(opts.Ref, opts.InputFile)
	if err != nil {
		return "", err
	}
	target := xlsx.New(xlsx.Options{File: opts.XLSXFile})
	return app.App{
		Source:        src,
		Target:        target,
		TitleOverride: opts.Title,
	}.Run(ctx)
}

// RunSheets runs the full generation pipeline writing output to Google Sheets.
// It opens a browser for OAuth on the first run; subsequent runs use the cached
// token at ~/.config/greeksheet/token.json.
func (AppRunner) RunSheets(ctx context.Context, opts SheetsOptions) (string, error) {
	src, err := buildSource(opts.Ref, opts.InputFile)
	if err != nil {
		return "", err
	}
	client, err := auth.Authenticate(ctx)
	if err != nil {
		return "", err
	}
	target := googlesheets.New(client, googlesheets.Options{
		SheetID:  opts.SheetID,
		FolderID: opts.FolderID,
	})
	return app.App{
		Source:        src,
		Target:        target,
		TitleOverride: opts.Title,
	}.Run(ctx)
}

// buildSource constructs the appropriate Source from a scripture reference
// string or a local plain-text input file path. Exactly one of ref or
// inputFile must be non-empty; the function returns an error if both are empty.
func buildSource(ref, inputFile string) (source.Source, error) {
	if inputFile != "" {
		return textfile.New(inputFile), nil
	}
	if ref != "" {
		return greekbible.New(ref), nil
	}
	return nil, fmt.Errorf("either a scripture reference or an input file must be provided")
}
