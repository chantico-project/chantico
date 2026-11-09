package kubernetes

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"chantico/api/v1alpha1"
	"chantico/internal/webapp/internal/graph"
)

type KubernetesClient struct {
	client         client.Client
	CurrentContext string
	Host           string
}

func New(kubeconfigPath string) (*KubernetesClient, error) {
	config, currentContext, err := loadRESTConfig(kubeconfigPath)
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
		CurrentContext: currentContext,
		Host:           config.Host,
	}, nil
}

func loadRESTConfig(kubeconfigPath string) (*rest.Config, string, error) {
	if kubeconfigPath == "" {
		config, err := rest.InClusterConfig()
		return config, "in-cluster", err
	}

	return loadKubeconfig(kubeconfigPath)
}

func loadKubeconfig(kubeconfigPath string) (*rest.Config, string, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, "", err
	}

	rawConfig, err := clientcmd.LoadFromFile(kubeconfigPath)
	if err != nil {
		return nil, "", err
	}

	return config, rawConfig.CurrentContext, nil
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

type Pod struct {
	Namespace string
	Name      string
}

func (k *KubernetesClient) GetDataCenterResource(id string) (*v1alpha1.DataCenterResource, error) {
	dcrList := &v1alpha1.DataCenterResourceList{}
	if err := k.client.List(context.Background(), dcrList); err != nil {
		return nil, fmt.Errorf("failed to list DataCenterResources: %w", err)
	}

	for i := range dcrList.Items {
		dcr := &dcrList.Items[i]
		if dcr.GetNamespace()+"-"+dcr.GetName() == id {
			return dcr, nil
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

func (k *KubernetesClient) GetDataCenterResources() ([]*graph.Node, error) {
	dcrList := &v1alpha1.DataCenterResourceList{}
	if err := k.client.List(context.Background(), dcrList); err != nil {
		return nil, fmt.Errorf("failed to list DataCenterResources: %w", err)
	}

	var output []*graph.Node

	for _, dcr := range dcrList.Items {
		name := dcr.GetNamespace() + "-" + dcr.GetName()
		node := graph.Node{
			Name: name,
		}

		parents := dcr.Spec.ParentNames()
		for _, parent := range parents {
			p := &graph.Node{
				Name: dcr.GetNamespace() + "-" + parent,
			}
			node.Parents = append(node.Parents, p)
		}
		output = append(output, &node)
	}

	return output, nil
}
