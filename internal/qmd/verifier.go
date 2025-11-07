package qmd

import (
	"sort"

	"github.com/rmitchellscott/rm-qmd-verify/pkg/hashtab"
)

type VerifyResult struct {
	Compatible    bool
	MissingHashes []HashWithPosition
}

// VerifyWithHashes verifies that all hashes exist in the hashtable
// This is kept for backwards compatibility with the existing hash-only verification mode
func VerifyWithHashes(hashes []HashWithPosition, ht *hashtab.Hashtab) *VerifyResult {
	var missingHashes []HashWithPosition
	for _, hashPos := range hashes {
		if _, exists := ht.Entries[hashPos.Hash]; !exists {
			missingHashes = append(missingHashes, hashPos)
		}
	}

	sort.Slice(missingHashes, func(i, j int) bool {
		if missingHashes[i].Line != missingHashes[j].Line {
			return missingHashes[i].Line < missingHashes[j].Line
		}
		return missingHashes[i].Column < missingHashes[j].Column
	})

	return &VerifyResult{
		Compatible:    len(missingHashes) == 0,
		MissingHashes: missingHashes,
	}
}
