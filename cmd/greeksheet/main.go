/*
Command greeksheet creates a Greek NT translation-practice spreadsheet in
Google Sheets.

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
	greeksheet -input practice.txt
	greeksheet -input practice.txt -title "My Practice Sheet"
	greeksheet -input practice.txt -sheet-id <ID STRING FROM SHEETS URL>
	greeksheet -ref "John 1:1-14" -title "John 1"
	greeksheet -ref "John 1:1-14" -title "John 1" -folder-id <ID FROM DRIVE FOLDER URL>
	greeksheet -ref "John 1:50-2:10" -title "John cross-chapter"
	greeksheet -ref "Ephesians 1" -title "Ephesians 1"
	greeksheet -ref "Ephesians 1-6" -sheet-id <ID>
	greeksheet -ref "Ephesians 1-6" -title "Ephesians"
	greeksheet -ref "Ephesians 1-6" -title "Ephesians" -folder-id <ID FROM DRIVE FOLDER URL>
*/
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/nathj07/greeksheet/internal/app"
	"github.com/nathj07/greeksheet/internal/auth"
	"github.com/nathj07/greeksheet/internal/output/googlesheets"
	"github.com/nathj07/greeksheet/internal/source"
	"github.com/nathj07/greeksheet/internal/source/greekbible"
	"github.com/nathj07/greeksheet/internal/source/textfile"
)

func main() {
	if err := run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
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
	if *sheetID != "" && *title != "" {
		fmt.Fprintln(os.Stderr, "Warning: -title is ignored when -sheet-id is provided")
	}

	var src source.Source
	if *inputFile != "" {
		if _, err := os.Stat(*inputFile); err != nil {
			return fmt.Errorf("cannot access input file %s: %w", *inputFile, err)
		}
		src = textfile.New(*inputFile)
	} else {
		src = greekbible.New(*refFlag)
	}

	ctx := context.Background()

	// Authenticate before loading content so credential problems surface before
	// the (potentially slow) fetch from greekbible.com.
	fmt.Println("Authenticating with Google…")
	client, err := auth.Authenticate(ctx)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	target := googlesheets.New(client, googlesheets.Options{
		SheetID:  *sheetID,
		FolderID: *folderID,
		Verbose:  *verbose,
	})

	return app.App{
		Source:        src,
		Target:        target,
		TitleOverride: *title,
	}.Run(ctx)
}
