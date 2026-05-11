package measurementdevice

import (
	"fmt"
	"path/filepath"

	"k8s.io/apimachinery/pkg/types"
)

const (
	GenerationAnnotation = "chantico.ci.tno.nl/generation"
	ConfigHashAnnotation = "chantico.ci.tno.nl/config-hash"
)

// Subdirectories under the chantico data volume.
const (
	generatorsSubdir = "snmp/generators"
	snmpSubdir       = "snmp/yml"
	mibsSubdir       = "snmp/mibs"
	mergedSNMPFile   = "snmp.yml" // sits inside SNMPDir alongside per-device files
)

// This helper struct defines the read/write paths used by the SNMPGenerator controller.
type Paths struct {
	Root string
}

func NewPaths(root string) Paths { return Paths{Root: root} }

func (l Paths) generatorsDir() string  { return filepath.Join(l.Root, generatorsSubdir) }
func (l Paths) SNMPDir() string        { return filepath.Join(l.Root, snmpSubdir) }
func (l Paths) MIBsDir() string        { return filepath.Join(l.Root, mibsSubdir) }
func (l Paths) MergedSNMPFile() string { return filepath.Join(l.SNMPDir(), mergedSNMPFile) }

func (l Paths) GeneratorFile(uid types.UID) string {
	return filepath.Join(l.generatorsDir(), fmt.Sprintf("generator-%s.yaml", uid))
}
func (l Paths) SNMPFile(uid types.UID) string {
	return filepath.Join(l.SNMPDir(), fmt.Sprintf("snmp-%s.yaml", uid))
}
