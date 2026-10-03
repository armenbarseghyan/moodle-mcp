package study_test

import (
	"testing"
	"time"

	"github.com/armenbarseghyan/moodle-mcp/internal/study"
)

func TestNewMaterials(t *testing.T) {
	t.Parallel()
	since := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	before, after := since.Add(-time.Hour), since.Add(time.Hour)
	file := func(name string, created, modified time.Time) study.File {
		return study.File{Name: name, URL: "https://m.test/pluginfile.php/1/" + name, Created: created, Modified: modified}
	}
	secs := []study.Section{{Name: "Week 1", Items: []study.Item{
		{Name: "Slides", Files: []study.File{
			file("new.pdf", after, after),
			file("updated.pdf", before, after),
			file("old.pdf", before, before),
			file("no-created.pdf", time.Time{}, after), // Moodle left timecreated out
		}},
		{Name: "Page", Kind: "page", Content: []study.File{file("index.html", before, after)}},
		{Name: "Sub", Sub: &study.Section{Name: "Part A", Items: []study.Item{
			{Name: "Sheet", Files: []study.File{file("sheet.pdf", after, after)}},
		}}},
	}}}
	got := map[string]study.NewMaterial{}
	for _, m := range study.NewMaterials(study.CourseRef{Label: "C"}, secs, since) {
		got[m.File.Name] = m
	}
	for name, wantAdded := range map[string]bool{
		"new.pdf": true, "updated.pdf": false, "no-created.pdf": false, "index.html": false, "sheet.pdf": true,
	} {
		m, ok := got[name]
		if !ok {
			t.Errorf("%s missing", name)
			continue
		}
		if m.Added != wantAdded {
			t.Errorf("%s: Added = %v, want %v", name, m.Added, wantAdded)
		}
	}
	if _, ok := got["old.pdf"]; ok {
		t.Error("old.pdf is older than the period")
	}
	if p := got["sheet.pdf"].Path; len(p) != 2 || p[1] != "Part A" {
		t.Errorf("subsection path = %v", p)
	}
	if len(got) != 5 {
		t.Errorf("got %d materials, want 5", len(got))
	}
}
