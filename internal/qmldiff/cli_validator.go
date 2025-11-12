package qmldiff

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/rmitchellscott/rm-qmd-verify/internal/logging"
	"github.com/rmitchellscott/rm-qmd-verify/internal/qmd"
)

// TreeValidationResult contains the result of validating a QMD against a QML tree
type TreeValidationResult struct {
	// FilesProcessed is the number of QML files processed
	FilesProcessed int
	// FilesModified is the number of files that were modified
	FilesModified int
	// FilesWithErrors is the number of files that had processing errors
	FilesWithErrors int
	// Errors contains any errors encountered during validation
	Errors []TreeValidationError
	// HasHashErrors indicates if there were hash lookup errors
	HasHashErrors bool
	// FailedHashes contains the hash IDs that failed lookup during validation
	FailedHashes []uint64
	// DependencyResults contains per-file validation results including LOADed dependencies
	DependencyResults map[string]*qmd.ValidationResult
}

// TreeValidationError represents an error encountered during tree validation
type TreeValidationError struct {
	FilePath string
	Error    string
	Line     int
	Column   int
}

// BatchTreeValidationResult contains results for multiple QMD files
type BatchTreeValidationResult struct {
	Results map[string]*TreeValidationResult
	Errors  map[string]error
}

// ValidateMultipleQMDsWithCLI validates multiple QMD files by calling the qmldiff CLI binary
// Each QMD file is processed in a separate qmldiff process for isolation
func ValidateMultipleQMDsWithCLI(qmdPaths []string, hashtabPath string, treePath string, qmldiffBinary string) (*BatchTreeValidationResult, error) {
	result := &BatchTreeValidationResult{
		Results: make(map[string]*TreeValidationResult),
		Errors:  make(map[string]error),
	}

	for _, qmdPath := range qmdPaths {
		logging.Info(logging.ComponentQMLDiff, "Validating QMD with dependency tracking: %s", qmdPath)

		// Use dependency-aware validation
		depResults, err := ValidateWithDependencies(qmdPath, hashtabPath, treePath, qmldiffBinary)

		// Flatten dependency results to TreeValidationResult
		treeResult := flattenDependencyResults(depResults, err)

		if err != nil {
			result.Errors[qmdPath] = err
			// treeResult already contains error info from flattening
		}

		result.Results[qmdPath] = treeResult

		logging.Info(logging.ComponentQMLDiff, "CLI validation complete for %s: %d files processed, %d modified, %d errors",
			qmdPath, treeResult.FilesProcessed, treeResult.FilesModified, treeResult.FilesWithErrors)
	}

	return result, nil
}

// ValidateMultipleQMDsWithCLIAndCopy creates a temp directory with tree copy and validates QMDs
// This matches the behavior of the CGO version where we need a mutable tree
func ValidateMultipleQMDsWithCLIAndCopy(qmdPaths []string, hashtabPath string, treePath string, qmldiffBinary string) (*BatchTreeValidationResult, error) {
	result := &BatchTreeValidationResult{
		Results: make(map[string]*TreeValidationResult),
		Errors:  make(map[string]error),
	}

	for _, qmdPath := range qmdPaths {
		logging.Info(logging.ComponentQMLDiff, "Validating QMD via CLI with tree copy: %s", qmdPath)

		// Create temporary directory for tree output
		tempDir, err := os.MkdirTemp("", "qmldiff-tree-*")
		if err != nil {
			result.Errors[qmdPath] = fmt.Errorf("failed to create temp dir: %w", err)
			continue
		}
		defer os.RemoveAll(tempDir)

		// Copy QML tree to temp directory
		treeOutput := filepath.Join(tempDir, "tree")
		if err := copyTree(treePath, treeOutput); err != nil {
			result.Errors[qmdPath] = fmt.Errorf("failed to copy tree: %w", err)
			continue
		}

		// Run qmldiff apply-diffs (it modifies the tree in place)
		cmd := exec.Command(
			qmldiffBinary,
			"apply-diffs",
			"--hashtab", hashtabPath,
			"--collect-hash-errors",
			treeOutput,
			treeOutput, // Same path - modify in place
			qmdPath,
		)

		// Capture combined output
		output, err := cmd.CombinedOutput()
		outputStr := string(output)

		// Parse results
		treeResult := &TreeValidationResult{
			Errors: make([]TreeValidationError, 0),
		}

		if err != nil {
			// Process crashed or exited with error
			exitCode := -1
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			}

			// Exit code 1 with hash errors is expected behavior, not a failure
			if exitCode == 1 && strings.Contains(outputStr, "Hash lookup errors found:") {
				logging.Info(logging.ComponentQMLDiff, "qmldiff found hash errors for %s", qmdPath)
				// Continue to parse hash errors below (don't treat as error)
			} else if strings.Contains(outputStr, "panicked at") || strings.Contains(outputStr, "SIGABRT") {
				logging.Warn(logging.ComponentQMLDiff, "qmldiff panicked for %s: %s", qmdPath, outputStr)
				result.Errors[qmdPath] = fmt.Errorf("qmldiff panicked: %s", extractPanicMessage(outputStr))
				continue
			} else {
				logging.Warn(logging.ComponentQMLDiff, "qmldiff failed for %s (exit code %d): %s", qmdPath, exitCode, outputStr)
				result.Errors[qmdPath] = fmt.Errorf("qmldiff failed (exit %d): %v", exitCode, err)
				continue
			}
		}

		// Success - count modified files by comparing original and output trees
		modifiedCount := 0
		filepath.WalkDir(treeOutput, func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(path, ".qml") {
				// For now, assume all QML files in output are modified
				// Could enhance by comparing with original tree
				modifiedCount++
			}
			return nil
		})

		treeResult.FilesProcessed = modifiedCount
		treeResult.FilesModified = modifiedCount
		treeResult.FilesWithErrors = 0

		// Parse hash errors from qmldiff output
		// Expected format: "Cannot resolve hash <hash_id> required by <filename>"
		hashErrorRegex := regexp.MustCompile(`Cannot resolve hash (\d+) required by (.+)`)
		for _, line := range strings.Split(outputStr, "\n") {
			if matches := hashErrorRegex.FindStringSubmatch(line); len(matches) == 3 {
				hashID, _ := strconv.ParseUint(matches[1], 10, 64)
				treeResult.FailedHashes = append(treeResult.FailedHashes, hashID)
				treeResult.Errors = append(treeResult.Errors, TreeValidationError{
					FilePath: matches[2],
					Error:    fmt.Sprintf("Cannot resolve hash %s", matches[1]),
				})
			}
		}
		if len(treeResult.FailedHashes) > 0 {
			treeResult.HasHashErrors = true
			treeResult.FilesWithErrors++
		}

		result.Results[qmdPath] = treeResult

		logging.Info(logging.ComponentQMLDiff, "CLI validation complete for %s: %d files modified",
			qmdPath, treeResult.FilesModified)
	}

	return result, nil
}

// copyTree recursively copies a directory tree
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if d.IsDir() {
			return os.MkdirAll(dstPath, 0755)
		}

		// Copy file
		return copyFile(path, dstPath)
	})
}

// copyFile copies a single file
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// ValidateWithDependencies validates a single QMD file and tracks all LOAD dependencies
// Returns per-file results including files that were not attempted due to prior failures
func ValidateWithDependencies(qmdPath string, hashtabPath string, treePath string, qmldiffBinary string) (map[string]*qmd.ValidationResult, error) {
	logging.Info(logging.ComponentQMLDiff, "Validating QMD with dependency tracking: %s", qmdPath)

	// Build dependency info by extracting LOAD statements
	depInfo, err := qmd.BuildDependencyInfo(qmdPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build dependency info: %w", err)
	}

	logging.Info(logging.ComponentQMLDiff, "Found %d LOAD statements in %s", len(depInfo.ExpectedLoads), qmdPath)

	// Debug: Log QMD location info
	qmdDir := filepath.Dir(qmdPath)
	logging.Debug(logging.ComponentQMLDiff, "QMD absolute path: %s", qmdPath)
	logging.Debug(logging.ComponentQMLDiff, "QMD directory: %s", qmdDir)

	// Debug: List all QMD files in the temp directory
	logging.Debug(logging.ComponentQMLDiff, "Listing QMD files in temp directory:")
	filepath.Walk(qmdDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".qmd") {
			relPath, _ := filepath.Rel(qmdDir, path)
			logging.Debug(logging.ComponentQMLDiff, "  Found: %s", relPath)
		}
		return nil
	})

	// Create temporary output directory
	outputDir, err := os.MkdirTemp("", "qmldiff-output-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create output dir: %w", err)
	}
	defer os.RemoveAll(outputDir)

	// Run qmldiff with --collect-hash-errors
	cmd := exec.Command(
		qmldiffBinary,
		"apply-diffs",
		"--hashtab", hashtabPath,
		"--collect-hash-errors",
		treePath,
		outputDir,
		qmdPath,
	)

	// Debug: Log command details
	logging.Debug(logging.ComponentQMLDiff, "qmldiff command: %s", strings.Join(cmd.Args, " "))
	if cmd.Dir != "" {
		logging.Debug(logging.ComponentQMLDiff, "qmldiff working directory: %s", cmd.Dir)
	} else {
		cwd, _ := os.Getwd()
		logging.Debug(logging.ComponentQMLDiff, "qmldiff working directory: %s (current process dir)", cwd)
	}

	// Capture combined output (stdout + stderr)
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	// Debug: Log exit status
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			logging.Debug(logging.ComponentQMLDiff, "qmldiff exit code: %d", exitErr.ExitCode())
		} else {
			logging.Debug(logging.ComponentQMLDiff, "qmldiff error: %v", err)
		}
	} else {
		logging.Debug(logging.ComponentQMLDiff, "qmldiff exit code: 0 (success)")
	}

	// Log full output for debugging
	logging.Debug(logging.ComponentQMLDiff, "qmldiff output:\n%s", outputStr)

	// Parse qmldiff output
	parsed := qmd.ParseQmdiffOutput(outputStr)

	// Reconcile expected dependencies with actual results
	results := qmd.ReconcileResults(depInfo, parsed)

	// Log summary
	validated := 0
	failed := 0
	notAttempted := 0
	for _, result := range results {
		switch result.Status {
		case qmd.StatusValidated:
			validated++
		case qmd.StatusFailed:
			failed++
		case qmd.StatusNotAttempted:
			notAttempted++
		}
	}

	logging.Info(logging.ComponentQMLDiff, "Validation complete: %d validated, %d failed, %d not attempted",
		validated, failed, notAttempted)

	return results, nil
}

// flattenDependencyResults converts dependency-aware results into a TreeValidationResult
// This maintains backward compatibility with the existing validation pipeline
func flattenDependencyResults(depResults map[string]*qmd.ValidationResult, validationErr error) *TreeValidationResult {
	result := &TreeValidationResult{
		Errors:            make([]TreeValidationError, 0),
		FailedHashes:      make([]uint64, 0),
		DependencyResults: depResults, // Store original dependency results
	}

	logging.Info(logging.ComponentQMLDiff, "Created TreeValidationResult with %d dependency entries", len(depResults))

	if validationErr != nil {
		result.FilesWithErrors = 1
		result.Errors = append(result.Errors, TreeValidationError{
			FilePath: "validation",
			Error:    validationErr.Error(),
		})
		return result
	}

	// Aggregate results from all dependency files
	filesProcessed := 0
	filesModified := 0

	for filePath, fileResult := range depResults {
		if fileResult.Status == qmd.StatusValidated || fileResult.Status == qmd.StatusFailed {
			filesProcessed++
		}

		// If file was validated (successfully or with errors), count it as potentially modified
		if fileResult.Status != qmd.StatusNotAttempted {
			filesModified++
		}

		// Aggregate hash errors - only from root file
		// Dependency hash errors stay in DependencyResults and are handled by api.go flattening
		if fileResult.Position == -1 {
			for _, hashErr := range fileResult.HashErrors {
				result.FailedHashes = append(result.FailedHashes, hashErr.HashID)
				result.Errors = append(result.Errors, TreeValidationError{
					FilePath: filePath,
					Error:    hashErr.Error,
				})
			}
		}

		// Aggregate process errors
		for _, procErr := range fileResult.ProcessErrors {
			result.Errors = append(result.Errors, TreeValidationError{
				FilePath: filePath,
				Error:    procErr,
			})
		}

		// If file is not compatible, increment error count
		if !fileResult.Compatible {
			result.FilesWithErrors++
		}
	}

	result.FilesProcessed = filesProcessed
	result.FilesModified = filesModified

	// HasHashErrors should be true if ANY file (root or dependency) has hash errors
	result.HasHashErrors = len(result.FailedHashes) > 0
	if !result.HasHashErrors {
		for _, fileResult := range depResults {
			if len(fileResult.HashErrors) > 0 {
				result.HasHashErrors = true
				break
			}
		}
	}

	return result
}

// extractPanicMessage extracts the panic message from qmldiff output
func extractPanicMessage(output string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "panicked at") {
			return strings.TrimSpace(line)
		}
	}
	return "unknown panic"
}

// countFilesInOutput counts files mentioned in qmldiff output
func countFilesInOutput(output string) int {
	// Look for "Reading diff" or "Processing file" messages
	count := 0
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Reading diff") || strings.Contains(line, "Processing file") {
			count++
		}
	}
	return count
}

// countModifiedFiles counts modified files in qmldiff output
func countModifiedFiles(output string) int {
	// For now, same as files processed
	// Could parse more detailed output if available
	return countFilesInOutput(output)
}
