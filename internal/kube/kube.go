// Package kube wraps client-go for the few operations DevBuddy needs.
package kube

import (
	"context"
	"fmt"
	"io"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Client struct {
	cs      kubernetes.Interface
	cfg     *rest.Config
	Context string
	Name    string // the connection name from the config
}

// New loads the kubeconfig (explicit path, else the default loading rules) and
// connects with its current context, or kubeContext when set.
func New(kubeconfig, kubeContext string) (*Client, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		rules.ExplicitPath = kubeconfig
	}
	cc := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{CurrentContext: kubeContext})
	cfg, err := cc.ClientConfig()
	if err != nil {
		return nil, err
	}
	raw, _ := cc.RawConfig()
	name := raw.CurrentContext
	if kubeContext != "" {
		name = kubeContext
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Client{cs: cs, cfg: cfg, Context: name}, nil
}

// NewWith builds a Client around a ready clientset (tests).
func NewWith(cs kubernetes.Interface) *Client { return &Client{cs: cs, Context: "test"} }

type Port struct {
	Name          string `json:"name"`
	ContainerPort int32  `json:"container_port"`
	Protocol      string `json:"protocol"`
	Container     string `json:"container"`
}

type Pod struct {
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	Phase      string   `json:"phase"`
	Ready      string   `json:"ready"`
	Restarts   int32    `json:"restarts"`
	Age        string   `json:"age"`
	Node       string   `json:"node"`
	Containers []string `json:"containers"`
	Images     []string `json:"images"`
	Ports      []Port   `json:"ports"`
	OwnerKind  *string  `json:"owner_kind"`
}

type Namespace struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Age    string `json:"age"`
}

func (c *Client) Namespaces(ctx context.Context) ([]Namespace, error) {
	l, err := c.cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]Namespace, 0, len(l.Items))
	for _, n := range l.Items {
		out = append(out, Namespace{n.Name, string(n.Status.Phase), age(n.CreationTimestamp.Time)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (c *Client) Pods(ctx context.Context, ns string) ([]Pod, error) {
	l, err := c.cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]Pod, 0, len(l.Items))
	for i := range l.Items {
		out = append(out, toPod(&l.Items[i]))
	}
	return out, nil
}

func toPod(p *corev1.Pod) Pod {
	out := Pod{
		Name: p.Name, Namespace: p.Namespace, Phase: string(p.Status.Phase),
		Node: p.Spec.NodeName, Age: age(p.CreationTimestamp.Time),
		Containers: []string{}, Images: []string{}, Ports: []Port{},
	}
	if len(p.OwnerReferences) > 0 {
		k := p.OwnerReferences[0].Kind
		out.OwnerKind = &k
	}
	for _, ct := range p.Spec.Containers {
		out.Containers = append(out.Containers, ct.Name)
		out.Images = append(out.Images, ct.Image)
		for _, pt := range ct.Ports {
			out.Ports = append(out.Ports, Port{pt.Name, pt.ContainerPort, string(pt.Protocol), ct.Name})
		}
	}
	ready := 0
	for _, cs := range p.Status.ContainerStatuses {
		out.Restarts += cs.RestartCount
		if cs.Ready {
			ready++
		}
	}
	out.Ready = fmt.Sprintf("%d/%d", ready, len(p.Spec.Containers))
	return out
}

func age(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// Logs streams a container's log; the caller closes it.
// previous reads the last terminated container (the API refuses it together with follow).
// since > 0 limits it to the last that many seconds.
func (c *Client) Logs(ctx context.Context, ns, pod, container string, tail, since int64, follow, timestamps, previous bool) (io.ReadCloser, error) {
	o := &corev1.PodLogOptions{Container: container, Follow: follow && !previous, Timestamps: timestamps, Previous: previous}
	if since > 0 {
		o.SinceSeconds = &since
	}
	if tail > 0 {
		o.TailLines = &tail
	}
	return c.cs.CoreV1().Pods(ns).GetLogs(pod, o).Stream(ctx)
}

// Restart deletes the pod; its controller (if any) recreates it.
func (c *Client) Restart(ctx context.Context, ns, pod string) error {
	return c.cs.CoreV1().Pods(ns).Delete(ctx, pod, metav1.DeleteOptions{})
}

// GetPod returns one pod.
func (c *Client) GetPod(ctx context.Context, ns, pod string) (Pod, error) {
	p, err := c.cs.CoreV1().Pods(ns).Get(ctx, pod, metav1.GetOptions{})
	if err != nil {
		return Pod{}, err
	}
	return toPod(p), nil
}

// Version asks the API server for its version: the cheapest call that proves the connection works.
func (c *Client) Version(ctx context.Context) (string, error) {
	type result struct {
		v   string
		err error
	}
	ch := make(chan result, 1)
	go func() { // discovery has no context: give up waiting when the request does
		v, err := c.cs.Discovery().ServerVersion()
		if err != nil {
			ch <- result{"", err}
			return
		}
		ch <- result{v.GitVersion, nil}
	}()
	select {
	case r := <-ch:
		return r.v, r.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
