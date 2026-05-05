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

	go run create_greek_sheet.go -input practice.txt
	go run create_greek_sheet.go -input practice.txt -title "My Practice Sheet"
	go run create_greek_sheet.go -input practice.txt -sheet-id 1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgVE2upms
*/
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkg/browser"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

// ---------------------------------------------------------------------------
// Colours (RGB floats for the Sheets API)
// ---------------------------------------------------------------------------

type rgbColor struct{ Red, Green, Blue float64 }

func hexToRGB(hex string) rgbColor {
	hex = strings.TrimPrefix(hex, "#")
	parse := func(i int) float64 {
		var v uint8
		fmt.Sscanf(hex[i:i+2], "%02x", &v)
		return float64(v) / 255
	}
	return rgbColor{Red: parse(0), Green: parse(2), Blue: parse(4)}
}

var (
	colGrey   = hexToRGB("d9d9d9") // verse row
	colOrange = hexToRGB("fce5cd") // I row — interlinear practice
	colGreen  = hexToRGB("b7e1cd") // T row — translation practice

	// trailingDigitRE matches a heading token that is a pure chapter number,
	// e.g. the "13" in "1 Corinthians 13".
	trailingDigitRE = regexp.MustCompile(`^\d+$`)
)

func toAPIColor(c rgbColor) *sheets.Color {
	return &sheets.Color{Red: c.Red, Green: c.Green, Blue: c.Blue}
}

// ---------------------------------------------------------------------------
// Parsing
// ---------------------------------------------------------------------------

type section struct {
	heading string
	verses  []verse // non-nil only when heading == ""
}

type verse struct {
	num   string
	words []string
}

// parseInputFile reads the input file and returns a flat list of sections.
// Heading sections carry a text label; verse sections carry parsed verse data.
func parseInputFile(path string) ([]section, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var sections []section
	var pending []string

	flushPending := func() {
		if len(pending) == 0 {
			return
		}
		raw := strings.Join(pending, " ")
		pending = pending[:0]
		vs := parseVerses(raw)
		if len(vs) > 0 {
			sections = append(sections, section{verses: vs})
		}
	}

	scanner := bufio.NewScanner(f)
	// Default scanner buffer (64 KB) can overflow for long chapters pasted as a single line;
	// 1 MB handles even the largest NT books comfortably.
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			flushPending()
			text := strings.TrimSpace(strings.TrimLeft(line, "# "))
			sections = append(sections, section{heading: text})
		} else if strings.TrimSpace(line) != "" {
			pending = append(pending, strings.TrimSpace(line))
		}
	}
	flushPending()
	return sections, scanner.Err()
}

// verseRE splits greekbible.com copy-paste text: "1 word word. 2 word..."
// We split on a verse number that is not preceded by a word character.
var verseRE = regexp.MustCompile(`(?:^|(?:\W))(\d+)\s`)

// parseVerses parses the inline verse format into (number, words) pairs.
func parseVerses(text string) []verse {
	// FindAllStringSubmatchIndex gives us the locations of each verse-number match.
	matches := verseRE.FindAllStringSubmatchIndex(strings.TrimSpace(text), -1)
	if len(matches) == 0 {
		return nil
	}

	var verses []verse
	for i, m := range matches {
		numStart, numEnd := m[2], m[3]
		num := text[numStart:numEnd]

		// The verse body starts after the number+space and ends at the next match (or EOF).
		bodyStart := m[1] // end of the full match (past the trailing space)
		bodyEnd := len(text)
		if i+1 < len(matches) {
			bodyEnd = matches[i+1][0] // start of next match's leading non-word char
		}

		words := strings.Fields(text[bodyStart:bodyEnd])
		if len(words) > 0 {
			verses = append(verses, verse{num: num, words: words})
		}
	}
	return verses
}

// deriveTabName produces a "ch:v - ch:v" label from the parsed sections,
// suitable for use as a Google Sheets tab title.
//
// Chapter numbers are extracted from the trailing integer of each heading line
// (e.g. "1 Corinthians 13" → chapter 13). If no heading precedes the first
// verse block, chapter 1 is assumed. The range collapses to "ch:v" when the
// input contains exactly one verse. Falls back to "sheet" when no verses are
// found.
func deriveTabName(sections []section) string {
	type ref struct{ ch, v string }
	var first, last ref
	currentChapter := "1"

	for _, sec := range sections {
		if sec.heading != "" {
			// Extract the trailing digit sequence, e.g. "1 Corinthians 13" → "13"
			parts := strings.Fields(sec.heading)
			if len(parts) > 0 {
				candidate := parts[len(parts)-1]
				if trailingDigitRE.MatchString(candidate) {
					currentChapter = candidate
				}
			}
			continue
		}
		for _, v := range sec.verses {
			if first.ch == "" {
				first = ref{ch: currentChapter, v: v.num}
			}
			last = ref{ch: currentChapter, v: v.num}
		}
	}

	if first.ch == "" {
		return "sheet"
	}
	if first.ch == last.ch && first.v == last.v {
		return fmt.Sprintf("%s:%s", first.ch, first.v)
	}
	return fmt.Sprintf("%s:%s - %s:%s", first.ch, first.v, last.ch, last.v)
}

// ---------------------------------------------------------------------------
// Sheet layout builder
// ---------------------------------------------------------------------------

type sheetData struct {
	rows         [][]any
	bgRequests   []*sheets.Request
	mergeReqs    []*sheets.Request
	boldRequests []*sheets.Request
}

// buildSheetData converts parsed sections into row data and Sheets API
// formatting requests (backgrounds, merges, bold headings).
func buildSheetData(sections []section) sheetData {
	var d sheetData

	addVerseBlock := func(v verse) {
		wordCount := len(v.words)
		firstWordCol := int64(1) // column B (0-indexed)
		lastWordCol := int64(wordCount)

		// Verse row — grey background, verse number in col A then words
		r := int64(len(d.rows))
		row := make([]any, 1+wordCount)
		row[0] = v.num
		for i, w := range v.words {
			row[1+i] = w
		}
		d.rows = append(d.rows, row)
		d.bgRequests = append(d.bgRequests, bgReq(r, 0, r+1, lastWordCol+1, colGrey))

		// Two unlabelled rows for parsing and building word choices — one cell per
		// Greek word so each word's work sits directly beneath it.
		d.rows = append(d.rows, make([]any, 1+wordCount), make([]any, 1+wordCount))

		// I row — single merged cell for interlinear practice
		r = int64(len(d.rows))
		iRow := make([]any, 1+wordCount)
		iRow[0] = "I"
		d.rows = append(d.rows, iRow)
		d.bgRequests = append(d.bgRequests, bgReq(r, 0, r+1, lastWordCol+1, colOrange))
		if wordCount > 1 {
			d.mergeReqs = append(d.mergeReqs, mergeReq(r, firstWordCol, r+1, lastWordCol+1))
		}

		// T row — single merged cell for full translation practice
		r = int64(len(d.rows))
		tRow := make([]any, 1+wordCount)
		tRow[0] = "T"
		d.rows = append(d.rows, tRow)
		d.bgRequests = append(d.bgRequests, bgReq(r, 0, r+1, lastWordCol+1, colGreen))
		if wordCount > 1 {
			d.mergeReqs = append(d.mergeReqs, mergeReq(r, firstWordCol, r+1, lastWordCol+1))
		}

		// C row — single merged cell for commentary notes
		r = int64(len(d.rows))
		d.rows = append(d.rows, []any{"C"})
		if wordCount > 1 {
			d.mergeReqs = append(d.mergeReqs, mergeReq(r, firstWordCol, r+1, lastWordCol+1))
		}

		// N row — single merged cell for general notes
		r = int64(len(d.rows))
		d.rows = append(d.rows, []any{"N"})
		if wordCount > 1 {
			d.mergeReqs = append(d.mergeReqs, mergeReq(r, firstWordCol, r+1, lastWordCol+1))
		}
	}

	for _, sec := range sections {
		if sec.heading != "" {
			r := int64(len(d.rows))
			d.rows = append(d.rows, []any{sec.heading})
			d.boldRequests = append(d.boldRequests, boldReq(r, 0, r+1, 1))
		} else {
			for _, v := range sec.verses {
				addVerseBlock(v)
			}
		}
	}

	return d
}

func gridRange(sr, sc, er, ec int64) *sheets.GridRange {
	return &sheets.GridRange{
		SheetId:          0,
		StartRowIndex:    sr,
		EndRowIndex:      er,
		StartColumnIndex: sc,
		EndColumnIndex:   ec,
	}
}

func bgReq(sr, sc, er, ec int64, color rgbColor) *sheets.Request {
	return &sheets.Request{RepeatCell: &sheets.RepeatCellRequest{
		Range: gridRange(sr, sc, er, ec),
		Cell: &sheets.CellData{UserEnteredFormat: &sheets.CellFormat{
			BackgroundColorStyle: &sheets.ColorStyle{RgbColor: toAPIColor(color)},
		}},
		Fields: "userEnteredFormat.backgroundColorStyle",
	}}
}

func mergeReq(sr, sc, er, ec int64) *sheets.Request {
	return &sheets.Request{MergeCells: &sheets.MergeCellsRequest{
		Range:     gridRange(sr, sc, er, ec),
		MergeType: "MERGE_ALL",
	}}
}

func boldReq(sr, sc, er, ec int64) *sheets.Request {
	return &sheets.Request{RepeatCell: &sheets.RepeatCellRequest{
		Range: gridRange(sr, sc, er, ec),
		Cell: &sheets.CellData{UserEnteredFormat: &sheets.CellFormat{
			TextFormat: &sheets.TextFormat{Bold: true},
		}},
		Fields: "userEnteredFormat.textFormat.bold",
	}}
}

func narrowColAReq(sheetID int64) *sheets.Request {
	return &sheets.Request{UpdateDimensionProperties: &sheets.UpdateDimensionPropertiesRequest{
		Range: &sheets.DimensionRange{
			SheetId:    sheetID,
			Dimension:  "COLUMNS",
			StartIndex: 0,
			EndIndex:   1,
		},
		Properties: &sheets.DimensionProperties{PixelSize: 40},
		Fields:     "pixelSize",
	}}
}

// fontSizeReq sets the font size for all cells on the sheet in a single pass.
// Using a SheetId-only range (no row/column bounds) covers the entire sheet.
func fontSizeReq(sheetID int64, size int64) *sheets.Request {
	return &sheets.Request{RepeatCell: &sheets.RepeatCellRequest{
		Range: &sheets.GridRange{SheetId: sheetID},
		Cell: &sheets.CellData{UserEnteredFormat: &sheets.CellFormat{
			TextFormat: &sheets.TextFormat{FontSize: size},
		}},
		Fields: "userEnteredFormat.textFormat.fontSize",
	}}
}

// patchSheetID rewrites every GridRange.SheetId in d's formatting requests to
// the given id. This is needed after a tab is created and its real sheetId
// (assigned by Google) becomes known — buildSheetData uses 0 as a placeholder.
func patchSheetID(d *sheetData, id int64) {
	patchReqs := func(reqs []*sheets.Request) {
		for _, req := range reqs {
			if req.RepeatCell != nil && req.RepeatCell.Range != nil {
				req.RepeatCell.Range.SheetId = id
			}
			if req.MergeCells != nil && req.MergeCells.Range != nil {
				req.MergeCells.Range.SheetId = id
			}
			if req.UpdateDimensionProperties != nil && req.UpdateDimensionProperties.Range != nil {
				req.UpdateDimensionProperties.Range.SheetId = id
			}
		}
	}
	patchReqs(d.bgRequests)
	patchReqs(d.mergeReqs)
	patchReqs(d.boldRequests)
}

// ---------------------------------------------------------------------------
// Google OAuth2 + Sheets upload
// ---------------------------------------------------------------------------

const (
	scopeSheets = "https://www.googleapis.com/auth/spreadsheets"
	scopeDrive  = "https://www.googleapis.com/auth/drive"
)

// authenticate performs the OAuth2 browser flow using a local redirect server
// (matching the pattern Google's own Python library uses with run_local_server).
// It starts a temporary HTTP server on a random port, opens the consent URL in
// the user's browser, and waits for Google to redirect back with the auth code.
func authenticate(ctx context.Context, secretsFile string) (*http.Client, error) {
	secretData, err := os.ReadFile(secretsFile)
	if err != nil {
		return nil, fmt.Errorf("reading client secrets: %w", err)
	}

	cfg, err := google.ConfigFromJSON(secretData, scopeSheets, scopeDrive)
	if err != nil {
		return nil, fmt.Errorf("parsing client secrets: %w", err)
	}

	// Listen on a random available port so we can tell Google where to redirect.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("starting local auth server: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	cfg.RedirectURL = fmt.Sprintf("http://127.0.0.1:%d", port)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no code in redirect: %s", r.URL.RawQuery)
			http.Error(w, "auth failed", http.StatusBadRequest)
			return
		}
		fmt.Fprintln(w, "Authentication successful — you can close this tab.")
		codeCh <- code
	})}

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	defer server.Shutdown(ctx) //nolint:errcheck

	authURL := cfg.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Opening browser for Google authentication…\n  %s\n\n", authURL)
	if err := browser.OpenURL(authURL); err != nil {
		// Non-fatal: user can paste the URL manually if the open fails.
		fmt.Fprintf(os.Stderr, "Could not open browser automatically: %v\nPlease open the URL above manually.\n", err)
	}

	var code string
	select {
	case code = <-codeCh:
	case err = <-errCh:
		return nil, fmt.Errorf("auth redirect error: %w", err)
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	token, err := cfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchanging auth code: %w", err)
	}

	return cfg.Client(ctx, token), nil
}

// populateTab writes row data and applies all formatting to the sheet
// identified by sheetID inside the given spreadsheet.
func populateTab(ctx context.Context, sheetsSvc *sheets.Service, spreadsheetID string, sheetID int64, d sheetData) error {
	maxCols := 1
	for _, row := range d.rows {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}

	vr := make([]*sheets.RowData, len(d.rows))
	for i, row := range d.rows {
		cells := make([]*sheets.CellData, maxCols)
		for j := range maxCols {
			cd := &sheets.CellData{}
			if j < len(row) {
				if s, ok := row[j].(string); ok {
					cd.UserEnteredValue = &sheets.ExtendedValue{StringValue: &s}
				}
			}
			cells[j] = cd
		}
		vr[i] = &sheets.RowData{Values: cells}
	}

	// Expand column count first — the default 26-column limit would reject
	// writes beyond column Z for longer verses. Then write all cell values.
	updateReq := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{UpdateSheetProperties: &sheets.UpdateSheetPropertiesRequest{
				Properties: &sheets.SheetProperties{
					SheetId: sheetID,
					GridProperties: &sheets.GridProperties{
						ColumnCount: int64(maxCols),
					},
				},
				Fields: "gridProperties.columnCount",
			}},
			{UpdateCells: &sheets.UpdateCellsRequest{
				Start:  &sheets.GridCoordinate{SheetId: sheetID},
				Rows:   vr,
				Fields: "userEnteredValue",
			}},
		},
	}
	if _, err := sheetsSvc.Spreadsheets.BatchUpdate(spreadsheetID, updateReq).Context(ctx).Do(); err != nil {
		return fmt.Errorf("writing cell values: %w", err)
	}
	fmt.Printf("Written %d rows × %d cols\n", len(d.rows), maxCols)

	// Rewrite all placeholder SheetId=0 values to the real id, then apply
	// formatting in a single batch.
	patchSheetID(&d, sheetID)
	var allReqs []*sheets.Request
	allReqs = append(allReqs, d.mergeReqs...)
	allReqs = append(allReqs, d.bgRequests...)
	allReqs = append(allReqs, d.boldRequests...)
	allReqs = append(allReqs, narrowColAReq(sheetID))
	allReqs = append(allReqs, fontSizeReq(sheetID, 12))

	fmtReq := &sheets.BatchUpdateSpreadsheetRequest{Requests: allReqs}
	if _, err := sheetsSvc.Spreadsheets.BatchUpdate(spreadsheetID, fmtReq).Context(ctx).Do(); err != nil {
		return fmt.Errorf("applying formatting: %w", err)
	}
	fmt.Println("Formatting applied.")
	return nil
}

// createSpreadsheet creates a new Google Sheet with a single content tab named
// tabName, populates it with d, and makes it accessible via link. The spreadsheet
// title is used as the document name in Google Drive.
func createSpreadsheet(ctx context.Context, client *http.Client, title, tabName string, d sheetData) (string, error) {
	sheetsSvc, err := sheets.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return "", fmt.Errorf("creating Sheets service: %w", err)
	}
	driveSvc, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return "", fmt.Errorf("creating Drive service: %w", err)
	}

	// Create the spreadsheet with the content tab already named correctly —
	// this avoids needing to rename or delete a default "Sheet1" afterwards.
	ss, err := sheetsSvc.Spreadsheets.Create(&sheets.Spreadsheet{
		Properties: &sheets.SpreadsheetProperties{Title: title},
		Sheets: []*sheets.Sheet{{
			Properties: &sheets.SheetProperties{Title: tabName},
		}},
	}).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("creating spreadsheet: %w", err)
	}
	if len(ss.Sheets) == 0 {
		return "", fmt.Errorf("spreadsheet created but no sheets returned")
	}
	sheetID := ss.Sheets[0].Properties.SheetId
	fmt.Printf("Created: https://docs.google.com/spreadsheets/d/%s\n", ss.SpreadsheetId)

	if err := populateTab(ctx, sheetsSvc, ss.SpreadsheetId, sheetID, d); err != nil {
		return "", err
	}

	// Make the sheet accessible via link.
	if _, err = driveSvc.Permissions.Create(ss.SpreadsheetId, &drive.Permission{
		Type: "anyone",
		Role: "writer",
	}).Context(ctx).Do(); err != nil {
		// Non-fatal — sheet is still usable by the owner.
		fmt.Fprintf(os.Stderr, "Warning: could not set sharing permissions: %v\n", err)
	}

	return fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s", ss.SpreadsheetId), nil
}

// addTabToSpreadsheet adds a new tab named tabName to an existing spreadsheet
// and populates it with d.
func addTabToSpreadsheet(ctx context.Context, client *http.Client, spreadsheetID, tabName string, d sheetData) (string, error) {
	sheetsSvc, err := sheets.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return "", fmt.Errorf("creating Sheets service: %w", err)
	}

	resp, err := sheetsSvc.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{{
			AddSheet: &sheets.AddSheetRequest{
				Properties: &sheets.SheetProperties{Title: tabName},
			},
		}},
	}).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("adding tab: %w", err)
	}
	if len(resp.Replies) == 0 || resp.Replies[0].AddSheet == nil {
		return "", fmt.Errorf("unexpected empty reply when adding tab '%s'", tabName)
	}
	newSheetID := resp.Replies[0].AddSheet.Properties.SheetId
	fmt.Printf("Added tab '%s' to https://docs.google.com/spreadsheets/d/%s\n", tabName, spreadsheetID)

	if err := populateTab(ctx, sheetsSvc, spreadsheetID, newSheetID, d); err != nil {
		return "", err
	}

	return fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s", spreadsheetID), nil
}

// ---------------------------------------------------------------------------
// CLI
// ---------------------------------------------------------------------------

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
	inputFile   := flags.String("input", "", "Path to the input text file of Greek verses (required)")
	title       := flags.String("title", "", "Title for the Google Sheet (defaults to the input filename)")
	secretsFile := flags.String("secrets", "client_secret.json", "Path to the Google OAuth2 client secrets JSON file")
	sheetID     := flags.String("sheet-id", "", "ID of an existing Google Spreadsheet to add a tab to (optional; omit to create a new sheet)")
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
