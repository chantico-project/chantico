package k8s

import (
	"bytes"
	"context"
	"fmt"
	"text/template"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TemplateParameter struct {
	Name                    string `json:"name"`
	SecretConfigMapSelector `json:",inline,omitempty"`
}

type TemplateFrom struct {
	ValueSource `json:",inline,omitempty"`
	// +optional
	Parameters []TemplateParameter `json:"parameters,omitempty"`
}

func (p *TemplateParameter) Resolve(ctx context.Context, r client.Client, namespace string) (string, error) {
	return p.SecretConfigMapSelector.Resolve(ctx, r, namespace)
}

func (p *TemplateFrom) ResolveTemplate(ctx context.Context, r client.Client, namespace string) (string, error) {
	return p.ValueSource.Resolve(ctx, r, namespace)
}

func (p *TemplateFrom) ResolveParameters(ctx context.Context, r client.Client, namespace string) (map[string]string, error) {
	result := make(map[string]string)
	for _, param := range p.Parameters {
		value, err := param.Resolve(ctx, r, namespace)
		if err != nil {
			return nil, err
		}
		result[param.Name] = value
	}
	return result, nil
}

func (tf *TemplateFrom) Render(ctx context.Context, client client.Client, namespace string) (string, error) {
	templateText, err := tf.ResolveTemplate(ctx, client, namespace)
	if err != nil {
		return "", err
	}

	parameters, err := tf.ResolveParameters(ctx, client, namespace)
	if err != nil {
		return "", err
	}

	// Apply the template with the resolved parameters
	tmpl, err := template.New(tf.ConfigMapKeyRef.Name).Option("missingkey=error").Parse(templateText)
	if err != nil {
		return "", fmt.Errorf("parse template from ConfigMap %q: %w", tf.ConfigMapKeyRef.Name, err)
	}
	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, parameters); err != nil {
		return "", fmt.Errorf("render template from ConfigMap %q: %w", tf.ConfigMapKeyRef.Name, err)
	}
	return rendered.String(), nil
}
