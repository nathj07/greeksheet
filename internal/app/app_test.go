package app

import (
	"context"
	"errors"
	"testing"

	"github.com/nathj07/greeksheet/internal/document"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

type mockSource struct {
	mock.Mock
}

func (m *mockSource) Load(ctx context.Context) (document.Document, error) {
	args := m.Called(ctx)
	// Comma-ok guards against a nil return value producing a panic.
	result, _ := args.Get(0).(document.Document)
	return result, args.Error(1)
}

type mockTarget struct {
	mock.Mock
}

func (m *mockTarget) Render(ctx context.Context, d document.Document) (string, error) {
	args := m.Called(ctx, d)
	return args.String(0), args.Error(1)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// oneVerseDoc builds the smallest valid Document for use in tests that need a non-empty source.
func oneVerseDoc(title string) document.Document {
	return document.Document{
		Title: title,
		Tabs: []document.Tab{{
			Sections: []document.Section{{
				Verses: []document.Verse{{Num: "1", Words: []string{"ἐν"}}},
			}},
		}},
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestRunSourceLoadErrorPropagates(t *testing.T) {
	ctx := context.Background()
	src := &mockSource{}
	src.On("Load", ctx).Return(document.Document{}, errors.New("network failure"))

	tgt := &mockTarget{}

	_, err := App{Source: src, Target: tgt}.Run(ctx)

	require.EqualError(t, err, "network failure")
	src.AssertExpectations(t)
	tgt.AssertNotCalled(t, "Render")
}

func TestRunEmptyDocumentReturnsError(t *testing.T) {
	ctx := context.Background()
	src := &mockSource{}
	src.On("Load", ctx).Return(document.Document{Title: "Empty"}, nil)

	tgt := &mockTarget{}

	_, err := App{Source: src, Target: tgt}.Run(ctx)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no verses found")
	src.AssertExpectations(t)
	tgt.AssertNotCalled(t, "Render")
}

func TestRunRenderErrorPropagates(t *testing.T) {
	ctx := context.Background()
	src := &mockSource{}
	src.On("Load", ctx).Return(oneVerseDoc("John 1"), nil)

	tgt := &mockTarget{}
	tgt.On("Render", ctx, mock.Anything).Return("", errors.New("sheets API down"))

	_, err := App{Source: src, Target: tgt}.Run(ctx)

	require.EqualError(t, err, "sheets API down")
	src.AssertExpectations(t)
	tgt.AssertExpectations(t)
}

func TestRunHappyPathReturnsURL(t *testing.T) {
	ctx := context.Background()
	src := &mockSource{}
	src.On("Load", ctx).Return(oneVerseDoc("John 1"), nil)

	tgt := &mockTarget{}
	tgt.On("Render", ctx, mock.Anything).Return("https://docs.google.com/spreadsheets/d/abc", nil)

	url, err := App{Source: src, Target: tgt}.Run(ctx)

	require.NoError(t, err)
	assert.Equal(t, "https://docs.google.com/spreadsheets/d/abc", url)
	src.AssertExpectations(t)
	tgt.AssertExpectations(t)
}

func TestRunTitleOverrideReplacesDocumentTitle(t *testing.T) {
	ctx := context.Background()
	src := &mockSource{}
	src.On("Load", ctx).Return(oneVerseDoc("Original Title"), nil)

	// Capture the Document passed to Render so we can inspect its Title.
	var rendered document.Document
	tgt := &mockTarget{}
	tgt.On("Render", ctx, mock.MatchedBy(func(d document.Document) bool {
		rendered = d
		return true
	})).Return("https://docs.google.com/spreadsheets/d/xyz", nil)

	_, err := App{Source: src, Target: tgt, TitleOverride: "Custom Title"}.Run(ctx)

	require.NoError(t, err)
	assert.Equal(t, "Custom Title", rendered.Title)
	src.AssertExpectations(t)
	tgt.AssertExpectations(t)
}

func TestRunEmptyTitleOverridePreservesSourceTitle(t *testing.T) {
	ctx := context.Background()
	src := &mockSource{}
	src.On("Load", ctx).Return(oneVerseDoc("Source Title"), nil)

	var rendered document.Document
	tgt := &mockTarget{}
	tgt.On("Render", ctx, mock.MatchedBy(func(d document.Document) bool {
		rendered = d
		return true
	})).Return("https://docs.google.com/spreadsheets/d/xyz", nil)

	_, err := App{Source: src, Target: tgt, TitleOverride: ""}.Run(ctx)

	require.NoError(t, err)
	assert.Equal(t, "Source Title", rendered.Title)
	src.AssertExpectations(t)
	tgt.AssertExpectations(t)
}
