package moodle_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/armenbarseghyan/moodle-mcp/internal/moodle"
	"github.com/armenbarseghyan/moodle-mcp/internal/moodletest"
)

type clientOpt func(*moodle.Config)

func newClient(t *testing.T, srv *moodletest.Server, opts ...clientOpt) *moodle.Client {
	t.Helper()
	cfg := moodle.Config{
		BaseURL: srv.URL,
		Token:   moodletest.Token,
		Backoff: func(int) time.Duration { return 0 },
	}
	for _, o := range opts {
		o(&cfg)
	}
	c, err := moodle.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func withLogger(l *slog.Logger) clientOpt { return func(c *moodle.Config) { c.Logger = l } }
func withToken(tok string) clientOpt      { return func(c *moodle.Config) { c.Token = tok } }
func withTimeout(d time.Duration) clientOpt {
	return func(c *moodle.Config) { c.HTTPClient = &http.Client{Timeout: d} }
}

func assertNoToken(t *testing.T, what, s string) {
	t.Helper()
	if strings.Contains(s, moodletest.Token) {
		t.Errorf("%s leaks the token: %q", what, s)
	}
}

func TestNew(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		baseURL string
		token   string
		wantErr bool
	}{
		{"ok", "https://moodle.example.test", "tok", false},
		{"trailing slash", "https://moodle.example.test/", "tok", false},
		{"subpath", "https://example.test/moodle", "tok", false},
		{"empty token", "https://moodle.example.test", "", true},
		{"blank token", "https://moodle.example.test", "   ", true},
		{"no scheme", "moodle.example.test", "tok", true},
		{"ftp", "ftp://moodle.example.test", "tok", true},
		{"query", "https://moodle.example.test/?x=1", "tok", true},
		{"empty url", "", "tok", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := moodle.New(moodle.Config{BaseURL: tt.baseURL, Token: tt.token})
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCall_RequestShape(t *testing.T) {
	t.Parallel()
	srv := moodletest.New(t, moodletest.Fixture("core_webservice_get_site_info", "real"))
	c := newClient(t, srv)

	if _, err := c.SiteInfo(context.Background()); err != nil {
		t.Fatal(err)
	}
	reqs := srv.Requests()
	if len(reqs) != 1 {
		t.Fatalf("requests = %d, want 1", len(reqs))
	}
	r := reqs[0]
	if r.Method != http.MethodPost || r.Path != "/webservice/rest/server.php" {
		t.Errorf("request = %s %s", r.Method, r.Path)
	}
	if r.RawQuery != "" {
		t.Errorf("query string must be empty (token goes in the body), got %q", r.RawQuery)
	}
	if got := r.Form.Get("wstoken"); got != moodletest.Token {
		t.Errorf("wstoken = %q", got)
	}
	if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q", got)
	}
}

func TestCall_MoodleExceptions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		fixture  string
		sentinel error
		fatal    bool
	}{
		{"invalidtoken", moodle.ErrInvalidToken, true},
		{"accessexception", moodle.ErrAccessDenied, true},
		{"requireloginerror", moodle.ErrNotAccessible, false},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			t.Parallel()
			srv := moodletest.New(t, moodletest.Route("core_webservice_get_site_info",
				func(url.Values) moodletest.Resp { return moodletest.ErrorFile(t, tt.fixture) }))
			c := newClient(t, srv)

			_, err := c.SiteInfo(context.Background())
			if !errors.Is(err, tt.sentinel) {
				t.Fatalf("err = %v, want errors.Is %v", err, tt.sentinel)
			}
			var mErr *moodle.Error
			if !errors.As(err, &mErr) || mErr.ErrorCode != tt.fixture || mErr.Function != "core_webservice_get_site_info" {
				t.Errorf("err = %#v", err)
			}
			if moodle.IsFatal(err) != tt.fatal {
				t.Errorf("IsFatal = %v, want %v", !tt.fatal, tt.fatal)
			}
			if n := srv.Calls("core_webservice_get_site_info"); n != 1 {
				t.Errorf("calls = %d, exceptions must not be retried", n)
			}
		})
	}
}

func TestCall_WrongTokenGetsInvalidToken(t *testing.T) {
	t.Parallel()
	srv := moodletest.New(t, moodletest.Fixture("core_webservice_get_site_info", "real"))
	c := newClient(t, srv, withToken("wrong-token"))
	if _, err := c.SiteInfo(context.Background()); !errors.Is(err, moodle.ErrInvalidToken) {
		t.Fatalf("err = %v", err)
	}
}

func TestCall_UnknownExceptionIsGenericError(t *testing.T) {
	t.Parallel()
	srv := moodletest.New(t, moodletest.Sequence("core_webservice_get_site_info",
		moodletest.Raw(200, `{"exception":"moodle_exception","errorcode":"invalidrecord","message":"Datensatz fehlt"}`)))
	c := newClient(t, srv)
	_, err := c.SiteInfo(context.Background())
	var mErr *moodle.Error
	if !errors.As(err, &mErr) || mErr.ErrorCode != "invalidrecord" {
		t.Fatalf("err = %v", err)
	}
	for _, s := range []error{moodle.ErrInvalidToken, moodle.ErrAccessDenied, moodle.ErrNotAccessible} {
		if errors.Is(err, s) {
			t.Errorf("generic error must not match %v", s)
		}
	}
	if moodle.IsFatal(err) {
		t.Error("generic error must not be fatal")
	}
}

func TestCall_NotAllowedNeverHitsNetwork(t *testing.T) {
	t.Parallel()
	srv := moodletest.New(t)
	c := newClient(t, srv)
	for _, fn := range []string{
		"mod_assign_save_submission",
		"mod_assign_submit_for_grading",
		"core_user_update_users",
		"mod_forum_add_discussion_post",
		"core_calendar_delete_calendar_events",
		"",
	} {
		err := c.Call(context.Background(), fn, nil, nil)
		if !errors.Is(err, moodle.ErrNotAllowed) || !moodle.IsFatal(err) {
			t.Errorf("Call(%q) err = %v, want ErrNotAllowed", fn, err)
		}
	}
	if n := len(srv.Requests()); n != 0 {
		t.Fatalf("%d requests reached the server", n)
	}
}

func TestCall_UnexpectedResponse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
	}{
		{"html maintenance page", "<html><body>Wartungsmodus " + moodletest.Token + "</body></html>"},
		{"empty", ""},
		{"plain text", "error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := moodletest.New(t, moodletest.Sequence("core_webservice_get_site_info", moodletest.Raw(200, tt.body)))
			c := newClient(t, srv)
			_, err := c.SiteInfo(context.Background())
			if !errors.Is(err, moodle.ErrUnexpectedResponse) {
				t.Fatalf("err = %v", err)
			}
			assertNoToken(t, "error", err.Error())
		})
	}
}

func TestCall_MalformedJSON(t *testing.T) {
	t.Parallel()
	srv := moodletest.New(t, moodletest.Sequence("core_webservice_get_site_info", moodletest.Raw(200, `{"userid": "abc"`)))
	c := newClient(t, srv)
	if _, err := c.SiteInfo(context.Background()); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("err = %v", err)
	}
}

func TestCall_Retry(t *testing.T) {
	t.Parallel()
	ok := func(t *testing.T) moodletest.Resp { return moodletest.File(t, "core_webservice_get_site_info", "real") }
	tests := []struct {
		name      string
		resps     func(t *testing.T) []moodletest.Resp
		timeout   time.Duration
		wantErr   bool
		wantCalls int
	}{
		{"502 then ok", func(t *testing.T) []moodletest.Resp {
			return []moodletest.Resp{moodletest.Raw(502, "bad gateway"), ok(t)}
		}, 0, false, 2},
		{"503 twice", func(*testing.T) []moodletest.Resp {
			return []moodletest.Resp{moodletest.Raw(503, "unavailable")}
		}, 0, true, 2},
		{"network error then ok", func(t *testing.T) []moodletest.Resp {
			return []moodletest.Resp{{Hangup: true}, ok(t)}
		}, 0, false, 2},
		{"timeout then ok", func(t *testing.T) []moodletest.Resp {
			return []moodletest.Resp{{Delay: time.Second}, ok(t)}
		}, 100 * time.Millisecond, false, 2},
		{"404 not retried", func(*testing.T) []moodletest.Resp {
			return []moodletest.Resp{moodletest.Raw(404, "not found")}
		}, 0, true, 1},
		{"exception in 500 not retried", func(t *testing.T) []moodletest.Resp {
			r := moodletest.ErrorFile(t, "invalidtoken")
			r.Status = 500
			return []moodletest.Resp{r}
		}, 0, true, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := moodletest.New(t, moodletest.Sequence("core_webservice_get_site_info", tt.resps(t)...))
			opts := []clientOpt{}
			if tt.timeout > 0 {
				opts = append(opts, withTimeout(tt.timeout))
			}
			c := newClient(t, srv, opts...)
			_, err := c.SiteInfo(context.Background())
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if n := srv.Calls("core_webservice_get_site_info"); n != tt.wantCalls {
				t.Errorf("calls = %d, want %d", n, tt.wantCalls)
			}
		})
	}
}

func TestCall_HTTPErrorType(t *testing.T) {
	t.Parallel()
	srv := moodletest.New(t, moodletest.Sequence("core_webservice_get_site_info", moodletest.Raw(503, "down")))
	c := newClient(t, srv)
	_, err := c.SiteInfo(context.Background())
	var hErr *moodle.HTTPError
	if !errors.As(err, &hErr) || hErr.Status != 503 {
		t.Fatalf("err = %v", err)
	}
}

func TestCall_CanceledContextIsNotRetried(t *testing.T) {
	t.Parallel()
	srv := moodletest.New(t, moodletest.Sequence("core_webservice_get_site_info", moodletest.Resp{Delay: 2 * time.Second}))
	c := newClient(t, srv)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := c.SiteInfo(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
	if n := srv.Calls("core_webservice_get_site_info"); n != 1 {
		t.Errorf("calls = %d, want 1", n)
	}
}

func TestCall_MaxInFlight(t *testing.T) {
	t.Parallel()
	resp := moodletest.File(t, "core_webservice_get_site_info", "real")
	resp.Delay = 30 * time.Millisecond
	srv := moodletest.New(t, moodletest.Sequence("core_webservice_get_site_info", resp))
	c := newClient(t, srv) // default limit 5

	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if _, err := c.SiteInfo(context.Background()); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if got := srv.MaxInFlight(); got > 5 || got < 2 {
		t.Fatalf("max in flight = %d, want 2..5", got)
	}
}

func TestCall_LogsNeverContainToken(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	srv := moodletest.New(t,
		moodletest.Sequence("core_webservice_get_site_info",
			moodletest.Raw(502, "proxy error for token "+moodletest.Token),
			moodletest.Raw(200, "<html>"+moodletest.Token+"</html>")),
	)
	c := newClient(t, srv, withLogger(logger))
	_, err := c.SiteInfo(context.Background())
	if err == nil {
		t.Fatal("want error")
	}
	if buf.Len() == 0 {
		t.Fatal("expected log output")
	}
	assertNoToken(t, "log", buf.String())
	assertNoToken(t, "error", err.Error())
}
