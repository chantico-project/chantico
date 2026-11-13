package measurementdevice

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// JobLogReader reads the log of the pod that completed a Job.
type JobLogReader interface {
	GetJobLogs(ctx context.Context, job *batchv1.Job) ([]byte, error)
}

// ClientsetJobLogReader reads Job logs through the API server, as the cached client cannot read pod logs.
type ClientsetJobLogReader struct {
	Clientset kubernetes.Interface
}

func (r ClientsetJobLogReader) GetJobLogs(ctx context.Context, job *batchv1.Job) ([]byte, error) {
	pods, err := r.Clientset.CoreV1().Pods(job.GetNamespace()).List(ctx, metav1.ListOptions{
		LabelSelector: batchv1.ControllerUidLabel + "=" + string(job.GetUID()),
	})
	if err != nil {
		return nil, fmt.Errorf("list pods of job %s: %w", job.GetName(), err)
	}
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Status.Phase != corev1.PodSucceeded {
			continue
		}
		logs, err := r.Clientset.CoreV1().Pods(pod.GetNamespace()).GetLogs(pod.GetName(), &corev1.PodLogOptions{}).DoRaw(ctx)
		if err != nil {
			return nil, fmt.Errorf("read log of pod %s: %w", pod.GetName(), err)
		}
		return logs, nil
	}
	return nil, fmt.Errorf("no succeeded pod found for job %s", job.GetName())
}
