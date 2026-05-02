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

	go run create_greek_sheet.go practice.txt
	go run create_greek_sheet.go -title "My Practice Sheet" practice.txt
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

		// Two blank spacer rows keep the verse visually separated from practice rows
		d.rows = append(d.rows, []any{}, []any{})

		// I row — empty cells for interlinear word-by-word translation
		r = int64(len(d.rows))
		iRow := make([]any, 1+wordCount)
		iRow[0] = "I"
		d.rows = append(d.rows, iRow)
		d.bgRequests = append(d.bgRequests, bgReq(r, 0, r+1, lastWordCol+1, colOrange))

		// T row — empty cells for full translation practice
		r = int64(len(d.rows))
		tRow := make([]any, 1+wordCount)
		tRow[0] = "T"
		d.rows = append(d.rows, tRow)
		d.bgRequests = append(d.bgRequests, bgReq(r, 0, r+1, lastWordCol+1, colGreen))

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

func narrowColAReq() *sheets.Request {
	return &sheets.Request{UpdateDimensionProperties: &sheets.UpdateDimensionPropertiesRequest{
		Range: &sheets.DimensionRange{
			SheetId:    0,
			Dimension:  "COLUMNS",
			StartIndex: 0,
			EndIndex:   1,
		},
		Properties: &sheets.DimensionProperties{PixelSize: 40},
		Fields:     "pixelSize",
	}}
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

// upload creates a new Google Sheet, populates it with row data, and applies
// all formatting (backgrounds, merges, bold headings, column width).
func upload(ctx context.Context, client *http.Client, title string, d sheetData) (string, error) {
	sheetsSvc, err := sheets.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return "", fmt.Errorf("creating Sheets service: %w", err)
	}
	driveSvc, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return "", fmt.Errorf("creating Drive service: %w", err)
	}

	// Create the spreadsheet
	ss, err := sheetsSvc.Spreadsheets.Create(&sheets.Spreadsheet{
		Properties: &sheets.SpreadsheetProperties{Title: title},
	}).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("creating spreadsheet: %w", err)
	}
	fmt.Printf("Created: https://docs.google.com/spreadsheets/d/%s\n", ss.SpreadsheetId)

	// Determine the max column width so we can pad short rows
	maxCols := 1
	for _, row := range d.rows {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}

	// Build the value range, padding each row to maxCols
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

	updateReq := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			// Expand the sheet to fit all columns before writing — the default
			// 26 columns is too narrow for longer verses.
			{UpdateSheetProperties: &sheets.UpdateSheetPropertiesRequest{
				Properties: &sheets.SheetProperties{
					SheetId: 0,
					GridProperties: &sheets.GridProperties{
						ColumnCount: int64(maxCols),
					},
				},
				Fields: "gridProperties.columnCount",
			}},
			{UpdateCells: &sheets.UpdateCellsRequest{
				Start:  &sheets.GridCoordinate{SheetId: 0},
				Rows:   vr,
				Fields: "userEnteredValue",
			}},
		},
	}
	if _, err = sheetsSvc.Spreadsheets.BatchUpdate(ss.SpreadsheetId, updateReq).Context(ctx).Do(); err != nil {
		return "", fmt.Errorf("writing cell values: %w", err)
	}
	fmt.Printf("Written %d rows × %d cols\n", len(d.rows), maxCols)

	// Apply all formatting in a single batch
	var allReqs []*sheets.Request
	allReqs = append(allReqs, d.mergeReqs...)
	allReqs = append(allReqs, d.bgRequests...)
	allReqs = append(allReqs, d.boldRequests...)
	allReqs = append(allReqs, narrowColAReq())

	fmtReq := &sheets.BatchUpdateSpreadsheetRequest{Requests: allReqs}
	if _, err = sheetsSvc.Spreadsheets.BatchUpdate(ss.SpreadsheetId, fmtReq).Context(ctx).Do(); err != nil {
		return "", fmt.Errorf("applying formatting: %w", err)
	}
	fmt.Println("Formatting applied.")

	// Make the sheet accessible via link
	if _, err = driveSvc.Permissions.Create(ss.SpreadsheetId, &drive.Permission{
		Type: "anyone",
		Role: "writer",
	}).Context(ctx).Do(); err != nil {
		// Non-fatal — sheet is still usable by the owner
		fmt.Fprintf(os.Stderr, "Warning: could not set sharing permissions: %v\n", err)
	}

	return fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s", ss.SpreadsheetId), nil
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
	title       := flags.String("title", "", "Title for the Google Sheet (defaults to the input filename)")
	secretsFile := flags.String("secrets", "client_secret.json", "Path to the Google OAuth2 client secrets JSON file")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}

	if flags.NArg() == 0 {
		return fmt.Errorf("usage: %s [-title <name>] [-secrets <path>] <input-file>", args[0])
	}

	conf := config{
		InputFile:   flags.Arg(0),
		Title:       *title,
		SecretsFile: *secretsFile,
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

	url, err := upload(ctx, httpClient, a.conf.Title, d)
	if err != nil {
		return err
	}
	fmt.Printf("\nDone! Open your sheet at:\n  %s\n", url)
	return nil
}
