package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

// setTempHome redirects both HOME and XDG_CONFIG_HOME to a fresh temp directory
// so that os.UserConfigDir() always resolves into the test sandbox.
func setTempHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", "") // ensure Linux doesn't use a pre-existing XDG dir
}

func TestTokenCachePath(t *testing.T) {
	setTempHome(t)

	path, err := tokenCachePath()
	require.NoError(t, err)
	assert.Equal(t, "token.json", filepath.Base(path))
	assert.Equal(t, "greeksheet", filepath.Base(filepath.Dir(path)))
}

func TestLoadCachedToken_cache_miss(t *testing.T) {
	setTempHome(t)

	tok, err := loadCachedToken()
	require.Error(t, err)
	assert.Nil(t, tok)
}

func TestLoadCachedToken_corrupt_json(t *testing.T) {
	setTempHome(t)

	path, err := tokenCachePath()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path, []byte("not valid json"), 0600))

	tok, err := loadCachedToken()
	require.Error(t, err)
	assert.Nil(t, tok)
}

func TestTokenCache_round_trip(t *testing.T) {
	setTempHome(t)

	original := &oauth2.Token{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		// Truncate to seconds so JSON marshal/unmarshal is lossless.
		Expiry: time.Now().Add(time.Hour).Truncate(time.Second),
	}

	require.NoError(t, saveCachedToken(original))

	loaded, err := loadCachedToken()
	require.NoError(t, err)
	assert.Equal(t, original.AccessToken, loaded.AccessToken)
	assert.Equal(t, original.RefreshToken, loaded.RefreshToken)
	assert.Equal(t, original.Expiry.Unix(), loaded.Expiry.Unix())
}

func TestSaveCachedToken_creates_parent_dirs(t *testing.T) {
	setTempHome(t)

	// Confirm that saveCachedToken creates the greeksheet config dir automatically.
	tok := &oauth2.Token{AccessToken: "abc", RefreshToken: "xyz"}
	require.NoError(t, saveCachedToken(tok))

	path, err := tokenCachePath()
	require.NoError(t, err)
	assert.FileExists(t, path)
}

func TestOAuthConfig_via_env_vars(t *testing.T) {
	t.Setenv("GOOGLE_CLIENT_ID", "test-client-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test-client-secret")

	cfg, err := oauthConfig()
	require.NoError(t, err)
	assert.Equal(t, "test-client-id", cfg.ClientID)
	assert.Equal(t, "test-client-secret", cfg.ClientSecret)
	assert.Contains(t, cfg.Scopes, scopeSheets)
	assert.Contains(t, cfg.Scopes, scopeDrive)
	assert.NotEmpty(t, cfg.Endpoint.AuthURL)
	assert.NotEmpty(t, cfg.Endpoint.TokenURL)
}

func TestOAuthConfig_returns_independent_copies(t *testing.T) {
	// Each call must return a fresh struct so that doBrowserFlow's mutation
	// of RedirectURL doesn't bleed between calls.
	t.Setenv("GOOGLE_CLIENT_ID", "test-client-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test-client-secret")

	a, err := oauthConfig()
	require.NoError(t, err)
	b, err := oauthConfig()
	require.NoError(t, err)

	a.RedirectURL = "http://127.0.0.1:9999"
	assert.Empty(t, b.RedirectURL)
}
