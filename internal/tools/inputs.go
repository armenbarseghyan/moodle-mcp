package tools

// Tool inputs. Defaults and bounds are set on the generated JSON schema in
// register.go, so the model sees them and the SDK applies them.

type CoursesIn struct {
	IncludePast bool `json:"include_past,omitempty" jsonschema:"показать также скрытые, завершённые и закончившиеся курсы"`
	Refresh     bool `json:"refresh,omitempty" jsonschema:"обойти кэш и запросить Moodle заново"`
}

type DeadlinesIn struct {
	Days           int   `json:"days,omitempty" jsonschema:"сколько календарных дней вперёд, включая сегодня; окно заканчивается в конце последнего дня по Вене"`
	IncludeOverdue *bool `json:"include_overdue,omitempty" jsonschema:"показать несданные задания, просроченные не более чем на 7 дней"`
	Refresh        bool  `json:"refresh,omitempty" jsonschema:"обойти кэш и запросить Moodle заново"`
}

type ContentsIn struct {
	Course  string `json:"course" jsonschema:"id курса или часть названия (без учёта регистра); при нескольких совпадениях вернётся список кандидатов"`
	Refresh bool   `json:"refresh,omitempty" jsonschema:"обойти кэш и запросить Moodle заново"`
}

type SearchIn struct {
	Query   string `json:"query" jsonschema:"слова для поиска; все слова должны встретиться (регистр, умлауты и пунктуация не важны)"`
	Course  string `json:"course,omitempty" jsonschema:"ограничить одним курсом: id или часть названия"`
	InFiles bool   `json:"in_files,omitempty" jsonschema:"искать и внутри файлов: PDF, docx, pptx, ipynb, zip, страницы Moodle; первый вызов скачивает файлы и дольше"`
	Refresh bool   `json:"refresh,omitempty" jsonschema:"обойти кэш и запросить Moodle заново"`
}

type GradesIn struct {
	Course string `json:"course,omitempty" jsonschema:"id курса или часть названия; пусто — все активные курсы"`
}

type AnnouncementsIn struct {
	Days    int  `json:"days,omitempty" jsonschema:"за сколько последних дней"`
	Refresh bool `json:"refresh,omitempty" jsonschema:"обойти кэш и запросить Moodle заново"`
}

type DownloadIn struct {
	FileURL string `json:"fileurl" jsonschema:"ссылка на файл этого Moodle (…/pluginfile.php/… или …/webservice/pluginfile.php/…)"`
	Dest    string `json:"dest,omitempty" jsonschema:"папка или полный путь файла; пусто — MOODLE_DOWNLOAD_DIR или ~/Downloads/moodle"`
}

type WhoAmIIn struct{}
