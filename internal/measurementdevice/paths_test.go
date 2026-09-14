package measurementdevice

import (
	"chantico/internal/filestore"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/types"
)

func TestPaths(t *testing.T) {
	uid := types.UID("a-random-uid")
	filestore := filestore.VolumeFileStore{Root: "/data"}

	cases := map[string]struct {
		got, want string
	}{
		"MIBsDir":        {filestore.Resolve(MibsSubDir), "/data/snmp/mibs"},
		"MergedSNMPFile": {filestore.Resolve(SnmpMergedFile), "/data/snmp/yml/snmp.yml"},
		"GeneratorFile":  {filestore.Resolve(GeneratorFile(uid)), filepath.Join("/data/snmp/generators", "generator-a-random-uid.yaml")},
		"SNMPFile":       {filestore.Resolve(SnmpFile(uid)), filepath.Join("/data/snmp/yml", "snmp-a-random-uid.yaml")},
	}
	for name, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", name, c.got, c.want)
		}
	}
}
