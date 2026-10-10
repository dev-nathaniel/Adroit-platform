package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// PackResult summarizes the packaged archive.
type PackResult struct {
	Buffer      *bytes.Buffer
	SizeBytes   int64
	FileCount   int
	SHA256      string
	RuntimeType ProjectType
}

// PackDirectory packages the target directory into a compressed .tar.gz bundle.
func PackDirectory(dir string, customIgnores []string) (*PackResult, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve directory: %w", err)
	}

	fi, err := os.Stat(absDir)
	if err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("target path is not a valid directory: %s", absDir)
	}

	runtime := DetectRuntime(absDir)
	matcher := NewIgnoreMatcher(absDir, customIgnores)

	buf := new(bytes.Buffer)
	hasher := sha256.New()
	multiWriter := io.MultiWriter(buf, hasher)

	gw := gzip.NewWriter(multiWriter)
	tw := tar.NewWriter(gw)

	fileCount := 0

	err = filepath.Walk(absDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		// Calculate relative path
		relPath, err := filepath.Rel(absDir, path)
		if err != nil {
			return err
		}

		if relPath == "." {
			return nil
		}

		// Check ignore filter
		if matcher.ShouldIgnore(relPath) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Standardize header
		header, err := tar.FileInfoHeader(info, info.Name())
		if err != nil {
			return err
		}

		// Use normalized unix path in archive
		header.Name = filepath.ToSlash(relPath)

		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write tar header for %s: %w", relPath, err)
		}

		if info.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("failed to open file %s: %w", path, err)
			}
			defer file.Close()

			if _, err := io.Copy(tw, file); err != nil {
				return fmt.Errorf("failed to write file %s to archive: %w", relPath, err)
			}
			fileCount++
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to pack directory: %w", err)
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}

	hashStr := hex.EncodeToString(hasher.Sum(nil))

	return &PackResult{
		Buffer:      buf,
		SizeBytes:   int64(buf.Len()),
		FileCount:   fileCount,
		SHA256:      hashStr,
		RuntimeType: runtime,
	}, nil
}
