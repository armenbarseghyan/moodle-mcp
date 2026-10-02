package study_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"moodle-mcp/internal/extract"
	"moodle-mcp/internal/moodle"
	"moodle-mcp/internal/moodletest"
	"moodle-mcp/internal/study"
	"moodle-mcp/internal/textfmt"
)

const base = "https://moodle.example.test"

func vie(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, textfmt.Vienna)
	if err != nil {
		panic(err)
	}
	return t
}

func unix(s string) moodle.Unix { return moodle.Unix(vie(s).Unix()) }

func loadFixture[T any](t *testing.T, fn, scenario string) T {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(moodletest.TestdataDir(), "moodle", fn+"."+scenario+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestSharedPrefix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		names []string
		want  string
	}{
		{"semester tag", []string{"(DAT_WS2026_1) Probability", "(DAT_WS2026_1) Programming"}, "(DAT_WS2026_1) "},
		{"cut back to word boundary", []string{"(DAT_WS2026_1) Refresher on Maths", "(DAT_WS2026_1) Refresher on Unix"}, "(DAT_WS2026_1) Refresher on "},
		{"single course", []string{"(DAT_WS2026_1) Programming"}, ""},
		{"nothing shared", []string{"Algebra", "Biology"}, ""},
		{"shared word without space", []string{"Data Science", "Databases"}, ""},
		{"would empty a name", []string{"Intro ", "Intro Advanced"}, ""},
		{"different semesters", []string{"(DAT_WS2026_1) A", "(DAT_SS2027_2) B"}, ""},
		{"none", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := study.SharedPrefix(tt.names); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{"Prüfung", "prufung"},
		{"Pruefung", "prufung"},
		{"PRUFUNG", "prufung"},
		{"Straße", "strasse"},
		{"(DAT_WS2026_1) Data-Science & AI", "dat ws2026 1 data science ai"},
		{"  many   spaces ", "many spaces"},
		{"Ängste", "angste"},
		{"café", "cafe"},
		{"—", ""},
	}
	for _, tt := range tests {
		if got := study.Normalize(tt.in); got != tt.want {
			t.Errorf("Normalize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestResolveCourse(t *testing.T) {
	t.Parallel()
	courses := []study.Course{
		{ID: 12308, Name: "(DAT_WS2026_1) Programming and Data Processing", Label: "Programming and Data Processing"},
		{ID: 12320, Name: "(DAT_WS2026_1) Databases and Query Languages", Label: "Databases and Query Languages"},
		{ID: 12318, Name: "(DAT_WS2026_1) Refresher on Mathematics", Label: "Refresher on Mathematics"},
		{ID: 12326, Name: "(DAT_WS2026_1) Refresher on Unix Shells and LaTeX", Label: "Refresher on Unix Shells and LaTeX"},
		{ID: 12309, Name: "(DAT_WS2026_1) Introduction to Data Science and Artificial Intelligence", Label: "Introduction to Data Science and Artificial Intelligence"},
		{ID: 11000, Name: "(DAT_SS2026_0) Mathematische Prüfungsvorbereitung", Label: "(DAT_SS2026_0) Mathematische Prüfungsvorbereitung", Past: true},
		{ID: 11001, Name: "(DAT_SS2026_0) Data Engineering", Label: "(DAT_SS2026_0) Data Engineering", Past: true},
	}
	tests := []struct {
		name       string
		query      string
		wantID     int
		wantAmbig  []int
		wantNotFnd bool
	}{
		{"numeric id", "12320", 12320, nil, false},
		{"numeric id with spaces", " 12326 ", 12326, nil, false},
		{"unknown id", "99999", 0, nil, true},
		{"exact label", "Refresher on Mathematics", 12318, nil, false},
		{"exact full name", "(DAT_WS2026_1) Refresher on Mathematics", 12318, nil, false},
		{"substring, case-insensitive", "unix", 12326, nil, false},
		{"substring of two", "refresher", 0, []int{12318, 12326}, false},
		{"words in any order", "latex unix", 12326, nil, false},
		{"exact beats substring", "databases and query languages", 12320, nil, false},
		{"data is ambiguous among active", "data", 0, []int{12308, 12320, 12309}, false},
		{"past course found when no active match", "prüfungsvorbereitung", 11000, nil, false},
		{"umlaut transliteration", "pruefungsvorbereitung", 11000, nil, false},
		{"active preferred over past", "data processing", 12308, nil, false},
		{"punctuation ignored", "data-science", 12309, nil, false},
		{"no match", "chemistry", 0, nil, true},
		{"empty", "  ", 0, nil, true},
		{"only punctuation", "--", 0, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, err := study.ResolveCourse(courses, tt.query)
			var amb *study.AmbiguousError
			var nf *study.NotFoundError
			switch {
			case tt.wantNotFnd:
				if !errors.As(err, &nf) {
					t.Fatalf("got %v, %v; want NotFoundError", c.ID, err)
				}
				for _, a := range nf.Available {
					if a.Past {
						t.Error("NotFound must list only active courses")
					}
				}
			case tt.wantAmbig != nil:
				if !errors.As(err, &amb) {
					t.Fatalf("got %v, %v; want AmbiguousError", c.ID, err)
				}
				var got []int
				for _, cand := range amb.Candidates {
					got = append(got, cand.ID)
				}
				if !sameInts(got, tt.wantAmbig) {
					t.Errorf("candidates = %v, want %v", got, tt.wantAmbig)
				}
			default:
				if err != nil || c.ID != tt.wantID {
					t.Fatalf("got %d, %v; want %d", c.ID, err, tt.wantID)
				}
			}
		})
	}
}

func sameInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[int]int{}
	for _, x := range a {
		m[x]++
	}
	for _, x := range b {
		m[x]--
	}
	for _, v := range m {
		if v != 0 {
			return false
		}
	}
	return true
}

func TestBrowserFileURL(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{"https://m.test/webservice/pluginfile.php/327811/mod_resource/content/1/IT%2B%20Labs%20%E2%80%94%20Working%20on%20a%20DAT%20VM.pdf?forcedownload=1",
			"https://m.test/pluginfile.php/327811/mod_resource/content/1/IT%2B%20Labs%20%E2%80%94%20Working%20on%20a%20DAT%20VM.pdf"},
		{"https://m.test/webservice/pluginfile.php/1/a.pdf?token=SECRET&forcedownload=1", "https://m.test/pluginfile.php/1/a.pdf"},
		{"https://m.test/pluginfile.php/1/a.pdf", "https://m.test/pluginfile.php/1/a.pdf"},
		{"https://m.test/sub/webservice/pluginfile.php/1/a.pdf", "https://m.test/sub/pluginfile.php/1/a.pdf"},
		{"https://gitlab.example.test/", "https://gitlab.example.test/"},
		{"https://m.test/webservice/pluginfile.php/1/a.pdf?other=1", "https://m.test/pluginfile.php/1/a.pdf?other=1"},
	}
	for _, tt := range tests {
		if got := study.BrowserFileURL(tt.in); got != tt.want {
			t.Errorf("BrowserFileURL(%s)\n got  %s\n want %s", tt.in, got, tt.want)
		}
	}
}

func ev(module string, instance int, due string, course int) moodle.Event {
	e := moodle.Event{ModuleName: module, Instance: instance, TimeSort: unix(due), EventType: "due",
		Name: "Event " + module + " ist fällig", ActivityName: "Event " + module,
		URL: base + "/mod/" + module + "/view.php?id=" + itoa(instance)}
	e.Course = &struct {
		ID       int    `json:"id"`
		FullName string `json:"fullname"`
	}{ID: course, FullName: "Course " + itoa(course)}
	return e
}

func itoa(i int) string    { return strings.TrimSpace(strings.Repeat(" ", 0) + jsonInt(i)) }
func jsonInt(i int) string { b, _ := json.Marshal(i); return string(b) }

func asg(id, cmid, course int, name, due string) moodle.Assignment {
	a := moodle.Assignment{ID: id, CMID: cmid, Course: course, Name: name}
	if due != "" {
		a.DueDate = unix(due)
	}
	return a
}

func TestMergeDeadlines(t *testing.T) {
	t.Parallel()
	refs := map[int]study.CourseRef{1: {ID: 1, Label: "Programming"}, 2: {ID: 2, Label: "Unix"}}
	type want struct {
		key     study.DeadlineKey
		title   string
		due     string
		sources int
		status  study.Submission
		course  string
	}
	tests := []struct {
		name    string
		events  []moodle.Event
		assigns []moodle.CourseAssignments
		want    []want
	}{
		{
			name:    "same assignment from both sources is merged",
			events:  []moodle.Event{ev("assign", 501, "2026-10-07 17:15", 1)},
			assigns: []moodle.CourseAssignments{{ID: 1, Assignments: []moodle.Assignment{asg(501, 9501, 1, "Homework R1", "2026-10-07 17:15")}}},
			want: []want{{study.DeadlineKey{Module: "assign", Instance: 501}, "Homework R1", "2026-10-07 17:15",
				study.FromCalendar | study.FromAssignments, study.SubmissionUnknown, "Programming"}},
		},
		{
			name:    "calendar time wins (user override)",
			events:  []moodle.Event{ev("assign", 501, "2026-10-09 12:00", 1)},
			assigns: []moodle.CourseAssignments{{ID: 1, Assignments: []moodle.Assignment{asg(501, 9501, 1, "Homework R1", "2026-10-07 17:15")}}},
			want: []want{{study.DeadlineKey{Module: "assign", Instance: 501}, "Homework R1", "2026-10-09 12:00",
				study.FromCalendar | study.FromAssignments, study.SubmissionUnknown, "Programming"}},
		},
		{
			name:    "assignment without event is kept (event vanishes after submission)",
			assigns: []moodle.CourseAssignments{{ID: 1, Assignments: []moodle.Assignment{asg(503, 9503, 1, "Homework R2", "2026-10-10 10:00")}}},
			want: []want{{study.DeadlineKey{Module: "assign", Instance: 503}, "Homework R2", "2026-10-10 10:00",
				study.FromAssignments, study.SubmissionUnknown, "Programming"}},
		},
		{
			name:    "no due date is not a deadline",
			assigns: []moodle.CourseAssignments{{ID: 1, Assignments: []moodle.Assignment{asg(504, 9504, 1, "Optional", "")}}},
			want:    nil,
		},
		{
			name:   "quiz from calendar only uses activity name",
			events: []moodle.Event{ev("quiz", 77, "2026-10-05 23:59", 2)},
			want: []want{{study.DeadlineKey{Module: "quiz", Instance: 77}, "Event quiz", "2026-10-05 23:59",
				study.FromCalendar, study.SubmissionNotApplicable, "Unix"}},
		},
		{
			name:    "same instance id in different modules is not merged",
			events:  []moodle.Event{ev("quiz", 501, "2026-10-05 23:59", 2)},
			assigns: []moodle.CourseAssignments{{ID: 1, Assignments: []moodle.Assignment{asg(501, 9501, 1, "Homework R1", "2026-10-07 17:15")}}},
			want: []want{
				{study.DeadlineKey{Module: "quiz", Instance: 501}, "Event quiz", "2026-10-05 23:59", study.FromCalendar, study.SubmissionNotApplicable, "Unix"},
				{study.DeadlineKey{Module: "assign", Instance: 501}, "Homework R1", "2026-10-07 17:15", study.FromAssignments, study.SubmissionUnknown, "Programming"},
			},
		},
		{
			name: "teacher grading deadline ignored",
			events: func() []moodle.Event {
				e := ev("assign", 501, "2026-10-14 12:00", 1)
				e.EventType = "gradingdue"
				return []moodle.Event{e}
			}(),
			want: nil,
		},
		{
			name:   "unknown course falls back to event course name",
			events: []moodle.Event{ev("quiz", 5, "2026-10-05 10:00", 99)},
			want: []want{{study.DeadlineKey{Module: "quiz", Instance: 5}, "Event quiz", "2026-10-05 10:00",
				study.FromCalendar, study.SubmissionNotApplicable, "Course 99"}},
		},
		{
			name:   "sorted by due date",
			events: []moodle.Event{ev("quiz", 2, "2026-10-09 10:00", 2), ev("quiz", 1, "2026-10-03 10:00", 2)},
			want: []want{
				{study.DeadlineKey{Module: "quiz", Instance: 1}, "Event quiz", "2026-10-03 10:00", study.FromCalendar, study.SubmissionNotApplicable, "Unix"},
				{study.DeadlineKey{Module: "quiz", Instance: 2}, "Event quiz", "2026-10-09 10:00", study.FromCalendar, study.SubmissionNotApplicable, "Unix"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := study.MergeDeadlines(tt.events, tt.assigns, refs, base)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d deadlines, want %d: %+v", len(got), len(tt.want), got)
			}
			for i, w := range tt.want {
				g := got[i]
				if g.Key != w.key || g.Title != w.title || !g.Due.Equal(vie(w.due)) || g.Sources != w.sources ||
					g.Status != w.status || g.Course.Label != w.course || g.URL == "" {
					t.Errorf("[%d] got %+v\nwant %+v", i, g, w)
				}
			}
		})
	}
}

func TestSubmissionState(t *testing.T) {
	t.Parallel()
	mk := func(status string, graded bool, team *moodle.Submission, ext string) *moodle.SubmissionStatus {
		st := &moodle.SubmissionStatus{}
		st.LastAttempt = &struct {
			Submission       *moodle.Submission `json:"submission"`
			TeamSubmission   *moodle.Submission `json:"teamsubmission"`
			Graded           moodle.Bool        `json:"graded"`
			ExtensionDueDate moodle.Unix        `json:"extensionduedate"`
			GradingStatus    string             `json:"gradingstatus"`
		}{Graded: moodle.Bool(graded), TeamSubmission: team}
		if status != "-" {
			st.LastAttempt.Submission = &moodle.Submission{Status: status}
		}
		if ext != "" {
			st.LastAttempt.ExtensionDueDate = unix(ext)
		}
		return st
	}
	tests := []struct {
		name    string
		in      *moodle.SubmissionStatus
		want    study.Submission
		wantExt string
	}{
		{"nil", nil, study.SubmissionUnknown, ""},
		{"no last attempt", &moodle.SubmissionStatus{}, study.SubmissionUnknown, ""},
		{"no submission yet", mk("-", false, nil, ""), study.SubmissionNotSubmitted, ""},
		{"new", mk("new", false, nil, ""), study.SubmissionNotSubmitted, ""},
		{"draft", mk("draft", false, nil, ""), study.SubmissionDraft, ""},
		{"submitted", mk("submitted", false, nil, ""), study.SubmissionSubmitted, ""},
		{"graded", mk("submitted", true, nil, ""), study.SubmissionGraded, ""},
		{"reopened", mk("reopened", false, nil, ""), study.SubmissionReopened, ""},
		{"team submission wins", mk("new", false, &moodle.Submission{Status: "submitted"}, ""), study.SubmissionSubmitted, ""},
		{"extension returned", mk("new", false, nil, "2026-10-12 23:59"), study.SubmissionNotSubmitted, "2026-10-12 23:59"},
		{"unknown status", mk("weird", false, nil, ""), study.SubmissionUnknown, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ext := study.SubmissionState(tt.in)
			if got != tt.want {
				t.Errorf("state = %v, want %v", got, tt.want)
			}
			if (tt.wantExt == "") != ext.IsZero() || (tt.wantExt != "" && !ext.Equal(vie(tt.wantExt))) {
				t.Errorf("ext = %v, want %q", ext, tt.wantExt)
			}
		})
	}
	// Fixtures decode into the expected states.
	for sc, want := range map[string]study.Submission{
		"synthetic-submitted": study.SubmissionSubmitted,
		"synthetic-draft":     study.SubmissionDraft,
		"synthetic-new":       study.SubmissionNotSubmitted,
	} {
		st := loadFixture[moodle.SubmissionStatus](t, "mod_assign_get_submission_status", sc)
		if got, _ := study.SubmissionState(&st); got != want {
			t.Errorf("%s: %v, want %v", sc, got, want)
		}
	}
}

func TestBuildTree_RealCourse(t *testing.T) {
	t.Parallel()
	secs := loadFixture[[]moodle.Section](t, "core_course_get_contents", "real-12308")
	tree := study.BuildTree(secs, base)
	if len(tree) != 2 || tree[0].Name != "Allgemeines" || tree[1].Name != "R1" {
		t.Fatalf("sections: %+v", tree)
	}
	var gitlab, syllabus *study.Item
	for i, it := range tree[0].Items {
		switch it.Name {
		case "GitLab IT+":
			gitlab = &tree[0].Items[i]
		case "Syllabus_DAT26_PDP":
			syllabus = &tree[0].Items[i]
		}
	}
	if gitlab == nil || gitlab.Kind != "url" || gitlab.Link != "https://gitlab.itplus.fh-joanneum.at/" ||
		!strings.HasPrefix(gitlab.Text, "Course materials will be provided") || strings.Contains(gitlab.Text, "<") {
		t.Errorf("url module: %+v", gitlab)
	}
	if syllabus == nil || len(syllabus.Files) != 1 {
		t.Fatalf("resource: %+v", syllabus)
	}
	f := syllabus.Files[0]
	if f.Name != "Syllabus_DAT26_PDP.pdf" || f.Size != 227077 ||
		f.URL != "https://moodle.fh-joanneum.at/pluginfile.php/318093/mod_resource/content/2/Syllabus_DAT26_PDP.pdf" {
		t.Errorf("file = %+v", f)
	}
}

func TestBuildTree_Subsections(t *testing.T) {
	t.Parallel()
	secs := loadFixture[[]moodle.Section](t, "core_course_get_contents", "real-12326-subsections")
	tree := study.BuildTree(secs, base)
	for _, s := range tree {
		if s.Name == "Learning The Shell - Part 1" {
			t.Fatal("delegated section must not appear at the top level")
		}
	}
	var unix *study.Section
	for i := range tree {
		if tree[i].Name == "Unix Shells" {
			unix = &tree[i]
		}
	}
	if unix == nil || len(unix.Items) != 1 {
		t.Fatalf("Unix Shells section: %+v", unix)
	}
	sub := unix.Items[0]
	if sub.Kind != "subsection" || sub.Sub == nil || len(sub.Sub.Items) != 2 || sub.Sub.Items[0].Kind != "lti" {
		t.Fatalf("subsection not nested: %+v", sub)
	}
}

func TestBuildTree_Synthetic(t *testing.T) {
	t.Parallel()
	mod := func(id int, kind, name string, visible bool) moodle.Module {
		return moodle.Module{ID: id, ModName: kind, Name: name, UserVisible: moodle.Bool(visible), Visible: true}
	}
	label := mod(1, "label", "Wichtig", true)
	label.Description = "<p>Wichtig: <b>Anwesenheit</b> ist Pflicht</p>"
	orphanSub := moodle.Section{ID: 500, Component: "mod_subsection", ItemID: 77, Name: "Orphan",
		Modules: []moodle.Module{mod(10, "resource", "Orphan file", true)}}
	linkedByInstance := mod(3, "subsection", "By instance", true)
	linkedByInstance.Instance = 88
	in := []moodle.Section{
		{ID: 1, Number: 0, Name: "General", Modules: []moodle.Module{label, mod(2, "assign", "Hidden task", false), linkedByInstance}},
		{ID: 2, Number: 1, Name: "Empty"},
		{ID: 3, Number: 2, Name: "Only hidden", Modules: []moodle.Module{mod(4, "quiz", "Secret", false)}},
		{ID: 600, Component: "mod_subsection", ItemID: 88, Name: "By instance", Modules: []moodle.Module{mod(11, "page", "Page", true)}},
		orphanSub,
	}
	tree := study.BuildTree(in, base)
	names := []string{}
	for _, s := range tree {
		names = append(names, s.Name)
	}
	if strings.Join(names, "|") != "General|Only hidden|Orphan" {
		t.Fatalf("sections = %v", names)
	}
	g := tree[0]
	if g.Hidden != 1 || len(g.Items) != 2 {
		t.Fatalf("general: %+v", g)
	}
	if g.Items[0].Text != "Wichtig: Anwesenheit ist Pflicht" || g.Items[0].URL != "" {
		t.Errorf("label: %+v", g.Items[0])
	}
	if g.Items[1].Sub == nil || g.Items[1].Sub.Items[0].Name != "Page" {
		t.Errorf("subsection linked by instance/itemid: %+v", g.Items[1])
	}
	if tree[1].Hidden != 1 || len(tree[1].Items) != 0 {
		t.Errorf("only hidden: %+v", tree[1])
	}
}

func TestSearchSections(t *testing.T) {
	t.Parallel()
	secs := study.BuildTree(loadFixture[[]moodle.Section](t, "core_course_get_contents", "real-12308"), base)
	course := study.CourseRef{ID: 12308, Label: "PDP"}
	tests := []struct {
		query     string
		wantFirst string
		wantN     int
	}{
		{"gitlab", "GitLab IT+", 1},
		{"GITLAB", "GitLab IT+", 1},
		{"syllabus", "Syllabus_DAT26_PDP", 1},
		{"linux git", "Instruction on Git and the Linux Server", 1},
		{"ssh_instructions", "Instruction on Git and the Linux Server", 1}, // file name
		{"homework", "Homework", 1},
		{"r1", "Lecture", 3},                    // section name: all three items of section R1
		{"materials provided", "GitLab IT+", 1}, // description
		{"nonexistent", "", 0},
		{"gitlab nonexistent", "", 0}, // every word must match
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			t.Parallel()
			hits := study.SearchSections(course, secs, tt.query)
			if len(hits) != tt.wantN {
				t.Fatalf("hits = %d, want %d: %+v", len(hits), tt.wantN, hits)
			}
			if tt.wantN > 0 && hits[0].Item.Name != tt.wantFirst {
				t.Errorf("first = %q, want %q", hits[0].Item.Name, tt.wantFirst)
			}
		})
	}
}

func TestBuildGrades(t *testing.T) {
	t.Parallel()
	resp := loadFixture[struct {
		UserGrades []struct {
			GradeItems []moodle.GradeItem `json:"gradeitems"`
		} `json:"usergrades"`
	}](t, "gradereport_user_get_grade_items", "synthetic")
	cg := study.BuildGrades(study.CourseRef{ID: 12308, Label: "PDP"}, resp.UserGrades[0].GradeItems, base)
	if len(cg.Items) != 1 || cg.Pending != 1 || cg.Total == nil {
		t.Fatalf("grades = %+v", cg)
	}
	g := cg.Items[0]
	if g.Name != "Homework R1" || g.Display != "87,50" || g.Max != 100 || g.Percent == nil || *g.Percent != 87.5 ||
		g.Feedback != "Gute Arbeit, aber Plots beschriften." || g.URL != base+"/mod/assign/view.php?id=190501" {
		t.Errorf("item = %+v", g)
	}

	empty := loadFixture[struct {
		UserGrades []struct {
			GradeItems []moodle.GradeItem `json:"gradeitems"`
		} `json:"usergrades"`
	}](t, "gradereport_user_get_grade_items", "real-empty")
	if cg := study.BuildGrades(study.CourseRef{}, empty.UserGrades[0].GradeItems, base); !cg.Empty() {
		t.Errorf("real empty course must be empty: %+v", cg)
	}

	raw := 7.0
	scaled := study.BuildGrades(study.CourseRef{}, []moodle.GradeItem{
		{ItemName: "Hidden", ItemType: "mod", GradeRaw: &raw, GradeMax: 10, GradeIsHidden: true},
		{ItemName: "Scale", ItemType: "mod", GradeFormatted: "Bestanden", GradeMax: 0},
		{ItemName: "Offset", ItemType: "manual", GradeRaw: &raw, GradeMin: 5, GradeMax: 9, GradeFormatted: "7,00"},
		{ItemName: "Feedback only", ItemType: "mod", GradeFormatted: "-", Feedback: "<p>Bitte nachreichen</p>"},
		{ItemName: "Category", ItemType: "category", GradeRaw: &raw, GradeMax: 10},
	}, base)
	if len(scaled.Items) != 3 || scaled.Total != nil {
		t.Fatalf("items = %+v", scaled.Items)
	}
	if scaled.Items[0].Percent != nil || scaled.Items[0].Display != "Bestanden" {
		t.Errorf("scale: %+v", scaled.Items[0])
	}
	if p := scaled.Items[1].Percent; p == nil || *p != 50 {
		t.Errorf("percent must respect grademin: %v", p)
	}
	if scaled.Items[2].Display != "" || scaled.Items[2].Feedback != "Bitte nachreichen" {
		t.Errorf("feedback only: %+v", scaled.Items[2])
	}
}

func TestToAnnouncement(t *testing.T) {
	t.Parallel()
	resp := loadFixture[struct {
		Discussions []moodle.Discussion `json:"discussions"`
	}](t, "mod_forum_get_forum_discussions", "synthetic")
	a := study.ToAnnouncement(study.CourseRef{Label: "Unix"}, resp.Discussions[0], base)
	if a.Title != "Raumänderung morgen" || a.Author != "Erika Mustermann" || !a.Unread || !a.Edited.IsZero() ||
		a.Text != "Die Vorlesung findet morgen in Raum 0.12 statt. Bitte Laptop mitbringen!" ||
		a.URL != base+"/mod/forum/discuss.php?d=7001" || !a.Posted.Equal(vie("2026-09-30 09:30")) {
		t.Errorf("announcement = %+v", a)
	}
	d := resp.Discussions[0]
	d.Modified = unix("2026-10-01 08:00")
	if a := study.ToAnnouncement(study.CourseRef{}, d, base); !a.Edited.Equal(vie("2026-10-01 08:00")) {
		t.Errorf("edited = %v", a.Edited)
	}
}

func TestMatchFile(t *testing.T) {
	t.Parallel()
	doc := []extract.Part{
		{Label: "p. 1", Text: "WORKING ON A DAT VM\nHost ca-crs-dat-NN\nNetwork on campus: directly"},
		{Label: "p. 2", Text: "Username your short FH username\noffcampus: FH VPN required\nRDP"},
		{Label: "p. 3", Text: "Windows: Remote Desktop Connection\nmacOS: Windows App — App Store\nVPN client: Ivanti"},
		{Label: "p. 4", Text: "The lab PC is di ff erent from your laptop"},
		{Label: "p. 5", Text: "A class hierarchy"},
	}
	tests := []struct {
		name        string
		query       string
		ok          bool
		wantLabels  []string
		wantSnippet string
	}{
		{"single word on two pages", "vpn", true, []string{"p. 2", "p. 3"}, "offcampus: FH VPN required"},
		{"pdf lost the space", "off campus", true, []string{"p. 2"}, "offcampus"},
		{"pdf invented spaces", "different", true, []string{"p. 4"}, "di ff erent"},
		{"case and punctuation", "Remote-Desktop", true, []string{"p. 3"}, "Remote Desktop Connection"},
		{"words on different pages", "campus ivanti", true, []string{"p. 1", "p. 2", "p. 3"}, ""},
		{"only pages with every word are listed", "vpn required", true, []string{"p. 2"}, "VPN required"},
		{"missing word", "vpn kubernetes", false, nil, ""},
		{"short word must not match across words", "ssh", false, nil, ""},
		{"empty query", "  ", false, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h, ok := study.MatchFile(doc, tt.query)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if !ok {
				return
			}
			if strings.Join(h.Labels, "|") != strings.Join(tt.wantLabels, "|") {
				t.Errorf("labels = %v, want %v", h.Labels, tt.wantLabels)
			}
			if !strings.Contains(h.Snippet, tt.wantSnippet) {
				t.Errorf("snippet = %q, want it to contain %q", h.Snippet, tt.wantSnippet)
			}
		})
	}
	h, _ := study.MatchFile([]extract.Part{{Text: "a"}, {Label: "1", Text: "x"}, {Label: "2", Text: "x"},
		{Label: "3", Text: "x"}, {Label: "4", Text: "x"}, {Label: "5", Text: "x"}}, "x")
	if len(h.Labels) != 3 || h.More != 2 {
		t.Errorf("labels capped at 3 with More=2, got %v more=%d", h.Labels, h.More)
	}
}
