/*
Creates a Greek NT translation-practice spreadsheet in Google Sheets.

There are two input modes — exactly one must be provided:

  -input <file>
	Read verses from a plain text file copied from greekbible.com.
	Lines beginning with '#' become bold chapter-heading rows (the '#' is
	stripped). All other non-blank lines are treated as a block of verses in
	the greekbible.com inline format:

		1 word word word. 2 word word...

  -ref "<Book ch:v-v>"
	Fetch Greek text directly from greekbible.com for the given reference
	range. Supports same-chapter ranges ("John 1:1-10") and cross-chapter
	ranges ("John 1:50-2:10"). Chapter headings are inserted automatically.

Example input file (e.g. practice.txt):

	# 1 Corinthians 13
	1 Ἐὰν ταῖς γλώσσαις...13 νυνὶ δὲ μένει...
	# 1 Corinthians 14
	1 Διώκετε τὴν ἀγάπην...

Usage:

	go run . -input practice.txt
	go run . -input practice.txt -title "My Practice Sheet"
	go run . -input practice.txt -sheet-id <ID STRING FROM SHEETS URL>
	go run . -ref "John 1:1-14" -title "John 1"
	go run . -ref "John 1:50-2:10" -title "John cross-chapter"
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
	Ref         string // non-empty = fetch mode; mutually exclusive with InputFile
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
	inputFile := flags.String("input", "", "Path to the input text file of Greek verses")
	refFlag := flags.String("ref", "", "Reference range to fetch from greekbible.com, e.g. \"John 1:1-10\" or \"John 1:50-2:10\"")
	title := flags.String("title", "", "Title for the Google Sheet (defaults to the input filename or ref)")
	secretsFile := flags.String("secrets", "client_secret.json", "Path to the Google OAuth2 client secrets JSON file")
	sheetID := flags.String("sheet-id", "", "ID of an existing Google Spreadsheet to add a tab to (optional; omit to create a new sheet)")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}

	if *inputFile == "" && *refFlag == "" {
		return fmt.Errorf("usage: %s (-input <file> | -ref <range>) [-title <name>] [-sheet-id <id>] [-secrets <path>]", args[0])
	}
	if *inputFile != "" && *refFlag != "" {
		return fmt.Errorf("-input and -ref are mutually exclusive: provide one or the other, not both")
	}

	conf := config{
		InputFile:   *inputFile,
		Ref:         *refFlag,
		Title:       *title,
		SecretsFile: *secretsFile,
		SheetID:     *sheetID,
	}
	if conf.SheetID != "" && *title != "" {
		fmt.Fprintln(os.Stderr, "Warning: -title is ignored when -sheet-id is provided")
	}
	if conf.Title == "" {
		if conf.InputFile != "" {
			base := filepath.Base(conf.InputFile)
			conf.Title = strings.TrimSuffix(base, filepath.Ext(base))
		} else {
			conf.Title = conf.Ref
		}
	}

	if conf.InputFile != "" {
		if _, err := os.Stat(conf.InputFile); err != nil {
			return fmt.Errorf("cannot access input file %s: %w", conf.InputFile, err)
		}
	}

	a := &app{conf: conf}
	return a.run(context.Background())
}

func (a *app) run(ctx context.Context) error {
	var (
		sections []section
		tabName  string
		err      error
	)

	if a.conf.Ref != "" {
		fmt.Printf("Fetching Greek text for %q from greekbible.com…\n", a.conf.Ref)
		sections, tabName, err = fetchSections(ctx, a.conf.Ref)
		if err != nil {
			return fmt.Errorf("fetching verses: %w", err)
		}
	} else {
		sections, err = parseInputFile(a.conf.InputFile)
		if err != nil {
			return fmt.Errorf("reading input: %w", err)
		}
		tabName = deriveTabName(sections)
	}

	if len(sections) == 0 {
		return fmt.Errorf("no content found")
	}

	var totalVerses int
	for _, s := range sections {
		totalVerses += len(s.verses)
	}
	if totalVerses == 0 {
		return fmt.Errorf("no verses found — check that the reference or input file contains valid content")
	}
	fmt.Printf("Parsed %d verses\n", totalVerses)
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
