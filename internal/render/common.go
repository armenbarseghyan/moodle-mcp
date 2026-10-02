// Package render turns study results into compact markdown for a human (and
// a token-conscious model) to read. Pure functions: no I/O, time is passed in.
package render

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"moodle-mcp/internal/moodle"
	"moodle-mcp/internal/study"
	"moodle-mcp/internal/textfmt"
)

// MaxChars bounds one tool answer. Lines beyond it are summarised.
const MaxChars = 12000

// staleAfter: answers built from older cached data say so.
const staleAfter = time.Minute

// doc accumulates lines up to MaxChars.
type doc struct {
	b       strings.Builder
	dropped int
	limit   int
}

func newDoc() *doc { return &doc{limit: MaxChars} }

func (d *doc) line(format string, args ...any) {
	s := format
	if len(args) > 0 {
		s = fmt.Sprintf(format, args...)
	}
	if d.dropped > 0 || d.b.Len()+len(s)+1 > d.limit {
		d.dropped++
		return
	}
	d.b.WriteString(s)
	d.b.WriteByte('\n')
}

func (d *doc) blank() {
	if d.dropped == 0 && d.b.Len() > 0 && !strings.HasSuffix(d.b.String(), "\n\n") {
		d.b.WriteByte('\n')
	}
}

// finish appends the truncation note and the cache note.
func (d *doc) finish(now, fetchedAt time.Time) string {
	if d.dropped > 0 {
		fmt.Fprintf(&d.b, "\n… ещё %d %s не показано — уточни запрос.\n",
			d.dropped, textfmt.Plural(d.dropped, "строка", "строки", "строк"))
	}
	if !fetchedAt.IsZero() && now.Sub(fetchedAt) >= staleAfter {
		fmt.Fprintf(&d.b, "\n_Данные из кэша, %s; для свежих — refresh=true._\n",
			textfmt.Relative(fetchedAt, now))
	}
	return strings.TrimRight(d.b.String(), "\n") + "\n"
}

// link renders a markdown link, escaping what would break it.
func link(text, href string) string {
	text = strings.NewReplacer("[", "\\[", "]", "\\]").Replace(text)
	if href == "" {
		return text
	}
	return "[" + text + "](" + escapeURL(href) + ")"
}

func escapeURL(u string) string {
	return strings.NewReplacer(" ", "%20", "(", "%28", ")", "%29").Replace(u)
}

// when renders "2026-10-07 17:15 (среда) — через 5 дней".
func when(t, now time.Time) string {
	return textfmt.Date(t) + " — " + textfmt.Relative(t, now)
}

// Size renders a byte count: "512 Б", "222 КБ", "2,8 МБ".
func Size(n int64) string {
	switch {
	case n <= 0:
		return ""
	case n < 1024:
		return fmt.Sprintf("%d Б", n)
	case n < 1024*1024:
		return fmt.Sprintf("%d КБ", (n+512)/1024)
	default:
		return strings.Replace(fmt.Sprintf("%.1f МБ", float64(n)/(1024*1024)), ".", ",", 1)
	}
}

func kindLabel(modname string) string {
	switch modname {
	case "assign":
		return "задание"
	case "quiz":
		return "тест"
	case "resource":
		return "файл"
	case "folder":
		return "папка"
	case "url":
		return "ссылка"
	case "page":
		return "страница"
	case "book":
		return "книга"
	case "forum":
		return "форум"
	case "lti":
		return "внешний инструмент"
	case "h5pactivity", "hvp":
		return "H5P"
	case "choice":
		return "опрос"
	case "feedback", "questionnaire":
		return "анкета"
	case "workshop":
		return "взаимооценка"
	case "lesson":
		return "урок"
	case "scorm":
		return "SCORM"
	case "glossary":
		return "глоссарий"
	case "wiki":
		return "вики"
	case "label":
		return "текст"
	case "subsection":
		return "подраздел"
	case "manual":
		return "вручную"
	default:
		return modname
	}
}

func courseErrors(d *doc, errs []study.CourseError) {
	if len(errs) == 0 {
		return
	}
	d.blank()
	for _, e := range errs {
		d.line("⚠ %s: %s", e.Course.Label, ErrorText(e.Err))
	}
}

// ErrorText explains an error in plain Russian. It never contains the token:
// every error from the client is already redacted.
func ErrorText(err error) string {
	var (
		hErr *moodle.HTTPError
		mErr *moodle.Error
		uErr *url.Error
	)
	switch {
	case err == nil:
		return ""
	case errors.Is(err, moodle.ErrInvalidToken):
		return "Токен Moodle недействителен или отозван. Создай новый в Moodle " +
			"(Profil → Einstellungen → Sicherheitsschlüssel, сервис moodle_mobile_app) и обнови MOODLE_TOKEN."
	case errors.Is(err, moodle.ErrAccessDenied):
		return "Moodle запретил вызов (access control): функция не входит в сервис токена или отключена для твоей роли."
	case errors.Is(err, moodle.ErrNotAccessible):
		return "Курс или активность недоступны (скрыты или ограничены)."
	case errors.Is(err, moodle.ErrMaintenance):
		return "Moodle сейчас на обслуживании, попробуй позже."
	case errors.Is(err, moodle.ErrNotAllowed):
		return "Эта функция Moodle не разрешена: сервер работает только на чтение."
	case errors.Is(err, moodle.ErrForeignURL):
		return "Ссылка не ведёт на файл этого Moodle (нужен адрес вида …/pluginfile.php/…). " +
			"С других адресов сервер не скачивает, чтобы не отдать токен."
	case errors.Is(err, moodle.ErrUnexpectedResponse):
		return "Moodle вернул не-JSON ответ — возможно, обслуживание или сбой прокси."
	case errors.Is(err, context.DeadlineExceeded):
		return "Moodle не ответил вовремя (таймаут)."
	case errors.As(err, &hErr):
		return fmt.Sprintf("Moodle ответил HTTP %d.", hErr.Status)
	case errors.As(err, &mErr):
		return fmt.Sprintf("Moodle вернул ошибку %s: %s", mErr.ErrorCode, mErr.Message)
	case errors.As(err, &uErr) && uErr.Timeout():
		return "Moodle не ответил вовремя (таймаут)."
	case errors.As(err, &uErr):
		return "Нет связи с Moodle: " + uErr.Err.Error()
	default:
		return err.Error()
	}
}
