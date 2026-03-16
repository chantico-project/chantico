package measurementdevice

// import (
// 	"context"
// 	"reflect"
// 	"testing"

// 	chantico "chantico/api/v1alpha1"
// 	"go.yaml.in/yaml/v2"
// 	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
// 	"k8s.io/client-go/kubernetes/fake"
// )

// func TestMakeJob(t *testing.T) {
// 	// This is an experiment with a kubernetes fake client for test purposes
// 	// TODO: make relevant tests
// 	client := fake.NewSimpleClientset()

// 	// Define test metadata
// 	device := chantico.MeasurementDevice{
// 		Status:     chantico.MeasurementDeviceStatus{JobName: "foo"},
// 		ObjectMeta: metav1.ObjectMeta{Namespace: "bar"},
// 	}

// 	// Create job object
// 	job := MakeJob(device)

// 	// Create job in fake client
// 	_, err := client.BatchV1().Jobs(device.ObjectMeta.Namespace).Create(
// 		context.TODO(),
// 		job,
// 		metav1.CreateOptions{},
// 	)
// 	if err != nil {
// 		t.Fatalf("Failed to create job: %v", err)
// 	}
// }

// func TestGenerateSnmpConfig(t *testing.T) {
// 	testCases := map[string]struct {
// 		Case     chantico.MeasurementDevice
// 		Expected []byte
// 	}{
// 		"single measurement device": {
// 			Case: chantico.MeasurementDevice{
// 				ObjectMeta: metav1.ObjectMeta{Name: "test"},
// 				Spec:       chantico.MeasurementDeviceSpec{Auth: chantico.Auth{Version: 3, Username: "guest"}, Walks: []string{"foo", "bar"}},
// 			},
// 			Expected: []byte(`
// auths:
//   test:
//     version: 3
//     username: guest
// modules:
//   test:
//     walk:
//     - foo
//     - bar
// `),
// 		},
// 	}

// 	// Check that the generated yaml is valid
// 	for name, tc := range testCases {
// 		t.Run(name, func(t *testing.T) {
// 			var out any
// 			generatedYaml, err := GenerateSNMPGeneratorConfig(tc.Case)
// 			if err != nil {
// 				t.Fatalf("GenerateSNMPGeneratorConfig could not produce a valid yaml file, got err=%s\n", err)
// 			}
// 			err = yaml.Unmarshal([]byte(generatedYaml), &out)
// 			if err != nil {
// 				t.Fatalf("The generated config is not a valid YAML file: \n%s\n", generatedYaml)
// 			}

// 			var expected any
// 			err = yaml.Unmarshal(tc.Expected, &expected)
// 			if err != nil {
// 				t.Fatalf("The expected yaml is not a valid YAML file: %s\n%s\n", err, tc.Expected)
// 			}
// 			if !reflect.DeepEqual(expected, out) {
// 				t.Fatalf("The GenerateSNMPGeneratorConfig(tc.Case) != tc.Output, \n%s\n, got=%s\n", expected, out)
// 			}
// 		})
// 	}
// }

// func TestMergeSNMPConfigs(t *testing.T) {
// 	testCases := map[string]struct {
// 		Files      [][]byte
// 		OutputFile []byte
// 	}{
// 		"two files to merge": {
// 			Files: [][]byte{
// 				[]byte(
// 					`
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
//         type: gauge`,
// 				),
// 				[]byte(
// 					`
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
//         type: gauge`,
// 				),
// 			},
// 			OutputFile: []byte(
// 				`
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
//         type: gauge`,
// 			),
// 		},
// 	}

// 	// Check that the generated yaml is valid
// 	for name, tc := range testCases {
// 		t.Run(name, func(t *testing.T) {
// 			mergedYamlString, err := MergeSNMPConfigs(tc.Files)
// 			if err != nil {
// 				t.Fatalf("MergeSNMPConfigs could not merge the config files %#v, got err=%s\n", tc.Files, err.Error())
// 			}

// 			var mergedYaml any
// 			err = yaml.Unmarshal([]byte(mergedYamlString), &mergedYaml)
// 			if err != nil {
// 				t.Fatalf("MergeSNMPConfigs could not produce a valid yaml file, got err=%s\n", err)
// 			}

// 			var outputYaml any
// 			err = yaml.Unmarshal(tc.OutputFile, &outputYaml)
// 			if err != nil {
// 				t.Fatalf("The test case is not valid json: %s\n", outputYaml)
// 			}
// 			if !reflect.DeepEqual(mergedYaml, outputYaml) {
// 				t.Fatalf("MergedSNMPConfigs(tc.Files) != tc.Output file expected \n%s,\n got=%s\n", mergedYaml, outputYaml)
// 			}
// 		})
// 	}

// }
