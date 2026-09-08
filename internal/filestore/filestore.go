package filestore

import (
	"context"
	"io"
)

type FileStore interface {
	Write(ctx context.Context, filepath string, r io.Reader) error
	Read(ctx context.Context, filepath string) (io.ReadCloser, error)
	ReadAll(ctx context.Context, filepath string) ([]byte, error)
	CollectSubFiles(ctx context.Context, path string) ([]string, error)
	Resolve(subpaths ...string) string
	Remove(ctx context.Context, filepath string) error
}
