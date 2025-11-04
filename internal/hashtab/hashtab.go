package hashtab

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rmitchellscott/rm-qmd-verify/internal/logging"
)

type Hashtab struct {
	Name      string
	Path      string
	OSVersion string
	Device    string
	Entries   map[uint64]string
}

func ParseVersion(filename string) (osVersion, device string) {
	parts := strings.Split(filename, "-")

	if len(parts) >= 2 {
		osVersion = parts[0]
		device = parts[1]
	} else if len(parts) == 1 {
		osVersion = parts[0]
		device = "unknown"
	} else {
		osVersion = filename
		device = "unknown"
	}

	return
}

func Load(path string) (*Hashtab, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open hashtab file: %w", err)
	}
	defer file.Close()

	entries := make(map[uint64]string)
	var hashtabVersion string

	for {
		var hash uint64
		err := binary.Read(file, binary.BigEndian, &hash)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read hash: %w", err)
		}

		var length uint32
		err = binary.Read(file, binary.BigEndian, &length)
		if err != nil {
			return nil, fmt.Errorf("failed to read length: %w", err)
		}

		data := make([]byte, length)
		_, err = io.ReadFull(file, data)
		if err != nil {
			return nil, fmt.Errorf("failed to read string data: %w", err)
		}

		str := string(data)

		if hash == 0 {
			logging.Info(logging.ComponentHashtab, "Found header: %s", str)
			continue
		} else if hash == 17607111715072197239 {
			hashtabVersion = str
			logging.Info(logging.ComponentHashtab, "Found version entry: %s", str)
		}

		entries[hash] = str
	}

	filename := filepath.Base(path)
	osVersion, device := ParseVersion(filename)

	if hashtabVersion != "" {
		osVersion = hashtabVersion
	}

	ht := &Hashtab{
		Name:      filename,
		Path:      path,
		OSVersion: osVersion,
		Device:    device,
		Entries:   entries,
	}

	return ht, nil
}

func DJB2Hash(s string) uint64 {
	hash := uint64(5481)
	for i := 0; i < len(s); i++ {
		hash = ((hash << 5) + hash) + uint64(s[i])
	}
	return hash
}
