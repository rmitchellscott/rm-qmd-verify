package qmldiff

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
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
// Uses two-phase validation: check-compatibility for hashes, then apply-diffs for structure
func ValidateMultipleQMDsWithCLIAndCopy(qmdPaths []string, hashtabPath string, treePath string, qmldiffBinary string) (*BatchTreeValidationResult, error) {
	result := &BatchTreeValidationResult{
		Results: make(map[string]*TreeValidationResult),
		Errors:  make(map[string]error),
	}

	for _, qmdPath := range qmdPaths {
		logging.Info(logging.ComponentQMLDiff, "Validating QMD via CLI with tree copy: %s", qmdPath)

		treeResult := &TreeValidationResult{
			Errors: make([]TreeValidationError, 0),
		}

		// Phase 1: Check hash compatibility
		compatResult, err := CheckCompatibility([]string{qmdPath}, hashtabPath, qmldiffBinary)
		if err != nil {
			result.Errors[qmdPath] = fmt.Errorf("check-compatibility failed: %w", err)
			continue
		}

		if compatResult.HasErrors {
			// Hash errors found - record them and continue to next file
			logging.Info(logging.ComponentQMLDiff, "check-compatibility found %d hash errors for %s", compatResult.TotalErrors, qmdPath)
			treeResult.HasHashErrors = true
			treeResult.FilesWithErrors = 1
			for filePath, hashIDs := range compatResult.HashErrors {
				for _, hashID := range hashIDs {
					treeResult.FailedHashes = append(treeResult.FailedHashes, hashID)
					treeResult.Errors = append(treeResult.Errors, TreeValidationError{
						FilePath: filePath,
						Error:    fmt.Sprintf("Cannot resolve hash %d", hashID),
					})
				}
			}
			result.Results[qmdPath] = treeResult
			continue
		}

		// Phase 2: Run apply-diffs for structural validation
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
			treeOutput,
			treeOutput, // Same path - modify in place
			qmdPath,
		)

		output, err := cmd.CombinedOutput()
		outputStr := string(output)

		if err != nil {
			exitCode := -1
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			}

			if strings.Contains(outputStr, "panicked at") || strings.Contains(outputStr, "SIGABRT") {
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

// CheckCompatibility runs qmldiff check-compatibility to validate hash compatibility
// This is Phase 1 of two-phase validation - checks that all hashes exist in the hashtab
func CheckCompatibility(qmdPaths []string, hashtabPath string, qmldiffBinary string) (*qmd.CheckCompatibilityResult, error) {
	args := []string{"check-compatibility", hashtabPath}
	args = append(args, qmdPaths...)

	cmd := exec.Command(qmldiffBinary, args...)
	logging.Debug(logging.ComponentQMLDiff, "check-compatibility command: %s", strings.Join(cmd.Args, " "))

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	logging.Debug(logging.ComponentQMLDiff, "check-compatibility output:\n%s", outputStr)

	result := qmd.ParseCheckCompatibilityOutput(outputStr)

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode := exitErr.ExitCode()
			logging.Debug(logging.ComponentQMLDiff, "check-compatibility exit code: %d", exitCode)
			if exitCode == 1 {
				// Exit 1 = hash errors found (expected behavior)
				result.HasErrors = true
			} else {
				// Other error
				return nil, fmt.Errorf("check-compatibility failed (exit %d): %w", exitCode, err)
			}
		} else {
			return nil, fmt.Errorf("check-compatibility failed: %w", err)
		}
	} else {
		logging.Debug(logging.ComponentQMLDiff, "check-compatibility exit code: 0 (no errors)")
	}

	return result, nil
}

// reconcileHashErrors converts CheckCompatibilityResult to ValidationResult map
func reconcileHashErrors(depInfo *qmd.DependencyInfo, compatResult *qmd.CheckCompatibilityResult) map[string]*qmd.ValidationResult {
	results := make(map[string]*qmd.ValidationResult)

	rootFileName := filepath.Base(depInfo.RootFile)
	rootResult := &qmd.ValidationResult{
		Path:       rootFileName,
		Status:     qmd.StatusValidated,
		Compatible: true,
		Position:   -1,
	}

	// Check if root file has hash errors
	for filePath, hashIDs := range compatResult.HashErrors {
		fileBase := filepath.Base(filePath)
		if fileBase == rootFileName || strings.HasSuffix(filePath, rootFileName) {
			rootResult.Status = qmd.StatusFailed
			rootResult.Compatible = false
			for _, hashID := range hashIDs {
				rootResult.HashErrors = append(rootResult.HashErrors, qmd.HashError{
					HashID: hashID,
					Error:  fmt.Sprintf("Cannot resolve hash %d", hashID),
				})
			}
		}
	}
	results[rootFileName] = rootResult

	// Process expected LOAD dependencies
	for i, expectedFile := range depInfo.ExpectedLoads {
		result := &qmd.ValidationResult{
			Path:       expectedFile,
			Position:   i,
			Status:     qmd.StatusValidated,
			Compatible: true,
		}

		// Check if this file has hash errors
		expectedFileBase := filepath.Base(expectedFile)
		for filePath, hashIDs := range compatResult.HashErrors {
			fileBase := filepath.Base(filePath)
			if fileBase == expectedFileBase || strings.HasSuffix(filePath, expectedFile) {
				result.Status = qmd.StatusFailed
				result.Compatible = false
				for _, hashID := range hashIDs {
					result.HashErrors = append(result.HashErrors, qmd.HashError{
						HashID: hashID,
						Error:  fmt.Sprintf("Cannot resolve hash %d", hashID),
					})
				}
				break
			}
		}

		results[expectedFile] = result
	}

	return results
}

// ValidateWithDependencies validates a single QMD file using two-phase approach:
// Phase 1: check-compatibility for hash validation
// Phase 2: apply-diffs for structural validation (only if Phase 1 passes)
func ValidateWithDependencies(qmdPath string, hashtabPath string, treePath string, qmldiffBinary string) (map[string]*qmd.ValidationResult, error) {
	logging.Info(logging.ComponentQMLDiff, "Validating QMD with dependency tracking: %s", qmdPath)

	// Build dependency info for UI reporting
	depInfo, err := qmd.BuildDependencyInfo(qmdPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build dependency info: %w", err)
	}

	logging.Info(logging.ComponentQMLDiff, "Found %d LOAD statements in %s", len(depInfo.ExpectedLoads), qmdPath)

	qmdDir := filepath.Dir(qmdPath)
	logging.Debug(logging.ComponentQMLDiff, "QMD absolute path: %s", qmdPath)
	logging.Debug(logging.ComponentQMLDiff, "QMD directory: %s", qmdDir)

	// Phase 1: Check hash compatibility
	logging.Info(logging.ComponentQMLDiff, "Phase 1: Running check-compatibility")
	compatResult, err := CheckCompatibility([]string{qmdPath}, hashtabPath, qmldiffBinary)
	if err != nil {
		return nil, fmt.Errorf("check-compatibility failed: %w", err)
	}

	if compatResult.HasErrors {
		// Hash errors found - return them without running apply-diffs
		logging.Info(logging.ComponentQMLDiff, "Phase 1 failed: %d hash errors found", compatResult.TotalErrors)
		return reconcileHashErrors(depInfo, compatResult), nil
	}

	logging.Info(logging.ComponentQMLDiff, "Phase 1 passed: No hash errors")

	// Phase 2: Run apply-diffs for structural validation
	logging.Info(logging.ComponentQMLDiff, "Phase 2: Running apply-diffs for structural validation")

	outputDir, err := os.MkdirTemp("", "qmldiff-output-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create output dir: %w", err)
	}
	defer os.RemoveAll(outputDir)

	cmd := exec.Command(
		qmldiffBinary,
		"apply-diffs",
		"--hashtab", hashtabPath,
		treePath,
		outputDir,
		qmdPath,
	)

	logging.Debug(logging.ComponentQMLDiff, "apply-diffs command: %s", strings.Join(cmd.Args, " "))

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	logging.Debug(logging.ComponentQMLDiff, "apply-diffs output:\n%s", outputStr)

	parsed := qmd.ParseApplyDiffsOutput(outputStr)

	if err != nil {
		exitCode := -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			logging.Debug(logging.ComponentQMLDiff, "apply-diffs exit code: %d", exitCode)
		} else {
			logging.Debug(logging.ComponentQMLDiff, "apply-diffs error: %v", err)
		}

		if parsed.HadPanic {
			panicMsg := extractPanicMessage(outputStr)
			logging.Warn(logging.ComponentQMLDiff, "apply-diffs panicked: %s", panicMsg)
			return createErrorResults(depInfo, fmt.Sprintf("qmldiff panicked: %s", panicMsg)), fmt.Errorf("qmldiff panicked")
		} else if exitCode > 0 {
			logging.Warn(logging.ComponentQMLDiff, "apply-diffs failed (exit %d), attempting to use partial results", exitCode)
		}
	} else {
		logging.Debug(logging.ComponentQMLDiff, "apply-diffs exit code: 0 (success)")
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
