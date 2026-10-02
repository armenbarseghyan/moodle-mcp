package extract_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"moodle-mcp/internal/extract"
	"moodle-mcp/internal/moodletest"
)

func readFile(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(moodletest.TestdataDir(), "files", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// zipOf builds an archive in memory.
func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func joined(parts []extract.Part) string {
	var b strings.Builder
	for _, p := range parts {
		b.WriteString("[" + p.Label + "] " + p.Text + "\n")
	}
	return b.String()
}

func TestText(t *testing.T) {
	t.Parallel()
	docx := zipOf(t, map[string]string{
		"word/document.xml": `<w:document xmlns:w="w"><w:body><w:p><w:r><w:t>Abgabe bis</w:t></w:r><w:r><w:t xml:space="preserve"> Freitag</w:t></w:r></w:p><w:p><w:r><w:t>Zweiter Absatz</w:t></w:r></w:p></w:body></w:document>`,
		"word/styles.xml":   `<w:styles xmlns:w="w"><w:t>NOT CONTENT</w:t></w:styles>`,
	})
	pptx := zipOf(t, map[string]string{
		"ppt/slides/slide2.xml":             `<p:sld xmlns:a="a" xmlns:p="p"><a:p><a:r><a:t>Second slide</a:t></a:r></a:p></p:sld>`,
		"ppt/slides/slide10.xml":            `<p:sld xmlns:a="a" xmlns:p="p"><a:p><a:r><a:t>Tenth slide</a:t></a:r></a:p></p:sld>`,
		"ppt/slides/slide1.xml":             `<p:sld xmlns:a="a" xmlns:p="p"><a:p><a:r><a:t>Title</a:t></a:r></a:p></p:sld>`,
		"ppt/slideLayouts/slideLayout1.xml": `<a:t xmlns:a="a">Layout</a:t>`,
		"ppt/slides/_rels/slide1.xml.rels":  `<x/>`,
	})
	notebook := `{"cells":[{"cell_type":"markdown","source":["# Lineare Regression\n","mit numpy"]},{"cell_type":"code","source":"import numpy as np"}]}`
	archive := zipOf(t, map[string]string{
		"ssh_instructions/index.html": `<h1>SSH</h1><p>Use <b>ssh-keygen</b></p>`,
		"ssh_instructions/notes.md":   "VPN off campus",
		"ssh_instructions/logo.png":   "\x89PNG",
		"__MACOSX/._index.html":       "junk",
		"inner.zip":                   string(zipOf(t, map[string]string{"deep.txt": "nested text"})),
	})

	tests := []struct {
		name    string
		file    string
		data    []byte
		want    []string // substrings of "[label] text" lines
		notWant []string
		wantErr error
	}{
		{"pdf pages and labels", "slides.pdf", readFile(t, "sample-two-pages.pdf"),
			[]string{"[стр. 1] WORKING ON A DAT VM", "Network on campus: directly", "[стр. 2] Off campus: FH VPN required", "Different setup"}, nil, nil},
		{"html", "page.HTML", readFile(t, "sample.html"),
			[]string{"[] Git & SSH\nConnect with ssh user@host from the VPN."}, []string{"<", "&amp;"}, nil},
		{"markdown", "README.md", []byte("# Title\n\n  some   text  \n"), []string{"[] # Title\nsome text"}, nil, nil},
		{"r source", "demo.R", []byte("x <- c(1, 2)"), []string{"x <- c(1, 2)"}, nil, nil},
		{"docx", "Aufgabe.docx", docx, []string{"[] Abgabe bis Freitag\nZweiter Absatz"}, []string{"NOT CONTENT"}, nil},
		{"pptx slides in numeric order", "lecture.pptx", pptx,
			[]string{"[слайд 1] Title\n[слайд 2] Second slide\n[слайд 10] Tenth slide"}, []string{"Layout"}, nil},
		{"notebook", "lab.ipynb", []byte(notebook), []string{"# Lineare Regression\nmit numpy\nimport numpy as np"}, nil, nil},
		{"zip with html, md and nested zip", "ssh_instructions.html.zip", archive,
			[]string{"[ssh_instructions/index.html] SSH\nUse ssh-keygen", "[ssh_instructions/notes.md] VPN off campus",
				"[inner.zip, deep.txt] nested text"}, []string{"PNG", "junk"}, nil},
		{"unsupported", "video.mp4", []byte("x"), nil, nil, extract.ErrUnsupported},
		{"no extension", "Makefile", []byte("x"), nil, nil, extract.ErrUnsupported},
		{"empty text file", "empty.txt", []byte("  \n "), nil, nil, nil},
		{"broken pdf", "broken.pdf", []byte("%PDF-1.4 garbage"), nil, nil, errAny},
		{"broken zip", "broken.zip", []byte("PK garbage"), nil, nil, errAny},
		{"broken notebook", "x.ipynb", []byte("{"), nil, nil, errAny},
		{"invalid utf8", "bad.txt", []byte("ok \xff\xfe text"), []string{"ok text"}, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			parts, err := extract.Text(tt.file, tt.data)
			switch {
			case errors.Is(tt.wantErr, errAny):
				if err == nil {
					t.Fatalf("want error, got %v", parts)
				}
				return
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			case err != nil:
				t.Fatal(err)
			}
			got := joined(parts)
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in:\n%s", w, got)
				}
			}
			for _, w := range tt.notWant {
				if strings.Contains(got, w) {
					t.Errorf("unexpected %q in:\n%s", w, got)
				}
			}
		})
	}
}

var errAny = errors.New("any error")

func TestSupported(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]bool{
		"a.pdf": true, "A.PDF": true, "x.docx": true, "x.pptx": true, "x.zip": true, "x.Rmd": true,
		"x.ipynb": true, "index.html": true, "x.mp4": false, "x.png": false, "x.doc": false, "x": false,
	} {
		if got := extract.Supported(name); got != want {
			t.Errorf("Supported(%q) = %v", name, got)
		}
	}
}

func TestZipBudget(t *testing.T) {
	// Not parallel: lowers the package-level budget.
	defer extract.SetZipBudget(3 << 20)()
	mib := strings.Repeat("a", 1<<20)
	files := map[string]string{}
	for i := range 6 { // 6 MiB of text, above the 3 MiB budget
		files[fmt.Sprintf("d/%d.txt", i)] = mib
	}
	parts, err := extract.Text("big.zip", zipOf(t, files))
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, p := range parts {
		total += len(p.Text)
	}
	if total > 3<<20 {
		t.Errorf("read %d bytes, budget is 3 MiB", total)
	}
	if len(parts) == 0 {
		t.Error("files within the budget must still be read")
	}
}

func TestClean(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{"di ﬀ erent", "di ff erent"},
		{"ﬁle", "file"},
		{"a b", "a b"},
		{"  x  \n\n  y ", "x\ny"},
		{"•\uFFFD To clone", "• To clone"},
		{"icon\uE001 text", "icon text"},
	}
	for _, tt := range tests {
		if got := extract.Clean(tt.in); got != tt.want {
			t.Errorf("Clean(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// Word-exported tables draw ☐/☒ in a CID font inside q…Q blocks; the rows
// after them used to vanish (fixed in third_party/ledongthuc-pdf, see PATCHES.md).
func TestPDFTableWithCheckboxes(t *testing.T) {
	t.Parallel()
	parts, err := extract.Text("syllabus.pdf", readFile(t, "sample-table-checkbox.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	got := joined(parts)
	for _, want := range []string{"Test 30", "☐", "Yes", "☒", "No", "Labs & Homework 30", "Capstone Project 30"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}
