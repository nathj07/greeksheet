# Google Sheets Authentication

greeksheet uses OAuth 2.0 to write to Google Sheets on your behalf. This page
explains what happens, where the token is stored, and how to revoke access.

## First use

The first time you choose Google Sheets output — whether through the GUI or the
CLI — a browser tab opens asking you to sign in to your Google account and
approve the requested permissions. After you click **Allow**, the tab shows:

> Authentication successful — you can close this tab.

greeksheet then continues automatically. You will not be asked again on the
same machine.

## Token storage

After you approve access, greeksheet stores a token file on disk. All
subsequent runs read from this file silently — no browser, no extra steps.

| Platform | Token location |
|----------|---------------|
| macOS | `~/Library/Application Support/greeksheet/token.json` |
| Windows | `%APPDATA%\greeksheet\token.json` |
| Linux | `~/.config/greeksheet/token.json` |

The file is readable only by your user account.

## Token expiry

Google issues a refresh token alongside the access token. Access tokens expire
after one hour, but greeksheet renews them automatically in the background
without opening a browser. Refresh tokens themselves are valid for approximately
six months of inactivity. If yours expires, the next run will open the browser
once more.

## Revoking access

To sign out or switch Google accounts, delete the token file listed above. The
next run will open the browser again.

You can also revoke access at any time from your
[Google Account security page](https://myaccount.google.com/permissions) —
look for *greeksheet* in the list of connected apps and remove it.

## Permissions requested

greeksheet requests the minimum scopes needed:

- **`spreadsheets`** — create spreadsheets and add tabs to existing ones.
- **`drive.file`** — place new spreadsheets inside a specific Google Drive
  folder when `-folder-id` is used. This scope only grants access to files
  greeksheet itself creates; it cannot read, list, or modify any of your
  existing Drive files.

## Excel output

When using Excel (`.xlsx`) output no Google account is needed and the
authentication step is skipped entirely.
