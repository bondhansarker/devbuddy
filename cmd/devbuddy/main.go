package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/bondhansarker/devbuddy/internal/api"
	"github.com/bondhansarker/devbuddy/internal/config"
	"github.com/bondhansarker/devbuddy/internal/kube"
	"github.com/bondhansarker/devbuddy/internal/web"
	"k8s.io/klog/v2"
)

// quietCancel drops client-go's "context canceled" log lines: closing a log tab or reloading the page
// cancels an in-flight request on purpose, and client-go reports that as an error. Everything else passes.
type quietCancel struct{}

func (quietCancel) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "context canceled") {
		return len(p), nil
	}
	return os.Stderr.Write(p)
}

func main() {
	cfgPath := flag.String("config", "", "config file, required (default: ./.config.json, then ~/.config/devbuddy/config.json)")
	flag.Parse()
	klog.LogToStderr(false)
	klog.SetOutput(quietCancel{})

	cfg, used, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Every Kubernetes connection from the config; one that cannot be set up is kept and shown as not connected.
	var clusters []api.Cluster
	for _, kc := range cfg.Kubernetes {
		c := api.Cluster{Name: kc.Name, Context: kc.Context}
		if c.Client, c.Err = kube.New(kc.Kubeconfig, kc.Context); c.Err != nil {
			log.Printf("kubernetes %q: %v", kc.Name, c.Err)
		}
		clusters = append(clusters, c)
	}
	fmt.Printf("DevBuddy on http://%s (%d Kubernetes, %d Redis, %d Elasticsearch; config: %s%s)\n", cfg.Addr, len(cfg.Kubernetes), len(cfg.Redis), len(cfg.Elasticsearch), used, map[bool]string{true: ", read-only"}[cfg.ReadOnly])
	for _, w := range config.Warnings(used, cfg) {
		log.Printf("warning: %s", w)
	}
	// header and idle timeouts only: logs (SSE) and shells (WebSocket) are long-lived on purpose
	srv := &http.Server{Addr: cfg.Addr, Handler: api.New(clusters, web.FS(), cfg), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 64 << 10}
	log.Fatal(srv.ListenAndServe())
}
