// Package render turns study results into compact markdown for a human (and
// a token-conscious model) to read. Pure functions: no I/O, time is passed in.
package render

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"moodle-mcp/internal/moodle"
	"moodle-mcp/internal/study"
	"moodle-mcp/internal/textfmt"
)

// MaxChars bounds one tool answer. Lines beyond it are summarised.
const MaxChars = 12000

// staleAfter: answers built from older cached data say so.
const staleAfter = time.Minute

// doc accumulates lines up to MaxChars.
type doc struct {
	b       strings.Builder
	dropped int
	limit   int
}

func newDoc() *doc { return &doc{limit: MaxChars} }

// line appends one line. format is an English catalog key printed through
// textfmt.P(), so it can be translated later; pass identifiers (ids, names,
// URLs) as strings, the printer localises %d numbers.
func (d *doc) line(format string, args ...any) {
	s := format
	if len(args) > 0 {
		s = textfmt.P().Sprintf(format, args...)
	}
	if d.dropped > 0 || d.b.Len()+len(s)+1 > d.limit {
		d.dropped++
		return
	}
	d.b.WriteString(s)
	d.b.WriteByte('\n')
}

func (d *doc) blank() {
	if d.dropped == 0 && d.b.Len() > 0 && !strings.HasSuffix(d.b.String(), "\n\n") {
		d.b.WriteByte('\n')
	}
}

// finish appends the truncation note and the cache note.
func (d *doc) finish(now, fetchedAt time.Time) string {
	if d.dropped > 0 {
		p := textfmt.P()
		fmt.Fprintf(&d.b, "\n… %s %s\n", p.Sprintf("%d more lines", d.dropped), p.Sprintf("not shown — narrow the request."))
	}
	if !fetchedAt.IsZero() && now.Sub(fetchedAt) >= staleAfter {
		fmt.Fprintf(&d.b, "\n_%s_\n", textfmt.P().Sprintf("Cached data from %s; use refresh=true for fresh data.",
			textfmt.Relative(fetchedAt, now)))
	}
	return strings.TrimRight(d.b.String(), "\n") + "\n"
}

// link renders a markdown link, escaping what would break it.
func link(text, href string) string {
	text = strings.NewReplacer("[", "\\[", "]", "\\]").Replace(text)
	if href == "" {
		return text
	}
	return "[" + text + "](" + escapeURL(href) + ")"
}

func escapeURL(u string) string {
	return strings.NewReplacer(" ", "%20", "(", "%28", ")", "%29").Replace(u)
}

// when renders "2026-10-07 17:15 (Wednesday) — in 5 days".
func when(t, now time.Time) string {
	return textfmt.Date(t) + " — " + textfmt.Relative(t, now)
}

// Size renders a byte count: "512 B", "222 KB", "2.8 MB".
func Size(n int64) string {
	switch {
	case n <= 0:
		return ""
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%d KB", (n+512)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}

var kindLabels = map[string]string{
	"assign": "assignment", "quiz": "quiz", "resource": "file", "folder": "folder", "url": "link",
	"page": "page", "book": "book", "forum": "forum", "lti": "external tool", "h5pactivity": "H5P",
	"hvp": "H5P", "choice": "choice", "feedback": "survey", "questionnaire": "survey",
	"workshop": "peer review", "lesson": "lesson", "scorm": "SCORM", "glossary": "glossary",
	"wiki": "wiki", "label": "text", "subsection": "subsection", "manual": "manual",
}

func kindLabel(modname string) string {
	if l, ok := kindLabels[modname]; ok {
		return textfmt.P().Sprintf(l)
	}
	return modname
}

func courseErrors(d *doc, errs []study.CourseError) {
	if len(errs) == 0 {
		return
	}
	d.blank()
	for _, e := range errs {
		d.line("⚠ %s: %s", e.Course.Label, ErrorText(e.Err))
	}
}

// ErrorText explains an error in plain words. It never contains the token:
// every error from the client is already redacted.
func ErrorText(err error) string {
	p := textfmt.P()
	var (
		hErr *moodle.HTTPError
		mErr *moodle.Error
		uErr *url.Error
	)
	switch {
	case err == nil:
		return ""
	case errors.Is(err, moodle.ErrInvalidToken):
		return p.Sprintf("The Moodle token is invalid or revoked. Create a new one in Moodle " +
			"(Profile → Preferences → Security keys, service moodle_mobile_app) and update MOODLE_TOKEN.")
	case errors.Is(err, moodle.ErrAccessDenied):
		return p.Sprintf("Moodle refused the call (access control): the function is not part of the token's service or is disabled for your role.")
	case errors.Is(err, moodle.ErrNotAccessible):
		return p.Sprintf("The course or activity is not accessible (hidden or restricted).")
	case errors.Is(err, moodle.ErrMaintenance):
		return p.Sprintf("Moodle is in maintenance mode, try again later.")
	case errors.Is(err, moodle.ErrNotAllowed):
		return p.Sprintf("This Moodle function is not allowed: the server is read-only.")
	case errors.Is(err, moodle.ErrForeignURL):
		return p.Sprintf("The link does not point to a file of this Moodle (expected …/pluginfile.php/…). " +
			"The server does not download from other sites, so the token never leaves.")
	case errors.Is(err, moodle.ErrUnexpectedResponse):
		return p.Sprintf("Moodle returned a non-JSON response — maintenance or a proxy problem.")
	case errors.Is(err, context.DeadlineExceeded):
		return p.Sprintf("Moodle did not answer in time (timeout).")
	case errors.As(err, &hErr):
		return p.Sprintf("Moodle answered HTTP %s.", strconv.Itoa(hErr.Status))
	case errors.As(err, &mErr):
		return p.Sprintf("Moodle returned the error %s: %s", mErr.ErrorCode, mErr.Message)
	case errors.As(err, &uErr) && uErr.Timeout():
		return p.Sprintf("Moodle did not answer in time (timeout).")
	case errors.As(err, &uErr):
		return p.Sprintf("Cannot reach Moodle: %s", uErr.Err.Error())
	default:
		return err.Error()
	}
}
