package textfmt

import (
	"sync/atomic"

	"golang.org/x/text/feature/plural"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// All user-facing text is written in English and printed through P(), a
// golang.org/x/text/message printer. The English source strings are the
// catalog keys: another language is added by registering translations for
// those keys (with CLDR plural forms where a count is involved) and calling
// SetLanguage — no output code changes.

var printer atomic.Pointer[message.Printer]

func init() {
	registerEnglishPlurals()
	SetLanguage(language.English)
}

// SetLanguage selects the language of all output.
func SetLanguage(tag language.Tag) {
	printer.Store(message.NewPrinter(tag))
}

// P returns the printer for user-facing text.
func P() *message.Printer { return printer.Load() }

// Counted phrases. Each key holds a single %d; English needs a singular form,
// other languages register their own plural cases for the same keys.
var countedEnglish = map[string]string{
	"%d days":           "%d day",
	"%d days ago":       "%d day ago",
	"in %d days":        "in %d day",
	"%d active courses": "%d active course",
	"%d matches":        "%d match",
	"%d assignments":    "%d assignment",
	"%d more lines":     "%d more line",
	"%d announcements":  "%d announcement",
}

func registerEnglishPlurals() {
	for key, one := range countedEnglish {
		if err := message.Set(language.English, key,
			plural.Selectf(1, "%d", plural.One, one, plural.Other, key)); err != nil {
			panic(err)
		}
	}
}
