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

// MergeSortedSNMPConfigs sorts the SNMP configs of the devices by name and returns the merged result.
func MergeSortedSNMPConfigs(configsByName map[string][]byte) ([]byte, error) {
	return merge(sortedConfigs(configsByName))
}
