package handlers

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/rmitchellscott/rm-qmd-verify/internal/jobs"
	"github.com/rmitchellscott/rm-qmd-verify/internal/logging"
	"github.com/rmitchellscott/rm-qmd-verify/internal/qmd"
	"github.com/rmitchellscott/rm-qmd-verify/internal/qmldiff"
	"github.com/rmitchellscott/rm-qmd-verify/pkg/qmltree"
)

// validateAgainstAllTreesWithWorkers uses the qmldiff CLI binary to validate QMD files in parallel
func (h *APIHandler) validateAgainstAllTreesWithWorkers(
	ctx context.Context,
	qmdPaths []string,
	filenames []string,
	jobStore *jobs.Store,
	jobID string,
) (map[string][]qmldiff.TreeComparisonResult, error) {

	hashtables := h.hashtabService.GetHashtables()
	trees := h.treeService.GetTrees()

	if len(hashtables) == 0 {
		return nil, fmt.Errorf("no hashtables available")
	}
	if len(trees) == 0 {
		return nil, fmt.Errorf("no QML trees available")
	}

	resultsMap := make(map[string][]qmldiff.TreeComparisonResult)
	for _, filename := range filenames {
		resultsMap[filename] = make([]qmldiff.TreeComparisonResult, 0)
	}

	totalComparisons := len(hashtables)
	completedComparisons := 0

	// Mutex for thread-safe access to shared state
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Semaphore to limit concurrent validations
	semaphore := make(chan struct{}, h.maxConcurrentValidations)

	logging.Info(logging.ComponentHandler, "Starting parallel validation with max concurrency: %d", h.maxConcurrentValidations)

	// Process each hashtable in parallel
	for _, ht := range hashtables {
		// Find matching tree
		var matchingTree *qmltree.Tree
		for i := range trees {
			if trees[i].OSVersion == ht.OSVersion && trees[i].Device == ht.Device {
				matchingTree = trees[i]
				break
			}
		}

		if matchingTree == nil {
			logging.Warn(logging.ComponentHandler, "No tree found for hashtable %s (version %s, device %s), skipping", ht.Name, ht.OSVersion, ht.Device)
			mu.Lock()
			completedComparisons++
			if jobStore != nil {
				progress := int((float64(completedComparisons) / float64(totalComparisons)) * 100)
				jobStore.UpdateProgress(jobID, progress)
			}
			mu.Unlock()
			continue
		}

		wg.Add(1)
		go func(htName string, htPath string, htOSVersion string, htDevice string, tree *qmltree.Tree) {
			defer wg.Done()

			// Acquire semaphore slot
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			logging.Info(logging.ComponentHandler, "Validating %d file(s) against hashtable %s and tree %s",
				len(qmdPaths), htName, tree.Name)

			// Call qmldiff service directly with CLI binary
			batchResult, err := h.qmldiffService.ValidateMultipleAgainstTreeSequential(
				qmdPaths,
				htPath,
				tree.Path,
			)

			mu.Lock()
			defer mu.Unlock()

			logging.Debug(logging.ComponentHandler, "Validation returned for %s: err=%v, hasResults=%v, resultCount=%d",
				htName, err != nil, batchResult != nil && len(batchResult.Results) > 0,
				func() int { if batchResult != nil { return len(batchResult.Results) }; return 0 }())

			if err != nil {
				logging.Error(logging.ComponentHandler, "Validation failed for %s/%s: %v", htName, tree.Name, err)

				// Add error results for all files
				logging.Debug(logging.ComponentHandler, "Taking error path for %s, adding %d file results", htName, len(filenames))
				for i, filename := range filenames {
					errorDetail := "QML application failed"
					if !strings.Contains(err.Error(), "panicked") {
						errorDetail = fmt.Sprintf("Validation error: %v", err)
					}

					var depResults map[string]*qmd.ValidationResult
					if batchResult != nil && len(qmdPaths) > i {
						qmdPath := qmdPaths[i]
						if treeResult, hasResult := batchResult.Results[qmdPath]; hasResult {
							depResults = treeResult.DependencyResults
							logging.Debug(logging.ComponentHandler, "  File %s: Found %d dependencies in error results", filename, len(depResults))
						}
					}

					resultsMap[filename] = append(resultsMap[filename], qmldiff.TreeComparisonResult{
						Hashtable:          htName,
						OSVersion:          htOSVersion,
						Device:             tree.Device,
						Compatible:         false,
						ErrorDetail:        errorDetail,
						DependencyResults:  depResults,
						ValidationMode:     "tree",
						TreeValidationUsed: true,
					})
				}
			} else {
				// Process results for each file
				logging.Debug(logging.ComponentHandler, "Taking success path for %s, processing %d files", htName, len(qmdPaths))

				// Debug: log all keys in batchResult.Results
				resultKeys := make([]string, 0, len(batchResult.Results))
				for key := range batchResult.Results {
					resultKeys = append(resultKeys, key)
				}
				logging.Debug(logging.ComponentHandler, "  batchResult.Results keys: %v", resultKeys)

				for i, qmdPath := range qmdPaths {
					filename := filenames[i]
					logging.Debug(logging.ComponentHandler, "  Looking for qmdPath='%s' in results", qmdPath)

					// Check if this file had an error
					if fileErr, hasError := batchResult.Errors[qmdPath]; hasError {
						logging.Debug(logging.ComponentHandler, "  File %s: Has file-level error", filename)
						errorDetail := "QML application failed"
						if !strings.Contains(fileErr.Error(), "panicked") {
							errorDetail = fmt.Sprintf("validation error: %v", fileErr)
						}

						var depResults map[string]*qmd.ValidationResult
						if treeResult, hasResult := batchResult.Results[qmdPath]; hasResult {
							depResults = treeResult.DependencyResults
						}

						resultsMap[filename] = append(resultsMap[filename], qmldiff.TreeComparisonResult{
							Hashtable:          htName,
							OSVersion:          htOSVersion,
							Device:             tree.Device,
							Compatible:         false,
							ErrorDetail:        errorDetail,
							DependencyResults:  depResults,
							ValidationMode:     "tree",
							TreeValidationUsed: true,
						})
					} else if treeResult, hasResult := batchResult.Results[qmdPath]; hasResult {
						compatible := treeResult.FilesWithErrors == 0 && !treeResult.HasHashErrors
						logging.Debug(logging.ComponentHandler, "  File %s: Has result, compatible=%v, depCount=%d",
							filename, compatible, len(treeResult.DependencyResults))
						errorDetail := ""
						var missingHashes []qmd.HashWithPosition

						// Map failed hashes to positions in the QMD file
						if len(treeResult.FailedHashes) > 0 {
							qmdContents, err := os.ReadFile(qmdPath)
							if err != nil {
								logging.Error(logging.ComponentHandler, "Failed to read QMD file %s: %v", qmdPath, err)
							} else {
								qmdStr := string(qmdContents)
								missingHashes = qmd.FindHashPositions(qmdStr, treeResult.FailedHashes)
								errorDetail = fmt.Sprintf("missing %d hash(es)", len(missingHashes))
								logging.Warn(logging.ComponentHandler, "Validation failed for %s on %s: %d missing hashes",
									filename, htName, len(missingHashes))
							}
						} else if !compatible {
							// Errors are in dependency files, not the root file
							if treeResult.FilesWithErrors == 1 {
								errorDetail = "1 dependency file has errors"
							} else {
								errorDetail = fmt.Sprintf("%d dependency files have errors", treeResult.FilesWithErrors)
							}
						}

						resultsMap[filename] = append(resultsMap[filename], qmldiff.TreeComparisonResult{
							Hashtable:          htName,
							OSVersion:          htOSVersion,
							Device:             tree.Device,
							Compatible:         compatible,
							ErrorDetail:        errorDetail,
							MissingHashes:      missingHashes,
							DependencyResults:  treeResult.DependencyResults,
							ValidationMode:     "tree",
							TreeValidationUsed: true,
							FilesProcessed:     treeResult.FilesProcessed,
							FilesModified:      treeResult.FilesModified,
							FilesWithErrors:    treeResult.FilesWithErrors,
						})
						logging.Debug(logging.ComponentHandler, "  Added result to resultsMap[%s]: %s (compatible=%v, depCount=%d)",
							filename, htName, compatible, len(treeResult.DependencyResults))
					} else {
						// No result or error - this shouldn't happen
						logging.Warn(logging.ComponentHandler, "  File %s: No result or error received from validation!", filename)
						resultsMap[filename] = append(resultsMap[filename], qmldiff.TreeComparisonResult{
							Hashtable:          htName,
							OSVersion:          htOSVersion,
							Device:             tree.Device,
							Compatible:         false,
							ErrorDetail:        "no validation result received",
							ValidationMode:     "tree",
							TreeValidationUsed: true,
						})
					}
				}
			}

			completedComparisons++
			if jobStore != nil {
				progress := int((float64(completedComparisons) / float64(totalComparisons)) * 100)
				jobStore.UpdateProgress(jobID, progress)
			}
		}(ht.Name, ht.Path, ht.OSVersion, ht.Device, matchingTree)
	}

	// Wait for all validations to complete
	wg.Wait()

	logging.Info(logging.ComponentHandler, "Parallel validation complete: %d hashtables processed", completedComparisons)

	return resultsMap, nil
}
