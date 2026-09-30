package gitintegrationservice

import (
	"testing"
)

func TestNormalizeGitURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		wantErr  bool
	}{
		{
			input:    "https://github.com/octocat/Hello-World.git",
			expected: "github.com/octocat/Hello-World",
			wantErr:  false,
		},
		{
			input:    "https://github.com/octocat/Hello-World",
			expected: "github.com/octocat/Hello-World",
			wantErr:  false,
		},
		{
			input:    "git@github.com:octocat/Hello-World.git",
			expected: "github.com/octocat/Hello-World",
			wantErr:  false,
		},
		{
			input:    "git@github.com:octocat/Hello-World",
			expected: "github.com/octocat/Hello-World",
			wantErr:  false,
		},
		{
			input:    "ssh://git@gitlab.com/group/subgroup/project.git",
			expected: "gitlab.com/group/subgroup/project",
			wantErr:  false,
		},
		{
			input:    "https://gitlab.com/group/subgroup/project.git",
			expected: "gitlab.com/group/subgroup/project",
			wantErr:  false,
		},
		{
			input:    "git@gitlab.company.internal:engineering/core/api.git",
			expected: "gitlab.company.internal/engineering/core/api",
			wantErr:  false,
		},
		{
			input:    "",
			expected: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		got, err := NormalizeGitURL(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("NormalizeGitURL(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.expected {
			t.Errorf("NormalizeGitURL(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestGitURLsMatch(t *testing.T) {
	url1 := "https://github.com/my-org/my-service.git"
	url2 := "git@github.com:my-org/my-service.git"
	url3 := "https://github.com/my-org/other-service.git"

	if !GitURLsMatch(url1, url2) {
		t.Errorf("expected %q and %q to match", url1, url2)
	}

	if GitURLsMatch(url1, url3) {
		t.Errorf("expected %q and %q NOT to match", url1, url3)
	}
}

func TestExtractRepoDetails(t *testing.T) {
	host, owner, name, err := ExtractRepoDetails("https://gitlab.com/acme/backend/microservice.git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "gitlab.com" {
		t.Errorf("expected host gitlab.com, got %s", host)
	}
	if owner != "acme/backend" {
		t.Errorf("expected owner acme/backend, got %s", owner)
	}
	if name != "microservice" {
		t.Errorf("expected name microservice, got %s", name)
	}
}

func TestCleanBranchName(t *testing.T) {
	if CleanBranchName("refs/heads/main") != "main" {
		t.Errorf("expected main, got %s", CleanBranchName("refs/heads/main"))
	}
	if CleanBranchName("refs/heads/feature/cool-stuff") != "feature/cool-stuff" {
		t.Errorf("expected feature/cool-stuff, got %s", CleanBranchName("refs/heads/feature/cool-stuff"))
	}
}
