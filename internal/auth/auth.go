// Package auth handles Google OAuth2 authentication for the Sheets and Drive
// APIs: resolving client credentials, running the browser consent flow, and
// caching the resulting token between runs.
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/browser"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	scopeSheets = "https://www.googleapis.com/auth/spreadsheets"
	scopeDrive  = "https://www.googleapis.com/auth/drive"
)

// embeddedClientID and embeddedClientSecret are set at build time via
//
//	-ldflags "-X main.embeddedClientID=<id> -X main.embeddedClientSecret=<secret>"
//
// They are empty in the source tree so that the credentials are never committed.
// For desktop OAuth2 apps the secret is not truly secret by design —
// see https://developers.google.com/identity/protocols/oauth2/native-app
var (
	embeddedClientID     string
	embeddedClientSecret string
)

// oauthConfig resolves OAuth credentials and returns a fresh *oauth2.Config.
// It tries four sources in priority order:
//  1. Variables injected at build time via -ldflags (used in release binaries).
//  2. GOOGLE_CLIENT_ID / GOOGLE_CLIENT_SECRET environment variables (CI / testing).
//  3. client_secret.json in the platform user-config directory
//     (e.g. ~/Library/Application Support/greeksheet/ on macOS). This is the
//     recommended location for development use with the GUI, since the app
//     may not be launched from the repo root.
//  4. client_secret.json in the current working directory (legacy fallback for
//     the CLI, which is always run from the repo root).
//
// A fresh struct is returned on each call so that doBrowserFlow can safely
// set RedirectURL without affecting other callers.
func oauthConfig() (*oauth2.Config, error) {
	id, secret := embeddedClientID, embeddedClientSecret

	if id == "" {
		id = os.Getenv("GOOGLE_CLIENT_ID")
	}
	if secret == "" {
		secret = os.Getenv("GOOGLE_CLIENT_SECRET")
	}

	if id != "" && secret != "" {
		return &oauth2.Config{
			ClientID:     id,
			ClientSecret: secret,
			Scopes:       []string{scopeSheets, scopeDrive},
			Endpoint:     google.Endpoint,
		}, nil
	}

	// Try the user config directory first — this works regardless of the
	// working directory, so both the CLI and the GUI find the file reliably.
	if configDir, err := os.UserConfigDir(); err == nil {
		configPath := filepath.Join(configDir, "greeksheet", "client_secret.json")
		if data, err := os.ReadFile(configPath); err == nil {
			return google.ConfigFromJSON(data, scopeSheets, scopeDrive)
		}
	}

	// Fall back to the working directory for developers running the CLI from
	// the repo root where the file is checked in (but not committed to git).
	if data, err := os.ReadFile("client_secret.json"); err == nil {
		return google.ConfigFromJSON(data, scopeSheets, scopeDrive)
	}

	return nil, fmt.Errorf(
		"OAuth credentials not found: download a release binary (credentials are built in), "+
			"set GOOGLE_CLIENT_ID/GOOGLE_CLIENT_SECRET environment variables, "+
			"or place client_secret.json in %s or the working directory",
		placeholderConfigPath())
}

// placeholderConfigPath returns a human-readable example config path for use
// in error messages. It returns a static string if UserConfigDir fails.
func placeholderConfigPath() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "greeksheet", "client_secret.json")
	}
	return "$XDG_CONFIG_HOME/greeksheet/client_secret.json"
}

// tokenCachePath returns the path where the OAuth2 token is cached between runs.
// The file lives at $XDG_CONFIG_HOME/greeksheet/token.json (Unix) or
// ~/Library/Application Support/greeksheet/token.json (macOS) or
// %AppData%\greeksheet\token.json (Windows).
func tokenCachePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "greeksheet", "token.json"), nil
}

// loadCachedToken reads a previously saved OAuth2 token from disk.
// On any failure (cache miss, unreadable file, or corrupt JSON) it returns a nil
// token and a non-nil error. Callers that want best-effort caching can ignore
// the error and fall back to a browser flow.
func loadCachedToken() (*oauth2.Token, error) {
	path, err := tokenCachePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var t oauth2.Token
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// saveCachedToken persists the OAuth2 token to disk with owner-only permissions.
func saveCachedToken(t *oauth2.Token) error {
	path, err := tokenCachePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600) // owner read/write only
}

// doBrowserFlow opens the Google consent page in the user's default browser,
// starts a temporary local server to receive the OAuth2 redirect, and exchanges
// the resulting code for a token. It times out after 5 minutes.
// cfg.RedirectURL is set to the local server's address before the browser opens.
func doBrowserFlow(ctx context.Context, cfg *oauth2.Config) (*oauth2.Token, error) {
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

	select {
	case code := <-codeCh:
		return cfg.Exchange(ctx, code)
	case err := <-errCh:
		return nil, fmt.Errorf("auth redirect error: %w", err)
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(5 * time.Minute):
		return nil, fmt.Errorf("auth timeout: no response from browser within 5 minutes")
	}
}

// Authenticate returns an HTTP client authorised for Google Sheets and Drive.
// On the first run it opens the browser for OAuth2 consent and caches the token
// at the platform config directory (e.g. ~/.config/greeksheet/token.json).
// Subsequent runs are silent: the cached refresh token avoids the browser entirely.
func Authenticate(ctx context.Context) (*http.Client, error) {
	cfg, err := oauthConfig()
	if err != nil {
		return nil, err
	}

	// Use the cached token when available. We accept an expired access token as
	// long as a refresh token is present — the oauth2 library will refresh
	// silently, keeping reruns browser-free.
	tok, _ := loadCachedToken()
	if tok != nil && (tok.Valid() || tok.RefreshToken != "") {
		return cfg.Client(ctx, tok), nil
	}

	// No usable cached token; open the browser once to get consent.
	tok, err = doBrowserFlow(ctx, cfg)
	if err != nil {
		return nil, err
	}

	if err := saveCachedToken(tok); err != nil {
		return nil, fmt.Errorf("saving token cache: %w", err)
	}

	return cfg.Client(ctx, tok), nil
}
