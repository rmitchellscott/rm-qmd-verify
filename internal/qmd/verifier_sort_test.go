package qmd

import (
	"testing"

	"github.com/rmitchellscott/rm-qmd-verify/pkg/hashtab"
)

func TestVerifySortsByLineAndColumn(t *testing.T) {
	content := `Line 1
Line 2 [[9999999999999999999]]
Line 3 [[1111111111111111111]]
Line 4 [[2222222222222222222]]
Line 2 again [[8888888888888888888]]`

	ht := &hashtab.Hashtab{
		Name:    "test",
		Entries: make(map[uint64]string),
	}

	result, err := VerifyAgainstHashtab(content, ht)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if len(result.MissingHashes) != 4 {
		t.Fatalf("Expected 4 missing hashes, got %d", len(result.MissingHashes))
	}

	if result.MissingHashes[0].Line != 2 {
		t.Errorf("First hash should be on line 2, got line %d", result.MissingHashes[0].Line)
	}

	if result.MissingHashes[0].Column != 8 {
		t.Errorf("First hash should be at column 8, got column %d", result.MissingHashes[0].Column)
	}

	for i := 0; i < len(result.MissingHashes)-1; i++ {
		curr := result.MissingHashes[i]
		next := result.MissingHashes[i+1]

		if curr.Line > next.Line {
			t.Errorf("Hashes not sorted by line: hash %d (L%d:C%d) comes before hash %d (L%d:C%d)",
				curr.Hash, curr.Line, curr.Column, next.Hash, next.Line, next.Column)
		}

		if curr.Line == next.Line && curr.Column > next.Column {
			t.Errorf("Hashes on same line not sorted by column: hash %d (L%d:C%d) comes before hash %d (L%d:C%d)",
				curr.Hash, curr.Line, curr.Column, next.Hash, next.Line, next.Column)
		}
	}

	t.Logf("Hashes sorted correctly by line/column:")
	for _, h := range result.MissingHashes {
		t.Logf("  L%d:C%d %d", h.Line, h.Column, h.Hash)
	}
}
