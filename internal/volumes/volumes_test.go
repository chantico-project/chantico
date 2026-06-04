package volumes

import (
	config "chantico/internal/configuration"
	"testing"
)

func TestGetChanticoVolume(t *testing.T) {
	t.Setenv(config.ChanticoVolumeClaimEnv, "test-test")
	config.ValidatedEnv, _ = config.ValidateEnv()
	volume, err := GetChanticoVolume()
	if err == nil && volume.VolumeSource.PersistentVolumeClaim.ClaimName != "test-test" {
		t.Errorf("%#v is not in sync with the volume definition %#v", config.ChanticoVolumeClaimEnv, &volume.VolumeSource.PersistentVolumeClaim)
	}
}
