package measurementdevice

import (
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/types"
)

func TestPaths(t *testing.T) {
	p := NewPaths("/data")
	uid := types.UID("a-random-uid")

	cases := map[string]struct {
		got, want string
	}{
		"SNMPDir":        {p.SNMPDir(), "/data/snmp/yml"},
		"MIBsDir":        {p.MIBsDir(), "/data/snmp/mibs"},
		"MergedSNMPFile": {p.MergedSNMPFile(), "/data/snmp/yml/snmp.yml"},
		"GeneratorFile":  {p.GeneratorFile(uid), filepath.Join("/data/snmp/generators", "generator-a-random-uid.yaml")},
		"SNMPFile":       {p.SNMPFile(uid), filepath.Join("/data/snmp/yml", "snmp-a-random-uid.yaml")},
	}
	for name, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", name, c.got, c.want)
		}
	}
}
