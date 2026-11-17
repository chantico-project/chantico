package measurementdevice

import (
	chantico "chantico/api/v1alpha1"

	batchv1 "k8s.io/api/batch/v1"

	"chantico/internal/filestore"
	img "chantico/internal/images"
	vol "chantico/internal/volumes"
	"context"
	"fmt"
	"os"
	"path"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// GeneratorJobCreator creates the SNMP generator Job owned by a measurement device.
type GeneratorJobBuilder interface {
	CreateJob(ctx context.Context, device *chantico.MeasurementDevice) (*batchv1.Job, error)
}

// ClientGeneratorJobBuilder builds and creates generator Jobs through the Kubernetes API.
type ClientGeneratorJobBuilder struct {
	Client client.Client
	Scheme *runtime.Scheme
}

func (builder ClientGeneratorJobBuilder) CreateJob(ctx context.Context, device *chantico.MeasurementDevice) (*batchv1.Job, error) {
	job, err := BuildGeneratorJob(device)
	if err != nil {
		return nil, fmt.Errorf("build SNMP Generator job: %w", err)
	}
	if err := controllerutil.SetControllerReference(device, job, builder.Scheme); err != nil {
		return nil, fmt.Errorf("set controller reference for SNMP Generator job: %w", err)
	}
	if err := builder.Client.Create(ctx, job); err != nil {
		return nil, fmt.Errorf("create SNMP Generator job: %w", err)
	}
	return job, nil
}

// BuildGeneratorJob builds the Job that generates the SNMP configuration of a device. The
// controller reads the configuration from the log of the Job's pod.
func BuildGeneratorJob(measurementDevice *chantico.MeasurementDevice) (*batchv1.Job, error) {
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
	outputPath := "/tmp/snmp.yml"
	// The generated config is the only output, so the controller can read it from the pod log.
	// The generator's own log is only shown when it fails.
	script := `set -eu
if ! /bin/generator "$@" > /tmp/generator.log 2>&1; then
  cat /tmp/generator.log >&2
  exit 1
fi
cat ` + outputPath
	claim := volume.PersistentVolumeClaim.ClaimName
	backoffLimit := int32(0)
	uid := int64(os.Getuid())
	gid := int64(os.Getgid())
	runAsNonRoot := uid != 0

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
								"/bin/sh", "-c", script, "generator",
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
									MountPath: mibsDir,
									SubPath:   path.Clean(MibsSubDir),
									ReadOnly:  true,
								},
								{
									Name:      generatorVolumeName,
									MountPath: generatorDir,
									ReadOnly:  true,
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
