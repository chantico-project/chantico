package volumes

import (
	configuration "chantico/internal/configuration"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ChanticoVolumeMount = "chantico-volume-mount"
	// DataVolumeLabel must match chantico.dataVolumeLabel in the Helm chart.
	DataVolumeLabel = "chantico-project.github.io/data-volume"
)

func GetChanticoVolume() (corev1.Volume, error) {
	volumeClaim := configuration.ValidatedEnv.VolumeClaim
	return corev1.Volume{
		Name: ChanticoVolumeMount,
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: volumeClaim,
			},
		},
	}, nil

}

// DataVolumeAffinity co-locates pods sharing a ReadWriteOnce claim on one node.
func DataVolumeAffinity(claim string) *corev1.Affinity {
	return &corev1.Affinity{
		PodAffinity: &corev1.PodAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{
				LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{DataVolumeLabel: claim}},
				TopologyKey:   corev1.LabelHostname,
			}},
		},
	}
}
