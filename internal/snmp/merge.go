package snmp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v2"
)

func merge(configs [][]byte) ([]byte, error) {
	out := MergedConfig{
		Auths:   map[string]any{},
		Modules: map[string]any{},
	}
	for i, raw := range configs {
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		var frag MergedConfig
		if err := yaml.Unmarshal(raw, &frag); err != nil {
			return nil, fmt.Errorf("fragment %d: %w", i, err)
		}
		maps.Copy(out.Auths, frag.Auths)
		maps.Copy(out.Modules, frag.Modules)
	}
	return yaml.Marshal(out)
}

// Hash returns a stable hex-encoded SHA-256 of content.
func Hash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func sortedConfigs(filesByName map[string][]byte) [][]byte {
	names := make([]string, 0, len(filesByName))
	for n := range filesByName {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([][]byte, 0, len(names))
	for _, n := range names {
		out = append(out, filesByName[n])
	}
	return out
}

// readPerDeviceSnmpConfigs reads every snmp-*.yaml in the SNMP dir and returns them keyed by filename and sorted.
func readPerDeviceSnmpConfigs(dir string) (map[string][]byte, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("list %s: %w", dir, err)
	}
	out := map[string][]byte{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		// Only per-device files: snmp-<uid>.yaml. Excludes the merged snmp.yml.
		if !strings.HasPrefix(name, "snmp-") || filepath.Ext(name) != ".yaml" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		if len(bytes.TrimSpace(content)) == 0 {
			continue // empty placeholder before generator job runs
		}
		out[name] = content
	}
	return out, nil
}

func GetMergedSortedSNMPConfig(dir string) ([]byte, error) {
	filesByName, err := readPerDeviceSnmpConfigs(dir)
	if err != nil {
		return nil, err
	}
	return merge(sortedConfigs(filesByName))
}
