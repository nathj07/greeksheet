package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/pkg/browser"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

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
