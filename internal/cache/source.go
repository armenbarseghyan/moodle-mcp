package cache

import (
	"context"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"moodle-mcp/internal/extract"
	"moodle-mcp/internal/moodle"
	"moodle-mcp/internal/study"
)

// TTLs per data kind (docs/ARCHITECTURE.md §3).
const (
	CoursesTTL   = 15 * time.Minute
	ContentsTTL  = 15 * time.Minute
	ForumsTTL    = 15 * time.Minute
	DeadlinesTTL = 5 * time.Minute // events, assignments, submission status
	NewsTTL      = 5 * time.Minute // discussions
	forever      = 0
)

// Source implements study.Source over a *moodle.Client with in-memory TTL
// caches. Grade items and downloads are never cached.
type Source struct {
	c   *moodle.Client
	now func() time.Time

	siteInfo    *TTL[*moodle.SiteInfo]
	courses     *TTL[[]moodle.Course]
	contents    *TTL[[]moodle.Section]
	events      *TTL[[]moodle.Event]
	assignments *TTL[study.Assignments]
	status      *TTL[*moodle.SubmissionStatus]
	quiz        *TTL[[]moodle.QuizAttempt]
	forums      *TTL[[]moodle.Forum]
	discussions *TTL[[]moodle.Discussion]
	texts       *TTL[[]extract.Part]
}

var _ study.Source = (*Source)(nil)

// NewSource wraps c. now defaults to time.Now.
func NewSource(c *moodle.Client, now func() time.Time) *Source {
	if now == nil {
		now = time.Now
	}
	return &Source{
		c:           c,
		now:         now,
		siteInfo:    New[*moodle.SiteInfo](forever, now),
		courses:     New[[]moodle.Course](CoursesTTL, now),
		contents:    New[[]moodle.Section](ContentsTTL, now),
		events:      New[[]moodle.Event](DeadlinesTTL, now),
		assignments: New[study.Assignments](DeadlinesTTL, now),
		status:      New[*moodle.SubmissionStatus](DeadlinesTTL, now),
		quiz:        New[[]moodle.QuizAttempt](DeadlinesTTL, now),
		forums:      New[[]moodle.Forum](ForumsTTL, now),
		discussions: New[[]moodle.Discussion](NewsTTL, now),
		texts:       New[[]extract.Part](forever, now), // keyed by URL+size+mtime
	}
}

func fetched[T any](e Entry[T], err error) (study.Fetched[T], error) {
	return study.Fetched[T]{Value: e.Value, At: e.FetchedAt}, err
}

func (s *Source) SiteInfo(ctx context.Context, refresh bool) (study.Fetched[*moodle.SiteInfo], error) {
	return fetched(s.siteInfo.Get(ctx, "siteinfo", refresh, s.c.SiteInfo))
}

func (s *Source) Courses(ctx context.Context, userID int, refresh bool) (study.Fetched[[]moodle.Course], error) {
	return fetched(s.courses.Get(ctx, "courses:"+strconv.Itoa(userID), refresh,
		func(ctx context.Context) ([]moodle.Course, error) { return s.c.UserCourses(ctx, userID) }))
}

func (s *Source) Contents(ctx context.Context, courseID int, refresh bool) (study.Fetched[[]moodle.Section], error) {
	return fetched(s.contents.Get(ctx, "contents:"+strconv.Itoa(courseID), refresh,
		func(ctx context.Context) ([]moodle.Section, error) { return s.c.CourseContents(ctx, courseID) }))
}

func (s *Source) Events(ctx context.Context, from, to time.Time, refresh bool) (study.Fetched[[]moodle.Event], error) {
	key := "events:" + strconv.FormatInt(from.Unix(), 10) + ":" + strconv.FormatInt(to.Unix(), 10)
	return fetched(s.events.Get(ctx, key, refresh,
		func(ctx context.Context) ([]moodle.Event, error) { return s.c.ActionEvents(ctx, from, to) }))
}

func (s *Source) Assignments(ctx context.Context, courseIDs []int, refresh bool) (study.Fetched[study.Assignments], error) {
	return fetched(s.assignments.Get(ctx, "assignments:"+idsKey(courseIDs), refresh,
		func(ctx context.Context) (study.Assignments, error) {
			cs, ws, err := s.c.Assignments(ctx, courseIDs)
			return study.Assignments{Courses: cs, Warnings: ws}, err
		}))
}

func (s *Source) SubmissionStatus(ctx context.Context, assignID int, refresh bool) (study.Fetched[*moodle.SubmissionStatus], error) {
	return fetched(s.status.Get(ctx, "status:"+strconv.Itoa(assignID), refresh,
		func(ctx context.Context) (*moodle.SubmissionStatus, error) {
			return s.c.SubmissionStatus(ctx, assignID)
		}))
}

func (s *Source) QuizAttempts(ctx context.Context, quizID int, refresh bool) (study.Fetched[[]moodle.QuizAttempt], error) {
	return fetched(s.quiz.Get(ctx, "quiz:"+strconv.Itoa(quizID), refresh,
		func(ctx context.Context) ([]moodle.QuizAttempt, error) { return s.c.QuizAttempts(ctx, quizID) }))
}

func (s *Source) Forums(ctx context.Context, courseIDs []int, refresh bool) (study.Fetched[[]moodle.Forum], error) {
	return fetched(s.forums.Get(ctx, "forums:"+idsKey(courseIDs), refresh,
		func(ctx context.Context) ([]moodle.Forum, error) { return s.c.Forums(ctx, courseIDs) }))
}

func (s *Source) Discussions(ctx context.Context, forumID int, refresh bool) (study.Fetched[[]moodle.Discussion], error) {
	return fetched(s.discussions.Get(ctx, "discussions:"+strconv.Itoa(forumID), refresh,
		func(ctx context.Context) ([]moodle.Discussion, error) { return s.c.Discussions(ctx, forumID, 10) }))
}

func (s *Source) GradeItems(ctx context.Context, courseID, userID int) ([]moodle.GradeItem, error) {
	return s.c.GradeItems(ctx, courseID, userID)
}

func (s *Source) Download(ctx context.Context, fileURL, dest string) (moodle.Downloaded, error) {
	return s.c.Download(ctx, fileURL, dest)
}

// FileText downloads a file into memory and extracts its text. The key
// includes size and modification time, so a changed file is read again.
func (s *Source) FileText(ctx context.Context, f study.File) (study.Fetched[[]extract.Part], error) {
	key := "text:" + f.URL + "@" + strconv.FormatInt(f.Size, 10) + ":" + strconv.FormatInt(f.Modified.Unix(), 10)
	return fetched(s.texts.Get(ctx, key, false, func(ctx context.Context) ([]extract.Part, error) {
		file, err := s.c.FetchFile(ctx, f.URL, extract.MaxFileBytes)
		if err != nil {
			return nil, err
		}
		return extract.Text(f.Name, file.Data)
	}))
}

func (s *Source) BaseURL() *url.URL { return s.c.BaseURL() }

// idsKey is order-independent: the same set of courses hits the same entry.
func idsKey(ids []int) string {
	s := slices.Clone(ids)
	slices.Sort(s)
	parts := make([]string, len(s))
	for i, id := range s {
		parts[i] = strconv.Itoa(id)
	}
	return strings.Join(parts, ",")
}
