// Package prscan converts Pathfinder assessments into reviewdog diagnostics
// and parses PR-scan issue inputs for cross-repo feedback.
package prscan

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// PRRef identifies a GitHub pull request that Pathfinder should assess.
type PRRef struct {
	Owner  string
	Repo   string
	Number int
	Host   string // normally github.com
}

var (
	prPathPattern = regexp.MustCompile(`(?i)^/([^/]+)/([^/]+)/pull/(\d+)/?$`)
	prAPIPattern  = regexp.MustCompile(`(?i)^/repos/([^/]+)/([^/]+)/pulls/(\d+)/?$`)
	// GitHub owner/repo slug: no slashes, spaces, or control chars that break API paths.
	safeGitHubName = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$`)
)

// ParsePRURL accepts common GitHub PR URL shapes, including api.github.com.
func ParsePRURL(raw string) (PRRef, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return PRRef{}, fmt.Errorf("PR URL is empty")
	}
	// Allow bare owner/repo#123 shorthand from operators.
	if strings.Count(raw, "/") == 1 && strings.Contains(raw, "#") {
		parts := strings.SplitN(raw, "#", 2)
		ownerRepo := strings.SplitN(parts[0], "/", 2)
		n, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil || n <= 0 || len(ownerRepo) != 2 {
			return PRRef{}, fmt.Errorf("invalid PR shorthand %q (want owner/repo#123)", raw)
		}
		owner, repo := ownerRepo[0], ownerRepo[1]
		if err := validateGitHubName(owner, "owner"); err != nil {
			return PRRef{}, err
		}
		if err := validateGitHubName(repo, "repo"); err != nil {
			return PRRef{}, err
		}
		return PRRef{Owner: owner, Repo: repo, Number: n, Host: "github.com"}, nil
	}

	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return PRRef{}, fmt.Errorf("parse PR URL: %w", err)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return PRRef{}, fmt.Errorf("PR URL missing host: %q", raw)
	}
	path := strings.TrimSuffix(u.EscapedPath(), "/")
	var m []string
	switch {
	case strings.HasPrefix(path, "/repos/"):
		m = prAPIPattern.FindStringSubmatch(path)
	default:
		m = prPathPattern.FindStringSubmatch(path)
	}
	if m == nil {
		return PRRef{}, fmt.Errorf("PR URL must look like https://github.com/owner/repo/pull/123 (got %q)", raw)
	}
	n, err := strconv.Atoi(m[3])
	if err != nil || n <= 0 {
		return PRRef{}, fmt.Errorf("invalid PR number in %q", raw)
	}
	owner, repo := m[1], m[2]
	if err := validateGitHubName(owner, "owner"); err != nil {
		return PRRef{}, err
	}
	if err := validateGitHubName(repo, "repo"); err != nil {
		return PRRef{}, err
	}
	if host != "github.com" && host != "www.github.com" && host != "api.github.com" {
		// Still accept enterprise hosts; operators must supply a capable token.
	}
	if host == "www.github.com" || host == "api.github.com" {
		host = "github.com"
	}
	return PRRef{Owner: owner, Repo: repo, Number: n, Host: host}, nil
}

func validateGitHubName(name, kind string) error {
	if !safeGitHubName.MatchString(name) {
		return fmt.Errorf("invalid GitHub %s %q", kind, name)
	}
	return nil
}

// HTMLURL returns the canonical browser URL for the PR.
func (p PRRef) HTMLURL() string {
	host := p.Host
	if host == "" {
		host = "github.com"
	}
	return fmt.Sprintf("https://%s/%s/%s/pull/%d", host, p.Owner, p.Repo, p.Number)
}

// FullName returns owner/repo.
func (p PRRef) FullName() string {
	return p.Owner + "/" + p.Repo
}
