package pdf

import (
	"strings"
	"testing"
)

// TestContentRestoresEncoderOnQ: text after "q … /F2 Tf … Q" must be decoded
// with the restored font's encoding, and an unbalanced Q must not panic.
// The fixture draws checkboxes in a Type0 Identity-H font inside q/Q blocks,
// like Word-exported tables do.
func TestContentRestoresEncoderOnQ(t *testing.T) {
	for _, file := range []string{"testdata/qq-checkbox-balanced.pdf", "testdata/qq-checkbox.pdf"} {
		t.Run(file, func(t *testing.T) { checkCheckboxTable(t, file) })
	}
}

// checkCheckboxTable: qq-checkbox-balanced.pdf isolates the encoder bug;
// qq-checkbox.pdf additionally ends with an unbalanced Q.
func checkCheckboxTable(t *testing.T, file string) {
	f, r, err := Open(file)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var b strings.Builder
	for _, txt := range r.Page(1).Content().Text {
		b.WriteString(txt.S)
	}
	got := b.String()
	wants := []string{"Test 30", "☐", "Yes", "☒", "No", "Labs & Homework 30"}
	if strings.HasSuffix(file, "qq-checkbox.pdf") {
		wants = append(wants, "Capstone Project 30")
	}
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}
