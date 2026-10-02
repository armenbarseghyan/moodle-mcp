package study

import (
	"net/url"
	"strconv"
	"strings"
)

func itoa(i int) string { return strconv.Itoa(i) }

// BrowserFileURL turns a web service file URL into one the user can open in a
// logged-in browser: /webservice/pluginfile.php → /pluginfile.php, and the
// token and forcedownload parameters are dropped (PDFs open in a tab).
// Non-pluginfile URLs are returned unchanged.
func BrowserFileURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || !strings.Contains(u.Path, "/pluginfile.php/") {
		return raw
	}
	if u.RawPath != "" {
		u.RawPath = strings.Replace(u.RawPath, "/webservice/pluginfile.php/", "/pluginfile.php/", 1)
	}
	u.Path = strings.Replace(u.Path, "/webservice/pluginfile.php/", "/pluginfile.php/", 1)
	q := u.Query()
	q.Del("token")
	q.Del("forcedownload")
	u.RawQuery = q.Encode()
	return u.String()
}

func moduleURL(base, modname string, cmid int) string {
	if cmid <= 0 || modname == "" {
		return ""
	}
	return base + "/mod/" + modname + "/view.php?id=" + itoa(cmid)
}
