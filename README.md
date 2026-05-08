# greeksheet

A command-line tool that builds a formatted Google Sheet for Greek New Testament
translation practice. It supports two input modes:

- **File mode** (`-input`): read verses from a plain-text file copied from
  [greekbible.com](https://greekbible.com).
- **Fetch mode** (`-ref`): fetch Greek text directly from greekbible.com for a
  reference range such as `"John 1:1-14"` or `"John 1:50-2:10"` — no copy-pasting
  required.

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
greekbible.com, one verse at a time:

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
range. Cross-book ranges are not supported.

A small delay is added between requests to avoid hammering the site.

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
| `-ref` | | Reference range to fetch from greekbible.com, e.g. `"John 1:1-14"` (**mutually exclusive with `-input`**) |
| `-title` | input filename (without extension), or the ref string | Title for a **new** Google Sheet (ignored when `-sheet-id` is used) |
| `-sheet-id` | *(omit to create a new sheet)* | ID of an **existing** Google Spreadsheet to add a new tab to |
| `-secrets` | `client_secret.json` | Path to your OAuth 2.0 client secrets file |

### Finding a spreadsheet ID

The spreadsheet ID is the long string of letters and numbers in the Google
Sheets URL, between `/d/` and the next `/`:

```
https://docs.google.com/spreadsheets/d/<ID STRING IS HERE>/edit
```

Copy everything between `/d/` and `/edit` (or the next `/`) and pass it as
`-sheet-id`.

### Tab naming

The tab is named automatically from the verse range. In file mode the range is
derived from the parsed verses (e.g. `13:1 - 14:40`). In fetch mode it comes
directly from the `-ref` flag (e.g. `1:1 - 1:14`). The book name is omitted
from the tab label — it typically appears in the spreadsheet title instead.

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

# Secrets file lives somewhere else
go run . -input 1cor13.txt -secrets ~/keys/my_secret.json
```

## Output

The tool prints progress as it runs:

```
Fetching Greek text for "John 1:1-14" from greekbible.com…
Parsed 14 verses
Authenticating with Google…
Opening browser for Google authentication…
Created: https://docs.google.com/spreadsheets/d/<sheet-id>
Written 98 rows × 19 cols
Formatting applied.

Done! Open your sheet at:
  https://docs.google.com/spreadsheets/d/<sheet-id>
```

When adding a tab to an existing sheet the output shows
`Added tab '1:1 - 1:14' to …` instead of `Created: …`.

The sheet is automatically shared with "anyone with the link can edit", so you
can open it on any device without extra steps.

## License

MIT — see [LICENSE](LICENSE).
