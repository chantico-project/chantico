package physicalmeasurement

import (
	chantico "chantico/api/v1alpha1"
	config "chantico/internal/configuration"
	sm "chantico/internal/statemachine"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestActionMap(t *testing.T) {
	for state, stateActions := range StateMachine.Actions {
		for _, action := range stateActions {
			t.Run(fmt.Sprintf("action %#v in state %#v", action.Type, state), func(t *testing.T) {
				switch action.Type {
				case sm.ActionFunctionPure:
					if action.IO != nil {
						t.Errorf("Pure action should not have IO: %#v", action)
					}
					if action.Pure == nil {
						t.Errorf("Pure action must have Pure function: %#v", action)
					}
				case sm.ActionFunctionIO:
					if action.IO == nil {
						t.Errorf("IO action must have IO function: %#v", action)
					}
					if action.Pure != nil {
						t.Errorf("IO action should not have Pure function: %#v", action)
					}
				default:
					t.Errorf("Unknown action type: %#v", action.Type)
				}
			})
		}
	}
}

func TestTargetFileAddition(t *testing.T) {
	testCases := map[string]struct {
		physicalMeasurement *chantico.PhysicalMeasurement
		expectedFiles       []string
	}{
		"target file created": {
			physicalMeasurement: &chantico.PhysicalMeasurement{
				ObjectMeta: metav1.ObjectMeta{
					UID:  "18ac6360-39e7-4ee3-a9b8-58992958e29a",
					Name: "physical_measurement",
				},
				Spec: chantico.PhysicalMeasurementSpec{
					MeasurementDevice: "device-a",
					Ip:                "192.168.1.10",
				},
			},
			expectedFiles: []string{
				"prometheus/targets/physical_measurement.json",
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			tmpDir := testCreateTmpDirectories(t)

			_ = WriteTargetFile(t.Context(), tc.physicalMeasurement)

			for _, expectedFile := range tc.expectedFiles {
				absPath := filepath.Join(tmpDir, expectedFile)
				_, err := os.Stat(absPath)
				if err != nil {
					t.Fatalf("Expected file %s to exist: %v\n", absPath, err)
				}

				// Verify the file is valid JSON with correct structure
				targets, err := LoadFileSDTargets(absPath)
				if err != nil {
					t.Fatalf("Failed to parse target file: %v\n", err)
				}
				if len(targets) != 1 {
					t.Fatalf("Expected 1 target group, got %d\n", len(targets))
				}
				if targets[0].Labels["__param_module"] != tc.physicalMeasurement.Spec.MeasurementDevice {
					t.Fatalf("Expected module label %s, got %s\n",
						tc.physicalMeasurement.Spec.MeasurementDevice,
						targets[0].Labels["__param_module"])
				}
			}

			targetsDir := filepath.Join(tmpDir, "prometheus/targets")
			observedFiles := []string{}
			_ = filepath.Walk(targetsDir, func(path string, info fs.FileInfo, err error) error {
				if path != targetsDir && !info.IsDir() {
					observedFiles = append(observedFiles, path)
				}
				return nil
			})
			if len(observedFiles) != len(tc.expectedFiles) {
				t.Fatalf("Mismatch: expected %v files, got %v\n", tc.expectedFiles, observedFiles)
			}
		})
	}
}

func TestTargetFileDeletion(t *testing.T) {
	testCases := map[string]struct {
		beforeFiles         []string
		physicalMeasurement *chantico.PhysicalMeasurement
		afterFiles          []string
	}{
		"target file deleted": {
			beforeFiles: []string{
				"prometheus/targets/physical_measurement.json",
			},
			physicalMeasurement: &chantico.PhysicalMeasurement{
				ObjectMeta: metav1.ObjectMeta{
					UID:  "18ac6360-39e7-4ee3-a9b8-58992958e29a",
					Name: "physical_measurement",
				},
			},
			afterFiles: []string{},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			tmpDir := testCreateTmpDirectories(t)

			for _, f := range tc.beforeFiles {
				_ = os.WriteFile(filepath.Join(tmpDir, f), []byte("[]"), 0755)
			}

			_ = DeleteTargetFile(t.Context(), tc.physicalMeasurement)

			for _, afterFile := range tc.afterFiles {
				absPath := filepath.Join(tmpDir, afterFile)
				_, err := os.Stat(absPath)
				if err != nil {
					t.Fatalf("Expected file %s to exist: %v\n", absPath, err)
				}
			}

			targetsDir := filepath.Join(tmpDir, "prometheus/targets")
			observedFiles := []string{}
			_ = filepath.Walk(targetsDir, func(path string, info fs.FileInfo, err error) error {
				if path != targetsDir && !info.IsDir() {
					observedFiles = append(observedFiles, path)
				}
				return nil
			})
			if len(observedFiles) != len(tc.afterFiles) {
				t.Fatalf("Mismatch: expected %v files, got %v\n", tc.afterFiles, observedFiles)
			}
		})
	}
}

func TestMultipleTargetFiles(t *testing.T) {
	testCases := map[string]struct {
		physicalMeasurements []*chantico.PhysicalMeasurement
		expectedFiles        int
		expectedTargets      map[string]string // filename -> expected device module label
	}{
		"two measurements for different devices": {
			physicalMeasurements: []*chantico.PhysicalMeasurement{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "measurement-1",
						UID:  "uid-1",
					},
					Spec: chantico.PhysicalMeasurementSpec{
						MeasurementDevice: "device-type-a",
						Ip:                "192.168.1.10",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "measurement-2",
						UID:  "uid-2",
					},
					Spec: chantico.PhysicalMeasurementSpec{
						MeasurementDevice: "device-type-b",
						Ip:                "192.168.1.20",
					},
				},
			},
			expectedFiles: 2,
			expectedTargets: map[string]string{
				"measurement-1.json": "device-type-a",
				"measurement-2.json": "device-type-b",
			},
		},
		"two measurements for same device": {
			physicalMeasurements: []*chantico.PhysicalMeasurement{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "measurement-1",
						UID:  "uid-1",
					},
					Spec: chantico.PhysicalMeasurementSpec{
						MeasurementDevice: "same-device",
						Ip:                "192.168.1.10",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "measurement-2",
						UID:  "uid-2",
					},
					Spec: chantico.PhysicalMeasurementSpec{
						MeasurementDevice: "same-device",
						Ip:                "192.168.1.20",
					},
				},
			},
			expectedFiles: 2,
			expectedTargets: map[string]string{
				"measurement-1.json": "same-device",
				"measurement-2.json": "same-device",
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			tmpDir := testCreateTmpDirectories(t)

			for _, pm := range tc.physicalMeasurements {
				WriteTargetFile(t.Context(), pm)
			}

			targetsDir := filepath.Join(tmpDir, "prometheus/targets")
			entries, err := os.ReadDir(targetsDir)
			if err != nil {
				t.Fatalf("Failed to read targets dir: %v", err)
			}

			jsonFiles := []string{}
			for _, e := range entries {
				if !e.IsDir() {
					jsonFiles = append(jsonFiles, e.Name())
				}
			}

			if len(jsonFiles) != tc.expectedFiles {
				t.Errorf("Expected %d files, got %d: %v", tc.expectedFiles, len(jsonFiles), jsonFiles)
			}

			// Verify each target file has the right device label
			for fileName, expectedModule := range tc.expectedTargets {
				filePath := filepath.Join(targetsDir, fileName)
				data, err := os.ReadFile(filePath)
				if err != nil {
					t.Errorf("Failed to read %s: %v", fileName, err)
					continue
				}

				var targets []FileSDTarget
				if err := json.Unmarshal(data, &targets); err != nil {
					t.Errorf("Failed to parse %s: %v", fileName, err)
					continue
				}

				if len(targets) != 1 {
					t.Errorf("Expected 1 target group in %s, got %d", fileName, len(targets))
					continue
				}

				if targets[0].Labels["__param_module"] != expectedModule {
					t.Errorf("In %s: expected module %s, got %s",
						fileName, expectedModule, targets[0].Labels["__param_module"])
				}
			}
		})
	}
}

func testCreateTmpDirectories(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	t.Setenv(config.ChanticoVolumeLocationEnv, tmpDir)
	config.ValidatedEnv, _ = config.ValidateEnv()

	for _, subDir := range []string{"prometheus/targets"} {
		subDirAbsPath := filepath.Join(tmpDir, subDir)
		err := os.MkdirAll(subDirAbsPath, 0755)
		if err != nil {
			t.Fatalf("Could not create directory %s\n", subDirAbsPath)
		}
	}

	return tmpDir
}
