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
	SecretConfigMapSelector `json:",inline,omitempty"`
	// +optional
	TemplateParameters []TemplateParameter `json:"templateParameters,omitempty"`
}

func (p *TemplateParameter) Resolve(ctx context.Context, r client.Client, namespace string) (string, error) {
	return p.SecretConfigMapSelector.Resolve(ctx, r, namespace)
}

// Gets the template string from either the Value or the ValueFrom field of the TemplateFrom struct.
func (p *TemplateFrom) ResolveTemplate(ctx context.Context, r client.Client, namespace string) (string, error) {
	if p.Value != "" {
		return p.Value, nil
	}
	return p.ValueFrom.Resolve(ctx, r, namespace)
}

// Resolves all template parameters into values the renderer can substitute into the template.
func (p *TemplateFrom) ResolveTemplateParameters(ctx context.Context, r client.Client, namespace string) (map[string]string, error) {
	result := make(map[string]string)
	for _, param := range p.TemplateParameters {
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

	parameters, err := tf.ResolveTemplateParameters(ctx, client, namespace)
	if err != nil {
		return "", err
	}

	if len(parameters) == 0 {
		// Short circuit if there are no template parameters to substitute
		return templateText, nil
	} else {
		// Apply the template with the resolved parameters
		tmpl, err := template.New("template").Option("missingkey=error").Parse(templateText)
		if err != nil {
			return "", fmt.Errorf("parse template: %w", err)
		}
		var rendered bytes.Buffer
		if err := tmpl.Execute(&rendered, parameters); err != nil {
			return "", fmt.Errorf("render template: %w", err)
		}
		return rendered.String(), nil
	}
}
