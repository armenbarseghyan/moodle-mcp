package textfmt_test

import (
	"strings"
	"testing"
	"time"

	"moodle-mcp/internal/textfmt"
)

func vie(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, textfmt.Vienna)
	if err != nil {
		panic(err)
	}
	return t
}

func TestDate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   time.Time
		want string
	}{
		{vie("2026-10-07 17:15"), "2026-10-07 17:15 (Wednesday)"},
		{vie("2026-10-04 00:00"), "2026-10-04 00:00 (Sunday)"},
		// Moodle timestamps are UTC instants; output must be Vienna wall time.
		{time.Unix(1791386100, 0).UTC(), "2026-10-07 17:15 (Wednesday)"},
		// Winter time: 23:59 UTC on 31.12 is already 1 January in Vienna.
		{time.Date(2026, 12, 31, 23, 59, 0, 0, time.UTC), "2027-01-01 00:59 (Friday)"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			if got := textfmt.Date(tt.in); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRelative(t *testing.T) {
	t.Parallel()
	now := vie("2026-10-02 12:00")
	tests := []struct {
		name string
		t    time.Time
		now  time.Time
		want string
	}{
		{"now", now.Add(20 * time.Second), now, "now"},
		{"minutes", now.Add(40 * time.Minute), now, "in 40 min"},
		{"hours today", vie("2026-10-02 23:59"), now, "today, in 11 h"},
		{"tomorrow early", vie("2026-10-03 00:30"), now, "tomorrow"},
		{"tomorrow by minutes", vie("2026-10-03 00:10"), vie("2026-10-02 23:50"), "in 20 min"},
		{"2 days", vie("2026-10-04 09:00"), now, "in 2 days"},
		{"4 days", vie("2026-10-06 23:59"), now, "in 4 days"},
		{"5 days", vie("2026-10-07 17:15"), now, "in 5 days"},
		{"calendar days, not 24h", vie("2026-10-04 08:00"), vie("2026-10-02 23:00"), "in 2 days"},
		{"21 days", vie("2026-10-23 12:00"), now, "in 21 days"},
		{"just now", now.Add(-10 * time.Second), now, "just now"},
		{"minutes ago", now.Add(-15 * time.Minute), now, "15 min ago"},
		{"hours ago", vie("2026-10-02 09:00"), now, "today, 3 h ago"},
		{"yesterday", vie("2026-10-01 23:59"), now, "yesterday"},
		{"days ago", vie("2026-09-30 09:30"), now, "2 days ago"},
		{"11 days ago", vie("2026-09-21 12:00"), now, "11 days ago"},
		// DST ends 2026-10-25 03:00 CEST → 02:00 CET: that day has 25 hours.
		{"DST day is still today", vie("2026-10-25 23:30"), vie("2026-10-25 00:30"), "today, in 24 h"},
		{"across DST", vie("2026-10-26 12:00"), vie("2026-10-24 12:00"), "in 2 days"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := textfmt.Relative(tt.t, tt.now); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDayStart(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		t      time.Time
		offset int
		want   time.Time
	}{
		{"same day", vie("2026-10-02 12:00"), 0, vie("2026-10-02 00:00")},
		{"window end for days=14", vie("2026-10-02 12:00"), 15, vie("2026-10-17 00:00")},
		{"over month end", vie("2026-10-30 08:00"), 3, vie("2026-11-02 00:00")},
		{"over DST", vie("2026-10-24 12:00"), 2, vie("2026-10-26 00:00")},
		{"UTC input late evening", time.Date(2026, 10, 2, 22, 30, 0, 0, time.UTC), 0, vie("2026-10-03 00:00")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := textfmt.DayStart(tt.t, tt.offset); !got.Equal(tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
	// 25-hour day: next midnight is 25h after the previous one.
	if d := textfmt.DayStart(vie("2026-10-25 12:00"), 1).Sub(textfmt.DayStart(vie("2026-10-25 12:00"), 0)); d != 25*time.Hour {
		t.Errorf("DST day length = %v, want 25h", d)
	}
}

func TestCountedPhrases(t *testing.T) {
	t.Parallel()
	p := textfmt.P()
	tests := []struct {
		key  string
		n    int
		want string
	}{
		{"in %d days", 1, "in 1 day"},
		{"in %d days", 2, "in 2 days"},
		{"%d days ago", 1, "1 day ago"},
		{"%d matches", 0, "0 matches"},
		{"%d matches", 1, "1 match"},
		{"%d assignments", 3, "3 assignments"},
		{"%d active courses", 1, "1 active course"},
		{"%d more lines", 12, "12 more lines"},
	}
	for _, tt := range tests {
		if got := p.Sprintf(tt.key, tt.n); got != tt.want {
			t.Errorf("Sprintf(%q, %d) = %q, want %q", tt.key, tt.n, got, tt.want)
		}
	}
}

func TestStripHTML(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, in, want string
	}{
		{"plain", "Hello   world", "Hello world"},
		{"empty", "", ""},
		{"only tags", "<p></p><div> </div>", ""},
		{"inline and entities", "<p>Solve the <i>exercises</i>&nbsp;in R &amp; Python.</p>", "Solve the exercises in R & Python."},
		{"paragraphs", "<p>One</p><p>Two</p>", "One\nTwo"},
		{"br", "Line 1<br>Line 2<br/>Line 3", "Line 1\nLine 2\nLine 3"},
		{"source newlines are spaces", "<p>Die Vorlesung\n   findet statt</p>", "Die Vorlesung findet statt"},
		{"list", "<ul><li>Laptop</li><li>Ladekabel</li></ul>", "- Laptop\n- Ladekabel"},
		{"headings", "<h3>Termine</h3><p>Montag</p>", "Termine\nMontag"},
		{"script and style removed", "<style>p{color:red}</style><p>Text</p><script>alert(1)</script>", "Text"},
		{"link with url", `Siehe <a href="https://gitlab.example.test/">GitLab</a>.`, "Siehe GitLab (https://gitlab.example.test/)."},
		{"link text equals url", `<a href="https://x.test/">https://x.test/</a>`, "https://x.test/"},
		{"relative link kept as text", `<a href="/mod/forum/view.php?id=1">Forum</a>`, "Forum"},
		{"moodle no-overflow wrapper", `<div class="no-overflow"><p>Course materials will be provided</p></div>`, "Course materials will be provided"},
		{"table", "<table><tr><td>A</td><td>B</td></tr><tr><td>C</td><td>D</td></tr></table>", "A B\nC D"},
		{"multilang prefers de", `<span lang="en" class="multilang">Homework</span><span lang="de" class="multilang">Hausübung</span>`, "Hausübung"},
		{"multilang falls back to en", `<span class="multilang" lang="fr">Devoir</span> <span class="multilang" lang="en">Homework</span>`, "Homework"},
		{"multilang falls back to first", `<span class="multilang" lang="fr">Devoir</span><span class="multilang" lang="it">Compito</span>`, "Devoir"},
		{"multilang inside text", `Abgabe: <span lang="de" class="multilang">Freitag</span><span lang="en" class="multilang">Friday</span>!`, "Abgabe: Freitag!"},
		{"broken html", "<p>Unclosed <b>bold", "Unclosed bold"},
		{"zero width space", "a\u200bb", "ab"},
		{"numeric entities", "&#252;bung &#x2014; ok", "übung — ok"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := textfmt.StripHTML(tt.in); got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"short", "abc", 5, "abc"},
		{"exact", "abcde", 5, "abcde"},
		{"runes not bytes", "Übungsblatt", 11, "Übungsblatt"},
		{"cut on word", "Die Vorlesung findet morgen statt", 20, "Die Vorlesung…"},
		{"no space nearby", strings.Repeat("x", 30), 10, "xxxxxxxxx…"},
		{"trailing punctuation dropped", "Hallo, Welt, wie geht", 13, "Hallo, Welt…"},
		{"zero", "abc", 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := textfmt.Truncate(tt.in, tt.n)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
			if n := len([]rune(got)); n > tt.n {
				t.Errorf("result has %d runes, limit %d", n, tt.n)
			}
		})
	}
}

func TestOneLine(t *testing.T) {
	t.Parallel()
	if got := textfmt.OneLine("Topics:\n- Shell\n- SSH"); got != "Topics: · Shell · SSH" {
		t.Errorf("got %q", got)
	}
}
