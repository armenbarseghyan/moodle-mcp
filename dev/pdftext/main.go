// Command pdftext prints the text moodle-mcp extracts from local files
// (the same code that powers moodle_search in_files). Development only.
//
//	go run ./dev/pdftext file.pdf [more files]
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/armenbarseghyan/moodle-mcp/internal/extract"
)

func main() {
	for _, p := range os.Args[1:] {
		data, err := os.ReadFile(p) //nolint:gosec // dev tool: reads the files named on its command line
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}
		parts, err := extract.Text(filepath.Base(p), data)
		_, _ = fmt.Fprintf(os.Stdout, "===== %s (err=%v)\n", filepath.Base(p), err)
		for _, part := range parts {
			_, _ = fmt.Fprintf(os.Stdout, "--- %s\n%s\n", part.Label, part.Text)
		}
	}
}
