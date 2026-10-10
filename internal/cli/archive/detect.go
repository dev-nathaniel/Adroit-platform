package archive

import (
	"os"
	"path/filepath"
)

// ProjectType represents the detected application technology stack.
type ProjectType struct {
	Name        string // "dockerfile", "nodejs", "python", "go", "rust", "ruby", "java", "static"
	DefaultPort int
	HasDocker   bool
}

// DetectRuntime analyzes the root of a project directory to determine the runtime framework.
func DetectRuntime(dir string) ProjectType {
	hasFile := func(name string) bool {
		fi, err := os.Stat(filepath.Join(dir, name))
		return err == nil && !fi.IsDir()
	}

	hasDocker := hasFile("Dockerfile") || hasFile("dockerfile")

	switch {
	case hasDocker:
		return ProjectType{Name: "dockerfile", DefaultPort: 8080, HasDocker: true}
	case hasFile("package.json"):
		return ProjectType{Name: "nodejs", DefaultPort: 3000, HasDocker: false}
	case hasFile("requirements.txt") || hasFile("pyproject.toml") || hasFile("Pipfile"):
		return ProjectType{Name: "python", DefaultPort: 8000, HasDocker: false}
	case hasFile("go.mod"):
		return ProjectType{Name: "go", DefaultPort: 8080, HasDocker: false}
	case hasFile("Cargo.toml"):
		return ProjectType{Name: "rust", DefaultPort: 8080, HasDocker: false}
	case hasFile("Gemfile"):
		return ProjectType{Name: "ruby", DefaultPort: 3000, HasDocker: false}
	case hasFile("pom.xml") || hasFile("build.gradle"):
		return ProjectType{Name: "java", DefaultPort: 8080, HasDocker: false}
	case hasFile("index.html"):
		return ProjectType{Name: "static", DefaultPort: 80, HasDocker: false}
	default:
		return ProjectType{Name: "generic", DefaultPort: 8080, HasDocker: false}
	}
}
