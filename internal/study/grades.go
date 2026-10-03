package study

import (
	"context"
	"strings"

	"github.com/armenbarseghyan/moodle-mcp/internal/moodle"
	"github.com/armenbarseghyan/moodle-mcp/internal/textfmt"
)

// CourseGrades are the grades of one course.
type CourseGrades struct {
	Course  CourseRef
	Items   []Grade
	Total   *Grade // course total, when graded
	Pending int    // visible items not graded yet
}

// Empty reports whether the course has nothing to show.
func (g CourseGrades) Empty() bool { return len(g.Items) == 0 && g.Total == nil }

// GradesResult answers moodle_grades. Grades are always fetched live.
type GradesResult struct {
	Courses []CourseGrades
	Errors  []CourseError
}

// Grades returns grades for one course (query is an id or name fragment) or
// for all active courses when query is empty.
func (s *Service) Grades(ctx context.Context, query string) (GradesResult, error) {
	var res GradesResult
	uid, err := s.user(ctx)
	if err != nil {
		return res, err
	}
	var courses []Course
	if strings.TrimSpace(query) == "" {
		courses, _, err = s.activeCourses(ctx, false)
	} else {
		var c Course
		c, _, err = s.resolve(ctx, query, false)
		courses = []Course{c}
	}
	if err != nil {
		return res, err
	}
	items, errs, err := fanOut(ctx, courses, func(ctx context.Context, c Course) ([]moodle.GradeItem, error) {
		return s.src.GradeItems(ctx, c.ID, uid)
	})
	if err != nil {
		return res, err
	}
	for i, c := range courses {
		if errs[i] != nil {
			res.Errors = append(res.Errors, CourseError{Course: c.Ref(), Err: errs[i]})
			continue
		}
		res.Courses = append(res.Courses, BuildGrades(c.Ref(), items[i], s.base()))
	}
	return res, nil
}

// BuildGrades converts grade items into a CourseGrades: hidden items are
// skipped, ungraded items without feedback are only counted, category totals
// are skipped, and the percentage is computed from raw values (Moodle's
// percentageformatted is localised and empty for some scales).
func BuildGrades(course CourseRef, items []moodle.GradeItem, base string) CourseGrades {
	cg := CourseGrades{Course: course}
	for _, it := range items {
		if it.GradeIsHidden {
			continue
		}
		g := Grade{
			Name:     strings.TrimSpace(it.ItemName),
			Kind:     it.ItemModule,
			Display:  strings.TrimSpace(it.GradeFormatted),
			Max:      it.GradeMax,
			Feedback: textfmt.StripHTML(it.Feedback),
			GradedAt: it.GradeDateGraded.Time(),
			URL:      moduleURL(base, it.ItemModule, it.CMID),
		}
		if g.Kind == "" {
			g.Kind = it.ItemType
		}
		if g.Display == "-" {
			g.Display = ""
		}
		if it.GradeRaw != nil && it.GradeMax > it.GradeMin {
			p := (*it.GradeRaw - it.GradeMin) / (it.GradeMax - it.GradeMin) * 100
			g.Percent = &p
		}
		graded := it.GradeRaw != nil || g.Display != ""
		switch it.ItemType {
		case "course":
			if graded {
				g.Name = ""
				cg.Total = &g
			}
		case "category":
			// Category subtotals add noise; the course total is enough.
		default:
			if !graded && g.Feedback == "" {
				cg.Pending++
				continue
			}
			cg.Items = append(cg.Items, g)
		}
	}
	return cg
}
