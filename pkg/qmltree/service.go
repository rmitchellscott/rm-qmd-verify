package qmltree

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Service manages QML tree discovery and lookup
type Service struct {
	dir      string
	trees    map[string]*Tree      // Map of tree name -> Tree
	modTimes map[string]time.Time  // Map of tree path -> modification time
	mu       sync.RWMutex
}

// NewService creates a new QML tree service
func NewService(dir string) *Service {
	s := &Service{
		dir:      dir,
		trees:    make(map[string]*Tree),
		modTimes: make(map[string]time.Time),
	}

	// Initial load
	if err := s.load(); err != nil {
		// Log error but don't fail - gracefully fall back to empty trees
		fmt.Fprintf(os.Stderr, "[qmltree] Failed to load trees from %s: %v\n", dir, err)
	}

	return s
}

// GetTrees returns all discovered trees
func (s *Service) GetTrees() []*Tree {
	s.mu.RLock()
	defer s.mu.RUnlock()

	trees := make([]*Tree, 0, len(s.trees))
	for _, tree := range s.trees {
		trees = append(trees, tree)
	}
	return trees
}

// GetTreeByName looks up a tree by name
func (s *Service) GetTreeByName(name string) (*Tree, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tree, exists := s.trees[name]
	return tree, exists
}

// CheckAndReload checks if trees have changed and reloads if necessary
func (s *Service) CheckAndReload() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if directory exists
	if _, err := os.Stat(s.dir); os.IsNotExist(err) {
		// Directory doesn't exist - clear trees if we had any
		if len(s.trees) > 0 {
			s.trees = make(map[string]*Tree)
			s.modTimes = make(map[string]time.Time)
			fmt.Fprintf(os.Stderr, "[qmltree] Tree directory removed, clearing trees\n")
		}
		return nil
	}

	currentDirs := make(map[string]time.Time)
	needsReload := false

	// Read directory entries and check modification times
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		path := filepath.Join(s.dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		modTime := info.ModTime()
		currentDirs[path] = modTime

		// Check if this is a new or modified directory
		if lastMod, exists := s.modTimes[path]; !exists || !lastMod.Equal(modTime) {
			needsReload = true
		}
	}

	// Check for deleted directories
	if !needsReload {
		for path := range s.modTimes {
			if _, exists := currentDirs[path]; !exists {
				needsReload = true
				break
			}
		}
	}

	// No changes detected
	if !needsReload {
		return nil
	}

	fmt.Fprintf(os.Stderr, "[qmltree] Detected tree changes, reloading...\n")

	// Clear existing trees and modTimes
	s.trees = make(map[string]*Tree)
	s.modTimes = make(map[string]time.Time)

	// Reload all trees
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		path := filepath.Join(s.dir, name)

		// Create Tree object
		tree, err := NewTree(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[qmltree] Failed to load tree %s: %v\n", name, err)
			continue
		}

		s.trees[name] = tree

		// Store modification time
		info, err := entry.Info()
		if err == nil {
			s.modTimes[path] = info.ModTime()
		}
	}

	fmt.Fprintf(os.Stderr, "[qmltree] Reload complete: %d trees loaded\n", len(s.trees))

	return nil
}

// load discovers and loads all QML trees from the directory
func (s *Service) load() error {
	// Check if directory exists
	if _, err := os.Stat(s.dir); os.IsNotExist(err) {
		// Directory doesn't exist - not an error, just means no trees available
		s.mu.Lock()
		s.trees = make(map[string]*Tree)
		s.modTimes = make(map[string]time.Time)
		s.mu.Unlock()
		return nil
	}

	// Read directory entries
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	newTrees := make(map[string]*Tree)
	newModTimes := make(map[string]time.Time)

	// Look for subdirectories that match the pattern {version}-{device}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		path := filepath.Join(s.dir, name)

		// Create Tree object
		tree, err := NewTree(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[qmltree] Failed to load tree %s: %v\n", name, err)
			continue
		}

		newTrees[name] = tree

		// Store modification time
		info, err := entry.Info()
		if err == nil {
			newModTimes[path] = info.ModTime()
		}
	}

	// Update the service's tree map and modification times
	s.mu.Lock()
	s.trees = newTrees
	s.modTimes = newModTimes
	s.mu.Unlock()

	return nil
}

// Count returns the number of discovered trees
func (s *Service) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.trees)
}
