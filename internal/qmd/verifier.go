package qmd

import (
	"github.com/rmitchellscott/rm-qmd-verify/pkg/hashtab"
)

type VerifyResult struct {
	Compatible    bool
	MissingHashes []uint64
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

	var missingHashes []uint64
	for _, hash := range hashes {
		if _, exists := v.hashtab.Entries[hash]; !exists {
			missingHashes = append(missingHashes, hash)
		}
	}

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
