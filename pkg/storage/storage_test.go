package storage

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInMemStorage(t *testing.T) {
	ctx := context.Background()
	store := NewInMemStorage("http://localhost:8080/api/v1/source/download")

	content := "package main\nfunc main() {}"
	url, err := store.Save(ctx, "app/source.tar.gz", strings.NewReader(content))
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if !strings.HasSuffix(url, "app/source.tar.gz") {
		t.Errorf("unexpected URL: %s", url)
	}

	rc, size, err := store.Open(ctx, "app/source.tar.gz")
	if err != nil {
		t.Fatalf("open failed: %v", err)
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	if string(data) != content {
		t.Errorf("expected %q, got %q", content, string(data))
	}
	if size != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), size)
	}

	if err := store.Delete(ctx, "app/source.tar.gz"); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	_, _, err = store.Open(ctx, "app/source.tar.gz")
	if err != ErrFileNotFound {
		t.Errorf("expected ErrFileNotFound, got %v", err)
	}
}

func TestLocalStorage(t *testing.T) {
	ctx := context.Background()
	tempDir, err := os.MkdirTemp("", "storage-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	store, err := NewLocalStorage(tempDir, "/api/v1/services/source/download")
	if err != nil {
		t.Fatalf("failed to init LocalStorage: %v", err)
	}

	content := "test local storage content"
	url, err := store.Save(ctx, "test-service/bundle.tar.gz", strings.NewReader(content))
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if !strings.Contains(url, "test-service/bundle.tar.gz") {
		t.Errorf("unexpected URL: %s", url)
	}

	rc, size, err := store.Open(ctx, "test-service/bundle.tar.gz")
	if err != nil {
		t.Fatalf("open failed: %v", err)
	}
	defer rc.Close()
	readBytes, _ := io.ReadAll(rc)
	if string(readBytes) != content {
		t.Errorf("content mismatch")
	}
	if size != int64(len(content)) {
		t.Errorf("size mismatch")
	}

	diskPath := store.GetPath("test-service/bundle.tar.gz")
	if _, err := os.Stat(diskPath); err != nil {
		t.Errorf("file not on disk at %s: %v", diskPath, err)
	}
}

func TestNormalizeToTarGz_ZipInput(t *testing.T) {
	// Create a test zip archive in memory
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)

	f1, err := zw.Create("index.js")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f1.Write([]byte("console.log('hello from zip');"))

	f2, err := zw.Create("package.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f2.Write([]byte(`{"name":"test-app","version":"1.0.0"}`))

	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	norm, err := NormalizeToTarGz(&zipBuf, "app-archive.zip")
	if err != nil {
		t.Fatalf("NormalizeToTarGz failed: %v", err)
	}

	if !strings.HasPrefix(norm.Checksum, "sha256:") {
		t.Errorf("expected sha256 checksum, got %s", norm.Checksum)
	}
	if norm.FileCount != 2 {
		t.Errorf("expected 2 files, got %d", norm.FileCount)
	}
	if filepath.Ext(norm.Filename) != ".gz" {
		t.Errorf("expected .tar.gz extension, got %s", norm.Filename)
	}

	// Verify the returned bytes can be read as a valid tar.gz
	gzr, err := gzip.NewReader(bytes.NewReader(norm.TarGzData))
	if err != nil {
		t.Fatalf("failed to read gzip output: %v", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	filesFound := make(map[string]string)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar read error: %v", err)
		}
		content, _ := io.ReadAll(tr)
		filesFound[hdr.Name] = string(content)
	}

	if filesFound["index.js"] != "console.log('hello from zip');" {
		t.Errorf("index.js content mismatch")
	}
	if !strings.Contains(filesFound["package.json"], "test-app") {
		t.Errorf("package.json content mismatch")
	}
}

func TestNormalizeToTarGz_TarGzInput(t *testing.T) {
	// Create a test tar.gz
	var tgzBuf bytes.Buffer
	gzw := gzip.NewWriter(&tgzBuf)
	tw := tar.NewWriter(gzw)

	hdr := &tar.Header{
		Name: "main.go",
		Mode: 0644,
		Size: int64(len("package main")),
	}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write([]byte("package main"))
	_ = tw.Close()
	_ = gzw.Close()

	norm, err := NormalizeToTarGz(&tgzBuf, "source.tar.gz")
	if err != nil {
		t.Fatalf("NormalizeToTarGz failed on tar.gz: %v", err)
	}
	if norm.FileCount != 1 {
		t.Errorf("expected 1 file, got %d", norm.FileCount)
	}
}
