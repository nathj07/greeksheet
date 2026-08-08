# greeksheet

A tool that builds a formatted spreadsheet for Greek New Testament
translation practice. It supports two **output** types and two **input** modes.

## Output types

| Flag | What it produces | Google account needed? |
|------|-----------------|----------------------|
| `-output sheets` *(default)* | A Google Sheets spreadsheet — URL printed at the end | Yes (browser opens once) |
| `-output xlsx` | A local Excel `.xlsx` file — file path printed at the end | No |

Both output types produce **identical layout and formatting**: the same
colour-coded rows, merged cells, column widths, and 12 pt font.

## Input modes

- **File mode** (`-input`): read verses from a plain-text file copied from
  [greekbible.com](https://greekbible.com).
- **Fetch mode** (`-ref`): fetch Greek text directly from greekbible.com — no
  copy-pasting required. One HTTP request is made per chapter. Three ref formats
  are supported:
  - `"Book ch:v-v"` or `"Book ch:v-ch:v"` — fetches a verse range into a
    single tab.
  - `"Book ch-ch"` — fetches whole chapters and creates one tab per chapter
    automatically, useful for whole-book or multi-chapter study.
  - `"Book ch"` — fetches a single whole chapter; shorthand for the above when
    only one chapter is needed.

## Download

greeksheet comes in two flavours — pick the one that suits you:

### GUI app (no command line needed)

A desktop app with a point-and-click interface. Download from the
[latest UI release](https://github.com/nathj07/greeksheet/releases?q=ui%2Fv&expanded=true)
(tags prefixed `ui/v`):

| Platform | File | Instructions |
|----------|------|--------------|
| macOS (Apple Silicon / M-series) | `greeksheetui-darwin-arm64.zip` | Unzip, then double-click `Greek Sheet UI.app` |
| Windows | `greeksheetui-windows-amd64.exe` | Double-click to run |

> **macOS security note:** On first launch, macOS may block the app.
> Open *System Settings → Privacy & Security* and click **Open Anyway**.

### CLI tool

A command-line binary. Download from the
[latest CLI release](https://github.com/nathj07/greeksheet/releases?q=v&expanded=true)
(tags prefixed `v`):

| Platform | File |
|----------|------|
| macOS (Apple Silicon / M-series) | `greeksheet-darwin-arm64` |
| Windows | `greeksheet-windows-amd64.exe` |

**macOS** — after downloading, mark the binary as executable and run it:

```bash
chmod +x greeksheet-darwin-arm64
./greeksheet-darwin-arm64 [flags]
```

> **macOS security note:** the first time you run the binary, macOS may block it.
> If so, go to *System Settings → Privacy & Security* and click **Open Anyway**.

**Windows** — run from PowerShell or CMD:

```
greeksheet-windows-amd64.exe [flags]
```

### Build from source

If you prefer to compile yourself, you need Go 1.26 or later:

```bash
go build -o greeksheet .
./greeksheet [flags]
```

## What it produces

For each verse the spreadsheet contains eight rows:

| Row label | Colour | Purpose                                                                                         |
|-----------|--------|-------------------------------------------------------------------------------------------------|
| *(verse number + words)* | Grey | The Greek text, one word per cell                                                               |
| *(unlabelled × 2)* | *(plain)* | Per-word parsing and individual word-choice work — one cell per Greek word                      |
| **O** | Blue | Original — The original Greek text for the verse; merged across all word columns                |
| **I** | Orange | Intermediary — assemble your word choices into a coherent verse; merged across all word columns |
| **T** | Green | Final translation — write your polished English translation; merged across all word columns     |
| **C** | *(plain)* | Commentary notes, merged across all word columns                                                |
| **N** | *(plain)* | General notes, merged across all word columns                                                   |

Chapter boundaries appear as bold number rows that group the verses beneath
them. In file mode these come from `#` heading lines; in fetch mode they are
inserted automatically whenever the chapter changes.

All text is set at 12 pt for comfortable reading of Greek characters.

![Greeksheet screenshot](docs/images/screenshot.png)
Generated using `./greeksheet -sheet-id <my-sheet-id> -ref "John 9:1-23"`, details on the command, and the various flags it supports are provided below.

## Prerequisites

- A Google account *(only required for `-output sheets`)*
- Go 1.26 or later *(only required if building from source)*

## Authentication (Google Sheets output only)

The first time you run greeksheet with `-output sheets` (the default) a browser
tab opens asking you to sign in and approve the requested permissions. After you
click **Allow**, the tab shows "Authentication successful — you can close this
tab." and the tool continues automatically.

The token is cached at `~/.config/greeksheet/token.json` (macOS:
`~/Library/Application Support/greeksheet/token.json`). Every subsequent run
is silent — no browser, no extra steps.

> **Revoking access:** delete the token file above and the browser prompt will
> appear on the next run. You can also revoke access at any time from your
> [Google Account security page](https://myaccount.google.com/permissions).

Refresh tokens are typically valid for ~6 months of inactivity; if yours expires (or is revoked), the next run will ask you to re-authenticate.

When using `-output xlsx` no Google account is needed and authentication is
skipped entirely.

## Input modes

### File mode (`-input`)

Create a plain-text file with one or more sections. Lines beginning with `#`
become bold heading rows in the sheet. All other non-blank lines are treated
as a block of verses in the inline format used by greekbible.com — verse
number, followed by the words of that verse, followed by the next verse number,
and so on.

```
# 1 Corinthians 13
1 Ἐὰν ταῖς γλώσσαις τῶν ἀνθρώπων λαλῶ καὶ τῶν ἀγγέλων, ἀγάπην δὲ μὴ ἔχω, γέγονα χαλκὸς ἠχῶν ἢ κύμβαλον ἀλαλάζον. 2 καὶ ἐὰν ἔχω προφητείαν...
# 1 Corinthians 14
1 Διώκετε τὴν ἀγάπην, ζηλοῦτε δὲ τὰ πνευματικά...
```

You can paste text directly from greekbible.com — copy the verse range for a
chapter and save it as a `.txt` file. Long chapters pasted as a single line
are handled correctly.

### Fetch mode (`-ref`)

Pass a reference range and the tool fetches the Greek text directly from
greekbible.com. One HTTP request is made per chapter (not per verse), so a
multi-chapter range is still fast:

```bash
# Same-chapter range
go run . -ref "John 1:1-14" -title "John 1"

# Cross-chapter range — chapter heading inserted automatically
go run . -ref "John 3:36-4:5" -title "John 3–4"

# Multi-word book name
go run . -ref "1 Corinthians 13:1-13" -title "1 Cor 13"
```

The reference format is `"<Book> <chapter>:<verse>-<verse>"` for a same-chapter
range, or `"<Book> <chapter>:<verse>-<chapter>:<verse>"` for a cross-chapter
range. Verses outside the requested range are filtered out in-process after
each chapter page is fetched.

> **Cross-book ranges are not supported.** Both endpoints must belong to the
> same book. A reference like `"Ephesians 6:1-Philippians 2:6"` will be
> rejected — split it into two separate runs instead.

### Whole-chapter fetch (`-ref "Book ch-ch"` or `-ref "Book ch"`)

When the reference is in `"<Book> <startChapter>-<endChapter>"` format,
greeksheet automatically fetches entire chapters and creates one tab per
chapter, named by chapter number (`"1"`, `"2"`, …). No extra flag is
needed — the format is detected automatically.

For a single chapter, you can use the shorter `"<Book> <chapter>"` form instead
of writing `"ch-ch"`:

```bash
# Single chapter — short form
go run . -ref "Ephesians 1" -title "Ephesians 1"

# Single chapter — equivalent long form (still works)
go run . -ref "Romans 8-8" -title "Romans 8"

# Create a new spreadsheet with one tab per chapter
go run . -ref "Philippians 1-4" -title "Philippians"

# Add tabs to an existing spreadsheet
go run . -ref "Ephesians 1-6" -sheet-id <ID>
```

> **Cross-book ranges are not supported.** All chapters must belong to the
> same book. A reference like `"Ephesians 6-Philippians 2"` will be rejected —
> run each book separately instead.

## Usage

Run the downloaded binary directly:

```bash
# macOS
./greeksheet-darwin-arm64 [flags]

# Windows
greeksheet-windows-amd64.exe [flags]
```

If you built from source, use your own binary name:

```bash
./greeksheet [flags]
```

Or run without building:

```bash
go run . [flags]
```

Exactly one of `-input` or `-ref` must be supplied.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-input` | | Path to the input text file of Greek verses (**mutually exclusive with `-ref`**) |
| `-ref` | | Reference range to fetch from greekbible.com. Use `"Book ch:v-v"` or `"Book ch:v-ch:v"` for a verse range (single tab); use `"Book ch-ch"` for whole chapters (one tab per chapter, detected automatically); use `"Book ch"` for a single whole chapter (**mutually exclusive with `-input`**) |
| `-output` | `sheets` | Output type: `sheets` (Google Sheets) or `xlsx` (local Excel file, requires `-xlsx-file`) |
| `-title` | input filename (without extension), or the ref string | Title for a **new** spreadsheet (ignored when `-sheet-id` is used; ignored for `-output xlsx`) |
| **Google Sheets flags** | | *(only used with `-output sheets`)* |
| `-sheet-id` | *(omit to create a new sheet)* | ID of an **existing** Google Spreadsheet to add a new tab to (**mutually exclusive with `-folder-id`**) |
| `-folder-id` | *(omit to create in Drive root)* | Google Drive folder ID to create the **new** spreadsheet inside (**mutually exclusive with `-sheet-id`**) |
| `-verbose` | `false` | Log Sheets API retry attempts (429 Too Many Requests) to stderr |
| **xlsx flags** | | *(only used with `-output xlsx`)* |
| `-xlsx-file` | | Path to the `.xlsx` file to write — created if it does not exist, tabs appended if it does |

### Finding a spreadsheet ID

The spreadsheet ID is the long string of letters and numbers in the Google
Sheets URL, between `/d/` and the next `/`:

```
https://docs.google.com/spreadsheets/d/<ID STRING IS HERE>/edit
```

Copy everything between `/d/` and `/edit` (or the next `/`) and pass it as
`-sheet-id`.

### Finding a Drive folder ID

The folder ID appears in the Google Drive URL when you open a folder:

```
https://drive.google.com/drive/folders/<FOLDER ID IS HERE>
```

Copy the ID at the end of the URL and pass it as `-folder-id`. The new
spreadsheet will be created directly inside that folder — it will never
appear in your Drive root.

### Tab naming

- **File mode / verse-range fetch**: the tab is named from the verse range,
  e.g. `1:1-1:14` or `13:1-14:40`. The book name is omitted — it
  typically appears in the spreadsheet title instead.
- **Whole-chapter fetch**: each tab is named by chapter number only, e.g.
  `1`, `2`, `3`.
- **xlsx output**: Excel sheet names may not contain `:`, so verse-range tab
  names use `.` as a separator instead (e.g. `1.1-1.14`). Whole-chapter tabs
  are unchanged (`1`, `2`, `3`).

### Examples

```bash
# File mode — create a new Google spreadsheet; tab named from the verse range
go run . -input 1cor13.txt

# File mode — set a custom spreadsheet title
go run . -input 1cor13.txt -title "1 Corinthians study"

# File mode — add a new tab to an existing spreadsheet
go run . -input 1cor14.txt -sheet-id <ID FROM URL OF EXISTING SHEET>

# Fetch mode — create a new Google spreadsheet from a reference range
go run . -ref "John 1:1-14" -title "John 1"

# Fetch mode — cross-chapter range
go run . -ref "John 3:36-4:5" -title "John cross-chapter"

# Fetch mode — add a tab to an existing spreadsheet
go run . -ref "Romans 8:1-17" -title "Romans 8" -sheet-id <ID>

# Single chapter fetch
go run . -ref "Ephesians 1" -title "Ephesians 1"

# Whole-chapter fetch — create a new spreadsheet with one tab per chapter
go run . -ref "Philippians 1-4" -title "Philippians"

# Whole-chapter fetch — add tabs to an existing spreadsheet
go run . -ref "Ephesians 1-6" -sheet-id <ID>

# Create a new spreadsheet inside a specific Drive folder
go run . -ref "John 1:1-14" -title "John 1" -folder-id <FOLDER ID FROM DRIVE URL>

# Whole-chapter fetch — create a new spreadsheet in a specific Drive folder
go run . -ref "Ephesians 1-6" -title "Ephesians" -folder-id <FOLDER ID FROM DRIVE URL>

# xlsx output — create a new file (no Google account needed)
go run . -output xlsx -ref "John 1:1-14" -xlsx-file ~/sheets/john.xlsx

# xlsx output — whole-chapter fetch into a new file
go run . -output xlsx -ref "Ephesians 1-6" -xlsx-file ~/sheets/ephesians.xlsx

# xlsx output — append more tabs to an existing file
go run . -output xlsx -ref "Philippians 1-4" -xlsx-file ~/study.xlsx

# xlsx output — file mode
go run . -output xlsx -input practice.txt -xlsx-file ~/sheets/practice.xlsx
```

## Output

### Google Sheets (default)

First run (browser opens once):

```
Authenticating with Google…
Opening browser for Google authentication…
  https://accounts.google.com/o/oauth2/auth?…

Fetching Greek text for "John 1:1-14" from greekbible.com…
  Fetching John chapter 1…
Parsed 14 verses
…

Done! Open your sheet at:
  https://docs.google.com/spreadsheets/d/<sheet-id>
```

Subsequent runs (silent auth):

```
Authenticating with Google…
Fetching Greek text for "John 1:1-14" from greekbible.com…
  Fetching John chapter 1…
Parsed 14 verses
…

Done! Open your sheet at:
  https://docs.google.com/spreadsheets/d/<sheet-id>
```

The sheet is automatically shared with "anyone with the link can view", so you
can open it on any device without extra steps.

### xlsx output

No authentication step — output goes straight to disk:

```
Fetching Greek text for "John 1:1-14" from greekbible.com…
  Fetching John chapter 1…
Parsed 14 verses
…

Done! Open your sheet at:
  /home/user/sheets/John 1.xlsx
```

### Whole-chapter fetch (`-ref "Book ch-ch"` or `-ref "Book ch"`)

```
Authenticating with Google…
Fetching Ephesians chapters 1–6 from greekbible.com…
  Chapter 1: 23 verses
…
  Chapter 6: 24 verses

Done! Open your sheet at:
  https://docs.google.com/spreadsheets/d/<sheet-id>
```

## GUI App

`gui/greeksheetui` is a desktop front-end built with [Fyne](https://fyne.io/).
It exposes the same two output modes — Excel and Google Sheets — in a
point-and-click interface without requiring any command-line use.

Pre-built releases are published under `ui/v*` tags; see [Download](#download) above.
To build it locally you need a C compiler (Xcode CLT on macOS, MinGW on Windows)
in addition to Go 1.26+:

```bash
go build ./gui/greeksheetui
```

## Contributing

Contributions are welcome. Here's what you need to know before opening a PR.

### Prerequisites

- Go 1.26 or later
- A Google OAuth client secret for local testing of the `sheets` output — see [Build from source](#build-from-source) and [Authentication](#authentication-google-sheets-output-only)

### Running tests locally

```bash
go build ./...
go vet ./...
go test ./...
```

All three commands must pass cleanly. The CI workflow enforces the same checks on every PR.

### Submitting a change

1. **Open an issue first** — describe the bug or feature so it can be discussed before any code is written. This avoids wasted effort if the approach needs to change.
2. Fork the repo and create a branch from `main`
3. Make your changes with tests where appropriate
4. Open a pull request targeting `main` that:
   - References the issue (e.g. `Closes #123` in the description)
   - Includes a clear description of *what* changed and *why*
   - Notes any non-obvious decisions or trade-offs

CI will run automatically. A review from the repo owner is required before merging — you will be auto-assigned as a reviewer via CODEOWNERS.

## License

MIT — see [LICENSE](LICENSE).
