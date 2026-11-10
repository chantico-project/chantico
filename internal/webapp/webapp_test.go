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

import "testing"

func TestLoadConfigPort(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    int
		wantErr bool
	}{
		{name: "default", want: DefaultPort},
		{name: "explicit", env: map[string]string{"PORT": "9090"}, want: 9090},
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
