// Package source defines the contract for anything that can produce a
// translation-practice document: a parsed text file, a live fetch from
// greekbible.com, or a future verse provider.
package source

import (
	"context"

	"github.com/nathj07/greeksheet/internal/document"
)

// Source loads Greek NT content and returns it as a fully-built Document.
//
// Implementations own all the details of where the text comes from and how it
// is grouped into tabs and sections; callers only see the assembled Document.
type Source interface {
	Load(ctx context.Context) (document.Document, error)
}
