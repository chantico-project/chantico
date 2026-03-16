package measurementdevice

// import (
// 	"encoding/json"
// 	"os"
// 	"testing"

// 	appsv1 "k8s.io/api/apps/v1"
// 	batchv1 "k8s.io/api/batch/v1"
// 	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// 	chantico "chantico/api/v1alpha1"
// )

// func TestGetState(t *testing.T) {
// 	testCases := map[string]struct {
// 		MeasurementDevice *chantico.MeasurementDevice
// 		JobPath           string
// 		Deployment        appsv1.Deployment
// 		Expected          string
// 	}{
// 		"empty state": {
// 			MeasurementDevice: &chantico.MeasurementDevice{
// 				Status: chantico.MeasurementDeviceStatus{
// 					State: "",
// 				},
// 			},
// 			JobPath:  "./testdata/states/job_failed.json",
// 			Expected: StateInit,
// 		},
// 		"nil device": {
// 			MeasurementDevice: nil,
// 			JobPath:           "./testdata/states/job_failed.json",
// 			Expected:          StateEndPoint,
// 		},
// 		"failed case": {
// 			MeasurementDevice: &chantico.MeasurementDevice{
// 				ObjectMeta: v1.ObjectMeta{
// 					Finalizers: []string{chantico.SNMPUpdateFinalizer},
// 				},
// 				Status: chantico.MeasurementDeviceStatus{
// 					State: StatePendingSNMPConfigUpdate,
// 				},
// 			},
// 			Expected: StateFailed,
// 			JobPath:  "./testdata/states/job_failed.json",
// 		},
// 	}

// 	for name, tc := range testCases {
// 		t.Run(name, func(t *testing.T) {
// 			// Read the json file as
// 			jobBytes, err := os.ReadFile(tc.JobPath)
// 			if err != nil {
// 				t.Fatalf("error: %s", err)
// 			}

// 			var job batchv1.Job
// 			err = json.Unmarshal(jobBytes, &job)
// 			if err != nil {
// 				t.Fatalf("error: %s", err)
// 			}

// 			UpdateState(tc.MeasurementDevice, &job)
// 			if tc.MeasurementDevice == nil {
// 				return
// 			}
// 			if tc.MeasurementDevice.Status.State != tc.Expected {
// 				t.Errorf("GetState(%v) = %v, want %v", tc.MeasurementDevice, tc.MeasurementDevice.Status.State, tc.Expected)
// 			}
// 		})
// 	}
// }
