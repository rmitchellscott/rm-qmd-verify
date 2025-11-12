package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/rmitchellscott/rm-qmd-verify/internal/jobs"
	"github.com/rmitchellscott/rm-qmd-verify/internal/logging"
	"github.com/rmitchellscott/rm-qmd-verify/internal/qmd"
	"github.com/rmitchellscott/rm-qmd-verify/internal/qmldiff"
	"github.com/rmitchellscott/rm-qmd-verify/pkg/hashtab"
	"github.com/rmitchellscott/rm-qmd-verify/pkg/qmltree"
)

type APIHandler struct {
	qmldiffService           *qmldiff.Service
	hashtabService           *hashtab.Service
	treeService              *qmltree.Service
	jobStore                 *jobs.Store
	maxConcurrentValidations int
}

func NewAPIHandler(qmldiffService *qmldiff.Service, hashtabService *hashtab.Service, treeService *qmltree.Service, jobStore *jobs.Store, maxConcurrentValidations int) *APIHandler {
	return &APIHandler{
		qmldiffService:           qmldiffService,
		hashtabService:           hashtabService,
		treeService:              treeService,
		jobStore:                 jobStore,
		maxConcurrentValidations: maxConcurrentValidations,
	}
}

type CompareResponse struct {
	Compatible   []qmldiff.TreeComparisonResult `json:"compatible"`
	Incompatible []qmldiff.TreeComparisonResult `json:"incompatible"`
	TotalChecked int                            `json:"total_checked"`
	Mode         string                         `json:"mode"` // "tree" or "hash"
}

func (h *APIHandler) Compare(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form (max 100MB for batch uploads)
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		logging.Error(logging.ComponentHandler, "Failed to parse multipart form: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Failed to parse form data",
		})
		return
	}

	// Get all uploaded files
	var fileHeaders []*multipart.FileHeader
	var filePaths []string

	// Try batch upload first (new API)
	if files := r.MultipartForm.File["files"]; len(files) > 0 {
		fileHeaders = files
		// Get corresponding paths (sent separately to bypass browser path sanitization)
		filePaths = r.MultipartForm.Value["paths"]
	} else {
		// Try single file upload (backward compatibility)
		file, header, err := r.FormFile("file")
		if err != nil {
			logging.Error(logging.ComponentHandler, "No files uploaded: %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "No file uploaded or invalid form data",
			})
			return
		}
		file.Close()
		fileHeaders = []*multipart.FileHeader{header}
		filePaths = []string{header.Filename}
	}

	// Create temp directory for uploaded files
	tempDir, err := os.MkdirTemp("", "qmd-upload-*")
	if err != nil {
		logging.Error(logging.ComponentHandler, "Failed to create temp directory: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Failed to create temp directory",
		})
		return
	}

	// Save uploaded files to temp directory
	qmdPaths := make([]string, 0, len(fileHeaders))
	filenames := make([]string, 0, len(fileHeaders))

	for i, fileHeader := range fileHeaders {
		file, err := fileHeader.Open()
		if err != nil {
			logging.Error(logging.ComponentHandler, "Failed to open uploaded file %s: %v", fileHeader.Filename, err)
			os.RemoveAll(tempDir)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"error": fmt.Sprintf("Failed to open file %s", fileHeader.Filename),
			})
			return
		}

		// Preserve folder structure by cleaning the path and creating parent directories
		// Use path from separate field if available (bypasses browser sanitization)
		var relativePath string
		if i < len(filePaths) && filePaths[i] != "" {
			relativePath = filepath.Clean(filePaths[i])
		} else {
			relativePath = filepath.Clean(fileHeader.Filename)
		}
		logging.Debug(logging.ComponentHandler, "Received file: %s (path field: %s) → cleaned: %s",
			fileHeader.Filename, filePaths[i], relativePath)
		tempPath := filepath.Join(tempDir, relativePath)

		// Create parent directories if they don't exist
		if err := os.MkdirAll(filepath.Dir(tempPath), 0755); err != nil {
			file.Close()
			os.RemoveAll(tempDir)
			logging.Error(logging.ComponentHandler, "Failed to create directory for %s: %v", fileHeader.Filename, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"error": fmt.Sprintf("Failed to create directory for file %s", fileHeader.Filename),
			})
			return
		}

		tempFile, err := os.Create(tempPath)
		if err != nil {
			file.Close()
			os.RemoveAll(tempDir)
			logging.Error(logging.ComponentHandler, "Failed to create temp file for %s: %v", fileHeader.Filename, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"error": fmt.Sprintf("Failed to save file %s", fileHeader.Filename),
			})
			return
		}

		bytesWritten, err := io.Copy(tempFile, file)
		file.Close()
		tempFile.Close()

		if err != nil {
			os.RemoveAll(tempDir)
			logging.Error(logging.ComponentHandler, "Failed to save file content for %s: %v", fileHeader.Filename, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"error": fmt.Sprintf("Failed to save file %s", fileHeader.Filename),
			})
			return
		}

		if bytesWritten == 0 {
			logging.Warn(logging.ComponentHandler, "Skipping empty file: %s", fileHeader.Filename)
			continue
		}

		qmdPaths = append(qmdPaths, tempPath)
		filenames = append(filenames, relativePath)
	}

	if len(qmdPaths) == 0 {
		os.RemoveAll(tempDir)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "All uploaded files are empty",
		})
		return
	}

	logging.Info(logging.ComponentHandler, "Received %d file upload(s): %v", len(filenames), filenames)

	// Filter to root-level QMD files only (mimics qmldiff behavior)
	rootLevelQMDs := qmd.GetRootLevelFiles(tempDir, qmdPaths)
	if len(rootLevelQMDs) == 0 {
		os.RemoveAll(tempDir)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "No root-level .qmd files found. Only files at the top level of the upload are validated.",
		})
		return
	}

	// Update paths and filenames to only include root-level files
	originalQmdCount := len(qmdPaths)
	qmdPaths = rootLevelQMDs

	// Extract relative paths for root-level files (preserve directory structure)
	rootFilenames := make([]string, len(rootLevelQMDs))
	for i, path := range rootLevelQMDs {
		relPath, err := filepath.Rel(tempDir, path)
		if err != nil {
			rootFilenames[i] = filepath.Base(path)
		} else {
			rootFilenames[i] = relPath
		}
	}
	filenames = rootFilenames

	if len(qmdPaths) < originalQmdCount {
		logging.Info(logging.ComponentHandler, "Filtered to %d root-level QMD files (excluded %d in subdirectories)",
			len(qmdPaths), originalQmdCount-len(qmdPaths))
	}

	// Get validation mode from query parameter (default: tree)
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "tree"
	}

	jobID := uuid.New().String()
	h.jobStore.Create(jobID)

	logging.Info(logging.ComponentHandler, "Created job %s for %d file(s) (mode: %s)", jobID, len(filenames), mode)

	go func() {
		defer os.RemoveAll(tempDir) // Clean up temp files after processing

		if mode == "tree" {
			// New default: tree validation mode with batch processing using worker pool
			logging.Info(logging.ComponentHandler, "Starting batch tree validation for job %s (%d files)", jobID, len(filenames))
			ctx := context.Background()
			resultsMap, err := h.validateAgainstAllTreesWithWorkers(ctx, qmdPaths, filenames, h.jobStore, jobID)
			if err != nil {
				logging.Error(logging.ComponentHandler, "Tree validation failed for job %s: %v", jobID, err)
				h.jobStore.Update(jobID, "error", fmt.Sprintf("Validation failed: %v", err), nil)
				return
			}

			// For single file, return flat structure (backward compatibility)
			if originalQmdCount == 1 {
				results := resultsMap[filenames[0]]
				compatible := make([]qmldiff.TreeComparisonResult, 0)
				incompatible := make([]qmldiff.TreeComparisonResult, 0)

				for _, result := range results {
					if result.Compatible {
						compatible = append(compatible, result)
					} else {
						incompatible = append(incompatible, result)
					}
				}

				logging.Info(logging.ComponentHandler, "Tree validation complete for job %s: %d compatible, %d incompatible",
					jobID, len(compatible), len(incompatible))

				response := CompareResponse{
					Compatible:   compatible,
					Incompatible: incompatible,
					TotalChecked: len(results),
					Mode:         "tree",
				}

				h.jobStore.SetResults(jobID, response)
				h.jobStore.Update(jobID, "success", "Validation complete", nil)
			} else {
				// For multiple files, return map structure
				batchResponse := make(map[string]CompareResponse)

				for filename, results := range resultsMap {
					compatible := make([]qmldiff.TreeComparisonResult, 0)
					incompatible := make([]qmldiff.TreeComparisonResult, 0)

					for _, result := range results {
						if result.Compatible {
							compatible = append(compatible, result)
						} else {
							incompatible = append(incompatible, result)
						}
					}

					batchResponse[filename] = CompareResponse{
						Compatible:   compatible,
						Incompatible: incompatible,
						TotalChecked: len(results),
						Mode:         "tree",
					}
				}

				// Create filename -> qmdPath map for resolving dependency paths
				filenameToPaths := make(map[string]string)
				for i, filename := range filenames {
					filenameToPaths[filename] = qmdPaths[i]
				}

				// Flatten dependency results into top-level response
				// For each root file's results, extract dependency results and add them as separate entries
				logging.Debug(logging.ComponentHandler, "Starting dependency flattening for %d root files", len(batchResponse))
				for rootFilename, response := range batchResponse {
					// Check both compatible and incompatible results
					// Create fresh slice to avoid underlying array sharing issues
					allResults := make([]qmldiff.TreeComparisonResult, 0, len(response.Compatible)+len(response.Incompatible))
					allResults = append(allResults, response.Compatible...)
					allResults = append(allResults, response.Incompatible...)
					logging.Debug(logging.ComponentHandler, "Processing root file '%s' with %d total results (%d compatible, %d incompatible)",
						rootFilename, len(allResults), len(response.Compatible), len(response.Incompatible))

					// Debug: log hashtables in each category
					compatibleNames := make([]string, len(response.Compatible))
					for i, r := range response.Compatible {
						compatibleNames[i] = r.Hashtable
					}
					incompatibleNames := make([]string, len(response.Incompatible))
					for i, r := range response.Incompatible {
						incompatibleNames[i] = r.Hashtable
					}
					logging.Debug(logging.ComponentHandler, "  Compatible: %v", compatibleNames)
					logging.Debug(logging.ComponentHandler, "  Incompatible: %v", incompatibleNames)

					// Debug: log what's actually in allResults
					allResultsNames := make([]string, len(allResults))
					for i, r := range allResults {
						allResultsNames[i] = r.Hashtable
					}
					logging.Debug(logging.ComponentHandler, "  allResults after append: %v", allResultsNames)

					logging.Debug(logging.ComponentHandler, "  About to iterate over %d results in allResults", len(allResults))
					for i, treeResult := range allResults {
						logging.Debug(logging.ComponentHandler, "  Loop iteration %d: Hashtable %s", i, treeResult.Hashtable)
						depCount := 0
						if treeResult.DependencyResults != nil {
							depCount = len(treeResult.DependencyResults)
						}
						logging.Debug(logging.ComponentHandler, "  Hashtable %s (v%s, %s): %d dependencies",
							treeResult.Hashtable, treeResult.OSVersion, treeResult.Device, depCount)

						if treeResult.DependencyResults != nil && len(treeResult.DependencyResults) > 0 {
							// For each dependency file in this hashtable's results
							for depPath, depResult := range treeResult.DependencyResults {
								logging.Debug(logging.ComponentHandler, "    Processing dependency '%s': compatible=%v, %d hash errors, %d process errors",
									depPath, depResult.Compatible, len(depResult.HashErrors), len(depResult.ProcessErrors))

								// Create a TreeComparisonResult for this dependency
								depTreeResult := qmldiff.TreeComparisonResult{
									Hashtable:          treeResult.Hashtable,
									OSVersion:          treeResult.OSVersion,
									Device:             treeResult.Device,
									Compatible:         depResult.Compatible,
									ValidationMode:     "tree",
									TreeValidationUsed: true,
								}

								// Add error details if the dependency failed
								if !depResult.Compatible {
									if len(depResult.HashErrors) > 0 {
										logging.Debug(logging.ComponentHandler, "      === Dependency Hash Error Processing ===")
										logging.Debug(logging.ComponentHandler, "      Dependency: '%s'", depPath)
										logging.Debug(logging.ComponentHandler, "      Root file: '%s'", rootFilename)

										hashIDs := make([]uint64, len(depResult.HashErrors))
										for i, hashErr := range depResult.HashErrors {
											hashIDs[i] = hashErr.HashID
										}
										logging.Debug(logging.ComponentHandler, "      Hash IDs to find (%d): %v", len(hashIDs), hashIDs)

										// Resolve dependency path relative to root file
										rootPath := filenameToPaths[rootFilename]
										resolvedDepPath := qmd.ResolveLoadPath(rootPath, depPath)
										logging.Debug(logging.ComponentHandler, "      Resolving depPath '%s' relative to root '%s' -> '%s'",
											depPath, rootPath, resolvedDepPath)

										depContents, err := os.ReadFile(resolvedDepPath)
										if err != nil {
											logging.Error(logging.ComponentHandler, "      Failed to read dependency file %s: %v", resolvedDepPath, err)
											depTreeResult.ErrorDetail = fmt.Sprintf("%d hash lookup error(s)", len(depResult.HashErrors))
										} else {
											logging.Debug(logging.ComponentHandler, "      Successfully read file, size: %d bytes", len(depContents))
											depStr := string(depContents)
											depTreeResult.MissingHashes = qmd.FindHashPositions(depStr, hashIDs)
											logging.Debug(logging.ComponentHandler, "      FindHashPositions returned %d positions: %v",
												len(depTreeResult.MissingHashes), depTreeResult.MissingHashes)

											// If we found positions in the file, show them
											if len(depTreeResult.MissingHashes) > 0 {
												depTreeResult.ErrorDetail = fmt.Sprintf("missing %d hash(es)", len(depTreeResult.MissingHashes))
											} else {
												// Hash errors exist but aren't in this file - they're in referenced files
												// Show hash IDs without positions
												depTreeResult.ErrorDetail = fmt.Sprintf("%d hash lookup error(s)", len(depResult.HashErrors))
												// Create MissingHashes entries with the hash IDs but no position info
												depTreeResult.MissingHashes = make([]qmd.HashWithPosition, len(hashIDs))
												for i, hashID := range hashIDs {
													depTreeResult.MissingHashes[i] = qmd.HashWithPosition{
														Hash:   hashID,
														Line:   0,
														Column: 0,
													}
												}
											}
											logging.Debug(logging.ComponentHandler, "      Final ErrorDetail: '%s'", depTreeResult.ErrorDetail)
										}
									} else if len(depResult.ProcessErrors) > 0 {
													depTreeResult.ErrorDetail = "QML failed to apply"
									} else {
										// Provide user-friendly message based on status
										if depResult.Status == qmd.StatusNotAttempted {
											if depResult.BlockedBy != "" {
												depTreeResult.ErrorDetail = fmt.Sprintf("Not validated due to failure of dependency %s", depResult.BlockedBy)
											} else {
												depTreeResult.ErrorDetail = "Not attempted due to prior failure"
											}
										} else {
											depTreeResult.ErrorDetail = fmt.Sprintf("Validation status: %s", depResult.Status)
										}
									}
								}

								// Get or create the CompareResponse for this dependency
								if existingResponse, exists := batchResponse[depPath]; exists {
									// Append to existing response
									logging.Debug(logging.ComponentHandler, "      Appending to existing entry (now %d total)", existingResponse.TotalChecked+1)
									if depResult.Compatible {
										existingResponse.Compatible = append(existingResponse.Compatible, depTreeResult)
									} else {
										existingResponse.Incompatible = append(existingResponse.Incompatible, depTreeResult)
									}
									existingResponse.TotalChecked++
									batchResponse[depPath] = existingResponse
								} else {
									// Create new response for this dependency
									logging.Debug(logging.ComponentHandler, "      Creating new entry for dependency '%s'", depPath)
									newResponse := CompareResponse{
										Mode:         "tree",
										TotalChecked: 1,
									}
									if depResult.Compatible {
										newResponse.Compatible = []qmldiff.TreeComparisonResult{depTreeResult}
										newResponse.Incompatible = []qmldiff.TreeComparisonResult{}
									} else {
										newResponse.Compatible = []qmldiff.TreeComparisonResult{}
										newResponse.Incompatible = []qmldiff.TreeComparisonResult{depTreeResult}
									}
									batchResponse[depPath] = newResponse
								}
							}
						}
					}

					logging.Debug(logging.ComponentHandler, "Flattened dependency results for %s", rootFilename)
				}

				// Log final batchResponse summary
				logging.Debug(logging.ComponentHandler, "Final batchResponse contains %d entries:", len(batchResponse))
				for filename, response := range batchResponse {
					logging.Debug(logging.ComponentHandler, "  '%s': %d total (%d compatible, %d incompatible)",
						filename, response.TotalChecked, len(response.Compatible), len(response.Incompatible))
				}

				logging.Info(logging.ComponentHandler, "Batch tree validation complete for job %s: %d files processed, %d total results (including dependencies)",
					jobID, len(filenames), len(batchResponse))

				h.jobStore.SetResults(jobID, batchResponse)
				h.jobStore.Update(jobID, "success", "Batch validation complete", nil)
			}
		} else {
			// Legacy hash-only mode (temporarily disabled with worker pool migration)
			logging.Warn(logging.ComponentHandler, "Hash-only mode temporarily disabled during worker pool migration")
			h.jobStore.Update(jobID, "error", "Hash-only mode temporarily unavailable", nil)
			return

			// TODO: Implement hash-only mode with worker pool
			/*
			if len(qmdContents) > 1 {
				h.jobStore.Update(jobID, "error", "Hash-only mode does not support batch uploads", nil)
				return
			}

			logging.Info(logging.ComponentHandler, "Starting legacy hash-only comparison for job %s", jobID)
			results, err := h.qmldiffService.CompareAgainstAllWithProgress(qmdContents[0], h.jobStore, jobID)
			if err != nil {
				logging.Error(logging.ComponentHandler, "Comparison failed for job %s: %v", jobID, err)
				h.jobStore.Update(jobID, "error", fmt.Sprintf("Comparison failed: %v", err), nil)
				return
			}

			// Convert ComparisonResult to TreeComparisonResult for consistent response format
			compatible := make([]qmldiff.TreeComparisonResult, 0)
			incompatible := make([]qmldiff.TreeComparisonResult, 0)

			for _, result := range results {
				treeResult := qmldiff.TreeComparisonResult{
					Hashtable:          result.Hashtable,
					OSVersion:          result.OSVersion,
					Device:             result.Device,
					Compatible:         result.Compatible,
					ErrorDetail:        result.ErrorDetail,
					MissingHashes:      result.MissingHashes,
					ValidationMode:     "hash",
					TreeValidationUsed: false,
				}

				if treeResult.Compatible {
					compatible = append(compatible, treeResult)
				} else {
					incompatible = append(incompatible, treeResult)
				}
			}

			logging.Info(logging.ComponentHandler, "Comparison complete for job %s: %d compatible, %d incompatible",
				jobID, len(compatible), len(incompatible))

			response := CompareResponse{
				Compatible:   compatible,
				Incompatible: incompatible,
				TotalChecked: len(results),
				Mode:         "hash",
			}

			h.jobStore.SetResults(jobID, response)
			h.jobStore.Update(jobID, "success", "Comparison complete", nil)
			*/
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"jobId": jobID,
	})
}

type HashtableInfo struct {
	Name       string `json:"name"`
	OSVersion  string `json:"os_version"`
	Device     string `json:"device"`
	EntryCount int    `json:"entry_count"`
}

func (h *APIHandler) ListHashtables(w http.ResponseWriter, r *http.Request) {
	if err := h.hashtabService.CheckAndReload(); err != nil {
		logging.Error(logging.ComponentHandler, "Failed to check/reload hashtables: %v", err)
	}

	hashtables := h.hashtabService.GetHashtables()

	info := make([]HashtableInfo, len(hashtables))
	for i, ht := range hashtables {
		info[i] = HashtableInfo{
			Name:       ht.Name,
			OSVersion:  ht.OSVersion,
			Device:     ht.Device,
			EntryCount: len(ht.Entries),
		}
	}

	response := map[string]interface{}{
		"hashtables": info,
		"count":      len(info),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

type TreeInfo struct {
	Name      string `json:"name"`
	OSVersion string `json:"os_version"`
	Device    string `json:"device"`
	FileCount int    `json:"file_count"`
}

func (h *APIHandler) ListValidatedVersions(w http.ResponseWriter, r *http.Request) {
	hashtables := h.hashtabService.GetHashtables()
	versionSet := make(map[string]bool)

	for _, ht := range hashtables {
		_, treeExists := h.treeService.GetTreeByName(ht.Name)
		if treeExists {
			versionSet[ht.OSVersion] = true
		}
	}

	versions := make([]string, 0, len(versionSet))
	for version := range versionSet {
		versions = append(versions, version)
	}

	response := map[string]interface{}{
		"versions": versions,
		"count":    len(versions),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	go func() {
		if err := h.hashtabService.CheckAndReload(); err != nil {
			logging.Error(logging.ComponentHandler, "Failed to check/reload hashtables: %v", err)
		}
		if err := h.treeService.CheckAndReload(); err != nil {
			logging.Error(logging.ComponentHandler, "Failed to check/reload trees: %v", err)
		}
	}()
}

func (h *APIHandler) ListTrees(w http.ResponseWriter, r *http.Request) {
	if err := h.treeService.CheckAndReload(); err != nil {
		logging.Error(logging.ComponentHandler, "Failed to check/reload trees: %v", err)
	}

	trees := h.treeService.GetTrees()

	info := make([]TreeInfo, len(trees))
	for i, tree := range trees {
		info[i] = TreeInfo{
			Name:      tree.Name,
			OSVersion: tree.OSVersion,
			Device:    tree.Device,
			FileCount: tree.FileCount,
		}
	}

	response := map[string]interface{}{
		"trees": info,
		"count": len(info),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func (h *APIHandler) GetResults(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobId")
	if jobID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Job ID required",
		})
		return
	}

	job, ok := h.jobStore.Get(jobID)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Job not found",
		})
		return
	}

	if job.Status != "success" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  job.Status,
			"message": job.Message,
		})
		return
	}

	if job.Results == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Results not available",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(job.Results)
}

// ValidateTree validates a QMD file against a full QML tree
func (h *APIHandler) ValidateTree(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form (max 32MB)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		logging.Error(logging.ComponentHandler, "Failed to parse multipart form: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Failed to parse form data",
		})
		return
	}

	// Get QMD file
	file, header, err := r.FormFile("file")
	if err != nil {
		logging.Error(logging.ComponentHandler, "Failed to get uploaded file: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "No file uploaded or invalid form data",
		})
		return
	}
	defer file.Close()

	// Get form values
	hashtabPath := r.FormValue("hashtab_path")
	treePath := r.FormValue("tree_path")
	workersStr := r.FormValue("workers")

	if hashtabPath == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "hashtab_path is required",
		})
		return
	}

	if treePath == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "tree_path is required",
		})
		return
	}

	workers := 4
	if workersStr != "" {
		if _, err := fmt.Sscanf(workersStr, "%d", &workers); err != nil || workers < 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "workers must be a positive integer",
			})
			return
		}
	}

	logging.Info(logging.ComponentHandler, "Received tree validation request: %s, hashtab=%s, tree=%s, workers=%d",
		header.Filename, hashtabPath, treePath, workers)

	// Save QMD file to temporary location
	qmdPath, err := qmldiff.SaveUploadedFile(file, header.Filename)
	if err != nil {
		logging.Error(logging.ComponentHandler, "Failed to save uploaded file: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Failed to save uploaded file",
		})
		return
	}

	jobID := uuid.New().String()
	h.jobStore.Create(jobID)
	logging.Info(logging.ComponentHandler, "Created tree validation job %s for file %s", jobID, header.Filename)

	// Run validation in background
	go func() {
		defer os.RemoveAll(filepath.Dir(qmdPath))

		logging.Info(logging.ComponentHandler, "Starting tree validation for job %s", jobID)
		h.jobStore.UpdateWithOperation(jobID, "running", "Validating QMD against QML tree", nil, "validating")
		h.jobStore.UpdateProgress(jobID, 10)

		// Validate using qmldiff service
		result, err := h.qmldiffService.ValidateAgainstTree(qmdPath, hashtabPath, treePath)
		if err != nil {
			logging.Error(logging.ComponentHandler, "Tree validation failed for job %s: %v", jobID, err)
			h.jobStore.Update(jobID, "error", fmt.Sprintf("Validation failed: %v", err), nil)
			return
		}

		response := map[string]interface{}{
			"files_processed":   result.FilesProcessed,
			"files_modified":    result.FilesModified,
			"files_with_errors": result.FilesWithErrors,
			"has_hash_errors":   result.HasHashErrors,
			"errors":            result.Errors,
			"failed_hashes":     result.FailedHashes,
			"success":           result.FilesWithErrors == 0 && !result.HasHashErrors,
		}

		logging.Info(logging.ComponentHandler, "Tree validation complete for job %s: %d processed, %d modified, %d errors",
			jobID, result.FilesProcessed, result.FilesModified, result.FilesWithErrors)

		h.jobStore.SetResults(jobID, response)
		h.jobStore.Update(jobID, "success", "Validation complete", nil)
		h.jobStore.UpdateProgress(jobID, 100)
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"jobId": jobID,
	})
}
