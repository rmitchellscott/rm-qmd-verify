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
	BlockedBy        string      `json:"blocked_by,omitempty"` // File that caused validation to stop
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
	PanicFile        string                  // The file being processed when panic occurred
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
	// Hash error: "/path/to/file.qmd - Cannot resolve hash 123"
	// Also supports old format: "Cannot resolve hash 123 required by file.qmd"
	hashErrorRegex := regexp.MustCompile(`(?:(.+\.qmd) - Cannot resolve hash (\d+)|Cannot resolve hash (\d+) required by (.+\.qmd))`)

	// Process error: "(On behalf of 'file.qmd'): error message"
	processErrorRegex := regexp.MustCompile(`\(On behalf of '(.+\.qmd)'\): (.+)`)

	// Written file: "Written file /path/to/file.qml - N diff(s) applied"
	writtenFileRegex := regexp.MustCompile(`Written file (.+\.qml) - (\d+) diff\(s\) applied`)

	// File not found error: "Cannot read file <path>"
	fileNotFoundRegex := regexp.MustCompile(`Cannot read file (.+\.qmd)`)

	// Panic detection: "panicked at"
	panicRegex := regexp.MustCompile(`panicked at`)

	lines := strings.Split(output, "\n")

	// First check if there's a panic in the entire output
	if panicRegex.MatchString(output) {
		result.HadPanic = true
		panicLineIdx := -1
		for i, line := range lines {
			if strings.Contains(line, "panicked at") {
				result.PanicMessage = strings.TrimSpace(line)
				panicLineIdx = i
				break
			}
		}

		// Look forward from panic to find the file mentioned in the error message
		// Panic messages typically have "Cannot resolve hash ... required by <file>!" on the next line
		if panicLineIdx >= 0 && panicLineIdx < len(lines)-1 {
			requiredByRegex := regexp.MustCompile(`required by (.+\.qmd)`)
			// Check the next few lines after the panic
			for i := panicLineIdx + 1; i < len(lines) && i < panicLineIdx+5; i++ {
				if matches := requiredByRegex.FindStringSubmatch(lines[i]); len(matches) == 2 {
					result.PanicFile = filepath.Base(matches[1])
					logging.Debug(logging.ComponentQMD, "Panic occurred while processing file: %s", result.PanicFile)
					break
				}
			}
		}

		// If we didn't find a "required by" line, fall back to looking backwards for "Reading diff"
		if result.PanicFile == "" && panicLineIdx > 0 {
			readingDiffRegex := regexp.MustCompile(`Reading diff (.+\.qmd)`)
			for i := panicLineIdx - 1; i >= 0; i-- {
				if matches := readingDiffRegex.FindStringSubmatch(lines[i]); len(matches) == 2 {
					result.PanicFile = filepath.Base(matches[1])
					logging.Debug(logging.ComponentQMD, "Panic occurred while processing file (from Reading diff): %s", result.PanicFile)
					break
				}
			}
		}

		logging.Debug(logging.ComponentQMD, "Detected qmldiff panic: %s", result.PanicMessage)
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Check for hash errors
		// Regex groups: 1=file(new), 2=hash(new), 3=hash(old), 4=file(old)
		if matches := hashErrorRegex.FindStringSubmatch(line); len(matches) == 5 {
			var hashID uint64
			var qmdFile string

			// Check which format matched
			if matches[1] != "" && matches[2] != "" {
				// New format: "/path/file.qmd - Cannot resolve hash 123"
				qmdFile = matches[1]
				hashID, _ = strconv.ParseUint(matches[2], 10, 64)
			} else if matches[3] != "" && matches[4] != "" {
				// Old format: "Cannot resolve hash 123 required by file.qmd"
				hashID, _ = strconv.ParseUint(matches[3], 10, 64)
				qmdFile = matches[4]
			} else {
				continue // Shouldn't happen, but skip if neither format matched
			}

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

	// If qmldiff panicked, check if we have parseable hash errors
	// If we have hash errors, process them normally; otherwise treat as fatal panic
	if parsedOutput.HadPanic {
		hasHashErrors := len(parsedOutput.HashErrors) > 0

		if !hasHashErrors && parsedOutput.PanicFile == "" {
			// Fatal panic with no parseable hash errors and no identified file - bail out
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
					BlockedBy:  rootFileName,
				}
			}

			logging.Info(logging.ComponentQMD, "Reconciled results: %d files total, all not_attempted due to fatal panic", len(results))
			return results
		}

		// Has hash errors or identified panic file - continue processing
		if parsedOutput.PanicFile != "" {
			logging.Debug(logging.ComponentQMD, "Panic occurred in file %s, will mark it as failed", parsedOutput.PanicFile)
		} else {
			logging.Debug(logging.ComponentQMD, "Panic occurred but hash errors were collected, continuing with normal processing")
		}
	}

	// Debug: Log path matching details for root file
	logging.Debug(logging.ComponentQMD, "=== Root File Path Matching ===")
	logging.Debug(logging.ComponentQMD, "  depInfo.RootFile (looking for): '%s'", depInfo.RootFile)
	logging.Debug(logging.ComponentQMD, "  depInfo.RootFile basename: '%s'", filepath.Base(depInfo.RootFile))

	// Log all error paths
	errorPaths := make([]string, 0, len(parsedOutput.HashErrors))
	for path := range parsedOutput.HashErrors {
		errorPaths = append(errorPaths, path)
	}
	logging.Debug(logging.ComponentQMD, "  parsedOutput.HashErrors keys (%d): %v", len(errorPaths), errorPaths)

	processErrorPaths := make([]string, 0, len(parsedOutput.ProcessErrors))
	for path := range parsedOutput.ProcessErrors {
		processErrorPaths = append(processErrorPaths, path)
	}
	logging.Debug(logging.ComponentQMD, "  parsedOutput.ProcessErrors keys (%d): %v", len(processErrorPaths), processErrorPaths)

	// Check if root file had errors - try multiple path variations
	// Try exact match with absolute path
	if hashErrs, exists := parsedOutput.HashErrors[depInfo.RootFile]; exists {
		logging.Debug(logging.ComponentQMD, "  ✓ Found via exact match (absolute path): %d hash errors", len(hashErrs))
		rootResult.HashErrors = append(rootResult.HashErrors, hashErrs...)
		rootResult.Compatible = false
		rootResult.Status = StatusFailed
	}

	// Try basename match
	rootBasename := filepath.Base(depInfo.RootFile)
	if hashErrs, exists := parsedOutput.HashErrors[rootBasename]; exists {
		logging.Debug(logging.ComponentQMD, "  ✓ Found via basename match: %d hash errors", len(hashErrs))
		rootResult.HashErrors = append(rootResult.HashErrors, hashErrs...)
		rootResult.Compatible = false
		rootResult.Status = StatusFailed
	}

	// Try suffix match as fallback (only if no errors found yet)
	if len(rootResult.HashErrors) == 0 {
		for errorPath, hashErrs := range parsedOutput.HashErrors {
			if strings.HasSuffix(errorPath, rootBasename) {
				logging.Debug(logging.ComponentQMD, "  ✓ Found via suffix match: '%s' matches '%s', %d hash errors", errorPath, rootBasename, len(hashErrs))
				rootResult.HashErrors = append(rootResult.HashErrors, hashErrs...)
				rootResult.Compatible = false
				rootResult.Status = StatusFailed
				break
			}
		}
	}

	// Process errors - try multiple path variations
	if procErrs, exists := parsedOutput.ProcessErrors[depInfo.RootFile]; exists {
		rootResult.ProcessErrors = append(rootResult.ProcessErrors, procErrs...)
		rootResult.Compatible = false
		rootResult.Status = StatusFailed
	}
	if procErrs, exists := parsedOutput.ProcessErrors[rootBasename]; exists {
		rootResult.ProcessErrors = append(rootResult.ProcessErrors, procErrs...)
		rootResult.Compatible = false
		rootResult.Status = StatusFailed
	}

	// Check if root file is the panic file
	if parsedOutput.PanicFile != "" && parsedOutput.PanicFile == rootBasename {
		rootResult.Status = StatusFailed
		rootResult.Compatible = false
		rootResult.ProcessErrors = append(rootResult.ProcessErrors, "qmldiff panicked: "+parsedOutput.PanicMessage)
		logging.Debug(logging.ComponentQMD, "  Root file is the panic file - marking as failed")
	}

	// Log final root file result
	logging.Debug(logging.ComponentQMD, "  Final root result: Compatible=%v, Status=%s, HashErrors=%d, ProcessErrors=%d",
		rootResult.Compatible, rootResult.Status, len(rootResult.HashErrors), len(rootResult.ProcessErrors))

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
			result.BlockedBy = depInfo.ExpectedLoads[failurePoint]
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

		// Check if this file is the panic file
		expectedFileBase := filepath.Base(expectedFile)
		isPanicFile := parsedOutput.PanicFile != "" && parsedOutput.PanicFile == expectedFileBase

		// Debug logging for panic file matching
		logging.Debug(logging.ComponentQMD, "Dependency check: expectedFile='%s', expectedFileBase='%s', PanicFile='%s', isPanicFile=%v, isFailure=%v, wasProcessed=%v",
			expectedFile, expectedFileBase, parsedOutput.PanicFile, isPanicFile, isFailure, wasProcessed)

		if isFailure {
			failurePoint = i
			result.Status = StatusFailed
			result.Compatible = false
			result.ProcessErrors = append(result.ProcessErrors, "LOAD failed: Cannot read file")
			logging.Debug(logging.ComponentQMD, "File caused failure: %s", expectedFile)
		} else if isPanicFile {
			failurePoint = i
			result.Status = StatusFailed
			result.Compatible = false
			result.ProcessErrors = append(result.ProcessErrors, "qmldiff panicked: "+parsedOutput.PanicMessage)
			logging.Debug(logging.ComponentQMD, "File caused panic: %s", expectedFile)
		} else if wasProcessed {
			// Check for errors - try multiple path variations
			// Try exact match with expected file (relative path)
			if hashErrs, exists := parsedOutput.HashErrors[expectedFile]; exists {
				result.HashErrors = hashErrs
				result.Compatible = false
			}

			// Try exact match with resolved path (absolute path)
			if hashErrs, exists := parsedOutput.HashErrors[resolvedPath]; exists {
				result.HashErrors = append(result.HashErrors, hashErrs...)
				result.Compatible = false
			}

			// If not found yet, try matching by suffix (fallback)
			if len(result.HashErrors) == 0 {
				for errorPath, hashErrs := range parsedOutput.HashErrors {
					// Check if the error path ends with the expected file path
					if strings.HasSuffix(errorPath, expectedFile) {
						result.HashErrors = append(result.HashErrors, hashErrs...)
						result.Compatible = false
						break
					}
				}
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
