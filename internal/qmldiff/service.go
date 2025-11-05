package qmldiff

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/google/uuid"
	"github.com/rmitchellscott/rm-qmd-verify/pkg/hashtab"
	"github.com/rmitchellscott/rm-qmd-verify/internal/jobs"
	"github.com/rmitchellscott/rm-qmd-verify/internal/logging"
	"github.com/rmitchellscott/rm-qmd-verify/internal/qmd"
)

type ComparisonResult struct {
	Hashtable     string   `json:"hashtable"`
	OSVersion     string   `json:"os_version"`
	Device        string   `json:"device"`
	Compatible    bool     `json:"compatible"`
	ErrorDetail   string   `json:"error_detail,omitempty"`
	MissingHashes []uint64 `json:"-"`
}

func (cr ComparisonResult) MarshalJSON() ([]byte, error) {
	type Alias ComparisonResult

	var missingHashesStr []string
	if len(cr.MissingHashes) > 0 {
		missingHashesStr = make([]string, len(cr.MissingHashes))
		for i, hash := range cr.MissingHashes {
			missingHashesStr[i] = strconv.FormatUint(hash, 10)
		}
	}

	return json.Marshal(&struct {
		*Alias
		MissingHashes []string `json:"missing_hashes,omitempty"`
	}{
		Alias:         (*Alias)(&cr),
		MissingHashes: missingHashesStr,
	})
}

type Service struct {
	hashtabService *hashtab.Service
}

func NewService(binaryPath string, hashtabService *hashtab.Service) *Service {
	return &Service{
		hashtabService: hashtabService,
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
		jobStore.UpdateWithOperation(jobID, "running", "Parsing QMD file", nil, "parsing")
		jobStore.UpdateProgress(jobID, 10)
	}

	parser := qmd.NewParser(string(qmdContent))
	hashes, err := parser.ExtractHashes()
	if err != nil {
		if jobStore != nil && jobID != "" {
			jobStore.Update(jobID, "error", fmt.Sprintf("Failed to parse QMD file: %v", err), nil)
		}
		return nil, fmt.Errorf("failed to extract hashes: %w", err)
	}

	if jobStore != nil && jobID != "" {
		jobStore.UpdateWithOperation(jobID, "running", "Comparing against hashtables", nil, "comparing")
		jobStore.UpdateProgress(jobID, 20)
	}

	results := make([]ComparisonResult, len(hashtables))
	var wg sync.WaitGroup
	var mu sync.Mutex
	completed := 0

	for i, ht := range hashtables {
		wg.Add(1)
		go func(idx int, hashtable *hashtab.Hashtab) {
			defer wg.Done()

			result := s.compareWithHashes(hashes, hashtable)

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

func (s *Service) compareAgainstHashtable(qmdContent []byte, hashtable *hashtab.Hashtab) ComparisonResult {
	result := ComparisonResult{
		Hashtable: hashtable.Name,
		OSVersion: hashtable.OSVersion,
		Device:    hashtable.Device,
	}

	verifyResult, err := qmd.VerifyAgainstHashtab(string(qmdContent), hashtable)
	if err != nil {
		result.ErrorDetail = fmt.Sprintf("verification failed: %v", err)
		logging.Error(logging.ComponentQMLDiff, "Verification failed for %s: %v", hashtable.Name, err)
		return result
	}

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

func (s *Service) compareWithHashes(hashes []uint64, hashtable *hashtab.Hashtab) ComparisonResult {
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
	logging.Info(logging.ComponentQMLDiff, "Using native Go QMD verification (no binary required)")
	return nil
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
