package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"sigs.k8s.io/yaml"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"chantico/api/v1alpha1"
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
	mux.HandleFunc("GET /api/graph", h.GraphAPI)
	mux.HandleFunc("GET /api/namespaces", h.NamespacesAPI)
	mux.HandleFunc("GET /api/nodes/suggest", h.SuggestAPI)
	mux.HandleFunc("GET /api/nodes/{id}", h.NodeAPI)
	mux.HandleFunc("PUT /api/nodes/{id}", h.UpdateNodeAPI)
	mux.HandleFunc("DELETE /api/nodes/{id}", h.DeleteNodeAPI)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(html.StaticFS()))))

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

func (h *HTTPServerHandler) GraphAPI(w http.ResponseWriter, r *http.Request) {
	namespace := r.URL.Query().Get("namespace")
	nodes, err := h.Kubernetes.GetDataCenterResources(namespace)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if name := r.URL.Query().Get("node"); name != "" {
		nodes = graph.Neighbourhood(nodes, namespace+"-"+name)
		if nodes == nil {
			http.Error(w, "node not found", http.StatusNotFound)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(graph.BuildElements(nodes))
}

func (h *HTTPServerHandler) NamespacesAPI(w http.ResponseWriter, r *http.Request) {
	namespaces, err := h.Kubernetes.ListDataCenterNamespaces()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(namespaces)
}

func (h *HTTPServerHandler) NodeAPI(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	dcr, err := h.Kubernetes.GetDataCenterResource(id)
	if errors.Is(err, kubernetes.ErrNotFound) {
		http.Error(w, "node not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.renderNode(w, id, dcr)
}

const suggestionLimit = 10

func (h *HTTPServerHandler) SuggestAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	if query == "" {
		return
	}

	nodes, err := h.Kubernetes.GetDataCenterResources(r.URL.Query().Get("namespace"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var items []html.SuggestionItem
	for _, node := range nodes {
		if strings.Contains(strings.ToLower(node.Label), query) {
			items = append(items, html.SuggestionItem{ID: node.Name, Label: node.Label})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Label < items[j].Label })
	if len(items) > suggestionLimit {
		items = items[:suggestionLimit]
	}

	if err := h.Renderer.RenderSuggestionsPartial(w, html.SuggestionsPartialData{Items: items}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *HTTPServerHandler) UpdateNodeAPI(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	dcr, err := h.Kubernetes.UpdateDataCenterResource(id, []byte(r.PostFormValue("manifest")))
	if errors.Is(err, kubernetes.ErrNotFound) {
		http.Error(w, "node not found", http.StatusNotFound)
		return
	}
	if errors.Is(err, kubernetes.ErrInvalidManifest) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.renderNode(w, id, dcr)
}

func (h *HTTPServerHandler) DeleteNodeAPI(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	err := h.Kubernetes.DeleteDataCenterResource(id)
	if errors.Is(err, kubernetes.ErrNotFound) {
		http.Error(w, "node not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "node-deleted")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<h2>Selected node</h2><p>None</p>`))

}

func (h *HTTPServerHandler) renderNode(w http.ResponseWriter, id string, dcr *v1alpha1.DataCenterResource) {
	editable := dcr.DeepCopy()
	editable.ManagedFields = nil
	editable.CreationTimestamp = metav1.Time{}
	editable.UID = ""
	editable.ResourceVersion = ""
	editable.Generation = 0
	editable.Finalizers = nil
	editable.Status = v1alpha1.DataCenterResourceStatus{}

	manifest, err := yaml.Marshal(editable)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	parents := dcr.Spec.ParentNames()
	data := html.NodePartialData{
		ID:           id,
		Name:         dcr.GetName(),
		Namespace:    dcr.GetNamespace(),
		Type:         dcr.Spec.Type,
		Parents:      parents,
		EnergyMetric: dcr.Spec.EnergyMetric,
		Status:       kubernetes.NodeStatus(dcr.Status.Conditions),
		Manifest:     string(manifest),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Renderer.RenderNodePartial(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *HTTPServerHandler) HomePage(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Handling home page request:", r.URL.Path)

	namespaces, err := h.Kubernetes.ListDataCenterNamespaces()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.Renderer.RenderErrorPage(w, html.ErrorPageData{
			Host:           h.Kubernetes.Host,
			CurrentContext: h.Kubernetes.CurrentContext,
			Error:          err.Error(),
		})
		return
	}

	h.Renderer.RenderHomePage(w, html.HomePageData{
		Host:           h.Kubernetes.Host,
		CurrentContext: h.Kubernetes.CurrentContext,
		Namespaces:     namespaces,
	})
}
