package measurementdevice

import (
	"fmt"
	"path"
	"strconv"

	chantico "chantico/api/v1alpha1"
)

const (
	GenerationAnnotation      = "chantico-project.github.io/generation"
	GeneratorOutputAnnotation = "chantico-project.github.io/generator-output"
	ConfigHashAnnotation      = "chantico-project.github.io/config-hash"
	SnmpFragmentLabel         = "chantico-project.github.io/snmp-fragment"
)

// Subdirectories under the chantico data volume.
const (
	GeneratorSubDir       = "snmp/generators/"
	GeneratorOutputSubDir = "snmp/output/"
	MibsSubDir            = "snmp/mibs/"
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

// GeneratorOutputDir isolates a device's generated files on the shared volume.
func GeneratorOutputDir(device *chantico.MeasurementDevice) string {
	return path.Join(GeneratorOutputSubDir, string(device.UID))
}

// GeneratorOutputPath identifies the output for one device generation.
func GeneratorOutputPath(device *chantico.MeasurementDevice) string {
	return path.Join(GeneratorOutputDir(device), strconv.FormatInt(device.Generation, 10)+".yml")
}
