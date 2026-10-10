package archive

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// DefaultIgnorePatterns contains patterns excluded from archive packages by default.
var DefaultIgnorePatterns = []string{
	".git",
	".git/*",
	"node_modules",
	"node_modules/*",
	"vendor",
	"vendor/*",
	".env*",
	"*.log",
	"*.tmp",
	".idea",
	".vscode",
	"dist",
	"build",
	"target",
	"bin",
	"*.exe",
	"*.tar.gz",
	"*.zip",
	".adroit",
}

// IgnoreMatcher checks whether files match ignore criteria.
type IgnoreMatcher struct {
	patterns []string
}

// NewIgnoreMatcher loads ignore rules from directory (.adroitignore, .gitignore, and defaults).
func NewIgnoreMatcher(dir string, customPatterns []string) *IgnoreMatcher {
	patterns := append([]string{}, DefaultIgnorePatterns...)
	patterns = append(patterns, customPatterns...)

	// Load .gitignore
	if gitignore, err := os.Open(filepath.Join(dir, ".gitignore")); err == nil {
		defer gitignore.Close()
		scanner := bufio.NewScanner(gitignore)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" && !strings.HasPrefix(line, "#") {
				patterns = append(patterns, line)
			}
		}
	}

	// Load .adroitignore
	if adroitignore, err := os.Open(filepath.Join(dir, ".adroitignore")); err == nil {
		defer adroitignore.Close()
		scanner := bufio.NewScanner(adroitignore)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" && !strings.HasPrefix(line, "#") {
				patterns = append(patterns, line)
			}
		}
	}

	return &IgnoreMatcher{patterns: patterns}
}

// ShouldIgnore tests if the relative file path matches any ignore pattern.
func (m *IgnoreMatcher) ShouldIgnore(relPath string) bool {
	// Normalize path separators to forward slash
	cleanPath := filepath.ToSlash(filepath.Clean(relPath))
	parts := strings.Split(cleanPath, "/")

	for _, pattern := range m.patterns {
		pat := filepath.ToSlash(pattern)
		pat = strings.TrimPrefix(pat, "./")
		pat = strings.TrimPrefix(pat, "/")

		// Exact match or directory component match
		for _, part := range parts {
			if matched, _ := filepath.Match(pat, part); matched {
				return true
			}
		}

		// Full path match
		if matched, _ := filepath.Match(pat, cleanPath); matched {
			return true
		}

		// Prefix wildcard match (e.g. "dist/*")
		if strings.HasSuffix(pat, "/*") {
			base := strings.TrimSuffix(pat, "/*")
			if strings.HasPrefix(cleanPath, base+"/") || cleanPath == base {
				return true
			}
		}
	}

	return false
}
