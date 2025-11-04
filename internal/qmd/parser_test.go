package qmd

import (
	"testing"
)

func TestExtractHashes_ValidDiffHashes(t *testing.T) {
	content := `
AFFECT /SomeFile.qml
	TRAVERSE Root [[12345678901234567890]]
	TRAVERSE Child [[9876543210987654321]]
`
	parser := NewParser(content)
	hashes, err := parser.ExtractHashes()
	if err != nil {
		t.Fatalf("ExtractHashes failed: %v", err)
	}

	expected := map[uint64]bool{
		12345678901234567890: true,
		9876543210987654321:  true,
	}

	if len(hashes) != len(expected) {
		t.Errorf("Expected %d hashes, got %d", len(expected), len(hashes))
	}

	for _, hash := range hashes {
		if !expected[hash] {
			t.Errorf("Unexpected hash: %d", hash)
		}
	}
}

func TestExtractHashes_ValidQMLHashes(t *testing.T) {
	content := `
AFFECT /SomeFile.qml
	INSERT {
		id: ~&12345678901234567890&~
		text: ~&"9876543210987654321&~
	}
`
	parser := NewParser(content)
	hashes, err := parser.ExtractHashes()
	if err != nil {
		t.Fatalf("ExtractHashes failed: %v", err)
	}

	expected := map[uint64]bool{
		12345678901234567890: true,
		9876543210987654321:  true,
	}

	if len(hashes) != len(expected) {
		t.Errorf("Expected %d hashes, got %d", len(expected), len(hashes))
	}

	for _, hash := range hashes {
		if !expected[hash] {
			t.Errorf("Unexpected hash: %d", hash)
		}
	}
}

func TestExtractHashes_JavaScriptArrayNotHash(t *testing.T) {
	content := `
AFFECT /SomeFile.qml
	INSERT {
		var result = array[[5]];
		var nested = matrix[[0]][[1]];
	}
`
	parser := NewParser(content)
	hashes, err := parser.ExtractHashes()
	if err != nil {
		t.Fatalf("ExtractHashes failed: %v", err)
	}

	if len(hashes) != 0 {
		t.Errorf("Expected 0 hashes (no false positives), got %d: %v", len(hashes), hashes)
	}
}

func TestExtractHashes_QuotedDiffHashes(t *testing.T) {
	content := `
AFFECT /SomeFile.qml
	TRAVERSE Root [['12345678901234567890]]
	TRAVERSE Child [["9876543210987654321]]
`
	parser := NewParser(content)
	hashes, err := parser.ExtractHashes()
	if err != nil {
		t.Fatalf("ExtractHashes failed: %v", err)
	}

	expected := map[uint64]bool{
		12345678901234567890: true,
		9876543210987654321:  true,
	}

	if len(hashes) != len(expected) {
		t.Errorf("Expected %d hashes, got %d", len(expected), len(hashes))
	}

	for _, hash := range hashes {
		if !expected[hash] {
			t.Errorf("Unexpected hash: %d", hash)
		}
	}
}

func TestExtractHashes_MixedHashesAndArrays(t *testing.T) {
	content := `
AFFECT /SomeFile.qml
	TRAVERSE Root [[12345678901234567890]]
	INSERT {
		var arr = items[[0]];
		id: ~&9876543210987654321&~
		var nested = matrix[[1]][[2]];
	}
`
	parser := NewParser(content)
	hashes, err := parser.ExtractHashes()
	if err != nil {
		t.Fatalf("ExtractHashes failed: %v", err)
	}

	expected := map[uint64]bool{
		12345678901234567890: true,
		9876543210987654321:  true,
	}

	if len(hashes) != len(expected) {
		t.Errorf("Expected %d hashes, got %d", len(expected), len(hashes))
	}

	for _, hash := range hashes {
		if !expected[hash] {
			t.Errorf("Unexpected hash: %d", hash)
		}
	}
}

func TestExtractHashes_EmptyContent(t *testing.T) {
	content := ``
	parser := NewParser(content)
	hashes, err := parser.ExtractHashes()
	if err != nil {
		t.Fatalf("ExtractHashes failed: %v", err)
	}

	if len(hashes) != 0 {
		t.Errorf("Expected 0 hashes for empty content, got %d", len(hashes))
	}
}
