package qmd

import (
	"fmt"
	"strconv"
	"strings"
)

// FindHashPositions searches a QMD file for specific hash IDs and returns their positions
// This is much more efficient than full parsing when we only need to find a few failed hashes
func FindHashPositions(qmdContent string, failedHashes []uint64) []HashWithPosition {
	if len(failedHashes) == 0 {
		return nil
	}

	// Build a set of hash strings to search for
	hashSet := make(map[string]uint64)
	for _, hash := range failedHashes {
		hashStr := strconv.FormatUint(hash, 10)
		hashSet[hashStr] = hash
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

		// Check for hashed value pattern: [[hash]]
		if ch == '[' && i+1 < len(qmdContent) && qmdContent[i+1] == '[' {
			if pos := findHashInPattern(qmdContent, i, "[[", "]]", hashSet, found, line, col); pos != nil {
				results = append(results, *pos)
				// Skip past the pattern we just found
				i = skipToEnd(qmdContent, i, "]]")
				col = 1 // Will be recalculated
				continue
			}
		}

		// Check for hash extension pattern: ~&hash&~
		if ch == '~' && i+1 < len(qmdContent) && qmdContent[i+1] == '&' {
			if pos := findHashInPattern(qmdContent, i, "~&", "&~", hashSet, found, line, col); pos != nil {
				results = append(results, *pos)
				// Skip past the pattern we just found
				i = skipToEnd(qmdContent, i, "&~")
				col = 1 // Will be recalculated
				continue
			}
		}

		col++
	}

	return results
}

// findHashInPattern searches for a hash within a specific pattern (e.g., [[hash]] or ~&hash&~)
func findHashInPattern(content string, startIdx int, openDelim, closeDelim string, hashSet map[string]uint64, found map[uint64]bool, line, col int) *HashWithPosition {
	// Skip the opening delimiter
	idx := startIdx + len(openDelim)
	if idx >= len(content) {
		return nil
	}

	// Extract the content between delimiters
	endIdx := strings.Index(content[idx:], closeDelim)
	if endIdx == -1 {
		return nil
	}

	hashStr := strings.TrimSpace(content[idx : idx+endIdx])

	// Check if this hash is in our search set
	if hashID, exists := hashSet[hashStr]; exists {
		// Check if we haven't already found this hash
		if !found[hashID] {
			found[hashID] = true
			return &HashWithPosition{
				Hash:   hashID,
				Line:   line,
				Column: col,
			}
		}
	}

	return nil
}

// skipToEnd skips to the end of a pattern
func skipToEnd(content string, startIdx int, endDelim string) int {
	idx := strings.Index(content[startIdx:], endDelim)
	if idx == -1 {
		return len(content) - 1
	}
	return startIdx + idx + len(endDelim) - 1
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
