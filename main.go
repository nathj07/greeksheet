/*
Creates a Greek NT translation-practice spreadsheet in Google Sheets from a
plain text file of verses copied from greekbible.com (or any source using the
same format).

Input file format
-----------------
Lines beginning with '#' are treated as section headings and rendered as a bold
label row in the sheet (the '#' is stripped). All other non-blank content is
treated as a block of verses in the greekbible.com inline format:

	1 word word word. 2 word word...

Example input file (e.g. practice.txt):

	# 1 Corinthians 13
	1 Ἐὰν ταῖς γλώσσαις...13 νυνὶ δὲ μένει...
	# 1 Corinthians 14
	1 Διώκετε τὴν ἀγάπην...

Usage:

	go run . -input practice.txt
	go run . -input practice.txt -title "My Practice Sheet"
	go run . -input practice.txt -sheet-id <ID STRING FROM SHEETS URL>
*/
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// clientSecretJSON is used only to validate the secrets file at startup.
type clientSecretJSON struct {
	Installed *struct{} `json:"installed"`
	Web       *struct{} `json:"web"`
}

type config struct {
	InputFile   string
	Title       string
	SecretsFile string
	SheetID     string // non-empty = add a tab to this existing spreadsheet
}

type app struct {
	conf config
}

func main() {
	if err := start(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func start(args []string) error {
	flags := flag.NewFlagSet(args[0], flag.ExitOnError)
	inputFile := flags.String("input", "", "Path to the input text file of Greek verses (required)")
	title := flags.String("title", "", "Title for the Google Sheet (defaults to the input filename)")
	secretsFile := flags.String("secrets", "client_secret.json", "Path to the Google OAuth2 client secrets JSON file")
	sheetID := flags.String("sheet-id", "", "ID of an existing Google Spreadsheet to add a tab to (optional; omit to create a new sheet)")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}

	if *inputFile == "" {
		return fmt.Errorf("usage: %s -input <file> [-title <name>] [-sheet-id <id>] [-secrets <path>]", args[0])
	}

	conf := config{
		InputFile:   *inputFile,
		Title:       *title,
		SecretsFile: *secretsFile,
		SheetID:     *sheetID,
	}
	if conf.SheetID != "" && *title != "" {
		fmt.Fprintln(os.Stderr, "Warning: -title is ignored when -sheet-id is provided")
	}
	if conf.Title == "" {
		base := filepath.Base(conf.InputFile)
		conf.Title = strings.TrimSuffix(base, filepath.Ext(base))
	}

	if _, err := os.Stat(conf.InputFile); err != nil {
		return fmt.Errorf("file not found: %s", conf.InputFile)
	}

	a := &app{conf: conf}
	return a.run(context.Background())
}

func (a *app) run(ctx context.Context) error {
	sections, err := parseInputFile(a.conf.InputFile)
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}
	if len(sections) == 0 {
		return fmt.Errorf("no content found in %s", a.conf.InputFile)
	}

	var totalVerses int
	for _, s := range sections {
		totalVerses += len(s.verses)
	}
	fmt.Printf("Parsed %d verses from '%s'\n", totalVerses, a.conf.InputFile)

	tabName := deriveTabName(sections)
	d := buildSheetData(sections)

	// Validate that client_secret.json exists and is parseable before opening
	// the browser, so we fail fast with a clear error message.
	secretData, err := os.ReadFile(a.conf.SecretsFile)
	if err != nil {
		return fmt.Errorf("client_secret.json not found at %s: %w", a.conf.SecretsFile, err)
	}
	var csj clientSecretJSON
	if err := json.Unmarshal(secretData, &csj); err != nil {
		return fmt.Errorf("invalid client_secret.json: %w", err)
	}
	if csj.Installed == nil && csj.Web == nil {
		return fmt.Errorf("client_secret.json must contain an 'installed' or 'web' key")
	}

	fmt.Println("Authenticating with Google…")
	httpClient, err := authenticate(ctx, a.conf.SecretsFile)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	var url string
	if a.conf.SheetID != "" {
		url, err = addTabToSpreadsheet(ctx, httpClient, a.conf.SheetID, tabName, d)
	} else {
		url, err = createSpreadsheet(ctx, httpClient, a.conf.Title, tabName, d)
	}
	if err != nil {
		return err
	}
	fmt.Printf("\nDone! Open your sheet at:\n  %s\n", url)
	return nil
}
