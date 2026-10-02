package study

import (
	"context"
	"time"
)

// CoursesResult answers moodle_courses.
type CoursesResult struct {
	Active    []Course
	Past      []Course // only with includePast
	Prefix    string   // shared prefix stripped from labels
	FetchedAt time.Time
}

// Courses lists enrolled courses; past ones (hidden, completed or ended)
// only when includePast is set.
func (s *Service) Courses(ctx context.Context, includePast, refresh bool) (CoursesResult, error) {
	all, at, err := s.allCourses(ctx, refresh)
	if err != nil {
		return CoursesResult{}, err
	}
	res := CoursesResult{FetchedAt: at}
	var names []string
	for _, c := range all {
		if c.Past {
			if includePast {
				res.Past = append(res.Past, c)
			}
			continue
		}
		res.Active = append(res.Active, c)
		names = append(names, c.Name)
	}
	res.Prefix = SharedPrefix(names)
	return res, nil
}

// resolve finds a course among all enrolled courses.
func (s *Service) resolve(ctx context.Context, query string, refresh bool) (Course, time.Time, error) {
	all, at, err := s.allCourses(ctx, refresh)
	if err != nil {
		return Course{}, at, err
	}
	c, err := ResolveCourse(all, query)
	return c, at, err
}
