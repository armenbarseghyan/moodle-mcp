package textfmt_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/armenbarseghyan/moodle-mcp/internal/textfmt"
)

func FuzzStripHTML(f *testing.F) {
	for _, s := range []string{
		"", "plain", "<p>Hello <b>world</b>&nbsp;!</p>", "<ul><li>a</li><li>b</li></ul>",
		`<span lang="de" class="multilang">Hallo</span><span lang="en" class="multilang">Hi</span>`,
		"<script>alert(1)</script>x", "<a href=\"https://x.test/\">link</a>", "<table><tr><td>1</td></tr>",
		"&#xFFFF;&amp;&lt;", "<<<>>>", "\xff\xfe<p>",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		out := textfmt.StripHTML(in)
		if utf8.ValidString(in) && !utf8.ValidString(out) {
			t.Fatalf("invalid UTF-8 from valid input %q: %q", in, out)
		}
		for _, l := range strings.Split(out, "\n") {
			if l != strings.TrimSpace(l) || (out != "" && l == "") {
				t.Fatalf("output not normalised: %q", out)
			}
		}
		if strings.Contains(strings.ToLower(out), "alert(1)") && strings.Contains(strings.ToLower(in), "<script>alert(1)") &&
			!strings.Contains(strings.ToLower(in), "&lt;script") && strings.Count(strings.ToLower(in), "alert(1)") == 1 {
			t.Fatalf("script content leaked: %q", out)
		}
	})
}

func FuzzTruncate(f *testing.F) {
	f.Add("Die Vorlesung findet morgen statt", 10)
	f.Add("ü€😀", 2)
	f.Fuzz(func(t *testing.T, s string, n int) {
		if n < 0 || n > 10000 || !utf8.ValidString(s) {
			return
		}
		out := textfmt.Truncate(s, n)
		if utf8.RuneCountInString(out) > n {
			t.Fatalf("Truncate(%q, %d) = %q is longer than %d runes", s, n, out, n)
		}
		if !utf8.ValidString(out) {
			t.Fatalf("cut inside a character: %q", out)
		}
	})
}
