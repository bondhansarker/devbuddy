# Changelog

## Unreleased

- Renamed to **DevBuddy**.
- Kubernetes is its own section with several connections (`kubernetes` list in the config) and a Clusters page: nothing connects until you press Connect or switch a cluster on in the sidebar; each cluster is a parent node (with an on/off switch) whose pages are its children, with its own lists.
- Redis (keys, console) and Elasticsearch / OpenSearch (documents, indices, query, delete) are built the same way: flat connections with a switch under a collapsible group, nothing connects until switched on, each connection keeps its own state; Kubernetes, Redis and Elasticsearch groups and each connected node fold.

- Pods: **Describe** (status, why a container restarted, events, as JSON) and CPU / memory columns when the cluster has metrics-server (hidden otherwise). Deployments: **Scale** and **Rollout restart**. Logs: "since" range and a level filter.
- Auto refresh checkbox and a refresh-now button; pod metrics are fetched every third refresh and failed refreshes back off.
- Redis: edit a string value (TTL kept), export keys with their values as JSON. Elasticsearch: export the loaded documents as JSON or CSV.
- Command toasts have a button that copies the command.
- UI: icons, in-app dialogs instead of browser pop-ups, JSON shown indented everywhere, minimize / close for the console dock.
- Security: requests whose `Host` is not localhost are refused (DNS rebinding guard); the `--insecure-bind` flag is gone, so the listener is always loopback.
- Removed: the old single-cluster config keys (`kubeconfig` / `context` at the top level: use the `kubernetes` list), the Namespaces page (use the namespace filter), the Pods Images and Services External IP columns (shown as tooltips), the sidebar's Cluster line, and the committed `dist/` binaries (they are built with `scripts/build-all.sh` and attached to releases).

## 0.1.0

First release.

- Lists for namespaces, services, deployments, jobs and pods across all namespaces, with namespace, status, owner and text filters.
- Logs and shell in a bottom dock (one tab per pod, resizable, full screen, restored after a reload); shells keep running on the server and re-attach after a page refresh.
- Logs: structured JSON lines, details on double-click, search with highlights, dividers, timestamps, previous container, download; merged logs for deployments, services and jobs.
- Restart pods, trigger and delete jobs, port forward (also from deployments and services).
- Strict JSON config, loopback-only listener, `read_only` mode.
- Prebuilt binaries for macOS and Linux, arm64 and amd64 (`dist/`).
