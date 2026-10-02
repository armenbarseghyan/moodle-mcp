package moodletest

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// SiteURL is the host of the real site in the recorded fixtures. World
// rewrites it to the fake server's own URL.
const SiteURL = "https://moodle.fh-joanneum.at"

// World is a small, consistent fake semester built from the fixtures:
// courses 12308 (Programming and Data Processing), 12326 (Unix Shells and
// LaTeX, with 4.5 subsections) and 12307 (empty); deadlines with an overdue,
// a submitted, an unsubmitted and a draft assignment plus a calendar-only
// quiz; one recent announcement; one graded assignment with feedback; and
// sample files behind every pluginfile URL.
//
// extra options are applied last and override routes of the world.
func World(r Reporter, extra ...Option) []Option {
	statusByAssign := map[string]string{
		"501": "synthetic-submitted", "502": "synthetic-draft",
		"503": "synthetic-new", "505": "synthetic-new", "506": "synthetic-new",
	}
	opts := []Option{
		RewriteHost(SiteURL),
		Files(SampleFiles(r)),
		Fixture("core_webservice_get_site_info", "real"),
		Fixture("core_enrol_get_users_courses", "real"),
		Route("core_course_get_contents", func(p url.Values) Resp {
			switch p.Get("courseid") {
			case "12308":
				return File(r, "core_course_get_contents", "real-12308")
			case "12326":
				return File(r, "core_course_get_contents", "real-12326-subsections")
			}
			return Raw(200, "[]")
		}),
		Fixture("core_calendar_get_action_events_by_timesort", "synthetic"),
		Fixture("mod_assign_get_assignments", "synthetic"),
		Route("mod_assign_get_submission_status", func(p url.Values) Resp {
			if sc, ok := statusByAssign[p.Get("assignid")]; ok {
				return File(r, "mod_assign_get_submission_status", sc)
			}
			return ErrorFile(r, "requireloginerror")
		}),
		Route("gradereport_user_get_grade_items", func(p url.Values) Resp {
			if p.Get("courseid") == "12308" {
				return File(r, "gradereport_user_get_grade_items", "synthetic")
			}
			return File(r, "gradereport_user_get_grade_items", "real-empty")
		}),
		Fixture("mod_forum_get_forums_by_courses", "real"),
		Route("mod_forum_get_forum_discussions", func(p url.Values) Resp {
			if p.Get("forumid") == "21334" {
				return File(r, "mod_forum_get_forum_discussions", "synthetic")
			}
			return File(r, "mod_forum_get_forum_discussions", "real-empty")
		}),
	}
	return append(opts, extra...)
}

// SampleFiles serves synthetic files for pluginfile URLs: every PDF gets a
// two-page sample, every zip an instructions archive, everything else 404.
func SampleFiles(r Reporter) http.Handler {
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join(TestdataDir(), "files", name))
		if err != nil {
			r.Fatalf("moodletest: sample file: %v", err)
		}
		return b
	}
	pdf, zip := read("sample-two-pages.pdf"), read("sample-instructions.zip")
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Query().Get("token") != Token {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"Invalid token","errorcode":"invalidtoken"}`))
			return
		}
		switch strings.ToLower(filepath.Ext(req.URL.Path)) {
		case ".pdf":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(pdf)
		case ".zip":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(zip)
		default:
			http.NotFound(w, req)
		}
	})
}
