package measurementdevice

import "fmt"

const (
	GenerationAnnotation = "chantico-project.github.io/generation"
	ConfigHashAnnotation = "chantico-project.github.io/config-hash"
	SnmpFragmentLabel    = "chantico-project.github.io/snmp-fragment"
)

// Subdirectories under the chantico data volume.
const (
	GeneratorSubDir = "snmp/generators/"
	MibsSubDir      = "snmp/mibs/"
)

// ConfigMap names and keys. The merged ConfigMap lives in the namespace of the controller.
const (
	GeneratorTemplate = "%s-generator"
	SnmpTemplate      = "%s-snmp"

	MergedConfigMapName = "chantico-snmp-config"

	GeneratorConfigKey = "generator.yml"
	SnmpConfigKey      = "snmp.yml"
)

func GeneratorConfigMapName(deviceName string) string {
	return fmt.Sprintf(GeneratorTemplate, deviceName)
}
func SnmpFragmentConfigMapName(deviceName string) string {
	return fmt.Sprintf(SnmpTemplate, deviceName)
}
