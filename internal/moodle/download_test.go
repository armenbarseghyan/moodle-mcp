package moodle_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"moodle-mcp/internal/moodle"
	"moodle-mcp/internal/moodletest"
)

func TestToWebserviceURL(t *testing.T) {
	t.Parallel()
	c, err := moodle.New(moodle.Config{BaseURL: "https://moodle.example.test", Token: "tok"})
	if err != nil {
		t.Fatal(err)
	}
	sub, err := moodle.New(moodle.Config{BaseURL: "https://example.test/moodle", Token: "tok"})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		client *moodle.Client
		in     string
		want   string
	}{
		{"pluginfile", c,
			"https://moodle.example.test/pluginfile.php/318093/mod_resource/content/2/Syllabus.pdf",
			"https://moodle.example.test/webservice/pluginfile.php/318093/mod_resource/content/2/Syllabus.pdf"},
		{"already webservice", c,
			"https://moodle.example.test/webservice/pluginfile.php/1/mod_resource/content/0/a.pdf?forcedownload=1",
			"https://moodle.example.test/webservice/pluginfile.php/1/mod_resource/content/0/a.pdf?forcedownload=1"},
		{"existing token dropped", c,
			"https://moodle.example.test/webservice/pluginfile.php/1/x/a.pdf?token=OLD&forcedownload=1",
			"https://moodle.example.test/webservice/pluginfile.php/1/x/a.pdf?forcedownload=1"},
		{"escaped name kept", c,
			"https://moodle.example.test/webservice/pluginfile.php/329218/mod_resource/content/0/%5BPDP%5D%20-%20Demo.zip",
			"https://moodle.example.test/webservice/pluginfile.php/329218/mod_resource/content/0/%5BPDP%5D%20-%20Demo.zip"},
		{"http upgraded", c,
			"http://moodle.example.test/pluginfile.php/1/a.pdf",
			"https://moodle.example.test/webservice/pluginfile.php/1/a.pdf"},
		{"host case-insensitive", c,
			"https://MOODLE.example.test/pluginfile.php/1/a.pdf",
			"https://moodle.example.test/webservice/pluginfile.php/1/a.pdf"},
		{"site in subdirectory", sub,
			"https://example.test/moodle/pluginfile.php/1/a.pdf",
			"https://example.test/moodle/webservice/pluginfile.php/1/a.pdf"},
		{"foreign host", c, "https://evil.example/pluginfile.php/1/a.pdf", ""},
		{"lookalike host", c, "https://moodle.example.test.evil.example/pluginfile.php/1/a.pdf", ""},
		{"other port", c, "https://moodle.example.test:8443/pluginfile.php/1/a.pdf", ""},
		{"credentials", c, "https://u:p@moodle.example.test/pluginfile.php/1/a.pdf", ""},
		{"not a file url", c, "https://moodle.example.test/mod/resource/view.php?id=1", ""},
		{"tokenpluginfile", c, "https://moodle.example.test/tokenpluginfile.php/abc/1/a.pdf", ""},
		{"traversal", c, "https://moodle.example.test/pluginfile.php/../login/index.php", ""},
		{"outside subdirectory", sub, "https://example.test/pluginfile.php/1/a.pdf", ""},
		{"javascript", c, "javascript:alert(1)", ""},
		{"garbage", c, "::::", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := tt.client.ToWebserviceURL(tt.in)
			if tt.want == "" {
				if !errors.Is(err, moodle.ErrForeignURL) {
					t.Fatalf("got %v, %v; want ErrForeignURL", got, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.String() != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}

const pdfBody = "%PDF-1.7 fake"

// fileServer serves a file and checks that the token arrives in the query.
func fileServer(t *testing.T, header http.Header, body string, status int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != moodletest.Token {
			t.Errorf("token missing in file request")
		}
		for k, v := range header {
			w.Header()[k] = v
		}
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
}

func TestDownload(t *testing.T) {
	t.Parallel()
	lastMod := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		header   http.Header
		urlPath  string
		dest     func(dir string) string
		wantName string
	}{
		{
			name:     "name from Content-Disposition",
			header:   http.Header{"Content-Disposition": {`attachment; filename="Syllabus DAT26.pdf"`}},
			urlPath:  "/pluginfile.php/1/mod_resource/content/2/x.pdf",
			dest:     func(d string) string { return d },
			wantName: "Syllabus DAT26.pdf",
		},
		{
			name:     "RFC 5987 filename",
			header:   http.Header{"Content-Disposition": {`attachment; filename*=UTF-8''%C3%9Cbung%201.pdf`}},
			urlPath:  "/pluginfile.php/1/x.pdf",
			dest:     func(d string) string { return d },
			wantName: "Übung 1.pdf",
		},
		{
			name:     "name from URL path",
			urlPath:  "/webservice/pluginfile.php/329218/mod_resource/content/0/%5BPDP%5D%20-%20Demo.zip",
			dest:     func(d string) string { return d },
			wantName: "[PDP] - Demo.zip",
		},
		{
			name:     "traversal in header is neutralised",
			header:   http.Header{"Content-Disposition": {`attachment; filename="../../../etc/passwd"`}},
			urlPath:  "/pluginfile.php/1/x.pdf",
			dest:     func(d string) string { return d },
			wantName: "passwd",
		},
		{
			name:     "explicit file path",
			urlPath:  "/pluginfile.php/1/x.pdf",
			dest:     func(d string) string { return filepath.Join(d, "sub", "renamed.pdf") },
			wantName: filepath.Join("sub", "renamed.pdf"),
		},
		{
			name:     "dir with trailing slash is created",
			urlPath:  "/pluginfile.php/1/x.pdf",
			dest:     func(d string) string { return filepath.Join(d, "new") + "/" },
			wantName: filepath.Join("new", "x.pdf"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := tt.header.Clone()
			if h == nil {
				h = http.Header{}
			}
			h.Set("Last-Modified", lastMod.Format(http.TimeFormat))
			srv := moodletest.New(t, moodletest.Files(fileServer(t, h, pdfBody, 0)))
			c := newClient(t, srv)
			dir := t.TempDir()

			res, err := c.Download(context.Background(), srv.URL+tt.urlPath, tt.dest(dir))
			if err != nil {
				t.Fatal(err)
			}
			want := filepath.Join(dir, tt.wantName)
			if res.Path != want {
				t.Errorf("path = %s, want %s", res.Path, want)
			}
			b, err := os.ReadFile(res.Path)
			if err != nil || string(b) != pdfBody || res.Size != int64(len(pdfBody)) {
				t.Errorf("content = %q (%v), size %d", b, err, res.Size)
			}
			fi, _ := os.Stat(res.Path)
			if !fi.ModTime().Equal(lastMod) {
				t.Errorf("mtime = %v, want Last-Modified %v", fi.ModTime(), lastMod)
			}
			if fi.Mode().Perm() != 0o644 {
				t.Errorf("mode = %v", fi.Mode().Perm())
			}
			assertNoToken(t, "result URL", res.URL)
			assertNoToken(t, "result path", res.Path)
			leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(res.Path), ".moodle-dl-*"))
			if len(leftovers) != 0 {
				t.Errorf("temp files left: %v", leftovers)
			}
		})
	}
}

func TestDownload_ExistingFile(t *testing.T) {
	t.Parallel()
	lastMod := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	h := http.Header{"Last-Modified": {lastMod.Format(http.TimeFormat)}}
	srv := moodletest.New(t, moodletest.Files(fileServer(t, h, pdfBody, 0)))
	c := newClient(t, srv)
	dir := t.TempDir()
	u := srv.URL + "/pluginfile.php/1/a.pdf"

	first, err := c.Download(context.Background(), u, dir)
	if err != nil || first.Reused {
		t.Fatalf("first: %+v %v", first, err)
	}
	second, err := c.Download(context.Background(), u, dir)
	if err != nil || !second.Reused || second.Path != first.Path {
		t.Fatalf("identical file must be reused: %+v %v", second, err)
	}

	// Same name, different content: keep the old file, write "a (1).pdf".
	if err := os.WriteFile(first.Path, []byte("older version, different size"), 0o644); err != nil {
		t.Fatal(err)
	}
	third, err := c.Download(context.Background(), u, dir)
	if err != nil || third.Reused || filepath.Base(third.Path) != "a (1).pdf" {
		t.Fatalf("collision: %+v %v", third, err)
	}
}

func TestDownload_Errors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		header   http.Header
		body     string
		status   int
		sentinel error
		status2  int
	}{
		{"json invalidtoken with 403", http.Header{"Content-Type": {"application/json"}},
			`{"error":"Ungültiges Token","errorcode":"invalidtoken","stacktrace":null}`, 403, moodle.ErrInvalidToken, 0},
		{"json error with 200", http.Header{"Content-Type": {"application/json"}},
			`{"error":"Kein Zugriff","errorcode":"requireloginerror"}`, 200, moodle.ErrNotAccessible, 0},
		{"plain 404", nil, "not found", 404, nil, 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := moodletest.New(t, moodletest.Files(fileServer(t, tt.header, tt.body, tt.status)))
			c := newClient(t, srv)
			dir := t.TempDir()
			_, err := c.Download(context.Background(), srv.URL+"/pluginfile.php/1/a.pdf", dir)
			if err == nil {
				t.Fatal("want error")
			}
			if tt.sentinel != nil && !errors.Is(err, tt.sentinel) {
				t.Errorf("err = %v, want %v", err, tt.sentinel)
			}
			var hErr *moodle.HTTPError
			if tt.status2 != 0 && (!errors.As(err, &hErr) || hErr.Status != tt.status2) {
				t.Errorf("err = %v, want HTTP %d", err, tt.status2)
			}
			assertNoToken(t, "error", err.Error())
			if entries, _ := os.ReadDir(dir); len(entries) != 0 {
				t.Errorf("nothing must be written on error, got %v", entries)
			}
		})
	}
}

func TestDownload_JSONFileIsNotAnError(t *testing.T) {
	t.Parallel()
	h := http.Header{"Content-Type": {"application/json"}}
	srv := moodletest.New(t, moodletest.Files(fileServer(t, h, `{"data":[1,2,3]}`, 200)))
	res, err := newClient(t, srv).Download(context.Background(), srv.URL+"/pluginfile.php/1/data.json", t.TempDir())
	if err != nil || res.Size == 0 {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestDownload_ForeignHostNeverRequested(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	other := moodletest.New(t, moodletest.Files(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) })))
	srv := moodletest.New(t)
	c := newClient(t, srv)
	_, err := c.Download(context.Background(), strings.Replace(other.URL, "127.0.0.1", "localhost", 1)+"/pluginfile.php/1/a.pdf", t.TempDir())
	if !errors.Is(err, moodle.ErrForeignURL) {
		t.Fatalf("err = %v", err)
	}
	if hits.Load() != 0 || len(srv.Requests()) != 0 {
		t.Fatal("no request may be sent for a foreign URL")
	}
}

func TestDownload_RedirectToForeignHostRefused(t *testing.T) {
	t.Parallel()
	var leaked atomic.Int32
	evil := moodletest.New(t, moodletest.Files(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked.Add(1)
	})))
	evilURL := strings.Replace(evil.URL, "127.0.0.1", "localhost", 1)
	srv := moodletest.New(t, moodletest.Files(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, evilURL+"/webservice/pluginfile.php/1/a.pdf?"+r.URL.RawQuery, http.StatusFound)
	})))
	_, err := newClient(t, srv).Download(context.Background(), srv.URL+"/pluginfile.php/1/a.pdf", t.TempDir())
	if !errors.Is(err, moodle.ErrForeignURL) {
		t.Fatalf("err = %v", err)
	}
	if leaked.Load() != 0 {
		t.Fatal("token leaked to foreign host via redirect")
	}
	assertNoToken(t, "error", err.Error())
}

func TestDownload_NetworkErrorDoesNotLeakToken(t *testing.T) {
	t.Parallel()
	srv := moodletest.New(t, moodletest.Files(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, _ := w.(http.Hijacker)
		conn, _, _ := hj.Hijack()
		conn.Close()
	})))
	_, err := newClient(t, srv).Download(context.Background(), srv.URL+"/pluginfile.php/1/a.pdf", t.TempDir())
	if err == nil {
		t.Fatal("want error")
	}
	assertNoToken(t, "error", err.Error())
	if n := srv.Calls("pluginfile"); n != 2 {
		t.Errorf("calls = %d, want 2 (one retry)", n)
	}
}

func TestSanitizeFilename(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{"report.pdf", "report.pdf"},
		{"../../etc/passwd", "passwd"},
		{`..\..\windows\win.ini`, "win.ini"},
		{"a:b*c?.txt", "a_b_c_.txt"},
		{".hidden", "hidden"},
		{"..", "download"},
		{"", "download"},
		{"name\x00\x1f.pdf", "name.pdf"},
		{"Übung 1.pdf", "Übung 1.pdf"},
		{strings.Repeat("ü", 150) + ".pdf", strings.Repeat("ü", 100)},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			if got := moodle.SanitizeFilename(tt.in); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFetchFile(t *testing.T) {
	t.Parallel()
	h := http.Header{"Content-Disposition": {`attachment; filename="notes.md"`}}
	srv := moodletest.New(t, moodletest.Files(fileServer(t, h, "# Notes", 0)))
	c := newClient(t, srv)
	f, err := c.FetchFile(context.Background(), srv.URL+"/pluginfile.php/1/mod_page/content/3/index.html", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if f.Name != "notes.md" || string(f.Data) != "# Notes" || strings.Contains(f.URL, "token") {
		t.Errorf("file = %+v", f)
	}

	_, err = c.FetchFile(context.Background(), srv.URL+"/pluginfile.php/1/x.md", 3)
	if !errors.Is(err, moodle.ErrTooLarge) {
		t.Errorf("err = %v, want ErrTooLarge", err)
	}
	_, err = c.FetchFile(context.Background(), "https://evil.example/pluginfile.php/1/x.md", 10)
	if !errors.Is(err, moodle.ErrForeignURL) {
		t.Errorf("err = %v, want ErrForeignURL", err)
	}
}
