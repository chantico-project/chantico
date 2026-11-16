package webapp

/*
Copyright 2025-2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

import (
	"context"
	"errors"
	"net"
	"os"
	"syscall"
	"testing"

	"chantico/internal/webapp/internal/html"
	"chantico/internal/webapp/internal/http"
	"chantico/internal/webapp/internal/kubernetes"
)

func TestLoadConfigPort(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    int
		wantErr bool
	}{
		{name: "default", want: DefaultPort},
		{name: "explicit", env: map[string]string{"PORT": "9090"}, want: 9090},
		{name: "lower bound", env: map[string]string{"PORT": "1"}, want: 1},
		{name: "upper bound", env: map[string]string{"PORT": "65535"}, want: 65535},
		{name: "zero", env: map[string]string{"PORT": "0"}, wantErr: true},
		{name: "negative", env: map[string]string{"PORT": "-1"}, wantErr: true},
		{name: "not a number", env: map[string]string{"PORT": "abc"}, wantErr: true},
		{name: "out of range", env: map[string]string{"PORT": "70000"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg, err := loadConfig(func(key string) string { return test.env[key] })
			if (err != nil) != test.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, test.wantErr)
			}
			if cfg.Port != test.want {
				t.Fatalf("Port = %d, want %d", cfg.Port, test.want)
			}
		})
	}
}

func TestNewWithGetter(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    int
		wantErr bool
	}{
		{name: "default port", want: DefaultPort},
		{name: "custom port", env: map[string]string{"PORT": "7000"}, want: 7000},
		{name: "invalid port", env: map[string]string{"PORT": "x"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			a, err := newWithGetter(func(key string) string { return test.env[key] })
			if (err != nil) != test.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, test.wantErr)
			}
			if test.wantErr {
				if a != nil {
					t.Fatalf("app = %v, want nil on error", a)
				}
				return
			}
			impl, ok := a.(*app)
			if !ok {
				t.Fatalf("app type = %T, want *app", a)
			}
			if impl.cfg.Port != test.want {
				t.Fatalf("Port = %d, want %d", impl.cfg.Port, test.want)
			}
		})
	}
}

func TestNewUsesEnvironment(t *testing.T) {
	t.Setenv("PORT", "6543")

	a, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if got := a.(*app).cfg.Port; got != 6543 {
		t.Fatalf("Port = %d, want 6543", got)
	}
}

func TestSignalHandling(t *testing.T) {
	ctx, stop := SignalHandling()
	defer stop()

	if ctx.Err() != nil {
		t.Fatalf("context already done: %v", ctx.Err())
	}

	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess() error = %v", err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("Signal() error = %v", err)
	}

	<-ctx.Done()
	if ctx.Err() != context.Canceled {
		t.Fatalf("ctx.Err() = %v, want %v", ctx.Err(), context.Canceled)
	}
}

func TestRun(t *testing.T) {
	errKubernetes := errors.New("kubernetes unavailable")
	errRenderer := errors.New("renderer unavailable")

	okKubernetes := func() (*kubernetes.KubernetesClient, error) {
		return &kubernetes.KubernetesClient{}, nil
	}
	failKubernetes := func() (*kubernetes.KubernetesClient, error) {
		return nil, errKubernetes
	}
	okRenderer := func() (*html.TemplateRenderer, error) {
		return html.New()
	}
	failRenderer := func() (*html.TemplateRenderer, error) {
		return nil, errRenderer
	}
	interrupted := func() (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx, cancel
	}

	tests := []struct {
		name           string
		newKubernetes  func() (*kubernetes.KubernetesClient, error)
		newRenderer    func() (*html.TemplateRenderer, error)
		occupyPort     bool
		signalHandling func() (context.Context, context.CancelFunc)
		wantErr        bool
		wantIs         []error
	}{
		{
			name:           "kubernetes fails",
			newKubernetes:  failKubernetes,
			newRenderer:    okRenderer,
			signalHandling: interrupted,
			wantErr:        true,
			wantIs:         []error{errKubernetes},
		},
		{
			name:           "renderer fails",
			newKubernetes:  okKubernetes,
			newRenderer:    failRenderer,
			signalHandling: interrupted,
			wantErr:        true,
			wantIs:         []error{errRenderer},
		},
		{
			name:           "both fail",
			newKubernetes:  failKubernetes,
			newRenderer:    failRenderer,
			signalHandling: interrupted,
			wantErr:        true,
			wantIs:         []error{errKubernetes, errRenderer},
		},
		{
			name:           "port already in use",
			newKubernetes:  okKubernetes,
			newRenderer:    okRenderer,
			occupyPort:     true,
			signalHandling: func() (context.Context, context.CancelFunc) { return context.WithCancel(context.Background()) },
			wantErr:        true,
		},
		{
			name:           "interrupt stops server cleanly",
			newKubernetes:  okKubernetes,
			newRenderer:    okRenderer,
			signalHandling: interrupted,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			port := 0
			if test.occupyPort {
				ln, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatalf("listen: %v", err)
				}
				defer ln.Close()
				port = ln.Addr().(*net.TCPAddr).Port
			}

			a := &app{
				cfg:            config{Port: port},
				newKubernetes:  test.newKubernetes,
				newRenderer:    test.newRenderer,
				newHTTPServer:  http.New,
				signalHandling: test.signalHandling,
			}

			err := a.Run()
			if (err != nil) != test.wantErr {
				t.Fatalf("Run() error = %v, wantErr %v", err, test.wantErr)
			}
			for _, want := range test.wantIs {
				if !errors.Is(err, want) {
					t.Errorf("Run() error = %v, want it to wrap %v", err, want)
				}
			}
		})
	}
}
