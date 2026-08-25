package filestore

import (
	"context"
	"io"
	"os"
	fp "path/filepath"

	"github.com/google/renameio"
)

type VolumeFileStore struct {
	Root string
}

func (v VolumeFileStore) Write(ctx context.Context, filepath string, r io.Reader) error {
	dir := fp.Dir(fp.Join(v.Root, filepath))
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	err = renameio.WriteFile(filepath, data, 0644)
	return err
}

func (v VolumeFileStore) Read(ctx context.Context, filepath string) (io.ReadCloser, error) {
	return os.Open(fp.Join(v.Root, filepath))
}

func (v VolumeFileStore) ReadAll(ctx context.Context, filepath string) ([]byte, error) {
	rc, err := v.Read(ctx, filepath)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func (v VolumeFileStore) CollectSubFiles(ctx context.Context, filepath string) ([]string, error) {
	dirEntries, err := os.ReadDir(fp.Join(v.Root, filepath))
	subPaths := []string{}
	for _, entry := range dirEntries {
		if entry.IsDir() {
			continue
		}
		subPaths = append(subPaths, entry.Name())
	}
	return subPaths, err
}
