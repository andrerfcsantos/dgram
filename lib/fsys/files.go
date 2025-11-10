package fsys

import (
	"os"
	"path/filepath"
)

func FileExists(file string) bool {
	_, err := os.Stat(file)
	return !os.IsNotExist(err)
}

// IsBeingDownloaded checks if a file is currently being downloaded by checking
// for the existence of temporary download files with common browser suffixes.
// Common suffixes:
//   - .part (Firefox, wget, curl)
//   - .crdownload (Chrome, Edge, Opera)
//   - .download (Safari)
//   - .partial (IE/Edge Legacy)
//   - .tmp (various download tools)
//   - .aria2 (aria2 download manager)
//
// Also handles Firefox's hash-based temporary files (e.g., "video.a1b2c3.mp4.part")
func IsBeingDownloaded(filePath string) bool {
	downloadSuffixes := []string{
		".part",
		".crdownload",
		".download",
		".partial",
		".tmp",
		".aria2",
	}

	// Check if any temporary download file exists (exact match)
	for _, suffix := range downloadSuffixes {
		tempFile := filePath + suffix
		if FileExists(tempFile) {
			return true
		}
	}

	dir := filepath.Dir(filePath)
	baseName := filepath.Base(filePath)
	ext := filepath.Ext(baseName)
	baseNameWithoutExt := baseName[:len(baseName)-len(ext)]

	// Check if the base file exists with these extensions
	// (e.g., "video.mp4" might have "video.part" as companion)
	for _, suffix := range downloadSuffixes {
		tempFile := filepath.Join(dir, baseNameWithoutExt+suffix)
		if FileExists(tempFile) {
			return true
		}
	}

	// Check for Firefox-style hash patterns (e.g., "video.a1b2c3.mp4.part")
	// Pattern: basename.*.ext.suffix
	for _, suffix := range downloadSuffixes {
		// Build glob pattern: "video.*.mp4.part"
		pattern := filepath.Join(dir, baseNameWithoutExt+".*"+ext+suffix)
		matches, err := filepath.Glob(pattern)
		if err == nil && len(matches) > 0 {
			return true
		}
	}

	return false
}
