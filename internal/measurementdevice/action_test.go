package measurementdevice

// import (
// 	"fmt"
// 	"io/fs"
// 	"os"
// 	"reflect"
// 	"testing"
// 	"time"

// 	"path/filepath"

// 	"go.yaml.in/yaml/v2"

// 	chantico "chantico/api/v1alpha1"
// 	ph "chantico/internal/patch"
// 	vol "chantico/internal/volumes"

// 	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
// )

// const (
// 	yamlSNMPConfigFoo = `
// auths:
//   foo:
//     version: 3
//     username: guest
// modules:
//   foo:
//     walk:
//     - 1.3.6.1.4.1.31034.12.1.1.2.7.2
//     metrics:
//     - name: sdbDevOutMtIndex
//       oid: 1.3.6.1.4.1.31034.12.1.1.2.7.2.1.1
//       type: gauge
//       help: A unique value for each outlet - 1.3.6.1.4.1.31034.12.1.1.2.7.2.1.1
//       indexes:
//       - labelname: sdbDevIdIndex
//         type: gauge
//       - labelname: sdbDevOutMtIndex
//         type: gauge`

// 	yamlSNMPConfigBar = `
// auths:
//   bar:
//     version: 3
//     username: guest
// modules:
//   bar:
//     walk:
//     - 1.3.6.1.4.1.31034.12.1.1.2.7.2.1.7
//     metrics:
//     - name: sdbDevOutMtActualVoltage
//       oid: 1.3.6.1.4.1.31034.12.1.1.2.7.2.1.7
//       type: gauge
//       help: Actual voltage on outlet. - 1.3.6.1.4.1.31034.12.1.1.2.7.2.1.7
//       indexes:
//       - labelname: sdbDevIdIndex
//         type: gauge
//       - labelname: sdbDevOutMtIndex
//         type: gauge`
// )

// func equalStringSlices(a, b []string) bool {
// 	if len(a) != len(b) {
// 		return false
// 	}
// 	for i := range a {
// 		if a[i] != b[i] {
// 			return false
// 		}
// 	}
// 	return true
// }

// func TestInitializeFinalizer(t *testing.T) {
// 	testCases := map[string]struct {
// 		Case               *chantico.MeasurementDevice
// 		ExpectedPatchType  ph.PatchType
// 		ExpectedFinalizers []string
// 	}{
// 		"empty finalizer": {
// 			Case: &chantico.MeasurementDevice{
// 				ObjectMeta: metav1.ObjectMeta{
// 					Finalizers: []string{},
// 				}},
// 			ExpectedPatchType:  ph.PatchResource,
// 			ExpectedFinalizers: []string{chantico.SNMPUpdateFinalizer},
// 		},
// 		"already initialized": {
// 			Case: &chantico.MeasurementDevice{
// 				ObjectMeta: metav1.ObjectMeta{
// 					Finalizers: []string{"test"},
// 				}},
// 			ExpectedPatchType:  ph.PatchResource,
// 			ExpectedFinalizers: []string{"test", chantico.SNMPUpdateFinalizer},
// 		},
// 		"already contains": {
// 			Case: &chantico.MeasurementDevice{
// 				ObjectMeta: metav1.ObjectMeta{
// 					Finalizers: []string{chantico.SNMPUpdateFinalizer},
// 				}},
// 			ExpectedPatchType:  ph.PatchResourceNone,
// 			ExpectedFinalizers: []string{chantico.SNMPUpdateFinalizer},
// 		},
// 	}

// 	for name, tc := range testCases {
// 		t.Run(name, func(t *testing.T) {
// 			result := InitializeFinalizer(tc.Case)
// 			if result == nil && tc.ExpectedPatchType != ph.PatchResourceNone {
// 				t.Errorf("InitializeFinalizer(%#v) = %#v, want %#v\n", tc, result, tc.ExpectedPatchType)
// 			}
// 			if result != nil && result.PatchType != tc.ExpectedPatchType {
// 				t.Errorf("InitializeFinalizer(%#v) = %#v, want %#v\n", tc, result, tc.ExpectedPatchType)
// 			}
// 			if !equalStringSlices(tc.ExpectedFinalizers, tc.Case.ObjectMeta.Finalizers) {
// 				t.Errorf("InitializeFinalizer(%#v) = %#v -> %#v, want %#v -> %#v\n", tc, result, tc.Case.ObjectMeta.Finalizers, tc.ExpectedPatchType, tc.ExpectedFinalizers)
// 			}
// 		})
// 	}
// }

// func TestUpdateFinalizer(t *testing.T) {
// 	testCases := map[string]struct {
// 		Case               *chantico.MeasurementDevice
// 		ExpectedPatchType  ph.PatchType
// 		ExpectedFinalizers []string
// 	}{
// 		"removes SNMPUpdateFinalizer on deletion": {
// 			Case: &chantico.MeasurementDevice{
// 				ObjectMeta: metav1.ObjectMeta{
// 					DeletionTimestamp: &metav1.Time{Time: time.Now()},
// 					Finalizers:        []string{"test", chantico.SNMPUpdateFinalizer},
// 				},
// 			},
// 			ExpectedPatchType:  ph.PatchResource,
// 			ExpectedFinalizers: []string{"test"},
// 		},
// 		"retains SNMPUpdateFinalizer when not deletion": {
// 			Case: &chantico.MeasurementDevice{
// 				ObjectMeta: metav1.ObjectMeta{
// 					Finalizers: []string{chantico.SNMPUpdateFinalizer},
// 				},
// 			},
// 			ExpectedPatchType:  ph.PatchResourceNone,
// 			ExpectedFinalizers: []string{chantico.SNMPUpdateFinalizer},
// 		},
// 	}

// 	for name, tc := range testCases {
// 		t.Run(name, func(t *testing.T) {
// 			result := UpdateFinalizer(tc.Case)
// 			if result == nil && tc.ExpectedPatchType != ph.PatchResourceNone {
// 				t.Errorf("UpdateFinalizer(%#v) = %#v, want %#v\n", tc, result, tc.ExpectedPatchType)
// 			}
// 			if result != nil && result.PatchType != tc.ExpectedPatchType {
// 				t.Errorf("UpdateFinalizer(%#v) = %#v, want %#v\n", tc, result, tc.ExpectedPatchType)
// 			}
// 			if !equalStringSlices(tc.ExpectedFinalizers, tc.Case.ObjectMeta.Finalizers) {
// 				t.Errorf("UpdateFinalizer(%#v) = %#v -> %#v, want %#v -> %#v\n", tc, result, tc.Case.ObjectMeta.Finalizers, tc.ExpectedPatchType, tc.ExpectedFinalizers)
// 			}
// 		})
// 	}
// }

// func TestUpdateModification(t *testing.T) {
// 	testCases := map[string]struct {
// 		Case     *chantico.MeasurementDevice
// 		Expected int64
// 	}{
// 		"copies generation to status": {
// 			Case: &chantico.MeasurementDevice{
// 				ObjectMeta: metav1.ObjectMeta{
// 					Generation: 5,
// 				},
// 			},
// 			Expected: 5,
// 		},
// 	}

// 	for name, tc := range testCases {
// 		t.Run(name, func(t *testing.T) {
// 			UpdateModification(tc.Case)
// 			if tc.Case.Status.UpdateGeneration != tc.Expected {
// 				t.Errorf("UpdateModification(%#v) = %#v, want %#v\n", tc.Case, tc.Case.Status.UpdateGeneration, tc.Expected)
// 			}
// 		})
// 	}
// }

// func TestRequeueWithDelay(t *testing.T) {
// 	testCases := map[string]struct {
// 		Case     *chantico.MeasurementDevice
// 		Expected time.Duration
// 	}{
// 		"default requeue delay": {
// 			Case:     nil,
// 			Expected: chantico.RequeueDelay,
// 		},
// 	}

// 	for name, tc := range testCases {
// 		t.Run(name, func(t *testing.T) {
// 			result := RequeueWithDelay(tc.Case)
// 			if result.RequeueAfter != tc.Expected {
// 				t.Errorf("RequeueWithDelay() = %#v, want %#v", result.RequeueAfter, tc.Expected)
// 			}
// 		})
// 	}
// }

// func TestActionMap(t *testing.T) {
// 	for state, stateActions := range ActionMap {
// 		for _, action := range stateActions {
// 			t.Run(fmt.Sprintf("action %#v in state %#v", action.Type, state), func(t *testing.T) {
// 				switch action.Type {
// 				case ActionFunctionPure:
// 					if action.IO != nil {
// 						t.Errorf("Pure action should not have IO: %#v", action)
// 					}
// 					if action.Pure == nil {
// 						t.Errorf("Pure action must have Pure function: %#v", action)
// 					}
// 				case ActionFunctionIO:
// 					if action.IO == nil {
// 						t.Errorf("IO action must have IO function: %#v", action)
// 					}
// 					if action.Pure != nil {
// 						t.Errorf("IO action should not have Pure function: %#v", action)
// 					}
// 				default:
// 					t.Errorf("Unknown action type: %#v", action.Type)
// 				}
// 			})
// 		}
// 	}
// }

// func TestCreateSNMPGenerator(t *testing.T) {
// 	testCases := map[string]struct {
// 		Case *chantico.MeasurementDevice
// 	}{
// 		"default requeue delay": {
// 			Case: &chantico.MeasurementDevice{ObjectMeta: metav1.ObjectMeta{UID: "8cc3100d-538a-401c-ad5a-49d54fa45e57"}},
// 		},
// 	}

// 	for name, tc := range testCases {
// 		t.Run(name, func(t *testing.T) {
// 			// Set up the temporary directory
// 			tmpDir := t.TempDir()
// 			t.Setenv(vol.ChanticoVolumeLocationEnv, tmpDir)
// 			tmpSNMPConfigDir := fmt.Sprintf("%s/%s", tmpDir, snmpConfigDir)
// 			err := os.MkdirAll(tmpSNMPConfigDir, 0755)
// 			if err != nil {
// 				t.Fatalf("Could not create folder %s\n", tmpSNMPConfigDir)
// 			}

// 			// Run the function
// 			_ = CreateSNMPGenerator(tc.Case)

// 			// Check that the file exist
// 			yamlFile := fmt.Sprintf("%s/generator_%s.yml", tmpSNMPConfigDir, string(tc.Case.GetUID()))
// 			if _, err = os.Stat(yamlFile); err != nil {
// 				t.Fatalf("yamlFile: %s does not exist\n", yamlFile)
// 			}

// 			yamlFileBytes, err := os.ReadFile(yamlFile)
// 			if err != nil {
// 				t.Fatalf("Could not load yamlFile: %s\n", yamlFile)
// 			}

// 			// Check that it is a valid yaml
// 			var expected any
// 			err = yaml.Unmarshal(yamlFileBytes, &expected)
// 			if err != nil {
// 				t.Fatalf("The expected yaml is not a valid YAML file: %s\n", yamlFile)
// 			}
// 		})
// 	}
// }

// func TestDeleteSNMPConfig(t *testing.T) {
// 	testCases := map[string]struct {
// 		BeforeDeletionFiles []string
// 		MeasurementDevice   *chantico.MeasurementDevice
// 		AfterDeletionFiles  []string
// 	}{
// 		"files present": {
// 			BeforeDeletionFiles: []string{
// 				"snmp/config/generator_18ac6360-39e7-4ee3-a9b8-58992958e29a.yml",
// 				"snmp/config/config_18ac6360-39e7-4ee3-a9b8-58992958e29a.yml",
// 				"snmp/config/generator_36eab0e9-60f9-4fa9-beb3-f68834322f6b.yml",
// 				"snmp/config/config_36eab0e9-60f9-4fa9-beb3-f68834322f6b.yml",
// 			},
// 			MeasurementDevice: &chantico.MeasurementDevice{ObjectMeta: metav1.ObjectMeta{UID: "18ac6360-39e7-4ee3-a9b8-58992958e29a"}},
// 			AfterDeletionFiles: []string{
// 				"snmp/config/generator_36eab0e9-60f9-4fa9-beb3-f68834322f6b.yml",
// 				"snmp/config/config_36eab0e9-60f9-4fa9-beb3-f68834322f6b.yml",
// 			},
// 		},
// 		"file non-present": {
// 			BeforeDeletionFiles: []string{},
// 			MeasurementDevice:   &chantico.MeasurementDevice{ObjectMeta: metav1.ObjectMeta{UID: "18ac6360-39e7-4ee3-a9b8-58992958e29a"}},
// 			AfterDeletionFiles:  []string{},
// 		},
// 	}

// 	for name, tc := range testCases {
// 		t.Run(name, func(t *testing.T) {
// 			// Create temporary directory
// 			tmpSNMPDir := testCreateTmpSNMPDirectories(t)
// 			// Create files
// 			for _, beforeDeletionFile := range tc.BeforeDeletionFiles {
// 				os.WriteFile(
// 					filepath.Join(tmpSNMPDir, beforeDeletionFile),
// 					[]byte{},
// 					0755,
// 				)
// 			}

// 			// Call function
// 			_ = DeleteSNMPConfig(tc.MeasurementDevice)

// 			// Check that the file exist
// 			for _, afterDeletionFile := range tc.AfterDeletionFiles {
// 				afterDeletionAbsPath := filepath.Join(tmpSNMPDir, afterDeletionFile)
// 				_, err := os.Stat(afterDeletionAbsPath)
// 				if err != nil {
// 					t.Fatalf("Error with file %s\n", afterDeletionAbsPath)
// 				}
// 			}
// 			observedAfterDeletionFiles := []string{}
// 			filepath.Walk(filepath.Join(tmpSNMPDir, snmpConfigDir), func(path string, info fs.FileInfo, err error) error {
// 				if path != filepath.Join(tmpSNMPDir, snmpConfigDir) {
// 					observedAfterDeletionFiles = append(observedAfterDeletionFiles, path)
// 				}
// 				return nil
// 			})
// 			if len(observedAfterDeletionFiles) != len(tc.AfterDeletionFiles) {
// 				t.Fatalf("Mismatch after deletion files expected: %v, got %v\n", tc.AfterDeletionFiles, observedAfterDeletionFiles)
// 			}
// 		})
// 	}
// }

// func TestCreateSNMPDeploymentConfig(t *testing.T) {
// 	var err error

// 	testCases := map[string]struct {
// 		Case                  [][]byte
// 		SnmpMergedConfigBytes []byte
// 	}{
// 		"Two files case": {
// 			Case: [][]byte{
// 				[]byte(yamlSNMPConfigFoo),
// 				[]byte(yamlSNMPConfigBar),
// 			},
// 			SnmpMergedConfigBytes: []byte(`
// auths:
//   foo:
//     version: 3
//     username: guest
//   bar:
//     version: 3
//     username: guest
// modules:
//   foo:
//     walk:
//     - 1.3.6.1.4.1.31034.12.1.1.2.7.2
//     metrics:
//     - name: sdbDevOutMtIndex
//       oid: 1.3.6.1.4.1.31034.12.1.1.2.7.2.1.1
//       type: gauge
//       help: A unique value for each outlet - 1.3.6.1.4.1.31034.12.1.1.2.7.2.1.1
//       indexes:
//       - labelname: sdbDevIdIndex
//         type: gauge
//       - labelname: sdbDevOutMtIndex
//         type: gauge
//   bar:
//     walk:
//     - 1.3.6.1.4.1.31034.12.1.1.2.7.2.1.7
//     metrics:
//     - name: sdbDevOutMtActualVoltage
//       oid: 1.3.6.1.4.1.31034.12.1.1.2.7.2.1.7
//       type: gauge
//       help: Actual voltage on outlet. - 1.3.6.1.4.1.31034.12.1.1.2.7.2.1.7
//       indexes:
//       - labelname: sdbDevIdIndex
//         type: gauge
//       - labelname: sdbDevOutMtIndex
//         type: gauge`),
// 		},
// 	}

// 	for name, tc := range testCases {
// 		t.Run(name, func(t *testing.T) {
// 			// Set up the temporary directory
// 			tmpSNMPDir := testCreateTmpSNMPDirectories(t)

// 			// Save config to disk
// 			for i, bytes := range tc.Case {
// 				snmpConfigPath := filepath.Join(
// 					tmpSNMPDir,
// 					snmpConfigDir,
// 					fmt.Sprintf("config_%d.yml", i),
// 				)
// 				err := os.WriteFile(snmpConfigPath, bytes, 0755)
// 				if err != nil {
// 					t.Fatalf("Could not write %s to disk", snmpConfigPath)
// 				}
// 			}

// 			// Create SNMP deployment config
// 			_ = CreateSNMPDeploymentConfig(nil)

// 			snmpMergedConfigPath := filepath.Join(tmpSNMPDir, snmpYmlDir, "snmp.yml")

// 			_, err = os.Stat(snmpMergedConfigPath)
// 			if err != nil {
// 				t.Fatalf("%s was not created\n", snmpMergedConfigPath)
// 			}

// 			// Check if the yaml file are similar
// 			var snmpMergedConfigGoInterface any
// 			snmpMergedConfigBytes, err := os.ReadFile(snmpMergedConfigPath)
// 			if err != nil {
// 				t.Fatalf("Could not read file %s\n", snmpMergedConfigPath)
// 			}
// 			err = yaml.Unmarshal(snmpMergedConfigBytes, &snmpMergedConfigGoInterface)
// 			if err != nil {
// 				t.Fatalf("CreateSNMPDeploymentConfig could not produce a valid yaml file, got err=%s\n", err)
// 			}

// 			var expectYaml any
// 			err = yaml.Unmarshal(tc.SnmpMergedConfigBytes, &expectYaml)
// 			if err != nil {
// 				t.Fatalf("tc.SnmpMergedConfigBytes is not valid yaml: %s\n", string(tc.SnmpMergedConfigBytes))
// 			}
// 			if !reflect.DeepEqual(snmpMergedConfigGoInterface, expectYaml) {
// 				t.Fatalf("CreateSNMPDeploymentConfig(nil) != tc.MergedConfigBytes \n%s,\n got=%s\n", snmpMergedConfigGoInterface, expectYaml)
// 			}

// 		})
// 	}
// }

// func testCreateTmpSNMPDirectories(t *testing.T) string {
// 	t.Helper()

// 	// Set environment
// 	tmpSNMPDir := t.TempDir()
// 	t.Setenv(vol.ChanticoVolumeLocationEnv, tmpSNMPDir)

// 	// Create SNMP sudirectory
// 	for _, snmpSubDir := range []string{snmpConfigDir, snmpYmlDir, snmpMibsDir} {
// 		snmpSubDirAbsPath := filepath.Join(tmpSNMPDir, snmpSubDir)
// 		err := os.MkdirAll(snmpSubDirAbsPath, 0755)
// 		if err != nil {
// 			t.Fatalf("Could not create directory %s\n", snmpSubDirAbsPath)
// 		}
// 	}

// 	return tmpSNMPDir
// }
