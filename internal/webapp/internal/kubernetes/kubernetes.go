package kubernetes

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, err
	}

	rawConfig, err := clientcmd.LoadFromFile(kubeconfigPath)
	if err != nil {
		return nil, err
	}

	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	v1alpha1.AddToScheme(scheme)

	k, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}

	return &KubernetesClient{
		client:         k,
		CurrentContext: rawConfig.CurrentContext,
		Host:           config.Host,
	}, nil
}

type Pod struct {
	Namespace string
	Name      string
}

func (k *KubernetesClient) GetPods() ([]Pod, error) {
	pods := &corev1.PodList{}
	if err := k.client.List(context.Background(), pods); err != nil {
		return nil, fmt.Errorf("failed to list Pods: %w", err)
	}

	p := make([]Pod, 0, pods.Size())

	for _, pod := range pods.Items {
		p = append(p, Pod{Namespace: pod.Namespace, Name: pod.Name})
	}
	return p, nil
}

func (k *KubernetesClient) GetNamespaces() (*corev1.NamespaceList, error) {
	namespaces := &corev1.NamespaceList{}
	if err := k.client.List(context.TODO(), namespaces); err != nil {
		return nil, fmt.Errorf("failed to list Namespaces: %w", err)
	}
	return namespaces, nil
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
