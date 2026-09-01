package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

type Info struct {
	Path   string `json:"path" jsonschema:"absolute local artifact path"`
	Size   int64  `json:"size" jsonschema:"artifact size in bytes"`
	SHA256 string `json:"sha256" jsonschema:"lowercase SHA-256 digest"`
}

func Inspect(path string) (Info, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Info{}, fmt.Errorf("read artifact %q: %w", path, err)
	}
	sum := sha256.Sum256(data)
	return Info{Path: path, Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}, nil
}
