package qmd

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/rmitchellscott/rm-qmd-verify/internal/logging"
)

// FileStatus represents the validation status of a single QMD file
type FileStatus string

const (
	StatusValidated     FileStatus = "validated"      // File was successfully validated
	StatusFailed        FileStatus = "failed"          // File had errors during validation
	StatusNotAttempted  FileStatus = "not_attempted"   // File was not validated due to prior failure
)

// ValidationResult contains the results for a single QMD file
type ValidationResult struct {
	Path             string      `json:"path"`
	Status           FileStatus  `json:"status"`
	Compatible       bool        `json:"compatible"`
	HashErrors       []HashError `json:"hash_errors,omitempty"`
	ProcessErrors    []string    `json:"process_errors,omitempty"`
	QMLFilesModified []string    `json:"qml_files_modified,omitempty"`
	Position         int         `json:"position"` // Position in LOAD order
}

// HashError represents a hash lookup error
type HashError struct {
	HashID uint64 `json:"hash_id"`
	Error  string `json:"error"`
}

// ParsedOutput contains all parsed information from qmldiff output
type ParsedOutput struct {
	HashErrors       map[string][]HashError  // QMD file -> hash errors
	ProcessErrors    map[string][]string     // QMD file -> process errors
	WrittenFiles     map[string][]string     // QMD file -> QML files modified
	ProcessedFiles   map[string]bool         // Which QMD files were actually processed
	FailureFile      string                  // First file that caused failure (if any)
	HadPanic         bool                    // Whether qmldiff panicked
	PanicMessage     string                  // The panic message if it panicked
}

// ParseQmdiffOutput parses the output from qmldiff CLI
func ParseQmdiffOutput(output string) *ParsedOutput {
	result := &ParsedOutput{
		HashErrors:     make(map[string][]HashError),
		ProcessErrors:  make(map[string][]string),
		WrittenFiles:   make(map[string][]string),
		ProcessedFiles: make(map[string]bool),
	}

	// Regex patterns
	// Hash error: "file.qmd - Cannot resolve hash 123"
	hashErrorRegex := regexp.MustCompile(`(.+\.qmd) - Cannot resolve hash (\d+)`)

	// Process error: "(On behalf of 'file.qmd'): error message"
	processErrorRegex := regexp.MustCompile(`\(On behalf of '(.+\.qmd)'\): (.+)`)

	// Written file: "Written file /path/to/file.qml - N diff(s) applied"
	writtenFileRegex := regexp.MustCompile(`Written file (.+\.qml) - (\d+) diff\(s\) applied`)

	// File not found error: "Cannot read file <path>"
	fileNotFoundRegex := regexp.MustCompile(`Cannot read file (.+\.qmd)`)

	// Panic detection: "thread 'main' panicked at"
	panicRegex := regexp.MustCompile(`thread '.+' panicked at`)

	lines := strings.Split(output, "\n")

	// First check if there's a panic in the entire output
	if panicRegex.MatchString(output) {
		result.HadPanic = true
		for _, line := range lines {
			if strings.Contains(line, "panicked at") {
				result.PanicMessage = strings.TrimSpace(line)
				break
			}
		}
		logging.Debug(logging.ComponentQMD, "Detected qmldiff panic: %s", result.PanicMessage)
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Check for hash errors
		if matches := hashErrorRegex.FindStringSubmatch(line); len(matches) == 3 {
			qmdFile := matches[1]
			hashID, _ := strconv.ParseUint(matches[2], 10, 64)

			result.HashErrors[qmdFile] = append(result.HashErrors[qmdFile], HashError{
				HashID: hashID,
				Error:  line,
			})
			result.ProcessedFiles[qmdFile] = true

			logging.Debug(logging.ComponentQMD, "Parsed hash error from %s: hash %d", qmdFile, hashID)
		}

		// Check for process errors
		if matches := processErrorRegex.FindStringSubmatch(line); len(matches) == 3 {
			qmdFile := matches[1]
			errorMsg := matches[2]

			result.ProcessErrors[qmdFile] = append(result.ProcessErrors[qmdFile], errorMsg)
			result.ProcessedFiles[qmdFile] = true

			logging.Debug(logging.ComponentQMD, "Parsed process error from %s: %s", qmdFile, errorMsg)
		}

		// Check for file not found errors (indicates LOAD failure)
		if matches := fileNotFoundRegex.FindStringSubmatch(line); len(matches) == 2 {
			qmdFile := matches[1]
			if result.FailureFile == "" {
				result.FailureFile = qmdFile
			}

			logging.Debug(logging.ComponentQMD, "Detected LOAD failure at: %s", qmdFile)
		}

		// Check for written files (doesn't directly tell us QMD, but shows success)
		if matches := writtenFileRegex.FindStringSubmatch(line); len(matches) == 3 {
			qmlFile := matches[1]
			// We'll associate these with QMD files later
			logging.Debug(logging.ComponentQMD, "QML file modified: %s", qmlFile)
		}
	}

	return result
}

// ReconcileResults combines expected dependencies with actual results
func ReconcileResults(depInfo *DependencyInfo, parsedOutput *ParsedOutput) map[string]*ValidationResult {
	results := make(map[string]*ValidationResult)

	// Start with the root file
	rootResult := &ValidationResult{
		Path:       filepath.Base(depInfo.RootFile),
		Status:     StatusValidated,
		Compatible: true,
		Position:   -1, // Root file is position -1
	}

	// If qmldiff panicked, mark root as failed and all dependencies as not_attempted
	if parsedOutput.HadPanic {
		rootResult.Status = StatusFailed
		rootResult.Compatible = false
		rootResult.ProcessErrors = append(rootResult.ProcessErrors, "qmldiff panicked: "+parsedOutput.PanicMessage)
		rootFileName := filepath.Base(depInfo.RootFile)
		results[rootFileName] = rootResult

		// Mark all expected loads as not_attempted
		for i, expectedFile := range depInfo.ExpectedLoads {
			results[expectedFile] = &ValidationResult{
				Path:       expectedFile,
				Position:   i,
				Status:     StatusNotAttempted,
				Compatible: false,
			}
		}

		logging.Info(logging.ComponentQMD, "Reconciled results: %d files total, all not_attempted due to panic", len(results))
		return results
	}

	// Check if root file had errors
	if hashErrs, exists := parsedOutput.HashErrors[depInfo.RootFile]; exists {
		rootResult.HashErrors = hashErrs
		rootResult.Compatible = false
		rootResult.Status = StatusFailed
	}
	if procErrs, exists := parsedOutput.ProcessErrors[depInfo.RootFile]; exists {
		rootResult.ProcessErrors = procErrs
		rootResult.Compatible = false
		rootResult.Status = StatusFailed
	}

	rootFileName := filepath.Base(depInfo.RootFile)
	results[rootFileName] = rootResult

	// Process each expected LOAD
	failurePoint := -1

	for i, expectedFile := range depInfo.ExpectedLoads {
		// Resolve the path (LOAD paths are relative to root file)
		resolvedPath := ResolveLoadPath(depInfo.RootFile, expectedFile)

		result := &ValidationResult{
			Path:       expectedFile, // Keep original path from LOAD statement
			Position:   i,
			Compatible: true,
			Status:     StatusValidated,
		}

		// Check if we already hit a failure
		if failurePoint != -1 && i > failurePoint {
			result.Status = StatusNotAttempted
			result.Compatible = false
			results[expectedFile] = result
			logging.Debug(logging.ComponentQMD, "File not attempted: %s (stopped at position %d)", expectedFile, failurePoint)
			continue
		}

		// Check if this file was processed
		wasProcessed := parsedOutput.ProcessedFiles[expectedFile] ||
			parsedOutput.ProcessedFiles[resolvedPath]

		// Check if this file is the failure point
		isFailure := parsedOutput.FailureFile == expectedFile ||
			parsedOutput.FailureFile == resolvedPath

		if isFailure {
			failurePoint = i
			result.Status = StatusFailed
			result.Compatible = false
			result.ProcessErrors = append(result.ProcessErrors, "LOAD failed: Cannot read file")
			logging.Debug(logging.ComponentQMD, "File caused failure: %s", expectedFile)
		} else if wasProcessed {
			// Check for errors
			if hashErrs, exists := parsedOutput.HashErrors[expectedFile]; exists {
				result.HashErrors = hashErrs
				result.Compatible = false
			}
			if hashErrs, exists := parsedOutput.HashErrors[resolvedPath]; exists {
				result.HashErrors = append(result.HashErrors, hashErrs...)
				result.Compatible = false
			}

			if procErrs, exists := parsedOutput.ProcessErrors[expectedFile]; exists {
				result.ProcessErrors = procErrs
				result.Compatible = false
			}
			if procErrs, exists := parsedOutput.ProcessErrors[resolvedPath]; exists {
				result.ProcessErrors = append(result.ProcessErrors, procErrs...)
				result.Compatible = false
			}

			if len(result.HashErrors) > 0 || len(result.ProcessErrors) > 0 {
				result.Status = StatusFailed
			}
		} else if !isFailure {
			// File was expected but not in output and not a failure - likely succeeded silently
			result.Status = StatusValidated
			result.Compatible = true
		}

		results[expectedFile] = result
	}

	logging.Info(logging.ComponentQMD, "Reconciled results: %d files total, failure at position %d",
		len(results), failurePoint)

	return results
}
