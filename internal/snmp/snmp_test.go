package snmp

import (
	"reflect"
	"testing"

	"go.yaml.in/yaml/v2"
)

func TestGeneratorConfigYAML(t *testing.T) {
	cfg := GeneratorConfig{
		Auths:   map[string]*GeneratorAuth{"test": {Version: 3, Username: "guest"}},
		Modules: map[string]*GeneratorModule{"test": {Walk: []string{"foo", "bar"}}},
	}

	got, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var roundtrip any
	if err := yaml.Unmarshal(got, &roundtrip); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	expected := []byte(`
auths:
  test:
    username: guest
    version: 3
modules:
  test:
    walk:
    - foo
    - bar
`)
	var want any
	if err := yaml.Unmarshal(expected, &want); err != nil {
		t.Fatalf("unmarshal expected: %v", err)
	}
	if !reflect.DeepEqual(roundtrip, want) {
		t.Fatalf("yaml mismatch:\ngot:  %v\nwant: %v", roundtrip, want)
	}
}
