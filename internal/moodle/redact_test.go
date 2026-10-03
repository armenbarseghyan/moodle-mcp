package moodle_test

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"testing"

	"github.com/armenbarseghyan/moodle-mcp/internal/moodle"
)

func TestRedact(t *testing.T) {
	t.Parallel()
	const tok = "abc+def/ghi=" // needs escaping in queries and paths
	tests := []struct {
		name  string
		in    string
		token string
		want  string
	}{
		{"raw", "token=" + tok + "&x=1", tok, "token=[REDACTED]&x=1"},
		{"query escaped", "?token=" + url.QueryEscape(tok), tok, "?token=[REDACTED]"},
		{"path escaped", "/x/" + url.PathEscape(tok), tok, "/x/[REDACTED]"},
		{"several", tok + " " + tok, tok, "[REDACTED] [REDACTED]"},
		{"absent", "nothing here", tok, "nothing here"},
		{"empty token is a no-op", "keep me", "", "keep me"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := moodle.Redact(tt.in, tt.token); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

type stringer string

func (s stringer) String() string { return string(s) }

func TestRedactingHandler(t *testing.T) {
	t.Parallel()
	const tok = "s3cr3t0123456789"
	var buf bytes.Buffer
	h := moodle.NewRedactingHandler(slog.NewJSONHandler(&buf, nil), tok)
	log := slog.New(h).With("base", "https://x.test/?token="+tok).WithGroup("g")

	u, _ := url.Parse("https://x.test/webservice/pluginfile.php/1/a.pdf?token=" + tok)
	log.Info("msg "+tok,
		"str", "token="+tok,
		"err", fmt.Errorf("wrapped: %w", errors.New("Get "+u.String()+": EOF")),
		"url", u,
		"stringer", stringer(tok),
		"any", []string{tok},
		slog.Group("nested", "deep", tok),
		"int", 42,
	)
	out := buf.String()
	if strings.Contains(out, tok) {
		t.Fatalf("log leaks token:\n%s", out)
	}
	if !strings.Contains(out, `"int":42`) {
		t.Errorf("non-string attrs must survive: %s", out)
	}
	if strings.Count(out, "[REDACTED]") < 8 {
		t.Errorf("expected every occurrence to be redacted:\n%s", out)
	}
}
