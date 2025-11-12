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

	hashErrorRegex := regexp.MustCompile(`(?:(.+\.qmd) - Cannot resolve hash (\d+)|Cannot resolve hash (\d+) required by (.+\.qmd))`)
	processErrorRegex := regexp.MustCompile(`\(On behalf of '(.+\.qmd)'\): (.+)`)
	writtenFileRegex := regexp.MustCompile(`Written file (.+\.qml) - (\d+) diff\(s\) applied`)
	fileNotFoundRegex := regexp.MustCompile(`Cannot read file (.+\.qmd)`)
	panicRegex := regexp.MustCompile(`panicked at`)

	lines := strings.Split(output, "\n")

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

		if panicLineIdx >= 0 && panicLineIdx < len(lines)-1 {
			requiredByRegex := regexp.MustCompile(`required by (.+\.qmd)`)
			for i := panicLineIdx + 1; i < len(lines) && i < panicLineIdx+5; i++ {
				if matches := requiredByRegex.FindStringSubmatch(lines[i]); len(matches) == 2 {
					result.PanicFile = filepath.Base(matches[1])
					logging.Debug(logging.ComponentQMD, "Panic occurred while processing file: %s", result.PanicFile)
					break
				}
			}
		}

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

		if matches := hashErrorRegex.FindStringSubmatch(line); len(matches) == 5 {
			var hashID uint64
			var qmdFile string

			if matches[1] != "" && matches[2] != "" {
				qmdFile = matches[1]
				hashID, _ = strconv.ParseUint(matches[2], 10, 64)
			} else if matches[3] != "" && matches[4] != "" {
				hashID, _ = strconv.ParseUint(matches[3], 10, 64)
				qmdFile = matches[4]
			} else {
				continue
			}

			result.HashErrors[qmdFile] = append(result.HashErrors[qmdFile], HashError{
				HashID: hashID,
				Error:  line,
			})
			result.ProcessedFiles[qmdFile] = true

			logging.Debug(logging.ComponentQMD, "Parsed hash error from %s: hash %d", qmdFile, hashID)
		}

		if matches := processErrorRegex.FindStringSubmatch(line); len(matches) == 3 {
			qmdFile := matches[1]
			errorMsg := matches[2]

			result.ProcessErrors[qmdFile] = append(result.ProcessErrors[qmdFile], errorMsg)
			result.ProcessedFiles[qmdFile] = true

			logging.Debug(logging.ComponentQMD, "Parsed process error from %s: %s", qmdFile, errorMsg)
		}

		if matches := fileNotFoundRegex.FindStringSubmatch(line); len(matches) == 2 {
			qmdFile := matches[1]
			if result.FailureFile == "" {
				result.FailureFile = qmdFile
			}

			logging.Debug(logging.ComponentQMD, "Detected LOAD failure at: %s", qmdFile)
		}

		if matches := writtenFileRegex.FindStringSubmatch(line); len(matches) == 3 {
			qmlFile := matches[1]
			logging.Debug(logging.ComponentQMD, "QML file modified: %s", qmlFile)
		}
	}

	return result
}

// ReconcileResults combines expected dependencies with actual results
func ReconcileResults(depInfo *DependencyInfo, parsedOutput *ParsedOutput) map[string]*ValidationResult {
	results := make(map[string]*ValidationResult)

	rootResult := &ValidationResult{
		Path:       filepath.Base(depInfo.RootFile),
		Status:     StatusValidated,
		Compatible: true,
		Position:   -1,
	}

	if parsedOutput.HadPanic {
		hasHashErrors := len(parsedOutput.HashErrors) > 0

		if !hasHashErrors && parsedOutput.PanicFile == "" {
			rootResult.Status = StatusFailed
			rootResult.Compatible = false
			rootResult.ProcessErrors = append(rootResult.ProcessErrors, "qmldiff panicked: "+parsedOutput.PanicMessage)
			rootFileName := filepath.Base(depInfo.RootFile)
			results[rootFileName] = rootResult

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

		if parsedOutput.PanicFile != "" {
			logging.Debug(logging.ComponentQMD, "Panic occurred in file %s, will mark it as failed", parsedOutput.PanicFile)
		} else {
			logging.Debug(logging.ComponentQMD, "Panic occurred but hash errors were collected, continuing with normal processing")
		}
	}

	logging.Debug(logging.ComponentQMD, "=== Root File Path Matching ===")
	logging.Debug(logging.ComponentQMD, "  depInfo.RootFile (looking for): '%s'", depInfo.RootFile)
	logging.Debug(logging.ComponentQMD, "  depInfo.RootFile basename: '%s'", filepath.Base(depInfo.RootFile))

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

	if hashErrs, exists := parsedOutput.HashErrors[depInfo.RootFile]; exists {
		logging.Debug(logging.ComponentQMD, "  ✓ Found via exact match (absolute path): %d hash errors", len(hashErrs))
		rootResult.HashErrors = append(rootResult.HashErrors, hashErrs...)
		rootResult.Compatible = false
		rootResult.Status = StatusFailed
	}

	rootBasename := filepath.Base(depInfo.RootFile)
	if hashErrs, exists := parsedOutput.HashErrors[rootBasename]; exists {
		logging.Debug(logging.ComponentQMD, "  ✓ Found via basename match: %d hash errors", len(hashErrs))
		rootResult.HashErrors = append(rootResult.HashErrors, hashErrs...)
		rootResult.Compatible = false
		rootResult.Status = StatusFailed
	}

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

	if parsedOutput.PanicFile != "" && parsedOutput.PanicFile == rootBasename {
		rootResult.Status = StatusFailed
		rootResult.Compatible = false
		rootResult.ProcessErrors = append(rootResult.ProcessErrors, "qmldiff panicked: "+parsedOutput.PanicMessage)
		logging.Debug(logging.ComponentQMD, "  Root file is the panic file - marking as failed")
	}

	logging.Debug(logging.ComponentQMD, "  Final root result: Compatible=%v, Status=%s, HashErrors=%d, ProcessErrors=%d",
		rootResult.Compatible, rootResult.Status, len(rootResult.HashErrors), len(rootResult.ProcessErrors))

	rootFileName := filepath.Base(depInfo.RootFile)
	results[rootFileName] = rootResult

	failurePoint := -1

	for i, expectedFile := range depInfo.ExpectedLoads {
		resolvedPath := ResolveLoadPath(depInfo.RootFile, expectedFile)

		result := &ValidationResult{
			Path:       expectedFile, // Keep original path from LOAD statement
			Position:   i,
			Compatible: true,
			Status:     StatusValidated,
		}

		if failurePoint != -1 && i > failurePoint {
			result.Status = StatusNotAttempted
			result.Compatible = false
			result.BlockedBy = depInfo.ExpectedLoads[failurePoint]
			results[expectedFile] = result
			logging.Debug(logging.ComponentQMD, "File not attempted: %s (stopped at position %d)", expectedFile, failurePoint)
			continue
		}

		wasProcessed := parsedOutput.ProcessedFiles[expectedFile] ||
			parsedOutput.ProcessedFiles[resolvedPath]

		isFailure := parsedOutput.FailureFile == expectedFile ||
			parsedOutput.FailureFile == resolvedPath

		expectedFileBase := filepath.Base(expectedFile)
		isPanicFile := parsedOutput.PanicFile != "" && parsedOutput.PanicFile == expectedFileBase

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
			if hashErrs, exists := parsedOutput.HashErrors[expectedFile]; exists {
				result.HashErrors = hashErrs
				result.Compatible = false
			}

			if hashErrs, exists := parsedOutput.HashErrors[resolvedPath]; exists {
				result.HashErrors = append(result.HashErrors, hashErrs...)
				result.Compatible = false
			}

			if len(result.HashErrors) == 0 {
				for errorPath, hashErrs := range parsedOutput.HashErrors {
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
