package measurementdevice

import (
	"fmt"

	"k8s.io/apimachinery/pkg/types"
)

const (
	GenerationAnnotation = "chantico-project.github.io/generation"
	ConfigHashAnnotation = "chantico-project.github.io/config-hash"
)

// Subdirectories under the chantico data volume.
const (
	GeneratorTemplate = "snmp/generators/generator-%s.yaml"

	SnmpSubDir     = "snmp/yml/"
	SnmpTemplate   = "snmp/yml/snmp-%s.yaml"
	SnmpMergedFile = "snmp/yml/snmp.yml"

	MibsSubDir = "snmp/mibs/"
)

func GeneratorFile(uid types.UID) string {
	return fmt.Sprintf(GeneratorTemplate, uid)
}
func SnmpFile(uid types.UID) string {
	return fmt.Sprintf(SnmpTemplate, uid)
}
