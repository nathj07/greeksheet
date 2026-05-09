# greeksheet

A command-line tool that builds a formatted Google Sheet for Greek New Testament
translation practice. It supports two input modes and three fetch strategies:

- **File mode** (`-input`): read verses from a plain-text file copied from
  [greekbible.com](https://greekbible.com).
- **Fetch mode** (`-ref`): fetch Greek text directly from greekbible.com for a
  reference range such as `"John 1:1-14"` or `"John 1:50-2:10"` — no copy-pasting
  required. One HTTP request is made per chapter, making it much faster than
  fetching verse-by-verse.
- **Chapter-per-tab mode** (`-ref … -chapter-per-tab`): fetch entire chapters and
  create one Google Sheets tab per chapter, useful for whole-book or multi-chapter
  study ranges like `"Ephesians 1-6"`.

## What it produces

For each verse the sheet contains six rows:

| Row label | Colour | Purpose |
|-----------|--------|---------|
| *(verse number + words)* | Grey | The Greek text, one word per cell |
| *(unlabelled × 2)* | *(plain)* | Per-word parsing and individual word-choice work — one cell per Greek word |
| **I** | Orange | Intermediary — assemble your word choices into a coherent verse; merged across all word columns |
| **T** | Green | Final translation — write your polished English translation; merged across all word columns |
| **C** | *(plain)* | Commentary notes, merged across all word columns |
| **N** | *(plain)* | General notes, merged across all word columns |

Chapter boundaries appear as bold number rows that group the verses beneath
them. In file mode these come from `#` heading lines; in fetch mode they are
inserted automatically whenever the chapter changes.

All text is set at 12 pt for comfortable reading of Greek characters.

## Prerequisites

- Go 1.21 or later
- A Google account
- A Google Cloud OAuth 2.0 **client secrets** file (see below)

## Getting a client secrets file

The tool needs permission to create Google Sheets and set their sharing on
your behalf. You grant this once by creating an OAuth 2.0 app in Google Cloud:

1. Go to the [Google Cloud Console](https://console.cloud.google.com/) and
   create a new project (or select an existing one).
2. Enable the **Google Sheets API** and the **Google Drive API** for that
   project (*APIs & Services → Library*).
3. Go to *APIs & Services → Credentials* and click **Create Credentials →
   OAuth client ID**.
4. Choose **Desktop app** as the application type, give it any name, and click
   **Create**.
5. Download the JSON file that appears — this is your `client_secret.json`.
6. Place the file in the same directory as the tool, or pass its path with the
   `-secrets` flag (see below).

> **Note:** the first time you run the tool, a browser tab will open asking
> you to sign in and approve the requested permissions. After you approve,
> the tab shows "Authentication successful — you can close this tab." and the
> tool continues automatically.

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
each chapter page is fetched. Cross-book ranges are not supported.

### Chapter-per-tab mode (`-ref … -chapter-per-tab`)

For multi-chapter study, use a whole-chapter range and the `-chapter-per-tab`
flag. Each chapter is fetched as a single page and written to its own
spreadsheet tab named by chapter number (`"1"`, `"2"`, …):

```bash
# Create a new spreadsheet with one tab per chapter
go run . -ref "Ephesians 1-6" -chapter-per-tab -title "Ephesians"

# Add tabs to an existing spreadsheet
go run . -ref "Ephesians 1-6" -chapter-per-tab -sheet-id <ID>

# Single chapter
go run . -ref "Romans 8-8" -chapter-per-tab -title "Romans 8"
```

The whole-chapter reference format is `"<Book> <startChapter>-<endChapter>"`.
This format is **only** valid with `-chapter-per-tab`; for verse-range fetches
use the `ch:v-v` format above.

## Usage

```
go run . [flags]
```

Or build first and then run the binary:

```
go build -o greeksheet .
./greeksheet [flags]
```

Exactly one of `-input` or `-ref` must be supplied.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-input` | | Path to the input text file of Greek verses (**mutually exclusive with `-ref`**) |
| `-ref` | | Reference range to fetch from greekbible.com. Use `"Book ch:v-v"` or `"Book ch:v-ch:v"` for verse ranges; use `"Book ch-ch"` with `-chapter-per-tab` for whole chapters (**mutually exclusive with `-input`**) |
| `-chapter-per-tab` | `false` | Create one tab per chapter; requires `-ref` with a whole-chapter range like `"Ephesians 1-6"` |
| `-title` | input filename (without extension), or the ref string | Title for a **new** Google Sheet (ignored when `-sheet-id` is used) |
| `-sheet-id` | *(omit to create a new sheet)* | ID of an **existing** Google Spreadsheet to add a new tab to (**mutually exclusive with `-folder-id`**) |
| `-folder-id` | *(omit to create in Drive root)* | Google Drive folder ID to create the **new** spreadsheet inside (**mutually exclusive with `-sheet-id`**) |
| `-secrets` | `client_secret.json` | Path to your OAuth 2.0 client secrets file |

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
  e.g. `1:1 - 1:14` or `13:1 - 14:40`. The book name is omitted — it
  typically appears in the spreadsheet title instead.
- **Chapter-per-tab mode**: each tab is named by chapter number only, e.g.
  `1`, `2`, `3`.

### Examples

```bash
# File mode — create a new spreadsheet; tab named from the verse range
go run . -input 1cor13.txt

# File mode — set a custom spreadsheet title
go run . -input 1cor13.txt -title "1 Corinthians study"

# File mode — add a new tab to an existing spreadsheet
go run . -input 1cor14.txt -sheet-id <ID FROM URL OF EXISTING SHEET>

# Fetch mode — create a new spreadsheet from a reference range
go run . -ref "John 1:1-14" -title "John 1"

# Fetch mode — cross-chapter range
go run . -ref "John 3:36-4:5" -title "John cross-chapter"

# Fetch mode — add a tab to an existing spreadsheet
go run . -ref "Romans 8:1-17" -title "Romans 8" -sheet-id <ID>

# Chapter-per-tab — create a new spreadsheet with 6 tabs (one per chapter)
go run . -ref "Ephesians 1-6" -chapter-per-tab -title "Ephesians"

# Chapter-per-tab — add tabs to an existing spreadsheet
go run . -ref "Ephesians 1-6" -chapter-per-tab -sheet-id <ID>

# Create a new spreadsheet inside a specific Drive folder
go run . -ref "John 1:1-14" -title "John 1" -folder-id <FOLDER ID FROM DRIVE URL>

# Chapter-per-tab — create a new spreadsheet in a specific Drive folder
go run . -ref "Ephesians 1-6" -chapter-per-tab -title "Ephesians" -folder-id <FOLDER ID FROM DRIVE URL>

# Secrets file lives somewhere else
go run . -input 1cor13.txt -secrets ~/keys/my_secret.json
```

## Output

### Verse-range fetch (`-ref`)

```
Fetching Greek text for "John 1:1-14" from greekbible.com…
  Fetching John chapter 1…
Parsed 14 verses
Authenticating with Google…
Opening browser for Google authentication…
Created: https://docs.google.com/spreadsheets/d/<sheet-id>
Written 98 rows × 19 cols
Formatting applied.

Done! Open your sheet at:
  https://docs.google.com/spreadsheets/d/<sheet-id>
```

### Chapter-per-tab (`-ref … -chapter-per-tab`)

```
Fetching Ephesians chapters 1–6 from greekbible.com…
  Fetching Ephesians chapter 1…
  Chapter 1: 23 verses
…
  Fetching Ephesians chapter 6…
  Chapter 6: 24 verses
Authenticating with Google…
…

Done! Open your sheet at:
  https://docs.google.com/spreadsheets/d/<sheet-id>
```

The sheet is automatically shared with "anyone with the link can edit", so you
can open it on any device without extra steps.

## License

MIT — see [LICENSE](LICENSE).
