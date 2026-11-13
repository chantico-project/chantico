package volumes

import (
	config "chantico/internal/configuration"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestGetChanticoVolume(t *testing.T) {
	t.Setenv(config.ChanticoVolumeClaimEnv, "test-test")
	config.ValidatedEnv, _ = config.ValidateEnv()
	volume, err := GetChanticoVolume()
	if err == nil && volume.VolumeSource.PersistentVolumeClaim.ClaimName != "test-test" {
		t.Errorf("%#v is not in sync with the volume definition %#v", config.ChanticoVolumeClaimEnv, &volume.VolumeSource.PersistentVolumeClaim)
	}
}

func TestDataVolumeAffinity(t *testing.T) {
	for _, claim := range []string{"test-claim", "other-claim"} {
		t.Run(claim, func(test *testing.T) {
			affinity := DataVolumeAffinity(claim)
			terms := affinity.PodAffinity.RequiredDuringSchedulingIgnoredDuringExecution
			if len(terms) != 1 {
				test.Fatalf("expected 1 required pod affinity term, got %d", len(terms))
			}
			if got := terms[0].LabelSelector.MatchLabels[DataVolumeLabel]; got != claim {
				test.Errorf("label selector %q = %q, want %q", DataVolumeLabel, got, claim)
			}
			if terms[0].TopologyKey != corev1.LabelHostname {
				test.Errorf("topology key = %q, want %q", terms[0].TopologyKey, corev1.LabelHostname)
			}
			if len(terms[0].Namespaces) != 0 || terms[0].NamespaceSelector != nil {
				test.Error("pod affinity must stay within the pod namespace")
			}
		})
	}
}
