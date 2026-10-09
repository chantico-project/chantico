/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package datacenterresource

import (
	"bytes"
	chantico "chantico/api/v1alpha1"
	"context"
	"fmt"
	"html/template"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ResolvedDataCenterResource struct {
	Name             string
	Type             string
	AdditionalLabels map[string]string
	ServiceId        string

	EnergyMetric string
	Parents      []ResolvedParent
}

type ResolvedParent struct {
	Name        string
	Coefficient string
}

type TemplateRenderer struct {
	client    client.Client
	namespace string
}

func NewTemplateRenderer(client client.Client, namespace string) *TemplateRenderer {
	return &TemplateRenderer{
		client:    client,
		namespace: namespace,
	}
}

// Helper function which resolves and performs the substitution for of the templates. Used for both the coefficient templates and energy metrics templates.
func (tr *TemplateRenderer) renderTemplate(ctx context.Context, templateFrom *chantico.TemplateFrom) (string, error) {
	// Lookup the configmap and resolve retrieve the template text
	templateText, err := templateFrom.ResolveTemplate(ctx, tr.client, tr.namespace)
	if err != nil {
		return "", err
	}

	parameters, err := templateFrom.ResolveParameters(ctx, tr.client, tr.namespace)
	if err != nil {
		return "", err
	}

	// Apply the template with the resolved parameters
	tmpl, err := template.New(templateFrom.ConfigMapKeyRef.Name).Option("missingkey=error").Parse(templateText)
	if err != nil {
		return "", fmt.Errorf("parse template from ConfigMap %q: %w", templateFrom.ConfigMapKeyRef.Name, err)
	}
	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, parameters); err != nil {
		return "", fmt.Errorf("render template from ConfigMap %q: %w", templateFrom.ConfigMapKeyRef.Name, err)
	}
	return rendered.String(), nil
}

func (tr *TemplateRenderer) resolveEnergyMetricTemplate(ctx context.Context, dataCenterResource *chantico.DataCenterResource) (string, error) {
	spec := dataCenterResource.Spec
	if spec.EnergyMetricFrom != nil {
		return tr.renderTemplate(ctx, spec.EnergyMetricFrom)
	}
	return spec.EnergyMetric, nil
}

func (tr *TemplateRenderer) resolveParentCoefficientTemplate(ctx context.Context, parent *chantico.ParentRef) (string, error) {
	if parent.CoefficientFrom != nil {
		return tr.renderTemplate(ctx, parent.CoefficientFrom)
	}
	if parent.Coefficient != "" {
		return parent.Coefficient, nil
	}
	return "", fmt.Errorf("no coefficient or coefficient template specified for parent %q", parent.Name)
}

func (tr *TemplateRenderer) ResolvedExpressions(ctx context.Context, dataCenterResource *chantico.DataCenterResource) (*ResolvedDataCenterResource, error) {
	energyMetric, err := tr.resolveEnergyMetricTemplate(ctx, dataCenterResource)
	if err != nil {
		return nil, err
	}

	var parents []ResolvedParent
	for _, parent := range dataCenterResource.Spec.Parents {
		coefficient, err := tr.resolveParentCoefficientTemplate(ctx, &parent)
		if err != nil {
			return nil, err
		}
		parents = append(parents, ResolvedParent{
			Name:        parent.Name,
			Coefficient: coefficient,
		})
	}

	return &ResolvedDataCenterResource{
		Name:             dataCenterResource.Name,
		Type:             dataCenterResource.Spec.Type,
		AdditionalLabels: dataCenterResource.Spec.AdditionalLabels,
		ServiceId:        dataCenterResource.Spec.ServiceId,
		EnergyMetric:     energyMetric,
		Parents:          parents,
	}, nil
}
