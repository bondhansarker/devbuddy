package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
)

// ---- describe: what `kubectl describe pod` is read for, as JSON

type Condition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

type ContainerState struct {
	State    string `json:"state"`
	Reason   string `json:"reason,omitempty"`
	Message  string `json:"message,omitempty"`
	ExitCode *int32 `json:"exit_code,omitempty"`
	Since    string `json:"since,omitempty"`
}

type ContainerInfo struct {
	Name         string            `json:"name"`
	Image        string            `json:"image"`
	Ready        bool              `json:"ready"`
	RestartCount int32             `json:"restart_count"`
	State        ContainerState    `json:"state"`
	LastState    *ContainerState   `json:"last_state,omitempty"`
	Requests     map[string]string `json:"requests,omitempty"`
	Limits       map[string]string `json:"limits,omitempty"`
	Ports        []int32           `json:"ports,omitempty"`
}

type Event struct {
	Type     string `json:"type"`
	Reason   string `json:"reason"`
	Message  string `json:"message"`
	Count    int32  `json:"count"`
	Source   string `json:"source,omitempty"`
	LastSeen string `json:"last_seen"`
}

type Description struct {
	Name       string            `json:"name"`
	Namespace  string            `json:"namespace"`
	Node       string            `json:"node,omitempty"`
	Phase      string            `json:"phase"`
	Reason     string            `json:"reason,omitempty"`
	Message    string            `json:"message,omitempty"`
	PodIP      string            `json:"pod_ip,omitempty"`
	QOS        string            `json:"qos_class,omitempty"`
	Age        string            `json:"age"`
	Owner      string            `json:"owner,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	Conditions []Condition       `json:"conditions"`
	Containers []ContainerInfo   `json:"containers"`
	Events     []Event           `json:"events"`
}

func stateOf(s corev1.ContainerState) ContainerState {
	switch {
	case s.Running != nil:
		return ContainerState{State: "running", Since: age(s.Running.StartedAt.Time)}
	case s.Terminated != nil:
		t := s.Terminated
		code := t.ExitCode
		return ContainerState{State: "terminated", Reason: t.Reason, Message: t.Message, ExitCode: &code, Since: age(t.FinishedAt.Time)}
	case s.Waiting != nil:
		return ContainerState{State: "waiting", Reason: s.Waiting.Reason, Message: s.Waiting.Message}
	}
	return ContainerState{State: "unknown"}
}

func quantities(l corev1.ResourceList) map[string]string {
	if len(l) == 0 {
		return nil
	}
	out := map[string]string{}
	for k, v := range l {
		out[string(k)] = v.String()
	}
	return out
}

// Describe gathers a pod's status, container states (with why the last one ended) and its events.
func (c *Client) Describe(ctx context.Context, ns, name string) (Description, error) {
	p, err := c.cs.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return Description{}, err
	}
	d := Description{Name: p.Name, Namespace: p.Namespace, Node: p.Spec.NodeName, Phase: string(p.Status.Phase),
		Reason: p.Status.Reason, Message: p.Status.Message, PodIP: p.Status.PodIP, QOS: string(p.Status.QOSClass),
		Age: age(p.CreationTimestamp.Time), Labels: p.Labels, Conditions: []Condition{}, Containers: []ContainerInfo{}, Events: []Event{}}
	if len(p.OwnerReferences) > 0 {
		d.Owner = p.OwnerReferences[0].Kind + "/" + p.OwnerReferences[0].Name
	}
	for _, cd := range p.Status.Conditions {
		d.Conditions = append(d.Conditions, Condition{string(cd.Type), string(cd.Status), cd.Reason, cd.Message})
	}
	status := map[string]corev1.ContainerStatus{}
	for _, cs := range p.Status.ContainerStatuses {
		status[cs.Name] = cs
	}
	for _, ct := range p.Spec.Containers {
		ci := ContainerInfo{Name: ct.Name, Image: ct.Image, Requests: quantities(ct.Resources.Requests), Limits: quantities(ct.Resources.Limits)}
		for _, port := range ct.Ports {
			ci.Ports = append(ci.Ports, port.ContainerPort)
		}
		if cs, ok := status[ct.Name]; ok {
			ci.Ready, ci.RestartCount, ci.State = cs.Ready, cs.RestartCount, stateOf(cs.State)
			if cs.LastTerminationState.Terminated != nil {
				ls := stateOf(cs.LastTerminationState)
				ci.LastState = &ls
			}
		} else {
			ci.State = ContainerState{State: "waiting"}
		}
		d.Containers = append(d.Containers, ci)
	}
	// events are best effort: a role without access to them still gets the rest
	evs, err := c.cs.CoreV1().Events(ns).List(ctx, metav1.ListOptions{FieldSelector: "involvedObject.name=" + name + ",involvedObject.kind=Pod"})
	if err == nil {
		items := evs.Items[:0:0]
		for _, e := range evs.Items {
			if e.InvolvedObject.Name == name { // the fake clientset ignores field selectors
				items = append(items, e)
			}
		}
		last := func(e corev1.Event) time.Time {
			if !e.LastTimestamp.IsZero() {
				return e.LastTimestamp.Time
			}
			if !e.EventTime.IsZero() {
				return e.EventTime.Time
			}
			return e.CreationTimestamp.Time
		}
		sort.Slice(items, func(i, j int) bool { return last(items[i]).After(last(items[j])) })
		for _, e := range items {
			src := e.Source.Component
			if src == "" {
				src = e.ReportingController
			}
			count := e.Count
			if count == 0 {
				count = 1
			}
			d.Events = append(d.Events, Event{e.Type, e.Reason, e.Message, count, src, age(last(e))})
		}
	}
	return d, nil
}

// ---- metrics (metrics-server): optional, a cluster may not have it

// ErrNoMetrics means the cluster has no metrics API (or this client cannot reach it).
var ErrNoMetrics = errors.New("metrics are not available (no metrics-server?)")

type PodMetric struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	CPUMilli  int64  `json:"cpu_milli"`
	MemBytes  int64  `json:"mem_bytes"`
}

// PodMetrics reads the current CPU and memory use of the pods in ns ("" = all namespaces).
func (c *Client) PodMetrics(ctx context.Context, ns string) ([]PodMetric, error) {
	rc, _ := c.cs.CoreV1().RESTClient().(*rest.RESTClient)
	if rc == nil {
		return nil, ErrNoMetrics
	}
	path := "/apis/metrics.k8s.io/v1beta1/pods"
	if ns != "" {
		path = "/apis/metrics.k8s.io/v1beta1/namespaces/" + ns + "/pods"
	}
	raw, err := rc.Get().AbsPath(path).DoRaw(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) || apierrors.IsServiceUnavailable(err) {
			return nil, ErrNoMetrics
		}
		return nil, err
	}
	return parsePodMetrics(raw)
}

func parsePodMetrics(raw []byte) ([]PodMetric, error) {
	var l struct {
		Items []struct {
			Metadata   struct{ Name, Namespace string }
			Containers []struct{ Usage map[string]string }
		}
	}
	if err := json.Unmarshal(raw, &l); err != nil {
		return nil, fmt.Errorf("unexpected metrics answer: %w", err)
	}
	out := make([]PodMetric, 0, len(l.Items))
	for _, it := range l.Items {
		m := PodMetric{Namespace: it.Metadata.Namespace, Name: it.Metadata.Name}
		for _, c := range it.Containers {
			if q, err := resource.ParseQuantity(c.Usage["cpu"]); err == nil {
				m.CPUMilli += q.MilliValue()
			}
			if q, err := resource.ParseQuantity(c.Usage["memory"]); err == nil {
				m.MemBytes += q.Value()
			}
		}
		out = append(out, m)
	}
	return out, nil
}

// ---- deployments: scale and rollout restart

// ScaleDeployment sets spec.replicas (what `kubectl scale` does).
func (c *Client) ScaleDeployment(ctx context.Context, ns, name string, replicas int32) error {
	if replicas < 0 {
		return errors.New("replicas must be 0 or more")
	}
	patch := fmt.Sprintf(`{"spec":{"replicas":%d}}`, replicas)
	_, err := c.cs.AppsV1().Deployments(ns).Patch(ctx, name, types.MergePatchType, []byte(patch), metav1.PatchOptions{})
	return err
}

// RestartDeployment starts a rolling restart by stamping the pod template (what `kubectl rollout restart` does).
func (c *Client) RestartDeployment(ctx context.Context, ns, name string) error {
	patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":%q}}}}}`, time.Now().Format(time.RFC3339))
	_, err := c.cs.AppsV1().Deployments(ns).Patch(ctx, name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	return err
}

// ---- namespaces: create and delete

var ErrBadNamespace = errors.New("a namespace name is lowercase letters, digits and '-', starts and ends with a letter or digit, and is at most 63 characters")

func validNamespace(name string) bool {
	if len(name) == 0 || len(name) > 63 || name[0] == '-' || name[len(name)-1] == '-' {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}

// ErrProtectedNamespace: default and kube-* are never deleted from here.
var ErrProtectedNamespace = errors.New("this namespace is protected and is not deleted from here")

// protectedNamespace names the ones that are never deleted from here.
func protectedNamespace(name string) bool {
	return name == "default" || strings.HasPrefix(name, "kube-")
}

// CreateNamespace creates an empty namespace (what `kubectl create namespace` does).
func (c *Client) CreateNamespace(ctx context.Context, name string) error {
	if !validNamespace(name) {
		return ErrBadNamespace
	}
	_, err := c.cs.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}, metav1.CreateOptions{})
	return err
}

// DeleteNamespace deletes a namespace and everything in it. default and kube-* are refused.
func (c *Client) DeleteNamespace(ctx context.Context, name string) error {
	if !validNamespace(name) {
		return ErrBadNamespace
	}
	if protectedNamespace(name) {
		return fmt.Errorf("%w: %q", ErrProtectedNamespace, name)
	}
	return c.cs.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{})
}

// ---- nodes: the health of the whole cluster

type NodeInfo struct {
	Name          string   `json:"name"`
	Roles         []string `json:"roles"`
	Status        string   `json:"status"` // Ready, NotReady or Unknown
	Unschedulable bool     `json:"unschedulable"`
	Pressure      []string `json:"pressure"` // conditions that are true and bad: MemoryPressure, DiskPressure, PIDPressure, NetworkUnavailable
	Version       string   `json:"version"`
	Age           string   `json:"age"`
	CPUAllocMilli int64    `json:"cpu_alloc_milli"`
	MemAllocBytes int64    `json:"mem_alloc_bytes"`
	PodsAlloc     int64    `json:"pods_alloc"`
	CPUReqMilli   int64    `json:"cpu_req_milli"` // requests of the pods on the node (approximate: containers only)
	MemReqBytes   int64    `json:"mem_req_bytes"`
	Pods          int      `json:"pods"`
	CPUUseMilli   *int64   `json:"cpu_use_milli"` // live use from metrics-server; nil without it
	MemUseBytes   *int64   `json:"mem_use_bytes"`
}

type NodesResult struct {
	Nodes    []NodeInfo `json:"nodes"`
	Metrics  bool       `json:"metrics"`  // live CPU / memory use is included
	Requests bool       `json:"requests"` // pod requests and counts are included (the pod list was readable)
}

// Nodes lists every node with its readiness, pressure, capacity, what its pods request and (when
// metrics-server is there) what it uses. The pod list and the metrics are best effort: a role that cannot
// read them still gets the nodes.
func (c *Client) Nodes(ctx context.Context) (NodesResult, error) {
	l, err := c.cs.CoreV1().Nodes().List(ctx, cachedList)
	if err != nil {
		return NodesResult{}, err
	}
	out := NodesResult{Nodes: make([]NodeInfo, 0, len(l.Items))}
	idx := map[string]int{}
	for _, n := range l.Items {
		ni := NodeInfo{Name: n.Name, Roles: []string{}, Status: "Unknown", Pressure: []string{}, Unschedulable: n.Spec.Unschedulable,
			Version: n.Status.NodeInfo.KubeletVersion, Age: age(n.CreationTimestamp.Time)}
		for k := range n.Labels {
			if r, ok := strings.CutPrefix(k, "node-role.kubernetes.io/"); ok && r != "" {
				ni.Roles = append(ni.Roles, r)
			}
		}
		sort.Strings(ni.Roles)
		for _, cd := range n.Status.Conditions {
			switch cd.Type {
			case corev1.NodeReady:
				switch cd.Status {
				case corev1.ConditionTrue:
					ni.Status = "Ready"
				case corev1.ConditionFalse:
					ni.Status = "NotReady"
				}
			case corev1.NodeMemoryPressure, corev1.NodeDiskPressure, corev1.NodePIDPressure, corev1.NodeNetworkUnavailable:
				if cd.Status == corev1.ConditionTrue {
					ni.Pressure = append(ni.Pressure, string(cd.Type))
				}
			}
		}
		a := n.Status.Allocatable
		if len(a) == 0 {
			a = n.Status.Capacity
		}
		ni.CPUAllocMilli = a.Cpu().MilliValue()
		ni.MemAllocBytes = a.Memory().Value()
		ni.PodsAlloc = a.Pods().Value()
		idx[n.Name] = len(out.Nodes)
		out.Nodes = append(out.Nodes, ni)
	}
	sort.Slice(out.Nodes, func(i, j int) bool { return out.Nodes[i].Name < out.Nodes[j].Name })
	for i, n := range out.Nodes {
		idx[n.Name] = i
	}
	if pl, err := c.cs.CoreV1().Pods("").List(ctx, cachedList); err == nil {
		out.Requests = true
		for _, p := range pl.Items {
			i, ok := idx[p.Spec.NodeName]
			if !ok || p.Status.Phase == corev1.PodSucceeded || p.Status.Phase == corev1.PodFailed {
				continue
			}
			out.Nodes[i].Pods++
			for _, ct := range p.Spec.Containers {
				out.Nodes[i].CPUReqMilli += ct.Resources.Requests.Cpu().MilliValue()
				out.Nodes[i].MemReqBytes += ct.Resources.Requests.Memory().Value()
			}
		}
	}
	if raw, err := c.nodeMetricsRaw(ctx); err == nil {
		if use, err := parseNodeMetrics(raw); err == nil {
			out.Metrics = true
			for name, u := range use {
				if i, ok := idx[name]; ok {
					cpu, mem := u[0], u[1]
					out.Nodes[i].CPUUseMilli, out.Nodes[i].MemUseBytes = &cpu, &mem
				}
			}
		}
	}
	return out, nil
}

func (c *Client) nodeMetricsRaw(ctx context.Context) ([]byte, error) {
	rc, _ := c.cs.CoreV1().RESTClient().(*rest.RESTClient)
	if rc == nil {
		return nil, ErrNoMetrics
	}
	raw, err := rc.Get().AbsPath("/apis/metrics.k8s.io/v1beta1/nodes").DoRaw(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) || apierrors.IsServiceUnavailable(err) || apierrors.IsForbidden(err) {
			return nil, ErrNoMetrics
		}
		return nil, err
	}
	return raw, nil
}

// parseNodeMetrics maps node name to {cpu milli, memory bytes}.
func parseNodeMetrics(raw []byte) (map[string][2]int64, error) {
	var l struct {
		Items []struct {
			Metadata struct{ Name string }
			Usage    map[string]string
		}
	}
	if err := json.Unmarshal(raw, &l); err != nil {
		return nil, fmt.Errorf("unexpected metrics answer: %w", err)
	}
	out := map[string][2]int64{}
	for _, it := range l.Items {
		var u [2]int64
		if q, err := resource.ParseQuantity(it.Usage["cpu"]); err == nil {
			u[0] = q.MilliValue()
		}
		if q, err := resource.ParseQuantity(it.Usage["memory"]); err == nil {
			u[1] = q.Value()
		}
		out[it.Metadata.Name] = u
	}
	return out, nil
}
