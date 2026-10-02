package moodle

import (
	"maps"
	"slices"
)

// allowed is the complete set of web service functions this server may call.
// Every entry must be read-only. Adding a function that changes data requires
// an explicit decision, and TestAllowlistIsReadOnly guards against it.
var allowed = map[string]struct{}{
	"core_webservice_get_site_info":               {},
	"core_enrol_get_users_courses":                {},
	"core_course_get_contents":                    {},
	"core_calendar_get_action_events_by_timesort": {},
	"mod_assign_get_assignments":                  {},
	"mod_assign_get_submission_status":            {},
	"gradereport_user_get_grade_items":            {},
	"mod_forum_get_forums_by_courses":             {},
	"mod_forum_get_forum_discussions":             {},
}

// Allowed reports whether fn may be called.
func Allowed(fn string) bool {
	_, ok := allowed[fn]
	return ok
}

// AllowedFunctions returns the allowlist, sorted.
func AllowedFunctions() []string {
	return slices.Sorted(maps.Keys(allowed))
}
