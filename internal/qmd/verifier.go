package qmd

import (
	"sort"

	"github.com/rmitchellscott/rm-qmd-verify/pkg/hashtab"
)

type VerifyResult struct {
	Compatible    bool
	MissingHashes []HashWithPosition
}

type Verifier struct {
	hashtab *hashtab.Hashtab
}

func NewVerifier(ht *hashtab.Hashtab) *Verifier {
	return &Verifier{hashtab: ht}
}

func (v *Verifier) Verify(qmdContent string) (*VerifyResult, error) {
	parser := NewParser(qmdContent)
	hashes, err := parser.ExtractHashes()
	if err != nil {
		return nil, err
	}

	var missingHashes []HashWithPosition
	for _, hashPos := range hashes {
		if _, exists := v.hashtab.Entries[hashPos.Hash]; !exists {
			missingHashes = append(missingHashes, hashPos)
		}
	}

	sort.Slice(missingHashes, func(i, j int) bool {
		if missingHashes[i].Line != missingHashes[j].Line {
			return missingHashes[i].Line < missingHashes[j].Line
		}
		return missingHashes[i].Column < missingHashes[j].Column
	})

	result := &VerifyResult{
		Compatible:    len(missingHashes) == 0,
		MissingHashes: missingHashes,
	}

	return result, nil
}

func VerifyAgainstHashtab(qmdContent string, ht *hashtab.Hashtab) (*VerifyResult, error) {
	verifier := NewVerifier(ht)
	return verifier.Verify(qmdContent)
}

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
