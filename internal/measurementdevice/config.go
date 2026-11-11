package measurementdevice

import (
	chantico "chantico/api/v1alpha1"

	batchv1 "k8s.io/api/batch/v1"

	"chantico/internal/filestore"
	img "chantico/internal/images"
	vol "chantico/internal/volumes"
	"os"
	"path"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func BuildGeneratorJob(measurementDevice *chantico.MeasurementDevice) (*batchv1.Job, error) {
	volume, err := vol.GetChanticoVolume() // ugly?
	if err != nil {
		return nil, err
	}

	const podMountPath = "/data"
	fs := filestore.VolumeFileStore{Root: podMountPath}

	generatorPath := fs.Resolve(GeneratorFile(measurementDevice.GetUID()))
	mibsDir := fs.Resolve(MibsSubDir)
	outputPath := fs.Resolve(SnmpFile(measurementDevice.GetUID()))
	claim := volume.PersistentVolumeClaim.ClaimName
	backoffLimit := int32(0)
	uid := int64(os.Getuid())
	gid := int64(os.Getgid())
	runAsNonRoot := uid != 0

	mount := func(subDir string, readOnly bool) corev1.VolumeMount {
		return corev1.VolumeMount{
			Name:      vol.ChanticoVolumeMount,
			MountPath: fs.Resolve(subDir),
			SubPath:   path.Clean(subDir),
			ReadOnly:  readOnly,
		}
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      measurementDevice.GetName(),
			Namespace: measurementDevice.GetNamespace(),
			Annotations: map[string]string{
				GenerationAnnotation: strconv.FormatInt(measurementDevice.GetGeneration(), 10),
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{vol.DataVolumeLabel: claim},
				},
				Spec: corev1.PodSpec{
					Affinity: vol.DataVolumeAffinity(claim),
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &runAsNonRoot,
						RunAsUser:    &uid,
						RunAsGroup:   &gid,
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "snmp-generator",
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
								mount(MibsSubDir, true),
								mount(GeneratorsSubDir, true),
								mount(SnmpSubDir, false),
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
