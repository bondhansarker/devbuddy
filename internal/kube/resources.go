package kube

import (
	"context"
	"errors"
	"fmt"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
)

type Service struct {
	Name       string            `json:"name"`
	Namespace  string            `json:"namespace"`
	Type       string            `json:"type"`
	ClusterIP  string            `json:"cluster_ip"`
	ExternalIP *string           `json:"external_ip"`
	Ports      []string          `json:"ports"`
	Targets    map[string]string `json:"targets"` // service port -> targetPort (a number, or a container port name)
	Age        string            `json:"age"`
}

func (c *Client) Services(ctx context.Context, ns string) ([]Service, error) {
	l, err := c.cs.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]Service, 0, len(l.Items))
	for _, s := range l.Items {
		v := Service{Name: s.Name, Namespace: s.Namespace, Type: string(s.Spec.Type), ClusterIP: s.Spec.ClusterIP,
			Ports: []string{}, Targets: map[string]string{}, Age: age(s.CreationTimestamp.Time)}
		for _, p := range s.Spec.Ports {
			v.Ports = append(v.Ports, fmt.Sprintf("%d/%s", p.Port, p.Protocol))
			v.Targets[fmt.Sprint(p.Port)] = p.TargetPort.String()
		}
		v.ExternalIP = externalIP(&s)
		out = append(out, v)
	}
	return out, nil
}

// externalIP is the load balancer address or the external name; nil when there is none.
func externalIP(s *corev1.Service) *string {
	var a []string
	for _, i := range s.Status.LoadBalancer.Ingress {
		if i.Hostname != "" {
			a = append(a, i.Hostname)
		} else if i.IP != "" {
			a = append(a, i.IP)
		}
	}
	a = append(a, s.Spec.ExternalIPs...)
	if s.Spec.ExternalName != "" {
		a = append(a, s.Spec.ExternalName)
	}
	if len(a) == 0 {
		return nil
	}
	v := strings.Join(a, ", ")
	return &v
}

type Deployment struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Ready     string `json:"ready"`
	UpToDate  int32  `json:"up_to_date"`
	Available int32  `json:"available"`
	Age       string `json:"age"`
	Replicas  int32  `json:"replicas"`
}

func (c *Client) Deployments(ctx context.Context, ns string) ([]Deployment, error) {
	l, err := c.cs.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]Deployment, 0, len(l.Items))
	for _, d := range l.Items {
		want := int32(1)
		if d.Spec.Replicas != nil {
			want = *d.Spec.Replicas
		}
		out = append(out, Deployment{d.Name, d.Namespace, fmt.Sprintf("%d/%d", d.Status.ReadyReplicas, want),
			d.Status.UpdatedReplicas, d.Status.AvailableReplicas, age(d.CreationTimestamp.Time), want})
	}
	return out, nil
}

type Job struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Completions string `json:"completions"`
	Active      int32  `json:"active"`
	Failed      int32  `json:"failed"`
	Status      string `json:"status"`
	Age         string `json:"age"`
}

func (c *Client) Jobs(ctx context.Context, ns string) ([]Job, error) {
	l, err := c.cs.BatchV1().Jobs(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]Job, 0, len(l.Items))
	for _, j := range l.Items {
		want := int32(1)
		if j.Spec.Completions != nil {
			want = *j.Spec.Completions
		}
		st := "Running"
		for _, c := range j.Status.Conditions {
			if c.Status == corev1.ConditionTrue && (c.Type == batchv1.JobComplete || c.Type == batchv1.JobFailed) {
				st = string(c.Type)
			}
		}
		out = append(out, Job{j.Name, j.Namespace, fmt.Sprintf("%d/%d", j.Status.Succeeded, want),
			j.Status.Active, j.Status.Failed, st, age(j.CreationTimestamp.Time)})
	}
	return out, nil
}

// PodsOf returns the pods a deployment ("deployments"), job ("jobs") or service ("services") selects.
func (c *Client) PodsOf(ctx context.Context, kind, ns, name string) ([]corev1.Pod, error) {
	var sel labels.Selector
	switch kind {
	case "deployments":
		d, err := c.cs.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if sel, err = metav1.LabelSelectorAsSelector(d.Spec.Selector); err != nil {
			return nil, err
		}
	case "jobs":
		j, err := c.cs.BatchV1().Jobs(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if sel, err = metav1.LabelSelectorAsSelector(j.Spec.Selector); err != nil {
			return nil, err
		}
	case "services":
		sv, err := c.cs.CoreV1().Services(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if len(sv.Spec.Selector) == 0 {
			return nil, errors.New("service has no selector: no pods to read logs from")
		}
		sel = labels.SelectorFromSet(sv.Spec.Selector)
	default:
		return nil, errors.New("unknown kind " + kind)
	}
	l, err := c.cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: sel.String()})
	if err != nil {
		return nil, err
	}
	return l.Items, nil
}

// PodFor picks the pod to reach a deployment or service through (as kubectl does): the first
// Running and Ready pod by name.
func (c *Client) PodFor(ctx context.Context, kind, ns, name string) (string, error) {
	pods, err := c.PodsOf(ctx, kind, ns, name)
	if err != nil {
		return "", err
	}
	best := ""
	for _, p := range pods {
		ready := false
		for _, c := range p.Status.Conditions {
			if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
				ready = true
			}
		}
		if p.Status.Phase == corev1.PodRunning && ready && p.DeletionTimestamp == nil && (best == "" || p.Name < best) {
			best = p.Name
		}
	}
	if best == "" {
		return "", errors.New("no running, ready pod behind it")
	}
	return best, nil
}

// DeleteJob deletes a job and, in the background, its pods (otherwise they would be left behind).
func (c *Client) DeleteJob(ctx context.Context, ns, name string) error {
	p := metav1.DeletePropagationBackground
	return c.cs.BatchV1().Jobs(ns).Delete(ctx, name, metav1.DeleteOptions{PropagationPolicy: &p})
}

// labels the Job controller adds to a job and its pods: they must not be copied into a new job
var controllerLabels = []string{"controller-uid", "job-name", "batch.kubernetes.io/controller-uid", "batch.kubernetes.io/job-name"}

func withoutControllerLabels(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		out[k] = v
	}
	for _, k := range controllerLabels {
		delete(out, k)
	}
	return out
}

// TriggerJob runs a job again by creating a new job from the spec of an existing one
// (like kubectl create job --from); it returns the new job's name.
func (c *Client) TriggerJob(ctx context.Context, ns, name string) (string, error) {
	j, err := c.cs.BatchV1().Jobs(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	spec := j.Spec.DeepCopy()
	spec.Selector, spec.ManualSelector = nil, nil // the controller sets its own
	spec.Template.Labels = withoutControllerLabels(spec.Template.Labels)
	base := name
	if len(base) > 50 {
		base = base[:50]
	}
	nj := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: base + "-" + utilrand.String(5), Namespace: ns,
			Labels:      withoutControllerLabels(j.Labels),
			Annotations: map[string]string{"devbuddy/triggered-from": name},
		},
		Spec: *spec,
	}
	created, err := c.cs.BatchV1().Jobs(ns).Create(ctx, nj, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}
	return created.Name, nil
}
