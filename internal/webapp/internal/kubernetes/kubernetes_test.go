package kubernetes

import (
	"errors"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"chantico/api/v1alpha1"
	"chantico/internal/testutil"
	"chantico/internal/webapp/internal/graph"
)

func newTestClient(t *testing.T, objs ...client.Object) *KubernetesClient {
	t.Helper()
	c := testutil.NewFakeClientBuilder(t, corev1.AddToScheme, v1alpha1.AddToScheme).
		WithObjects(objs...).
		Build()
	return &KubernetesClient{client: c}
}

func newDCR(namespace, name string, parents []string, conditions ...metav1.Condition) *v1alpha1.DataCenterResource {
	d := &v1alpha1.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec:       v1alpha1.DataCenterResourceSpec{Type: "pdu", EnergyMetric: "metric"},
		Status:     v1alpha1.DataCenterResourceStatus{Conditions: conditions},
	}
	for _, p := range parents {
		d.Spec.Parents = append(d.Spec.Parents, v1alpha1.ParentRef{Name: p, Coefficient: "1"})
	}
	return d
}

func readyCondition(status metav1.ConditionStatus, reason v1alpha1.ConditionReason) metav1.Condition {
	return metav1.Condition{
		Type:   string(v1alpha1.ConditionReady),
		Status: status,
		Reason: string(reason),
	}
}

func TestNodeStatus(t *testing.T) {
	tests := []struct {
		name       string
		conditions []metav1.Condition
		want       string
	}{
		{"no conditions", nil, "pending"},
		{"no ready condition", []metav1.Condition{{Type: string(v1alpha1.ConditionValidated), Status: metav1.ConditionTrue}}, "pending"},
		{"ready unknown", []metav1.Condition{readyCondition(metav1.ConditionUnknown, v1alpha1.ReasonReconciling)}, "pending"},
		{"ready true", []metav1.Condition{readyCondition(metav1.ConditionTrue, v1alpha1.ReasonReconciled)}, "ok"},
		{"ready false reconciling", []metav1.Condition{readyCondition(metav1.ConditionFalse, v1alpha1.ReasonReconciling)}, "faulty"},
		{"ready false generation pending", []metav1.Condition{readyCondition(metav1.ConditionFalse, v1alpha1.ReasonGenerationPending)}, "faulty"},
		{"apply failed", []metav1.Condition{readyCondition(metav1.ConditionFalse, v1alpha1.ReasonApplyFailed)}, "faulty"},
		{"invalid spec", []metav1.Condition{readyCondition(metav1.ConditionFalse, v1alpha1.ReasonInvalidSpec)}, "faulty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NodeStatus(tt.conditions); got != tt.want {
				t.Errorf("NodeStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetDataCenterResource(t *testing.T) {
	k := newTestClient(t,
		newDCR("ns1", "pdu-a", nil),
		newDCR("ns2", "pdu-a", nil),
	)

	got, err := k.GetDataCenterResource("ns2-pdu-a")
	if err != nil {
		t.Fatalf("GetDataCenterResource() unexpected error: %v", err)
	}
	if got.GetNamespace() != "ns2" || got.GetName() != "pdu-a" {
		t.Errorf("GetDataCenterResource() returned %s/%s, want ns2/pdu-a", got.GetNamespace(), got.GetName())
	}

	if _, err := k.GetDataCenterResource("ns3-pdu-a"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetDataCenterResource() error = %v, want ErrNotFound", err)
	}
}

func TestGetDataCenterResources(t *testing.T) {
	k := newTestClient(t,
		newDCR("ns1", "root", nil, readyCondition(metav1.ConditionTrue, v1alpha1.ReasonReconciled)),
		newDCR("ns1", "child", []string{"root", "other"}, readyCondition(metav1.ConditionFalse, v1alpha1.ReasonApplyFailed)),
		newDCR("ns2", "elsewhere", nil),
	)

	got, err := k.GetDataCenterResources("ns1")
	if err != nil {
		t.Fatalf("GetDataCenterResources() unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("GetDataCenterResources() returned %d nodes, want 2", len(got))
	}

	byName := map[string]*graph.Node{}
	for _, n := range got {
		byName[n.Name] = n
	}

	root, ok := byName["ns1-root"]
	if !ok {
		t.Fatalf("missing node ns1-root in %+v", got)
	}
	if root.Label != "root" || root.Status != "ok" || len(root.Parents) != 0 {
		t.Errorf("ns1-root = %+v, want label root, status ok, no parents", root)
	}

	child, ok := byName["ns1-child"]
	if !ok {
		t.Fatalf("missing node ns1-child in %+v", got)
	}
	if child.Label != "child" || child.Status != "faulty" {
		t.Errorf("ns1-child = %+v, want label child, status faulty", child)
	}
	if gotParents := nodeNames(child.Parents); !reflect.DeepEqual(gotParents, []string{"ns1-root", "ns1-other"}) {
		t.Errorf("ns1-child parents = %v, want [ns1-root ns1-other]", gotParents)
	}
}

func nodeNames(nodes []*graph.Node) []string {
	var names []string
	for _, n := range nodes {
		names = append(names, n.Name)
	}
	return names
}

func TestListDataCenterNamespaces(t *testing.T) {
	k := newTestClient(t,
		newDCR("zeta", "a", nil),
		newDCR("alpha", "a", nil),
		newDCR("alpha", "b", nil),
		newDCR("mid", "a", nil),
	)

	got, err := k.ListDataCenterNamespaces()
	if err != nil {
		t.Fatalf("ListDataCenterNamespaces() unexpected error: %v", err)
	}
	want := []string{"alpha", "mid", "zeta"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListDataCenterNamespaces() = %v, want %v", got, want)
	}

	empty := newTestClient(t)
	got, err = empty.ListDataCenterNamespaces()
	if err != nil {
		t.Fatalf("ListDataCenterNamespaces() on empty client unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListDataCenterNamespaces() on empty client = %v, want empty", got)
	}
}

func TestUpdateDataCenterResource(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		manifest string
		wantErr  error
	}{
		{
			name:     "success",
			id:       "ns1-pdu-a",
			manifest: "metadata:\n  name: pdu-a\n  namespace: ns1\nspec:\n  type: pdu\n  energyMetric: updated_metric\n",
		},
		{"not found", "ns9-missing", "metadata:\n  name: pdu-a\n  namespace: ns1\nspec:\n  type: pdu\n  energyMetric: m\n", ErrNotFound},
		{"invalid yaml", "ns1-pdu-a", "spec: [unclosed", ErrInvalidManifest},
		{"mismatched name", "ns1-pdu-a", "metadata:\n  name: other\n  namespace: ns1\nspec:\n  type: pdu\n  energyMetric: m\n", ErrInvalidManifest},
		{"mismatched namespace", "ns1-pdu-a", "metadata:\n  name: pdu-a\n  namespace: ns2\nspec:\n  type: pdu\n  energyMetric: m\n", ErrInvalidManifest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := newTestClient(t, newDCR("ns1", "pdu-a", nil))

			updated, err := k.UpdateDataCenterResource(tt.id, []byte(tt.manifest))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("UpdateDataCenterResource() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if updated.Spec.EnergyMetric != "updated_metric" {
				t.Errorf("returned EnergyMetric = %q, want updated_metric", updated.Spec.EnergyMetric)
			}

			stored, err := k.GetDataCenterResource(tt.id)
			if err != nil {
				t.Fatalf("GetDataCenterResource() after update unexpected error: %v", err)
			}
			if stored.Spec.EnergyMetric != "updated_metric" {
				t.Errorf("stored EnergyMetric = %q, want updated_metric", stored.Spec.EnergyMetric)
			}
		})
	}
}

func TestDeleteDataCenterResource(t *testing.T) {
	k := newTestClient(t, newDCR("ns1", "pdu-a", nil))

	if err := k.DeleteDataCenterResource("ns1-pdu-a"); err != nil {
		t.Fatalf("DeleteDataCenterResource() unexpected error: %v", err)
	}
	if _, err := k.GetDataCenterResource("ns1-pdu-a"); !errors.Is(err, ErrNotFound) {
		t.Errorf("resource still present after delete, error = %v", err)
	}
	if err := k.DeleteDataCenterResource("ns1-pdu-a"); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteDataCenterResource() on missing resource error = %v, want ErrNotFound", err)
	}
}
