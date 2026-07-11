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
		ranges ("John 1:50-2:10"). One HTTP request is made per chapter; verses
		are filtered in-process to the requested range.

	  -ref "<Book ch-ch>"
		Fetch entire chapters from greekbible.com. The whole-chapter format is
		detected automatically — one Google Sheets tab per chapter is created,
		each named by chapter number ("1", "2", …). Use -sheet-id to add tabs
		to an existing spreadsheet, or omit it to create a new one.

	  -ref "<Book ch>"
		Fetch a single whole chapter. Shorthand for the whole-chapter format
		above when only one chapter is needed — no need to write "ch-ch".

All ref formats are limited to a single book. Cross-book ranges such as
"Ephesians 6:1-Philippians 2:6" are not supported.

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
	go run . -ref "John 1:1-14" -title "John 1" -folder-id <ID FROM DRIVE FOLDER URL>
	go run . -ref "John 1:50-2:10" -title "John cross-chapter"
	go run . -ref "Ephesians 1" -title "Ephesians 1"
	go run . -ref "Ephesians 1-6" -sheet-id <ID>
	go run . -ref "Ephesians 1-6" -title "Ephesians"
	go run . -ref "Ephesians 1-6" -title "Ephesians" -folder-id <ID FROM DRIVE FOLDER URL>
*/
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type config struct {
	InputFile string
	Ref       string // non-empty = fetch mode; mutually exclusive with InputFile
	Title     string
	SheetID   string // non-empty = add a tab to this existing spreadsheet
	FolderID  string // non-empty = create the new spreadsheet inside this Drive folder
	Verbose   bool   // log Sheets API retry attempts to stderr
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
	refFlag := flags.String("ref", "", "Reference to fetch from greekbible.com (see ref formats below)")
	title := flags.String("title", "", "Title for the Google Sheet (defaults to the input filename or ref)")
	sheetID := flags.String("sheet-id", "", "ID of an existing Google Spreadsheet to add a tab to (optional; omit to create a new sheet)")
	folderID := flags.String("folder-id", "", "Google Drive folder ID to create the new spreadsheet in (optional; find it in the folder's URL)")
	verbose := flags.Bool("verbose", false, "Log Sheets API retry attempts to stderr")

	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, `Greek NT translation-practice spreadsheet generator.

Fetches Greek text from greekbible.com and writes it to a Google Sheet
formatted for translation practice. Exactly one of -input or -ref is required.

Usage:
  %s (-input <file> | -ref <ref>) [options]

Ref formats:
  "Book ch:v-v"       verse range within one chapter   e.g. "John 1:1-14"
  "Book ch:v-ch:v"    cross-chapter verse range         e.g. "John 1:50-2:10"
  "Book ch-ch"        whole chapters, one tab each      e.g. "Ephesians 1-6"
  "Book ch"           single whole chapter              e.g. "Ephesians 1"

Options:
`, args[0])
		flags.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  %s -input practice.txt
  %s -ref "John 1:1-14" -title "John 1"
  %s -ref "John 1:50-2:10" -title "John cross-chapter"
  %s -ref "Ephesians 1" -title "Ephesians 1"
  %s -ref "Ephesians 1-6" -title "Ephesians"
  %s -ref "Ephesians 1-6" -sheet-id <ID>
`, args[0], args[0], args[0], args[0], args[0], args[0])
	}

	if err := flags.Parse(args[1:]); err != nil {
		return err
	}

	if *inputFile == "" && *refFlag == "" {
		flags.Usage()
		return nil
	}
	if *inputFile != "" && *refFlag != "" {
		return fmt.Errorf("-input and -ref are mutually exclusive: provide one or the other, not both")
	}
	if *sheetID != "" && *folderID != "" {
		return fmt.Errorf("-sheet-id and -folder-id are mutually exclusive: -folder-id only applies when creating a new spreadsheet, but -sheet-id targets an existing one")
	}

	conf := config{
		InputFile: *inputFile,
		Ref:       *refFlag,
		Title:     *title,
		SheetID:   *sheetID,
		FolderID:  *folderID,
		Verbose:   *verbose,
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

	fmt.Println("Authenticating with Google…")
	httpClient, err := authenticate(ctx)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	if a.conf.Ref != "" {
		// Whole-chapter format ("Book ch-ch") is detected automatically and
		// creates one tab per chapter. Single-chapter format ("Book ch") is
		// detected next and creates exactly one tab. Verse-range format
		// ("Book ch:v-v") is the fallback for a single-tab fetch.
		//
		// We check whether the input structurally looks like a chapter range
		// (matches the pattern) before deciding which path to take. This way,
		// an inverted range such as "Ephesians 6-1" surfaces the specific
		// chapter-range error rather than silently falling through to
		// verse-range parsing and returning a confusing "invalid reference" error.
		if chapterRangeRE.MatchString(strings.TrimSpace(a.conf.Ref)) {
			cr, err := parseChapterRange(a.conf.Ref)
			if err != nil {
				return err
			}
			return a.runChapterPerTab(ctx, httpClient, cr)
		}
		if singleChapterRE.MatchString(strings.TrimSpace(a.conf.Ref)) {
			cr, err := parseSingleChapter(a.conf.Ref)
			if err != nil {
				return err
			}
			return a.runChapterPerTab(ctx, httpClient, cr)
		}
		// we will check the ref in fetchSections
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

	var url string
	if a.conf.SheetID != "" {
		url, err = a.addTabToSpreadsheet(ctx, httpClient, a.conf.SheetID, tabName, d)
	} else {
		url, err = a.createSpreadsheet(ctx, httpClient, tabName, d)
	}
	if err != nil {
		return err
	}
	fmt.Printf("\nDone! Open your sheet at:\n  %s\n", url)
	return nil
}

// runChapterPerTab fetches a whole-chapter range and creates one Google Sheets
// tab per chapter. Each tab is named by chapter number ("1", "2", …).
//
// If -sheet-id is provided the tabs are added to that existing spreadsheet.
// Otherwise the first chapter's fetch creates a new spreadsheet and subsequent
// chapters add tabs to it.
func (a *app) runChapterPerTab(ctx context.Context, httpClient *http.Client, cr chapterRange) error {
	fmt.Printf("Fetching %s chapters %d–%d from greekbible.com…\n", cr.book, cr.startChapter, cr.endChapter)

	// client fetches chapter HTML from greekbible.com. httpClient is the
	// OAuth-authenticated client used only for Google Sheets/Drive API calls.
	client := &http.Client{Timeout: 15 * time.Second}
	spreadsheetID := a.conf.SheetID
	var sheetURL string

	for ch := cr.startChapter; ch <= cr.endChapter; ch++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		sections, tabName, err := fetchChapterSections(ctx, client, cr, ch)
		if err != nil {
			return err
		}

		var totalVerses int
		for _, s := range sections {
			totalVerses += len(s.verses)
		}
		fmt.Printf("  Chapter %d: %d verses\n", ch, totalVerses)

		d := buildSheetData(sections)

		if spreadsheetID == "" {
			// First chapter — create the spreadsheet; subsequent chapters add tabs.
			sheetURL, err = a.createSpreadsheet(ctx, httpClient, tabName, d)
			if err != nil {
				return err
			}
			spreadsheetID, err = extractSpreadsheetID(sheetURL)
			if err != nil {
				return err
			}
		} else {
			sheetURL, err = a.addTabToSpreadsheet(ctx, httpClient, spreadsheetID, tabName, d)
			if err != nil {
				return err
			}
		}

		if ch < cr.endChapter {
			select {
			case <-time.After(fetchDelay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	fmt.Printf("\nDone! Open your sheet at:\n  %s\n", sheetURL)
	return nil
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
