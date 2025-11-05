package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/rmitchellscott/rm-qmd-verify/pkg/hashtab"
	"github.com/rmitchellscott/rm-qmd-verify/internal/jobs"
	"github.com/rmitchellscott/rm-qmd-verify/internal/logging"
	"github.com/rmitchellscott/rm-qmd-verify/internal/qmldiff"
)

type APIHandler struct {
	qmldiffService *qmldiff.Service
	hashtabService *hashtab.Service
	jobStore       *jobs.Store
}

func NewAPIHandler(qmldiffService *qmldiff.Service, hashtabService *hashtab.Service, jobStore *jobs.Store) *APIHandler {
	return &APIHandler{
		qmldiffService: qmldiffService,
		hashtabService: hashtabService,
		jobStore:       jobStore,
	}
}

type CompareResponse struct {
	Compatible   []qmldiff.ComparisonResult `json:"compatible"`
	Incompatible []qmldiff.ComparisonResult `json:"incompatible"`
	TotalChecked int                        `json:"total_checked"`
}

func (h *APIHandler) Compare(w http.ResponseWriter, r *http.Request) {
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

	logging.Info(logging.ComponentHandler, "Received file upload: %s (%d bytes)", header.Filename, header.Size)

	content, err := io.ReadAll(file)
	if err != nil {
		logging.Error(logging.ComponentHandler, "Failed to read file content: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Failed to read uploaded file",
		})
		return
	}

	if len(content) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Uploaded file is empty",
		})
		return
	}

	jobID := uuid.New().String()
	h.jobStore.Create(jobID)

	logging.Info(logging.ComponentHandler, "Created job %s for file %s", jobID, header.Filename)

	go func() {
		logging.Info(logging.ComponentHandler, "Starting comparison against all hashtables for job %s", jobID)
		results, err := h.qmldiffService.CompareAgainstAllWithProgress(content, h.jobStore, jobID)
		if err != nil {
			logging.Error(logging.ComponentHandler, "Comparison failed for job %s: %v", jobID, err)
			h.jobStore.Update(jobID, "error", fmt.Sprintf("Comparison failed: %v", err), nil)
			return
		}

		compatible := make([]qmldiff.ComparisonResult, 0)
		incompatible := make([]qmldiff.ComparisonResult, 0)

		for _, result := range results {
			if result.Compatible {
				compatible = append(compatible, result)
			} else {
				incompatible = append(incompatible, result)
			}
		}

		logging.Info(logging.ComponentHandler, "Comparison complete for job %s: %d compatible, %d incompatible",
			jobID, len(compatible), len(incompatible))

		response := CompareResponse{
			Compatible:   compatible,
			Incompatible: incompatible,
			TotalChecked: len(results),
		}

		h.jobStore.SetResults(jobID, response)
		h.jobStore.Update(jobID, "success", "Comparison complete", nil)
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
