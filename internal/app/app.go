// Package app wires a verse source to an output target and runs the end-to-end
// flow: load a Document, validate it, and render it to its destination.
package app

import (
	"context"
	"fmt"

	"github.com/nathj07/greeksheet/internal/output"
	"github.com/nathj07/greeksheet/internal/source"
)

// App runs a single generation: it loads content from Source and renders it to
// Target. TitleOverride, when non-empty, replaces the Document title chosen by
// the source (used for the -title flag).
type App struct {
	Source        source.Source
	Target        output.Target
	TitleOverride string
}

// Run loads the document, fails fast if it contains no verses, applies any
// title override, and renders it — printing the resulting location.
func (a App) Run(ctx context.Context) (string, error) {
	d, err := a.Source.Load(ctx)
	if err != nil {
		return "", err
	}

	if d.TotalVerses() == 0 {
		return "", fmt.Errorf("no verses found — check that the reference or input file contains valid content")
	}
	fmt.Printf("Parsed %d verses\n", d.TotalVerses())

	if a.TitleOverride != "" {
		d.Title = a.TitleOverride
	}

	url, err := a.Target.Render(ctx, d)
	if err != nil {
		return "", err
	}
	return url, nil
}
