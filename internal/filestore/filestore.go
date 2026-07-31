package filestore

import "os"

type FileStore interface {
	Write(filename string, data []byte, perm os.FileMode) error
	Read(filename string) ([]byte, error)
}
