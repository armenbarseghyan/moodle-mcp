package study

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/armenbarseghyan/moodle-mcp/internal/moodle"
)

const fanOutLimit = 5

// Options configures a Service.
type Options struct {
	Now         func() time.Time // default time.Now
	DownloadDir string           // default ~/Downloads/moodle
	Logger      *slog.Logger
}

// Service answers the tools' questions.
type Service struct {
	src         Source
	now         func() time.Time
	downloadDir string
	log         *slog.Logger

	mu     sync.Mutex
	userID int
}

// New returns a Service over src.
func New(src Source, opts Options) *Service {
	s := &Service{src: src, now: opts.Now, downloadDir: opts.DownloadDir, log: opts.Logger}
	if s.now == nil {
		s.now = time.Now
	}
	if s.downloadDir == "" {
		s.downloadDir = "~/Downloads/moodle"
	}
	if s.log == nil {
		s.log = slog.New(slog.DiscardHandler)
	}
	return s
}

// user returns the token owner's id (fetched once per process).
func (s *Service) user(ctx context.Context) (int, error) {
	s.mu.Lock()
	id := s.userID
	s.mu.Unlock()
	if id != 0 {
		return id, nil
	}
	si, err := s.src.SiteInfo(ctx, false)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	s.userID = si.Value.UserID
	s.mu.Unlock()
	return si.Value.UserID, nil
}

// allCourses returns every enrolled course, labelled and sorted by label.
func (s *Service) allCourses(ctx context.Context, refresh bool) ([]Course, time.Time, error) {
	uid, err := s.user(ctx)
	if err != nil {
		return nil, time.Time{}, err
	}
	f, err := s.src.Courses(ctx, uid, refresh)
	if err != nil {
		return nil, time.Time{}, err
	}
	return buildCourses(f.Value, s.now(), s.base()), f.At, nil
}

// activeCourses returns the courses that are neither hidden, completed nor ended.
func (s *Service) activeCourses(ctx context.Context, refresh bool) ([]Course, time.Time, error) {
	all, at, err := s.allCourses(ctx, refresh)
	if err != nil {
		return nil, at, err
	}
	return slices.DeleteFunc(all, func(c Course) bool { return c.Past }), at, nil
}

func (s *Service) base() string {
	return strings.TrimRight(s.src.BaseURL().String(), "/")
}

// buildCourses converts wire courses into labelled domain courses.
func buildCourses(in []moodle.Course, now time.Time, base string) []Course {
	out := make([]Course, 0, len(in))
	for _, c := range in {
		end := c.EndDate.Time()
		dc := Course{
			ID:       c.ID,
			Name:     strings.TrimSpace(c.FullName),
			Start:    c.StartDate.Time(),
			End:      end,
			Progress: c.Progress,
			Past:     bool(c.Hidden) || bool(c.Completed) || (!end.IsZero() && end.Before(now)),
			URL:      base + "/course/view.php?id=" + itoa(c.ID),
		}
		if sn := strings.TrimSpace(c.ShortName); sn != dc.Name {
			dc.ShortName = sn
		}
		out = append(out, dc)
	}
	var active []string
	for _, c := range out {
		if !c.Past {
			active = append(active, c.Name)
		}
	}
	prefix := SharedPrefix(active)
	for i := range out {
		out[i].Label = out[i].Name
		if prefix != "" && strings.HasPrefix(out[i].Name, prefix) && len(out[i].Name) > len(prefix) {
			out[i].Label = out[i].Name[len(prefix):]
		}
	}
	slices.SortFunc(out, func(a, b Course) int { return strings.Compare(a.Label, b.Label) })
	return out
}

// SharedPrefix returns the prefix all names share, cut back to a word
// boundary, e.g. "(DAT_WS2026_1) ". It needs at least two names, and is ""
// when stripping it would leave any name empty.
func SharedPrefix(names []string) string {
	if len(names) < 2 {
		return ""
	}
	p := names[0]
	for _, n := range names[1:] {
		for !strings.HasPrefix(n, p) {
			p = p[:len(p)-1]
		}
	}
	i := strings.LastIndexAny(p, " \t")
	if i < 0 {
		return ""
	}
	p = p[:i+1]
	if strings.TrimSpace(p) == "" {
		return ""
	}
	for _, n := range names {
		if strings.TrimSpace(n[len(p):]) == "" {
			return ""
		}
	}
	return p
}

// fanOut runs fn for every item with bounded concurrency. Non-fatal errors
// are returned per item; a fatal error (bad token, access control, ...) or
// cancellation aborts everything.
func fanOut[T, R any](ctx context.Context, items []T, fn func(context.Context, T) (R, error)) ([]R, []error, error) {
	res := make([]R, len(items))
	errs := make([]error, len(items))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(fanOutLimit)
	for i, it := range items {
		g.Go(func() error {
			r, err := fn(gctx, it)
			if err != nil {
				if moodle.IsFatal(err) || ctx.Err() != nil {
					return err
				}
				errs[i] = err
				return nil
			}
			res[i] = r
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, nil, err
	}
	return res, errs, nil
}

// oldest tracks the oldest fetch time among the data used for an answer.
type oldest struct{ t time.Time }

func (o *oldest) add(t time.Time) {
	if !t.IsZero() && (o.t.IsZero() || t.Before(o.t)) {
		o.t = t
	}
}

func ids(cs []Course) []int {
	out := make([]int, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}

func refsByID(cs []Course) map[int]CourseRef {
	m := make(map[int]CourseRef, len(cs))
	for _, c := range cs {
		m[c.ID] = c.Ref()
	}
	return m
}
