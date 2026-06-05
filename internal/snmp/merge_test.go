package snmp

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"go.yaml.in/yaml/v2"
)

const yamlSNMPConfigFoo = `
auths:
  foo: { version: 3, username: guest }
modules:
  foo:
    walk: [1.3.6.1.4.1.31034.12.1.1.2.7.2]
`

const yamlSNMPConfigBar = `
auths:
  bar: { version: 3, username: guest }
modules:
  bar:
    walk: [1.3.6.1.4.1.31034.12.1.1.2.7.2.1.7]
`

func TestMerge(t *testing.T) {
	got, err := merge([][]byte{[]byte(yamlSNMPConfigFoo), []byte(yamlSNMPConfigBar)})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	var gotAny, wantAny any
	if err := yaml.Unmarshal(got, &gotAny); err != nil {
		t.Fatalf("unmarshal got: %v", err)
	}
	want := []byte(`
auths:
  foo: { version: 3, username: guest }
  bar: { version: 3, username: guest }
modules:
  foo: { walk: [1.3.6.1.4.1.31034.12.1.1.2.7.2] }
  bar: { walk: [1.3.6.1.4.1.31034.12.1.1.2.7.2.1.7] }
`)
	if err := yaml.Unmarshal(want, &wantAny); err != nil {
		t.Fatalf("unmarshal want: %v", err)
	}
	if !reflect.DeepEqual(gotAny, wantAny) {
		t.Fatalf("merge mismatch:\ngot:  %v\nwant: %v", gotAny, wantAny)
	}
}

func TestMergeSkipsEmpty(t *testing.T) {
	got, err := merge([][]byte{[]byte(yamlSNMPConfigFoo), {}, []byte("   \n")})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	var out MergedConfig
	if err := yaml.Unmarshal(got, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := out.Auths["foo"]; !ok {
		t.Fatalf("expected foo to be present, got %v", out.Auths)
	}
}

func TestSortedConfigsIsDeterministic(t *testing.T) {
	in := map[string][]byte{
		"snmp-c.yaml": []byte("c"),
		"snmp-a.yaml": []byte("a"),
		"snmp-b.yaml": []byte("b"),
	}
	got := sortedConfigs(in)
	want := [][]byte{[]byte("a"), []byte("b"), []byte("c")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sortedConfigs = %q, want %q", got, want)
	}
}

func TestGetMergedSortedSNMPConfig(t *testing.T) {
	dir := t.TempDir()

	// per-device files (only snmp-*.yaml are picked up)
	writeFile(t, filepath.Join(dir, "snmp-a.yaml"), []byte(yamlSNMPConfigFoo))
	writeFile(t, filepath.Join(dir, "snmp-b.yaml"), []byte(yamlSNMPConfigBar))
	// must be excluded
	writeFile(t, filepath.Join(dir, "snmp.yml"), []byte("ignored: true\n"))
	writeFile(t, filepath.Join(dir, "snmp-empty.yaml"), []byte(""))
	writeFile(t, filepath.Join(dir, "other.txt"), []byte("ignored"))

	merged, err := GetMergedSortedSNMPConfig(dir)
	if err != nil {
		t.Fatalf("GetMergedSortedSNMPConfig: %v", err)
	}

	var out MergedConfig
	if err := yaml.Unmarshal(merged, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := out.Auths["foo"]; !ok {
		t.Errorf("missing foo auth in merged output")
	}
	if _, ok := out.Auths["bar"]; !ok {
		t.Errorf("missing bar auth in merged output")
	}
	if _, ok := out.Auths["ignored"]; ok {
		t.Errorf("snmp.yml content leaked into merged output")
	}
}

func TestGetMergedSortedSNMPConfigMissingDir(t *testing.T) {
	merged, err := GetMergedSortedSNMPConfig(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("expected nil error for missing dir, got %v", err)
	}
	// merge of nothing should be a valid empty MergedConfig
	var out MergedConfig
	if err := yaml.Unmarshal(merged, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Auths) != 0 || len(out.Modules) != 0 {
		t.Fatalf("expected empty merge, got %v", out)
	}
}

func TestHashStable(t *testing.T) {
	a := Hash([]byte("foo"))
	b := Hash([]byte("foo"))
	c := Hash([]byte("bar"))
	if a != b {
		t.Errorf("Hash not stable: %s != %s", a, b)
	}
	if a == c {
		t.Errorf("Hash collision foo==bar")
	}
}

func writeFile(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.WriteFile(path, b, 0777); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
