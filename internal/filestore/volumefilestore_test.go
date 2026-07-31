package filestore

import (
	"bytes"
	_ "embed"
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
		perm     os.FileMode
		wantErr  bool
	}{
		{"simple write", "test.txt", []byte("Hello world!"), 0o666, false},
		{"empty file write", "test2.txt", []byte{}, 0o666, false},
		{"empty file write", "i/do/not/exist/test.txt", []byte{}, 0o666, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// TODO: construct the receiver type.
			var v VolumeFileStore
			gotErr := v.Write(filepath.Join(tempDir, tt.filename), tt.bytes, tt.perm)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("Write() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("Write() succeeded unexpectedly")
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
		wantErr  bool
	}{
		{"simple file content read", "testdata/mockfile.txt", []byte(mockFileContent), false},
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// TODO: construct the receiver type.
			var v VolumeFileStore
			got, gotErr := v.Read(tt.filename)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("Read() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("Read() succeeded unexpectedly")
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("Read() = %v, want %v", got, tt.want)
			}
		})
	}
}
