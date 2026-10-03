package moodle

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	pluginfileSeg   = "/pluginfile.php/"
	wsPluginfileSeg = "/webservice/pluginfile.php/"
	maxFilenameLen  = 200
)

// Downloaded describes a file saved by Download.
type Downloaded struct {
	Path   string // absolute local path
	Size   int64
	URL    string // canonical file URL, never contains the token
	Reused bool   // an identical file already existed; nothing was written
}

// ToWebserviceURL validates that raw points to this site's pluginfile.php and
// returns the canonical /webservice/pluginfile.php URL without any token.
func (c *Client) ToWebserviceURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: cannot parse URL", ErrForeignURL)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || !strings.EqualFold(u.Hostname(), c.base.Hostname()) {
		return nil, fmt.Errorf("%w: host must be %s", ErrForeignURL, c.base.Hostname())
	}
	if u.Port() != c.base.Port() {
		return nil, fmt.Errorf("%w: unexpected port", ErrForeignURL)
	}
	if u.User != nil {
		return nil, fmt.Errorf("%w: URL must not contain credentials", ErrForeignURL)
	}

	p := u.EscapedPath()
	basePath := strings.TrimRight(c.base.EscapedPath(), "/")
	if !strings.HasPrefix(p, basePath+"/") {
		return nil, fmt.Errorf("%w: path outside the site", ErrForeignURL)
	}
	rest := strings.TrimPrefix(p, basePath)
	switch {
	case strings.HasPrefix(rest, wsPluginfileSeg):
	case strings.HasPrefix(rest, pluginfileSeg):
		rest = wsPluginfileSeg + strings.TrimPrefix(rest, pluginfileSeg)
	default:
		return nil, fmt.Errorf("%w: not a pluginfile.php URL", ErrForeignURL)
	}
	if strings.Contains(rest, "/../") || strings.HasSuffix(rest, "/..") {
		return nil, fmt.Errorf("%w: path traversal", ErrForeignURL)
	}

	q := u.Query()
	q.Del("token")
	out := &url.URL{
		Scheme:   c.base.Scheme, // never downgrade to http
		Host:     c.base.Host,
		RawQuery: q.Encode(),
	}
	if err := setEscapedPath(out, basePath+rest); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrForeignURL, err)
	}
	return out, nil
}

func setEscapedPath(u *url.URL, escaped string) error {
	unescaped, err := url.PathUnescape(escaped)
	if err != nil {
		return err
	}
	u.Path = unescaped
	u.RawPath = escaped
	return nil
}

// fileResponse is an open, validated pluginfile response.
type fileResponse struct {
	clean  *url.URL
	resp   *http.Response
	body   *bufio.Reader
	cancel context.CancelFunc
}

func (f *fileResponse) Close() {
	_ = f.resp.Body.Close()
	f.cancel()
}

// openFile validates rawURL, requests it with the token and checks the
// response. The caller must Close the result.
func (c *Client) openFile(ctx context.Context, rawURL string) (*fileResponse, error) {
	clean, err := c.ToWebserviceURL(rawURL)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, c.dlTimeout)
	withToken := *clean
	q := withToken.Query()
	q.Set("token", c.token)
	withToken.RawQuery = q.Encode()

	resp, err := c.getFile(ctx, clean.String(), withToken.String()) //nolint:bodyclose // owned by fileResponse, closed by Close
	if err != nil {
		cancel()
		return nil, err
	}
	f := &fileResponse{clean: clean, resp: resp, body: bufio.NewReader(resp.Body), cancel: cancel} //nolint:bodyclose // closed by fileResponse.Close
	if err := c.checkFileResponse(clean.String(), resp, f.body); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// File is a file read into memory by FetchFile.
type File struct {
	Name string // from Content-Disposition or the URL path, sanitised
	URL  string // canonical file URL, never contains the token
	Data []byte
}

// FetchFile reads a pluginfile URL into memory. Files larger than maxBytes
// are rejected with ErrTooLarge without being read completely.
func (c *Client) FetchFile(ctx context.Context, rawURL string, maxBytes int64) (File, error) {
	f, err := c.openFile(ctx, rawURL)
	if err != nil {
		return File{}, err
	}
	defer f.Close()
	if f.resp.ContentLength > maxBytes {
		return File{}, fmt.Errorf("%w: %d bytes", ErrTooLarge, f.resp.ContentLength)
	}
	data, err := io.ReadAll(io.LimitReader(f.body, maxBytes+1))
	if err != nil {
		return File{}, fmt.Errorf("moodle: read %s: %s", f.clean, c.Redact(err.Error()))
	}
	if int64(len(data)) > maxBytes {
		return File{}, fmt.Errorf("%w: more than %d bytes", ErrTooLarge, maxBytes)
	}
	return File{Name: filenameFrom(f.resp, f.clean), URL: f.clean.String(), Data: data}, nil
}

// Download fetches a pluginfile URL and stores it under dest.
// dest is a directory when it exists as one or ends with a path separator;
// otherwise it is the full target file path. The write is atomic.
func (c *Client) Download(ctx context.Context, rawURL, dest string) (Downloaded, error) {
	if dest == "" {
		if _, err := c.ToWebserviceURL(rawURL); err != nil {
			return Downloaded{}, err
		}
		return Downloaded{}, errors.New("moodle: download destination is empty")
	}
	f, err := c.openFile(ctx, rawURL)
	if err != nil {
		return Downloaded{}, err
	}
	defer f.Close()

	target, err := resolveTarget(dest, filenameFrom(f.resp, f.clean))
	if err != nil {
		return Downloaded{}, err
	}
	res := Downloaded{URL: f.clean.String()}
	lastMod, _ := http.ParseTime(f.resp.Header.Get("Last-Modified"))

	if fi, err := os.Stat(target); err == nil {
		if f.resp.ContentLength >= 0 && fi.Size() == f.resp.ContentLength &&
			(lastMod.IsZero() || fi.ModTime().Equal(lastMod)) {
			res.Path, res.Size, res.Reused = target, fi.Size(), true
			return res, nil
		}
		target = uniqueName(target)
	}

	n, err := writeAtomic(target, f.body, lastMod)
	if err != nil {
		return Downloaded{}, fmt.Errorf("moodle: download %s: %s", f.clean, c.Redact(err.Error()))
	}
	res.Path, res.Size = target, n
	return res, nil
}

// getFile performs the GET with one retry on network errors and 5xx.
// Errors never contain the token-bearing URL.
func (c *Client) getFile(ctx context.Context, cleanURL, tokenURL string) (*http.Response, error) {
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			if err := sleep(ctx, c.backoff(attempt)); err != nil {
				return nil, err
			}
		}
		resp, status, err := c.getOnce(ctx, tokenURL)
		if err == nil && status < 500 {
			return resp, nil
		}
		retry := c.retryable(ctx, status, err)
		if err != nil {
			lastErr = scrubURLError(err, cleanURL, c.redactor)
		} else {
			lastErr = &HTTPError{Function: "download", Status: status, Snippet: cleanURL}
			_ = resp.Body.Close()
		}
		c.log.Warn("download failed", "url", cleanURL, "attempt", attempt, "status", status,
			"retry", retry && attempt < maxAttempts, "err", lastErr)
		if !retry {
			break
		}
	}
	return nil, lastErr
}

func (c *Client) getOnce(ctx context.Context, tokenURL string) (*http.Response, int, error) {
	if err := c.acquire(ctx); err != nil {
		return nil, 0, err
	}
	defer c.release()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.dl.Do(req)
	if err != nil {
		return nil, 0, err
	}
	return resp, resp.StatusCode, nil
}

// checkFileResponse turns non-200 responses and JSON error bodies into errors.
func (c *Client) checkFileResponse(cleanURL string, resp *http.Response, br *bufio.Reader) error {
	ct, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	looksJSON := ct == "application/json"
	if resp.StatusCode == http.StatusOK && !looksJSON {
		return nil
	}
	head, _ := br.Peek(64 << 10)
	if mErr := c.parseException("download", head); mErr != nil {
		return mErr
	}
	if resp.StatusCode != http.StatusOK {
		return &HTTPError{Function: "download", Status: resp.StatusCode, Snippet: cleanURL}
	}
	return nil // a genuine JSON file
}

func (c *Client) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return errors.New("too many redirects")
	}
	if !strings.EqualFold(req.URL.Hostname(), c.base.Hostname()) || req.URL.Scheme != c.base.Scheme {
		return fmt.Errorf("%w: refusing redirect to another host", ErrForeignURL)
	}
	return nil
}

// scrubURLError replaces the URL inside *url.Error (which carries the token
// in its query) with the clean one, and redacts the rest of the message.
func scrubURLError(err error, cleanURL string, r *Redactor) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return &url.Error{Op: ue.Op, URL: cleanURL, Err: redactedErr{ue.Err, r}}
	}
	return redactedErr{err, r}
}

type redactedErr struct {
	err error
	r   *Redactor
}

func (e redactedErr) Error() string { return e.r.String(e.err.Error()) }
func (e redactedErr) Unwrap() error { return e.err }

func filenameFrom(resp *http.Response, u *url.URL) string {
	if _, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition")); err == nil {
		if name := params["filename"]; name != "" {
			return SanitizeFilename(name)
		}
	}
	return SanitizeFilename(path.Base(u.Path))
}

// SanitizeFilename reduces name to a safe base name for the local filesystem.
func SanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Base(name)
	var b strings.Builder
	for _, r := range name {
		switch {
		case unicode.IsControl(r):
		case strings.ContainsRune(`/:*?"<>|`, r):
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	s := strings.TrimLeft(strings.TrimSpace(b.String()), ".")
	if s == "" {
		s = "download"
	}
	for len(s) > maxFilenameLen {
		_, size := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-size]
	}
	return s
}

func resolveTarget(dest, name string) (string, error) {
	// Check the trailing separator before expandHome: filepath.Join drops it.
	isDir := strings.HasSuffix(dest, string(os.PathSeparator)) || strings.HasSuffix(dest, "/")
	dest = expandHome(dest)
	if fi, err := os.Stat(dest); err == nil && fi.IsDir() {
		isDir = true
	}
	var target string
	if isDir {
		target = filepath.Join(dest, name)
	} else {
		target = dest
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
		return "", fmt.Errorf("moodle: create directory: %w", err)
	}
	return abs, nil
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// uniqueName returns "name (1).ext", "name (2).ext", ... for the first free slot.
func uniqueName(p string) string {
	ext := filepath.Ext(p)
	stem := strings.TrimSuffix(p, ext)
	for i := 1; ; i++ {
		cand := stem + " (" + strconv.Itoa(i) + ")" + ext
		if _, err := os.Stat(cand); errors.Is(err, os.ErrNotExist) {
			return cand
		}
	}
}

func writeAtomic(target string, r io.Reader, modTime time.Time) (int64, error) {
	tmp, err := os.CreateTemp(filepath.Dir(target), ".moodle-dl-*")
	if err != nil {
		return 0, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	n, err := io.Copy(tmp, r)
	if err != nil {
		return 0, err
	}
	if err := tmp.Chmod(0o644); err != nil {
		return 0, err
	}
	if err := tmp.Sync(); err != nil {
		return 0, err
	}
	if err := tmp.Close(); err != nil {
		return 0, err
	}
	if !modTime.IsZero() {
		_ = os.Chtimes(tmp.Name(), modTime, modTime)
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		return 0, err
	}
	ok = true
	return n, nil
}
