package auth

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

// setTempHome redirects HOME to a fresh temp directory and unsets XDG_CONFIG_HOME
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

func TestLoadCachedTokenCacheMiss(t *testing.T) {
	setTempHome(t)

	tok, err := loadCachedToken()
	require.Error(t, err)
	assert.Nil(t, tok)
}

func TestLoadCachedTokenCorruptJson(t *testing.T) {
	setTempHome(t)

	path, err := tokenCachePath()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path, []byte("not valid json"), 0600))

	tok, err := loadCachedToken()
	require.Error(t, err)
	assert.Nil(t, tok)
}

func TestTokenCacheRoundTrip(t *testing.T) {
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

func TestSaveCachedTokenCreatesParentDirs(t *testing.T) {
	setTempHome(t)

	// Confirm that saveCachedToken creates the greeksheet config dir automatically.
	tok := &oauth2.Token{AccessToken: "abc", RefreshToken: "xyz"}
	require.NoError(t, saveCachedToken(tok))

	path, err := tokenCachePath()
	require.NoError(t, err)
	assert.FileExists(t, path)
}

func TestOAuthConfigViaEnvVars(t *testing.T) {
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

func TestOAuthConfigViaUserConfigDir(t *testing.T) {
	setTempHome(t)
	// Ensure env vars don't shadow the file lookup.
	t.Setenv("GOOGLE_CLIENT_ID", "")
	t.Setenv("GOOGLE_CLIENT_SECRET", "")

	// Write a minimal client_secret.json into the user config dir.
	configDir, err := os.UserConfigDir()
	require.NoError(t, err)
	secretDir := filepath.Join(configDir, "greeksheet")
	require.NoError(t, os.MkdirAll(secretDir, 0700))

	secret := `{"installed":{"client_id":"cfg-dir-id","client_secret":"cfg-dir-secret",` +
		`"redirect_uris":["urn:ietf:wg:oauth:2.0:oob"],"auth_uri":"https://accounts.google.com/o/oauth2/auth",` +
		`"token_uri":"https://oauth2.googleapis.com/token"}}`
	require.NoError(t, os.WriteFile(filepath.Join(secretDir, "client_secret.json"), []byte(secret), 0600))

	cfg, err := oauthConfig()
	require.NoError(t, err)
	assert.Equal(t, "cfg-dir-id", cfg.ClientID)
	assert.Equal(t, "cfg-dir-secret", cfg.ClientSecret)
}

func TestOAuthConfigUserConfigDirTakesPriorityOverCwd(t *testing.T) {
	setTempHome(t)
	t.Setenv("GOOGLE_CLIENT_ID", "")
	t.Setenv("GOOGLE_CLIENT_SECRET", "")

	// Place a file in the user config dir.
	configDir, err := os.UserConfigDir()
	require.NoError(t, err)
	secretDir := filepath.Join(configDir, "greeksheet")
	require.NoError(t, os.MkdirAll(secretDir, 0700))
	configSecret := `{"installed":{"client_id":"config-dir-id","client_secret":"config-dir-secret",` +
		`"redirect_uris":["urn:ietf:wg:oauth:2.0:oob"],"auth_uri":"https://accounts.google.com/o/oauth2/auth",` +
		`"token_uri":"https://oauth2.googleapis.com/token"}}`
	require.NoError(t, os.WriteFile(filepath.Join(secretDir, "client_secret.json"), []byte(configSecret), 0600))

	// Also place a different file in a temp working directory.
	cwdSecret := `{"installed":{"client_id":"cwd-id","client_secret":"cwd-secret",` +
		`"redirect_uris":["urn:ietf:wg:oauth:2.0:oob"],"auth_uri":"https://accounts.google.com/o/oauth2/auth",` +
		`"token_uri":"https://oauth2.googleapis.com/token"}}`
	cwdFile := filepath.Join(t.TempDir(), "client_secret.json")
	require.NoError(t, os.WriteFile(cwdFile, []byte(cwdSecret), 0600))
	orig, _ := os.Getwd()
	require.NoError(t, os.Chdir(filepath.Dir(cwdFile)))
	t.Cleanup(func() { _ = os.Chdir(orig) })

	cfg, err := oauthConfig()
	require.NoError(t, err)
	// The config-dir file must win.
	assert.Equal(t, "config-dir-id", cfg.ClientID)
}

func TestOAuthConfigReturnsIndependentCopies(t *testing.T) {
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
