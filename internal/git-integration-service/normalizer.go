package gitintegrationservice

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

var (
	// scpSyntaxRegex matches SCP-like SSH syntax: [user@]host.xz:path/to/repo[.git/]
	scpSyntaxRegex = regexp.MustCompile(`^(?:([a-zA-Z0-9_\-]+)@)?([a-zA-Z0-9.\-]+):([a-zA-Z0-9_.\-\/]+)$`)
)

// NormalizeGitURL converts any standard Git URL (HTTPS, HTTP, SSH, git://)
// into a standardized canonical string: "<host>/<path>" (without .git suffix).
// Examples:
//   "https://github.com/org/repo.git"      -> "github.com/org/repo"
//   "git@github.com:org/repo.git"          -> "github.com/org/repo"
//   "ssh://git@gitlab.com/grp/sub/prj.git" -> "gitlab.com/grp/sub/prj"
func NormalizeGitURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("empty repository url")
	}

	// 1. If it has a scheme (e.g. https://, http://, ssh://, git://), use standard URL parsing
	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "", err
		}

		host := strings.ToLower(parsed.Host)
		// Remove port if present (e.g. host:22)
		if colonIdx := strings.Index(host, ":"); colonIdx != -1 {
			host = host[:colonIdx]
		}

		path := strings.Trim(parsed.Path, "/")
		path = strings.TrimSuffix(path, ".git")

		if host == "" || path == "" {
			return "", errors.New("invalid git repository url")
		}

		return host + "/" + path, nil
	}

	// 2. Check for SCP-style SSH syntax: git@host:path/repo.git or host:path/repo.git
	if matches := scpSyntaxRegex.FindStringSubmatch(raw); len(matches) == 4 {
		host := strings.ToLower(matches[2])
		path := strings.Trim(matches[3], "/")
		path = strings.TrimSuffix(path, ".git")
		if host == "" || path == "" {
			return "", errors.New("invalid scp git repository url")
		}
		return host + "/" + path, nil
	}

	// 3. Check for schemeless or already canonical URL: e.g. github.com/org/repo[.git]
	if slashIdx := strings.Index(raw, "/"); slashIdx != -1 {
		host := strings.ToLower(raw[:slashIdx])
		if strings.Contains(host, ".") {
			path := strings.Trim(raw[slashIdx+1:], "/")
			path = strings.TrimSuffix(path, ".git")
			if path != "" {
				return host + "/" + path, nil
			}
		}
	}

	return "", errors.New("unrecognized git repository url format")
}

// GitURLsMatch tests whether two repository URLs point to the same canonical repository
func GitURLsMatch(url1, url2 string) bool {
	norm1, err1 := NormalizeGitURL(url1)
	norm2, err2 := NormalizeGitURL(url2)
	if err1 != nil || err2 != nil {
		return strings.EqualFold(strings.TrimSpace(url1), strings.TrimSpace(url2))
	}
	return strings.EqualFold(norm1, norm2)
}

// ExtractRepoDetails extracts the host, owner/group namespace, and repository name from a Git URL
func ExtractRepoDetails(raw string) (host, owner, repoName string, err error) {
	canonical, err := NormalizeGitURL(raw)
	if err != nil {
		return "", "", "", err
	}

	parts := strings.Split(canonical, "/")
	if len(parts) < 3 {
		return "", "", "", errors.New("repository url lacks host, owner, and repository name")
	}

	host = parts[0]
	repoName = parts[len(parts)-1]
	owner = strings.Join(parts[1:len(parts)-1], "/")
	return host, owner, repoName, nil
}

// CleanBranchName normalizes a Git ref string into a clean branch name
// e.g. "refs/heads/feature/xyz" -> "feature/xyz"
func CleanBranchName(ref string) string {
	ref = strings.TrimSpace(ref)
	ref = strings.TrimPrefix(ref, "refs/heads/")
	ref = strings.TrimPrefix(ref, "refs/tags/")
	return ref
}
