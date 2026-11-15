package kube

import (
	"context"
	"strings"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestTriggerJobCopiesSpecWithoutControllerState(t *testing.T) {
	old := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "migrate", Namespace: "app", Labels: map[string]string{"app": "x", "job-name": "migrate"}},
		Spec: batchv1.JobSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"controller-uid": "u1"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "x", "controller-uid": "u1", "job-name": "migrate"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "m", Image: "img"}}},
			},
		},
	}
	cs := fake.NewSimpleClientset(old)
	name, err := NewWith(cs).TriggerJob(context.Background(), "app", "migrate")
	if err != nil || !strings.HasPrefix(name, "migrate-") || name == "migrate" {
		t.Fatalf("name %q err %v", name, err)
	}
	nj, _ := cs.BatchV1().Jobs("app").Get(context.Background(), name, metav1.GetOptions{})
	if nj.Spec.Selector != nil || nj.Spec.Template.Labels["controller-uid"] != "" || nj.Spec.Template.Labels["job-name"] != "" || nj.Labels["job-name"] != "" {
		t.Fatalf("controller state copied: %+v", nj)
	}
	if nj.Spec.Template.Labels["app"] != "x" || nj.Spec.Template.Spec.Containers[0].Image != "img" {
		t.Fatalf("spec not copied: %+v", nj)
	}
	if old.Spec.Selector == nil || old.Spec.Template.Labels["job-name"] == "" {
		t.Fatal("the original job was modified")
	}
}
