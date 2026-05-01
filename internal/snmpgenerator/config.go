package snmpgenerator

import (
	chantico "chantico/api/v1alpha1"

	batchv1 "k8s.io/api/batch/v1"

	vol "chantico/internal/volumes"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func BuildGeneratorJob(snmpGeneratorImage string, snmpDevice *chantico.SNMPDevice) (*batchv1.Job, error) {
	volume, err := vol.GetChanticoVolume() // ugly?
	if err != nil {
		return nil, err
	}

	const podMountPath = "/data"
	podPath := NewPaths(podMountPath)

	generatorPath := podPath.GeneratorFile(snmpDevice.GetUID())
	mibsDir := podPath.MIBsDir()
	outputPath := podPath.SNMPFile(snmpDevice.GetUID())
	backoffLimit := int32(0)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      snmpDevice.GetName(),
			Namespace: snmpDevice.GetNamespace(),
			Annotations: map[string]string{
				GenerationAnnotation: strconv.FormatInt(snmpDevice.GetGeneration(), 10),
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "snmp-generator",
							Image: snmpGeneratorImage,
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
									MountPath: podMountPath,
								},
							},
						},
					},
					Volumes:       []corev1.Volume{volume},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
	}

	return job, nil
}
