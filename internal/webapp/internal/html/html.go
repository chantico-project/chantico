package html

import (
	"embed"
	"html/template"
	"io"
)

//go:embed templates/*.html
var fs embed.FS

type TemplateRenderer struct {
	tmpl *template.Template
}

func New() (*TemplateRenderer, error) {
	tmpl, err := template.ParseFS(fs, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &TemplateRenderer{
		tmpl: tmpl,
	}, nil
}

type HomePageData struct {
	Diagram        string
	Host           string
	CurrentContext string
}

func (r *TemplateRenderer) RenderHomePage(w io.Writer, d HomePageData) {
	_ = r.tmpl.ExecuteTemplate(w, "home.html", d)
}

type ErrorPageData struct {
	Error          string
	Host           string
	CurrentContext string
}

func (r *TemplateRenderer) RenderErrorPage(w io.Writer, d ErrorPageData) {
	_ = r.tmpl.ExecuteTemplate(w, "error.html", d)
}
