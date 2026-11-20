package measurementdevice

import (
	"strconv"
	"strings"
	"testing"

	chantico "chantico/api/v1alpha1"
	config "chantico/internal/configuration"
	img "chantico/internal/images"
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

	var builder GeneratorJobBuilder = SNMPGeneratorJobBuilder{}
	job, err := builder.BuildJob(dev)
	if err != nil {
		t.Fatalf("BuildGeneratorJob: %v", err)
	}
	if len(job.OwnerReferences) != 0 || job.ResourceVersion != "" {
		t.Fatal("builder must leave application and ownership to the controller")
	}

	if job.Name != "tno" || job.Namespace != "chantico" {
		t.Errorf("unexpected name/ns: %s/%s", job.Name, job.Namespace)
	}
	if len(job.Labels) != 0 {
		t.Errorf("builder must not add Job labels: %v", job.Labels)
	}

	gen := job.Annotations[GenerationAnnotation]
	if gen != strconv.FormatInt(7, 10) {
		t.Errorf("missing/wrong generation annotation: %q", gen)
	}

	if got := *job.Spec.BackoffLimit; got != 3 {
		t.Errorf("BackoffLimit = %d, want 3", got)
	}

	containers := job.Spec.Template.Spec.Containers
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}
	args := containers[0].Args
	wantContains := []string{
		"--output-path", "/data/snmp/output/a-random-uid/7.yml",
		"--generator-path", "/data/snmp/generators/" + GeneratorConfigKey,
		"--mibs-dir", "/data/snmp/mibs",
	}

	// Check arguments in job.
	joined := strings.Join(args, " ")
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
		"/data/snmp/output":     false,
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
		if m.Name == vol.ChanticoVolumeMount {
			if want := strings.TrimPrefix(m.MountPath, "/data/"); m.SubPath != want {
				t.Errorf("mount %q SubPath = %q, want %q", m.MountPath, m.SubPath, want)
			}
		}
	}

	volumes := pod.Spec.Volumes
	if len(volumes) != 2 || volumes[1].ConfigMap == nil || volumes[1].ConfigMap.Name != "tno-generator" {
		t.Errorf("expected generator ConfigMap volume %q, got %+v", "tno-generator", volumes)
	}
	if pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken {
		t.Fatal("automatic API token mounting must be disabled")
	}
	if pod.Spec.ServiceAccountName != "" || job.Annotations[GeneratorOutputAnnotation] != GeneratorOutputPath(dev) {
		t.Fatal("job must use generation-scoped output without a dedicated service account")
	}
	if pod.Spec.SecurityContext.FSGroup == nil {
		t.Fatal("shared output needs group write access for non-root generator")
	}
	if len(pod.Spec.InitContainers) != 0 || containers[0].Image != img.SnmpGenerator {
		t.Fatal("job must contain only the upstream SNMP generator")
	}
	if args[0] != "/data/snmp/output/a-random-uid" {
		t.Fatalf("unexpected output directory: %q", args[0])
	}
}
