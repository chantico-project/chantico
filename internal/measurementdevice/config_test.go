package measurementdevice

import (
	"strconv"
	"strings"
	"testing"

	chantico "chantico/api/v1alpha1"
	vol "chantico/internal/volumes"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestBuildGeneratorJob(t *testing.T) {
	t.Setenv(vol.ChanticoVolumeMount, "chantico-volume-claim")

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
	if got := pod.Labels[vol.DataVolumeLabel]; got != claim {
		t.Errorf("pod label %q = %q, want %q", vol.DataVolumeLabel, got, claim)
	}
	terms := pod.Spec.Affinity.PodAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	if len(terms) != 1 || terms[0].LabelSelector.MatchLabels[vol.DataVolumeLabel] != claim {
		t.Errorf("pod affinity does not select %q=%q: %+v", vol.DataVolumeLabel, claim, terms)
	}
}
