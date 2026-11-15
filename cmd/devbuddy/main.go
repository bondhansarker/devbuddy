package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

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
	insecure := flag.Bool("insecure-bind", false, "allow a non-loopback addr (exec = cluster access, no auth here)")
	flag.Parse()
	klog.LogToStderr(false)
	klog.SetOutput(quietCancel{})

	cfg, used, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	if host, _, _ := net.SplitHostPort(cfg.Addr); !*insecure && host != "localhost" {
		if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
			log.Fatalf("refusing non-loopback addr %q without --insecure-bind", cfg.Addr)
		}
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
	log.Fatal(http.ListenAndServe(cfg.Addr, api.New(clusters, web.FS(), cfg)))
}
