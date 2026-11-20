package measurementdevice

import (
	"chantico/internal/filestore"
	"testing"
)

func TestPaths(t *testing.T) {
	filestore := filestore.VolumeFileStore{Root: "/data"}

	cases := map[string]struct {
		got, want string
	}{
		"MIBsDir":      {filestore.Resolve(MibsSubDir), "/data/snmp/mibs"},
		"GeneratorDir": {filestore.Resolve(GeneratorSubDir), "/data/snmp/generators"},
		"GeneratorMap": {GeneratorConfigMapName("test"), "test-generator"},
		"FragmentMap":  {SnmpFragmentConfigMapName("test"), "test-snmp"},
	}
	for name, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", name, c.got, c.want)
		}
	}
}
