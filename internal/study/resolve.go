package study

import (
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// ResolveCourse finds a course by numeric id or by a loose piece of its name.
//
// Order: numeric id → exact name → substring → all words as substrings.
// The first step with exactly one match wins; a step with several matches
// returns *AmbiguousError. Active courses are searched before past ones.
// Matching ignores case, diacritics (ü = u = ue) and punctuation.
func ResolveCourse(courses []Course, query string) (Course, error) {
	q := strings.TrimSpace(query)
	if id, err := strconv.Atoi(q); err == nil {
		for _, c := range courses {
			if c.ID == id {
				return c, nil
			}
		}
		return Course{}, &NotFoundError{Query: query, Available: active(courses)}
	}
	nq := Normalize(q)
	if nq == "" {
		return Course{}, &NotFoundError{Query: query, Available: active(courses)}
	}
	var act, past []Course
	for _, c := range courses {
		if c.Past {
			past = append(past, c)
		} else {
			act = append(act, c)
		}
	}
	for _, set := range [][]Course{act, past} {
		if c, err, ok := resolveIn(set, query, nq); ok {
			return c, err
		}
	}
	return Course{}, &NotFoundError{Query: query, Available: act}
}

func resolveIn(set []Course, query, nq string) (Course, error, bool) {
	words := strings.Fields(nq)
	steps := []func(names []string) bool{
		func(names []string) bool { // exact
			for _, n := range names {
				if n == nq {
					return true
				}
			}
			return false
		},
		func(names []string) bool { // substring
			for _, n := range names {
				if strings.Contains(n, nq) {
					return true
				}
			}
			return false
		},
		func(names []string) bool { // all words
			hay := strings.Join(names, " ")
			for _, w := range words {
				if !strings.Contains(hay, w) {
					return false
				}
			}
			return true
		},
	}
	for _, match := range steps {
		var hits []Course
		for _, c := range set {
			names := []string{Normalize(c.Name), Normalize(c.Label)}
			if c.ShortName != "" {
				names = append(names, Normalize(c.ShortName))
			}
			if match(names) {
				hits = append(hits, c)
			}
		}
		switch len(hits) {
		case 0:
			continue
		case 1:
			return hits[0], nil, true
		default:
			return Course{}, &AmbiguousError{Query: query, Candidates: hits}, true
		}
	}
	return Course{}, nil, false
}

func active(cs []Course) []Course {
	var out []Course
	for _, c := range cs {
		if !c.Past {
			out = append(out, c)
		}
	}
	return out
}

var umlautsAlt = strings.NewReplacer("ß", "ss", "ae", "a", "oe", "o", "ue", "u")

// stripMarks removes diacritics. A transform.Chain keeps internal state and is
// not safe for concurrent use, so a new one is built per call.
func stripMarks() transform.Transformer {
	return transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
}

// Normalize prepares text for loose matching: lower case, no diacritics,
// German transliterations folded ("Prüfung", "Pruefung" and "Prufung" are
// equal), punctuation replaced by spaces, whitespace collapsed.
func Normalize(s string) string {
	s = strings.ToLower(s)
	if t, _, err := transform.String(stripMarks(), s); err == nil {
		s = t
	}
	s = umlautsAlt.Replace(s)
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return ' '
	}, s)
	return strings.Join(strings.Fields(s), " ")
}
