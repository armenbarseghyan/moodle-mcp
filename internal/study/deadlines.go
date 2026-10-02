package study

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"moodle-mcp/internal/moodle"
	"moodle-mcp/internal/textfmt"
)

// OverdueLookback is how far back unsubmitted assignments are reported.
const OverdueLookback = 7 * 24 * time.Hour

// DeadlinesResult answers moodle_deadlines.
type DeadlinesResult struct {
	Days      int
	Until     time.Time // exclusive: Vienna midnight after the last day
	Upcoming  []Deadline
	Overdue   []Deadline // unsubmitted assignments due in the last 7 days
	Hidden    int        // assignments Moodle reported as not accessible
	Warnings  []string   // e.g. "calendar unavailable"
	Errors    []CourseError
	FetchedAt time.Time
}

// Deadlines merges calendar action events with assignment due dates for the
// next `days` calendar days (until the end of the last day, Vienna time),
// adds the submission state of every assignment, and optionally reports
// unsubmitted assignments that are overdue by up to 7 days.
func (s *Service) Deadlines(ctx context.Context, days int, includeOverdue, refresh bool) (DeadlinesResult, error) {
	now := s.now()
	res := DeadlinesResult{Days: days, Until: textfmt.DayStart(now, days+1)}
	var stamp oldest

	all, at, err := s.allCourses(ctx, refresh)
	if err != nil {
		return res, err
	}
	stamp.add(at)
	courses := active(all)
	refs := refsByID(all)

	// Day-aligned bounds keep the cache key stable within a day.
	from := textfmt.DayStart(now.Add(-OverdueLookback), 0)
	var (
		events     []moodle.Event
		assigns    Assignments
		eventsErr  error
		assignsErr error
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		f, err := s.src.Events(gctx, from, res.Until, refresh)
		if err != nil {
			if moodle.IsFatal(err) {
				return err
			}
			eventsErr = err
			return nil
		}
		events = f.Value
		stamp.add(f.At)
		return nil
	})
	g.Go(func() error {
		f, err := s.src.Assignments(gctx, ids(courses), refresh)
		if err != nil {
			if moodle.IsFatal(err) {
				return err
			}
			assignsErr = err
			return nil
		}
		assigns = f.Value
		stamp.add(f.At)
		return nil
	})
	if err := g.Wait(); err != nil {
		return res, err
	}
	if eventsErr != nil && assignsErr != nil {
		return res, errors.Join(eventsErr, assignsErr)
	}
	if eventsErr != nil {
		res.Warnings = append(res.Warnings, textfmt.P().Sprintf("calendar unavailable: %s", eventsErr.Error()))
	}
	if assignsErr != nil {
		res.Warnings = append(res.Warnings, textfmt.P().Sprintf("assignment list unavailable, submission status unknown: %s", assignsErr.Error()))
	}
	for _, w := range assigns.Warnings {
		if w.Item == "module" || w.Item == "course" {
			res.Hidden++
		}
	}

	merged := MergeDeadlines(events, assigns.Courses, refs, s.base())
	lookbackStart := now.Add(-OverdueLookback)
	candidates := slices.DeleteFunc(merged, func(d Deadline) bool {
		return d.Due.Before(lookbackStart) || !d.Due.Before(res.Until)
	})

	// Submission state for every assignment and quiz candidate.
	type state struct {
		status Submission
		ext    time.Time
		at     time.Time
	}
	var idx []int
	for i, d := range candidates {
		if d.Key.Module == "assign" || d.Key.Module == "quiz" {
			idx = append(idx, i)
		}
	}
	states, errs, err := fanOut(ctx, idx, func(ctx context.Context, i int) (state, error) {
		d := candidates[i]
		if d.Key.Module == "quiz" {
			f, err := s.src.QuizAttempts(ctx, d.Key.Instance, refresh)
			return state{status: QuizState(f.Value), at: f.At}, err
		}
		f, err := s.src.SubmissionStatus(ctx, d.Key.Instance, refresh)
		if err != nil {
			return state{}, err
		}
		st, ext := SubmissionState(f.Value)
		return state{status: st, ext: ext, at: f.At}, nil
	})
	if err != nil {
		return res, err
	}
	for j, i := range idx {
		if errs[j] != nil {
			candidates[i].Status = SubmissionUnknown
			s.log.Warn("submission status", "module", candidates[i].Key.Module, "instance", candidates[i].Key.Instance, "err", errs[j])
			continue
		}
		stamp.add(states[j].at)
		candidates[i].Status = states[j].status
		if !states[j].ext.IsZero() {
			candidates[i].Due = states[j].ext // an extension overrides every other date
		}
	}

	for _, d := range candidates {
		switch {
		case !d.Due.Before(now) && d.Due.Before(res.Until):
			res.Upcoming = append(res.Upcoming, d)
		case includeOverdue && d.Due.Before(now) && !d.Due.Before(lookbackStart) &&
			d.Key.Module == "assign" && !d.Status.Done() && d.Status != SubmissionUnknown:
			res.Overdue = append(res.Overdue, d)
		}
	}
	sortDeadlines(res.Upcoming)
	sortDeadlines(res.Overdue)
	res.FetchedAt = stamp.t
	return res, nil
}

// MergeDeadlines combines calendar action events and assignment due dates,
// deduplicating by (module, instance).
//
// Rules: the calendar's time wins over the assignment's duedate (it includes
// user and group overrides); the assignment's name is preferred over the
// localised event name ("… ist fällig"); assignments without an event are
// kept (the event disappears once submitted); assignments without a due date
// are dropped; grading deadlines (teacher events) are ignored.
func MergeDeadlines(events []moodle.Event, courses []moodle.CourseAssignments, refs map[int]CourseRef, base string) []Deadline {
	byKey := map[DeadlineKey]*Deadline{}
	var order []DeadlineKey
	add := func(d *Deadline) {
		byKey[d.Key] = d
		order = append(order, d.Key)
	}
	ref := func(id int, fallback string) CourseRef {
		if r, ok := refs[id]; ok {
			return r
		}
		if fallback == "" {
			fallback = "course " + itoa(id)
		}
		return CourseRef{ID: id, Label: fallback}
	}

	for _, c := range courses {
		for _, a := range c.Assignments {
			due := a.DueDate.Time()
			if due.IsZero() {
				continue
			}
			courseID := a.Course
			if courseID == 0 {
				courseID = c.ID
			}
			add(&Deadline{
				Key:     DeadlineKey{Module: "assign", Instance: a.ID},
				Course:  ref(courseID, c.FullName),
				Title:   strings.TrimSpace(a.Name),
				URL:     moduleURL(base, "assign", a.CMID),
				Due:     due,
				Status:  SubmissionUnknown,
				Sources: FromAssignments,
			})
		}
	}

	for _, e := range events {
		if e.ModuleName == "" || e.Instance == 0 || e.EventType == "gradingdue" {
			continue
		}
		key := DeadlineKey{Module: e.ModuleName, Instance: e.Instance}
		due := e.TimeSort.Time()
		if d, ok := byKey[key]; ok {
			d.Sources |= FromCalendar
			if !due.IsZero() {
				d.Due = due
			}
			if d.URL == "" {
				d.URL = e.URL
			}
			continue
		}
		title := strings.TrimSpace(e.ActivityName)
		if title == "" {
			title = strings.TrimSpace(e.Name)
		}
		var courseID int
		var courseName string
		if e.Course != nil {
			courseID, courseName = e.Course.ID, e.Course.FullName
		}
		status := SubmissionNotApplicable
		if e.ModuleName == "assign" || e.ModuleName == "quiz" {
			status = SubmissionUnknown
		}
		add(&Deadline{
			Key:     key,
			Course:  ref(courseID, courseName),
			Title:   title,
			URL:     e.URL,
			Due:     due,
			Status:  status,
			Sources: FromCalendar,
		})
	}

	out := make([]Deadline, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	sortDeadlines(out)
	return out
}

func sortDeadlines(ds []Deadline) {
	slices.SortStableFunc(ds, func(a, b Deadline) int {
		if c := a.Due.Compare(b.Due); c != 0 {
			return c
		}
		if c := strings.Compare(a.Course.Label, b.Course.Label); c != 0 {
			return c
		}
		return strings.Compare(a.Title, b.Title)
	})
}

// QuizState maps the user's quiz attempts to a Submission: a finished attempt
// counts as done, an open one (in progress, or overdue and still to be
// submitted) as in progress; no attempts or only abandoned ones as not done.
func QuizState(attempts []moodle.QuizAttempt) Submission {
	st := SubmissionNotSubmitted
	for _, a := range attempts {
		switch a.State {
		case moodle.QuizFinished:
			return SubmissionSubmitted
		case moodle.QuizInProgress, moodle.QuizOverdue:
			st = SubmissionInProgress
		}
	}
	return st
}

// SubmissionState maps mod_assign_get_submission_status to a Submission and
// returns the user's extension due date, if any. Team submissions take
// precedence when present.
func SubmissionState(st *moodle.SubmissionStatus) (Submission, time.Time) {
	if st == nil || st.LastAttempt == nil {
		return SubmissionUnknown, time.Time{}
	}
	la := st.LastAttempt
	ext := la.ExtensionDueDate.Time()
	sub := la.Submission
	if la.TeamSubmission != nil {
		sub = la.TeamSubmission
	}
	if sub == nil {
		return SubmissionNotSubmitted, ext
	}
	switch sub.Status {
	case moodle.SubmissionSubmitted:
		if bool(la.Graded) || la.GradingStatus == "graded" {
			return SubmissionGraded, ext
		}
		return SubmissionSubmitted, ext
	case moodle.SubmissionDraft:
		return SubmissionDraft, ext
	case moodle.SubmissionReopened:
		return SubmissionReopened, ext
	case moodle.SubmissionNew, "":
		return SubmissionNotSubmitted, ext
	default:
		return SubmissionUnknown, ext
	}
}
