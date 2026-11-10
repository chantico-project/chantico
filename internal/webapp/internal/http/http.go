package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"sigs.k8s.io/yaml"

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
	nodes, err := h.Kubernetes.GetDataCenterResources()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(graph.BuildElements(nodes))
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
	manifest, err := yaml.Marshal(dcr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pms, err := h.Kubernetes.ListPhysicalMeasurements(dcr.GetNamespace())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	mds, err := h.Kubernetes.ListMeasurementDevices(dcr.GetNamespace())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	mdByName := make(map[string]v1alpha1.MeasurementDevice, len(mds))
	for _, md := range mds {
		mdByName[md.GetName()] = md
	}

	var physicalMeasurement *html.PhysicalMeasurementRow
	// There is only one PhysicalMeasurement per namespace, so the first entry is the one.
	if len(dcr.Spec.PhysicalMeasurements) > 0 && len(pms) > 0 && pms[0].GetName() == dcr.Spec.PhysicalMeasurements[0] {
		pm := pms[0]
		row := html.PhysicalMeasurementRow{
			Name:              pm.GetName(),
			Type:              string(pm.Spec.Type),
			MeasurementDevice: pm.Spec.MeasurementDevice,
		}
		if md, ok := mdByName[pm.Spec.MeasurementDevice]; ok {
			row.Walks = md.Spec.Walks
		}
		physicalMeasurement = &row
	}

	parents := dcr.Spec.ParentNames()
	data := html.NodePartialData{
		ID:                  id,
		Name:                dcr.GetName(),
		Namespace:           dcr.GetNamespace(),
		Type:                dcr.Spec.Type,
		Parents:             parents,
		EnergyMetric:        dcr.Spec.EnergyMetric,
		PhysicalMeasurement: physicalMeasurement,
		Manifest:            string(manifest),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Renderer.RenderNodePartial(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *HTTPServerHandler) HomePage(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Handling home page request:", r.URL.Path)

	if _, err := h.Kubernetes.GetDataCenterResources(); err != nil {
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
	})
}
