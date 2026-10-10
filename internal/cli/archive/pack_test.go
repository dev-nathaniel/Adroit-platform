package archive

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestPackDirectory_IgnoresAndCompresses(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Create a dummy node project
	_ = os.WriteFile(filepath.Join(tempDir, "package.json"), []byte(`{"name":"test-app"}`), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, "index.js"), []byte(`console.log("hello world");`), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, ".env"), []byte(`SECRET=123`), 0644)

	// Subdirectory that should be ignored
	nodeModules := filepath.Join(tempDir, "node_modules", "express")
	_ = os.MkdirAll(nodeModules, 0755)
	_ = os.WriteFile(filepath.Join(nodeModules, "index.js"), []byte(`module.exports={}`), 0644)

	res, err := PackDirectory(tempDir, nil)
	if err != nil {
		t.Fatalf("unexpected error packing directory: %v", err)
	}

	if res.RuntimeType.Name != "nodejs" {
		t.Errorf("expected runtime nodejs, got %s", res.RuntimeType.Name)
	}

	// Decompress and verify files in archive
	gr, err := gzip.NewReader(res.Buffer)
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	tr := tar.NewReader(gr)

	foundFiles := make(map[string]bool)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("error reading tar header: %v", err)
		}
		foundFiles[header.Name] = true
	}

	if !foundFiles["package.json"] {
		t.Errorf("expected package.json to be in archive")
	}
	if !foundFiles["index.js"] {
		t.Errorf("expected index.js to be in archive")
	}
	if foundFiles[".env"] {
		t.Errorf(".env should have been ignored")
	}
	for name := range foundFiles {
		if filepath.HasPrefix(name, "node_modules") {
			t.Errorf("node_modules should have been ignored, found: %s", name)
		}
	}
}
