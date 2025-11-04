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
	pathByName map[string]string
}

func NewService(dir string) (*Service, error) {
	service := &Service{
		hashtables: make([]*Hashtab, 0),
		dir:        dir,
		modTimes:   make(map[string]time.Time),
		pathByName: make(map[string]string),
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
	loadedNames := make(map[string]string)

	err := filepath.WalkDir(s.dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		filename := filepath.Base(path)

		if existingPath, exists := loadedNames[filename]; exists {
			logging.Warn(logging.ComponentHashtab, "Skipping duplicate hashtable file %s (already loaded from %s)", path, existingPath)
			return nil
		}

		logging.Info(logging.ComponentHashtab, "Loading hashtable: %s", filename)

		ht, err := Load(path)
		if err != nil {
			logging.Error(logging.ComponentHashtab, "Failed to load hashtable %s: %v", filename, err)
			return nil
		}

		formatType := "hashtab (with strings)"
		if ht.IsHashlist() {
			formatType = "hashlist (hash-only)"
		}
		logging.Info(logging.ComponentHashtab, "Loaded %s: %s, %d entries, version %s", filename, formatType, len(ht.Entries), ht.OSVersion)

		s.hashtables = append(s.hashtables, ht)
		loadedNames[filename] = path
		s.pathByName[filename] = path

		fileInfo, err := d.Info()
		if err == nil {
			s.modTimes[path] = fileInfo.ModTime()
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk hashtable directory: %w", err)
	}

	return nil
}

func (s *Service) CheckAndReload() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	currentFiles := make(map[string]time.Time)
	needsReload := false

	err := filepath.WalkDir(s.dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		fileInfo, err := d.Info()
		if err != nil {
			return nil
		}

		modTime := fileInfo.ModTime()
		currentFiles[path] = modTime

		if lastMod, exists := s.modTimes[path]; !exists || !lastMod.Equal(modTime) {
			needsReload = true
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk hashtable directory: %w", err)
	}

	if !needsReload {
		for path := range s.modTimes {
			if _, exists := currentFiles[path]; !exists {
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
	s.pathByName = make(map[string]string)

	loadedNames := make(map[string]string)

	err = filepath.WalkDir(s.dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		filename := filepath.Base(path)

		if existingPath, exists := loadedNames[filename]; exists {
			logging.Warn(logging.ComponentHashtab, "Skipping duplicate hashtable file %s (already loaded from %s)", path, existingPath)
			return nil
		}

		logging.Info(logging.ComponentHashtab, "Loading hashtable: %s", filename)

		ht, err := Load(path)
		if err != nil {
			logging.Error(logging.ComponentHashtab, "Failed to load hashtable %s: %v", filename, err)
			return nil
		}

		formatType := "hashtab (with strings)"
		if ht.IsHashlist() {
			formatType = "hashlist (hash-only)"
		}
		logging.Info(logging.ComponentHashtab, "Loaded %s: %s, %d entries, version %s", filename, formatType, len(ht.Entries), ht.OSVersion)

		s.hashtables = append(s.hashtables, ht)
		loadedNames[filename] = path
		s.pathByName[filename] = path

		fileInfo, err := d.Info()
		if err == nil {
			s.modTimes[path] = fileInfo.ModTime()
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to reload hashtables: %w", err)
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
