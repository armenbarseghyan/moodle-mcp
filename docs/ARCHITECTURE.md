# moodle-mcp — data architecture

Status: agreed 2026-10-02.

## 1. Layers and the dependency rule

```
            ┌──────────────────────────── cmd/moodle-mcp ───────────────────────────┐
            │ env → Config → moodle.Client → cached Source → study.Service →        │
            │ tools.Register(mcp.Server) → transport (stdio; stage 2: streamable HTTP)│
            └───────────────────────────────────────────────────────────────────────┘
                                         │
  internal/tools    MCP adapter: input schemas, call into study, error → IsError result
        │
  internal/study    use cases: aggregation, merge, resolution, error policy.  Pure logic.
        │      └──► internal/render   domain models → markdown (no I/O)
        │      └──► internal/textfmt  Vienna dates, "in N days", HTML → text, message printer
        ▼
  study.Source (interface) ◄── internal/cache.Source (decorator: TTL + singleflight)
        ▲                               │
        └────────── internal/moodle ◄───┘   transport + wire types + Moodle errors
```

Rules:
- `moodle` knows nothing about MCP, markdown or the cache. It returns wire types (nearly 1:1 with the JSON).
- `study` knows nothing about MCP or HTTP. It takes a `Source` and a clock and returns domain models.
- `render` is pure functions `Model → string`. Tested with golden files.
- `tools` is a thin layer: 8 handlers of 5–10 lines each. In stage 2 only `cmd/` changes.

> Change from the first draft: the logic moved out of `internal/tools` into
> `internal/study` + `internal/render`. `tools` then holds only the
> "tool definitions", as originally intended, and the logic is tested without MCP.

## 2. Fetching data (internal/moodle)

### 2.1 Transport
- **POST** `application/x-www-form-urlencoded` to `/webservice/rest/server.php`.
  The token goes in the body, not the URL, so it never ends up in `url.Error`, proxy logs or history.
- Arrays are sent as `courseids[0]=…&courseids[1]=…`, via the helper `params.IntList("courseids", ids)`.
- `http.Client{Timeout: 20s}` for REST. File downloads use a separate client: 5 min timeout
  for the response body and 20 s for headers. Otherwise a 50 MB PDF would never finish.
- **Function allowlist.** `Call` rejects (`ErrNotAllowed`) any `wsfunction` not on the list.
  A test also checks that the list contains no names matching
  `_(submit|save|add|update|delete|create|set|send|mark|edit|remove|toggle)_`.
- **Global semaphore of 5 in-flight requests** inside `Client`. The limit is shared by all
  tools. If `errgroup.SetLimit(5)` were only set inside a single tool, two parallel calls
  would produce 10 requests.

### 2.2 Retry
| Situation | Retry? |
|---|---|
| network error (dial, reset, EOF), except ctx cancellation | once |
| HTTP 5xx | once |
| 20 s timeout | once (new deadline) |
| HTTP 4xx, Moodle exception, decoding error | no |
| `ctx.Done()` | no, return `ctx.Err()` immediately |

Backoff: 300 ms ± 30 % jitter, set via `Config.RetryDelay` (0 in tests).

### 2.3 Response decoding
1. The body is read in full with a 32 MB limit (`io.LimitReader`).
2. If the body does not start with `{` or `[` (a maintenance HTML page or a
   proxy response), `ErrUnexpectedResponse` is returned with the first 200 characters of the body after redaction.
3. If it is an object, it is first trial-decoded into `struct{Exception, ErrorCode, Message, DebugInfo *string}`.
   If `exception != nil`, a `*moodle.Error` is returned.
4. Otherwise `json.Unmarshal(body, out)`.

### 2.4 Error classification (by `errorcode`; the site's messages are in German)
| errorcode | sentinel | what the user sees | fatal for the request* |
|---|---|---|---|
| `invalidtoken` | `ErrInvalidToken` | "The Moodle token is invalid or revoked. Create a new one: Profil → "Sicherheitsschlüssel" (security keys), update MOODLE_TOKEN." | yes |
| `accessexception` | `ErrAccessDenied` | "Function X is not available to the moodle_mobile_app service (access control)." | yes |
| `requireloginerror` | `ErrNotAccessible` | "The course or activity is not accessible (hidden or restricted)." | no |
| `sitemaintenance` | `ErrMaintenance` | "Moodle is in maintenance mode." | yes |
| other | `*Error` | `moodle: <fn>: <errorcode>: <message>` | no |

\* "Fatal" means the fan-out is aborted (`errgroup` cancels the context) and the tool
returns a single error. A non-fatal error for one course becomes a line
"⚠ course X: not accessible" in the output; the other courses are still shown.
Helper: `moodle.IsFatal(err) bool`.

`*Error` implements `Is(target)`, so `errors.Is(err, moodle.ErrInvalidToken)` works.

### 2.5 Wire types
- **Only the fields we use** are declared. The rest are ignored. The wire type thus
  doubles as a contract: it shows what we rely on.
- Moodle is inconsistent with types. Examples from real responses:
  `visible: 1` next to `hidden: false`; `progress: null`; `lastaccess` may be empty. Hence:
  - `moodle.Bool` accepts `true/false/0/1/"0"/"1"/null`;
  - `moodle.Time` accepts unix seconds; `0` and `null` give the zero time (`IsZero()`);
  - `*float64` for `progress`, `graderaw`.
- Warnings (`warnings[]`) are not lost: methods return them as a second value.

### 2.6 Client methods
```go
SiteInfo(ctx) (*SiteInfo, error)
UserCourses(ctx, userID) ([]Course, error)
CourseContents(ctx, courseID) ([]Section, error)
ActionEvents(ctx, from, to time.Time) ([]Event, error)          // pagination via aftereventid, limitnum=50
Assignments(ctx, courseIDs []int) ([]CourseAssignments, []Warning, error)  // ONE call for all courses
SubmissionStatus(ctx, assignID) (*SubmissionStatus, error)
GradeItems(ctx, courseID, userID) ([]GradeItem, error)
Forums(ctx, courseIDs []int) ([]Forum, error)                    // one call for all courses
Discussions(ctx, forumID, perPage) ([]Discussion, error)         // sortorder=3 (CREATED_DESC), page=0
Download(ctx, fileURL, dest string) (Downloaded, error)
```

## 3. Cache (internal/cache)

- **Moodle data is cached, not rendered text.** Rendering runs on every call,
  so "in 4 days" is always computed from the current moment.
- Implemented as the decorator `cache.Source`, which implements `study.Source` on top of `moodle.Client`.
- Generic core: `TTL[K,V]` with a replaceable clock and per-key `singleflight`: 5 parallel
  requests for the same course produce one HTTP call. Errors are not cached.
- Cache bypass: `refresh=true` reloads the data and **overwrites** the entry (rather than just reading past the cache).

| Key | TTL |
|---|---|
| `siteinfo` | process lifetime (userid is needed), retried on error |
| `courses` | 15 min |
| `contents:<courseid>` | 15 min |
| `forums:<sorted ids>` | 15 min |
| `events:<from-day>:<to-day>` | 5 min |
| `assignments:<sorted ids>` | 5 min |
| `subst:<assignid>` | 5 min |
| `discussions:<forumid>` | 5 min |
| grades | **not cached**: always a fresh request |

`study` receives a `FetchedAt` timestamp from `Source`. If the data is older than 1 min, render appends
a line such as "_Cached data from 7 min ago; use refresh=true for fresh data._" so it is clear when `refresh` is needed.

## 4. Domain models (internal/study)

All times are stored as `time.Time` in Europe/Vienna, strings are already free of HTML, empty
means "do not output".

```go
type Course struct { ID int; Name, Short string /* Short="" if equal to Name */
    Start, End time.Time; Progress *float64; Past bool; URL string }

type Section struct { Num int; Name, Summary string; Items []Item; Hidden int /* number of hidden items */ }
type Item struct { CMID int; Kind, Name, Description, URL string; Files []File; Sub *Section /* subsection */ }
type File struct { Name, URL string /* browser URL: /pluginfile.php/… without /webservice and without token */; Size int64; MIME string }

type Deadline struct {
    Key      DeadlineKey   // {Module string; Instance int}: deduplication key
    Course   CourseRef
    Title, Kind, URL string
    Due      time.Time     // final due date after all rules
    Sources  SourceSet     // calendar | assign (for debugging and tests)
    Status   Submission    // Unknown | NotSubmitted | Draft | Submitted | Reopened | NotApplicable
    Overdue  bool
}

type Grade struct { Course CourseRef; Item, Kind, Display string; Max float64; Percent *float64; Feedback string; GradedAt time.Time }
type Announcement struct { Course CourseRef; Subject, Author, Text, URL string; Posted time.Time; Unread bool }
type Hit struct { Course CourseRef; SectionPath []string; Item Item; Field MatchField; Snippet string }
```

## 5. Data flow per tool

Common step almost everywhere: `activeCourses()` = `Source.Courses()` → filter `!Past`.
Past rule: `hidden || completed || (!End.IsZero() && End < now)`.

### moodle_courses(include_past, refresh)
`Courses` → filter → sort by Name → render.

### moodle_deadlines(days=14, include_overdue=true, refresh)
```
window = [now, end of day (now + days) in Vienna]
overdueWindow = [now − 7d, now)                       ← only for NOT submitted assign
        ┌─ ActionEvents(window.from − 7d, window.to) ──┐
parallel┤                                              ├→ Merge → [assign: SubmissionStatus ×N (≤5)] → filter → sort → render
        └─ Assignments(activeIDs)  (one call) ─────────┘
```
Merge rules (key `(modulename, instance)`; for assign `instance == assignment.id`):
1. A calendar event and an assignment with the same key produce one entry. Title and URL come from
   the calendar (it accounts for overrides), the course from the assignment.
2. **Due date**: `extensionduedate` from the submission status > calendar `timesort` > assignment `duedate`.
   The calendar accounts for individual overrides, so it takes precedence over `duedate`.
3. An assignment **without an event** is kept. This matters: the assign action event disappears after
   submission, and without the second source a submitted assignment would simply vanish from the list.
4. `duedate == 0` (no due date) is not a deadline and is dropped.
5. Non-assign events (quiz, choice, …) get status `NotApplicable`; no submission is requested for them.
6. "No access rights" `warnings` become a footnote "Hidden or not yet available: N assignments."

Status: `lastattempt.submission` (or `teamsubmission` if team submissions are enabled) → `status`:
`new`→"❌ not submitted", `draft`→"📝 draft, not submitted", `submitted`→"✅ submitted", `reopened`→"↩️ reopened, submit again".
If `graded` — "✅ submitted, graded".

Sorting: `Due`, then course, then title. Overdue items form a separate block at the top.

### moodle_course_contents(course, refresh)
`ResolveCourse(q)` → `CourseContents(id)` → `BuildTree`:
- Sections with `component == "mod_subsection"` are removed from the top level and attached to the
  subsection module via `module.customdata.sectionid == section.id` (verified on real course 12326).
- `uservisible == false` → `Section.Hidden++`, the module is not output.
- Empty sections (no visible modules and no summary) are not output.
- `label` is output as text (cleaned, up to 200 characters).

**ResolveCourse(q)**: `q` all digits → exact ID (only among the user's own courses) →
normalized full-name match → substring → all tokens of q as substrings.
The first step that yields exactly 1 result returns. More than 1 result gives `*AmbiguousError{Candidates}`
(render shows the list with ids). 0 gives `*NotFoundError` with the list of all active courses.
Active courses are searched first; with 0 matches, past courses are searched (and marked as such).
Normalization: lower case, NFD without diacritics, `ß→ss`, `ä→a` (and `ae→a`, so that "Pruefung" finds "Prüfung"), whitespace collapsed.

### moodle_search(query, refresh)
`activeCourses` → `CourseContents ×N` (errgroup ≤5, cached) → flat index → match.
Fields and weights: module name 3, file name 2, section name 1, description 1. All query tokens
must occur (AND). Top 20, then "N more matches, narrow the query".

### moodle_grades(course="")
One course: `GradeItems(id, userid)`. All courses: fan-out ≤5.
- `gradeishidden` → skipped.
- **The percentage is computed by us**: `(graderaw − grademin)/(grademax − grademin)`, and printed
  with a decimal dot (`87.5%`). `percentageformatted` is localized by the site ("87,50 %") and is empty for scales.
- `Display = gradeformatted` (works for scales/letters too) and keeps the site's own notation. "-" means "not graded".
- A course with no grades and no feedback at all collapses into a single line "no grades yet".
- The course total (`itemtype=course`) is output as the last line.

### moodle_announcements(days=7, refresh)
`Forums(activeIDs)` → `type == "news"` → `Discussions(forumid, perPage=10)` ×N (≤5)
→ `created ≥ now − days` (or `modified` if the post was edited) → sort descending → render.
Text: StripHTML, truncated to 1500 characters + "… [in full](url)". Pinned posts are marked 📌.
URL: `/mod/forum/discuss.php?d=<discussion>`.

### moodle_download(fileurl, dest="")
```
parse → host == host(MOODLE_URL)? otherwise REFUSE (the token never goes to a foreign host)
      → path contains /pluginfile.php/ ? otherwise refuse
      → /pluginfile.php/… → /webservice/pluginfile.php/…  (already webservice — unchanged)
      → remove token from the query if present; add our own
      → GET (separate client) → status 200? JSON body → parse as a Moodle error
      → name: Content-Disposition → last path segment (url-decoded) → sanitize
      → write to a temp file in dest → fsync → rename (atomic, no broken files)
```
- `dest`: empty → `$MOODLE_DOWNLOAD_DIR` or `~/Downloads/moodle`. If `dest` is an existing
  directory or ends with `/`, the file is placed inside it. Otherwise `dest` is treated as the full file path.
- If the file already exists: when size and `Last-Modified` match, "already downloaded" is returned;
  otherwise the file is written as `name (1).ext`.
- sanitize: base name only, no `..`, `/`, control characters, length ≤ 200.
- Returns: local path + size (B/KB/MB). The URL in the response never contains the token.

### moodle_whoami()
`SiteInfo` → name, login, site, version, number of functions. Plus a **check of which
functions the server needs are missing**. This is the first tool for diagnostics.

## 6. Time and text (internal/textfmt)

- `Vienna` is loaded via `time.LoadLocation`. `time/tzdata` is embedded in the binary so
  it does not depend on the system time zone database.
- `Date(t)` → `2026-10-07 17:15 (Wednesday)`.
- `Relative(t, now)` counts **calendar days in Vienna**, not 24 h periods:
  under 1 h → "in 40 min", same day → "today, in 3 h", 1 day → "tomorrow",
  N days → "in N days" (singular "in 1 day" via the plural rules below).
  In the past: "yesterday", "3 days ago".
  The switch to winter time on 2026-10-25 is part of the test cases.
- **Output language and the message printer** (`lang.go`). All user-facing text is English and is
  printed through a `golang.org/x/text/message` printer: `textfmt.P()` returns it, `SetLanguage(tag)`
  selects the language (English by default). The English source strings are the catalog keys.
  Counted phrases ("in %d days", "%d days ago", "%d matches", "%d assignments", …) are registered
  with CLDR plural forms, so English gets "1 day" / "2 days". Another language is added by
  registering a catalog for the same keys and calling `SetLanguage`, without touching output code.
  Identifiers such as course ids are passed as strings so the printer does not localize them as numbers
  (no digit grouping).
- `StripHTML` uses the `golang.org/x/net/html` tokenizer, not a regex:
  block tags and `<br>` → newline, `<li>` → "- ", entities are decoded, `&nbsp;` → space,
  whitespace is collapsed, `<script>/<style>` are removed entirely. Moodle multilang
  (`<span lang="xx" class="multilang">`): `de` is taken if present, otherwise the first variant.
- `Truncate(s, n)` cuts by runes, not bytes, and appends "…".

## 7. Output format (internal/render)

General rules: markdown; every entity has an ID so the model can make the next call;
empty fields are not output; no unix timestamps; a limit of about 12,000 characters per response with a tail
"… N more lines not shown — narrow the request.". All text goes through `textfmt.P()` (section 6), so
the output is English and counted phrases use the correct plural form. Percentages computed by the
server use a decimal dot (`87.5%`); grades formatted by Moodle keep the site's notation.
File sizes are shown as B/KB/MB; search hits inside files cite "p. N" (PDF pages) or "slide N" (pptx).

```markdown
## Deadlines until 2026-10-16 (14 days)

**Overdue and not submitted**
- 2026-09-30 23:59 (Wednesday) — 2 days ago · Programming and Data Processing · [Homework R0](…) · assignment · ❌ not submitted

- 2026-10-05 23:59 (Monday) — in 3 days · Refresher on Unix Shells and LaTeX · [Quiz LaTeX Basics](…) · quiz
- 2026-10-07 17:15 (Wednesday) — in 5 days · Programming and Data Processing · [Homework R1](…) · assignment · ✅ submitted
- 2026-10-10 10:00 (Saturday) — in 8 days · Programming and Data Processing · [Homework R2](…) · assignment · ❌ not submitted

_Hidden or not yet available: 1 assignment._
```

The prefix `(DAT_WS2026_1)` repeats across all courses, so lists strip the **common
prefix of all courses** (an algorithm, not a hardcoded string). `moodle_courses`
shows the full name.

### File links
The main way to work with materials is **a link the user opens in the browser**
(they are logged in to Moodle). So throughout the output:
- a module links to its page `…/mod/<type>/view.php?id=<cmid>`;
- a file has a **browser** link `…/pluginfile.php/…`: `/webservice/` is removed, `forcedownload`
  is removed (a PDF opens in a tab), and there is never a token;
- `moodle_download` remains for cases where the file is needed locally (for example, to read its
  contents); it accepts both link forms.

## 8. Errors at the MCP boundary

- Errors from `study` are returned as `CallToolResult{IsError: true, Content: [text]}`,
  not as a protocol error. This way the model sees a human-readable reason and can react
  (for example, suggest renewing the token).
- `AmbiguousError` is not an error: it is a normal result with a list of candidates.
- All error texts pass through `Redact`.

## 9. Security and logs

- The token exists only in `moodle.Client` (unexported field) and in one place that builds the download URL.
- `Redact(s)` replaces the token both raw and URL-encoded. Applied in: the slog handler
  (every attribute), error texts, the download response, ErrUnexpectedResponse.
- A `url.Error` from download contains the URL with the token, so it is wrapped before being returned upward.
- Logs: `slog` to stderr; fields `fn, attempt, status, dur_ms, cache=hit|miss|refresh, bytes`.
  Request parameters are not logged. The level is set via `MOODLE_LOG_LEVEL` (default info).
- Stdout is used by the MCP protocol. There is not a single `fmt.Print` in the code — enforced by the `forbidigo` linter.

## 10. Configuration and startup

| env | required | default |
|---|---|---|
| `MOODLE_URL` | yes | — |
| `MOODLE_TOKEN` | yes | — |
| `MOODLE_DOWNLOAD_DIR` | no | `~/Downloads/moodle` |
| `MOODLE_LOG_LEVEL` | no | `info` |

Moodle is **not called** at startup: only the presence of env vars and URL validity are checked.
If the server failed at startup, Claude Code would only show "Connection closed".
It is better to start and return a clear error from the first tool call.
The version is set via `-ldflags "-X main.version=…"`.

## 11. Files

```
cmd/moodle-mcp/main.go            env, graph assembly, stdio
internal/moodle/
  client.go        Config, New, Call, retry, semaphore, allowlist
  errors.go        Error, sentinels, IsFatal, classification
  decode.go        Bool, Time, exception parsing
  types.go         wire types
  api.go           typed methods
  download.go      ToWebserviceURL, Download, sanitize, atomic write
  redact.go        Redact, RedactingHandler
  allowlist.go     function list
internal/cache/
  ttl.go           TTL[K,V] + singleflight
  source.go        study.Source decorator
internal/study/
  source.go        interface Source
  service.go       Service, New, activeCourses, fan-out helper
  model.go         domain models
  resolve.go       ResolveCourse, normalize
  deadlines.go     Merge, status, windows
  contents.go      BuildTree
  search.go        index and ranking
  grades.go  announcements.go  download.go  whoami.go
internal/render/   one file per tool + common.go (limit, course prefix)
internal/textfmt/  date.go (Date, Relative) html.go lang.go (P, SetLanguage, plurals)
internal/tools/    register.go (AddTool ×8), inputs.go (input structs with jsonschema tags)
testdata/moodle/<wsfunction>.<scenario>.json   response fixtures
testdata/errors/<errorcode>.json               error fixtures
testdata/golden/<tool>.<scenario>.md           expected output
docs/  Makefile  README.md  .gitignore  .golangci.yml
```

## 12. Tests

**Conventions.** Table tests `[]struct{name; …}` + `t.Run`, `t.Parallel()` wherever there is no
global state. Everything runs with `-race`. No network calls at all: the base URL
always comes from `httptest.Server`.

**Fake Moodle** (`internal/moodletest`):
```go
srv := moodletest.New(t,
    moodletest.Fixture("core_enrol_get_users_courses", "real"),
    moodletest.Route("mod_assign_get_submission_status", func(p url.Values) moodletest.Resp {
        return moodletest.File("synthetic-" + statusByID[p.Get("assignid")])
    }),
    moodletest.Sequence("core_course_get_contents", moodletest.HTTP(502), moodletest.File("real-12308")),
)
srv.Calls("core_course_get_contents")   // counter for checking cache and singleflight
srv.Requests()                          // full requests: method, token in the body rather than the URL
```
Fixtures are looked up as `<wsfunction>.<scenario>.json`. If no route is set for a function,
the test fails with a clear message (rather than receiving a 404).

| Level | What | How |
|---|---|---|
| moodle | decoding of every fixture; exception → sentinel (table over `testdata/errors/*`); HTML body; 5xx→retry→ok; 5xx×2→err; 4xx without retry; timeout; ctx cancellation; allowlist; Bool/Time | httptest |
| moodle/download | URL normalization (table: pluginfile, webservice, foreign host, no pluginfile, with ?token=, with forcedownload); name from Content-Disposition; sanitize `../../etc`; atomicity; collisions | httptest + `t.TempDir()` |
| redact | token in URL, URL-encoded, in error text, in logs (slog buffer), in download output | property: `assertNoToken(t, everyOutput)` |
| cache | hit/miss/expire with a fake clock; refresh overwrites; errors not cached; singleflight (10 goroutines → 1 load) | fake clock, no sleep |
| textfmt | Date/Relative/plural forms/DST/midnight; StripHTML (table of ~20 cases from real descriptions) | pure |
| study | ResolveCourse (id, substring, case, umlauts, ambiguity, past); Merge (both sources, calendar only, assign only, due=0, due-date precedence, extension, outside window, overdue); BuildTree with subsections; search/ranking; partial error policy | fake Source, clock `2026-10-02 12:00 Vienna` |
| render | output of every tool | golden files, `go test ./... -update` |
| tools (e2e) | 8 tools via `mcp.NewInMemoryTransports()`: list_tools (schemas), call_tool → golden; invalidtoken → `IsError` with a clear text | httptest + real client + in-memory MCP |

**Makefile**: `build`, `test` (`-race -count=1`), `cover`, `lint` (golangci-lint:
govet, staticcheck, errcheck, gosec, forbidigo), `golden` (`-update`).

## 12a. Additions after the first version

**Search inside files** (`moodle_search(in_files=true)`):
- `internal/extract` — pure "bytes → text" functions: PDF (`ledongthuc/pdf`, pure Go; lines and
  spaces are reconstructed from glyph coordinates in stream order), HTML, docx/pptx (XML inside
  zip), ipynb, plain text and source code, zip (one level of nesting, 64 MB budget per archive, ≤500 entries).
  A parser failure is an error, not a panic (recover).
- `moodle.Client.FetchFile` reads a pluginfile into memory with a limit; same URL validation as download.
- `cache.Source.FileText` caches text forever under the key URL + size + mtime: a changed file
  is re-read automatically.
- Matching: words are searched as substrings in normalized text with spaces (a missing space
  in "offcampus" does not matter); words of 6+ letters are also searched in text without spaces ("di ff erent"). Shorter
  words cannot be: "ssh" would match in "cla**ss h**ierarchy".
- Per-file result: pages/slides/archive entries containing all words (parts with only some
  words — only if there are no complete ones), cited as "p. N" / "slide N", plus a quote — the line with the most words.

**HTTP transport** (`internal/server`): one `*mcp.Server` for stdio and streamable HTTP. Protection
layers: DNS rebinding (SDK), `http.CrossOriginProtection`, optional bearer
(`auth.RequireBearerToken`, constant-time comparison). A non-loopback address without a token is refused
at startup. `/healthz` requires no authorization.

**Workaround for a go-sdk v1.8.0 bug:** `"arguments": null` combined with schema default values causes
a panic (nil map in jsonschema-go) and crashes the process. The `nullArguments` middleware replaces `null` with `{}`.

**Found by tests under `-race`:** `transform.Chain` from `x/text` is stateful — a shared instance
in `Normalize` failed under parallel calls; it is now created per call.

## 13. Decisions

1. The "N days" window runs **to the end of the N-th day in Vienna** (calendar days, correct across DST changes),
   not N×24 h: Moodle deadlines are usually at 23:59, and the result does not depend on the hour of the request.
2. Overdue unsubmitted assignments from the last 7 days are shown (`include_overdue=true`).
3. Grades are not cached.
4. The common prefix of course names is stripped in lists.
5. No `make contract` / live tests (live checks are manual, with a smoke client).
6. Go: `GOTOOLCHAIN=auto` (go-sdk v1.8 requires 1.25).
7. Output is English by default. Other languages are planned via the message catalog
   (`textfmt.SetLanguage` + translations of the English keys), with no changes to output code.
