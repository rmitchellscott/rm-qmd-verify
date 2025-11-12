package qmd

import (
	"fmt"
	"strconv"
)

// FindHashPositions searches a QMD file for specific hash IDs and returns their positions
// Just searches for the hash ID as a decimal string anywhere in the file
func FindHashPositions(qmdContent string, failedHashes []uint64) []HashWithPosition {
	if len(failedHashes) == 0 {
		return nil
	}

	// Build a map of hash strings to search for
	hashStrings := make(map[string]uint64)
	for _, hash := range failedHashes {
		hashStr := strconv.FormatUint(hash, 10)
		hashStrings[hashStr] = hash
	}

	results := make([]HashWithPosition, 0, len(failedHashes))
	found := make(map[uint64]bool)

	line := 1
	col := 1

	// Scan through the content character by character
	for i := 0; i < len(qmdContent); i++ {
		ch := qmdContent[i]

		// Track line and column
		if ch == '\n' {
			line++
			col = 1
			continue
		}

		// Check if any hash string starts at this position
		for hashStr, hashID := range hashStrings {
			// Skip if already found
			if found[hashID] {
				continue
			}

			// Check if we have enough characters left
			if i+len(hashStr) > len(qmdContent) {
				continue
			}

			// Check if the hash string matches at this position
			if qmdContent[i:i+len(hashStr)] == hashStr {
				found[hashID] = true
				results = append(results, HashWithPosition{
					Hash:   hashID,
					Line:   line,
					Column: col,
				})
				// Don't break - continue checking other hashes
			}
		}

		col++
	}

	return results
}

// FindHashPosition is a convenience function to find a single hash position
func FindHashPosition(qmdContent string, hash uint64) *HashWithPosition {
	positions := FindHashPositions(qmdContent, []uint64{hash})
	if len(positions) > 0 {
		return &positions[0]
	}
	return nil
}

// FormatHashError formats a hash error with its position for display
func FormatHashError(hash uint64, line, column int) string {
	return fmt.Sprintf("Cannot resolve hash %d at line %d, column %d", hash, line, column)
}
