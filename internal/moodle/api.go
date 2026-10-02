package moodle

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

const (
	eventsPageSize = 50 // Moodle's maximum for limitnum
	eventsMaxPages = 20
	// Forum discussion sort orders (mod_forum discussion_list vault).
	sortCreatedDesc = 3
)

// SiteInfo returns information about the token's user and the site.
func (c *Client) SiteInfo(ctx context.Context) (*SiteInfo, error) {
	var out SiteInfo
	if err := c.Call(ctx, "core_webservice_get_site_info", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UserCourses returns all courses the user is enrolled in.
func (c *Client) UserCourses(ctx context.Context, userID int) ([]Course, error) {
	var out []Course
	p := url.Values{"userid": {strconv.Itoa(userID)}}
	if err := c.Call(ctx, "core_enrol_get_users_courses", p, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CourseContents returns the sections and modules of a course.
func (c *Client) CourseContents(ctx context.Context, courseID int) ([]Section, error) {
	var out []Section
	p := url.Values{"courseid": {strconv.Itoa(courseID)}}
	if err := c.Call(ctx, "core_course_get_contents", p, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ActionEvents returns action events (deadlines) with timesort in [from, to],
// following Moodle's aftereventid pagination.
func (c *Client) ActionEvents(ctx context.Context, from, to time.Time) ([]Event, error) {
	var all []Event
	after := 0
	for range eventsMaxPages {
		p := url.Values{
			"timesortfrom": {strconv.FormatInt(from.Unix(), 10)},
			"timesortto":   {strconv.FormatInt(to.Unix(), 10)},
			"limitnum":     {strconv.Itoa(eventsPageSize)},
		}
		if after > 0 {
			p.Set("aftereventid", strconv.Itoa(after))
		}
		var page eventsResponse
		if err := c.Call(ctx, "core_calendar_get_action_events_by_timesort", p, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Events...)
		if len(page.Events) < eventsPageSize || page.LastID == 0 || page.LastID == after {
			return all, nil
		}
		after = page.LastID
	}
	return all, fmt.Errorf("moodle: action events: more than %d pages", eventsMaxPages)
}

// Assignments returns the assignments of the given courses in a single call.
// Courses or modules the user cannot access are reported as warnings.
func (c *Client) Assignments(ctx context.Context, courseIDs []int) ([]CourseAssignments, []Warning, error) {
	if len(courseIDs) == 0 {
		// Without courseids Moodle returns every enrolled course; be explicit.
		return nil, nil, nil
	}
	var out assignmentsResponse
	if err := c.Call(ctx, "mod_assign_get_assignments", intList("courseids", courseIDs), &out); err != nil {
		return nil, nil, err
	}
	return out.Courses, out.Warnings, nil
}

// SubmissionStatus returns the current user's submission status for an assignment.
func (c *Client) SubmissionStatus(ctx context.Context, assignID int) (*SubmissionStatus, error) {
	var out SubmissionStatus
	p := url.Values{"assignid": {strconv.Itoa(assignID)}}
	if err := c.Call(ctx, "mod_assign_get_submission_status", p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GradeItems returns the user's grade items in a course.
func (c *Client) GradeItems(ctx context.Context, courseID, userID int) ([]GradeItem, error) {
	var out gradeItemsResponse
	p := url.Values{"courseid": {strconv.Itoa(courseID)}, "userid": {strconv.Itoa(userID)}}
	if err := c.Call(ctx, "gradereport_user_get_grade_items", p, &out); err != nil {
		return nil, err
	}
	for _, ug := range out.UserGrades {
		if ug.UserID == userID {
			return ug.GradeItems, nil
		}
	}
	return nil, nil
}

// Forums returns the forums of the given courses in a single call.
func (c *Client) Forums(ctx context.Context, courseIDs []int) ([]Forum, error) {
	if len(courseIDs) == 0 {
		return nil, nil
	}
	var out []Forum
	if err := c.Call(ctx, "mod_forum_get_forums_by_courses", intList("courseids", courseIDs), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Discussions returns the newest discussions of a forum (by creation time).
func (c *Client) Discussions(ctx context.Context, forumID, perPage int) ([]Discussion, error) {
	var out discussionsResponse
	p := url.Values{
		"forumid":   {strconv.Itoa(forumID)},
		"sortorder": {strconv.Itoa(sortCreatedDesc)},
		"page":      {"0"},
		"perpage":   {strconv.Itoa(perPage)},
	}
	if err := c.Call(ctx, "mod_forum_get_forum_discussions", p, &out); err != nil {
		return nil, err
	}
	return out.Discussions, nil
}

// intList encodes ids as name[0]=…&name[1]=… (PHP array syntax).
func intList(name string, ids []int) url.Values {
	v := url.Values{}
	for i, id := range ids {
		v.Set(name+"["+strconv.Itoa(i)+"]", strconv.Itoa(id))
	}
	return v
}
