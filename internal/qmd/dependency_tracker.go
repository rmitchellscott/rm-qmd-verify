package qmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rmitchellscott/rm-qmd-verify/internal/logging"
)

// DependencyInfo contains information about a QMD file's dependencies
type DependencyInfo struct {
	RootFile      string              // The root QMD file being validated
	ExpectedLoads []string            // All files expected to be LOADed (in discovered order)
	LoadOrder     map[string]int      // Map of file path to first occurrence position
	LoadGraph     map[string][]string // Parent file -> child files loaded by it
}

// ExtractLoadStatements parses a QMD file and extracts LOAD statements
// Returns the list of file paths in the order they appear
func ExtractLoadStatements(qmdPath string) ([]string, error) {
	content, err := os.ReadFile(qmdPath)
	if err != nil {
		return nil, err
	}

	// Regex to match LOAD statements (but not LOAD EXTERNAL)
	// Matches: "LOAD <path>" at the start of a line
	loadRegex := regexp.MustCompile(`(?m)^LOAD\s+([^\s]+)`)

	// Exclude LOAD EXTERNAL
	loadExternalRegex := regexp.MustCompile(`(?m)^LOAD\s+EXTERNAL`)

	contentStr := string(content)
	loads := []string{}

	// Find all LOAD statements
	matches := loadRegex.FindAllStringSubmatch(contentStr, -1)
	positions := loadRegex.FindAllStringSubmatchIndex(contentStr, -1)

	for i, match := range matches {
		if len(match) > 1 {
			// Check if this is LOAD EXTERNAL by examining the full match
			pos := positions[i]
			lineStart := pos[0]
			lineEnd := pos[1]
			fullLine := contentStr[lineStart:lineEnd]

			if !loadExternalRegex.MatchString(fullLine) {
				loadPath := strings.TrimSpace(match[1])
				loads = append(loads, loadPath)
			}
		}
	}

	logging.Debug(logging.ComponentQMD, "Extracted %d LOAD statements from %s", len(loads), qmdPath)
	return loads, nil
}

// BuildDependencyInfo creates a complete dependency map for a QMD file by recursively
// following all LOAD statements to build a complete dependency tree
func BuildDependencyInfo(qmdPath string) (*DependencyInfo, error) {
	// Initialize data structures
	allLoads := []string{}
	loadOrder := make(map[string]int)
	loadGraph := make(map[string][]string)
	visited := make(map[string]bool)

	// Get root file directory for path normalization
	rootDir := filepath.Dir(qmdPath)

	// Queue for BFS traversal: each item is (filePath, parentPath, depth)
	type queueItem struct {
		filePath   string
		parentPath string
		depth      int
	}
	queue := []queueItem{{filePath: qmdPath, parentPath: "", depth: 0}}
	visited[qmdPath] = true

	const maxDepth = 100

	// BFS traversal to discover all dependencies
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		// Check depth limit
		if current.depth > maxDepth {
			return nil, fmt.Errorf("LOAD nesting too deep (max %d levels)", maxDepth)
		}

		// Extract LOAD statements from current file
		loads, err := ExtractLoadStatements(current.filePath)
		if err != nil {
			// File not found or read error - log warning but continue
			logging.Warn(logging.ComponentQMD, "Cannot read file %s: %v", current.filePath, err)
			continue
		}

		// Track the children of this file
		children := []string{}

		// Process each LOAD statement
		for _, loadPath := range loads {
			// Resolve relative to current file
			resolvedPath := ResolveLoadPath(current.filePath, loadPath)

			// Normalize path to be relative to root file directory
			normalizedPath, err := filepath.Rel(rootDir, resolvedPath)
			if err != nil {
				// If we can't make it relative, use the basename as fallback
				logging.Warn(logging.ComponentQMD, "Cannot make path %s relative to %s: %v", resolvedPath, rootDir, err)
				normalizedPath = filepath.Base(resolvedPath)
			}

			// Track this child using normalized path
			children = append(children, normalizedPath)

			// Check for circular dependency
			if visited[resolvedPath] {
				// File already in dependency tree - could be circular or just duplicate LOAD
				logging.Debug(logging.ComponentQMD, "File %s already visited (loaded by multiple files or circular)", normalizedPath)
				continue
			}

			// Add to discovered loads and mark as visited (using normalized path)
			allLoads = append(allLoads, normalizedPath)
			loadOrder[normalizedPath] = len(allLoads) - 1
			visited[resolvedPath] = true

			// Add to queue for processing
			queue = append(queue, queueItem{
				filePath:   resolvedPath,
				parentPath: current.filePath,
				depth:      current.depth + 1,
			})
		}

		// Record parent-child relationship in graph
		if len(children) > 0 {
			loadGraph[current.filePath] = children
		}
	}

	info := &DependencyInfo{
		RootFile:      qmdPath,
		ExpectedLoads: allLoads,
		LoadOrder:     loadOrder,
		LoadGraph:     loadGraph,
	}

	logging.Info(logging.ComponentQMD, "Built dependency info for %s: %d expected loads (recursive)",
		filepath.Base(qmdPath), len(allLoads))

	return info, nil
}

// GetRootLevelFiles returns only the .qmd files at the root of the given directory
// (mimics qmldiff's behavior of not recursing into subdirectories)
func GetRootLevelFiles(baseDir string, allUploadedPaths []string) []string {
	rootFiles := []string{}

	for _, path := range allUploadedPaths {
		// Get relative path from base directory
		relPath, err := filepath.Rel(baseDir, path)
		if err != nil {
			continue
		}

		// Check if file is at root level (no directory separators in relative path)
		if !strings.Contains(relPath, string(filepath.Separator)) &&
		   strings.HasSuffix(strings.ToLower(relPath), ".qmd") {
			rootFiles = append(rootFiles, path)
		}
	}

	logging.Info(logging.ComponentQMD, "Found %d root-level QMD files in %s",
		len(rootFiles), baseDir)

	return rootFiles
}

// ResolveLoadPath resolves a LOAD path relative to the loading file
// Matches qmldiff's path resolution logic
func ResolveLoadPath(loadingFile string, loadPath string) string {
	// Get the directory of the file doing the loading
	loadingDir := filepath.Dir(loadingFile)

	// Resolve the path relative to that directory
	resolved := filepath.Join(loadingDir, loadPath)

	return filepath.Clean(resolved)
}
