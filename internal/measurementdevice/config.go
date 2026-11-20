package measurementdevice

import (
	chantico "chantico/api/v1alpha1"

	batchv1 "k8s.io/api/batch/v1"

	"chantico/internal/filestore"
	img "chantico/internal/images"
	vol "chantico/internal/volumes"
	"fmt"
	"os"
	"path"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// GeneratorJobBuilder constructs the desired SNMP generator Job without applying it.
type GeneratorJobBuilder interface {
	BuildJob(device *chantico.MeasurementDevice) (*batchv1.Job, error)
}

// SNMPGeneratorJobBuilder builds generator Jobs that use the shared volume.
type SNMPGeneratorJobBuilder struct{}

// BuildJob returns the desired Job for a measurement device.
func (SNMPGeneratorJobBuilder) BuildJob(device *chantico.MeasurementDevice) (*batchv1.Job, error) {
	return BuildGeneratorJob(device)
}

// BuildGeneratorJob builds a Job that writes SNMP configuration to the shared volume.
func BuildGeneratorJob(measurementDevice *chantico.MeasurementDevice) (*batchv1.Job, error) {
	if measurementDevice.UID == "" {
		return nil, fmt.Errorf("measurement device UID is required for generator output")
	}
	volume, err := vol.GetChanticoVolume() // ugly?
	if err != nil {
		return nil, err
	}

	const podMountPath = "/data"
	const generatorVolumeName = "generator-config"
	fs := filestore.VolumeFileStore{Root: podMountPath}

	generatorDir := fs.Resolve(GeneratorSubDir)
	generatorPath := path.Join(generatorDir, GeneratorConfigKey)
	mibsDir := fs.Resolve(MibsSubDir)
	outputPath := fs.Resolve(GeneratorOutputPath(measurementDevice))
	script := `set -eu
umask 077
mkdir -p "$1"
shift
exec /bin/generator "$@"`
	claim := volume.PersistentVolumeClaim.ClaimName
	backoffLimit := int32(3)
	uid := int64(os.Getuid())
	gid := int64(os.Getgid())
	runAsNonRoot := uid != 0

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      measurementDevice.GetName(),
			Namespace: measurementDevice.GetNamespace(),
			Annotations: map[string]string{
				GenerationAnnotation:      strconv.FormatInt(measurementDevice.GetGeneration(), 10),
				GeneratorOutputAnnotation: GeneratorOutputPath(measurementDevice),
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{vol.DataVolumeLabel: claim},
				},
				Spec: corev1.PodSpec{
					AutomountServiceAccountToken: ptr.To(false),
					Affinity:                     vol.DataVolumeAffinity(claim),
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &runAsNonRoot,
						RunAsUser:    &uid,
						RunAsGroup:   &gid,
						FSGroup:      &gid,
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "snmp-generator",
							Image: img.SnmpGenerator,
							Command: []string{
								"/bin/sh", "-c", script, "generator",
							},
							Args: []string{
								path.Dir(outputPath),
								"generate",
								"--output-path", outputPath,
								"--generator-path", generatorPath,
								"--mibs-dir", mibsDir,
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      vol.ChanticoVolumeMount,
									MountPath: mibsDir,
									SubPath:   path.Clean(MibsSubDir),
									ReadOnly:  true,
								},
								{
									Name:      generatorVolumeName,
									MountPath: generatorDir,
									ReadOnly:  true,
								},
								{
									Name:      vol.ChanticoVolumeMount,
									MountPath: fs.Resolve(GeneratorOutputSubDir),
									SubPath:   path.Clean(GeneratorOutputSubDir),
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						volume,
						{
							Name: generatorVolumeName,
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: GeneratorConfigMapName(measurementDevice.GetName())},
								},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
	}

	return job, nil
}
