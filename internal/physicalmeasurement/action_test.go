package physicalmeasurement

import (
	chantico "chantico/api/v1alpha1"
	vol "chantico/internal/volumes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestActionMap(t *testing.T) {
	for state, stateActions := range ActionMap {
		for _, action := range stateActions {
			t.Run(fmt.Sprintf("action %#v in state %#v", action.Type, state), func(t *testing.T) {
				switch action.Type {
				case ActionFunctionPure:
					if action.IO != nil {
						t.Errorf("Pure action should not have IO: %#v", action)
					}
					if action.Pure == nil {
						t.Errorf("Pure action must have Pure function: %#v", action)
					}
				case ActionFunctionIO:
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

func TestConfigAddition(t *testing.T) {
	testCases := map[string]struct {
		BeforeAdditionFiles []string
		physicalMeasurement *chantico.PhysicalMeasurement
		AfterAdditionFiles  []string
	}{
		"files present": {
			BeforeAdditionFiles: []string{},
			physicalMeasurement: &chantico.PhysicalMeasurement{ObjectMeta: metav1.ObjectMeta{UID: "18ac6360-39e7-4ee3-a9b8-58992958e29a", Name: "physical_measurement"}},
			AfterAdditionFiles: []string{
				"prometheus/yml/physical_measurement.yml",
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// Create temporary directory
			tmpDir := testCreateTmpDirectories(t)

			// Call function
			_ = WriteConfigFile(tc.physicalMeasurement)

			// Check that the file exist
			for _, afterAdditionFile := range tc.AfterAdditionFiles {
				afterAdditionAbsPath := filepath.Join(tmpDir, afterAdditionFile)
				_, err := os.Stat(afterAdditionAbsPath)
				if err != nil {
					t.Fatalf("Error with file %s\n", afterAdditionAbsPath)
				}
			}

			ymlDir := filepath.Join(tmpDir, "prometheus/yml")
			observedAfterAdditionFiles := []string{}
			filepath.Walk(ymlDir, func(path string, info fs.FileInfo, err error) error {
				if path != ymlDir {
					observedAfterAdditionFiles = append(observedAfterAdditionFiles, path)
				}
				return nil
			})
			if len(observedAfterAdditionFiles) != len(tc.AfterAdditionFiles) {
				t.Fatalf("Mismatch after addition files expected: %v, got %v\n", tc.AfterAdditionFiles, observedAfterAdditionFiles)
			}
		})
	}
}

func TestConfigDeletion(t *testing.T) {
	testCases := map[string]struct {
		BeforeDeletionFiles []string
		physicalMeasurement *chantico.PhysicalMeasurement
		AfterDeletionFiles  []string
	}{
		"files present": {
			BeforeDeletionFiles: []string{
				"prometheus/yml/physical_measurement.yml",
			},
			physicalMeasurement: &chantico.PhysicalMeasurement{ObjectMeta: metav1.ObjectMeta{UID: "18ac6360-39e7-4ee3-a9b8-58992958e29a", Name: "physical_measurement"}},
			AfterDeletionFiles:  []string{},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// Create temporary directory
			tmpDir := testCreateTmpDirectories(t)
			// Create files
			for _, beforeDeletionFile := range tc.BeforeDeletionFiles {
				os.WriteFile(
					filepath.Join(tmpDir, beforeDeletionFile),
					[]byte{},
					0755,
				)
			}

			// Call function
			_ = DeleteConfigFile(tc.physicalMeasurement)

			// Check that the file exist
			for _, afterDeletionFile := range tc.AfterDeletionFiles {
				afterDeletionAbsPath := filepath.Join(tmpDir, afterDeletionFile)
				_, err := os.Stat(afterDeletionAbsPath)
				if err != nil {
					t.Fatalf("Error with file %s\n", afterDeletionAbsPath)
				}
			}

			ymlDir := filepath.Join(tmpDir, "prometheus/yml")
			observedAfterDeletionFiles := []string{}
			filepath.Walk(ymlDir, func(path string, info fs.FileInfo, err error) error {
				if path != ymlDir {
					observedAfterDeletionFiles = append(observedAfterDeletionFiles, path)
				}
				return nil
			})
			if len(observedAfterDeletionFiles) != len(tc.AfterDeletionFiles) {
				t.Fatalf("Mismatch after deletion files expected: %v, got %v\n", tc.AfterDeletionFiles, observedAfterDeletionFiles)
			}
		})
	}
}

func TestCombineConfigFiles(t *testing.T) {
	testCases := map[string]struct {
		physicalMeasurements []*chantico.PhysicalMeasurement
		expectedJobNames     []string
		expectedTargets      map[string][]string // job_name -> targets
	}{
		"two configs with different jobs": {
			physicalMeasurements: []*chantico.PhysicalMeasurement{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "measurement-1",
						UID:  "uid-1",
					},
					Spec: chantico.PhysicalMeasurementSpec{
						SNMPDevice: "device-type-a",
						Ip:         "192.168.1.10",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "measurement-2",
						UID:  "uid-2",
					},
					Spec: chantico.PhysicalMeasurementSpec{
						SNMPDevice: "device-type-b",
						Ip:         "192.168.1.20",
					},
				},
			},
			expectedJobNames: []string{"device-type-a", "device-type-b"},
			expectedTargets: map[string][]string{
				"device-type-a": {"192.168.1.10"},
				"device-type-b": {"192.168.1.20"},
			},
		},
		"two configs with same job merged": {
			physicalMeasurements: []*chantico.PhysicalMeasurement{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "measurement-1",
						UID:  "uid-1",
					},
					Spec: chantico.PhysicalMeasurementSpec{
						SNMPDevice: "same-device",
						Ip:         "192.168.1.10",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "measurement-2",
						UID:  "uid-2",
					},
					Spec: chantico.PhysicalMeasurementSpec{
						SNMPDevice: "same-device",
						Ip:         "192.168.1.20",
					},
				},
			},
			expectedJobNames: []string{"same-device"},
			expectedTargets: map[string][]string{
				"same-device": {"192.168.1.10", "192.168.1.20"},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// Create temporary directory
			tmpDir := testCreateTmpDirectories(t)

			// Write config files for each physical measurement
			for _, pm := range tc.physicalMeasurements {
				WriteConfigFile(pm)
			}

			// Combine all config files into prometheus.yml
			CombineConfigFiles(nil)

			// Read and verify prometheus.yml content
			prometheusYmlPath := filepath.Join(tmpDir, "prometheus/yml/prometheus.yml")
			config, err := LoadPrometheusConfig(prometheusYmlPath)
			if err != nil {
				t.Fatalf("Failed to load prometheus.yml: %v", err)
			}

			// Check that all expected jobs are present
			foundJobs := make(map[string]bool)
			for _, scrape := range config.ScrapeConfigs {
				foundJobs[scrape.JobName] = true

				// Verify targets for this job
				if expectedTargets, ok := tc.expectedTargets[scrape.JobName]; ok {
					actualTargets := scrape.StaticConfigs[0].Targets
					for _, expectedTarget := range expectedTargets {
						found := false
						for _, actualTarget := range actualTargets {
							if actualTarget == expectedTarget {
								found = true
								break
							}
						}
						if !found {
							t.Errorf("Expected target %s not found in job %s, got targets: %v",
								expectedTarget, scrape.JobName, actualTargets)
						}
					}
				}
			}

			// Verify all expected job names are present
			for _, expectedJobName := range tc.expectedJobNames {
				if !foundJobs[expectedJobName] {
					t.Errorf("Expected job %s not found in prometheus.yml", expectedJobName)
				}
			}

			// Verify no unexpected jobs
			if len(config.ScrapeConfigs) != len(tc.expectedJobNames) {
				t.Errorf("Expected %d jobs, got %d", len(tc.expectedJobNames), len(config.ScrapeConfigs))
			}
		})
	}
}

func testCreateTmpDirectories(t *testing.T) string {
	t.Helper()

	// Set environment
	tmpDir := t.TempDir()
	t.Setenv(vol.ChanticoVolumeLocationEnv, tmpDir)

	// Create Prometheus sudirectory
	for _, subDir := range []string{"prometheus/yml"} {
		subDirAbsPath := filepath.Join(tmpDir, subDir)
		err := os.MkdirAll(subDirAbsPath, 0755)
		if err != nil {
			t.Fatalf("Could not create directory %s\n", subDirAbsPath)
		}
	}

	return tmpDir
}
