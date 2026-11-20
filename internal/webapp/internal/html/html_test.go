package html

import (
	"bytes"
	"io/fs"
	"strings"
	"testing"
)

func newRenderer(t *testing.T) *TemplateRenderer {
	t.Helper()
	r, err := New()
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	return r
}

func TestNew(t *testing.T) {
	r := newRenderer(t)
	for _, name := range []string{"home.html", "node.html", "error.html"} {
		if r.tmpl.Lookup(name) == nil {
			t.Errorf("New() did not parse template %q", name)
		}
	}
}

func TestStaticFS(t *testing.T) {
	sub := StaticFS()
	for _, name := range []string{"style.css", "graph.js", "chantico-dark.png"} {
		if _, err := fs.Stat(sub, name); err != nil {
			t.Errorf("StaticFS() missing %q: %v", name, err)
		}
	}
}

func TestRenderHomePage(t *testing.T) {
	r := newRenderer(t)

	var buf bytes.Buffer
	r.RenderHomePage(&buf, HomePageData{
		Host:           "https://cluster.example",
		CurrentContext: "kind-chantico",
		Namespaces:     []string{"alpha", "beta"},
	})
	out := buf.String()

	for _, want := range []string{
		"https://cluster.example",
		"kind-chantico",
		`<option value="alpha">alpha</option>`,
		`<option value="beta">beta</option>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("RenderHomePage() output missing %q", want)
		}
	}
}

func TestRenderHomePageNoNamespaces(t *testing.T) {
	r := newRenderer(t)

	var buf bytes.Buffer
	r.RenderHomePage(&buf, HomePageData{Host: "h", CurrentContext: "c"})

	if strings.Contains(buf.String(), "<option") {
		t.Errorf("RenderHomePage() with no namespaces rendered options")
	}
}

func TestRenderNodePartial(t *testing.T) {
	r := newRenderer(t)

	var buf bytes.Buffer
	err := r.RenderNodePartial(&buf, NodePartialData{
		ID:           "ns1-pdu-a",
		Name:         "pdu-a",
		Namespace:    "ns1",
		Type:         "pdu",
		Status:       "ok",
		EnergyMetric: "metric",
		Manifest:     "kind: DataCenterResource",
	})
	if err != nil {
		t.Fatalf("RenderNodePartial() unexpected error: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"<h2>pdu-a</h2>",
		"<strong>ok</strong>",
		`hx-put="/api/nodes/ns1-pdu-a"`,
		`hx-delete="/api/nodes/ns1-pdu-a"`,
		"kind: DataCenterResource",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("RenderNodePartial() output missing %q", want)
		}
	}
}

func TestRenderErrorPage(t *testing.T) {
	r := newRenderer(t)

	var buf bytes.Buffer
	r.RenderErrorPage(&buf, ErrorPageData{
		Error:          "connection refused",
		Host:           "https://cluster.example",
		CurrentContext: "kind-chantico",
	})
	out := buf.String()

	for _, want := range []string{"connection refused", "https://cluster.example", "kind-chantico"} {
		if !strings.Contains(out, want) {
			t.Errorf("RenderErrorPage() output missing %q", want)
		}
	}
}
