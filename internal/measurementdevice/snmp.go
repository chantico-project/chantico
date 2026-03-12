package measurementdevice

import (
	"fmt"
	"log"
	"maps"
	"path/filepath"

	"go.yaml.in/yaml/v2"

	chantico "chantico/api/v1alpha1"
	img "chantico/internal/images"
	vol "chantico/internal/volumes"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	snmpDir       = "snmp"
	snmpYmlDir    = "snmp/yml"
	snmpConfigDir = "snmp/config"
	snmpMibsDir   = "snmp/mibs"
)

func MakeJob(measurementDevice chantico.MeasurementDevice) *batchv1.Job {
	volume, err := vol.GetChanticoVolume()
	if err != nil {
		log.Printf("ERR: %s\n", err)
	}

	configFileName := fmt.Sprintf("config_%s.yml", measurementDevice.UID)
	generatorFileName := fmt.Sprintf("generator_%s.yml", measurementDevice.UID)

	outputPath := filepath.Join("/data", snmpConfigDir, configFileName)
	generatorPath := filepath.Join("/data", snmpConfigDir, generatorFileName)
	mibsDir := filepath.Join("/data", snmpMibsDir)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      measurementDevice.Status.JobName,
			Namespace: measurementDevice.ObjectMeta.Namespace,
		},

		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "create-snmp-config",
							Image: img.SnmpGenerator,
							Command: []string{
								"/bin/generator",
							},
							Args: []string{
								"generate",
								"--output-path", outputPath,
								"--generator-path", generatorPath,
								"--mibs-dir", mibsDir,
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      vol.ChanticoVolumeMount,
									MountPath: "/data/",
								},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
					Volumes:       []corev1.Volume{volume},
				},
			},
		},
	}
	return job
}


type generatorModule struct {
	Walk []string `yaml:"walk"`
}

type snmpGeneratorConfig struct {
	Auths   map[string]chantico.Auth   `yaml:"auths"`
	Modules map[string]generatorModule `yaml:"modules"`
}

func GenerateSNMPGeneratorConfig(measurementDevice chantico.MeasurementDevice) (string, error) {
	modules := map[string]generatorModule{}
	modules[measurementDevice.Name] = generatorModule{Walk: measurementDevice.Spec.Walks}

	auths := map[string]chantico.Auth{}
	auths[measurementDevice.Name] = measurementDevice.Spec.Auth
	measurementDeviceSNMPConfig := snmpGeneratorConfig{Auths: auths, Modules: modules}

	out, err := yaml.Marshal(measurementDeviceSNMPConfig)
	return string(out), err
}

type snmpConfig struct {
	Auths   map[string]chantico.Auth `yaml:"auths"`
	Modules map[string]any           `yaml:"modules"`
}

func MergeSNMPConfigs(fileContents [][]byte) (string, error) {
	acc := snmpConfig{Auths: map[string]chantico.Auth{}, Modules: map[string]any{}}
	for _, fileContent := range fileContents {
		snmpconfig := snmpConfig{Auths: map[string]chantico.Auth{}, Modules: map[string]any{}}
		err := yaml.Unmarshal(fileContent, &snmpconfig)
		if err != nil {
			return "", err
		}
		maps.Copy(acc.Auths, snmpconfig.Auths)
		maps.Copy(acc.Modules, snmpconfig.Modules)
	}
	out, err := yaml.Marshal(acc)
	if err != nil {
		return "", err
	}
	return string(out), err
}
