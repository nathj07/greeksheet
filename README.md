# greeksheet

A command-line tool that turns a plain-text file of Greek New Testament verses
into a formatted Google Sheet for translation practice. Each verse gets a row
of Greek words followed by two per-word rows for parsing and word-choice work,
then merged rows for assembling an intermediary reading, writing the final
translation, and adding commentary and general notes.

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

Section headings (lines beginning with `#` in the input file) appear as bold
label rows that group the verses beneath them.

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

## Input file format

Create a plain-text file with one or more sections. Lines beginning with `#`
become bold heading rows in the sheet. All other non-blank lines are treated
as a block of verses in the inline format used by [greekbible.com](https://greekbible.com)
— verse number, followed by the words of that verse, followed by the next
verse number, and so on.

```
# 1 Corinthians 13
1 Ἐὰν ταῖς γλώσσαις τῶν ἀνθρώπων λαλῶ καὶ τῶν ἀγγέλων, ἀγάπην δὲ μὴ ἔχω, γέγονα χαλκὸς ἠχῶν ἢ κύμβαλον ἀλαλάζον. 2 καὶ ἐὰν ἔχω προφητείαν...
# 1 Corinthians 14
1 Διώκετε τὴν ἀγάπην, ζηλοῦτε δὲ τὰ πνευματικά...
```

You can paste text directly from greekbible.com — copy the verse range for a
chapter and save it as a `.txt` file. Long chapters pasted as a single line
are handled correctly.

## Usage

```
go run create_greek_sheet.go [flags] <input-file>
```

Or build first and then run the binary:

```
go build -o greeksheet .
./greeksheet [flags] <input-file>
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-title` | input filename (without extension) | Title of the new Google Sheet |
| `-secrets` | `client_secret.json` | Path to your OAuth 2.0 client secrets file |

### Examples

```bash
# Use all defaults — sheet title matches the filename
go run create_greek_sheet.go practice.txt

# Set a custom title
go run create_greek_sheet.go -title "1 Corinthians — Week 3" practice.txt

# Secrets file lives somewhere else
go run create_greek_sheet.go -secrets ~/keys/my_secret.json practice.txt
```

## Output

The tool prints progress as it runs:

```
Parsed 13 verses from 'practice.txt'
Authenticating with Google…
Opening browser for Google authentication…
Created: https://docs.google.com/spreadsheets/d/<sheet-id>
Written 91 rows × 32 cols
Formatting applied.

Done! Open your sheet at:
  https://docs.google.com/spreadsheets/d/<sheet-id>
```

The sheet is automatically shared with "anyone with the link can edit", so you
can open it on any device without extra steps.

## License

MIT — see [LICENSE](LICENSE).
