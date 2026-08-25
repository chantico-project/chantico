package filestore

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestVolumeFileStore_Write(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		filename string
		bytes    []byte
		wantErr  error
	}{
		{"simple write", "test.txt", []byte("Hello world!"), nil},
		{"empty file write", "test2.txt", []byte{}, nil},
		{"nested dir write", "i/do/not/exist/test.txt", []byte{}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v VolumeFileStore
			ctx := context.Background()
			target := filepath.Join(tempDir, tt.filename)
			gotErr := v.Write(ctx, target, bytes.NewReader(tt.bytes))
			if !errors.Is(gotErr, tt.wantErr) {
				t.Fatalf("Write() error = %v, want %v", gotErr, tt.wantErr)
			}
			if gotErr != nil {
				return
			}

			// Verify file content on disk.
			got, err := os.ReadFile(target)
			if err != nil {
				t.Fatalf("reading written file failed: %v", err)
			}
			if !bytes.Equal(got, tt.bytes) {
				t.Errorf("written content = %v, want %v", got, tt.bytes)
			}
		})
	}
}

//go:embed testdata/mockfile.txt
var mockFileContent string

func TestVolumeFileStore_Read(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		filename string
		want     []byte
		wantErr  error
	}{
		{"simple file content read", "testdata/mockfile.txt", []byte(mockFileContent), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v VolumeFileStore
			ctx := context.Background()
			rc, gotErr := v.Read(ctx, tt.filename)
			if !errors.Is(gotErr, tt.wantErr) {
				t.Fatalf("got error %v, want %v", gotErr, tt.wantErr)
			}
			if gotErr != nil {
				return
			}
			defer rc.Close()
			got, err := io.ReadAll(rc)
			if err != nil {
				t.Fatalf("reading from Read() returned reader failed: %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("Read() = %v, want %v", got, tt.want)
			}
		})
	}
}
