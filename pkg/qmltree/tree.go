package qmltree

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// Tree represents a QML tree directory
type Tree struct {
	Name      string // e.g., "3.22.0.65-rmppm"
	Path      string // Full path to tree directory
	OSVersion string // e.g., "3.22.0.65"
	Device    string // e.g., "rmppm"
	FileCount int    // Number of .qml files in tree
}

// NewTree creates a new Tree from a directory path
func NewTree(path string) (*Tree, error) {
	name := filepath.Base(path)
	version, device := parseNameComponents(name)

	// Count QML files
	fileCount := 0
	filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(strings.ToLower(p), ".qml") {
			fileCount++
		}
		return nil
	})

	return &Tree{
		Name:      name,
		Path:      path,
		OSVersion: version,
		Device:    device,
		FileCount: fileCount,
	}, nil
}

// parseNameComponents extracts version and device from tree directory name
// Expected format: {version}-{device}
// Example: "3.22.0.65-rmppm" → ("3.22.0.65", "rmppm")
func parseNameComponents(name string) (version string, device string) {
	// Find the last hyphen to split version and device
	lastHyphen := strings.LastIndex(name, "-")
	if lastHyphen == -1 {
		return name, ""
	}

	version = name[:lastHyphen]
	device = name[lastHyphen+1:]
	return
}
