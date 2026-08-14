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

func New(r *html.TemplateRenderer, k *kubernetes.KubernetesClient, port int, readHeaderTimeout time.Duration) *HTTPServer {
	h := &Handler{
		renderer:   r,
		kubernetes: k,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", h.HomePage)

	server := &http.Server{
		Addr:              ":" + strconv.Itoa(port),
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	return &HTTPServer{
		server: server,
		port:   port,
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

type Handler struct {
	renderer   *html.TemplateRenderer
	kubernetes *kubernetes.KubernetesClient
}

func (h *Handler) HomePage(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Handling home page request:", r.URL.Path)

	nodes, err := h.kubernetes.GetDataCenterResources()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.renderer.RenderErrorPage(w, html.ErrorPageData{
			Host:           h.kubernetes.Host,
			CurrentContext: h.kubernetes.CurrentContext,
			Error:          err.Error(),
		})
		return
	}

	m := graph.GenerateMermaidString(nodes)
	h.renderer.RenderHomePage(w, html.HomePageData{
		Host:           h.kubernetes.Host,
		CurrentContext: h.kubernetes.CurrentContext,
		Diagram:        m,
	})
}
