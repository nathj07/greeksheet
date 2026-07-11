// Package output defines the contract for rendering a document.Document to a
// concrete destination: a Google Sheet today, and potentially other formats
// (CSV, PDF, a web page) in future.
package output

import (
	"context"

	"github.com/nathj07/greeksheet/internal/document"
)

// Target renders a Document to some destination and returns a location for the
// result — for Google Sheets this is the spreadsheet URL.
//
// Implementations own the details of how a multi-tab Document maps onto the
// destination (e.g. whether to create a new spreadsheet or append tabs to an
// existing one).
type Target interface {
	Render(ctx context.Context, d document.Document) (string, error)
}
