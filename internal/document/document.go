/*
Package document defines the shared domain model that flows from verse sources
to output targets.

A Source produces a Document; a Target renders it. The types form a small
hierarchy:

	Document ── has many ──▶ Tab ── has many ──▶ Section ── has many ──▶ Verse

A Section holds either a heading or a block of verses (never both), matching the
"# heading" lines and verse blocks of the original text input format.
*/
package document

// Verse is a single parsed verse: its number and the individual Greek words.
type Verse struct {
	Num   string
	Words []string
}

// Section is either a chapter heading or a block of verses. Exactly one of
// Heading or Verses is populated; Verses is non-nil only when Heading == "".
type Section struct {
	Heading string
	Verses  []Verse
}

// Tab is one named unit of output. For Google Sheets it becomes a spreadsheet
// tab; for other targets it might become a page or a file. The Name is the tab
// label (e.g. "1:1-1:10" or a chapter number).
type Tab struct {
	Name     string
	Sections []Section
}

// Document is a complete renderable unit: a titled collection of tabs. A
// single-tab fetch produces one Tab; a whole-chapter range produces one Tab per
// chapter.
type Document struct {
	Title string
	Tabs  []Tab
}

// TotalVerses returns the number of verses across every section of every tab.
// It is used to detect empty results before handing a Document to a Target.
func (d Document) TotalVerses() int {
	var n int
	for _, t := range d.Tabs {
		for _, s := range t.Sections {
			n += len(s.Verses)
		}
	}
	return n
}
