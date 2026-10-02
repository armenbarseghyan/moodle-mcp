package moodle

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
)

const redacted = "[REDACTED]"

// Redactor replaces a secret, in raw and URL-escaped form, with [REDACTED].
type Redactor struct {
	r *strings.Replacer
}

// NewRedactor returns a Redactor for secret. An empty secret redacts nothing.
func NewRedactor(secret string) *Redactor {
	if secret == "" {
		return &Redactor{}
	}
	var pairs []string
	seen := map[string]bool{}
	for _, v := range []string{secret, url.QueryEscape(secret), url.PathEscape(secret)} {
		if !seen[v] {
			seen[v] = true
			pairs = append(pairs, v, redacted)
		}
	}
	return &Redactor{r: strings.NewReplacer(pairs...)}
}

// String returns s with the secret removed.
func (r *Redactor) String(s string) string {
	if r == nil || r.r == nil {
		return s
	}
	return r.r.Replace(s)
}

// Redact removes token from s.
func Redact(s, token string) string { return NewRedactor(token).String(s) }

// RedactingHandler wraps a slog.Handler and removes the token from the message
// and from every attribute value, including errors, Stringers and groups.
// It is a safety net: the client never logs request parameters in the first place.
type RedactingHandler struct {
	next slog.Handler
	r    *Redactor
}

// NewRedactingHandler wraps next.
func NewRedactingHandler(next slog.Handler, token string) *RedactingHandler {
	return &RedactingHandler{next: next, r: NewRedactor(token)}
}

func (h *RedactingHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.next.Enabled(ctx, l)
}

func (h *RedactingHandler) Handle(ctx context.Context, rec slog.Record) error {
	out := slog.NewRecord(rec.Time, rec.Level, h.r.String(rec.Message), rec.PC)
	rec.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(h.attr(a))
		return true
	})
	return h.next.Handle(ctx, out)
}

func (h *RedactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clean := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		clean[i] = h.attr(a)
	}
	return &RedactingHandler{next: h.next.WithAttrs(clean), r: h.r}
}

func (h *RedactingHandler) WithGroup(name string) slog.Handler {
	return &RedactingHandler{next: h.next.WithGroup(name), r: h.r}
}

func (h *RedactingHandler) attr(a slog.Attr) slog.Attr {
	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindString:
		return slog.String(a.Key, h.r.String(v.String()))
	case slog.KindGroup:
		g := v.Group()
		clean := make([]any, len(g))
		for i, ga := range g {
			clean[i] = h.attr(ga)
		}
		return slog.Group(a.Key, clean...)
	case slog.KindAny:
		switch x := v.Any().(type) {
		case error:
			return slog.String(a.Key, h.r.String(x.Error()))
		case fmt.Stringer:
			return slog.String(a.Key, h.r.String(x.String()))
		default:
			return slog.String(a.Key, h.r.String(fmt.Sprint(x)))
		}
	default:
		return slog.Attr{Key: a.Key, Value: v}
	}
}
