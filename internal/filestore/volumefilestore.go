package filestore

import (
	"os"

	"path/filepath"

	"github.com/google/renameio"
)

type VolumeFileStore struct{}

func (VolumeFileStore) Write(filename string, bytes []byte, perm os.FileMode) error {
	dir := filepath.Dir(filename)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return renameio.WriteFile(filename, bytes, perm)
}

func (VolumeFileStore) Read(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}
