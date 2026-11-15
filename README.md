# DevBuddy

A local web UI for debugging your stack: **Kubernetes** (namespaces, services, deployments, jobs, pods, logs, shell, restart, job trigger, port-forward; several clusters at once), a **Redis** key browser and console, and an **Elasticsearch / OpenSearch** viewer. One Go binary, UI embedded. Every section is optional: connect only what you use.

## Prerequisites

- For the Kubernetes section: a kubeconfig with a working context, plus the cloud CLI it calls (e.g. `aws` for EKS). `kubectl` is not needed. Without one, the Kubernetes pages show "not connected" and Redis and Elasticsearch still work.
- Permissions for what you use: list/get on pods, services, deployments, jobs, namespaces; pods/log; pods/exec; pods/portforward; delete pods; create/delete jobs.
- To build: Go 1.22.5+.

## Run

Prebuilt binaries are in `dist/` (`devbuddy-{darwin,linux}-{arm64,amd64}`, checksums in `SHA256SUMS`). Or build: `go build -o bin/devbuddy ./cmd/devbuddy` (`./scripts/build-all.sh` builds all four).

```bash
cp .config.example.json .config.json   # fill in the connections you use
./devbuddy                                # http://127.0.0.1:9100  (--config prod.json for another file)
```

## Config

A JSON file is required and every key must be set. Lookup: `--config <path>`, `./.config.json`, `~/.config/devbuddy/config.json`.

| key | meaning |
|---|---|
| `addr` | listen address |
| `kubernetes` | *optional* list of Kubernetes connections: `name`, `kubeconfig` (path, `~/` expanded) and `context` each, see below |
| `refresh_seconds` | list refresh interval, at least 1 |
| `log_tail` | initial log lines |
| `read_only` | disables shell, restart, job trigger/delete, port forward, Redis key deletes and Elasticsearch document deletes |
| `redis` | *optional* list of Redis servers for the Redis page, see below |
| `elasticsearch` | *optional* list of Elasticsearch / OpenSearch clusters for the read-only Elasticsearch page, see below |

## Kubernetes

```json
"kubernetes": [
  { "name": "dev",     "kubeconfig": "~/.kube/config", "context": "my-dev-context" },
  { "name": "staging", "kubeconfig": "~/.kube/config", "context": "my-staging-context" }
]
```

Each entry is a separate connection with a unique `name`. In the sidebar the connections sit flat under the **Kubernetes** heading, each with an on/off switch, and **nothing connects until you switch one on** (not at start-up, not on reload). Switched on, a cluster shows all its pages as children (Namespaces, Services, Deployments, Jobs, Pods), headed with its name (`dev › Pods`); switched off, it is untouched. Every cluster keeps its own lists and filters: pods of two clusters are never mixed. Connect as many as you like; clicking a page under a cluster makes it the active one. Switching off drops that cluster's lists (its shells keep running until you close the tab). A cluster whose kubeconfig cannot be loaded shows a red switch with the reason in its tooltip, and the app still starts, so you can use it for Redis or Elasticsearch only. After a reload, switch the cluster on again: its open shells and log tabs come back. A name or context containing `prod` shows a red "production" pill for the active cluster.

Everything in the sidebar folds: click the arrow next to **Kubernetes**, **Redis** or **Elasticsearch** to fold the whole group, or the arrow of a connected cluster / server to fold just its pages (remembered across reloads). Clicking a group's name opens its overview, a table with each connection's status, error and Connect / Disconnect buttons.

The older form (`"kubeconfig"` and `"context"` as two top-level keys) still works and counts as one connection named after the context.

## Redis

Add servers to the config to get a **Redis** group in the sidebar, built like Kubernetes: the servers sit flat under the heading, each with an on/off switch, and nothing connects until you switch one on. A connected server shows **Keys** and **Console** under its name and keeps its own keys, db choice, search, type filter and console session. On the Keys page the filter bar has a dropdown of db indexes (tick several), and the key table's header holds the controls: a search box in the Key column (plain text matches anywhere, or a glob like `user:*`), a type filter in the Type column (done by the server with `SCAN … TYPE`, so it needs Redis 6+), and a click on the TTL header sorts the keys shown (soonest first, longest first, off; keys without an expiry count as the longest). You can also view a key of any type, delete one or several. Keys are listed with `SCAN`, never `KEYS`; large values are cut off (1 MB of a string, 500 elements of a collection).

```json
"redis": [{ "name": "cache", "addr": "10.0.0.5:6379", "username": "", "password": "", "tls": false }]
```

The app connects straight to `addr`, so it must be reachable from your machine (VPN, public address, or a port forward you started on the Port forwards page: use `127.0.0.1:<local port>`). Passwords are stored in the config file, which is gitignored; only `name` and `addr` are ever sent to the browser. Non-UTF-8 keys and values are shown with replacement characters, so a binary key cannot be deleted from the UI.

## Elasticsearch / OpenSearch

A viewer with document delete, built like Redis: the clusters sit flat under the **Elasticsearch** heading with an on/off switch each, nothing connects until you switch one on, and a connected cluster shows **Documents**, **Indices** and **Query** under its name, each keeping its own state. The filter bar has an index dropdown (tick several; none = all).

```json
"elasticsearch": [{ "name": "logs", "addr": "https://es.example.com:9200", "username": "", "password": "", "api_key": "", "insecure_skip_verify": false }]
```

Only `name` and `addr` are required (`http://` is assumed without a scheme). Use `username` and `password` for basic auth, or `api_key` (the base64 `id:key` value) instead; `insecure_skip_verify` skips certificate checks for self-signed clusters. It talks plain HTTP and JSON, so it works with both Elasticsearch and OpenSearch, and the cluster must be reachable from your machine (VPN, public address, or a port forward from the Port forwards page).

- **Documents:** the box in the table header takes a Lucene query (`user:alice AND status:active`) or plain words; 50 per page with **Load more**, up to Elasticsearch's 10,000 result window. **View** shows the full document as JSON.
- **Indices:** name, health, docs, size and shards with a name filter; **Documents** jumps to that index, **Mapping** shows its mapping.
- **Query:** read-only REST calls: any `GET`, and `POST` on `_search`, `_count`, `_msearch`, `_mget`, `_analyze` and similar. `PUT`, `DELETE`, `_bulk`, `_update` and the like are refused there, always: the Query tab never writes.
- **Delete:** the Documents tab has a **Delete** button per row and in the document dialog, and checkboxes with **Delete selected**. Each document is deleted by index and id from the exact index it was listed in (never by pattern or wildcard, up to 500 at once), after a confirmation. `read_only: true` removes every delete control and the server refuses the request. A deleted document can still show in searches for about a second, and disk space returns later when Elasticsearch merges segments. Your user or API key needs `delete` rights on the index.

## Project layout

```
cmd/devbuddy/        main: flags, config, start the server
internal/
  config/            strict JSON config
  api/               HTTP routes: api.go (server, routing, helpers), k8s.go, logs.go, forwards.go,
                     exec.go (shell), redis.go, es.go
  kube/              client-go: lists, logs, exec, port forward, jobs
  redisview/         Redis keys and console
  esview/            Elasticsearch / OpenSearch
  web/               the UI, embedded into the binary
    static/          index.html, css/, js/ (plain scripts loaded in order), vendor/ (xterm.js)
scripts/build-all.sh builds macOS and Linux binaries into dist/
```

## Security

No authentication, and a shell is full access to whatever your kubeconfig can reach; the Redis page can delete any key on the configured servers and the Elasticsearch page can delete documents (both are off with `read_only: true`). It listens on loopback only (`--insecure-bind` overrides it; don't), refuses cross-origin browser requests, and `read_only: true` is advised for production. Config files are gitignored.

[MIT](LICENSE). `internal/web/static/vendor/` is xterm.js (MIT).
