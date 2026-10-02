package moodle_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"testing"
	"time"

	"moodle-mcp/internal/moodle"
	"moodle-mcp/internal/moodletest"
)

// Every fixture must decode into its wire type. This catches drift between
// fixtures and types when either side changes.
func TestFixturesDecode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	calls := map[string]func(c *moodle.Client) error{
		"core_webservice_get_site_info": func(c *moodle.Client) error { _, err := c.SiteInfo(ctx); return err },
		"core_enrol_get_users_courses":  func(c *moodle.Client) error { _, err := c.UserCourses(ctx, 4242); return err },
		"core_course_get_contents":      func(c *moodle.Client) error { _, err := c.CourseContents(ctx, 1); return err },
		"core_calendar_get_action_events_by_timesort": func(c *moodle.Client) error {
			_, err := c.ActionEvents(ctx, time.Unix(0, 0), time.Unix(2e9, 0))
			return err
		},
		"mod_assign_get_assignments":       func(c *moodle.Client) error { _, _, err := c.Assignments(ctx, []int{1}); return err },
		"mod_assign_get_submission_status": func(c *moodle.Client) error { _, err := c.SubmissionStatus(ctx, 1); return err },
		"gradereport_user_get_grade_items": func(c *moodle.Client) error { _, err := c.GradeItems(ctx, 1, 4242); return err },
		"mod_forum_get_forums_by_courses":  func(c *moodle.Client) error { _, err := c.Forums(ctx, []int{1}); return err },
		"mod_forum_get_forum_discussions":  func(c *moodle.Client) error { _, err := c.Discussions(ctx, 1, 10); return err },
		"mod_quiz_get_user_attempts":       func(c *moodle.Client) error { _, err := c.QuizAttempts(ctx, 77); return err },
	}
	for _, fn := range moodle.AllowedFunctions() {
		call, ok := calls[fn]
		if !ok {
			t.Errorf("no decode test for allowlisted function %s", fn)
			continue
		}
		scenarios := moodletest.Fixtures(fn)
		if len(scenarios) == 0 {
			t.Errorf("no fixtures for %s", fn)
		}
		for _, sc := range scenarios {
			t.Run(fn+"/"+sc, func(t *testing.T) {
				t.Parallel()
				srv := moodletest.New(t, moodletest.Fixture(fn, sc))
				if err := call(newClient(t, srv)); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestSiteInfo(t *testing.T) {
	t.Parallel()
	srv := moodletest.New(t, moodletest.Fixture("core_webservice_get_site_info", "real"))
	si, err := newClient(t, srv).SiteInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if si.UserID != 4242 || si.Username != "s00000" || si.Release == "" || !bool(si.DownloadFiles) || len(si.Functions) == 0 {
		t.Errorf("unexpected site info: %+v", si)
	}
}

func TestUserCourses(t *testing.T) {
	t.Parallel()
	srv := moodletest.New(t, moodletest.Fixture("core_enrol_get_users_courses", "real"))
	courses, err := newClient(t, srv).UserCourses(context.Background(), 4242)
	if err != nil {
		t.Fatal(err)
	}
	if got := srv.Requests()[0].Form.Get("userid"); got != "4242" {
		t.Errorf("userid = %q", got)
	}
	if len(courses) != 3 {
		t.Fatalf("courses = %d, want 3", len(courses))
	}
	c := courses[0]
	if c.ID == 0 || c.FullName == "" || !bool(c.Visible) || bool(c.Hidden) || bool(c.Completed) {
		t.Errorf("course = %+v", c)
	}
	if c.StartDate.Time().IsZero() {
		t.Error("startdate must be set")
	}
	if !c.EndDate.Time().IsZero() {
		t.Error("enddate 0 must map to zero time")
	}
	if c.Progress != nil {
		t.Errorf("progress null must stay nil, got %v", *c.Progress)
	}
}

func TestCourseContents(t *testing.T) {
	t.Parallel()
	t.Run("files and urls", func(t *testing.T) {
		t.Parallel()
		srv := moodletest.New(t, moodletest.Fixture("core_course_get_contents", "real-12308"))
		secs, err := newClient(t, srv).CourseContents(context.Background(), 12308)
		if err != nil {
			t.Fatal(err)
		}
		if len(secs) != 2 || secs[0].Number != 0 || secs[1].Name != "R1" {
			t.Fatalf("sections = %+v", secs)
		}
		var files, urls int
		for _, s := range secs {
			for _, m := range s.Modules {
				for _, ct := range m.Contents {
					switch ct.Type {
					case "file":
						files++
						if ct.FileURL == "" || ct.FileSize == 0 {
							t.Errorf("file without url/size: %+v", ct)
						}
					case "url":
						urls++
					}
				}
			}
		}
		if files == 0 || urls == 0 {
			t.Errorf("files=%d urls=%d", files, urls)
		}
	})
	t.Run("subsections", func(t *testing.T) {
		t.Parallel()
		srv := moodletest.New(t, moodletest.Fixture("core_course_get_contents", "real-12326-subsections"))
		secs, err := newClient(t, srv).CourseContents(context.Background(), 12326)
		if err != nil {
			t.Fatal(err)
		}
		var sub *moodle.Module
		var delegated *moodle.Section
		for i := range secs {
			if secs[i].Component == "mod_subsection" {
				delegated = &secs[i]
			}
			for j := range secs[i].Modules {
				if secs[i].Modules[j].ModName == "subsection" {
					sub = &secs[i].Modules[j]
				}
			}
		}
		if sub == nil || delegated == nil {
			t.Fatal("fixture must contain a subsection module and its delegated section")
		}
		var cd struct {
			SectionID string `json:"sectionid"`
		}
		if err := json.Unmarshal([]byte(sub.CustomData), &cd); err != nil {
			t.Fatal(err)
		}
		if cd.SectionID != strconv.Itoa(delegated.ID) || sub.Instance != delegated.ItemID {
			t.Errorf("subsection link broken: customdata=%s instance=%d section id=%d itemid=%d",
				sub.CustomData, sub.Instance, delegated.ID, delegated.ItemID)
		}
	})
}

func TestActionEvents(t *testing.T) {
	t.Parallel()
	srv := moodletest.New(t, moodletest.Fixture("core_calendar_get_action_events_by_timesort", "synthetic"))
	from := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	to := from.Add(30 * 24 * time.Hour)
	evs, err := newClient(t, srv).ActionEvents(context.Background(), from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 3 {
		t.Fatalf("events = %d", len(evs))
	}
	e := evs[1]
	if e.ModuleName != "assign" || e.Instance != 501 || e.Course == nil || e.Course.ID != 12308 ||
		e.URL == "" || e.Action == nil || !bool(e.Action.Actionable) || e.TimeSort.Time().IsZero() {
		t.Errorf("event = %+v", e)
	}
	f := srv.Requests()[0].Form
	if f.Get("timesortfrom") != strconv.FormatInt(from.Unix(), 10) || f.Get("timesortto") != strconv.FormatInt(to.Unix(), 10) ||
		f.Get("limitnum") != "50" || f.Has("aftereventid") {
		t.Errorf("params = %v", f)
	}
}

func TestActionEvents_Pagination(t *testing.T) {
	t.Parallel()
	page := func(first, n int) moodletest.Resp {
		evs := make([]map[string]any, n)
		for i := range n {
			evs[i] = map[string]any{"id": first + i, "name": fmt.Sprint("e", first+i), "modulename": "assign",
				"instance": first + i, "timesort": 1791386100}
		}
		b, _ := json.Marshal(map[string]any{"events": evs, "firstid": first, "lastid": first + n - 1})
		return moodletest.Resp{Body: b}
	}
	srv := moodletest.New(t, moodletest.Route("core_calendar_get_action_events_by_timesort", func(p url.Values) moodletest.Resp {
		switch p.Get("aftereventid") {
		case "":
			return page(1, 50)
		case "50":
			return page(51, 50)
		case "100":
			return page(101, 7)
		}
		t.Errorf("unexpected aftereventid %q", p.Get("aftereventid"))
		return page(0, 0)
	}))
	evs, err := newClient(t, srv).ActionEvents(context.Background(), time.Now(), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 107 || srv.Calls("core_calendar_get_action_events_by_timesort") != 3 {
		t.Fatalf("events = %d, calls = %d", len(evs), srv.Calls("core_calendar_get_action_events_by_timesort"))
	}
}

func TestAssignments(t *testing.T) {
	t.Parallel()
	t.Run("batch call with warnings", func(t *testing.T) {
		t.Parallel()
		srv := moodletest.New(t, moodletest.Fixture("mod_assign_get_assignments", "synthetic"))
		courses, warns, err := newClient(t, srv).Assignments(context.Background(), []int{12308, 12326})
		if err != nil {
			t.Fatal(err)
		}
		f := srv.Requests()[0].Form
		if f.Get("courseids[0]") != "12308" || f.Get("courseids[1]") != "12326" {
			t.Errorf("courseids = %v", f)
		}
		if len(courses) != 2 || len(courses[0].Assignments) != 5 || len(warns) != 1 {
			t.Fatalf("courses=%d warnings=%d", len(courses), len(warns))
		}
		a := courses[0].Assignments[0]
		if a.ID != 501 || a.CMID != 190501 || a.DueDate.Time().IsZero() {
			t.Errorf("assignment = %+v", a)
		}
		if !courses[0].Assignments[2].DueDate.Time().IsZero() {
			t.Error("duedate 0 must be zero time")
		}
	})
	t.Run("no courses means no call", func(t *testing.T) {
		t.Parallel()
		srv := moodletest.New(t)
		c, w, err := newClient(t, srv).Assignments(context.Background(), nil)
		if err != nil || c != nil || w != nil || len(srv.Requests()) != 0 {
			t.Fatalf("got %v %v %v, requests %d", c, w, err, len(srv.Requests()))
		}
	})
}

func TestSubmissionStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		scenario string
		want     string
	}{
		{"synthetic-submitted", moodle.SubmissionSubmitted},
		{"synthetic-draft", moodle.SubmissionDraft},
		{"synthetic-new", moodle.SubmissionNew},
	}
	for _, tt := range tests {
		t.Run(tt.scenario, func(t *testing.T) {
			t.Parallel()
			srv := moodletest.New(t, moodletest.Fixture("mod_assign_get_submission_status", tt.scenario))
			st, err := newClient(t, srv).SubmissionStatus(context.Background(), 501)
			if err != nil {
				t.Fatal(err)
			}
			if srv.Requests()[0].Form.Get("assignid") != "501" {
				t.Error("assignid not sent")
			}
			if st.LastAttempt == nil || st.LastAttempt.Submission == nil || st.LastAttempt.Submission.Status != tt.want {
				t.Fatalf("status = %+v", st.LastAttempt)
			}
		})
	}
}

func TestGradeItems(t *testing.T) {
	t.Parallel()
	srv := moodletest.New(t, moodletest.Fixture("gradereport_user_get_grade_items", "synthetic"))
	items, err := newClient(t, srv).GradeItems(context.Background(), 12308, 4242)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("items = %d", len(items))
	}
	if items[0].GradeRaw == nil || *items[0].GradeRaw != 87.5 || items[0].GradeMax != 100 || items[0].Feedback == "" {
		t.Errorf("graded item = %+v", items[0])
	}
	if items[1].GradeRaw != nil {
		t.Error("ungraded item must have nil graderaw")
	}
	if items[2].ItemType != "course" || items[2].ItemName != "" {
		t.Errorf("course total = %+v", items[2])
	}
}

func TestGradeItems_OtherUserIgnored(t *testing.T) {
	t.Parallel()
	srv := moodletest.New(t, moodletest.Fixture("gradereport_user_get_grade_items", "synthetic"))
	items, err := newClient(t, srv).GradeItems(context.Background(), 12308, 1)
	if err != nil || items != nil {
		t.Fatalf("items = %v, err = %v", items, err)
	}
}

func TestForumsAndDiscussions(t *testing.T) {
	t.Parallel()
	srv := moodletest.New(t,
		moodletest.Fixture("mod_forum_get_forums_by_courses", "real"),
		moodletest.Fixture("mod_forum_get_forum_discussions", "synthetic"),
	)
	c := newClient(t, srv)
	forums, err := c.Forums(context.Background(), []int{12326, 12323})
	if err != nil {
		t.Fatal(err)
	}
	if len(forums) != 2 || forums[0].Type != "news" || forums[0].CMID == 0 {
		t.Fatalf("forums = %+v", forums)
	}
	ds, err := c.Discussions(context.Background(), forums[0].ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	f := srv.Requests()[1].Form
	if f.Get("sortorder") != "3" || f.Get("perpage") != "10" || f.Get("page") != "0" {
		t.Errorf("params = %v", f)
	}
	if len(ds) != 2 || ds[0].Subject == "" || ds[0].Message == "" || ds[0].Created.Time().IsZero() || ds[0].Discussion != 7001 {
		t.Errorf("discussions = %+v", ds)
	}
}

func TestQuizAttempts(t *testing.T) {
	t.Parallel()
	srv := moodletest.New(t, moodletest.Fixture("mod_quiz_get_user_attempts", "synthetic-finished"))
	atts, err := newClient(t, srv).QuizAttempts(context.Background(), 77)
	if err != nil {
		t.Fatal(err)
	}
	f := srv.Requests()[0].Form
	if f.Get("quizid") != "77" || f.Get("status") != "all" {
		t.Errorf("params = %v (status must be all: unfinished attempts matter)", f)
	}
	if len(atts) != 2 || atts[1].State != moodle.QuizFinished || atts[1].SumGrades == nil || atts[1].TimeFinish.Time().IsZero() {
		t.Errorf("attempts = %+v", atts)
	}
}
