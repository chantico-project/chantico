package snmpexporter

import (
	"fmt"

	chantico "chantico/api/v1alpha1"
	"chantico/internal/config"
	vol "chantico/internal/volumes"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	DeploymentName       = "chantico-snmp"
	ConfigHashAnnotation = "chantico.ci.tno.nl/config-hash"

	podMountPath = "/data"
	snmpYAMLPath = podMountPath + "/snmp/snmp.yml"
	listenPort   = int32(9116)
)

// BuildDeployment builds the snmp_exporter Deployment for an
// SNMPExporter. The configHash is stamped as a pod-template annotation
// so any change forces a rollout.
func BuildDeployment(cfg config.Config, e *chantico.SNMPExporter, configHash string) (*appsv1.Deployment, error) {
	volume, err := vol.GetChanticoVolume()
	if err != nil {
		return nil, err
	}

	replicas := int32(1)
	if e.Spec.Replicas != nil {
		replicas = *e.Spec.Replicas
	}

	labels := map[string]string{
		"app.kubernetes.io/name":       "snmp-exporter",
		"app.kubernetes.io/instance":   e.GetName(),
		"app.kubernetes.io/managed-by": "chantico",
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", DeploymentName, e.GetName()),
			Namespace: e.GetNamespace(),
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: map[string]string{ConfigHashAnnotation: configHash},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "snmp-exporter",
						Image: cfg.Images.SnmpExporter,
						Args:  []string{"--config.file=" + snmpYAMLPath},
						Ports: []corev1.ContainerPort{{
							Name:          "http",
							ContainerPort: listenPort,
							Protocol:      corev1.ProtocolTCP,
						}},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/health",
									Port: intstr.FromInt32(listenPort),
								},
							},
						},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      vol.ChanticoVolumeMount,
							MountPath: podMountPath,
							ReadOnly:  true,
						}},
					}},
					Volumes: []corev1.Volume{volume},
				},
			},
		},
	}, nil
}
