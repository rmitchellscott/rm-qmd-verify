package hashtab

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rmitchellscott/rm-qmd-verify/internal/logging"
)

type Service struct {
	hashtables []*Hashtab
	dir        string
	mu         sync.RWMutex
	modTimes   map[string]time.Time
}

func NewService(dir string) (*Service, error) {
	service := &Service{
		hashtables: make([]*Hashtab, 0),
		dir:        dir,
		modTimes:   make(map[string]time.Time),
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		logging.Warn(logging.ComponentHashtab, "Hashtable directory does not exist: %s", dir)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create hashtable directory: %w", err)
		}
		logging.Info(logging.ComponentHashtab, "Created hashtable directory: %s", dir)
		return service, nil
	}

	err := service.loadHashtables()
	if err != nil {
		return nil, err
	}

	return service, nil
}

func (s *Service) loadHashtables() error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return fmt.Errorf("failed to read hashtable directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		path := filepath.Join(s.dir, entry.Name())
		logging.Info(logging.ComponentHashtab, "Loading hashtable: %s", entry.Name())

		ht, err := Load(path)
		if err != nil {
			logging.Error(logging.ComponentHashtab, "Failed to load hashtable %s: %v", entry.Name(), err)
			continue
		}

		formatType := "hashtab (with strings)"
		if ht.IsHashlist() {
			formatType = "hashlist (hash-only)"
		}
		logging.Info(logging.ComponentHashtab, "Loaded %s: %s, %d entries, version %s", entry.Name(), formatType, len(ht.Entries), ht.OSVersion)

		s.hashtables = append(s.hashtables, ht)

		fileInfo, err := entry.Info()
		if err == nil {
			s.modTimes[entry.Name()] = fileInfo.ModTime()
		}
	}

	return nil
}

func (s *Service) CheckAndReload() error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return fmt.Errorf("failed to read hashtable directory: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	currentFiles := make(map[string]bool)
	needsReload := false

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		currentFiles[filename] = true

		fileInfo, err := entry.Info()
		if err != nil {
			continue
		}

		modTime := fileInfo.ModTime()

		if lastMod, exists := s.modTimes[filename]; !exists || !lastMod.Equal(modTime) {
			needsReload = true
			break
		}
	}

	if !needsReload {
		for filename := range s.modTimes {
			if !currentFiles[filename] {
				needsReload = true
				break
			}
		}
	}

	if !needsReload {
		return nil
	}

	logging.Info(logging.ComponentHashtab, "Detected hashtable changes, reloading...")

	s.hashtables = make([]*Hashtab, 0)
	s.modTimes = make(map[string]time.Time)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		path := filepath.Join(s.dir, entry.Name())
		logging.Info(logging.ComponentHashtab, "Loading hashtable: %s", entry.Name())

		ht, err := Load(path)
		if err != nil {
			logging.Error(logging.ComponentHashtab, "Failed to load hashtable %s: %v", entry.Name(), err)
			continue
		}

		formatType := "hashtab (with strings)"
		if ht.IsHashlist() {
			formatType = "hashlist (hash-only)"
		}
		logging.Info(logging.ComponentHashtab, "Loaded %s: %s, %d entries, version %s", entry.Name(), formatType, len(ht.Entries), ht.OSVersion)

		s.hashtables = append(s.hashtables, ht)

		fileInfo, err := entry.Info()
		if err == nil {
			s.modTimes[entry.Name()] = fileInfo.ModTime()
		}
	}

	logging.Info(logging.ComponentHashtab, "Reload complete: %d hashtables loaded", len(s.hashtables))

	return nil
}

func (s *Service) GetHashtables() []*Hashtab {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hashtables
}

func (s *Service) GetHashtable(name string) *Hashtab {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, ht := range s.hashtables {
		if ht.Name == name {
			return ht
		}
	}
	return nil
}
