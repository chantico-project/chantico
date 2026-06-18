package volumes

import (
	configuration "chantico/internal/configuration"
	corev1 "k8s.io/api/core/v1"
)

const (
	ChanticoVolumeMount = "chantico-volume-mount"
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
