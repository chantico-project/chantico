package filestore

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
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
		{"uncreatable dir write", "idonothavethewrite/test.txt", []byte{}, nil},
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

func TestVolumeFileStore_ReadAll(t *testing.T) {
	var v VolumeFileStore
	ctx := context.Background()

	t.Run("reads full file content", func(t *testing.T) {
		got, err := v.ReadAll(ctx, "testdata/mockfile.txt")
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		if !bytes.Equal(got, []byte(mockFileContent)) {
			t.Fatalf("ReadAll() = %v, want %v", got, []byte(mockFileContent))
		}
	})

	t.Run("returns error for missing file", func(t *testing.T) {
		_, err := v.ReadAll(ctx, "does-not-exist.txt")
		if err == nil {
			t.Fatalf("ReadAll() expected error for missing file, got nil")
		}
	})
}

func TestVolumeFileStore_CollectSubFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "vfs-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create directory tree:
	// tmpDir/
	//   dir1/
	//     a.txt
	//     sub/
	//       b.txt
	//   c.txt
	if err := os.MkdirAll(filepath.Join(tmpDir, "dir1", "sub"), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "dir1", "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatalf("write a.txt failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "dir1", "sub", "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatalf("write b.txt failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "c.txt"), []byte("c"), 0o644); err != nil {
		t.Fatalf("write c.txt failed: %v", err)
	}

	v := VolumeFileStore{Root: tmpDir}

	got, err := v.CollectSubFiles(context.Background(), "")
	if err != nil {
		t.Fatalf("CollectSubFiles returned error: %v", err)
	}

	sort.Strings(got)

	expected := []string{
		filepath.Join("dir1", "a.txt"),
		filepath.Join("dir1", "sub", "b.txt"),
		filepath.Join("c.txt"),
	}
	sort.Strings(expected)

	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("unexpected files:\n got: %v\nwant: %v", got, expected)
	}
}

func TestVolumeFileStore_Remove(t *testing.T) {
	dir := t.TempDir()

	v := VolumeFileStore{Root: dir}

	v.Write(context.Background(), "testfile.txt", bytes.NewReader([]byte("test content")))

	err := v.Remove(context.Background(), "testfile.txt")
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	err = v.Remove(context.Background(), "othertest.txt")
	if err == nil {
		t.Fatalf("Remove() should error")
	}
}
