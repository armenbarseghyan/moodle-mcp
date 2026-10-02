package extract_test

import (
	"testing"
	"unicode/utf8"

	"moodle-mcp/internal/extract"
)

// FuzzText feeds arbitrary bytes to every extractor: course files come from
// teachers and the web, so malformed input must yield an error, never a panic,
// and any text returned must be valid UTF-8.
func FuzzText(f *testing.F) {
	names := []string{"a.pdf", "a.zip", "a.docx", "a.pptx", "a.ipynb", "a.html", "a.txt"}
	f.Add(uint8(0), readFileF(f, "sample-two-pages.pdf"))
	f.Add(uint8(0), readFileF(f, "sample-table-checkbox.pdf"))
	f.Add(uint8(1), readFileF(f, "sample-instructions.zip"))
	f.Add(uint8(4), []byte(`{"cells":[{"source":["x"]}]}`))
	f.Add(uint8(5), readFileF(f, "sample.html"))
	f.Fuzz(func(t *testing.T, kind uint8, data []byte) {
		name := names[int(kind)%len(names)]
		parts, err := extract.Text(name, data)
		if err != nil {
			return
		}
		for _, p := range parts {
			if !utf8.ValidString(p.Text) || !utf8.ValidString(p.Label) {
				t.Fatalf("%s: invalid UTF-8 in %q", name, p)
			}
		}
	})
}
