package webapp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"chantico/internal/webapp/internal/html"
	"chantico/internal/webapp/internal/http"
	"chantico/internal/webapp/internal/kubernetes"
)

func New() (App, error) {
	return newWithGetter(os.Getenv)
}

func newWithGetter(get envGetter) (App, error) {
	slog.Info("loading environment variables")
	cfg, err := loadConfig(get)
	if err != nil {
		return nil, err
	}
	return &app{
		cfg: cfg,
	}, nil
}

func (a *app) Run() error {
	slog.Info("running startup checks")
	var errs []error

	k, err := kubernetes.New()
	if err != nil {
		errs = append(errs, err)
	}

	t, err := html.New()
	if err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	slog.Info("completed startup checks")

	ctxInterrupt, stopInterrupt := SignalHandling()
	defer stopInterrupt()

	handler := &http.HTTPServerHandler{
		Renderer:   t,
		Kubernetes: k,
	}
	serverConfig := &http.HTTPServerConfig{
		Port:              a.cfg.Port,
		ReadHeaderTimeout: 10 * time.Second,
	}
	httpServer := http.New(handler, serverConfig)
	httpServerErrChannel := make(chan error, 1)
	go httpServer.Run(httpServerErrChannel)

	select {
	case <-ctxInterrupt.Done():
		slog.Info("interrupt signal received, closing services within 30 seconds")
		ctxTimeout, stopTimeout := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopTimeout()

		return httpServer.Stop(ctxTimeout)

	case serverErr := <-httpServerErrChannel:
		slog.Error("http server failed, closing everything", "error", serverErr)
		return serverErr
	}
}

type App interface {
	Run() error
}

type app struct {
	cfg config
}

type config struct {
	Port int
}

type envGetter func(string) string

const DefaultPort = 8080

func loadConfig(get envGetter) (config, error) {
	port, err := loadPortConfig(get)
	if err != nil {
		return config{}, err
	}

	return config{
		Port: port,
	}, nil
}

func loadPortConfig(get envGetter) (int, error) {
	portStr := get("PORT")
	if portStr == "" {
		return DefaultPort, nil
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("PORT must be an integer between 1 and 65535, got %q", portStr)
	}
	return port, nil
}

func SignalHandling() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}
