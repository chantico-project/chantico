package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"

	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/yaml"

	"chantico/api/v1alpha1"
	"chantico/internal/webapp/internal/graph"
)

type KubernetesClient struct {
	client         client.Client
	CurrentContext string
	Host           string
}

func New() (*KubernetesClient, error) {
	config, err := ctrlconfig.GetConfig()
	if err != nil {
		return nil, err
	}

	scheme, err := newScheme()
	if err != nil {
		return nil, err
	}

	k, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}

	return &KubernetesClient{
		client:         k,
		CurrentContext: currentContext(),
		Host:           config.Host,
	}, nil
}

func currentContext() string {
	rawConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	).RawConfig()
	if err != nil || rawConfig.CurrentContext == "" {
		return "in-cluster"
	}
	return rawConfig.CurrentContext
}

func newScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	return scheme, nil
}

func (k *KubernetesClient) listDataCenterResources() ([]v1alpha1.DataCenterResource, error) {
	dcrList := &v1alpha1.DataCenterResourceList{}
	if err := k.client.List(context.Background(), dcrList); err != nil {
		return nil, fmt.Errorf("failed to list DataCenterResources: %w", err)
	}
	return dcrList.Items, nil
}

func nodeName(dcr *v1alpha1.DataCenterResource) string {
	return dcr.GetNamespace() + "-" + dcr.GetName()
}

func (k *KubernetesClient) GetDataCenterResource(id string) (*v1alpha1.DataCenterResource, error) {
	dcrs, err := k.listDataCenterResources()
	if err != nil {
		return nil, err
	}

	for i := range dcrs {
		if nodeName(&dcrs[i]) == id {
			return &dcrs[i], nil
		}
	}

	return nil, ErrNotFound
}

var ErrNotFound = errors.New("not found")

var ErrInvalidManifest = errors.New("invalid manifest")

func (k *KubernetesClient) UpdateDataCenterResource(id string, manifest []byte) (*v1alpha1.DataCenterResource, error) {
	existing, err := k.GetDataCenterResource(id)
	if err != nil {
		return nil, err
	}

	desired := &v1alpha1.DataCenterResource{}
	if err := yaml.Unmarshal(manifest, desired); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidManifest, err.Error())
	}
	if desired.GetNamespace() != existing.GetNamespace() || desired.GetName() != existing.GetName() {
		return nil, fmt.Errorf("%w: name and namespace must match the node", ErrInvalidManifest)
	}

	desired.SetResourceVersion(existing.GetResourceVersion())
	if err := k.client.Update(context.Background(), desired); err != nil {
		return nil, fmt.Errorf("failed to update DataCenterResource: %w", err)
	}
	return desired, nil
}

func (k *KubernetesClient) DeleteDataCenterResource(id string) error {
	existing, err := k.GetDataCenterResource(id)
	if err != nil {
		return err
	}
	if err := k.client.Delete(context.Background(), existing); err != nil {
		return fmt.Errorf("failed to delete DataCenterResource: %w", err)
	}
	return nil
}

func (k *KubernetesClient) ListDataCenterNamespaces() ([]string, error) {
	dcrs, err := k.listDataCenterResources()
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	var namespaces []string
	for i := range dcrs {
		ns := dcrs[i].GetNamespace()
		if !seen[ns] {
			seen[ns] = true
			namespaces = append(namespaces, ns)
		}
	}
	sort.Strings(namespaces)
	return namespaces, nil
}

func NodeStatus(conditions []metav1.Condition) string {
	ready := meta.FindStatusCondition(conditions, string(v1alpha1.ConditionReady))
	switch {
	case ready == nil, ready.Status == metav1.ConditionUnknown:
		return "pending"
	case ready.Status == metav1.ConditionTrue:
		return "ok"
	case ready.Reason == string(v1alpha1.ReasonReconciling), ready.Reason == string(v1alpha1.ReasonGenerationPending):
		return "pending"
	default:
		return "faulty"
	}
}

func (k *KubernetesClient) GetDataCenterResources(namespace string) ([]*graph.Node, error) {
	dcrList := &v1alpha1.DataCenterResourceList{}
	if err := k.client.List(context.Background(), dcrList, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list DataCenterResources: %w", err)
	}

	var output []*graph.Node

	for i := range dcrList.Items {
		dcr := &dcrList.Items[i]
		node := graph.Node{
			Name:   nodeName(dcr),
			Label:  dcr.GetName(),
			Status: NodeStatus(dcr.Status.Conditions),
		}

		for _, parent := range dcr.Spec.ParentNames() {
			node.Parents = append(node.Parents, &graph.Node{
				Name: dcr.GetNamespace() + "-" + parent,
			})
		}
		output = append(output, &node)
	}

	return output, nil
}

func (k *KubernetesClient) ListPhysicalMeasurements(namespace string) ([]v1alpha1.PhysicalMeasurement, error) {
	list := &v1alpha1.PhysicalMeasurementList{}
	if err := k.client.List(context.Background(), list, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list PhysicalMeasurements: %w", err)
	}
	return list.Items, nil
}

func (k *KubernetesClient) ListMeasurementDevices(namespace string) ([]v1alpha1.MeasurementDevice, error) {
	list := &v1alpha1.MeasurementDeviceList{}
	if err := k.client.List(context.Background(), list, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list MeasurementDevices: %w", err)
	}
	return list.Items, nil
}
