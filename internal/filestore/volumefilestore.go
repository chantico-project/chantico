package filestore

import (
	"context"
	"io"
	"io/fs"
	"os"
	fp "path/filepath"

	"github.com/google/renameio"
)

type VolumeFileStore struct {
	Root string
}

func (v VolumeFileStore) Write(ctx context.Context, filepath string, r io.Reader) error {
	dir := fp.Dir(v.Resolve(filepath))
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	err = renameio.WriteFile(v.Resolve(filepath), data, 0644)
	return err
}

func (v VolumeFileStore) Read(ctx context.Context, filepath string) (io.ReadCloser, error) {
	return os.Open(v.Resolve(filepath))
}

func (v VolumeFileStore) ReadAll(ctx context.Context, filepath string) ([]byte, error) {
	rc, err := v.Read(ctx, filepath)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func (v VolumeFileStore) CollectSubFiles(ctx context.Context, path string) ([]string, error) {
	subPaths := []string{}

	err := fp.WalkDir(v.Resolve(path), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// honor context cancellation
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			return nil
		}
		rel, err := fp.Rel(v.Root, path)
		if err != nil {
			return err
		}
		subPaths = append(subPaths, rel)
		return nil
	})

	return subPaths, err
}

func (v VolumeFileStore) Resolve(subpaths ...string) string {
	return fp.Join(append([]string{v.Root}, subpaths...)...)
}

func (v VolumeFileStore) Remove(ctx context.Context, filepath string) error {
	return os.Remove(v.Resolve(filepath))
}
