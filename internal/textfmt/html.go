package textfmt

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// PreferredLangs decides which variant of Moodle multilang content is kept
// (<span lang="xx" class="multilang">). The site language comes first.
var PreferredLangs = []string{"de", "en"}

// StripHTML turns Moodle HTML into compact plain text: tags removed, entities
// decoded, block elements and <br> become line breaks, list items get "- ",
// links keep their target, whitespace is collapsed and empty lines dropped.
func StripHTML(s string) string {
	if !strings.ContainsAny(s, "<&") {
		return collapse(s)
	}
	body := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := html.ParseFragment(strings.NewReader(s), body)
	if err != nil {
		return collapse(s)
	}
	var w textWriter
	for _, n := range nodes {
		body.AppendChild(n)
	}
	w.children(body)
	return collapse(w.b.String())
}

type textWriter struct{ b strings.Builder }

func (w *textWriter) nl() { w.b.WriteByte('\n') }

func (w *textWriter) text(s string) {
	// Source newlines inside text are just whitespace in HTML.
	w.b.WriteString(strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		return r
	}, s))
}

func (w *textWriter) node(n *html.Node) {
	switch n.Type {
	case html.TextNode:
		w.text(n.Data)
		return
	case html.ElementNode:
	default:
		w.children(n)
		return
	}
	switch n.DataAtom {
	case atom.Script, atom.Style, atom.Head, atom.Title, atom.Noscript, atom.Template:
		return
	case atom.Br, atom.Hr:
		w.nl()
		return
	case atom.Li:
		w.nl()
		w.b.WriteString("- ")
		w.children(n)
		w.nl()
		return
	case atom.Td, atom.Th:
		w.b.WriteByte(' ')
		w.children(n)
		w.b.WriteByte(' ')
		return
	case atom.A:
		start := w.b.Len()
		w.children(n)
		label := strings.TrimSpace(w.b.String()[start:])
		href := attr(n, "href")
		if (strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://")) && label != href {
			w.b.WriteString(" (" + href + ")")
		}
		return
	}
	block := isBlock(n.DataAtom)
	if block {
		w.nl()
	}
	w.children(n)
	if block {
		w.nl()
	}
}

// children walks n's children, keeping only one variant of each run of
// consecutive multilang spans.
func (w *textWriter) children(n *html.Node) {
	for c := n.FirstChild; c != nil; {
		if !isMultilang(c) {
			w.node(c)
			c = c.NextSibling
			continue
		}
		var group []*html.Node
		for c != nil && (isMultilang(c) || isBlankText(c)) {
			if isMultilang(c) {
				group = append(group, c)
			}
			c = c.NextSibling
		}
		w.children(pickLang(group))
	}
}

func pickLang(group []*html.Node) *html.Node {
	for _, lang := range PreferredLangs {
		for _, n := range group {
			if strings.EqualFold(attr(n, "lang"), lang) {
				return n
			}
		}
	}
	return group[0]
}

func isMultilang(n *html.Node) bool {
	return n.Type == html.ElementNode && n.DataAtom == atom.Span &&
		slices.Contains(strings.Fields(attr(n, "class")), "multilang")
}

func isBlankText(n *html.Node) bool {
	return n.Type == html.TextNode && strings.TrimSpace(n.Data) == ""
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func isBlock(a atom.Atom) bool {
	switch a {
	case atom.P, atom.Div, atom.Section, atom.Article, atom.Header, atom.Footer, atom.Aside,
		atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6,
		atom.Ul, atom.Ol, atom.Dl, atom.Dt, atom.Dd, atom.Table, atom.Tr,
		atom.Blockquote, atom.Pre, atom.Figure, atom.Figcaption:
		return true
	}
	return false
}

// collapse normalises whitespace per line and drops empty lines.
func collapse(s string) string {
	s = strings.ReplaceAll(s, " ", " ")
	s = strings.ReplaceAll(s, "\u200b", "")
	lines := strings.Split(s, "\n")
	out := lines[:0]
	for _, l := range lines {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// OneLine joins the lines of s with " · ", dropping list markers.
func OneLine(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimPrefix(l, "- ")
	}
	return strings.Join(lines, " · ")
}

// Truncate shortens s to at most n runes, preferring a word boundary, and
// appends "…" when something was cut.
func Truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	full := []rune(s)
	r := full[:n-1]
	// Back off to a word boundary if one is reasonably close, unless the cut
	// already falls on one.
	for i := len(r) - 1; !unicode.IsSpace(full[n-1]) && i >= len(r)-20 && i > 0; i-- {
		if unicode.IsSpace(r[i]) {
			r = r[:i]
			break
		}
	}
	return strings.TrimRightFunc(string(r), func(c rune) bool {
		return unicode.IsSpace(c) || unicode.IsPunct(c) && c != ')' && c != '"'
	}) + "…"
}
