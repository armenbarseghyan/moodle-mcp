// Package moodletest provides a fake Moodle web service for tests.
//
// Responses come from fixture files testdata/moodle/<wsfunction>.<scenario>.json
// and testdata/errors/<errorcode>.json. The server never talks to the network.
package moodletest

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Reporter receives problems found by the fake server. *testing.T and
// *testing.B satisfy it; dev tools can pass a logging implementation.
type Reporter interface {
	Helper()
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
	Cleanup(func())
}

// Token is the only token the fake server accepts. Any other token gets the
// real invalidtoken fixture back, like the real site would answer.
const Token = "f4k3t0k3n0123456789abcdef0123456"

// Resp is one scripted response.
type Resp struct {
	Status int // default 200
	Body   []byte
	Header http.Header
	Delay  time.Duration
	// Hangup closes the connection without a response (network error).
	Hangup bool
}

// Handler computes a response from the request's form parameters.
type Handler func(params url.Values) Resp

// Request is a recorded request.
type Request struct {
	Method   string
	Path     string
	RawQuery string
	Form     url.Values
	Header   http.Header
}

// Server is a fake Moodle site.
type Server struct {
	*httptest.Server
	t Reporter

	mu       sync.Mutex
	routes   map[string]Handler
	files    http.Handler
	calls    map[string]int
	requests []Request
	inFlight int
	maxIn    int
	rewrite  string
}

// Option configures a Server.
type Option func(*Server)

// New starts a fake Moodle and registers cleanup.
func New(t Reporter, opts ...Option) *Server {
	t.Helper()
	s := &Server{t: t, routes: map[string]Handler{}, calls: map[string]int{}}
	for _, o := range opts {
		o(s)
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

// Fixture answers fn with testdata/moodle/<fn>.<scenario>.json.
func Fixture(fn, scenario string) Option {
	return func(s *Server) {
		r := File(s.t, fn, scenario)
		s.routes[fn] = func(url.Values) Resp { return r }
	}
}

// Route answers fn with a custom handler.
func Route(fn string, h Handler) Option {
	return func(s *Server) { s.routes[fn] = h }
}

// Sequence answers fn with resps in order; the last one repeats.
func Sequence(fn string, resps ...Resp) Option {
	return func(s *Server) {
		var mu sync.Mutex
		i := 0
		s.routes[fn] = func(url.Values) Resp {
			mu.Lock()
			defer mu.Unlock()
			r := resps[min(i, len(resps)-1)]
			i++
			return r
		}
	}
}

// RewriteHost replaces origin (e.g. "https://moodle.fh-joanneum.at") with the
// fake server's own URL in every REST response, so that links in real
// fixtures point back at the fake server.
func RewriteHost(origin string) Option {
	return func(s *Server) { s.rewrite = origin }
}

// Files serves /webservice/pluginfile.php/... with h.
func Files(h http.Handler) Option {
	return func(s *Server) { s.files = h }
}

// File loads testdata/moodle/<fn>.<scenario>.json as a 200 response.
func File(t Reporter, fn, scenario string) Resp {
	t.Helper()
	return Resp{Body: read(t, filepath.Join("moodle", fn+"."+scenario+".json"))}
}

// ErrorFile loads testdata/errors/<code>.json as a 200 response (as Moodle does).
func ErrorFile(t Reporter, code string) Resp {
	t.Helper()
	return Resp{Body: read(t, filepath.Join("errors", code+".json"))}
}

// Raw builds a response from a status and body.
func Raw(status int, body string) Resp { return Resp{Status: status, Body: []byte(body)} }

// Calls returns how many times fn was requested.
func (s *Server) Calls(fn string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[fn]
}

// Requests returns all recorded requests.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.requests...)
}

// MaxInFlight returns the highest number of concurrently served requests.
func (s *Server) MaxInFlight() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.maxIn
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	s.mu.Lock()
	s.requests = append(s.requests, Request{
		Method: r.Method, Path: r.URL.Path, RawQuery: r.URL.RawQuery,
		Form: r.Form, Header: r.Header.Clone(),
	})
	s.inFlight++
	s.maxIn = max(s.maxIn, s.inFlight)
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.inFlight--
		s.mu.Unlock()
	}()

	switch {
	case strings.HasPrefix(r.URL.Path, "/webservice/pluginfile.php/"):
		s.mu.Lock()
		s.calls["pluginfile"]++
		s.mu.Unlock()
		if s.files == nil {
			s.t.Errorf("moodletest: unexpected file request %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		s.files.ServeHTTP(w, r)
	case r.URL.Path == "/webservice/rest/server.php":
		s.serveREST(w, r)
	default:
		s.t.Errorf("moodletest: unexpected path %s", r.URL.Path)
		http.NotFound(w, r)
	}
}

func (s *Server) serveREST(w http.ResponseWriter, r *http.Request) {
	fn := r.PostForm.Get("wsfunction")
	s.mu.Lock()
	s.calls[fn]++
	h, ok := s.routes[fn]
	s.mu.Unlock()

	if r.Method != http.MethodPost {
		s.t.Errorf("moodletest: %s called with %s, want POST", fn, r.Method)
	}
	if r.URL.Query().Has("wstoken") {
		s.t.Errorf("moodletest: %s: token sent in URL query", fn)
	}
	if r.PostForm.Get("moodlewsrestformat") != "json" {
		s.t.Errorf("moodletest: %s: moodlewsrestformat != json", fn)
	}

	var resp Resp
	switch {
	case r.PostForm.Get("wstoken") != Token:
		resp = ErrorFile(s.t, "invalidtoken")
	case !ok:
		s.t.Errorf("moodletest: no route for wsfunction %q", fn)
		resp = ErrorFile(s.t, "accessexception")
	default:
		resp = h(r.PostForm)
	}
	if s.rewrite != "" {
		resp.Body = bytes.ReplaceAll(resp.Body, []byte(s.rewrite), []byte(s.URL))
		resp.Body = bytes.ReplaceAll(resp.Body, []byte(strings.ReplaceAll(s.rewrite, "/", `\/`)), []byte(strings.ReplaceAll(s.URL, "/", `\/`)))
	}
	write(w, r, resp)
}

func write(w http.ResponseWriter, r *http.Request, resp Resp) {
	if resp.Delay > 0 {
		select {
		case <-time.After(resp.Delay):
		case <-r.Context().Done():
			return
		}
	}
	if resp.Hangup {
		if hj, ok := w.(http.Hijacker); ok {
			if conn, _, err := hj.Hijack(); err == nil {
				if tc, ok := conn.(*net.TCPConn); ok {
					_ = tc.SetLinger(0)
				}
				_ = conn.Close()
				return
			}
		}
	}
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	}
	status := resp.Status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_, _ = w.Write(resp.Body)
}

// TestdataDir returns the absolute path of the repository's testdata directory.
func TestdataDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata")
}

func read(t Reporter, rel string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(TestdataDir(), rel))
	if err != nil {
		t.Fatalf("moodletest: fixture: %v", err)
	}
	return b
}

// Fixtures lists the scenarios available for fn, for table-driven tests.
func Fixtures(fn string) []string {
	matches, _ := filepath.Glob(filepath.Join(TestdataDir(), "moodle", fn+".*.json"))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, strings.TrimSuffix(strings.TrimPrefix(filepath.Base(m), fn+"."), ".json"))
	}
	return out
}

// String implements fmt.Stringer for nicer failure messages.
func (r Request) String() string {
	return fmt.Sprintf("%s %s fn=%s", r.Method, r.Path, r.Form.Get("wsfunction"))
}
