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

		depResults, err := ValidateWithDependencies(qmdPath, hashtabPath, treePath, qmldiffBinary)
		treeResult := flattenDependencyResults(depResults, err)

		if err != nil {
			result.Errors[qmdPath] = err
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

		tempDir, err := os.MkdirTemp("", "qmldiff-tree-*")
		if err != nil {
			result.Errors[qmdPath] = fmt.Errorf("failed to create temp dir: %w", err)
			continue
		}
		defer os.RemoveAll(tempDir)

		treeOutput := filepath.Join(tempDir, "tree")
		if err := copyTree(treePath, treeOutput); err != nil {
			result.Errors[qmdPath] = fmt.Errorf("failed to copy tree: %w", err)
			continue
		}

		cmd := exec.Command(
			qmldiffBinary,
			"apply-diffs",
			"--hashtab", hashtabPath,
			"--collect-hash-errors",
			treeOutput,
			treeOutput, // Same path - modify in place
			qmdPath,
		)

		output, err := cmd.CombinedOutput()
		outputStr := string(output)

		treeResult := &TreeValidationResult{
			Errors: make([]TreeValidationError, 0),
		}

		if err != nil {
			exitCode := -1
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			}

			if exitCode == 1 && strings.Contains(outputStr, "Hash lookup errors found:") {
				logging.Info(logging.ComponentQMLDiff, "qmldiff found hash errors for %s", qmdPath)
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

		modifiedCount := 0
		filepath.WalkDir(treeOutput, func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(path, ".qml") {
				modifiedCount++
			}
			return nil
		})

		treeResult.FilesProcessed = modifiedCount
		treeResult.FilesModified = modifiedCount
		treeResult.FilesWithErrors = 0

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

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if d.IsDir() {
			return os.MkdirAll(dstPath, 0755)
		}

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

	depInfo, err := qmd.BuildDependencyInfo(qmdPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build dependency info: %w", err)
	}

	logging.Info(logging.ComponentQMLDiff, "Found %d LOAD statements in %s", len(depInfo.ExpectedLoads), qmdPath)

	qmdDir := filepath.Dir(qmdPath)
	logging.Debug(logging.ComponentQMLDiff, "QMD absolute path: %s", qmdPath)
	logging.Debug(logging.ComponentQMLDiff, "QMD directory: %s", qmdDir)

	logging.Debug(logging.ComponentQMLDiff, "Listing QMD files in temp directory:")
	filepath.Walk(qmdDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".qmd") {
			relPath, _ := filepath.Rel(qmdDir, path)
			logging.Debug(logging.ComponentQMLDiff, "  Found: %s", relPath)
		}
		return nil
	})

	outputDir, err := os.MkdirTemp("", "qmldiff-output-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create output dir: %w", err)
	}
	defer os.RemoveAll(outputDir)

	cmd := exec.Command(
		qmldiffBinary,
		"apply-diffs",
		"--hashtab", hashtabPath,
		"--collect-hash-errors",
		treePath,
		outputDir,
		qmdPath,
	)

	logging.Debug(logging.ComponentQMLDiff, "qmldiff command: %s", strings.Join(cmd.Args, " "))
	if cmd.Dir != "" {
		logging.Debug(logging.ComponentQMLDiff, "qmldiff working directory: %s", cmd.Dir)
	} else {
		cwd, _ := os.Getwd()
		logging.Debug(logging.ComponentQMLDiff, "qmldiff working directory: %s (current process dir)", cwd)
	}

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	logging.Debug(logging.ComponentQMLDiff, "qmldiff output:\n%s", outputStr)

	parsed := qmd.ParseQmdiffOutput(outputStr)

	if err != nil {
		exitCode := -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			logging.Debug(logging.ComponentQMLDiff, "qmldiff exit code: %d", exitCode)
		} else {
			logging.Debug(logging.ComponentQMLDiff, "qmldiff error: %v", err)
		}

		if exitCode == 1 && strings.Contains(outputStr, "Hash lookup errors found:") {
			logging.Debug(logging.ComponentQMLDiff, "qmldiff found hash errors (exit 1)")
		} else if parsed.HadPanic && len(parsed.HashErrors) == 0 {
			panicMsg := extractPanicMessage(outputStr)
			logging.Warn(logging.ComponentQMLDiff, "qmldiff panicked: %s", panicMsg)
			return createErrorResults(depInfo, fmt.Sprintf("qmldiff panicked: %s", panicMsg)), fmt.Errorf("qmldiff panicked")
		} else if exitCode > 1 && !parsed.HadPanic {
			logging.Warn(logging.ComponentQMLDiff, "qmldiff failed (exit %d), attempting to use partial results", exitCode)
		}
	} else {
		logging.Debug(logging.ComponentQMLDiff, "qmldiff exit code: 0 (success)")
	}

	results := qmd.ReconcileResults(depInfo, parsed)

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
		DependencyResults: depResults,
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

	filesProcessed := 0
	filesModified := 0

	for filePath, fileResult := range depResults {
		if fileResult.Status == qmd.StatusValidated || fileResult.Status == qmd.StatusFailed {
			filesProcessed++
		}

		if fileResult.Status != qmd.StatusNotAttempted {
			filesModified++
		}

		if fileResult.Position == -1 {
			for _, hashErr := range fileResult.HashErrors {
				result.FailedHashes = append(result.FailedHashes, hashErr.HashID)
				result.Errors = append(result.Errors, TreeValidationError{
					FilePath: filePath,
					Error:    hashErr.Error,
				})
			}
		}

		for _, procErr := range fileResult.ProcessErrors {
			result.Errors = append(result.Errors, TreeValidationError{
				FilePath: filePath,
				Error:    procErr,
			})
		}

		if !fileResult.Compatible {
			result.FilesWithErrors++
		}
	}

	result.FilesProcessed = filesProcessed
	result.FilesModified = filesModified

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

// createErrorResults creates ValidationResults for all files when qmldiff fails
// Marks root file as failed with error message, and all dependencies as not attempted
func createErrorResults(depInfo *qmd.DependencyInfo, errorMsg string) map[string]*qmd.ValidationResult {
	results := make(map[string]*qmd.ValidationResult)

	rootFile := filepath.Base(depInfo.RootFile)
	results[rootFile] = &qmd.ValidationResult{
		Path:          rootFile,
		Status:        qmd.StatusFailed,
		Compatible:    false,
		ProcessErrors: []string{errorMsg},
		Position:      -1,
	}

	for i, depPath := range depInfo.ExpectedLoads {
		results[depPath] = &qmd.ValidationResult{
			Path:          depPath,
			Status:        qmd.StatusNotAttempted,
			Compatible:    false,
			ProcessErrors: []string{"not attempted due to prior failure"},
			Position:      i,
		}
	}

	return results
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
