package study

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/armenbarseghyan/moodle-mcp/internal/extract"
	"github.com/armenbarseghyan/moodle-mcp/internal/textfmt"
)

// Search limits.
const (
	MaxSearchHits = 20
	MaxFileHits   = 15
	maxHitLabels  = 3
	snippetRunes  = 240
)

// Field weights for metadata search ranking.
const (
	weightName    = 3
	weightFile    = 2
	weightSection = 1
	weightText    = 1
	bonusPhrase   = 2
)

// SearchQuery describes a search.
type SearchQuery struct {
	Text    string
	Course  string // optional id or name fragment; empty: all active courses
	InFiles bool   // also search inside PDFs, pages and documents
	Refresh bool
}

// Hit is a module whose name, files, section or description match.
type Hit struct {
	Course CourseRef
	Path   []string // section names, outermost first
	Item   Item
	Score  int
}

// FileHit is a file whose content matches.
type FileHit struct {
	Course  CourseRef
	Path    []string
	Item    Item // the module the file belongs to (without files)
	File    File
	Labels  []string // matching pages/slides/entries, at most maxHitLabels
	More    int      // further matching parts not listed
	Snippet string
	Score   int
}

// SearchResult answers moodle_search.
type SearchResult struct {
	Query     string
	Scope     *CourseRef // set when the search was limited to one course
	Hits      []Hit      // at most MaxSearchHits
	Total     int
	InFiles   bool
	FileHits  []FileHit // at most MaxFileHits
	FileTotal int
	Indexed   int // files whose text was searched
	Skipped   int // unsupported or too large
	Failed    int // could not be read
	Errors    []CourseError
	FetchedAt time.Time
}

// Search finds modules whose name, file names, section or description
// contain every word of the query; with InFiles it also searches the text of
// PDFs, Office documents, notebooks, archives and Moodle pages.
func (s *Service) Search(ctx context.Context, q SearchQuery) (SearchResult, error) {
	res := SearchResult{Query: q.Text, InFiles: q.InFiles}
	var stamp oldest

	var courses []Course
	if strings.TrimSpace(q.Course) != "" {
		c, at, err := s.resolve(ctx, q.Course, q.Refresh)
		if err != nil {
			return res, err
		}
		stamp.add(at)
		ref := c.Ref()
		res.Scope = &ref
		courses = []Course{c}
	} else {
		cs, at, err := s.activeCourses(ctx, q.Refresh)
		if err != nil {
			return res, err
		}
		stamp.add(at)
		courses = cs
	}

	trees, errs, err := fanOut(ctx, courses, func(ctx context.Context, c Course) (Fetched[[]Section], error) {
		f, err := s.src.Contents(ctx, c.ID, q.Refresh)
		if err != nil {
			return Fetched[[]Section]{}, err
		}
		return Fetched[[]Section]{Value: BuildTree(f.Value, s.base()), At: f.At}, nil
	})
	if err != nil {
		return res, err
	}
	var hits []Hit
	var files []located
	for i, c := range courses {
		if errs[i] != nil {
			res.Errors = append(res.Errors, CourseError{Course: c.Ref(), Err: errs[i]})
			continue
		}
		stamp.add(trees[i].At)
		hits = append(hits, SearchSections(c.Ref(), trees[i].Value, q.Text)...)
		if q.InFiles {
			files = append(files, collectFiles(c.Ref(), trees[i].Value)...)
		}
	}
	slices.SortStableFunc(hits, func(a, b Hit) int {
		if a.Score != b.Score {
			return b.Score - a.Score
		}
		if c := strings.Compare(a.Course.Label, b.Course.Label); c != 0 {
			return c
		}
		return strings.Compare(a.Item.Name, b.Item.Name)
	})
	res.Total = len(hits)
	res.Hits = hits[:min(len(hits), MaxSearchHits)]

	if q.InFiles {
		if err := s.searchFiles(ctx, &res, files, q.Text); err != nil {
			return res, err
		}
	}
	res.FetchedAt = stamp.t
	return res, nil
}

// located is a file with the place it was found in.
type located struct {
	course CourseRef
	path   []string
	item   Item
	file   File
}

// collectFiles lists every distinct file of a course tree, including the
// HTML of pages and books.
func collectFiles(course CourseRef, secs []Section) []located {
	seen := map[string]bool{}
	var out []located
	var walk func(secs []Section, path []string)
	walk = func(secs []Section, path []string) {
		for _, sec := range secs {
			p := append(slices.Clone(path), sec.Name)
			for _, it := range sec.Items {
				if it.Sub != nil {
					walk([]Section{*it.Sub}, p)
				}
				flat := it
				flat.Sub, flat.Files, flat.Content = nil, nil, nil
				for _, f := range append(slices.Clone(it.Files), it.Content...) {
					if f.URL == "" || seen[f.URL] {
						continue
					}
					seen[f.URL] = true
					out = append(out, located{course: course, path: p, item: flat, file: f})
				}
			}
		}
	}
	walk(secs, nil)
	return out
}

func (s *Service) searchFiles(ctx context.Context, res *SearchResult, files []located, query string) error {
	var todo []located
	for _, f := range files {
		if !extract.Supported(f.file.Name) || f.file.Size > extract.MaxFileBytes {
			res.Skipped++
			continue
		}
		todo = append(todo, f)
	}
	texts, errs, err := fanOut(ctx, todo, func(ctx context.Context, f located) ([]extract.Part, error) {
		t, err := s.src.FileText(ctx, f.file)
		return t.Value, err
	})
	if err != nil {
		return err
	}
	var hits []FileHit
	for i, f := range todo {
		if errs[i] != nil {
			res.Failed++
			s.log.Warn("file text", "file", f.file.Name, "err", errs[i])
			continue
		}
		res.Indexed++
		if h, ok := MatchFile(texts[i], query); ok {
			h.Course, h.Path, h.Item, h.File = f.course, f.path, f.item, f.file
			hits = append(hits, h)
		}
	}
	slices.SortStableFunc(hits, func(a, b FileHit) int {
		if a.Score != b.Score {
			return b.Score - a.Score
		}
		if c := strings.Compare(a.Course.Label, b.Course.Label); c != 0 {
			return c
		}
		return strings.Compare(a.File.Name, b.File.Name)
	})
	res.FileTotal = len(hits)
	res.FileHits = hits[:min(len(hits), MaxFileHits)]
	return nil
}

// minCompactWord: words at least this long may also match with all spaces
// removed. Shorter ones would match across word boundaries ("class
// hierarchy" contains "ssh").
const minCompactWord = 6

// haystack is a text prepared for word matching. PDFs often lose spaces
// ("offcampus": substring matching already copes) or invent them ("di ff
// erent": only the space-free form matches).
type haystack struct{ spaced, compact string }

func newHaystack(s string) haystack {
	n := Normalize(s)
	return haystack{spaced: n, compact: strings.ReplaceAll(n, " ", "")}
}

func (h haystack) has(word string) bool {
	return strings.Contains(h.spaced, word) ||
		(len([]rune(word)) >= minCompactWord && strings.Contains(h.compact, word))
}

// MatchFile reports whether the document contains every word of query and
// returns the matching parts and a snippet. Parts containing all words rank
// above parts containing only some.
func MatchFile(parts []extract.Part, query string) (FileHit, bool) {
	words := strings.Fields(Normalize(query))
	if len(words) == 0 || len(parts) == 0 {
		return FileHit{}, false
	}
	type scored struct {
		idx, n int
	}
	var full, partial []scored
	found := map[string]bool{}
	for i, p := range parts {
		hs := newHaystack(p.Text)
		n := 0
		for _, w := range words {
			if hs.has(w) {
				n++
				found[w] = true
			}
		}
		switch {
		case n == len(words):
			full = append(full, scored{i, n})
		case n > 0:
			partial = append(partial, scored{i, n})
		}
	}
	if len(found) < len(words) {
		return FileHit{}, false // some word appears nowhere in the document
	}
	// Parts with every word are the answer; parts with only some words are
	// listed only when no single part has them all.
	ranked := full
	if len(ranked) == 0 {
		slices.SortStableFunc(partial, func(a, b scored) int { return b.n - a.n })
		ranked = partial
	}
	h := FileHit{Score: 10*len(full) + len(partial)}
	for _, sc := range ranked {
		if l := parts[sc.idx].Label; l != "" {
			if len(h.Labels) < maxHitLabels {
				h.Labels = append(h.Labels, l)
			} else {
				h.More++
			}
		}
	}
	h.Snippet = snippet(parts[ranked[0].idx].Text, words)
	return h, true
}

// snippet returns the line with most query words plus the next line.
func snippet(text string, words []string) string {
	lines := strings.Split(text, "\n")
	best, bestN := 0, -1
	for i, l := range lines {
		hs := newHaystack(l)
		n := 0
		for _, w := range words {
			if hs.has(w) {
				n++
			}
		}
		if n > bestN {
			best, bestN = i, n
		}
	}
	s := lines[best]
	if best+1 < len(lines) && len([]rune(s)) < snippetRunes/2 {
		s += " " + lines[best+1]
	}
	if best > 0 && len([]rune(s)) < snippetRunes/3 {
		s = lines[best-1] + " " + s
	}
	return textfmt.Truncate(s, snippetRunes)
}

// SearchSections scores every item of a course tree against query.
func SearchSections(course CourseRef, secs []Section, query string) []Hit {
	words := strings.Fields(Normalize(query))
	if len(words) == 0 {
		return nil
	}
	phrase := strings.Join(words, " ")
	var hits []Hit
	var walk func(secs []Section, path []string)
	walk = func(secs []Section, path []string) {
		for _, sec := range secs {
			p := append(slices.Clone(path), sec.Name)
			for _, it := range sec.Items {
				if it.Sub != nil {
					walk([]Section{*it.Sub}, p)
				}
				if score := scoreItem(it, sec.Name, words, phrase); score > 0 {
					flat := it
					flat.Sub, flat.Content = nil, nil
					hits = append(hits, Hit{Course: course, Path: p, Item: flat, Score: score})
				}
			}
		}
	}
	walk(secs, nil)
	return hits
}

func scoreItem(it Item, section string, words []string, phrase string) int {
	name := Normalize(it.Name)
	sec := Normalize(section)
	text := Normalize(it.Text + " " + it.Link)
	var files []string
	for _, f := range it.Files {
		files = append(files, Normalize(f.Name))
	}
	fileText := strings.Join(files, " ")

	total := 0
	for _, w := range words {
		best := 0
		for _, f := range []struct {
			s string
			w int
		}{{name, weightName}, {fileText, weightFile}, {sec, weightSection}, {text, weightText}} {
			if f.w > best && strings.Contains(f.s, w) {
				best = f.w
			}
		}
		if best == 0 {
			return 0 // every word must match somewhere
		}
		total += best
	}
	if strings.Contains(name, phrase) {
		total += bonusPhrase
	}
	return total
}
