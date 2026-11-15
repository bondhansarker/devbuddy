package kube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"

	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

type Forward struct {
	ID            string `json:"id"`
	Cluster       string `json:"cluster"`
	Namespace     string `json:"namespace"`
	Pod           string `json:"pod"`
	ContainerPort int    `json:"container_port"`
	LocalPort     int    `json:"local_port"`
	Error         string `json:"error"`
	stop          chan struct{}
}

// Forwards tracks in-process port-forwards; they die with the app.
type Forwards struct {
	mu  sync.Mutex
	seq atomic.Int64
	m   map[string]*Forward
}

func NewForwards() *Forwards { return &Forwards{m: map[string]*Forward{}} }

func (f *Forwards) List() []Forward {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Forward, 0, len(f.m))
	for _, v := range f.m {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (f *Forwards) Stop(id string) bool {
	f.mu.Lock()
	v, ok := f.m[id]
	delete(f.m, id)
	f.mu.Unlock()
	if ok {
		close(v.stop)
	}
	return ok
}

// freePort returns want when it is free on loopback, else any free port (want 0 = any).
func freePort(want int) (int, error) {
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", want))
	if err != nil && want != 0 {
		l, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// Start forwards 127.0.0.1:local to the pod's containerPort. local 0 = same as
// containerPort, falling back to a free port on conflict.
func (c *Client) StartForward(ctx context.Context, fw *Forwards, ns, pod string, containerPort, local int) (Forward, error) {
	if c.cfg == nil {
		return Forward{}, errors.New("no rest config")
	}
	if containerPort <= 0 || containerPort > 65535 {
		return Forward{}, errors.New("container_port out of range")
	}
	if local == 0 {
		local = containerPort
	}
	local, err := freePort(local)
	if err != nil {
		return Forward{}, err
	}
	rt, upgrader, err := spdy.RoundTripperFor(c.cfg)
	if err != nil {
		return Forward{}, err
	}
	u := c.cs.CoreV1().RESTClient().Post().Resource("pods").Namespace(ns).Name(pod).SubResource("portforward").URL()
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: rt}, "POST", u)
	stop, ready := make(chan struct{}), make(chan struct{})
	pf, err := portforward.NewOnAddresses(dialer, []string{"127.0.0.1"},
		[]string{fmt.Sprintf("%d:%d", local, containerPort)}, stop, ready, io.Discard, io.Discard)
	if err != nil {
		return Forward{}, err
	}
	v := &Forward{
		ID: fmt.Sprintf("f%d", fw.seq.Add(1)), Cluster: c.Name, Namespace: ns, Pod: pod,
		ContainerPort: containerPort, LocalPort: local, stop: stop,
	}
	errc := make(chan error, 1)
	go func() { errc <- pf.ForwardPorts() }()
	select {
	case <-ready:
	case err := <-errc:
		return Forward{}, err
	case <-ctx.Done():
		close(stop)
		return Forward{}, ctx.Err()
	}
	fw.mu.Lock()
	fw.m[v.ID] = v
	fw.mu.Unlock()
	// Pod gone or connection lost: record why and keep the row so the UI shows it.
	go func() {
		err := <-errc
		fw.mu.Lock()
		if cur, ok := fw.m[v.ID]; ok && err != nil {
			cur.Error = err.Error()
		} else if ok {
			delete(fw.m, v.ID)
		}
		fw.mu.Unlock()
	}()
	return *v, nil
}
