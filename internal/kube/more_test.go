package kube

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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

func TestNodesReadinessPressureCapacityAndRequests(t *testing.T) {
	q := resource.MustParse
	node := func(name string, ready corev1.ConditionStatus, extra ...corev1.NodeCondition) *corev1.Node {
		return &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"node-role.kubernetes.io/control-plane": ""}},
			Status: corev1.NodeStatus{
				NodeInfo:    corev1.NodeSystemInfo{KubeletVersion: "v1.30.1"},
				Conditions:  append([]corev1.NodeCondition{{Type: corev1.NodeReady, Status: ready}}, extra...),
				Allocatable: corev1.ResourceList{corev1.ResourceCPU: q("3920m"), corev1.ResourceMemory: q("14Gi"), corev1.ResourcePods: q("110")},
			},
		}
	}
	pod := func(name, nodeName string, phase corev1.PodPhase, cpu, mem string) *corev1.Pod {
		return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "app"},
			Spec:   corev1.PodSpec{NodeName: nodeName, Containers: []corev1.Container{{Name: "c", Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: q(cpu), corev1.ResourceMemory: q(mem)}}}}},
			Status: corev1.PodStatus{Phase: phase}}
	}
	cs := fake.NewSimpleClientset(
		node("n2", corev1.ConditionFalse, corev1.NodeCondition{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionTrue}),
		node("n1", corev1.ConditionTrue, corev1.NodeCondition{Type: corev1.NodeDiskPressure, Status: corev1.ConditionFalse}),
		pod("a", "n1", corev1.PodRunning, "500m", "1Gi"),
		pod("b", "n1", corev1.PodPending, "250m", "512Mi"),
		pod("done", "n1", corev1.PodSucceeded, "1", "1Gi"), // finished pods do not count
		pod("c", "n2", corev1.PodRunning, "1", "2Gi"),
		pod("unscheduled", "", corev1.PodPending, "1", "1Gi"),
	)
	res, err := NewWith(cs).Nodes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Nodes) != 2 || res.Nodes[0].Name != "n1" || res.Nodes[1].Name != "n2" || !res.Requests || res.Metrics {
		t.Fatalf("%+v", res)
	}
	n1, n2 := res.Nodes[0], res.Nodes[1]
	if n1.Status != "Ready" || len(n1.Pressure) != 0 || n1.Roles[0] != "control-plane" || n1.Version != "v1.30.1" {
		t.Fatalf("n1: %+v", n1)
	}
	if n2.Status != "NotReady" || len(n2.Pressure) != 1 || n2.Pressure[0] != "MemoryPressure" {
		t.Fatalf("n2: %+v", n2)
	}
	if n1.CPUAllocMilli != 3920 || n1.MemAllocBytes != 14<<30 || n1.PodsAlloc != 110 {
		t.Fatalf("allocatable: %+v", n1)
	}
	if n1.Pods != 2 || n1.CPUReqMilli != 750 || n1.MemReqBytes != 1536<<20 || n2.Pods != 1 || n2.CPUReqMilli != 1000 {
		t.Fatalf("requests: n1 %+v n2 %+v", n1, n2)
	}
	if n1.CPUUseMilli != nil {
		t.Fatal("no metrics-server: no live use")
	}
}

func TestParseNodeMetrics(t *testing.T) {
	m, err := parseNodeMetrics([]byte(`{"items":[{"metadata":{"name":"n1"},"usage":{"cpu":"1250m","memory":"6Gi"}},{"metadata":{"name":"n2"},"usage":{"cpu":"250000000n","memory":"1024Ki"}}]}`))
	if err != nil || m["n1"] != [2]int64{1250, 6 << 30} || m["n2"] != [2]int64{250, 1 << 20} {
		t.Fatalf("%v %v", m, err)
	}
}

func TestStopClusterClosesOnlyThatConnectionsForwards(t *testing.T) {
	f := NewForwards()
	add := func(id, cluster string) chan struct{} {
		ch := make(chan struct{})
		f.m[id] = &Forward{ID: id, Cluster: cluster, stop: ch}
		return ch
	}
	d1, d2, s1 := add("1", "dev"), add("2", "dev"), add("3", "stg")
	closed := func(ch chan struct{}) bool {
		select {
		case <-ch:
			return true
		default:
			return false
		}
	}
	if n := f.StopCluster("dev"); n != 2 || !closed(d1) || !closed(d2) || closed(s1) || len(f.List()) != 1 {
		t.Fatalf("stopped %d, left %v", n, f.List())
	}
	if n := f.StopCluster("nope"); n != 0 {
		t.Fatalf("an unknown connection stops nothing, got %d", n)
	}
	if n := f.StopCluster(""); n != 1 || !closed(s1) || len(f.List()) != 0 {
		t.Fatalf("all: stopped %d, left %v", n, f.List())
	}
}
