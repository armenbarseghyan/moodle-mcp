package study

import (
	"context"
	"slices"
	"strings"
	"time"

	"moodle-mcp/internal/moodle"
	"moodle-mcp/internal/textfmt"
)

const (
	discussionsPerForum = 10
	editGrace           = time.Minute
)

// AnnouncementsResult answers moodle_announcements.
type AnnouncementsResult struct {
	Days      int
	Since     time.Time
	Items     []Announcement // newest first
	Errors    []CourseError
	FetchedAt time.Time
}

// Announcements returns posts from the announcements ("news") forums of all
// active courses that were posted or edited within the last `days` days.
func (s *Service) Announcements(ctx context.Context, days int, refresh bool) (AnnouncementsResult, error) {
	now := s.now()
	res := AnnouncementsResult{Days: days, Since: now.Add(-time.Duration(days) * 24 * time.Hour)}
	var stamp oldest

	courses, at, err := s.activeCourses(ctx, refresh)
	if err != nil {
		return res, err
	}
	stamp.add(at)
	refs := refsByID(courses)

	ff, err := s.src.Forums(ctx, ids(courses), refresh)
	if err != nil {
		return res, err
	}
	stamp.add(ff.At)
	var news []moodle.Forum
	for _, f := range ff.Value {
		if _, ok := refs[f.Course]; ok && f.Type == "news" {
			news = append(news, f)
		}
	}

	lists, errs, err := fanOut(ctx, news, func(ctx context.Context, f moodle.Forum) (Fetched[[]moodle.Discussion], error) {
		return s.src.Discussions(ctx, f.ID, refresh)
	})
	if err != nil {
		return res, err
	}
	for i, f := range news {
		if errs[i] != nil {
			res.Errors = append(res.Errors, CourseError{Course: refs[f.Course], Err: errs[i]})
			continue
		}
		stamp.add(lists[i].At)
		for _, d := range lists[i].Value {
			a := ToAnnouncement(refs[f.Course], d, s.base())
			if a.Posted.Before(res.Since) && (a.Edited.IsZero() || a.Edited.Before(res.Since)) {
				continue
			}
			res.Items = append(res.Items, a)
		}
	}
	slices.SortStableFunc(res.Items, func(a, b Announcement) int { return latest(b).Compare(latest(a)) })
	res.FetchedAt = stamp.t
	return res, nil
}

// ToAnnouncement converts a forum discussion.
func ToAnnouncement(course CourseRef, d moodle.Discussion, base string) Announcement {
	title := strings.TrimSpace(d.Subject)
	if title == "" {
		title = strings.TrimSpace(d.Name)
	}
	discussion := d.Discussion
	if discussion == 0 {
		discussion = d.ID
	}
	a := Announcement{
		Course: course,
		Title:  title,
		Author: strings.TrimSpace(d.UserFullName),
		Text:   textfmt.StripHTML(d.Message),
		URL:    base + "/mod/forum/discuss.php?d=" + itoa(discussion),
		Posted: d.Created.Time(),
		Pinned: bool(d.Pinned),
		Unread: d.NumUnread > 0,
	}
	if m := d.Modified.Time(); !m.IsZero() && m.Sub(a.Posted) > editGrace {
		a.Edited = m
	}
	return a
}

func latest(a Announcement) time.Time {
	if a.Edited.After(a.Posted) {
		return a.Edited
	}
	return a.Posted
}
