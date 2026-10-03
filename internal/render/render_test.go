package render_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/armenbarseghyan/moodle-mcp/internal/moodle"
	"github.com/armenbarseghyan/moodle-mcp/internal/render"
	"github.com/armenbarseghyan/moodle-mcp/internal/study"
	"github.com/armenbarseghyan/moodle-mcp/internal/textfmt"
)

type timeoutErr struct{}

func (timeoutErr) Error() string { return "i/o timeout" }
func (timeoutErr) Timeout() bool { return true }

func TestErrorText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"invalid token", &moodle.Error{ErrorCode: "invalidtoken"}, "The Moodle token is invalid or revoked"},
		{"wrapped invalid token", fmt.Errorf("courses: %w", &moodle.Error{ErrorCode: "invalidtoken"}), "setup.sh"},
		{"access control", &moodle.Error{ErrorCode: "accessexception"}, "access control"},
		{"require login", &moodle.Error{ErrorCode: "requireloginerror"}, "not accessible"},
		{"maintenance", &moodle.Error{ErrorCode: "sitemaintenance"}, "maintenance mode"},
		{"other moodle error", &moodle.Error{ErrorCode: "invalidrecord", Message: "Datensatz fehlt"}, "invalidrecord: Datensatz fehlt"},
		{"not allowed", fmt.Errorf("%w: x", moodle.ErrNotAllowed), "read-only"},
		{"foreign url", fmt.Errorf("%w: host", moodle.ErrForeignURL), "pluginfile.php"},
		{"html instead of json", &moodle.UnexpectedResponseError{}, "non-JSON"},
		{"http status", &moodle.HTTPError{Status: 502}, "HTTP 502"},
		{"deadline", fmt.Errorf("x: %w", context.DeadlineExceeded), "timeout"},
		{"net timeout", &url.Error{Op: "Post", URL: "https://m.test", Err: timeoutErr{}}, "timeout"},
		{"net error", &url.Error{Op: "Post", URL: "https://m.test", Err: errors.New("connection refused")}, "Cannot reach Moodle: connection refused"},
		{"plain", errors.New("boom"), "boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := render.ErrorText(tt.err); !strings.Contains(got, tt.want) {
				t.Errorf("got %q, want it to contain %q", got, tt.want)
			}
		})
	}
	if render.ErrorText(nil) != "" {
		t.Error("nil error must render empty")
	}
}

func TestSize(t *testing.T) {
	t.Parallel()
	for in, want := range map[int64]string{
		0: "", -1: "", 512: "512 B", 1024: "1 KB", 227077: "222 KB", 2958940: "2.8 MB",
	} {
		if got := render.Size(in); got != want {
			t.Errorf("Size(%d) = %q, want %q", in, got, want)
		}
	}
}

var now = time.Date(2026, 10, 2, 12, 0, 0, 0, textfmt.Vienna)

func TestOutputIsBounded(t *testing.T) {
	t.Parallel()
	var hits []study.Hit
	for i := range 2000 {
		hits = append(hits, study.Hit{
			Course: study.CourseRef{Label: "Course"},
			Path:   []string{"Section"},
			Item:   study.Item{Name: fmt.Sprintf("Item %d with a reasonably long name", i), Kind: "resource", URL: "https://m.test/x"},
		})
	}
	out := render.Search(study.SearchResult{Query: "item", Hits: hits, Total: len(hits)}, now)
	if len(out) > render.MaxChars+300 {
		t.Errorf("output %d chars, limit %d", len(out), render.MaxChars)
	}
	if !strings.Contains(out, "not shown — narrow the request") {
		t.Error("truncation must be announced")
	}
}

func TestStaleCacheNote(t *testing.T) {
	t.Parallel()
	r := study.CoursesResult{FetchedAt: now.Add(-7 * time.Minute)}
	if out := render.Courses(r, now); !strings.Contains(out, "Cached data from 7 min ago") {
		t.Errorf("stale note missing:\n%s", out)
	}
	r.FetchedAt = now.Add(-10 * time.Second)
	if out := render.Courses(r, now); strings.Contains(out, "Cached data") {
		t.Errorf("fresh data must not carry a cache note:\n%s", out)
	}
}

func TestLinkEscaping(t *testing.T) {
	t.Parallel()
	r := study.ContentsResult{
		Course: study.Course{ID: 1, Label: "C"},
		Sections: []study.Section{{Name: "S", Items: []study.Item{{
			Kind: "resource", Name: "[PDP] Demo (v2)", URL: "https://m.test/mod/resource/view.php?id=1",
			Files: []study.File{{Name: "a b (1).pdf", URL: "https://m.test/pluginfile.php/1/a b (1).pdf", Size: 10}},
		}}}},
	}
	out := render.Contents(r, now)
	for _, want := range []string{`[\[PDP\] Demo (v2)](https://m.test/mod/resource/view.php?id=1)`,
		`(https://m.test/pluginfile.php/1/a%20b%20%281%29.pdf)`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in:\n%s", want, out)
		}
	}
}

func TestDeadlineMarks(t *testing.T) {
	t.Parallel()
	item := func(title string, due time.Duration, s study.Submission) study.Deadline {
		return study.Deadline{Key: study.DeadlineKey{Module: "assign"}, Course: study.CourseRef{Label: "C"},
			Title: title, URL: "https://m.test/a", Due: now.Add(due), Status: s}
	}
	out := render.Deadlines(study.DeadlinesResult{
		Days: 14, Until: now.Add(14 * 24 * time.Hour),
		Overdue: []study.Deadline{item("Late", -24*time.Hour, study.SubmissionNotSubmitted)},
		Upcoming: []study.Deadline{
			item("Soon", 10*time.Hour, study.SubmissionNotSubmitted),
			item("SoonDraft", 20*time.Hour, study.SubmissionDraft),
			item("SoonDone", 10*time.Hour, study.SubmissionSubmitted),
			item("Later", 5*24*time.Hour, study.SubmissionNotSubmitted),
		},
	}, now)
	for _, tt := range []struct {
		title, want string
		urgent      bool
	}{
		{"Late", "was due", false}, // overdue has its own section, no ⏰
		{"Soon", "⬜ not submitted yet", true},
		{"SoonDraft", "📝 draft, not submitted", true},
		{"SoonDone", "✅ submitted", false},
		{"Later", "⬜ not submitted yet", false},
	} {
		head := "**[" + tt.title + "](https://m.test/a)**"
		i := strings.Index(out, head)
		if i < 0 {
			t.Fatalf("%s missing:\n%s", tt.title, out)
		}
		lineStart := strings.LastIndex(out[:i], "\n") + 1
		if got := strings.HasPrefix(out[lineStart:], "- ⏰ "); got != tt.urgent {
			t.Errorf("%s: urgent mark = %v, want %v:\n%s", tt.title, got, tt.urgent, out)
		}
		lines := strings.SplitN(out[i:], "\n", 3)
		if len(lines) < 2 || !strings.Contains(lines[1], tt.want) {
			t.Errorf("%s: second line %q, want %q", tt.title, lines, tt.want)
		}
	}
	if strings.Contains(out, "❌") && strings.Count(out, "❌") != 1 {
		t.Errorf("only the overdue item gets ❌:\n%s", out)
	}
}

func TestGradeNumbers(t *testing.T) {
	t.Parallel()
	pct := 87.5
	for display, want := range map[string]string{
		"87,50": "**87.5** / 100", "100,00": "**100** / 100", "-1,5": "**-1.5** / 100",
		"Sehr gut": "**Sehr gut** / 100", "87.50": "**87.50** / 100", "B+": "**B+** / 100",
	} {
		out := render.Grades(study.GradesResult{Courses: []study.CourseGrades{{
			Course: study.CourseRef{Label: "C"},
			Items:  []study.Grade{{Name: "H", Display: display, Max: 100, Percent: &pct}},
		}}}, now)
		if !strings.Contains(out, want) {
			t.Errorf("%q: want %q in:\n%s", display, want, out)
		}
	}
}
