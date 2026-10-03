package study

import (
	"context"
	"slices"
	"strings"
	"time"
)

// WhatsNewResult answers moodle_whats_new: what appeared in the active
// courses within the last Days days.
type WhatsNewResult struct {
	Days          int
	Since         time.Time
	Materials     []NewMaterial  // newest first
	Announcements []Announcement // newest first
	Graded        []GradedItem   // newest first
	Errors        []CourseError
	FetchedAt     time.Time
}

// NewMaterial is a course file added or changed within the period.
type NewMaterial struct {
	Course CourseRef
	Path   []string // section names, outermost first
	Item   Item     // the module, without its files
	File   File
	Added  bool // created within the period; otherwise an update of an older file
}

// When is the time that put the file in the result.
func (m NewMaterial) When() time.Time {
	if m.Added && !m.File.Created.IsZero() {
		return m.File.Created
	}
	return m.File.Modified
}

// GradedItem is a grade given within the period.
type GradedItem struct {
	Course CourseRef
	Grade  Grade
}

// WhatsNew collects new and updated course files, announcements and grades of
// the last days days. Each part fails on its own: an error in one course or
// one part is reported, the rest is still shown.
func (s *Service) WhatsNew(ctx context.Context, days int, refresh bool) (WhatsNewResult, error) {
	now := s.now()
	res := WhatsNewResult{Days: days, Since: now.Add(-time.Duration(days) * 24 * time.Hour)}
	var stamp oldest

	courses, at, err := s.activeCourses(ctx, refresh)
	if err != nil {
		return res, err
	}
	stamp.add(at)

	trees, errs, err := fanOut(ctx, courses, func(ctx context.Context, c Course) (Fetched[[]Section], error) {
		f, err := s.src.Contents(ctx, c.ID, refresh)
		if err != nil {
			return Fetched[[]Section]{}, err
		}
		return Fetched[[]Section]{Value: BuildTree(f.Value, s.base()), At: f.At}, nil
	})
	if err != nil {
		return res, err
	}
	for i, c := range courses {
		if errs[i] != nil {
			res.Errors = append(res.Errors, CourseError{Course: c.Ref(), Err: errs[i]})
			continue
		}
		stamp.add(trees[i].At)
		res.Materials = append(res.Materials, NewMaterials(c.Ref(), trees[i].Value, res.Since)...)
	}
	slices.SortStableFunc(res.Materials, func(a, b NewMaterial) int {
		if c := b.When().Compare(a.When()); c != 0 {
			return c
		}
		return strings.Compare(a.File.Name, b.File.Name)
	})

	ann, err := s.Announcements(ctx, days, refresh)
	if err != nil {
		return res, err
	}
	stamp.add(ann.FetchedAt)
	res.Announcements = ann.Items
	res.Errors = append(res.Errors, ann.Errors...)

	grades, err := s.Grades(ctx, "")
	if err != nil {
		return res, err
	}
	for _, cg := range grades.Courses {
		for _, g := range cg.Items {
			if !g.GradedAt.Before(res.Since) {
				res.Graded = append(res.Graded, GradedItem{Course: cg.Course, Grade: g})
			}
		}
	}
	slices.SortStableFunc(res.Graded, func(a, b GradedItem) int { return b.Grade.GradedAt.Compare(a.Grade.GradedAt) })
	res.Errors = append(res.Errors, grades.Errors...)

	res.FetchedAt = stamp.t
	return res, nil
}

// NewMaterials lists the files of a course tree created or modified since
// `since`, including the HTML of pages and books (a changed page is news too).
func NewMaterials(course CourseRef, secs []Section, since time.Time) []NewMaterial {
	var out []NewMaterial
	for _, f := range collectFiles(course, secs) {
		added := !f.file.Created.IsZero() && !f.file.Created.Before(since)
		if !added && f.file.Modified.Before(since) {
			continue
		}
		out = append(out, NewMaterial{Course: course, Path: f.path, Item: f.item, File: f.file, Added: added})
	}
	return out
}
