package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/rmitchellscott/rm-qmd-verify/pkg/hashtab"
	"github.com/rmitchellscott/rm-qmd-verify/internal/logging"
	"github.com/rmitchellscott/rm-qmd-verify/internal/qmldiff"
)

type APIHandler struct {
	qmldiffService *qmldiff.Service
	hashtabService *hashtab.Service
}

func NewAPIHandler(qmldiffService *qmldiff.Service, hashtabService *hashtab.Service) *APIHandler {
	return &APIHandler{
		qmldiffService: qmldiffService,
		hashtabService: hashtabService,
	}
}

type CompareResponse struct {
	Compatible   []qmldiff.ComparisonResult `json:"compatible"`
	Incompatible []qmldiff.ComparisonResult `json:"incompatible"`
	TotalChecked int                        `json:"total_checked"`
}

func (h *APIHandler) Compare(w http.ResponseWriter, r *http.Request) {
	if err := h.hashtabService.CheckAndReload(); err != nil {
		logging.Error(logging.ComponentHandler, "Failed to check/reload hashtables: %v", err)
	}

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

	logging.Info(logging.ComponentHandler, "Starting comparison against all hashtables")
	results, err := h.qmldiffService.CompareAgainstAll(content)
	if err != nil {
		logging.Error(logging.ComponentHandler, "Comparison failed: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("Comparison failed: %v", err),
		})
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

	logging.Info(logging.ComponentHandler, "Comparison complete: %d compatible, %d incompatible",
		len(compatible), len(incompatible))

	response := CompareResponse{
		Compatible:   compatible,
		Incompatible: incompatible,
		TotalChecked: len(results),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
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
