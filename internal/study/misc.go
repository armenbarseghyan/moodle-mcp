package study

import (
	"context"
	"strings"

	"moodle-mcp/internal/moodle"
)

// DownloadResult answers moodle_download.
type DownloadResult struct {
	Path       string
	Size       int64
	Reused     bool
	BrowserURL string
}

// Download saves a Moodle file locally. dest defaults to the configured
// download directory.
func (s *Service) Download(ctx context.Context, fileURL, dest string) (DownloadResult, error) {
	if strings.TrimSpace(dest) == "" {
		dest = s.downloadDir + "/"
	}
	d, err := s.src.Download(ctx, fileURL, dest)
	if err != nil {
		return DownloadResult{}, err
	}
	return DownloadResult{Path: d.Path, Size: d.Size, Reused: d.Reused, BrowserURL: BrowserFileURL(d.URL)}, nil
}

// WhoAmIResult answers moodle_whoami.
type WhoAmIResult struct {
	FullName, Username string
	UserID             int
	SiteName, SiteURL  string
	Release, Lang      string
	Functions          int
	Required, Missing  []string
	DownloadFiles      bool
}

// WhoAmI checks the token against the site (always live) and reports which
// functions this server needs are missing from the token's service.
func (s *Service) WhoAmI(ctx context.Context) (WhoAmIResult, error) {
	f, err := s.src.SiteInfo(ctx, true)
	if err != nil {
		return WhoAmIResult{}, err
	}
	si := f.Value
	have := map[string]bool{}
	for _, fn := range si.Functions {
		have[fn.Name] = true
	}
	res := WhoAmIResult{
		FullName: si.FullName, Username: si.Username, UserID: si.UserID,
		SiteName: si.SiteName, SiteURL: si.SiteURL, Release: si.Release, Lang: si.Lang,
		Functions: len(si.Functions), Required: moodle.AllowedFunctions(),
		DownloadFiles: bool(si.DownloadFiles),
	}
	for _, fn := range res.Required {
		if !have[fn] {
			res.Missing = append(res.Missing, fn)
		}
	}
	return res, nil
}
