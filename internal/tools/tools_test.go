package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/armenbarseghyan/moodle-mcp/internal/cache"
	"github.com/armenbarseghyan/moodle-mcp/internal/moodle"
	"github.com/armenbarseghyan/moodle-mcp/internal/moodletest"
	"github.com/armenbarseghyan/moodle-mcp/internal/study"
	"github.com/armenbarseghyan/moodle-mcp/internal/textfmt"
	"github.com/armenbarseghyan/moodle-mcp/internal/tools"
)

var update = flag.Bool("update", false, "rewrite golden files")

// now is the fixed clock of every scenario: Friday 2026-10-02 12:00 Vienna.
var now = time.Date(2026, 10, 2, 12, 0, 0, 0, textfmt.Vienna)

// fakeMoodle serves the shared fixture world (see moodletest.World).
func fakeMoodle(t *testing.T, extra ...moodletest.Option) *moodletest.Server {
	t.Helper()
	return moodletest.New(t, moodletest.World(t, extra...)...)
}

type env struct {
	srv     *moodletest.Server
	session *mcp.ClientSession
	dlDir   string
}

func setup(t *testing.T, token string, extra ...moodletest.Option) *env {
	t.Helper()
	srv := fakeMoodle(t, extra...)
	client, err := moodle.New(moodle.Config{BaseURL: srv.URL, Token: token, Backoff: func(int) time.Duration { return 0 }})
	if err != nil {
		t.Fatal(err)
	}
	dl := t.TempDir()
	clock := func() time.Time { return now }
	svc := study.New(cache.NewSource(client, clock), study.Options{Now: clock, DownloadDir: dl})
	return connect(t, tools.Deps{Service: svc, Now: clock, Version: "test"}, srv, dl)
}

func connect(t *testing.T, deps tools.Deps, srv *moodletest.Server, dl string) *env {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "moodle-mcp", Version: "test"}, nil)
	tools.Register(server, deps)
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return &env{srv: srv, session: cs, dlDir: dl}
}

func (e *env) call(t *testing.T, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := e.session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var b strings.Builder
	for _, c := range res.Content {
		b.WriteString(c.(*mcp.TextContent).Text)
	}
	out := b.String()
	if strings.Contains(out, moodletest.Token) {
		t.Fatalf("%s output leaks the token:\n%s", name, out)
	}
	return out, res.IsError
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	got = strings.ReplaceAll(got, goldenHost(t), "https://moodle.test")
	path := filepath.Join(moodletest.TestdataDir(), "golden", name+".md")
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden file (run: make golden): %v", err)
	}
	if got != string(want) {
		t.Errorf("%s differs from golden file (run: make golden to accept)\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

// goldenHost is replaced by a stable name, because httptest ports change.
var currentHost string

func goldenHost(*testing.T) string { return currentHost }

func TestToolsGolden(t *testing.T) {
	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"courses", "moodle_courses", nil},
		{"deadlines", "moodle_deadlines", nil},
		{"deadlines.no-overdue.30d", "moodle_deadlines", map[string]any{"days": 30, "include_overdue": false}},
		{"deadlines.3d", "moodle_deadlines", map[string]any{"days": 3}},
		{"contents.12308", "moodle_course_contents", map[string]any{"course": "12308"}},
		{"contents.unix-subsections", "moodle_course_contents", map[string]any{"course": "unix"}},
		{"contents.ambiguous", "moodle_course_contents", map[string]any{"course": "(DAT"}},
		{"contents.notfound", "moodle_course_contents", map[string]any{"course": "chemistry"}},
		{"search.gitlab", "moodle_search", map[string]any{"query": "gitlab"}},
		{"search.in-subsection", "moodle_search", map[string]any{"query": "screenrecording"}},
		{"search.nothing", "moodle_search", map[string]any{"query": "vpn"}},
		{"search.in-files.vpn", "moodle_search", map[string]any{"query": "vpn", "in_files": true}},
		{"search.in-files.course", "moodle_search", map[string]any{"query": "ssh-keygen", "in_files": true, "course": "programming"}},
		{"search.in-files.nothing", "moodle_search", map[string]any{"query": "kubernetes", "in_files": true, "course": "12308"}},
		{"grades.all", "moodle_grades", nil},
		{"grades.one", "moodle_grades", map[string]any{"course": "programming"}},
		{"announcements", "moodle_announcements", nil},
		{"announcements.1d", "moodle_announcements", map[string]any{"days": 1}},
		{"whats_new", "moodle_whats_new", nil},
		{"whats_new.5d", "moodle_whats_new", map[string]any{"days": 5}},
		{"whats_new.1d", "moodle_whats_new", map[string]any{"days": 1}},
		{"whoami", "moodle_whoami", nil},
	}
	e := setup(t, moodletest.Token)
	currentHost = e.srv.URL
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, isErr := e.call(t, tt.tool, tt.args)
			if isErr {
				t.Fatalf("unexpected error result:\n%s", out)
			}
			golden(t, tt.name, out)
		})
	}
}

// Semantic checks of the main tool, independent of formatting.
func TestDeadlines_Semantics(t *testing.T) {
	e := setup(t, moodletest.Token)
	out, _ := e.call(t, "moodle_deadlines", nil)
	mustContain := []string{
		"Homework R0", "❌ not submitted", // overdue, unsubmitted, within 7 days
		"Quiz LaTeX Basics",          // calendar only
		"Homework R1", "✅ submitted", // merged from both sources
		"Homework R2", // assignment only
		"2026-10-07 17:15 (Wednesday) — in 5 days",
		"Hidden or not yet available: 1 assignment",
	}
	for _, s := range mustContain {
		if !strings.Contains(out, s) {
			t.Errorf("missing %q in:\n%s", s, out)
		}
	}
	for _, s := range []string{"Old Exercise", "Optional Exercises", "Home Assignment Unix Shells", "ist fällig"} {
		if strings.Contains(out, s) {
			t.Errorf("must not contain %q:\n%s", s, out)
		}
	}
	if n := strings.Count(out, "Homework R1"); n != 1 {
		t.Errorf("Homework R1 appears %d times, want 1 (dedup)", n)
	}
}

func TestCaching(t *testing.T) {
	e := setup(t, moodletest.Token)
	for range 3 {
		e.call(t, "moodle_course_contents", map[string]any{"course": "12308"})
	}
	if n := e.srv.Calls("core_course_get_contents"); n != 1 {
		t.Errorf("contents fetched %d times, want 1 (cached)", n)
	}
	if n := e.srv.Calls("core_enrol_get_users_courses"); n != 1 {
		t.Errorf("courses fetched %d times, want 1", n)
	}
	if n := e.srv.Calls("core_webservice_get_site_info"); n != 1 {
		t.Errorf("site info fetched %d times, want 1", n)
	}

	e.call(t, "moodle_course_contents", map[string]any{"course": "12308", "refresh": true})
	if n := e.srv.Calls("core_course_get_contents"); n != 2 {
		t.Errorf("refresh must refetch: %d calls", n)
	}

	e.call(t, "moodle_grades", map[string]any{"course": "12308"})
	e.call(t, "moodle_grades", map[string]any{"course": "12308"})
	if n := e.srv.Calls("gradereport_user_get_grade_items"); n != 2 {
		t.Errorf("grades must not be cached: %d calls", n)
	}

	// Search reuses the contents cache filled above.
	before := e.srv.Calls("core_course_get_contents")
	e.call(t, "moodle_search", map[string]any{"query": "gitlab"})
	if got := e.srv.Calls("core_course_get_contents") - before; got != 2 {
		t.Errorf("search fetched contents %d times, want 2 (12308 cached; 12326 and 12307 new)", got)
	}
}

func TestInvalidToken(t *testing.T) {
	e := setup(t, "revoked-token")
	for _, tool := range []string{"moodle_courses", "moodle_deadlines", "moodle_search", "moodle_grades",
		"moodle_announcements", "moodle_whats_new", "moodle_whoami"} {
		args := map[string]any{}
		if tool == "moodle_search" {
			args["query"] = "git"
		}
		out, isErr := e.call(t, tool, args)
		if !isErr || !strings.Contains(out, "The Moodle token is invalid") {
			t.Errorf("%s: isErr=%v out=%q", tool, isErr, out)
		}
	}
}

func TestInitError(t *testing.T) {
	e := connect(t, tools.Deps{InitErr: errors.New("MOODLE_TOKEN is not set")}, nil, "")
	out, isErr := e.call(t, "moodle_deadlines", nil)
	if !isErr || !strings.Contains(out, "MOODLE_TOKEN") {
		t.Fatalf("isErr=%v out=%q", isErr, out)
	}
}

func TestPartialFailure(t *testing.T) {
	// One course's contents fail: search still answers from the others.
	e := setup(t, moodletest.Token, moodletest.Route("core_course_get_contents", func(p url.Values) moodletest.Resp {
		if p.Get("courseid") == "12326" {
			return moodletest.ErrorFile(t, "requireloginerror")
		}
		return moodletest.File(t, "core_course_get_contents", "real-12308")
	}))
	out, isErr := e.call(t, "moodle_search", map[string]any{"query": "gitlab"})
	if isErr || !strings.Contains(out, "GitLab IT+") || !strings.Contains(out, "⚠ Refresher on Unix Shells and LaTeX: The course or activity is not accessible") {
		t.Fatalf("isErr=%v\n%s", isErr, out)
	}
}

func TestDownload(t *testing.T) {
	files := moodletest.Files(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="IT+ Labs — Working on a DAT VM.pdf"`)
		_, _ = w.Write([]byte("%PDF-1.7"))
	}))
	e := setup(t, moodletest.Token, files)
	browserURL := e.srv.URL + "/pluginfile.php/327811/mod_resource/content/1/IT%2B%20Labs.pdf"

	out, isErr := e.call(t, "moodle_download", map[string]any{"fileurl": browserURL})
	want := filepath.Join(e.dlDir, "IT+ Labs — Working on a DAT VM.pdf")
	if isErr || !strings.Contains(out, want) || !strings.Contains(out, "In the browser: "+browserURL) {
		t.Fatalf("isErr=%v\n%s", isErr, out)
	}
	if b, err := os.ReadFile(want); err != nil || string(b) != "%PDF-1.7" {
		t.Fatalf("file: %q %v", b, err)
	}

	out, isErr = e.call(t, "moodle_download", map[string]any{"fileurl": "https://evil.example/pluginfile.php/1/a.pdf"})
	if !isErr || !strings.Contains(out, "does not point to a file of this Moodle") {
		t.Fatalf("foreign URL: isErr=%v %s", isErr, out)
	}
}

func TestListTools(t *testing.T) {
	e := setup(t, moodletest.Token)
	res, err := e.session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]*mcp.Tool{}
	for _, tl := range res.Tools {
		byName[tl.Name] = tl
	}
	want := []string{"moodle_courses", "moodle_deadlines", "moodle_course_contents", "moodle_search",
		"moodle_grades", "moodle_announcements", "moodle_whats_new", "moodle_download", "moodle_whoami"}
	if len(byName) != len(want) {
		t.Errorf("tools = %d, want %d", len(byName), len(want))
	}
	for _, n := range want {
		tl, ok := byName[n]
		if !ok {
			t.Errorf("missing tool %s", n)
			continue
		}
		if tl.Description == "" || tl.Annotations == nil {
			t.Errorf("%s: description/annotations missing", n)
		}
		if n != "moodle_download" && !tl.Annotations.ReadOnlyHint {
			t.Errorf("%s must be read-only", n)
		}
		if tl.OutputSchema != nil {
			t.Errorf("%s: unexpected output schema (output is markdown)", n)
		}
	}

	schema, _ := json.Marshal(byName["moodle_deadlines"].InputSchema)
	for _, s := range []string{`"default":14`, `"minimum":1`, `"maximum":90`, `"include_overdue"`} {
		if !strings.Contains(string(schema), s) {
			t.Errorf("deadlines schema lacks %s: %s", s, schema)
		}
	}
	contents, _ := json.Marshal(byName["moodle_course_contents"].InputSchema)
	if !strings.Contains(string(contents), `"required":["course"]`) {
		t.Errorf("course must be required: %s", contents)
	}
}

func TestInputValidation(t *testing.T) {
	e := setup(t, moodletest.Token)
	for _, tc := range []struct {
		tool string
		args map[string]any
	}{
		{"moodle_deadlines", map[string]any{"days": 0}},
		{"moodle_deadlines", map[string]any{"days": 365}},
		{"moodle_course_contents", map[string]any{}},
		{"moodle_search", map[string]any{"query": "x"}},
	} {
		if out, isErr := e.call(t, tc.tool, tc.args); !isErr {
			t.Errorf("%s %v must be rejected, got:\n%s", tc.tool, tc.args, out)
		}
	}
}

func TestSearchInFiles_TextIsCached(t *testing.T) {
	e := setup(t, moodletest.Token)
	out, isErr := e.call(t, "moodle_search", map[string]any{"query": "different setup", "in_files": true, "course": "12308"})
	if isErr || !strings.Contains(out, "p. 2") || !strings.Contains(out, "Different setup for macOS") {
		t.Fatalf("isErr=%v\n%s", isErr, out)
	}
	first := e.srv.Calls("pluginfile")
	if first == 0 {
		t.Fatal("files must be fetched on the first search")
	}
	e.call(t, "moodle_search", map[string]any{"query": "vpn", "in_files": true, "course": "12308"})
	if n := e.srv.Calls("pluginfile"); n != first {
		t.Errorf("unchanged files fetched again: %d → %d", first, n)
	}
}
