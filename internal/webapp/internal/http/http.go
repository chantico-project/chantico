package http

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"chantico/internal/webapp/internal/graph"
	"chantico/internal/webapp/internal/html"
	"chantico/internal/webapp/internal/kubernetes"
)

type HTTPServer struct {
	server *http.Server
	port   int
}

type HTTPServerHandler struct {
	Renderer   *html.TemplateRenderer
	Kubernetes *kubernetes.KubernetesClient
}

type HTTPServerConfig struct {
	Port              int
	ReadHeaderTimeout time.Duration
}

func New(h *HTTPServerHandler, config *HTTPServerConfig) *HTTPServer {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", h.HomePage)

	server := &http.Server{
		Addr:              ":" + strconv.Itoa(config.Port),
		Handler:           mux,
		ReadHeaderTimeout: config.ReadHeaderTimeout,
	}

	return &HTTPServer{
		server: server,
		port:   config.Port,
	}
}

func (s HTTPServer) Run(errChannel chan<- error) {
	fmt.Println("Starting HTTP server on port:", s.port)
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Printf("Error in HTTP server")
		errChannel <- err
	}
}

func (s HTTPServer) Stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (h *HTTPServerHandler) HomePage(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Handling home page request:", r.URL.Path)

	nodes, err := h.Kubernetes.GetDataCenterResources()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.Renderer.RenderErrorPage(w, html.ErrorPageData{
			Host:           h.Kubernetes.Host,
			CurrentContext: h.Kubernetes.CurrentContext,
			Error:          err.Error(),
		})
		return
	}

	m := graph.GenerateMermaidString(nodes)
	h.Renderer.RenderHomePage(w, html.HomePageData{
		Host:           h.Kubernetes.Host,
		CurrentContext: h.Kubernetes.CurrentContext,
		Diagram:        m,
	})
}
