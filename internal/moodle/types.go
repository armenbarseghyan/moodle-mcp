package moodle

// Wire types. Only fields the server uses are declared; everything else in
// Moodle's responses is ignored. Field shapes were checked against real
// responses from Moodle 4.5 (testdata/moodle/*.real*.json) and against the
// MOODLE_405_STABLE external function definitions.

// Warning is an entry of Moodle's "warnings" array.
type Warning struct {
	Item        string `json:"item"`
	ItemID      int    `json:"itemid"`
	WarningCode string `json:"warningcode"`
	Message     string `json:"message"`
}

// SiteInfo is core_webservice_get_site_info.
type SiteInfo struct {
	SiteName      string `json:"sitename"`
	SiteURL       string `json:"siteurl"`
	Username      string `json:"username"`
	FirstName     string `json:"firstname"`
	LastName      string `json:"lastname"`
	FullName      string `json:"fullname"`
	Lang          string `json:"lang"`
	UserID        int    `json:"userid"`
	Release       string `json:"release"`
	Version       string `json:"version"`
	DownloadFiles Bool   `json:"downloadfiles"`
	Functions     []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"functions"`
}

// Course is an element of core_enrol_get_users_courses.
type Course struct {
	ID          int      `json:"id"`
	ShortName   string   `json:"shortname"`
	FullName    string   `json:"fullname"`
	DisplayName string   `json:"displayname"`
	Summary     string   `json:"summary"`
	Visible     Bool     `json:"visible"`
	Hidden      Bool     `json:"hidden"`
	Completed   Bool     `json:"completed"`
	StartDate   Unix     `json:"startdate"`
	EndDate     Unix     `json:"enddate"`
	LastAccess  Unix     `json:"lastaccess"`
	Progress    *float64 `json:"progress"`
}

// Section is an element of core_course_get_contents.
type Section struct {
	ID          int      `json:"id"`
	Number      int      `json:"section"`
	Name        string   `json:"name"`
	Summary     string   `json:"summary"`
	Visible     Bool     `json:"visible"`
	UserVisible Bool     `json:"uservisible"`
	Component   string   `json:"component"` // "mod_subsection" for delegated sections (4.5+)
	ItemID      int      `json:"itemid"`
	Modules     []Module `json:"modules"`
}

// Module is a course module inside a Section.
type Module struct {
	ID          int       `json:"id"` // cmid
	Instance    int       `json:"instance"`
	ModName     string    `json:"modname"`
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	Description string    `json:"description"`
	Visible     Bool      `json:"visible"`
	UserVisible Bool      `json:"uservisible"`
	CustomData  string    `json:"customdata"` // JSON string; subsection: {"sectionid":"…"}
	Contents    []Content `json:"contents"`
}

// Content is a file or URL attached to a Module.
type Content struct {
	Type         string `json:"type"` // file | url | content
	FileName     string `json:"filename"`
	FilePath     string `json:"filepath"`
	FileURL      string `json:"fileurl"`
	FileSize     int64  `json:"filesize"`
	MimeType     string `json:"mimetype"`
	TimeModified Unix   `json:"timemodified"`
}

// Event is an element of core_calendar_get_action_events_by_timesort.
type Event struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`         // localised, e.g. "Homework R1 ist fällig"
	ActivityName string `json:"activityname"` // plain activity name
	Description  string `json:"description"`
	ModuleName   string `json:"modulename"`
	Instance     int    `json:"instance"`
	EventType    string `json:"eventtype"`
	TimeSort     Unix   `json:"timesort"`
	Overdue      Bool   `json:"overdue"`
	URL          string `json:"url"`
	Course       *struct {
		ID       int    `json:"id"`
		FullName string `json:"fullname"`
	} `json:"course"`
	Action *struct {
		Name       string `json:"name"`
		URL        string `json:"url"`
		Actionable Bool   `json:"actionable"`
	} `json:"action"`
}

type eventsResponse struct {
	Events  []Event `json:"events"`
	FirstID int     `json:"firstid"`
	LastID  int     `json:"lastid"`
}

// CourseAssignments groups mod_assign_get_assignments results by course.
type CourseAssignments struct {
	ID          int          `json:"id"`
	FullName    string       `json:"fullname"`
	ShortName   string       `json:"shortname"`
	Assignments []Assignment `json:"assignments"`
}

// Assignment is an assign activity.
type Assignment struct {
	ID                       int    `json:"id"`
	CMID                     int    `json:"cmid"`
	Course                   int    `json:"course"`
	Name                     string `json:"name"`
	Intro                    string `json:"intro"`
	DueDate                  Unix   `json:"duedate"`
	CutoffDate               Unix   `json:"cutoffdate"`
	AllowSubmissionsFromDate Unix   `json:"allowsubmissionsfromdate"`
	TeamSubmission           Bool   `json:"teamsubmission"`
}

type assignmentsResponse struct {
	Courses  []CourseAssignments `json:"courses"`
	Warnings []Warning           `json:"warnings"`
}

// Submission statuses as reported by mod_assign.
const (
	SubmissionNew       = "new"
	SubmissionDraft     = "draft"
	SubmissionSubmitted = "submitted"
	SubmissionReopened  = "reopened"
)

// SubmissionStatus is mod_assign_get_submission_status.
type SubmissionStatus struct {
	LastAttempt *struct {
		Submission       *Submission `json:"submission"`
		TeamSubmission   *Submission `json:"teamsubmission"`
		Graded           Bool        `json:"graded"`
		ExtensionDueDate Unix        `json:"extensionduedate"`
		GradingStatus    string      `json:"gradingstatus"`
	} `json:"lastattempt"`
	Feedback *struct {
		GradeForDisplay string `json:"gradefordisplay"`
		GradedDate      Unix   `json:"gradeddate"`
	} `json:"feedback"`
	Warnings []Warning `json:"warnings"`
}

// Submission is one submission attempt.
type Submission struct {
	ID            int    `json:"id"`
	Status        string `json:"status"`
	TimeModified  Unix   `json:"timemodified"`
	GradingStatus string `json:"gradingstatus"`
}

// GradeItem is an element of gradereport_user_get_grade_items.
type GradeItem struct {
	ID                  int      `json:"id"`
	ItemName            string   `json:"itemname"`
	ItemType            string   `json:"itemtype"` // mod | course | category | manual
	ItemModule          string   `json:"itemmodule"`
	ItemInstance        int      `json:"iteminstance"`
	CMID                int      `json:"cmid"`
	GradeRaw            *float64 `json:"graderaw"`
	GradeFormatted      string   `json:"gradeformatted"`
	GradeMin            float64  `json:"grademin"`
	GradeMax            float64  `json:"grademax"`
	PercentageFormatted string   `json:"percentageformatted"`
	Feedback            string   `json:"feedback"`
	GradeIsHidden       Bool     `json:"gradeishidden"`
	GradeDateGraded     Unix     `json:"gradedategraded"`
}

type gradeItemsResponse struct {
	UserGrades []struct {
		CourseID   int         `json:"courseid"`
		UserID     int         `json:"userid"`
		GradeItems []GradeItem `json:"gradeitems"`
	} `json:"usergrades"`
	Warnings []Warning `json:"warnings"`
}

// Forum is an element of mod_forum_get_forums_by_courses.
type Forum struct {
	ID             int    `json:"id"`
	Course         int    `json:"course"`
	Type           string `json:"type"` // "news" for announcements
	Name           string `json:"name"`
	CMID           int    `json:"cmid"`
	NumDiscussions int    `json:"numdiscussions"`
}

// Discussion is an element of mod_forum_get_forum_discussions.
type Discussion struct {
	ID           int    `json:"id"`
	Discussion   int    `json:"discussion"`
	Name         string `json:"name"`
	Subject      string `json:"subject"`
	Message      string `json:"message"`
	UserFullName string `json:"userfullname"`
	Created      Unix   `json:"created"`
	Modified     Unix   `json:"modified"`
	TimeModified Unix   `json:"timemodified"`
	Pinned       Bool   `json:"pinned"`
	NumUnread    int    `json:"numunread"`
}

type discussionsResponse struct {
	Discussions []Discussion `json:"discussions"`
	Warnings    []Warning    `json:"warnings"`
}
