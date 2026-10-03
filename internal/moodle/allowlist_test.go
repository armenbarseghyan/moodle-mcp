package moodle_test

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/armenbarseghyan/moodle-mcp/internal/moodle"
	"github.com/armenbarseghyan/moodle-mcp/internal/moodletest"
)

// The server must stay read-only: every allowlisted function is a getter, and
// none contains a write-like verb. ("enrol" is deliberately absent: it is the
// name of a component, as in core_enrol_get_users_courses.)
var writeLike = regexp.MustCompile(`_(submit|save|add|update|delete|create|set|send|mark|edit|remove|toggle|lock|start|process|copy|import|upload|duplicate|move|view|log|trigger|request|accept|decline|block|unblock|confirm)(_|$)`)

func TestAllowlistIsReadOnly(t *testing.T) {
	t.Parallel()
	fns := moodle.AllowedFunctions()
	if len(fns) == 0 {
		t.Fatal("empty allowlist")
	}
	for _, fn := range fns {
		if !strings.Contains(fn, "_get_") {
			t.Errorf("allowlisted function %s is not a getter", fn)
		}
		if writeLike.MatchString(fn) {
			t.Errorf("allowlist contains write-like function %s", fn)
		}
		if !moodle.Allowed(fn) {
			t.Errorf("Allowed(%s) = false", fn)
		}
	}
}

// Every allowlisted function must exist on the site (docs/functions.txt is the
// snapshot from core_webservice_get_site_info).
func TestAllowlistExistsOnSite(t *testing.T) {
	t.Parallel()
	f, err := os.Open(filepath.Join(moodletest.TestdataDir(), "..", "docs", "functions.txt"))
	if err != nil {
		t.Skipf("functions snapshot not available: %v", err)
	}
	defer func() { _ = f.Close() }()
	site := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		site[sc.Text()] = true
	}
	for _, fn := range moodle.AllowedFunctions() {
		if !site[fn] {
			t.Errorf("%s is not offered by the site", fn)
		}
	}
}
