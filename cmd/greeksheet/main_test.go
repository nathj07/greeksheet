package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// validateXlsxPath
// ---------------------------------------------------------------------------

func TestValidateXlsxPath(t *testing.T) {
	f := func(path string, wantErr string) {
		t.Helper()
		err := validateXlsxPath(path)
		if wantErr == "" {
			require.NoError(t, err)
			return
		}
		require.Error(t, err)
		assert.ErrorContains(t, err, wantErr)
	}

	t.Run("valid_path_not_yet_existing", func(t *testing.T) {
		f(filepath.Join(t.TempDir(), "out.xlsx"), "")
	})
	t.Run("valid_path_existing_file", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "existing.xlsx")
		require.NoError(t, os.WriteFile(p, []byte{}, 0o600))
		f(p, "")
	})
	t.Run("directory_path_rejected", func(t *testing.T) {
		f(t.TempDir()+string(filepath.Separator), "is a directory")
	})
	t.Run("missing_xlsx_extension_rejected", func(t *testing.T) {
		f(filepath.Join(t.TempDir(), "out"), "must have a .xlsx extension")
	})
	t.Run("wrong_extension_rejected", func(t *testing.T) {
		f(filepath.Join(t.TempDir(), "out.csv"), "must have a .xlsx extension")
	})
	t.Run("extension_check_is_case_insensitive", func(t *testing.T) {
		f(filepath.Join(t.TempDir(), "out.XLSX"), "")
	})
}

// ---------------------------------------------------------------------------
// run() — xlsx path validation fires before fetching
// ---------------------------------------------------------------------------

func TestRun_xlsxDirectoryPathError(t *testing.T) {
	err := run([]string{"greeksheet",
		"-output", "xlsx",
		"-ref", "John 1:1-14",
		"-xlsx-file", t.TempDir() + string(filepath.Separator),
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "is a directory")
}

func TestRun_xlsxMissingExtensionError(t *testing.T) {
	err := run([]string{"greeksheet",
		"-output", "xlsx",
		"-ref", "John 1:1-14",
		"-xlsx-file", filepath.Join(t.TempDir(), "out"),
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "must have a .xlsx extension")
}
