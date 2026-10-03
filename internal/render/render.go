package render

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/armenbarseghyan/moodle-mcp/internal/study"
	"github.com/armenbarseghyan/moodle-mcp/internal/textfmt"
)

const (
	maxItemText         = 200
	maxAnnouncementText = 1500
	maxFeedbackText     = 500
)

// itoa keeps identifiers out of the printer's number localisation ("12,308").
func itoa(n int) string { return strconv.Itoa(n) }

func tr(format string, args ...any) string { return textfmt.P().Sprintf(format, args...) }

// Courses renders moodle_courses.
func Courses(r study.CoursesResult, now time.Time) string {
	d := newDoc()
	d.line("## %s", tr("%d active courses", len(r.Active)))
	if r.Prefix != "" {
		d.line("_The common prefix %s is left out of the names._", "“"+strings.TrimSpace(r.Prefix)+"”")
	}
	d.blank()
	for _, c := range r.Active {
		d.line("- %s", courseLine(c))
	}
	if len(r.Active) == 0 {
		d.line("No active courses.")
	}
	if len(r.Past) > 0 {
		d.blank()
		d.line("### Past and hidden")
		for _, c := range r.Past {
			d.line("- %s", courseLine(c))
		}
	}
	return d.finish(now, r.FetchedAt)
}

func courseLine(c study.Course) string {
	parts := []string{"**" + link(c.Label, c.URL) + "**", "id " + itoa(c.ID)}
	if c.ShortName != "" && c.ShortName != c.Label {
		parts = append(parts, c.ShortName)
	}
	switch {
	case !c.Start.IsZero() && !c.End.IsZero():
		parts = append(parts, textfmt.Day(c.Start)+" – "+textfmt.Day(c.End))
	case !c.Start.IsZero():
		parts = append(parts, tr("since %s", textfmt.Day(c.Start)))
	}
	if c.Progress != nil {
		parts = append(parts, tr("progress %s%%", fmt.Sprintf("%.0f", *c.Progress)))
	}
	return strings.Join(parts, " · ")
}

// Deadlines renders moodle_deadlines.
func Deadlines(r study.DeadlinesResult, now time.Time) string {
	d := newDoc()
	last := r.Until.Add(-time.Second)
	d.line("## Deadlines until %s (%s)", textfmt.Day(last), tr("%d days", r.Days))
	if len(r.Overdue) > 0 {
		d.blank()
		d.line("### %s", tr("Overdue — not submitted"))
		for _, x := range r.Overdue {
			deadlineItem(d, x, now, true)
		}
	}
	d.blank()
	if len(r.Upcoming) == 0 {
		d.line("No deadlines in this period.")
	} else if len(r.Overdue) > 0 {
		d.line("### %s", tr("Coming up"))
	}
	for _, x := range r.Upcoming {
		deadlineItem(d, x, now, false)
	}
	if r.Hidden > 0 || len(r.Warnings) > 0 {
		d.blank()
	}
	if r.Hidden > 0 {
		d.line("_Hidden or not yet available: %s._", tr("%d assignments", r.Hidden))
	}
	for _, w := range r.Warnings {
		d.line("⚠ %s", w)
	}
	courseErrors(d, r.Errors)
	return d.finish(now, r.FetchedAt)
}

// urgentWithin marks open work due this soon with ⏰.
const urgentWithin = 48 * time.Hour

// deadlineItem writes two lines: what (title, course, kind), then when and
// the status, so a long list stays scannable.
func deadlineItem(d *doc, x study.Deadline, now time.Time, overdue bool) {
	title := "**" + link(x.Title, x.URL) + "**"
	open := x.Status == study.SubmissionNotSubmitted || x.Status == study.SubmissionDraft ||
		x.Status == study.SubmissionReopened || x.Status == study.SubmissionInProgress
	if !overdue && open && x.Due.Sub(now) < urgentWithin {
		title = "⏰ " + title
	}
	d.line("- %s · %s · %s", title, x.Course.Label, kindLabel(x.Key.Module))
	due := tr("due %s", when(x.Due, now))
	if overdue {
		due = tr("was due %s", when(x.Due, now))
	}
	if s := statusLabel(x.Key.Module, x.Status, overdue); s != "" {
		due += " · " + s
	}
	d.line("  %s", due)
}

// statusLabel names the submission state. Before the deadline, "not submitted"
// is a neutral to-do (⬜); only overdue work gets the red ❌.
func statusLabel(module string, s study.Submission, overdue bool) string {
	if s == study.SubmissionNotSubmitted && !overdue {
		if module == "quiz" {
			return tr("⬜ not attempted yet")
		}
		return tr("⬜ not submitted yet")
	}
	if module == "quiz" {
		switch s {
		case study.SubmissionNotSubmitted:
			return tr("❌ not attempted")
		case study.SubmissionInProgress:
			return tr("⏳ attempt in progress")
		case study.SubmissionSubmitted:
			return tr("✅ finished")
		}
	}
	switch s {
	case study.SubmissionNotSubmitted:
		return tr("❌ not submitted")
	case study.SubmissionDraft:
		return tr("📝 draft, not submitted")
	case study.SubmissionSubmitted:
		return tr("✅ submitted")
	case study.SubmissionGraded:
		return tr("✅ submitted, graded")
	case study.SubmissionReopened:
		return tr("↩️ reopened, submit again")
	case study.SubmissionInProgress:
		return tr("⏳ in progress")
	case study.SubmissionUnknown:
		return tr("status unknown")
	default:
		return ""
	}
}

// Contents renders moodle_course_contents.
func Contents(r study.ContentsResult, now time.Time) string {
	d := newDoc()
	d.line("## %s (id %s)", link(r.Course.Label, r.Course.URL), itoa(r.Course.ID))
	if len(r.Sections) == 0 {
		d.blank()
		d.line("The course has no visible material.")
	}
	for _, s := range r.Sections {
		d.blank()
		writeSection(d, s, 0)
	}
	return d.finish(now, r.FetchedAt)
}

func writeSection(d *doc, s study.Section, depth int) {
	indent := strings.Repeat("  ", depth)
	if depth == 0 {
		name := s.Name
		if name == "" {
			name = tr("Untitled")
		}
		d.line("### %s", name)
	}
	if s.Summary != "" {
		d.line("%s%s", indent, textfmt.Truncate(textfmt.OneLine(s.Summary), maxItemText))
	}
	for _, it := range s.Items {
		writeItem(d, it, indent)
		if it.Sub != nil {
			writeSection(d, *it.Sub, depth+1)
		}
	}
	if s.Hidden > 0 {
		d.line("%s_%s_", indent, tr("hidden or not available: %d", s.Hidden))
	}
}

func writeItem(d *doc, it study.Item, indent string) {
	if it.Kind == "label" {
		if it.Text != "" {
			d.line("%s- %s", indent, textfmt.Truncate(textfmt.OneLine(it.Text), maxItemText))
		}
		return
	}
	if it.Kind == "subsection" {
		d.line("%s- **%s**", indent, it.Name)
		return
	}
	parts := []string{link(it.Name, it.URL), kindLabel(it.Kind)}
	if it.Link != "" {
		parts = append(parts, "→ "+it.Link)
	}
	if len(it.Files) == 1 {
		parts = append(parts, fileLink(it.Files[0]))
	}
	d.line("%s- %s", indent, strings.Join(parts, " · "))
	if len(it.Files) > 1 {
		for _, f := range it.Files {
			d.line("%s  - %s", indent, fileLink(f))
		}
	}
	if it.Text != "" && it.Text != it.Name {
		d.line("%s  %s", indent, textfmt.Truncate(textfmt.OneLine(it.Text), maxItemText))
	}
}

func fileLink(f study.File) string {
	s := link(f.Name, f.URL)
	if sz := Size(f.Size); sz != "" {
		s += " (" + sz + ")"
	}
	return s
}

// Search renders moodle_search.
func Search(r study.SearchResult, now time.Time) string {
	d := newDoc()
	scope := ""
	if r.Scope != nil {
		scope = tr(" in %s", r.Scope.Label)
	}
	total := r.Total + r.FileTotal
	d.line("## Search “%s”%s: %s", r.Query, scope, tr("%d matches", total))
	if r.Total > 0 {
		d.blank()
		if r.InFiles {
			d.line("### In names and descriptions")
		}
		for _, h := range r.Hits {
			it := h.Item
			parts := []string{link(it.Name, it.URL), kindLabel(it.Kind), place(h.Course, h.Path, r.Scope)}
			if it.Link != "" {
				parts = append(parts, "→ "+it.Link)
			}
			if len(it.Files) == 1 {
				parts = append(parts, fileLink(it.Files[0]))
			}
			d.line("- %s", strings.Join(parts, " · "))
			if len(it.Files) > 1 {
				for _, f := range it.Files {
					d.line("  - %s", fileLink(f))
				}
			}
		}
		if r.Total > len(r.Hits) {
			d.line("_Showing the first %d of %d — narrow the query._", len(r.Hits), r.Total)
		}
	}
	if r.InFiles {
		d.blank()
		d.line("### Inside files")
		if r.FileTotal == 0 {
			d.line("No matches in the text of files.")
		}
		for _, h := range r.FileHits {
			parts := []string{link(h.File.Name, h.File.URL)}
			if len(h.Labels) > 0 {
				l := strings.Join(h.Labels, ", ")
				if h.More > 0 {
					l += tr(" and %d more", h.More)
				}
				parts = append(parts, l)
			}
			parts = append(parts, place(h.Course, h.Path, r.Scope)+" › "+link(h.Item.Name, h.Item.URL))
			d.line("- %s", strings.Join(parts, " · "))
			if h.Snippet != "" {
				d.line("  > %s", h.Snippet)
			}
		}
		if r.FileTotal > len(r.FileHits) {
			d.line("_Showing the first %d of %d files — narrow the query._", len(r.FileHits), r.FileTotal)
		}
		d.blank()
		note := tr("Files searched: %d", r.Indexed)
		if r.Skipped > 0 {
			note += tr("; skipped (video, images, too large): %d", r.Skipped)
		}
		if r.Failed > 0 {
			note += tr("; could not be read: %d", r.Failed)
		}
		d.line("_%s._", note)
	}
	if total == 0 && !r.InFiles {
		d.blank()
		d.line("Nothing found in names, descriptions and file names. " +
			"To search inside PDFs and documents too, use in_files=true.")
	}
	courseErrors(d, r.Errors)
	return d.finish(now, r.FetchedAt)
}

func place(c study.CourseRef, path []string, scope *study.CourseRef) string {
	p := strings.Join(path, " › ")
	if scope != nil {
		return p
	}
	return c.Label + " › " + p
}

// Grades renders moodle_grades.
func Grades(r study.GradesResult, now time.Time) string {
	d := newDoc()
	d.line("## Grades")
	var empty []string
	shown := 0
	for _, cg := range r.Courses {
		if cg.Empty() {
			empty = append(empty, cg.Course.Label)
			continue
		}
		shown++
		d.blank()
		d.line("### %s", cg.Course.Label)
		for _, g := range cg.Items {
			d.line("- %s", gradeLine(g))
			if g.Feedback != "" {
				d.line("  > %s", textfmt.Truncate(textfmt.OneLine(g.Feedback), maxFeedbackText))
			}
		}
		if cg.Pending > 0 {
			d.line("- ⏳ %s", tr("%d items not graded yet", cg.Pending))
		}
		if cg.Total != nil {
			d.line("- %s", tr("Course total: %s", gradeValue(*cg.Total)))
		}
	}
	if len(empty) > 0 {
		d.blank()
		if shown == 0 && len(empty) == 1 {
			d.line("%s: no grades yet.", empty[0])
		} else {
			d.line("No grades yet: %s.", strings.Join(empty, ", "))
		}
	}
	courseErrors(d, r.Errors)
	return d.finish(now, time.Time{})
}

func gradeLine(g study.Grade) string {
	name := g.Name
	if name == "" {
		name = tr("Untitled")
	}
	parts := []string{link(name, g.URL) + ": " + gradeValue(g)}
	if !g.GradedAt.IsZero() {
		parts = append(parts, tr("graded %s", textfmt.Day(g.GradedAt)))
	}
	return strings.Join(parts, " · ")
}

func gradeValue(g study.Grade) string {
	if g.Display == "" {
		return tr("not graded")
	}
	s := "**" + plainNumber(g.Display) + "**"
	if g.Max > 0 && g.Percent != nil {
		s += fmt.Sprintf(" / %s (%s%%)", trimFloat(g.Max), trimFloat(*g.Percent))
	}
	return s
}

// germanNumber is a decimal written with a comma, as this site formats grades.
var germanNumber = regexp.MustCompile(`^-?\d+,\d+$`)

// plainNumber writes Moodle's "87,50" as "87.5", matching the maximum and the
// percentage next to it. Letters, scales and other formats stay as they are.
func plainNumber(display string) string {
	if !germanNumber.MatchString(display) {
		return display
	}
	f, err := strconv.ParseFloat(strings.Replace(display, ",", ".", 1), 64)
	if err != nil {
		return display
	}
	return trimFloat(f)
}

// trimFloat renders 100 as "100" and 87.5 as "87.5". Moodle's own formatted
// grade (g.Display) keeps the site's notation.
func trimFloat(f float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", f), "0"), ".")
}

// Announcements renders moodle_announcements.
func Announcements(r study.AnnouncementsResult, now time.Time) string {
	d := newDoc()
	d.line("## Announcements, last %s: %d", tr("%d days", r.Days), len(r.Items))
	if len(r.Items) == 0 {
		d.blank()
		d.line("No new announcements.")
	}
	for _, a := range r.Items {
		d.blank()
		title := a.Title
		if a.Pinned {
			title = "📌 " + title
		}
		d.line("### %s", title)
		meta := []string{a.Course.Label}
		if a.Author != "" {
			meta = append(meta, a.Author)
		}
		meta = append(meta, when(a.Posted, now))
		if !a.Edited.IsZero() {
			meta = append(meta, tr("edited %s", textfmt.Relative(a.Edited, now)))
		}
		if a.Unread {
			meta = append(meta, tr("unread"))
		}
		meta = append(meta, link(tr("open"), a.URL))
		d.line("%s", strings.Join(meta, " · "))
		if a.Text != "" {
			d.line("")
			for _, l := range strings.Split(textfmt.Truncate(a.Text, maxAnnouncementText), "\n") {
				d.line("%s", l)
			}
		}
	}
	courseErrors(d, r.Errors)
	return d.finish(now, r.FetchedAt)
}

// Download renders moodle_download.
func Download(r study.DownloadResult) string {
	verb := tr("Downloaded")
	if r.Reused {
		verb = tr("Already downloaded (file unchanged)")
	}
	s := fmt.Sprintf("%s: `%s`", verb, r.Path)
	if sz := Size(r.Size); sz != "" {
		s += " (" + sz + ")"
	}
	if r.BrowserURL != "" {
		s += "\n" + tr("In the browser: %s", escapeURL(r.BrowserURL))
	}
	return s + "\n"
}

// WhoAmI renders moodle_whoami.
func WhoAmI(r study.WhoAmIResult, version string) string {
	d := newDoc()
	d.line("## %s", r.SiteName)
	d.line("- User: %s (%s), id %s", r.FullName, r.Username, itoa(r.UserID))
	d.line("- Site: %s · Moodle %s · language %s", r.SiteURL, r.Release, r.Lang)
	if len(r.Missing) == 0 {
		d.line("- Functions available: %d; all %d the server needs are there", r.Functions, len(r.Required))
	} else {
		d.line("- Functions available: %d; **%d of %d missing**: %s", r.Functions, len(r.Missing),
			len(r.Required), strings.Join(r.Missing, ", "))
	}
	if r.DownloadFiles {
		d.line("- File download via the API: allowed")
	} else {
		d.line("- File download via the API: **disabled** on the site")
	}
	if version != "" {
		d.line("- moodle-mcp %s", version)
	}
	return d.finish(time.Time{}, time.Time{})
}

// Ambiguous renders a course query that matched several courses.
func Ambiguous(e *study.AmbiguousError) string {
	d := newDoc()
	d.line("Several courses match “%s” — use a more specific name or the id:", e.Query)
	for _, c := range e.Candidates {
		past := ""
		if c.Past {
			past = " · " + tr("past")
		}
		d.line("- %s · id %s%s", c.Label, itoa(c.ID), past)
	}
	return d.finish(time.Time{}, time.Time{})
}

// NotFound renders a course query that matched nothing.
func NotFound(e *study.NotFoundError) string {
	d := newDoc()
	d.line("No course matches “%s”.", e.Query)
	if len(e.Available) > 0 {
		d.line("Active courses:")
		for _, c := range e.Available {
			d.line("- %s · id %s", c.Label, itoa(c.ID))
		}
	}
	return d.finish(time.Time{}, time.Time{})
}

// WhatsNew renders moodle_whats_new: materials grouped by course, then
// announcements and grades, each in one line so the whole week fits a glance.
func WhatsNew(r study.WhatsNewResult, now time.Time) string {
	d := newDoc()
	d.line("## What's new in the last %s", tr("%d days", r.Days))
	if len(r.Materials)+len(r.Announcements)+len(r.Graded) == 0 {
		d.blank()
		d.line("Nothing new in your courses in this period.")
	}

	if len(r.Materials) > 0 {
		d.blank()
		d.line("### 📄 %s", tr("%d new or updated files", len(r.Materials)))
		var order []string
		byCourse := map[string][]study.NewMaterial{}
		for _, m := range r.Materials {
			if _, ok := byCourse[m.Course.Label]; !ok {
				order = append(order, m.Course.Label)
			}
			byCourse[m.Course.Label] = append(byCourse[m.Course.Label], m)
		}
		for _, course := range order {
			d.line("**%s**", course)
			for _, m := range byCourse[course] {
				mark := tr("🆕 new")
				if !m.Added {
					mark = tr("✏️ updated")
				}
				where := strings.Join(m.Path, " › ")
				if m.Item.Name != "" && m.Item.URL != "" {
					where += " › " + link(m.Item.Name, m.Item.URL)
				}
				// A server clock slightly ahead must not make an upload "in 1 h".
				d.line("- %s %s · %s · %s", mark, link(m.File.Name, m.File.URL), where, textfmt.Relative(earliest(m.When(), now), now))
			}
		}
	}

	if len(r.Announcements) > 0 {
		d.blank()
		d.line("### 📣 %s", tr("%d announcements", len(r.Announcements)))
		for _, a := range r.Announcements {
			title := link(a.Title, a.URL)
			if a.Unread {
				title = "**" + title + "** · " + tr("unread")
			}
			d.line("- %s · %s · %s", title, a.Course.Label, textfmt.Relative(a.Changed(), now))
		}
		d.line("_Full text: moodle_announcements._")
	}

	if len(r.Graded) > 0 {
		d.blank()
		d.line("### 🎓 %s", tr("%d new grades", len(r.Graded)))
		for _, g := range r.Graded {
			d.line("- %s · %s", gradeLine(g.Grade), g.Course.Label)
		}
	}
	courseErrors(d, r.Errors)
	return d.finish(now, r.FetchedAt)
}

func earliest(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
