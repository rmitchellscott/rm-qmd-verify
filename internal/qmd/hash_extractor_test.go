package qmd

import (
	"testing"
)

func TestExtractHashesWithPositions(t *testing.T) {
	qmdContent := `AFFECT /SomeFile.qml
    TRAVERSE Root [[12345678901234567890]]
    INSERT {
        id: ~&9876543210987654321&~
        text: "hello"
    }
`

	hashes, err := ExtractHashes(qmdContent)
	if err != nil {
		t.Fatalf("ExtractHashes failed: %v", err)
	}

	if len(hashes) != 2 {
		t.Fatalf("Expected 2 hashes, got %d", len(hashes))
	}

	for _, h := range hashes {
		if h.Line == 0 || h.Column == 0 {
			t.Errorf("Hash %d has invalid position: Line=%d, Column=%d", h.Hash, h.Line, h.Column)
		}
		t.Logf("Hash %d at Line %d, Column %d", h.Hash, h.Line, h.Column)
	}
}

func TestLineAndColumnTracking(t *testing.T) {
	qmdContent := `Line 1
Line 2 [[12345678901234567890]]
Line 3`

	hashes, err := ExtractHashes(qmdContent)
	if err != nil {
		t.Fatalf("ExtractHashes failed: %v", err)
	}

	if len(hashes) != 1 {
		t.Fatalf("Expected 1 hash, got %d", len(hashes))
	}

	if hashes[0].Line != 2 {
		t.Errorf("Expected hash on line 2, got line %d", hashes[0].Line)
	}

	if hashes[0].Column != 8 {
		t.Errorf("Expected hash at column 8, got column %d", hashes[0].Column)
	}

	t.Logf("Hash found at Line %d, Column %d (correct!)", hashes[0].Line, hashes[0].Column)
}
