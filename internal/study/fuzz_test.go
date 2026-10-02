package study_test

import (
	"testing"

	"moodle-mcp/internal/extract"
	"moodle-mcp/internal/study"
)

// FuzzSearchInput: user queries and extracted text are arbitrary; matching,
// normalisation and course resolution must never panic.
func FuzzSearchInput(f *testing.F) {
	f.Add("vpn", "Off campus: FH VPN required")
	f.Add("Prüfung ß", "pruefung strasse")
	f.Add("", "\xff\xfe")
	courses := []study.Course{{ID: 1, Name: "(DAT_WS2026_1) Programming", Label: "Programming"},
		{ID: 2, Name: "(DAT_WS2026_1) Probability", Label: "Probability", Past: true}}
	f.Fuzz(func(t *testing.T, query, text string) {
		_ = study.Normalize(query)
		_, _ = study.MatchFile([]extract.Part{{Label: "p. 1", Text: text}}, query)
		_, _ = study.ResolveCourse(courses, query)
	})
}
