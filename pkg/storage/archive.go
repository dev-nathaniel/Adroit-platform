package storage

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

var (
	ErrEmptyArchive        = errors.New("uploaded archive is empty")
	ErrUnsupportedArchive  = errors.New("unsupported archive format; expected .zip, .tar.gz, or .tar")
	ErrCorruptArchive      = errors.New("archive is corrupted or malformed")
)

// ArchiveType represents recognized archive formats
type ArchiveType string

const (
	ArchiveTypeZip   ArchiveType = "zip"
	ArchiveTypeTarGz ArchiveType = "tar.gz"
	ArchiveTypeTar   ArchiveType = "tar"
)

// DetectArchiveType identifies the archive format by magic headers
func DetectArchiveType(data []byte) ArchiveType {
	if len(data) >= 4 && data[0] == 0x50 && data[1] == 0x4B && (data[2] == 0x03 || data[2] == 0x05 || data[2] == 0x07) {
		return ArchiveTypeZip
	}
	if len(data) >= 2 && data[0] == 0x1F && data[1] == 0x8B {
		return ArchiveTypeTarGz
	}
	// Tar format has "ustar" at byte 257 in POSIX tar header
	if len(data) >= 262 && string(data[257:262]) == "ustar" {
		return ArchiveTypeTar
	}
	return ""
}

// NormalizedArchive contains normalized tar.gz bytes and metadata
type NormalizedArchive struct {
	TarGzData []byte
	Checksum  string // "sha256:..."
	SizeBytes int64
	FileCount int
	Filename  string
}

// NormalizeToTarGz inspects an input stream and produces a standard .tar.gz bundle
// BuildKit natively downloads and unpacks HTTP tarballs via --opt context=http://...
func NormalizeToTarGz(r io.Reader, originalFilename string) (*NormalizedArchive, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read archive stream: %w", err)
	}

	if len(data) == 0 {
		return nil, ErrEmptyArchive
	}

	// 1. Detect format by content header or file extension
	detected := DetectArchiveType(data)
	ext := strings.ToLower(filepath.Ext(originalFilename))
	if detected == "" {
		if strings.HasSuffix(strings.ToLower(originalFilename), ".tar.gz") || ext == ".tgz" {
			detected = ArchiveTypeTarGz
		} else if ext == ".zip" {
			detected = ArchiveTypeZip
		} else if ext == ".tar" {
			detected = ArchiveTypeTar
		}
	}

	var targzBytes []byte
	var fileCount int

	switch detected {
	case ArchiveTypeZip:
		targzBytes, fileCount, err = zipToTarGz(data)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrCorruptArchive, err)
		}

	case ArchiveTypeTarGz:
		// Validate gzip stream
		gzr, gzErr := gzip.NewReader(bytes.NewReader(data))
		if gzErr != nil {
			return nil, fmt.Errorf("%w: invalid gzip header: %v", ErrCorruptArchive, gzErr)
		}
		defer gzr.Close()

		tr := tar.NewReader(gzr)
		for {
			_, tErr := tr.Next()
			if tErr == io.EOF {
				break
			}
			if tErr != nil {
				return nil, fmt.Errorf("%w: invalid tar structure: %v", ErrCorruptArchive, tErr)
			}
			fileCount++
		}
		targzBytes = data

	case ArchiveTypeTar:
		// Compress uncompressed tar to tar.gz
		var buf bytes.Buffer
		gzw := gzip.NewWriter(&buf)
		if _, wErr := gzw.Write(data); wErr != nil {
			return nil, wErr
		}
		if cErr := gzw.Close(); cErr != nil {
			return nil, cErr
		}
		targzBytes = buf.Bytes()
		fileCount = 1

	default:
		return nil, ErrUnsupportedArchive
	}

	// Calculate SHA256 checksum
	h := sha256.Sum256(targzBytes)
	checksum := "sha256:" + hex.EncodeToString(h[:])

	base := strings.TrimSuffix(originalFilename, filepath.Ext(originalFilename))
	if base == "" {
		base = "source"
	}
	canonicalName := fmt.Sprintf("%s.tar.gz", base)

	return &NormalizedArchive{
		TarGzData: targzBytes,
		Checksum:  checksum,
		SizeBytes: int64(len(targzBytes)),
		FileCount: fileCount,
		Filename:  canonicalName,
	}, nil
}

// zipToTarGz converts a zip byte slice into a .tar.gz byte slice
func zipToTarGz(zipData []byte) ([]byte, int, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, 0, err
	}

	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	fileCount := 0
	for _, f := range zr.File {
		// Clean and skip unsafe paths
		cleanPath := filepath.ToSlash(filepath.Clean(f.Name))
		if strings.HasPrefix(cleanPath, "../") || strings.HasPrefix(cleanPath, "/") {
			continue
		}

		rc, openErr := f.Open()
		if openErr != nil {
			return nil, 0, openErr
		}

		header := &tar.Header{
			Name:     cleanPath,
			Mode:     int64(f.Mode()),
			Size:     int64(f.UncompressedSize64),
			ModTime:  f.Modified,
			Typeflag: tar.TypeReg,
		}

		if f.FileInfo().IsDir() {
			header.Typeflag = tar.TypeDir
			header.Size = 0
			if !strings.HasSuffix(header.Name, "/") {
				header.Name += "/"
			}
		}

		if err := tw.WriteHeader(header); err != nil {
			rc.Close()
			return nil, 0, err
		}

		if !f.FileInfo().IsDir() {
			if _, err := io.Copy(tw, rc); err != nil {
				rc.Close()
				return nil, 0, err
			}
			fileCount++
		}
		rc.Close()
	}

	if err := tw.Close(); err != nil {
		return nil, 0, err
	}
	if err := gzw.Close(); err != nil {
		return nil, 0, err
	}

	return buf.Bytes(), fileCount, nil
}
