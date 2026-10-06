package kubernetes

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"sigs.k8s.io/controller-runtime/pkg/client"

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
