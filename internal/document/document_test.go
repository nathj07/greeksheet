package document

import "testing"

func TestTotalVerses(t *testing.T) {
	tests := []struct {
		name     string
		doc      Document
		expected int
	}{
		{
			name:     "empty_document",
			doc:      Document{},
			expected: 0,
		},
		{
			name: "single_tab_single_section",
			doc: Document{
				Title: "Test",
				Tabs: []Tab{
					{
						Name: "1:1-1:3",
						Sections: []Section{
							{Verses: []Verse{
								{Num: "1", Words: []string{"ἐν", "ἀρχῇ"}},
								{Num: "2", Words: []string{"καὶ"}},
								{Num: "3", Words: []string{"πάντα"}},
							}},
						},
					},
				},
			},
			expected: 3,
		},
		{
			name: "heading_section_contributes_zero_verses",
			doc: Document{
				Tabs: []Tab{
					{
						Sections: []Section{
							{Heading: "Chapter 1"},
							{Verses: []Verse{
								{Num: "1", Words: []string{"ἐν"}},
							}},
						},
					},
				},
			},
			expected: 1,
		},
		{
			name: "multiple_tabs",
			doc: Document{
				Tabs: []Tab{
					{
						Sections: []Section{
							{Verses: []Verse{
								{Num: "1", Words: []string{"ἐν"}},
								{Num: "2", Words: []string{"καὶ"}},
							}},
						},
					},
					{
						Sections: []Section{
							{Verses: []Verse{
								{Num: "3", Words: []string{"πάντα"}},
							}},
						},
					},
				},
			},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.doc.TotalVerses()
			if got != tt.expected {
				t.Errorf("TotalVerses() = %d, want %d", got, tt.expected)
			}
		})
	}
}
