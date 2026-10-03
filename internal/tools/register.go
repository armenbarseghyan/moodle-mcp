// Package tools exposes study use cases as MCP tools. It is a thin adapter:
// input schemas, a call into study, and rendering of the result or error.
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/armenbarseghyan/moodle-mcp/internal/render"
	"github.com/armenbarseghyan/moodle-mcp/internal/study"
)

// Deps are the tools' dependencies.
type Deps struct {
	Service *study.Service
	Now     func() time.Time
	Version string
	Logger  *slog.Logger
	// InitErr, when set, is returned by every tool: the server still starts,
	// so the user sees a clear message instead of "Connection closed".
	InitErr error
}

// Instructions are sent to the client on initialisation.
const Instructions = `Read-only access to an FH JOANNEUM student's Moodle.
What is due and when: moodle_deadlines. What teachers posted: moodle_announcements (check first, it is often the most urgent).
What changed lately — new or updated files, announcements, grades — in one call: moodle_whats_new.
Find material without knowing the course: moodle_search; a course's structure: moodle_course_contents.
Links in answers open in the user's browser, where they are signed in to Moodle — pass them on as they are.
Use moodle_download only when a file is needed locally. All dates are Europe/Vienna.`

// Register adds all tools to s.
func Register(s *mcp.Server, d Deps) {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Logger == nil {
		d.Logger = slog.New(slog.DiscardHandler)
	}
	s.AddReceivingMiddleware(nullArguments)
	ro := &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: ptr(true)}

	add(s, d, &mcp.Tool{
		Name:        "moodle_courses",
		Title:       "My courses",
		Description: "Active courses: id, name, dates, progress. Hidden and completed courses only with include_past.",
		Annotations: ro,
		InputSchema: schema[CoursesIn](nil),
	}, func(ctx context.Context, in CoursesIn) (string, error) {
		r, err := d.Service.Courses(ctx, in.IncludePast, in.Refresh)
		return render.Courses(r, d.Now()), err
	})

	add(s, d, &mcp.Tool{
		Name:  "moodle_deadlines",
		Title: "Deadlines",
		Description: "The main tool: what is due in the next N days. Merges the Moodle calendar with assignment due dates, " +
			"shows for every assignment whether it is submitted, plus unsubmitted ones overdue by up to 7 days. Date, course, title, type, status, link.",
		Annotations: ro,
		InputSchema: schema[DeadlinesIn](map[string]prop{
			"days":            {Default: 14, Min: ptr(1.0), Max: ptr(90.0)},
			"include_overdue": {Default: true},
		}),
	}, func(ctx context.Context, in DeadlinesIn) (string, error) {
		overdue := in.IncludeOverdue == nil || *in.IncludeOverdue
		r, err := d.Service.Deadlines(ctx, orDefault(in.Days, 14), overdue, in.Refresh)
		return render.Deadlines(r, d.Now()), err
	})

	add(s, d, &mcp.Tool{
		Name:  "moodle_course_contents",
		Title: "Course contents",
		Description: "A course's material by section: modules, types and links to files (open in the browser). " +
			"course is an id or part of the name.",
		Annotations: ro,
		InputSchema: schema[ContentsIn](map[string]prop{"course": {MinLen: ptr(1)}}),
	}, func(ctx context.Context, in ContentsIn) (string, error) {
		r, err := d.Service.CourseContents(ctx, in.Course, in.Refresh)
		return render.Contents(r, d.Now()), err
	})

	add(s, d, &mcp.Tool{
		Name:  "moodle_search",
		Title: "Search material",
		Description: "Find material when you remember the topic but not the course: module names, descriptions, sections and file names " +
			"in all active courses. With in_files=true also the text inside PDF, docx, pptx, ipynb, zip and Moodle pages " +
			"(with page numbers and a quote). File text is cached until the file changes.",
		Annotations: ro,
		InputSchema: schema[SearchIn](map[string]prop{"query": {MinLen: ptr(2)}}),
	}, func(ctx context.Context, in SearchIn) (string, error) {
		r, err := d.Service.Search(ctx, study.SearchQuery{Text: in.Query, Course: in.Course, InFiles: in.InFiles, Refresh: in.Refresh})
		return render.Search(r, d.Now()), err
	})

	add(s, d, &mcp.Tool{
		Name:        "moodle_grades",
		Title:       "Grades",
		Description: "Grades: score, maximum, percentage and teacher feedback, for one course or all active ones. Always fresh (no cache).",
		Annotations: ro,
		InputSchema: schema[GradesIn](nil),
	}, func(ctx context.Context, in GradesIn) (string, error) {
		r, err := d.Service.Grades(ctx, in.Course)
		return render.Grades(r, d.Now()), err
	})

	add(s, d, &mcp.Tool{
		Name:  "moodle_announcements",
		Title: "Announcements",
		Description: "Recent posts in the announcement forums of all courses — what teachers send at the last minute " +
			"(rescheduling, room changes). Check this first.",
		Annotations: ro,
		InputSchema: schema[AnnouncementsIn](map[string]prop{"days": {Default: 7, Min: ptr(1.0), Max: ptr(90.0)}}),
	}, func(ctx context.Context, in AnnouncementsIn) (string, error) {
		r, err := d.Service.Announcements(ctx, orDefault(in.Days, 7), in.Refresh)
		return render.Announcements(r, d.Now()), err
	})

	add(s, d, &mcp.Tool{
		Name:  "moodle_whats_new",
		Title: "What's new",
		Description: "Everything that changed in the active courses in the last N days, in one call: " +
			"new or updated files (lecture slides, task sheets, pages), announcements and new grades. " +
			"Use for \"what's new\", \"did they upload anything\", catching up after a few days off.",
		Annotations: ro,
		InputSchema: schema[WhatsNewIn](map[string]prop{"days": {Default: 7, Min: ptr(1.0), Max: ptr(60.0)}}),
	}, func(ctx context.Context, in WhatsNewIn) (string, error) {
		r, err := d.Service.WhatsNew(ctx, orDefault(in.Days, 7), in.Refresh)
		return render.WhatsNew(r, d.Now()), err
	})

	add(s, d, &mcp.Tool{
		Name:  "moodle_download",
		Title: "Download a file",
		Description: "Downloads a Moodle file to disk and returns the local path. Only needed when the file is needed locally " +
			"(e.g. to read it); to just open it, the link is enough.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(false), IdempotentHint: true, OpenWorldHint: ptr(true)},
		InputSchema: schema[DownloadIn](map[string]prop{"fileurl": {MinLen: ptr(1)}}),
	}, func(ctx context.Context, in DownloadIn) (string, error) {
		r, err := d.Service.Download(ctx, in.FileURL, in.Dest)
		return render.Download(r), err
	})

	add(s, d, &mcp.Tool{
		Name:        "moodle_whoami",
		Title:       "Diagnostics",
		Description: "Checks the token and the site: name, login, Moodle version, number of functions and which functions the server is missing.",
		Annotations: ro,
		InputSchema: schema[WhoAmIIn](nil),
	}, func(ctx context.Context, _ WhoAmIIn) (string, error) {
		r, err := d.Service.WhoAmI(ctx)
		return render.WhoAmI(r, d.Version), err
	})
}

// add registers a tool whose handler returns markdown. Errors become
// IsError results with a human explanation; an ambiguous or unknown course is
// a normal answer (a list to choose from).
func add[In any](s *mcp.Server, d Deps, t *mcp.Tool, h func(context.Context, In) (string, error)) {
	mcp.AddTool(s, t, func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		start := time.Now()
		if d.InitErr != nil {
			return errorResult(render.ErrorText(d.InitErr)), nil, nil //nolint:nilerr // reported as an IsError tool result
		}
		out, err := h(ctx, in)
		d.Logger.Info("tool call", "tool", t.Name, "dur_ms", time.Since(start).Milliseconds(), "err", err)

		var amb *study.AmbiguousError
		var nf *study.NotFoundError
		switch {
		case err == nil:
			return textResult(out), nil, nil
		case errors.As(err, &amb):
			return textResult(render.Ambiguous(amb)), nil, nil
		case errors.As(err, &nf):
			return textResult(render.NotFound(nf)), nil, nil
		default:
			return errorResult(render.ErrorText(err)), nil, nil
		}
	})
}

// nullArguments rewrites `"arguments": null` to `{}`. go-sdk v1.8.0 panics
// when it applies schema defaults to a null argument object (the map it
// fills is nil), which would take the whole server down.
func nullArguments(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if r, ok := req.(*mcp.CallToolRequest); ok && r.Params != nil {
			if a := bytes.TrimSpace(r.Params.Arguments); len(a) == 0 || bytes.Equal(a, []byte("null")) {
				r.Params.Arguments = json.RawMessage("{}")
			}
		}
		return next(ctx, method, req)
	}
}

func textResult(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

func errorResult(s string) *mcp.CallToolResult {
	r := textResult(s)
	r.IsError = true
	return r
}

type prop struct {
	Default any
	Min     *float64
	Max     *float64
	MinLen  *int
}

// schema infers the input schema of T and decorates properties with
// defaults and bounds.
func schema[T any](props map[string]prop) *jsonschema.Schema {
	sc, err := jsonschema.For[T](nil)
	if err != nil {
		panic(err)
	}
	if sc.Properties == nil {
		sc.Properties = map[string]*jsonschema.Schema{}
	}
	for name, p := range props {
		ps, ok := sc.Properties[name]
		if !ok {
			panic("tools: unknown property " + name)
		}
		if p.Default != nil {
			b, err := json.Marshal(p.Default)
			if err != nil {
				panic(err)
			}
			ps.Default = b
		}
		ps.Minimum, ps.Maximum, ps.MinLength = p.Min, p.Max, p.MinLen
	}
	return sc
}

func orDefault(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

func ptr[T any](v T) *T { return &v }
