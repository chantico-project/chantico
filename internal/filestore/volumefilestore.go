package filestore

import (
	"os"

	"github.com/google/renameio"
)

type VolumeFileStore struct{}

func (_ VolumeFileStore) Write(filename string, bytes []byte, perm os.FileMode) error {
	return renameio.WriteFile(filename, bytes, perm)
}

func (_ VolumeFileStore) Read(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}
