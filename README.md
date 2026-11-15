# DevBuddy

A local web UI for debugging your stack: **Kubernetes** (namespaces, services, deployments, jobs, pods, logs, shell, restart, job trigger, port-forward; several clusters at once), a **Redis** key browser and console, and an **Elasticsearch / OpenSearch** viewer. One Go binary, UI embedded. Every section is optional: connect only what you use.

## Prerequisites

- For the Kubernetes section: a kubeconfig with a working context, plus the cloud CLI it calls (e.g. `aws` for EKS). `kubectl` is not needed. Without one, the Kubernetes pages show "not connected" and Redis and Elasticsearch still work.
- Permissions for what you use: list/get on pods, services, deployments, jobs, namespaces; pods/log; pods/exec; pods/portforward; delete pods; create/delete jobs.
- To build: Go 1.22.5+.

## Run

Prebuilt binaries (`devbuddy-{darwin,linux}-{arm64,amd64}` with `SHA256SUMS`) are attached to the GitHub Releases. Or build: `go build -o bin/devbuddy ./cmd/devbuddy` (`./scripts/build-all.sh` builds all four into `dist/`, which is gitignored).

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

Each entry is a separate connection with a unique `name`. In the sidebar the connections sit flat under the **Kubernetes** heading, each with an on/off switch, and **nothing connects until you switch one on** (not on a first start; after a reload the ones that were on are switched on again). Switched on, a cluster shows all its pages as children (Nodes, Namespaces, Services, Deployments, Jobs, Pods; the namespace filter is in the bar), headed with its name (`dev › Pods`); switched off, it is untouched. Every cluster keeps its own lists and filters: pods of two clusters are never mixed. Connect as many as you like; clicking a page under a cluster makes it the active one. Switching off drops that cluster's lists and stops the port forwards it opened (its shells keep running until you close the tab). A cluster whose kubeconfig cannot be loaded shows a red switch with the reason in its tooltip, and the app still starts, so you can use it for Redis or Elasticsearch only. After a reload the connections that were on are switched on again (a connection you switched off stays off, and a first start connects nothing), with the same active connection, namespace filter and page, and its open shells and log tabs come back. A name or context containing `prod` shows a red "production" pill for the active cluster.

Everything in the sidebar folds: click the arrow next to **Kubernetes**, **Redis** or **Elasticsearch** to fold the whole group, or the arrow of a connected cluster / server to fold just its pages (remembered across reloads). **Nodes** shows the health of every node in the cluster: ready or not, memory / disk / PID pressure, cordoned nodes, CPU and memory use against what the node can allocate (from metrics-server when the cluster has it, else what the pods request), the pods on it against its capacity, and four totals for the cluster. It needs permission to list nodes; without it the page says so. Pods have a **Describe** action (status, container states with why the last one ended, and events, as JSON) and CPU / memory columns when the cluster runs metrics-server. Deployments can be **scaled** and **rollout-restarted** (both off with `read_only`). Nothing is read in the background: connecting a cluster is one connection check, a page's list is read when you open it (and shown again without a call if you come back within one refresh interval), the namespace list is read the first time a page needs it, and port forwards are read on their own page. The open page reloads every `refresh_seconds`; the **Auto refresh** switch in Settings turns that off (remembered in the browser), and the refresh button at the right of the filter bar reloads once. Failed reloads back off instead of repeating every tick. The lists ask the API server to answer from its cache (`resourceVersion=0`), so a list can be about a second old. URLs in logs are coloured by direction. **Blue** is this app's own: the `uri` / `path` of a request log (tagged ROUTE), the `host` it was addressed to (HOST) and anything on localhost (LOCAL). **Yellow** is a call to another service inside the cluster (SERVICE: `*.svc`, `*.cluster.local`, a bare service name, a private address). **Purple** is a call that leaves the cluster for the internet (EXTERNAL). A click copies the URL (it never opens it), and credentials inside a URL are left out. Log tabs have a "since" range and a level filter in the `⋯` menu. Every action shows the `kubectl` command it equals as a toast, with a button to copy it.

Clicking a group's name opens its overview, a table with each connection's status, error and Connect / Disconnect buttons.

## Redis

Add servers to the config to get a **Redis** group in the sidebar, built like Kubernetes: the servers sit flat under the heading, each with an on/off switch, and nothing connects until you switch one on. A connected server shows **Keys** and **Console** under its name and keeps its own keys, db choice, search, type filter and console session. On the Keys page the filter bar has a dropdown of db indexes (tick several), and the key table's header holds the controls: a search box in the Key column (plain text matches anywhere, or a glob like `user:*`), a type filter in the Type column (done by the server with `SCAN … TYPE`, so it needs Redis 6+), and a click on the TTL header sorts the keys shown (soonest first, longest first, off; keys without an expiry count as the longest). You can also view a key of any type, delete one or several, edit a string (its TTL is kept), and export keys (the selected ones, else the listed ones, up to 200) with their values as one JSON file. Edits are off with `read_only`; export is a read and stays on. Keys are listed with `SCAN`, never `KEYS`; large values are cut off (1 MB of a string, 500 elements of a collection).

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

- **Documents:** the box in the table header takes a Lucene query (`user:alice AND status:active`) or plain words; 50 per page with **Load more**, up to Elasticsearch's 10,000 result window. The hit count stops at 10,000 and shows as "≥ 10,000", so Elasticsearch never has to count a huge index exactly. **View** shows the full document as JSON.
- **Indices:** name, health, docs, size and shards with a name filter; **Documents** jumps to that index, **Mapping** shows its mapping.
- **Export:** the loaded documents (or the ticked ones) as JSON or CSV.
- **Query:** read-only REST calls: any `GET`, and `POST` on `_search`, `_count`, `_msearch`, `_mget`, `_analyze` and similar. `PUT`, `DELETE`, `_bulk`, `_update` and the like are refused there, always: the Query tab never writes.
- **Delete:** the Documents tab has a **Delete** button per row, and checkboxes with **Delete selected**. Each document is deleted by index and id from the exact index it was listed in (never by pattern or wildcard, up to 500 at once), after a confirmation. `read_only: true` removes every delete control and the server refuses the request. A deleted document can still show in searches for about a second, and disk space returns later when Elasticsearch merges segments. Your user or API key needs `delete` rights on the index.

The app remembers its UI state in this browser only (local storage, keys starting `k8ui.`): which connections were on, the active ones, the page, namespace filters, open tabs, folded groups, theme. **Settings** (the gear at the bottom of the sidebar) opens a panel beside it with the theme (Light or Dark), the Auto refresh switch and **Reset session**, which switches every connection off and also forgets the saved session (which connections were on, the page, filters, open tabs, port numbers; open shells and port forwards end; theme, Auto refresh and layout are kept). The power icon beside the Settings button is one switch: green while connections are on (click: every one goes off and its port forwards stop), grey when all are off (click: the ones that were on come back, with the same active connection, filters and page). It leaves the page, filters, tabs and shells alone. Nothing is stored on the server.

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
scripts/build-all.sh builds macOS and Linux binaries into dist/ (gitignored)
```

## Security

Passwords and API keys stay in the config file: the browser only gets connection names and addresses, credentials are refused inside `addr`, and error messages, Redis console output and Elasticsearch query results are masked if they repeat a configured secret (secrets under 6 characters are not masked); `CONFIG GET` hides password-like settings. Every response carries a strict `Content-Security-Policy` (only DevBuddy's own scripts run, the page cannot be framed), request bodies and the shell WebSocket have size limits, and a config file with passwords that other users can read produces a warning at start (`chmod 600` it). No authentication, and a shell is full access to whatever your kubeconfig can reach; the Redis page can delete any key on the configured servers and the Elasticsearch page can delete documents (both are off with `read_only: true`). It listens on loopback only (the config refuses any other `addr`), refuses cross-origin browser requests and any request whose `Host` is not localhost (a guard against DNS rebinding), and `read_only: true` is advised for production. Config files are gitignored.

[MIT](LICENSE). `internal/web/static/vendor/` is xterm.js (MIT).
