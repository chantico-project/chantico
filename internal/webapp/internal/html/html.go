package html

import (
	"embed"
	"html/template"
	"io"
	"io/fs"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

func StaticFS() fs.FS {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	return sub
}

type TemplateRenderer struct {
	tmpl *template.Template
}

func New() (*TemplateRenderer, error) {
	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &TemplateRenderer{
		tmpl: tmpl,
	}, nil
}

type HomePageData struct {
	Host           string
	CurrentContext string
}

func (r *TemplateRenderer) RenderHomePage(w io.Writer, d HomePageData) {
	_ = r.tmpl.ExecuteTemplate(w, "home.html", d)
}

type PhysicalMeasurementRow struct {
	Name              string
	Type              string
	MeasurementDevice string
	Walks             []string
}

type NodePartialData struct {
	ID                  string
	Name                string
	Namespace           string
	Type                string
	Parents             []string
	EnergyMetric        string
	PhysicalMeasurement *PhysicalMeasurementRow
	Manifest            string
}

func (r *TemplateRenderer) RenderNodePartial(w io.Writer, d NodePartialData) error {
	return r.tmpl.ExecuteTemplate(w, "node.html", d)
}

type ErrorPageData struct {
	Error          string
	Host           string
	CurrentContext string
}

func (r *TemplateRenderer) RenderErrorPage(w io.Writer, d ErrorPageData) {
	_ = r.tmpl.ExecuteTemplate(w, "error.html", d)
}
