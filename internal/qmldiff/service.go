package qmldiff

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/google/uuid"
	"github.com/rmitchellscott/rm-qmd-verify/internal/hashtab"
	"github.com/rmitchellscott/rm-qmd-verify/internal/logging"
)

type ComparisonResult struct {
	Hashtable   string `json:"hashtable"`
	OSVersion   string `json:"os_version"`
	Device      string `json:"device"`
	Compatible  bool   `json:"compatible"`
	ErrorDetail string `json:"error_detail,omitempty"`
}

type Service struct {
	binaryPath     string
	hashtabService *hashtab.Service
}

func NewService(binaryPath string, hashtabService *hashtab.Service) *Service {
	return &Service{
		binaryPath:     binaryPath,
		hashtabService: hashtabService,
	}
}

var hashErrorRegex = regexp.MustCompile(`(?:Cannot resolve hash|Couldn't resolve the hashed identifier)\s+(\d+)`)

func sanitizeQmldiffError(rawOutput string) string {
	matches := hashErrorRegex.FindStringSubmatch(rawOutput)
	if len(matches) >= 2 {
		return fmt.Sprintf("Cannot resolve hash %s", matches[1])
	}

	if rawOutput == "" {
		return "Unknown error"
	}

	return "qmldiff comparison failed"
}

func (s *Service) CompareAgainstAll(qmdContent []byte) ([]ComparisonResult, error) {
	hashtables := s.hashtabService.GetHashtables()
	if len(hashtables) == 0 {
		return nil, fmt.Errorf("no hashtables loaded")
	}

	results := make([]ComparisonResult, len(hashtables))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, ht := range hashtables {
		wg.Add(1)
		go func(idx int, hashtable *hashtab.Hashtab) {
			defer wg.Done()

			result := s.compareAgainstHashtable(qmdContent, hashtable)

			mu.Lock()
			results[idx] = result
			mu.Unlock()
		}(i, ht)
	}

	wg.Wait()
	return results, nil
}

func (s *Service) compareAgainstHashtable(qmdContent []byte, hashtable *hashtab.Hashtab) ComparisonResult {
	result := ComparisonResult{
		Hashtable: hashtable.Name,
		OSVersion: hashtable.OSVersion,
		Device:    hashtable.Device,
	}

	tempDir, err := os.MkdirTemp("", "qmd-compare-*")
	if err != nil {
		result.ErrorDetail = fmt.Sprintf("failed to create temp dir: %v", err)
		logging.Error(logging.ComponentQMLDiff, "Failed to create temp dir: %v", err)
		return result
	}
	defer os.RemoveAll(tempDir)

	tempQMD := filepath.Join(tempDir, "temp.qmd")
	err = os.WriteFile(tempQMD, qmdContent, 0644)
	if err != nil {
		result.ErrorDetail = fmt.Sprintf("failed to write temp file: %v", err)
		logging.Error(logging.ComponentQMLDiff, "Failed to write temp file: %v", err)
		return result
	}

	cmd := exec.Command(s.binaryPath, "hash-diffs", hashtable.Path, tempQMD)
	output, err := cmd.CombinedOutput()

	if err != nil {
		result.Compatible = false
		result.ErrorDetail = sanitizeQmldiffError(string(output))
		logging.Warn(logging.ComponentQMLDiff, "Comparison failed for %s: %v", hashtable.Name, err)
	} else {
		result.Compatible = true
		logging.Info(logging.ComponentQMLDiff, "Comparison succeeded for %s", hashtable.Name)
	}

	return result
}

func (s *Service) TestBinary() error {
	if _, err := os.Stat(s.binaryPath); os.IsNotExist(err) {
		return fmt.Errorf("qmldiff binary not found at: %s", s.binaryPath)
	}

	cmd := exec.Command(s.binaryPath, "--help")
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("qmldiff binary test failed: %w", err)
	}

	logging.Info(logging.ComponentQMLDiff, "qmldiff binary test successful: %s", s.binaryPath)
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
