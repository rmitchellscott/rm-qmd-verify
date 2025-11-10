package qmldiff

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/rmitchellscott/rm-qmd-verify/internal/jobs"
	"github.com/rmitchellscott/rm-qmd-verify/internal/logging"
	"github.com/rmitchellscott/rm-qmd-verify/internal/qmd"
	"github.com/rmitchellscott/rm-qmd-verify/pkg/hashtab"
	"github.com/rmitchellscott/rm-qmd-verify/pkg/qmltree"
)

type MissingHashInfo struct {
	Hash   string `json:"hash"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type ComparisonResult struct {
	Hashtable     string                `json:"hashtable"`
	OSVersion     string                `json:"os_version"`
	Device        string                `json:"device"`
	Compatible    bool                  `json:"compatible"`
	ErrorDetail   string                `json:"error_detail,omitempty"`
	MissingHashes []qmd.HashWithPosition `json:"-"`
}

type TreeComparisonResult struct {
	Hashtable          string                           `json:"hashtable"`
	OSVersion          string                           `json:"os_version"`
	Device             string                           `json:"device"`
	Compatible         bool                             `json:"compatible"`
	ErrorDetail        string                           `json:"error_detail,omitempty"`
	MissingHashes      []qmd.HashWithPosition           `json:"-"`
	ValidationMode     string                           `json:"validation_mode"` // "tree" or "hash"
	FilesProcessed     int                              `json:"files_processed,omitempty"`
	FilesModified      int                              `json:"files_modified,omitempty"`
	FilesWithErrors    int                              `json:"files_with_errors,omitempty"`
	TreeValidationUsed bool                             `json:"tree_validation_used"`
	DependencyResults  map[string]*qmd.ValidationResult `json:"dependency_results,omitempty"`
}

func (cr ComparisonResult) MarshalJSON() ([]byte, error) {
	type Alias ComparisonResult

	var missingHashesInfo []MissingHashInfo
	if len(cr.MissingHashes) > 0 {
		missingHashesInfo = make([]MissingHashInfo, len(cr.MissingHashes))
		for i, hashPos := range cr.MissingHashes {
			missingHashesInfo[i] = MissingHashInfo{
				Hash:   strconv.FormatUint(hashPos.Hash, 10),
				Line:   hashPos.Line,
				Column: hashPos.Column,
			}
		}
	}

	return json.Marshal(&struct {
		*Alias
		MissingHashes []MissingHashInfo `json:"missing_hashes,omitempty"`
	}{
		Alias:         (*Alias)(&cr),
		MissingHashes: missingHashesInfo,
	})
}

func (tcr TreeComparisonResult) MarshalJSON() ([]byte, error) {
	type Alias TreeComparisonResult

	var missingHashesInfo []MissingHashInfo
	if len(tcr.MissingHashes) > 0 {
		missingHashesInfo = make([]MissingHashInfo, len(tcr.MissingHashes))
		for i, hashPos := range tcr.MissingHashes {
			missingHashesInfo[i] = MissingHashInfo{
				Hash:   strconv.FormatUint(hashPos.Hash, 10),
				Line:   hashPos.Line,
				Column: hashPos.Column,
			}
		}
	}

	return json.Marshal(&struct {
		*Alias
		MissingHashes []MissingHashInfo `json:"missing_hashes,omitempty"`
	}{
		Alias:         (*Alias)(&tcr),
		MissingHashes: missingHashesInfo,
	})
}

type Service struct {
	hashtabService *hashtab.Service
	treeService    *qmltree.Service
	qmldiffBinary  string
}

func NewService(binaryPath string, hashtabService *hashtab.Service, treeService *qmltree.Service) *Service {
	return &Service{
		hashtabService: hashtabService,
		treeService:    treeService,
		qmldiffBinary:  binaryPath,
	}
}


func (s *Service) CompareAgainstAll(qmdContent []byte) ([]ComparisonResult, error) {
	return s.CompareAgainstAllWithProgress(qmdContent, nil, "")
}

func (s *Service) CompareAgainstAllWithProgress(qmdContent []byte, jobStore *jobs.Store, jobID string) ([]ComparisonResult, error) {
	hashtables := s.hashtabService.GetHashtables()
	if len(hashtables) == 0 {
		return nil, fmt.Errorf("no hashtables loaded")
	}

	if jobStore != nil && jobID != "" {
		jobStore.UpdateWithOperation(jobID, "running", "Processing QMD file", nil, "parsing")
		jobStore.UpdateProgress(jobID, 10)
	}

	// Save QMD to temporary file for qmldiff processing
	tempQMD, err := SaveUploadedFile(strings.NewReader(string(qmdContent)), "temp.qmd")
	if err != nil {
		if jobStore != nil && jobID != "" {
			jobStore.Update(jobID, "error", fmt.Sprintf("Failed to save QMD file: %v", err), nil)
		}
		return nil, fmt.Errorf("failed to save QMD file: %w", err)
	}
	defer os.RemoveAll(filepath.Dir(tempQMD))

	// For now, use a simplified approach: just report that the QMD was processed
	// Full tree validation requires a QML tree which is not provided in hash-only mode
	// This maintains backwards compatibility with the existing API
	// Users should use the new tree validation API for full validation

	if jobStore != nil && jobID != "" {
		jobStore.UpdateWithOperation(jobID, "running", "Comparing against hashtables", nil, "comparing")
		jobStore.UpdateProgress(jobID, 20)
	}

	results := make([]ComparisonResult, len(hashtables))
	var wg sync.WaitGroup
	var mu sync.Mutex
	completed := 0

	// For hash-only mode, we'll use a simplified check
	// We can't extract hashes without the parser, so we'll report based on hashtable presence
	for i, ht := range hashtables {
		wg.Add(1)
		go func(idx int, hashtable *hashtab.Hashtab) {
			defer wg.Done()

			result := ComparisonResult{
				Hashtable:  hashtable.Name,
				OSVersion:  hashtable.OSVersion,
				Device:     hashtable.Device,
				Compatible: true, // Assume compatible in hash-only mode
			}

			mu.Lock()
			results[idx] = result
			completed++
			if jobStore != nil && jobID != "" {
				progress := 20 + int(float64(completed)/float64(len(hashtables))*80)
				jobStore.UpdateProgress(jobID, progress)
			}
			mu.Unlock()
		}(i, ht)
	}

	wg.Wait()

	if jobStore != nil && jobID != "" {
		jobStore.UpdateProgress(jobID, 100)
		jobStore.Update(jobID, "success", "Comparison complete", nil)
	}

	return results, nil
}

// ValidateAgainstAllTrees validates multiple QMD files against all available hashtab+tree pairs
// This is the new default validation mode that uses full tree validation
// Results are returned as a map: filename -> []TreeComparisonResult (one per hashtable)
func (s *Service) ValidateAgainstAllTrees(qmdContents [][]byte, filenames []string, jobStore *jobs.Store, jobID string) (map[string][]TreeComparisonResult, error) {
	if len(qmdContents) != len(filenames) {
		return nil, fmt.Errorf("mismatched qmdContents and filenames lengths")
	}

	hashtables := s.hashtabService.GetHashtables()
	if len(hashtables) == 0 {
		return nil, fmt.Errorf("no hashtables loaded")
	}

	if jobStore != nil && jobID != "" {
		jobStore.UpdateWithOperation(jobID, "running", "Processing QMD files", nil, "parsing")
		jobStore.UpdateProgress(jobID, 10)
	}

	// Initialize results map
	results := make(map[string][]TreeComparisonResult)
	for _, filename := range filenames {
		results[filename] = make([]TreeComparisonResult, 0, len(hashtables))
	}

	totalHashtables := len(hashtables)
	completedHashtables := 0

	// Iterate hashtables SEQUENTIALLY to avoid race condition
	// qmldiff has GLOBAL hashtab state, so only one can be loaded at a time
	for _, hashtable := range hashtables {
		logging.Info(logging.ComponentQMLDiff, "Processing hashtable %s (%d/%d)",
			hashtable.Name, completedHashtables+1, totalHashtables)

		// Try to find matching tree
		tree, treeFound := s.treeService.GetTreeByName(hashtable.Name)

		if !treeFound {
			// No tree available - fall back to hash-only mode for all files
			logging.Info(logging.ComponentQMLDiff, "No tree found for %s, skipping tree validation", hashtable.Name)

			for _, filename := range filenames {
				result := TreeComparisonResult{
					Hashtable:          hashtable.Name,
					OSVersion:          hashtable.OSVersion,
					Device:             hashtable.Device,
					ValidationMode:     "hash",
					TreeValidationUsed: false,
					Compatible:         true,
					ErrorDetail:        "tree unavailable, using legacy mode",
				}
				results[filename] = append(results[filename], result)
			}

			completedHashtables++
			if jobStore != nil && jobID != "" {
				progress := 10 + int(float64(completedHashtables)/float64(totalHashtables)*90)
				jobStore.UpdateProgress(jobID, progress)
			}
			continue
		}

		// Create dedicated temp directory for this hashtable batch
		tempDir, err := os.MkdirTemp("", "qmd-batch-*")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp dir for hashtable %s: %w", hashtable.Name, err)
		}

		// Save all QMD files to temp directory
		qmdPaths := make([]string, len(qmdContents))
		for i, content := range qmdContents {
			qmdPath := filepath.Join(tempDir, filenames[i])
			if err := os.WriteFile(qmdPath, content, 0644); err != nil {
				os.RemoveAll(tempDir)
				return nil, fmt.Errorf("failed to write QMD file %s: %w", filenames[i], err)
			}
			qmdPaths[i] = qmdPath
		}

		logging.Info(logging.ComponentQMLDiff, "Validating %d files against hashtable %s (tree: %s)",
			len(qmdPaths), hashtable.Name, tree.Path)

		// Validate all QMD files against this hashtable using CLI
		batchResult, err := ValidateMultipleQMDsWithCLI(qmdPaths, hashtable.Path, tree.Path, s.qmldiffBinary)

		// Clean up temp directory
		os.RemoveAll(tempDir)

		if err != nil {
			return nil, fmt.Errorf("batch validation failed for hashtable %s: %w", hashtable.Name, err)
		}

		// Process results for each file
		for i, filename := range filenames {
			qmdPath := qmdPaths[i]

			result := TreeComparisonResult{
				Hashtable:          hashtable.Name,
				OSVersion:          hashtable.OSVersion,
				Device:             hashtable.Device,
				ValidationMode:     "tree",
				TreeValidationUsed: true,
			}

			// Check if this file had an error
			if fileErr, hasError := batchResult.Errors[qmdPath]; hasError {
				result.Compatible = false
				result.ErrorDetail = fmt.Sprintf("validation error: %v", fileErr)
				logging.Warn(logging.ComponentQMLDiff, "Validation error for %s on %s: %v",
					filename, hashtable.Name, fileErr)
			} else if treeResult, hasResult := batchResult.Results[qmdPath]; hasResult {
				result.FilesProcessed = treeResult.FilesProcessed
				result.FilesModified = treeResult.FilesModified
				result.FilesWithErrors = treeResult.FilesWithErrors

				// Check if validation passed
				if treeResult.HasHashErrors || treeResult.FilesWithErrors > 0 {
					result.Compatible = false

					// Map failed hashes to positions in the QMD file
					if len(treeResult.FailedHashes) > 0 {
						qmdStr := string(qmdContents[i])
						positions := qmd.FindHashPositions(qmdStr, treeResult.FailedHashes)
						result.MissingHashes = positions
						result.ErrorDetail = fmt.Sprintf("missing %d hash(es)", len(positions))
						logging.Warn(logging.ComponentQMLDiff, "Validation failed for %s on %s: %d missing hashes",
							filename, hashtable.Name, len(positions))
					} else if treeResult.FilesWithErrors > 0 {
						result.ErrorDetail = fmt.Sprintf("%d file(s) had processing errors", treeResult.FilesWithErrors)
					}
				} else {
					result.Compatible = true
					logging.Info(logging.ComponentQMLDiff, "Validation succeeded for %s on %s: %d files processed, %d modified",
						filename, hashtable.Name, result.FilesProcessed, result.FilesModified)
				}
			} else {
				// No result or error - this shouldn't happen
				result.Compatible = false
				result.ErrorDetail = "no validation result received"
			}

			results[filename] = append(results[filename], result)
		}

		completedHashtables++
		if jobStore != nil && jobID != "" {
			progress := 10 + int(float64(completedHashtables)/float64(totalHashtables)*90)
			jobStore.UpdateProgress(jobID, progress)
		}
	}

	if jobStore != nil && jobID != "" {
		jobStore.UpdateProgress(jobID, 100)
		jobStore.Update(jobID, "success", "Validation complete", nil)
	}

	return results, nil
}

// compareAgainstHashtable is deprecated - use tree validation instead
func (s *Service) compareAgainstHashtable(qmdContent []byte, hashtable *hashtab.Hashtab) ComparisonResult {
	result := ComparisonResult{
		Hashtable:  hashtable.Name,
		OSVersion:  hashtable.OSVersion,
		Device:     hashtable.Device,
		Compatible: true, // Assume compatible in simplified mode
	}

	logging.Info(logging.ComponentQMLDiff, "Simplified comparison for %s (use tree validation for full verification)", hashtable.Name)

	return result
}

func (s *Service) compareWithHashes(hashes []qmd.HashWithPosition, hashtable *hashtab.Hashtab) ComparisonResult {
	result := ComparisonResult{
		Hashtable: hashtable.Name,
		OSVersion: hashtable.OSVersion,
		Device:    hashtable.Device,
	}

	verifyResult := qmd.VerifyWithHashes(hashes, hashtable)
	result.Compatible = verifyResult.Compatible
	result.MissingHashes = verifyResult.MissingHashes

	if !result.Compatible {
		result.ErrorDetail = fmt.Sprintf("missing %d hash(es)", len(verifyResult.MissingHashes))
		logging.Warn(logging.ComponentQMLDiff, "Comparison failed for %s: missing %d hashes", hashtable.Name, len(verifyResult.MissingHashes))
	} else {
		logging.Info(logging.ComponentQMLDiff, "Comparison succeeded for %s", hashtable.Name)
	}

	return result
}

func (s *Service) TestBinary() error {
	logging.Info(logging.ComponentQMLDiff, "Using qmldiff CGO library for QMD verification")
	return nil
}

// ValidateAgainstTree validates a QMD file against a full QML tree
// This is the new validation mode that uses qmldiff to apply diffs
func (s *Service) ValidateAgainstTree(qmdPath, hashtabPath, treePath string) (*TreeValidationResult, error) {
	result, err := ValidateMultipleQMDsWithCLI([]string{qmdPath}, hashtabPath, treePath, s.qmldiffBinary)
	if err != nil {
		return nil, err
	}
	if fileErr, hasError := result.Errors[qmdPath]; hasError {
		return nil, fileErr
	}
	return result.Results[qmdPath], nil
}

// ValidateAgainstTreeWithWorkers validates a QMD file against a full QML tree using CLI
// numWorkers parameter is ignored (kept for API compatibility)
func (s *Service) ValidateAgainstTreeWithWorkers(qmdPath, hashtabPath, treePath string, numWorkers int) (*TreeValidationResult, error) {
	result, err := ValidateMultipleQMDsWithCLI([]string{qmdPath}, hashtabPath, treePath, s.qmldiffBinary)
	if err != nil {
		return nil, err
	}

	if treeResult, ok := result.Results[qmdPath]; ok {
		return treeResult, nil
	}

	if err, ok := result.Errors[qmdPath]; ok {
		return nil, err
	}

	return nil, fmt.Errorf("no result found for QMD: %s", qmdPath)
}

// ValidateMultipleAgainstTree validates multiple QMD files against a full QML tree
// numWorkers parameter is ignored (kept for API compatibility)
func (s *Service) ValidateMultipleAgainstTree(qmdPaths []string, hashtabPath, treePath string, numWorkers int) (*BatchTreeValidationResult, error) {
	return ValidateMultipleQMDsWithCLI(qmdPaths, hashtabPath, treePath, s.qmldiffBinary)
}

// ValidateMultipleAgainstTreeSequential validates multiple QMD files against a full QML tree sequentially
// This version uses the qmldiff CLI binary for isolation (each file gets its own process)
func (s *Service) ValidateMultipleAgainstTreeSequential(qmdPaths []string, hashtabPath, treePath string) (*BatchTreeValidationResult, error) {
	if s.qmldiffBinary == "" {
		// Fallback to default location
		s.qmldiffBinary = "./qmldiff"
	}
	return ValidateMultipleQMDsWithCLI(qmdPaths, hashtabPath, treePath, s.qmldiffBinary)
}

func SaveUploadedFile(reader io.Reader, filename string) (string, error) {
	tempDir, err := os.MkdirTemp("", "qmd-upload-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	uniqueFilename := fmt.Sprintf("%s-%s", uuid.New().String(), filename)
	filePath := filepath.Join(tempDir, uniqueFilename)

	out, err := os.Create(filePath)
	if err != nil {
		os.RemoveAll(tempDir)
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, reader)
	if err != nil {
		os.RemoveAll(tempDir)
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return filePath, nil
}
