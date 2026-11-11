package measurementdevice

import (
	"strconv"
	"strings"
	"testing"

	chantico "chantico/api/v1alpha1"
	config "chantico/internal/configuration"
	vol "chantico/internal/volumes"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestBuildGeneratorJob(t *testing.T) {
	previousEnv := config.ValidatedEnv
	config.ValidatedEnv.VolumeClaim = "chantico-volume-claim"
	t.Cleanup(func() { config.ValidatedEnv = previousEnv })

	dev := &chantico.MeasurementDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "tno",
			Namespace:  "chantico",
			UID:        types.UID("a-random-uid"),
			Generation: 7,
		},
	}

	job, err := BuildGeneratorJob(dev)
	if err != nil {
		t.Fatalf("BuildGeneratorJob: %v", err)
	}

	if job.Name != "tno" || job.Namespace != "chantico" {
		t.Errorf("unexpected name/ns: %s/%s", job.Name, job.Namespace)
	}

	gen := job.Annotations[GenerationAnnotation]
	if gen != strconv.FormatInt(7, 10) {
		t.Errorf("missing/wrong generation annotation: %q", gen)
	}

	if got := *job.Spec.BackoffLimit; got != 0 {
		t.Errorf("BackoffLimit = %d, want 0", got)
	}

	containers := job.Spec.Template.Spec.Containers
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}
	args := containers[0].Args
	wantContains := []string{
		"--output-path", "/data/snmp/yml/snmp-" + string(dev.UID) + ".yaml",
		"--generator-path", "/data/snmp/generators/generator-" + string(dev.UID) + ".yaml",
		"--mibs-dir", "/data/snmp/mibs",
	}

	// Check arguments in job.
	joined := ""
	for _, a := range args {
		joined += a + " "
	}
	for _, w := range wantContains {
		if !strings.Contains(joined, w) {
			t.Errorf("arg %q missing from %q", w, joined)
		}
	}

	pod := job.Spec.Template
	claim := pod.Spec.Volumes[0].PersistentVolumeClaim.ClaimName
	if claim != "chantico-volume-claim" {
		t.Errorf("volume claim = %q, want %q", claim, "chantico-volume-claim")
	}
	if got := pod.Labels[vol.DataVolumeLabel]; got != claim {
		t.Errorf("pod label %q = %q, want %q", vol.DataVolumeLabel, got, claim)
	}
	terms := pod.Spec.Affinity.PodAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	if len(terms) != 1 || terms[0].LabelSelector.MatchLabels[vol.DataVolumeLabel] != claim {
		t.Errorf("pod affinity does not select %q=%q: %+v", vol.DataVolumeLabel, claim, terms)
	}
	if len(terms) == 1 && terms[0].TopologyKey != corev1.LabelHostname {
		t.Errorf("topology key = %q, want %q", terms[0].TopologyKey, corev1.LabelHostname)
	}

	wantMounts := map[string]bool{
		"/data/snmp/mibs":       true,
		"/data/snmp/generators": true,
		"/data/snmp/yml":        false,
	}
	mounts := containers[0].VolumeMounts
	if len(mounts) != len(wantMounts) {
		t.Fatalf("expected %d mounts, got %d: %+v", len(wantMounts), len(mounts), mounts)
	}
	for _, m := range mounts {
		readOnly, ok := wantMounts[m.MountPath]
		if !ok {
			t.Errorf("unexpected mount path %q", m.MountPath)
			continue
		}
		if m.ReadOnly != readOnly {
			t.Errorf("mount %q ReadOnly = %v, want %v", m.MountPath, m.ReadOnly, readOnly)
		}
		if want := strings.TrimPrefix(m.MountPath, "/data/"); m.SubPath != want {
			t.Errorf("mount %q SubPath = %q, want %q", m.MountPath, m.SubPath, want)
		}
	}
}
