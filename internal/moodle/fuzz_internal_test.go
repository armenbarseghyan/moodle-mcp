package moodle

import (
	"net/url"
	"strings"
	"testing"
)

const fuzzToken = "0123456789abcdef0123456789abcdef"

func fuzzClient(t testing.TB) *Client {
	c, err := New(Config{BaseURL: "https://moodle.example.test/sub", Token: fuzzToken})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// FuzzDecode: any response body yields a value or an error — never a panic —
// and an error never repeats the token, even when the body contains it.
func FuzzDecode(f *testing.F) {
	for _, s := range []string{
		`{"exception":"x","errorcode":"invalidtoken","message":"bad ` + fuzzToken + `"}`,
		`{"error":"e","errorcode":"invalidtoken"}`, `[]`, `{}`, `null`, `<html>` + fuzzToken, ``, `{"userid":"x"}`,
		// %q escapes that end in "0" next to the rest of the token (found by the fuzzer)
		"\xb0" + fuzzToken[1:] + "0", "\x00" + fuzzToken[1:], "\x10" + fuzzToken[1:],
		// the token in fields other than message (found by the fuzzer)
		`{"exception":"x","errorcode":"` + fuzzToken + `"}`, `{"exception":"` + fuzzToken + `","errorcode":"e"}`,
		`{"error":"e","errorcode":"` + fuzzToken + `"}`,
	} {
		f.Add([]byte(s))
	}
	c := fuzzClient(f)
	f.Fuzz(func(t *testing.T, body []byte) {
		var out SiteInfo
		err := c.decode("core_webservice_get_site_info", body, &out)
		if err != nil && strings.Contains(err.Error(), fuzzToken) {
			t.Fatalf("error leaks the token: %v", err)
		}
	})
}

// FuzzToWebserviceURL: whatever link comes in, an accepted URL points to the
// configured site's webservice pluginfile endpoint and never carries a token.
func FuzzToWebserviceURL(f *testing.F) {
	for _, s := range []string{
		"https://moodle.example.test/sub/pluginfile.php/1/a.pdf",
		"https://moodle.example.test/sub/webservice/pluginfile.php/1/a.pdf?token=x&forcedownload=1",
		"http://MOODLE.example.test/sub/pluginfile.php/1/%5Ba%5D.pdf",
		"https://evil.test/sub/pluginfile.php/1/a.pdf", "https://moodle.example.test.evil.test/sub/pluginfile.php/1",
		"https://moodle.example.test/sub/pluginfile.php/../../login", "javascript:alert(1)", "//moodle.example.test/x",
	} {
		f.Add(s)
	}
	c := fuzzClient(f)
	f.Fuzz(func(t *testing.T, raw string) {
		u, err := c.ToWebserviceURL(raw)
		if err != nil {
			return
		}
		if u.Scheme != "https" || u.Host != "moodle.example.test" {
			t.Fatalf("%q accepted with foreign target %s", raw, u)
		}
		if !strings.HasPrefix(u.EscapedPath(), "/sub/webservice/pluginfile.php/") {
			t.Fatalf("%q accepted with path %s", raw, u.EscapedPath())
		}
		if strings.Contains(u.EscapedPath(), "/../") {
			t.Fatalf("%q accepted with traversal %s", raw, u.EscapedPath())
		}
		if q, _ := url.ParseQuery(u.RawQuery); q.Has("token") {
			t.Fatalf("%q kept a token parameter: %s", raw, u)
		}
	})
}
