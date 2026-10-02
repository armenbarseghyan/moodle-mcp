package moodle

import (
	"errors"
	"fmt"
)

// Sentinel errors. Match them with errors.Is; *Error implements Is.
var (
	ErrInvalidToken       = errors.New("moodle: invalid or revoked token")
	ErrAccessDenied       = errors.New("moodle: function not allowed for this web service (access control)")
	ErrNotAccessible      = errors.New("moodle: course or activity not accessible")
	ErrMaintenance        = errors.New("moodle: site is in maintenance mode")
	ErrNotAllowed         = errors.New("moodle: function is not on the read-only allowlist")
	ErrUnexpectedResponse = errors.New("moodle: unexpected response")
	ErrForeignURL         = errors.New("moodle: URL does not point to this Moodle site's files")
	ErrTooLarge           = errors.New("moodle: file too large")
)

// Error is an exception reported by Moodle (usually with HTTP 200).
type Error struct {
	Function  string
	Exception string
	ErrorCode string
	Message   string
	DebugInfo string
}

func (e *Error) Error() string {
	return fmt.Sprintf("moodle: %s: %s: %s", e.Function, e.ErrorCode, e.Message)
}

// Is maps Moodle error codes onto sentinel errors. Codes are matched rather
// than messages, because messages are localised (this site answers in German).
func (e *Error) Is(target error) bool {
	switch target {
	case ErrInvalidToken:
		return e.ErrorCode == "invalidtoken"
	case ErrAccessDenied:
		return e.ErrorCode == "accessexception"
	case ErrNotAccessible:
		return e.ErrorCode == "requireloginerror"
	case ErrMaintenance:
		return e.ErrorCode == "sitemaintenance"
	}
	return false
}

// HTTPError is a non-2xx response that was not a Moodle exception.
type HTTPError struct {
	Function string
	Status   int
	Snippet  string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("moodle: %s: HTTP %d: %s", e.Function, e.Status, e.Snippet)
}

// UnexpectedResponseError is a 2xx response that is not JSON (e.g. an HTML
// maintenance page served by a proxy).
type UnexpectedResponseError struct {
	Function string
	Snippet  string
}

func (e *UnexpectedResponseError) Error() string {
	return fmt.Sprintf("moodle: %s: unexpected non-JSON response: %q", e.Function, e.Snippet)
}

func (e *UnexpectedResponseError) Is(target error) bool { return target == ErrUnexpectedResponse }

// IsFatal reports whether err invalidates every other request too, so a
// fan-out over courses should stop instead of reporting per-course failures.
func IsFatal(err error) bool {
	return errors.Is(err, ErrInvalidToken) ||
		errors.Is(err, ErrAccessDenied) ||
		errors.Is(err, ErrMaintenance) ||
		errors.Is(err, ErrNotAllowed)
}
