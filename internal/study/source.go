// Package study implements the server's use cases: it combines several
// Moodle calls into answers to real questions (what is due, where is that
// file, what did I get) and returns domain models for rendering.
//
// It knows nothing about MCP or HTTP. Data comes from a Source.
package study

import (
	"context"
	"net/url"
	"time"

	"github.com/armenbarseghyan/moodle-mcp/internal/extract"
	"github.com/armenbarseghyan/moodle-mcp/internal/moodle"
)

// Fetched is a value with the time it was fetched from Moodle. Values served
// from a cache carry the original fetch time.
type Fetched[T any] struct {
	Value T
	At    time.Time
}

// Assignments is the result of mod_assign_get_assignments.
type Assignments struct {
	Courses  []moodle.CourseAssignments
	Warnings []moodle.Warning
}

// Source provides Moodle data. refresh=true bypasses (and refills) any cache.
// The production implementation is cache.Source over *moodle.Client.
type Source interface {
	SiteInfo(ctx context.Context, refresh bool) (Fetched[*moodle.SiteInfo], error)
	Courses(ctx context.Context, userID int, refresh bool) (Fetched[[]moodle.Course], error)
	Contents(ctx context.Context, courseID int, refresh bool) (Fetched[[]moodle.Section], error)
	Events(ctx context.Context, from, to time.Time, refresh bool) (Fetched[[]moodle.Event], error)
	Assignments(ctx context.Context, courseIDs []int, refresh bool) (Fetched[Assignments], error)
	SubmissionStatus(ctx context.Context, assignID int, refresh bool) (Fetched[*moodle.SubmissionStatus], error)
	QuizAttempts(ctx context.Context, quizID int, refresh bool) (Fetched[[]moodle.QuizAttempt], error)
	Forums(ctx context.Context, courseIDs []int, refresh bool) (Fetched[[]moodle.Forum], error)
	Discussions(ctx context.Context, forumID int, refresh bool) (Fetched[[]moodle.Discussion], error)
	// GradeItems is never cached.
	GradeItems(ctx context.Context, courseID, userID int) ([]moodle.GradeItem, error)
	Download(ctx context.Context, fileURL, dest string) (moodle.Downloaded, error)
	// FileText returns the searchable text of a course file. Implementations
	// cache it while the file is unchanged (same URL, size and modification time).
	FileText(ctx context.Context, f File) (Fetched[[]extract.Part], error)
	BaseURL() *url.URL
}
