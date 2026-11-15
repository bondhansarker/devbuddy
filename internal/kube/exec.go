package kube

import (
	"context"
	"errors"
	"io"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

// Terminal is the byte stream plus resize feed of one shell session.
type Terminal struct {
	In     io.Reader
	Out    io.Writer
	Resize <-chan remotecommand.TerminalSize
}

type sizeQueue <-chan remotecommand.TerminalSize

func (q sizeQueue) Next() *remotecommand.TerminalSize {
	s, ok := <-q
	if !ok {
		return nil
	}
	return &s
}

// Shell runs bash (else sh) in the container with a TTY until it exits.
func (c *Client) Shell(ctx context.Context, ns, pod, container string, t Terminal) error {
	if c.cfg == nil {
		return errors.New("no rest config")
	}
	cmd := []string{"/bin/sh", "-c", "[ -x /bin/bash ] && exec /bin/bash || exec /bin/sh"}
	req := c.cs.CoreV1().RESTClient().Post().Resource("pods").Namespace(ns).Name(pod).SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container, Command: cmd, Stdin: true, Stdout: true, TTY: true,
		}, scheme.ParameterCodec)
	ex, err := remotecommand.NewSPDYExecutor(c.cfg, "POST", req.URL())
	if err != nil {
		return err
	}
	return ex.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin: t.In, Stdout: t.Out, Tty: true, TerminalSizeQueue: sizeQueue(t.Resize),
	})
}
