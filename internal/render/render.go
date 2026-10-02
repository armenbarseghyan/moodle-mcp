package render

import (
	"fmt"
	"strings"
	"time"

	"moodle-mcp/internal/study"
	"moodle-mcp/internal/textfmt"
)

const (
	maxItemText         = 200
	maxAnnouncementText = 1500
	maxFeedbackText     = 500
)

// Courses renders moodle_courses.
func Courses(r study.CoursesResult, now time.Time) string {
	d := newDoc()
	d.line("## Курсы: %d %s", len(r.Active), textfmt.Plural(len(r.Active), "активный", "активных", "активных"))
	if r.Prefix != "" {
		d.line("_Общий префикс «%s» в названиях опущен._", strings.TrimSpace(r.Prefix))
	}
	d.blank()
	for _, c := range r.Active {
		d.line("- %s", courseLine(c))
	}
	if len(r.Active) == 0 {
		d.line("Активных курсов нет.")
	}
	if len(r.Past) > 0 {
		d.blank()
		d.line("### Прошедшие и скрытые")
		for _, c := range r.Past {
			d.line("- %s", courseLine(c))
		}
	}
	return d.finish(now, r.FetchedAt)
}

func courseLine(c study.Course) string {
	parts := []string{"**" + link(c.Label, c.URL) + "**", fmt.Sprintf("id %d", c.ID)}
	if c.ShortName != "" && c.ShortName != c.Label {
		parts = append(parts, c.ShortName)
	}
	switch {
	case !c.Start.IsZero() && !c.End.IsZero():
		parts = append(parts, textfmt.Day(c.Start)+" – "+textfmt.Day(c.End))
	case !c.Start.IsZero():
		parts = append(parts, "с "+textfmt.Day(c.Start))
	}
	if c.Progress != nil {
		parts = append(parts, fmt.Sprintf("прогресс %.0f%%", *c.Progress))
	}
	return strings.Join(parts, " · ")
}

// Deadlines renders moodle_deadlines.
func Deadlines(r study.DeadlinesResult, now time.Time) string {
	d := newDoc()
	last := r.Until.Add(-time.Second)
	d.line("## Дедлайны до %s (%d %s)", textfmt.Day(last), r.Days, textfmt.Plural(r.Days, "день", "дня", "дней"))
	if len(r.Overdue) > 0 {
		d.blank()
		d.line("**Просрочено и не сдано**")
		for _, x := range r.Overdue {
			d.line("- %s", deadlineLine(x, now))
		}
	}
	d.blank()
	if len(r.Upcoming) == 0 {
		d.line("Дедлайнов на этот период нет.")
	}
	for _, x := range r.Upcoming {
		d.line("- %s", deadlineLine(x, now))
	}
	if r.Hidden > 0 || len(r.Warnings) > 0 {
		d.blank()
	}
	if r.Hidden > 0 {
		d.line("_Ещё %d %s скрыто или пока недоступно._", r.Hidden,
			textfmt.Plural(r.Hidden, "задание", "задания", "заданий"))
	}
	for _, w := range r.Warnings {
		d.line("⚠ %s", w)
	}
	courseErrors(d, r.Errors)
	return d.finish(now, r.FetchedAt)
}

func deadlineLine(x study.Deadline, now time.Time) string {
	parts := []string{when(x.Due, now), x.Course.Label, link(x.Title, x.URL), kindLabel(x.Key.Module)}
	if s := statusLabel(x.Status); s != "" {
		parts = append(parts, s)
	}
	return strings.Join(parts, " · ")
}

func statusLabel(s study.Submission) string {
	switch s {
	case study.SubmissionNotSubmitted:
		return "❌ не сдано"
	case study.SubmissionDraft:
		return "📝 черновик, не отправлено"
	case study.SubmissionSubmitted:
		return "✅ сдано"
	case study.SubmissionGraded:
		return "✅ сдано, оценено"
	case study.SubmissionReopened:
		return "↩️ переоткрыто, нужно сдать снова"
	case study.SubmissionUnknown:
		return "статус неизвестен"
	default:
		return ""
	}
}

// Contents renders moodle_course_contents.
func Contents(r study.ContentsResult, now time.Time) string {
	d := newDoc()
	d.line("## %s (id %d)", link(r.Course.Label, r.Course.URL), r.Course.ID)
	if len(r.Sections) == 0 {
		d.blank()
		d.line("В курсе нет видимых материалов.")
	}
	for _, s := range r.Sections {
		d.blank()
		writeSection(d, s, 0)
	}
	return d.finish(now, r.FetchedAt)
}

func writeSection(d *doc, s study.Section, depth int) {
	indent := strings.Repeat("  ", depth)
	if depth == 0 {
		name := s.Name
		if name == "" {
			name = "Без названия"
		}
		d.line("### %s", name)
	}
	if s.Summary != "" {
		d.line("%s%s", indent, textfmt.Truncate(textfmt.OneLine(s.Summary), maxItemText))
	}
	for _, it := range s.Items {
		writeItem(d, it, indent)
		if it.Sub != nil {
			writeSection(d, *it.Sub, depth+1)
		}
	}
	if s.Hidden > 0 {
		d.line("%s_скрыто или недоступно: %d_", indent, s.Hidden)
	}
}

func writeItem(d *doc, it study.Item, indent string) {
	if it.Kind == "label" {
		if it.Text != "" {
			d.line("%s- %s", indent, textfmt.Truncate(textfmt.OneLine(it.Text), maxItemText))
		}
		return
	}
	if it.Kind == "subsection" {
		d.line("%s- **%s**", indent, it.Name)
		return
	}
	parts := []string{link(it.Name, it.URL), kindLabel(it.Kind)}
	if it.Link != "" {
		parts = append(parts, "→ "+it.Link)
	}
	if len(it.Files) == 1 {
		parts = append(parts, fileLink(it.Files[0]))
	}
	d.line("%s- %s", indent, strings.Join(parts, " · "))
	if len(it.Files) > 1 {
		for _, f := range it.Files {
			d.line("%s  - %s", indent, fileLink(f))
		}
	}
	if it.Text != "" && it.Text != it.Name {
		d.line("%s  %s", indent, textfmt.Truncate(textfmt.OneLine(it.Text), maxItemText))
	}
}

func fileLink(f study.File) string {
	s := link(f.Name, f.URL)
	if sz := Size(f.Size); sz != "" {
		s += " (" + sz + ")"
	}
	return s
}

// Search renders moodle_search.
func Search(r study.SearchResult, now time.Time) string {
	d := newDoc()
	scope := ""
	if r.Scope != nil {
		scope = " в курсе " + r.Scope.Label
	}
	total := r.Total + r.FileTotal
	d.line("## Поиск «%s»%s: %d %s", r.Query, scope, total, textfmt.Plural(total, "совпадение", "совпадения", "совпадений"))
	if r.Total > 0 {
		d.blank()
		if r.InFiles {
			d.line("### В названиях и описаниях")
		}
		for _, h := range r.Hits {
			it := h.Item
			parts := []string{link(it.Name, it.URL), kindLabel(it.Kind), place(h.Course, h.Path, r.Scope)}
			if it.Link != "" {
				parts = append(parts, "→ "+it.Link)
			}
			if len(it.Files) == 1 {
				parts = append(parts, fileLink(it.Files[0]))
			}
			d.line("- %s", strings.Join(parts, " · "))
			if len(it.Files) > 1 {
				for _, f := range it.Files {
					d.line("  - %s", fileLink(f))
				}
			}
		}
		if r.Total > len(r.Hits) {
			d.line("_Показаны первые %d из %d — уточни запрос._", len(r.Hits), r.Total)
		}
	}
	if r.InFiles {
		d.blank()
		d.line("### Внутри файлов")
		if r.FileTotal == 0 {
			d.line("Совпадений в тексте файлов нет.")
		}
		for _, h := range r.FileHits {
			parts := []string{link(h.File.Name, h.File.URL)}
			if len(h.Labels) > 0 {
				l := strings.Join(h.Labels, ", ")
				if h.More > 0 {
					l += fmt.Sprintf(" и ещё %d", h.More)
				}
				parts = append(parts, l)
			}
			parts = append(parts, place(h.Course, h.Path, r.Scope)+" › "+link(h.Item.Name, h.Item.URL))
			d.line("- %s", strings.Join(parts, " · "))
			if h.Snippet != "" {
				d.line("  > %s", h.Snippet)
			}
		}
		if r.FileTotal > len(r.FileHits) {
			d.line("_Показаны первые %d из %d файлов — уточни запрос._", len(r.FileHits), r.FileTotal)
		}
		d.blank()
		note := fmt.Sprintf("_Просмотрено файлов: %d", r.Indexed)
		if r.Skipped > 0 {
			note += fmt.Sprintf("; пропущено (видео, картинки, слишком большие): %d", r.Skipped)
		}
		if r.Failed > 0 {
			note += fmt.Sprintf("; не удалось прочитать: %d", r.Failed)
		}
		d.line("%s._", note)
	}
	if total == 0 && !r.InFiles {
		d.blank()
		d.line("Ничего не найдено в названиях, описаниях и именах файлов. " +
			"Можно искать и внутри PDF и документов: in_files=true.")
	}
	courseErrors(d, r.Errors)
	return d.finish(now, r.FetchedAt)
}

func place(c study.CourseRef, path []string, scope *study.CourseRef) string {
	p := strings.Join(path, " › ")
	if scope != nil {
		return p
	}
	return c.Label + " › " + p
}

// Grades renders moodle_grades.
func Grades(r study.GradesResult, now time.Time) string {
	d := newDoc()
	d.line("## Оценки")
	var empty []string
	shown := 0
	for _, cg := range r.Courses {
		if cg.Empty() {
			empty = append(empty, cg.Course.Label)
			continue
		}
		shown++
		d.blank()
		d.line("### %s", cg.Course.Label)
		for _, g := range cg.Items {
			d.line("- %s", gradeLine(g))
			if g.Feedback != "" {
				d.line("  > %s", textfmt.Truncate(textfmt.OneLine(g.Feedback), maxFeedbackText))
			}
		}
		if cg.Pending > 0 {
			d.line("- ещё не оценено: %d", cg.Pending)
		}
		if cg.Total != nil {
			d.line("- Итог курса: %s", gradeValue(*cg.Total))
		}
	}
	if len(empty) > 0 {
		d.blank()
		if shown == 0 && len(empty) == 1 {
			d.line("%s: оценок пока нет.", empty[0])
		} else {
			d.line("Без оценок: %s.", strings.Join(empty, ", "))
		}
	}
	courseErrors(d, r.Errors)
	return d.finish(now, time.Time{})
}

func gradeLine(g study.Grade) string {
	name := g.Name
	if name == "" {
		name = "Без названия"
	}
	parts := []string{link(name, g.URL) + ": " + gradeValue(g)}
	if !g.GradedAt.IsZero() {
		parts = append(parts, "оценено "+textfmt.Day(g.GradedAt))
	}
	return strings.Join(parts, " · ")
}

func gradeValue(g study.Grade) string {
	if g.Display == "" {
		return "не оценено"
	}
	s := "**" + g.Display + "**"
	if g.Max > 0 && g.Percent != nil {
		s += fmt.Sprintf(" / %s (%s%%)", trimFloat(g.Max), trimFloat(*g.Percent))
	}
	return s
}

func trimFloat(f float64) string {
	s := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", f), "0"), ".")
	return strings.Replace(s, ".", ",", 1) // German/Russian decimal comma, like Moodle's own values
}

// Announcements renders moodle_announcements.
func Announcements(r study.AnnouncementsResult, now time.Time) string {
	d := newDoc()
	d.line("## Объявления за %d %s: %d", r.Days, textfmt.Plural(r.Days, "день", "дня", "дней"), len(r.Items))
	if len(r.Items) == 0 {
		d.blank()
		d.line("Новых объявлений нет.")
	}
	for _, a := range r.Items {
		d.blank()
		title := a.Title
		if a.Pinned {
			title = "📌 " + title
		}
		d.line("### %s", title)
		meta := []string{a.Course.Label}
		if a.Author != "" {
			meta = append(meta, a.Author)
		}
		meta = append(meta, when(a.Posted, now))
		if !a.Edited.IsZero() {
			meta = append(meta, "изменено "+textfmt.Relative(a.Edited, now))
		}
		if a.Unread {
			meta = append(meta, "непрочитано")
		}
		meta = append(meta, link("открыть", a.URL))
		d.line("%s", strings.Join(meta, " · "))
		if a.Text != "" {
			d.line("")
			for _, l := range strings.Split(textfmt.Truncate(a.Text, maxAnnouncementText), "\n") {
				d.line("%s", l)
			}
		}
	}
	courseErrors(d, r.Errors)
	return d.finish(now, r.FetchedAt)
}

// Download renders moodle_download.
func Download(r study.DownloadResult) string {
	verb := "Скачано"
	if r.Reused {
		verb = "Уже было скачано (файл не изменился)"
	}
	s := fmt.Sprintf("%s: `%s`", verb, r.Path)
	if sz := Size(r.Size); sz != "" {
		s += " (" + sz + ")"
	}
	if r.BrowserURL != "" {
		s += "\nВ браузере: " + escapeURL(r.BrowserURL)
	}
	return s + "\n"
}

// WhoAmI renders moodle_whoami.
func WhoAmI(r study.WhoAmIResult, version string) string {
	d := newDoc()
	d.line("## %s", r.SiteName)
	d.line("- Пользователь: %s (%s), id %d", r.FullName, r.Username, r.UserID)
	d.line("- Сайт: %s · Moodle %s · язык %s", r.SiteURL, r.Release, r.Lang)
	if len(r.Missing) == 0 {
		d.line("- Функций доступно: %d; все %d нужных серверу есть", r.Functions, len(r.Required))
	} else {
		d.line("- Функций доступно: %d; **не хватает %d из %d**: %s", r.Functions, len(r.Missing),
			len(r.Required), strings.Join(r.Missing, ", "))
	}
	if r.DownloadFiles {
		d.line("- Скачивание файлов через API: разрешено")
	} else {
		d.line("- Скачивание файлов через API: **запрещено** на сайте")
	}
	if version != "" {
		d.line("- moodle-mcp %s", version)
	}
	return d.finish(time.Time{}, time.Time{})
}

// Ambiguous renders a course query that matched several courses.
func Ambiguous(e *study.AmbiguousError) string {
	d := newDoc()
	d.line("По запросу «%s» подходит несколько курсов — уточни название или укажи id:", e.Query)
	for _, c := range e.Candidates {
		past := ""
		if c.Past {
			past = " · прошедший"
		}
		d.line("- %s · id %d%s", c.Label, c.ID, past)
	}
	return d.finish(time.Time{}, time.Time{})
}

// NotFound renders a course query that matched nothing.
func NotFound(e *study.NotFoundError) string {
	d := newDoc()
	d.line("Курс «%s» не найден.", e.Query)
	if len(e.Available) > 0 {
		d.line("Активные курсы:")
		for _, c := range e.Available {
			d.line("- %s · id %d", c.Label, c.ID)
		}
	}
	return d.finish(time.Time{}, time.Time{})
}
