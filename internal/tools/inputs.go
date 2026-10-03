package tools

// Tool inputs. Defaults and bounds are set on the generated JSON schema in
// register.go, so the model sees them and the SDK applies them.

type CoursesIn struct {
	IncludePast bool `json:"include_past,omitempty" jsonschema:"also list hidden, completed and ended courses"`
	Refresh     bool `json:"refresh,omitempty" jsonschema:"bypass the cache and ask Moodle again"`
}

type DeadlinesIn struct {
	Days           int   `json:"days,omitempty" jsonschema:"how many calendar days ahead, including today; the window ends at the end of the last day, Vienna time"`
	IncludeOverdue *bool `json:"include_overdue,omitempty" jsonschema:"also list unsubmitted assignments overdue by up to 7 days"`
	Refresh        bool  `json:"refresh,omitempty" jsonschema:"bypass the cache and ask Moodle again"`
}

type ContentsIn struct {
	Course  string `json:"course" jsonschema:"course id or part of its name (case-insensitive); several matches return a list of candidates"`
	Refresh bool   `json:"refresh,omitempty" jsonschema:"bypass the cache and ask Moodle again"`
}

type SearchIn struct {
	Query   string `json:"query" jsonschema:"words to search for; all words must match (case, umlauts and punctuation are ignored)"`
	Course  string `json:"course,omitempty" jsonschema:"limit to one course: id or part of its name"`
	InFiles bool   `json:"in_files,omitempty" jsonschema:"also search inside files: PDF, docx, pptx, ipynb, zip and Moodle pages; the first call downloads the files and takes longer"`
	Refresh bool   `json:"refresh,omitempty" jsonschema:"bypass the cache and ask Moodle again"`
}

type GradesIn struct {
	Course string `json:"course,omitempty" jsonschema:"course id or part of its name; empty means all active courses"`
}

type AnnouncementsIn struct {
	Days    int  `json:"days,omitempty" jsonschema:"how many past days to include"`
	Refresh bool `json:"refresh,omitempty" jsonschema:"bypass the cache and ask Moodle again"`
}

type WhatsNewIn struct {
	Days    int  `json:"days,omitempty" jsonschema:"how many past days to include"`
	Refresh bool `json:"refresh,omitempty" jsonschema:"bypass the cache and ask Moodle again"`
}

type DownloadIn struct {
	FileURL string `json:"fileurl" jsonschema:"link to a file of this Moodle (…/pluginfile.php/… or …/webservice/pluginfile.php/…)"`
	Dest    string `json:"dest,omitempty" jsonschema:"folder or full file path; empty means MOODLE_DOWNLOAD_DIR or ~/Downloads/moodle"`
}

type WhoAmIIn struct{}
