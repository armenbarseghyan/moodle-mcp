package moodle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

func (c *Client) decode(fn string, body []byte, out any) error {
	b := bytes.TrimSpace(bytes.TrimPrefix(body, utf8BOM))
	if len(b) == 0 || (b[0] != '{' && b[0] != '[') {
		return &UnexpectedResponseError{Function: fn, Snippet: c.snippet(body)}
	}
	if mErr := c.parseException(fn, b); mErr != nil {
		return mErr
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("moodle: %s: decode response: %w", fn, err)
	}
	return nil
}

// parseException recognises both error shapes Moodle uses:
// REST:       {"exception": "...", "errorcode": "...", "message": "..."}
// pluginfile: {"error": "...", "errorcode": "...", ...}
// It returns nil when body is not an error object.
func (c *Client) parseException(fn string, body []byte) error {
	b := bytes.TrimSpace(bytes.TrimPrefix(body, utf8BOM))
	if len(b) == 0 || b[0] != '{' {
		return nil
	}
	var e struct {
		Exception string `json:"exception"`
		Error     string `json:"error"`
		ErrorCode string `json:"errorcode"`
		Message   string `json:"message"`
		DebugInfo string `json:"debuginfo"`
	}
	if json.Unmarshal(b, &e) != nil {
		return nil //nolint:nilerr // not an error object; the caller decodes it normally
	}
	if e.Exception == "" && (e.Error == "" || e.ErrorCode == "") {
		return nil
	}
	msg := e.Message
	if msg == "" {
		msg = e.Error
	}
	return &Error{
		Function:  fn,
		Exception: e.Exception,
		ErrorCode: e.ErrorCode,
		Message:   c.Redact(msg),
		DebugInfo: c.Redact(e.DebugInfo),
	}
}

// Bool accepts the many encodings Moodle uses for booleans:
// true/false, 1/0, "1"/"0", "true"/"false", "" and null.
type Bool bool

func (b *Bool) UnmarshalJSON(d []byte) error {
	switch string(d) {
	case "true", "1", `"1"`, `"true"`:
		*b = true
	case "false", "0", `"0"`, `"false"`, `""`, "null":
		*b = false
	default:
		return fmt.Errorf("moodle: cannot decode %s as bool", d)
	}
	return nil
}

// Unix is a Moodle timestamp in seconds. 0 and null mean "not set".
type Unix int64

func (u *Unix) UnmarshalJSON(d []byte) error {
	s := string(d)
	if s == "null" || s == `""` {
		*u = 0
		return nil
	}
	if len(s) >= 2 && s[0] == '"' {
		s = s[1 : len(s)-1]
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("moodle: cannot decode %s as timestamp", d)
	}
	*u = Unix(n)
	return nil
}

// Time returns the timestamp as time.Time, or the zero time when unset.
func (u Unix) Time() time.Time {
	if u <= 0 {
		return time.Time{}
	}
	return time.Unix(int64(u), 0)
}
