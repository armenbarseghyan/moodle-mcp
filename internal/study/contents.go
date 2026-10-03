package study

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/armenbarseghyan/moodle-mcp/internal/moodle"
	"github.com/armenbarseghyan/moodle-mcp/internal/textfmt"
)

const maxSubsectionDepth = 3

// ContentsResult answers moodle_course_contents.
type ContentsResult struct {
	Course    Course
	Sections  []Section
	FetchedAt time.Time
}

// CourseContents resolves a course by id or name fragment and returns its
// sections as a tree (subsections nested under their parent).
func (s *Service) CourseContents(ctx context.Context, query string, refresh bool) (ContentsResult, error) {
	c, at, err := s.resolve(ctx, query, refresh)
	if err != nil {
		return ContentsResult{}, err
	}
	f, err := s.src.Contents(ctx, c.ID, refresh)
	if err != nil {
		return ContentsResult{}, err
	}
	var stamp oldest
	stamp.add(at)
	stamp.add(f.At)
	return ContentsResult{Course: c, Sections: BuildTree(f.Value, s.base()), FetchedAt: stamp.t}, nil
}

// BuildTree converts core_course_get_contents into domain sections.
//
// Moodle 4.5 subsections arrive as separate top-level sections with
// component "mod_subsection"; they are attached to the "subsection" module
// whose customdata.sectionid (or instance == itemid) points at them.
// Modules the user cannot see are counted, not shown. Empty sections are
// dropped. Delegated sections nobody references are kept at the end so that
// no content is lost.
func BuildTree(in []moodle.Section, base string) []Section {
	delegated := map[int]moodle.Section{}
	byItem := map[int]int{} // itemid -> section id
	for _, s := range in {
		if s.Component == "mod_subsection" {
			delegated[s.ID] = s
			byItem[s.ItemID] = s.ID
		}
	}
	used := map[int]bool{}
	b := treeBuilder{base: base, delegated: delegated, byItem: byItem, used: used}

	var out []Section
	for _, s := range in {
		if s.Component == "mod_subsection" {
			continue
		}
		if sec, ok := b.section(s, 0); ok {
			out = append(out, sec)
		}
	}
	for _, s := range in {
		if s.Component == "mod_subsection" && !used[s.ID] {
			if sec, ok := b.section(s, 0); ok {
				out = append(out, sec)
			}
		}
	}
	return out
}

type treeBuilder struct {
	base      string
	delegated map[int]moodle.Section
	byItem    map[int]int
	used      map[int]bool
}

func (b treeBuilder) section(s moodle.Section, depth int) (Section, bool) {
	b.used[s.ID] = true
	sec := Section{
		Name:    strings.TrimSpace(s.Name),
		Summary: textfmt.StripHTML(s.Summary),
	}
	for _, m := range s.Modules {
		if !bool(m.UserVisible) {
			sec.Hidden++
			continue
		}
		it := b.item(m, depth)
		if it.Kind == "subsection" && it.Sub == nil {
			continue // empty subsection
		}
		sec.Items = append(sec.Items, it)
	}
	return sec, len(sec.Items) > 0 || sec.Summary != "" || sec.Hidden > 0
}

func (b treeBuilder) item(m moodle.Module, depth int) Item {
	it := Item{
		CMID: m.ID,
		Kind: m.ModName,
		Name: strings.TrimSpace(m.Name),
		URL:  m.URL,
	}
	if it.URL == "" {
		it.URL = moduleURL(b.base, m.ModName, m.ID)
	}
	switch m.ModName {
	case "label":
		it.Text = textfmt.StripHTML(m.Description)
		it.URL = "" // labels have no page of their own
		if it.Text == it.Name {
			it.Name = ""
		}
		return it
	case "subsection":
		if depth < maxSubsectionDepth {
			if ds, ok := b.subsectionFor(m); ok {
				if sub, ok := b.section(ds, depth+1); ok {
					it.Sub = &sub
				}
			}
		}
		return it
	}
	it.Text = textfmt.StripHTML(m.Description)
	for _, c := range m.Contents {
		switch c.Type {
		case "url":
			if m.ModName == "url" && it.Link == "" {
				it.Link = c.FileURL
			}
		case "file":
			name := strings.Trim(c.FilePath, "/")
			if name != "" {
				name += "/"
			}
			it.Files = append(it.Files, File{
				Name:     name + c.FileName,
				URL:      BrowserFileURL(c.FileURL),
				Size:     c.FileSize,
				Modified: c.TimeModified.Time(),
			})
		}
	}
	// Pages and books ship their HTML as files: searchable, but not useful
	// as links (the module page shows them).
	if m.ModName == "page" || m.ModName == "book" {
		it.Content, it.Files = it.Files, nil
	}
	return it
}

func (b treeBuilder) subsectionFor(m moodle.Module) (moodle.Section, bool) {
	var cd struct {
		SectionID json.RawMessage `json:"sectionid"`
	}
	if m.CustomData != "" && json.Unmarshal([]byte(m.CustomData), &cd) == nil {
		if id, err := strconv.Atoi(strings.Trim(string(cd.SectionID), `"`)); err == nil {
			if s, ok := b.delegated[id]; ok {
				return s, true
			}
		}
	}
	if id, ok := b.byItem[m.Instance]; ok {
		return b.delegated[id], true
	}
	return moodle.Section{}, false
}
