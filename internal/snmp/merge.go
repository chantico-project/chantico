package snmp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"sort"

	"go.yaml.in/yaml/v2"
)

// Merge merges multiple snmp_exporter config files into a single one.
// Inputs are expected to be the YAML content of per-device snmp.yml
// fragments produced by snmp-generator. Maps are merged shallowly;
// later entries win on key collision (callers should pre-sort their
// inputs deterministically — see MergeFiles).
func Merge(fragments [][]byte) ([]byte, error) {
	out := MergedConfig{
		Auths:   map[string]any{},
		Modules: map[string]any{},
	}
	for i, raw := range fragments {
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

// SortedFragments reads fragments in a deterministic order so Merge
// produces byte-stable output across reconciles.
func SortedFragments(filesByName map[string][]byte) [][]byte {
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
