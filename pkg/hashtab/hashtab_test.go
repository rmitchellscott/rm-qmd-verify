package hashtab

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestIsHashlist(t *testing.T) {
	tests := []struct {
		name    string
		hashtab *Hashtab
		want    bool
	}{
		{
			name: "all empty strings - is hashlist",
			hashtab: &Hashtab{
				Entries: map[uint64]string{
					123: "",
					456: "",
					789: "",
				},
			},
			want: true,
		},
		{
			name: "some non-empty strings - not hashlist",
			hashtab: &Hashtab{
				Entries: map[uint64]string{
					123: "property1",
					456: "",
					789: "property3",
				},
			},
			want: false,
		},
		{
			name: "all non-empty strings - not hashlist",
			hashtab: &Hashtab{
				Entries: map[uint64]string{
					123: "property1",
					456: "property2",
				},
			},
			want: false,
		},
		{
			name: "empty hashtab - is hashlist",
			hashtab: &Hashtab{
				Entries: map[uint64]string{},
			},
			want: true,
		},
		{
			name: "single entry with empty string - is hashlist",
			hashtab: &Hashtab{
				Entries: map[uint64]string{
					123: "",
				},
			},
			want: true,
		},
		{
			name: "version hash with empty string - is hashlist",
			hashtab: &Hashtab{
				Entries: map[uint64]string{
					17607111715072197239: "",
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.hashtab.IsHashlist()
			if got != tt.want {
				t.Errorf("IsHashlist() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWriteHashlist(t *testing.T) {
	tests := []struct {
		name    string
		hashes  []uint64
		wantErr bool
		wantLen int64
	}{
		{
			name:    "write multiple hashes",
			hashes:  []uint64{123, 456, 789},
			wantErr: false,
			wantLen: 3 * 12, // 3 hashes × 12 bytes each
		},
		{
			name:    "write single hash",
			hashes:  []uint64{123},
			wantErr: false,
			wantLen: 12,
		},
		{
			name:    "write empty slice",
			hashes:  []uint64{},
			wantErr: false,
			wantLen: 0,
		},
		{
			name:    "write many hashes",
			hashes:  []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			wantErr: false,
			wantLen: 10 * 12,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			outputPath := filepath.Join(tmpDir, "test.bin")

			err := WriteHashlist(tt.hashes, outputPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("WriteHashlist() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				stat, err := os.Stat(outputPath)
				if err != nil {
					t.Fatalf("Failed to stat output file: %v", err)
				}

				if stat.Size() != tt.wantLen {
					t.Errorf("File size = %d bytes, want %d bytes", stat.Size(), tt.wantLen)
				}

				file, err := os.Open(outputPath)
				if err != nil {
					t.Fatalf("Failed to open output file: %v", err)
				}
				defer file.Close()

				for i, expectedHash := range tt.hashes {
					var hash uint64
					if err := binary.Read(file, binary.BigEndian, &hash); err != nil {
						t.Fatalf("Failed to read hash %d: %v", i, err)
					}
					if hash != expectedHash {
						t.Errorf("Hash %d = %d, want %d", i, hash, expectedHash)
					}

					var length uint32
					if err := binary.Read(file, binary.BigEndian, &length); err != nil {
						t.Fatalf("Failed to read length %d: %v", i, err)
					}
					if length != 0 {
						t.Errorf("Length %d = %d, want 0", i, length)
					}
				}
			}
		})
	}
}

func TestWriteHashlistInvalidPath(t *testing.T) {
	err := WriteHashlist([]uint64{123}, "/invalid/path/that/does/not/exist/test.bin")
	if err == nil {
		t.Error("WriteHashlist() with invalid path should return error")
	}
}

func TestHashlistRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()

	originalHashes := []uint64{123, 456, 789, 17607111715072197239}
	hashlistPath := filepath.Join(tmpDir, "test-hashlist.bin")

	err := WriteHashlist(originalHashes, hashlistPath)
	if err != nil {
		t.Fatalf("WriteHashlist() failed: %v", err)
	}

	ht, err := Load(hashlistPath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if len(ht.Entries) != len(originalHashes) {
		t.Errorf("Loaded %d entries, want %d", len(ht.Entries), len(originalHashes))
	}

	for _, hash := range originalHashes {
		if _, exists := ht.Entries[hash]; !exists {
			t.Errorf("Hash %d not found in loaded hashtab", hash)
		}
	}

	if !ht.IsHashlist() {
		t.Error("Loaded hashtab should be detected as hashlist")
	}
}

func TestHashlistPreservesVersionHash(t *testing.T) {
	tmpDir := t.TempDir()

	versionHash := uint64(17607111715072197239)
	hashes := []uint64{123, 456, versionHash, 789}
	hashlistPath := filepath.Join(tmpDir, "test-with-version.bin")

	err := WriteHashlist(hashes, hashlistPath)
	if err != nil {
		t.Fatalf("WriteHashlist() failed: %v", err)
	}

	ht, err := Load(hashlistPath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if _, exists := ht.Entries[versionHash]; !exists {
		t.Error("Version hash was not preserved in conversion")
	}
}

func TestDJB2Hash(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  uint64
	}{
		{
			name:  "empty string",
			input: "",
			want:  5481,
		},
		{
			name:  "single character",
			input: "a",
			want:  180970,
		},
		{
			name:  "simple string",
			input: "test",
			want:  6504315593,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DJB2Hash(tt.input)
			if got != tt.want {
				t.Errorf("DJB2Hash(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name           string
		filename       string
		wantOSVersion  string
		wantDevice     string
	}{
		{
			name:           "standard format",
			filename:       "3.22.4.2-rmpp",
			wantOSVersion:  "3.22.4.2",
			wantDevice:     "rmpp",
		},
		{
			name:           "no device",
			filename:       "3.22.4.2",
			wantOSVersion:  "3.22.4.2",
			wantDevice:     "unknown",
		},
		{
			name:           "empty string",
			filename:       "",
			wantOSVersion:  "",
			wantDevice:     "unknown",
		},
		{
			name:           "multiple dashes",
			filename:       "3.22.4.2-rmpp-extra",
			wantOSVersion:  "3.22.4.2",
			wantDevice:     "rmpp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOSVersion, gotDevice := ParseVersion(tt.filename)
			if gotOSVersion != tt.wantOSVersion {
				t.Errorf("ParseVersion() osVersion = %v, want %v", gotOSVersion, tt.wantOSVersion)
			}
			if gotDevice != tt.wantDevice {
				t.Errorf("ParseVersion() device = %v, want %v", gotDevice, tt.wantDevice)
			}
		})
	}
}
