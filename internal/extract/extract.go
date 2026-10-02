// Package extract pulls searchable plain text out of course files: PDF,
// HTML, Office documents (docx, pptx), Jupyter notebooks, plain text and
// source files, and zip archives containing any of these.
//
// It is pure: bytes in, text out. Malformed input never panics the caller.
package extract

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/ledongthuc/pdf"
	"golang.org/x/text/unicode/norm"

	"moodle-mcp/internal/textfmt"
)

// Limits protecting against huge files and zip bombs.
const (
	MaxFileBytes    = 40 << 20 // files larger than this are not indexed
	maxZipEntries   = 500
	maxPDFPages     = 400
	maxNestingDepth = 1 // a zip inside a zip is read, deeper ones are not
)

// maxZipTotal is the total uncompressed bytes read from one archive.
// A variable only so tests can lower it.
var maxZipTotal int64 = 64 << 20

// ErrUnsupported is returned for file types without a text extractor.
var ErrUnsupported = errors.New("extract: unsupported file type")

// Part is a searchable piece of a document: a PDF page, a slide, a notebook
// cell group, or a file inside an archive.
type Part struct {
	Label string // "p. 3", "slide 2", "lecture/index.html"; "" for single-part files
	Text  string
}

type kind int

const (
	kindNone kind = iota
	kindPDF
	kindHTML
	kindText
	kindDOCX
	kindPPTX
	kindIPYNB
	kindZIP
)

var textExts = []string{
	".txt", ".md", ".markdown", ".csv", ".tsv", ".r", ".rmd", ".qmd", ".py", ".sql", ".tex", ".bib",
	".json", ".yaml", ".yml", ".sh", ".java", ".c", ".h", ".cpp", ".js", ".ts", ".go", ".m", ".jl", ".xml",
}

func kindOf(name string) kind {
	ext := strings.ToLower(path.Ext(name))
	switch {
	case ext == ".pdf":
		return kindPDF
	case ext == ".html" || ext == ".htm":
		return kindHTML
	case ext == ".docx":
		return kindDOCX
	case ext == ".pptx":
		return kindPPTX
	case ext == ".ipynb":
		return kindIPYNB
	case ext == ".zip":
		return kindZIP
	case slices.Contains(textExts, ext):
		return kindText
	}
	return kindNone
}

// Supported reports whether a file name has a text extractor.
func Supported(name string) bool { return kindOf(name) != kindNone }

// Text extracts the searchable parts of a file. The file type is decided by
// the name's extension.
func Text(name string, data []byte) (parts []Part, err error) {
	defer func() {
		if r := recover(); r != nil { // third-party parsers may panic on bad input
			parts, err = nil, fmt.Errorf("extract: %s: malformed file: %v", name, r)
		}
	}()
	return extract(name, data, 0)
}

func extract(name string, data []byte, depth int) ([]Part, error) {
	switch kindOf(name) {
	case kindPDF:
		return fromPDF(data)
	case kindHTML:
		return single(textfmt.StripHTML(string(data))), nil
	case kindText:
		return single(string(data)), nil
	case kindDOCX:
		return fromOOXML(data, "word/document.xml", "", "t")
	case kindPPTX:
		return fromOOXML(data, "ppt/slides/slide", "slide ", "t")
	case kindIPYNB:
		return fromNotebook(data)
	case kindZIP:
		if depth > maxNestingDepth {
			return nil, ErrUnsupported
		}
		return fromZip(data, depth)
	}
	return nil, ErrUnsupported
}

func single(s string) []Part {
	s = Clean(s)
	if s == "" {
		return nil
	}
	return []Part{{Text: s}}
}

// Clean normalises extracted text: invalid UTF-8 dropped, ligatures ("ﬁ")
// and other compatibility characters expanded (NFKC), whitespace collapsed
// per line, empty lines removed. Spacing errors around ligatures in PDFs
// ("di ff erent") are left alone: search matches whitespace-insensitively.
func Clean(s string) string {
	s = strings.ToValidUTF8(s, "")
	s = strings.Map(func(r rune) rune {
		// U+FFFD and private-use code points are unmapped PDF glyphs (bullets, icons).
		if r == '\uFFFD' || (r >= '\uE000' && r <= '\uF8FF') {
			return -1
		}
		return r
	}, s)
	s = norm.NFKC.String(s)
	lines := strings.Split(s, "\n")
	out := lines[:0]
	for _, l := range lines {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// --- PDF ---

func fromPDF(data []byte) ([]Part, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("extract: pdf: %w", err)
	}
	var parts []Part
	for i := 1; i <= r.NumPage() && i <= maxPDFPages; i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		if txt := Clean(pdfPageText(p)); txt != "" {
			parts = append(parts, Part{Label: "p. " + strconv.Itoa(i), Text: txt})
		}
	}
	return parts, nil
}

// pdfPageText rebuilds lines and word spaces from glyph positions, keeping
// the content stream order (which is the reading order in practice). Plain
// text extraction loses the spaces in many LaTeX-generated PDFs.
func pdfPageText(p pdf.Page) string {
	glyphs := p.Content().Text
	var b strings.Builder
	for i, g := range glyphs {
		if i > 0 {
			prev := glyphs[i-1]
			size := math.Max(math.Max(prev.FontSize, g.FontSize), 1)
			dy := math.Abs(g.Y - prev.Y)
			gap := g.X - (prev.X + prev.W)
			switch {
			case dy > 0.6*size:
				b.WriteByte('\n') // new line
			case gap > 0.15*size || gap < -2*size:
				b.WriteByte(' ') // word gap, or a jump back on the same line
			}
		}
		b.WriteString(g.S)
	}
	return b.String()
}

// --- Office Open XML (docx, pptx) ---

// fromOOXML reads the XML parts whose names start with prefix and collects
// the character data of <*:textTag> elements. For pptx each slide becomes a
// part labelled "slide N"; for docx the document is one part.
func fromOOXML(data []byte, prefix, label, textTag string) ([]Part, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("extract: office: %w", err)
	}
	type slide struct {
		n    int
		text string
	}
	var slides []slide
	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, prefix) || !strings.HasSuffix(f.Name, ".xml") {
			continue
		}
		n := 0
		if label != "" {
			n, err = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(f.Name, prefix), ".xml"))
			if err != nil {
				continue // slideLayout etc.
			}
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		txt, err := xmlText(io.LimitReader(rc, maxZipTotal), textTag)
		_ = rc.Close()
		if err != nil {
			continue
		}
		slides = append(slides, slide{n, txt})
	}
	sort.Slice(slides, func(i, j int) bool { return slides[i].n < slides[j].n })
	var parts []Part
	for _, s := range slides {
		if txt := Clean(s.text); txt != "" {
			p := Part{Text: txt}
			if label != "" {
				p.Label = label + strconv.Itoa(s.n)
			}
			parts = append(parts, p)
		}
	}
	return parts, nil
}

// xmlText collects text of <x:tag> elements; paragraphs (<x:p>) end lines.
func xmlText(r io.Reader, tag string) (string, error) {
	dec := xml.NewDecoder(r)
	var b strings.Builder
	in := false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return b.String(), nil
		}
		if err != nil {
			return b.String(), err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == tag {
				in = true
			}
			if t.Name.Local == "tab" || t.Name.Local == "br" {
				b.WriteByte(' ')
			}
		case xml.EndElement:
			if t.Name.Local == tag {
				in = false
			}
			if t.Name.Local == "p" {
				b.WriteByte('\n')
			}
		case xml.CharData:
			if in {
				b.Write(t)
			}
		}
	}
}

// --- Jupyter ---

func fromNotebook(data []byte) ([]Part, error) {
	var nb struct {
		Cells []struct {
			CellType string          `json:"cell_type"`
			Source   json.RawMessage `json:"source"`
		} `json:"cells"`
	}
	if err := json.Unmarshal(data, &nb); err != nil {
		return nil, fmt.Errorf("extract: ipynb: %w", err)
	}
	var b strings.Builder
	for _, c := range nb.Cells {
		var lines []string
		if json.Unmarshal(c.Source, &lines) != nil {
			var s string
			if json.Unmarshal(c.Source, &s) == nil {
				lines = []string{s}
			}
		}
		for _, l := range lines {
			b.WriteString(l)
		}
		b.WriteByte('\n')
	}
	return single(b.String()), nil
}

// --- zip ---

func fromZip(data []byte, depth int) ([]Part, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("extract: zip: %w", err)
	}
	var parts []Part
	var total int64
	for i, f := range zr.File {
		if i >= maxZipEntries {
			break
		}
		if f.FileInfo().IsDir() || strings.HasPrefix(path.Base(f.Name), ".") || strings.Contains(f.Name, "__MACOSX/") {
			continue
		}
		if kindOf(f.Name) == kindNone || (kindOf(f.Name) == kindZIP && depth >= maxNestingDepth) {
			continue
		}
		if f.UncompressedSize64 > MaxFileBytes || total+int64(f.UncompressedSize64) > maxZipTotal { //nolint:gosec // bounded by the check before
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		// Never trust the declared size: read at most what is left of the budget.
		b, err := io.ReadAll(io.LimitReader(rc, maxZipTotal-total+1))
		_ = rc.Close()
		if err != nil || int64(len(b)) > maxZipTotal-total {
			continue
		}
		total += int64(len(b))
		inner, err := extract(f.Name, b, depth+1)
		if err != nil {
			continue
		}
		for _, p := range inner {
			label := f.Name
			if p.Label != "" {
				label += ", " + p.Label
			}
			parts = append(parts, Part{Label: label, Text: p.Text})
		}
	}
	return parts, nil
}
