package measurementdevice

import (
	"context"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func jobPod(name, jobUID string, phase corev1.PodPhase) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "chantico",
			Labels:    map[string]string{batchv1.ControllerUidLabel: jobUID},
		},
		Status: corev1.PodStatus{Phase: phase},
	}
}

func TestClientsetJobLogReader(t *testing.T) {
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "tno", Namespace: "chantico", UID: types.UID("job-1")}}

	t.Run("reads the log of the succeeded pod of the job", func(t *testing.T) {
		reader := ClientsetJobLogReader{Clientset: fake.NewClientset(
			jobPod("other-job", "job-2", corev1.PodSucceeded),
			jobPod("failed", "job-1", corev1.PodFailed),
			jobPod("succeeded", "job-1", corev1.PodSucceeded),
		)}
		got, err := reader.GetJobLogs(context.Background(), job)
		if err != nil {
			t.Fatalf("JobLogs: %v", err)
		}
		if string(got) != "fake logs" { // log returned by the fake clientset
			t.Fatalf("JobLogs = %q", got)
		}
	})

	t.Run("errors without a succeeded pod of the job", func(t *testing.T) {
		reader := ClientsetJobLogReader{Clientset: fake.NewClientset(
			jobPod("other-job", "job-2", corev1.PodSucceeded),
			jobPod("running", "job-1", corev1.PodRunning),
		)}
		if _, err := reader.GetJobLogs(context.Background(), job); err == nil {
			t.Fatal("expected an error")
		}
	})
}
