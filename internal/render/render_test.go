package render_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"moodle-mcp/internal/moodle"
	"moodle-mcp/internal/render"
	"moodle-mcp/internal/study"
	"moodle-mcp/internal/textfmt"
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
		{"wrapped invalid token", fmt.Errorf("courses: %w", &moodle.Error{ErrorCode: "invalidtoken"}), "MOODLE_TOKEN"},
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
