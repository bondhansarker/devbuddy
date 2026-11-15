package kube

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDescribeShowsWhyAContainerRestartedAndItsEvents(t *testing.T) {
	now := metav1.NewTime(time.Now())
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "app", CreationTimestamp: now, OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "web-abc"}}},
		Spec:       corev1.PodSpec{NodeName: "n1", Containers: []corev1.Container{{Name: "app", Image: "img:1"}}},
		Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{
			Name: "app", RestartCount: 3,
			State:                corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
			LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled", ExitCode: 137, FinishedAt: now}},
		}}},
	}
	ev := &corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "e1", Namespace: "app"}, InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "web"},
		Type: "Warning", Reason: "BackOff", Message: "Back-off restarting failed container", Count: 5, LastTimestamp: now}
	other := &corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "e2", Namespace: "app"}, InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "other"}, Reason: "Nope"}
	d, err := NewWith(fake.NewSimpleClientset(pod, ev, other)).Describe(context.Background(), "app", "web")
	if err != nil {
		t.Fatal(err)
	}
	c := d.Containers[0]
	if d.Owner != "ReplicaSet/web-abc" || d.Node != "n1" || c.State.Reason != "CrashLoopBackOff" || c.LastState == nil || c.LastState.Reason != "OOMKilled" || *c.LastState.ExitCode != 137 || c.RestartCount != 3 {
		t.Fatalf("describe: %+v", d)
	}
	if len(d.Events) != 1 || d.Events[0].Reason != "BackOff" || d.Events[0].Count != 5 {
		t.Fatalf("events (only this pod's): %+v", d.Events)
	}
}

func TestScaleAndRestartDeployment(t *testing.T) {
	one := int32(1)
	cs := fake.NewSimpleClientset(&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "app"}, Spec: appsv1.DeploymentSpec{Replicas: &one}})
	c := NewWith(cs)
	ctx := context.Background()
	if err := c.ScaleDeployment(ctx, "app", "web", 4); err != nil {
		t.Fatal(err)
	}
	if err := c.ScaleDeployment(ctx, "app", "web", -1); err == nil {
		t.Fatal("negative replicas must be refused")
	}
	if err := c.RestartDeployment(ctx, "app", "web"); err != nil {
		t.Fatal(err)
	}
	d, _ := cs.AppsV1().Deployments("app").Get(ctx, "web", metav1.GetOptions{})
	if *d.Spec.Replicas != 4 || d.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] == "" {
		t.Fatalf("got replicas %d, annotations %v", *d.Spec.Replicas, d.Spec.Template.Annotations)
	}
}

func TestParsePodMetricsSumsContainers(t *testing.T) {
	raw := []byte(`{"items":[{"metadata":{"name":"web","namespace":"app"},"containers":[{"usage":{"cpu":"250m","memory":"64Mi"}},{"usage":{"cpu":"1","memory":"1Gi"}}]}]}`)
	l, err := parsePodMetrics(raw)
	if err != nil || len(l) != 1 || l[0].CPUMilli != 1250 || l[0].MemBytes != 64<<20+1<<30 {
		t.Fatalf("got %+v err %v", l, err)
	}
}

func TestPodMetricsWithoutARealClient(t *testing.T) {
	if _, err := NewWith(fake.NewSimpleClientset()).PodMetrics(context.Background(), ""); err != ErrNoMetrics {
		t.Fatalf("want ErrNoMetrics, got %v", err)
	}
}

func TestCreateAndDeleteNamespace(t *testing.T) {
	cs := fake.NewSimpleClientset()
	c := NewWith(cs)
	ctx := context.Background()
	if err := c.CreateNamespace(ctx, "team-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := cs.CoreV1().Namespaces().Get(ctx, "team-a", metav1.GetOptions{}); err != nil {
		t.Fatal("not created:", err)
	}
	for _, bad := range []string{"", "Team", "-a", "a-", "a_b", "a.b", strings.Repeat("a", 64)} {
		if err := c.CreateNamespace(ctx, bad); !errors.Is(err, ErrBadNamespace) {
			t.Errorf("create %q: want ErrBadNamespace, got %v", bad, err)
		}
	}
	for _, p := range []string{"default", "kube-system", "kube-public", "kube-node-lease"} {
		if err := c.DeleteNamespace(ctx, p); !errors.Is(err, ErrProtectedNamespace) {
			t.Errorf("delete %q: want ErrProtectedNamespace, got %v", p, err)
		}
	}
	if err := c.DeleteNamespace(ctx, "team-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := cs.CoreV1().Namespaces().Get(ctx, "team-a", metav1.GetOptions{}); err == nil {
		t.Fatal("still there")
	}
}
