package datacenterresource

import (
	"errors"
	"reflect"
	"testing"

	chantico "chantico/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidate(t *testing.T) {
	testCases := map[string]struct {
		Resource                 *chantico.DataCenterResource
		Resources                []chantico.DataCenterResource
		ExpectedVisited          []chantico.DataCenterResource
		ExpectedError            error
		ExpectedInvolvedResource string
	}{
		"creates resource if empty": {
			Resource: &chantico.DataCenterResource{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: chantico.DataCenterResourceSpec{
					Type:         "pdu",
					Parents:      []chantico.ParentRef{},
					EnergyMetric: `tnoPduPowerValue{job="tno"}`,
				},
			},
			Resources:                []chantico.DataCenterResource{},
			ExpectedVisited:          []chantico.DataCenterResource{},
			ExpectedError:            nil,
			ExpectedInvolvedResource: "",
		},
		"gives error if root node has no energyMetric": {
			Resource: &chantico.DataCenterResource{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: chantico.DataCenterResourceSpec{
					Type:    "pdu",
					Parents: []chantico.ParentRef{},
				},
			},
			Resources:                []chantico.DataCenterResource{},
			ExpectedVisited:          []chantico.DataCenterResource{},
			ExpectedError:            ErrorMissingEnergyMetric{InvolvedResource: "foo"},
			ExpectedInvolvedResource: "",
		},
		"creates resource with acyclic dependency": {
			Resource: &chantico.DataCenterResource{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: chantico.DataCenterResourceSpec{
					Type:    "baremetal",
					Parents: []chantico.ParentRef{{Name: "bar"}},
				},
			},
			Resources: []chantico.DataCenterResource{{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: chantico.DataCenterResourceSpec{
					Type:    "baremetal",
					Parents: []chantico.ParentRef{{Name: "bar"}},
				},
			}, {
				ObjectMeta: metav1.ObjectMeta{
					Name: "bar",
				},
				Spec: chantico.DataCenterResourceSpec{
					Type:    "pdu",
					Parents: []chantico.ParentRef{},
				},
			}},
			ExpectedVisited: []chantico.DataCenterResource{{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bar",
				},
				Spec: chantico.DataCenterResourceSpec{
					Type:    "pdu",
					Parents: []chantico.ParentRef{},
				},
			}},
			ExpectedError:            nil,
			ExpectedInvolvedResource: "",
		},
		"creates resource with convergent dependency": {
			Resource: &chantico.DataCenterResource{
				ObjectMeta: metav1.ObjectMeta{
					Name: "vm1",
				},
				Spec: chantico.DataCenterResourceSpec{
					Type:    "vm",
					Parents: []chantico.ParentRef{{Name: "bm1"}, {Name: "bm2"}},
				},
			},
			Resources: []chantico.DataCenterResource{{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pdu1",
				},
				Spec: chantico.DataCenterResourceSpec{
					Type:    "pdu",
					Parents: []chantico.ParentRef{},
				},
			}, {
				ObjectMeta: metav1.ObjectMeta{
					Name: "pdu2",
				},
				Spec: chantico.DataCenterResourceSpec{
					Type:    "pdu",
					Parents: []chantico.ParentRef{},
				},
			}, {
				ObjectMeta: metav1.ObjectMeta{
					Name: "bm1",
				},
				Spec: chantico.DataCenterResourceSpec{
					Type:    "baremetal",
					Parents: []chantico.ParentRef{{Name: "pdu1"}, {Name: "pdu2"}},
				},
			}, {
				ObjectMeta: metav1.ObjectMeta{
					Name: "bm2",
				},
				Spec: chantico.DataCenterResourceSpec{
					Type:    "baremetal",
					Parents: []chantico.ParentRef{{Name: "pdu1"}, {Name: "pdu2"}},
				},
			}, {
				ObjectMeta: metav1.ObjectMeta{
					Name: "vm1",
				},
				Spec: chantico.DataCenterResourceSpec{
					Type:    "vm",
					Parents: []chantico.ParentRef{{Name: "bm1"}, {Name: "bm2"}},
				},
			}},
			ExpectedVisited: []chantico.DataCenterResource{{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bm1",
				},
				Spec: chantico.DataCenterResourceSpec{
					Type:    "baremetal",
					Parents: []chantico.ParentRef{{Name: "pdu1"}, {Name: "pdu2"}},
				},
			}, {
				ObjectMeta: metav1.ObjectMeta{
					Name: "bm2",
				},
				Spec: chantico.DataCenterResourceSpec{
					Type:    "baremetal",
					Parents: []chantico.ParentRef{{Name: "pdu1"}, {Name: "pdu2"}},
				},
			}, {
				ObjectMeta: metav1.ObjectMeta{
					Name: "pdu1",
				},
				Spec: chantico.DataCenterResourceSpec{
					Type:    "pdu",
					Parents: []chantico.ParentRef{},
				},
			}, {
				ObjectMeta: metav1.ObjectMeta{
					Name: "pdu2",
				},
				Spec: chantico.DataCenterResourceSpec{
					Type:    "pdu",
					Parents: []chantico.ParentRef{},
				},
			}},
			ExpectedError:            nil,
			ExpectedInvolvedResource: "",
		},
		"gives error if a resource is not found": {
			Resource: &chantico.DataCenterResource{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: chantico.DataCenterResourceSpec{
					Type:    "pdu",
					Parents: []chantico.ParentRef{{Name: "bar"}},
				},
			},
			Resources:                []chantico.DataCenterResource{},
			ExpectedVisited:          []chantico.DataCenterResource{},
			ExpectedError:            ErrorResourceNotFound{InvolvedResource: "bar"},
			ExpectedInvolvedResource: "bar",
		},
		"gives error if a cycle is found": {
			Resource: &chantico.DataCenterResource{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: chantico.DataCenterResourceSpec{
					Type:    "pdu",
					Parents: []chantico.ParentRef{{Name: "bar"}},
				},
			},
			Resources: []chantico.DataCenterResource{{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: chantico.DataCenterResourceSpec{
					Type:    "pdu",
					Parents: []chantico.ParentRef{{Name: "bar"}},
				},
			}, {
				ObjectMeta: metav1.ObjectMeta{
					Name: "bar",
				},
				Spec: chantico.DataCenterResourceSpec{
					Type:    "pdu",
					Parents: []chantico.ParentRef{{Name: "foo"}},
				},
			}},
			ExpectedVisited:          []chantico.DataCenterResource{},
			ExpectedError:            ErrorCycleDetected{InvolvedResource: "bar"},
			ExpectedInvolvedResource: "bar",
		},
		"gives error if a self-reference is found": {
			Resource: &chantico.DataCenterResource{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: chantico.DataCenterResourceSpec{
					Type:    "pdu",
					Parents: []chantico.ParentRef{{Name: "foo"}},
				},
			},
			Resources: []chantico.DataCenterResource{{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: chantico.DataCenterResourceSpec{
					Type:    "pdu",
					Parents: []chantico.ParentRef{{Name: "foo"}},
				},
			}},
			ExpectedVisited:          []chantico.DataCenterResource{},
			ExpectedError:            ErrorCycleDetected{InvolvedResource: "foo"},
			ExpectedInvolvedResource: "foo",
		},
		"gives error if unknown type is found": {
			Resource: &chantico.DataCenterResource{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: chantico.DataCenterResourceSpec{
					Type:    "perpetuummobile",
					Parents: []chantico.ParentRef{},
				},
			},
			Resources:                []chantico.DataCenterResource{},
			ExpectedVisited:          []chantico.DataCenterResource{},
			ExpectedError:            ErrorUnknownType{Type: "perpetuummobile"},
			ExpectedInvolvedResource: "",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			visited, err, involvedResource := Validate(tc.Resource, tc.Resources, []chantico.PhysicalMeasurement{})
			if !reflect.DeepEqual(visited, tc.ExpectedVisited) || !errors.Is(err, tc.ExpectedError) || involvedResource != tc.ExpectedInvolvedResource {
				t.Errorf("Validate(%#v, %#v) = %#v, %#v, want %#v, %#v\n)", tc.Resource, FormatResources(tc.Resources), FormatResources(visited), err, FormatResources(tc.ExpectedVisited), tc.ExpectedError)
			}
		})
	}
}
