package handlers

import (
	"context"
	"fmt"

	"github.com/rmitchellscott/rm-qmd-verify/internal/jobs"
	"github.com/rmitchellscott/rm-qmd-verify/internal/logging"
	"github.com/rmitchellscott/rm-qmd-verify/internal/qmldiff"
	"github.com/rmitchellscott/rm-qmd-verify/pkg/qmltree"
)

// validateAgainstAllTreesWithWorkers uses the qmldiff CLI binary to validate QMD files
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

	totalComparisons := len(hashtables) * len(trees)
	completedComparisons := 0

	// Group hashtables by version for sequential processing
	hashtablesByVersion := make(map[string][]string)
	for _, ht := range hashtables {
		version := ht.OSVersion
		hashtablesByVersion[version] = append(hashtablesByVersion[version], ht.Name)
	}

	// Process each hashtable version sequentially
	for version, htNames := range hashtablesByVersion {
		for _, htName := range htNames {
			ht := h.hashtabService.GetHashtable(htName)
			if ht == nil {
				logging.Warn(logging.ComponentHandler, "Hashtable %s not found, skipping", htName)
				continue
			}

			// Find matching tree
			var matchingTree *qmltree.Tree
			for i := range trees {
				if trees[i].OSVersion == version {
					matchingTree = trees[i]
					break
				}
			}

			if matchingTree == nil {
				logging.Warn(logging.ComponentHandler, "No tree found for hashtable %s (version %s), skipping", htName, version)
				completedComparisons++
				continue
			}

			logging.Info(logging.ComponentHandler, "Validating %d file(s) against hashtable %s and tree %s",
				len(qmdPaths), htName, matchingTree.Name)

			// Call qmldiff service directly with CLI binary
			batchResult, err := h.qmldiffService.ValidateMultipleAgainstTreeSequential(
				qmdPaths,
				ht.Path,
				matchingTree.Path,
			)

			if err != nil {
				logging.Error(logging.ComponentHandler, "Validation failed for %s/%s: %v", htName, matchingTree.Name, err)

				// Add error results for all files
				for _, filename := range filenames {
					resultsMap[filename] = append(resultsMap[filename], qmldiff.TreeComparisonResult{
						Hashtable:          htName,
						OSVersion:          ht.OSVersion,
						Device:             matchingTree.Device,
						Compatible:         false,
						ErrorDetail:        fmt.Sprintf("Validation error: %v", err),
						ValidationMode:     "tree",
						TreeValidationUsed: true,
					})
				}
			} else {
				// Process results for each file
				for i, qmdPath := range qmdPaths {
					filename := filenames[i]

					// Check if this file had an error
					if fileErr, hasError := batchResult.Errors[qmdPath]; hasError {
						resultsMap[filename] = append(resultsMap[filename], qmldiff.TreeComparisonResult{
							Hashtable:          htName,
							OSVersion:          ht.OSVersion,
							Device:             matchingTree.Device,
							Compatible:         false,
							ErrorDetail:        fmt.Sprintf("validation error: %v", fileErr),
							ValidationMode:     "tree",
							TreeValidationUsed: true,
						})
					} else if treeResult, hasResult := batchResult.Results[qmdPath]; hasResult {
						compatible := treeResult.FilesWithErrors == 0
						errorDetail := ""
						if !compatible {
							errorDetail = fmt.Sprintf("%d files with errors", treeResult.FilesWithErrors)
						}

						resultsMap[filename] = append(resultsMap[filename], qmldiff.TreeComparisonResult{
							Hashtable:          htName,
							OSVersion:          ht.OSVersion,
							Device:             matchingTree.Device,
							Compatible:         compatible,
							ErrorDetail:        errorDetail,
							ValidationMode:     "tree",
							TreeValidationUsed: true,
							FilesProcessed:     treeResult.FilesProcessed,
							FilesModified:      treeResult.FilesModified,
							FilesWithErrors:    treeResult.FilesWithErrors,
						})
					} else {
						// No result or error - this shouldn't happen
						resultsMap[filename] = append(resultsMap[filename], qmldiff.TreeComparisonResult{
							Hashtable:          htName,
							OSVersion:          ht.OSVersion,
							Device:             matchingTree.Device,
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
		}
	}

	return resultsMap, nil
}
