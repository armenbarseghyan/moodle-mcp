package study

import (
	"fmt"
	"time"
)

// Domain models. Times are absolute instants (rendered in Vienna by render),
// strings are plain text without HTML, and empty means "not shown".

// CourseRef identifies a course in other models.
type CourseRef struct {
	ID    int
	Label string // display name without the prefix shared by all active courses
}

// Course is an enrolled course.
type Course struct {
	ID        int
	Name      string // full name
	Label     string // Name without the shared prefix
	ShortName string // only when it differs from Name
	Start     time.Time
	End       time.Time // zero: no end date
	Progress  *float64
	Past      bool
	URL       string
}

// Ref returns the course reference used in other models.
func (c Course) Ref() CourseRef { return CourseRef{ID: c.ID, Label: c.Label} }

// CourseError is a non-fatal failure for one course; the rest of the answer
// is still useful.
type CourseError struct {
	Course CourseRef
	Err    error
}

// Submission is the state of the user's submission for an assignment.
type Submission int

const (
	SubmissionNotApplicable Submission = iota // not an assignment
	SubmissionUnknown                         // status could not be fetched
	SubmissionNotSubmitted
	SubmissionDraft
	SubmissionSubmitted
	SubmissionGraded
	SubmissionReopened
)

// Done reports whether nothing is left to do for the user.
func (s Submission) Done() bool { return s == SubmissionSubmitted || s == SubmissionGraded }

// DeadlineKey identifies an activity across calendar events and assignments.
type DeadlineKey struct {
	Module   string // "assign", "quiz", ...
	Instance int
}

// Source flags for a merged deadline.
const (
	FromCalendar = 1 << iota
	FromAssignments
)

// Deadline is one merged, deduplicated deadline.
type Deadline struct {
	Key     DeadlineKey
	Course  CourseRef
	Title   string
	URL     string
	Due     time.Time
	Status  Submission
	Sources int
}

// Section is a course section with its visible items.
type Section struct {
	Name    string
	Summary string
	Items   []Item
	Hidden  int // modules the user cannot see
}

// Item is a course module.
type Item struct {
	CMID  int
	Kind  string // modname
	Name  string
	Text  string // description or label text
	URL   string // module page
	Link  string // external target of a "url" module
	Files []File
	// Content holds files that make up the module itself (the HTML of a page
	// or book). They are searchable but not shown as downloads.
	Content []File
	Sub     *Section // subsection content (Moodle 4.5+)
}

// File is a file attached to a module.
type File struct {
	Name     string
	URL      string // browser URL: /pluginfile.php/..., never contains a token
	Size     int64
	Modified time.Time
}

// Grade is one graded (or commented) grade item.
type Grade struct {
	Name     string
	Kind     string // assign, quiz, manual, ...
	Display  string // as Moodle formats it: "87,50", a scale or a letter
	Max      float64
	Percent  *float64
	Feedback string
	GradedAt time.Time
	URL      string
}

// Announcement is a post in a course's announcements forum.
type Announcement struct {
	Course CourseRef
	Title  string
	Author string
	Text   string
	URL    string
	Posted time.Time
	Edited time.Time // zero unless edited after posting
	Pinned bool
	Unread bool
}

// AmbiguousError is returned when a course query matches several courses.
// It is a normal answer (a list to choose from), not a failure.
type AmbiguousError struct {
	Query      string
	Candidates []Course
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("course %q is ambiguous: %d candidates", e.Query, len(e.Candidates))
}

// NotFoundError is returned when no course matches a query.
type NotFoundError struct {
	Query     string
	Available []Course
}

func (e *NotFoundError) Error() string { return fmt.Sprintf("course %q not found", e.Query) }
