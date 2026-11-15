# Changelog

## Unreleased

- Renamed to **DevBuddy**.
- Kubernetes is its own section with several connections (`kubernetes` list in the config) and a Connections page: nothing connects until you press Connect, and each connected cluster is a sidebar node with its own pages and lists.
- Redis page (keys, console) and a read-only-by-default Elasticsearch / OpenSearch viewer.

## 0.1.0

First release.

- Lists for namespaces, services, deployments, jobs and pods across all namespaces, with namespace, status, owner and text filters.
- Logs and shell in a bottom dock (one tab per pod, resizable, full screen, restored after a reload); shells keep running on the server and re-attach after a page refresh.
- Logs: structured JSON lines, details on double-click, search with highlights, dividers, timestamps, previous container, download; merged logs for deployments, services and jobs.
- Restart pods, trigger and delete jobs, port forward (also from deployments and services).
- Strict JSON config, loopback-only listener, `read_only` mode.
- Prebuilt binaries for macOS and Linux, arm64 and amd64 (`dist/`).
