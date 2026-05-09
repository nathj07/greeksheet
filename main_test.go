package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractSpreadsheetID(t *testing.T) {
	f := func(input, wantID string, wantErr bool) {
		t.Helper()
		got, err := extractSpreadsheetID(input)
		if wantErr {
			require.Error(t, err)
			return
		}
		require.NoError(t, err)
		assert.Equal(t, wantID, got)
	}

	t.Run("valid_url", func(t *testing.T) {
		f("https://docs.google.com/spreadsheets/d/abc123XYZ", "abc123XYZ", false)
	})
	t.Run("unexpected_format_returns_error", func(t *testing.T) {
		f("https://docs.google.com/spreadsheets/abc123", "", true)
	})
	t.Run("empty_string_returns_error", func(t *testing.T) {
		f("", "", true)
	})
}
