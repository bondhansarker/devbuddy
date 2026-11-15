// Kubernetes lists (namespaces, services, deployments, jobs, pods), their filters and row actions.
// (Plain scripts share one global scope: index.html loads them in numeric order.)

// Logs, Shell and Forward of a deployment or service act on the pods it selects (shell and forward on one ready pod).
const logsCell = (r) => {
  const k = esc(`${r.namespace}/${r.name}`);
  return `<td class="actions"><button type="button" class="sm" data-w="logs" data-k="${k}" title="Logs">${ICONS.logs}Logs</button>${settings.read_only ? '' :
    `<button type="button" class="sm" data-w="shell" data-k="${k}" title="Shell">${ICONS.shell}Shell</button><button type="button" class="sm" data-w="fwd" data-k="${k}" title="Forward">${ICONS.fwd}Forward</button>`}</td>`;
};
const depCell = (r) => logsCell(r).replace('</td>', settings.read_only ? '</td>' : `${ab('scale', 'Scale', `data-w="scale" data-k="${esc(`${r.namespace}/${r.name}`)}"`, '', true)}${ab('restart', 'Rollout restart', `data-w="rollout" data-k="${esc(`${r.namespace}/${r.name}`)}"`, 'danger', true)}</td>`);
const jobCell = (r) => {
  const k = esc(`${r.namespace}/${r.name}`);
  return `<td class="actions"><button type="button" class="sm" data-w="logs" data-k="${k}" title="Logs">${ICONS.logs}Logs</button>${settings.read_only ? '' : `${ab('play', 'Trigger', `data-w="trigger" data-k="${k}"`, '', true)}${ab('trash', 'Delete', `data-w="delete" data-k="${k}"`, 'danger', true)}`}</td>`;
};
// ---- Kubernetes connections: several can be configured; every k8s call carries ?cluster=
let clusters = [], cluster = '', kOk = false, kWhy = ''; // cluster = the active connection ('' = none)
const kstate = {};  // connection name -> { status: idle | connecting | connected | failed, version, ms, error }
const kstore = {};  // connection name -> its own lists, namespace filter and status/owner filters
const K8S_VIEWS = ['nodes', 'namespaces', 'services', 'deployments', 'jobs', 'pods']; // pages that need an active connection
const K_PAGES = [['nodes', 'Nodes'], ['namespaces', 'Namespaces'], ['services', 'Services'], ['deployments', 'Deployments'], ['jobs', 'Jobs'], ['pods', 'Pods']];
// every action shows the kubectl command it is the same as (and its outcome) as a toast
function ktoast(ns, args, result) {
  toast(`$ kubectl${ns ? ' -n ' + ns : ''} ${args}${result ? '\n' + result : ''}`);
}
const KTARGET = { pods: 'pod', deployments: 'deploy', services: 'svc', jobs: 'job' };
function klogs(r, kind = 'pods') { ktoast(r.namespace, `logs -f ${KTARGET[kind]}/${r.name} --tail=${settings.log_tail}${kind === 'pods' ? '' : ' --all-containers'}`); }
const kq = (path, c = cluster) => {
  if (!c) throw new Error('no Kubernetes connection is active'); // never fall back to "the first one": nothing connects by itself
  return `${path}${path.includes('?') ? '&' : '?'}cluster=${encodeURIComponent(c)}`;
};
const tag = (rows, c = cluster) => rows.map((r) => ({ ...r, cluster: c })); // every object remembers the connection it came from
const clusterInfo = () => clusters.find((c) => c.name === cluster);
// One entry per plain list view; pods have their own actions below.
const LISTS = {
  namespaces: { url: () => kq('/api/namespaces'), body: 'nsrows', badge: 'nscount', cols: 4,
    text: (r) => [r.name, r.status],
    row: (r) => `<td class="name">${esc(r.name)}</td>
      <td>${pill(r.status, r.status === 'Active' ? 'ok' : 'bad')}</td><td>${esc(r.age)}</td>
      <td class="actions">${settings.read_only || PROTECTED_NS(r.name) ? '' : ab('trash', 'Delete', `data-del="${esc(r.name)}"`, 'danger', true)}</td>` },
  services: { url: (ns) => kq(`/api/services?namespace=${encodeURIComponent(ns)}`), body: 'svcrows', badge: 'svccount', cols: 7,
    text: (r) => [r.name, r.namespace, r.type, r.cluster_ip, r.external_ip ?? '', ...r.ports],
    row: (r) => `<td class="name"${r.external_ip ? ` title="external IP ${esc(r.external_ip)}"` : ''}>${esc(r.name)}</td><td>${esc(r.namespace)}</td><td>${pill(r.type)}</td><td>${esc(r.cluster_ip)}</td>
      <td>${r.ports.length ? esc(r.ports.join(', ')) : none}</td><td>${esc(r.age)}</td>${logsCell(r)}` },
  deployments: { url: (ns) => kq(`/api/deployments?namespace=${encodeURIComponent(ns)}`), body: 'deprows', badge: 'depcount', cols: 7,
    text: (r) => [r.name, r.namespace],
    row: (r) => `<td class="name">${esc(r.name)}</td><td>${esc(r.namespace)}</td><td>${esc(r.ready)}</td><td class="num">${r.up_to_date}</td><td class="num">${r.available}</td><td>${esc(r.age)}</td>${depCell(r)}` },
  jobs: { url: (ns) => kq(`/api/jobs?namespace=${encodeURIComponent(ns)}`), body: 'jobrows', badge: 'jobcount', cols: 8,
    text: (r) => [r.name, r.namespace, r.status],
    row: (r) => `<td class="name">${esc(r.name)}</td><td>${esc(r.namespace)}</td><td class="num">${esc(r.completions)}</td>
      <td class="num">${r.active}</td><td class="num">${r.failed}</td><td>${pill(r.status, r.status === 'Complete' ? 'ok' : r.status === 'Failed' ? 'bad' : '')}</td><td>${esc(r.age)}</td>${jobCell(r)}` },
};
const emptyText = (n) => (n ? 'nothing matches the filter' : 'nothing here');
let data = { nodes: [], nodesMeta: {}, namespaces: [], services: [], deployments: [], jobs: [] }; // the lists of the ACTIVE connection (each connection keeps its own: see kstore)
let view = 'namespaces';
const PROTECTED_NS = (n) => n === 'default' || n.startsWith('kube-'); // the server refuses deleting these too

// loadNamespaces fills the namespace dropdown and the Namespaces page, and keeps the working namespace valid.
async function loadNamespaces() {
  const c0 = cluster;
  const list = tag(await api(kq('/api/namespaces', c0)), c0);
  if (c0 !== cluster) return; // the connection changed while this was loading
  data.namespaces = list;
  nsAt[c0] = Date.now();
  const names = list.map((n) => n.name);
  nsSel.selected = selected.filter((n) => names.includes(n)); // none = every namespace
  selected = nsSel.selected;
  nsSel.options(names);
  render('namespaces');
  $('nscreate').hidden = settings.read_only;
}

// The automatic refresh (see 09-boot.js): `auto` is set while it loads, `fails` counts failed refreshes in a row,
// `wait` is how many ticks to skip (backing off), `tick` counts refreshes (pod metrics only every third).
const poll = { auto: false, tick: 0, fails: 0, wait: 0, enabled: (() => { try { return localStorage.getItem('k8ui.autorefresh') !== '0'; } catch { return true; } })() }; // enabled: the Auto refresh checkbox, remembered
// a failed load: shown as a toast, except repeats during backoff (the first failure of a streak is shown)
const loadErr = (e) => { if (!(poll.auto && poll.fails)) toast(e.message); return false; };
// A list page loads when it is opened, on the refresh tick while it is open, and when its filter changes.
// loadedAt remembers when each list (per connection, page and namespace filter) was last read: opening a page
// again within one refresh interval shows what is there instead of asking the cluster again.
const busy = {}; // a view whose last load has not finished is not loaded again by the refresh timer
const loadedAt = {};
const loadKey = (v) => `${cluster}|${v}|${nsKey()}`;
const isFresh = (v) => Date.now() - (loadedAt[loadKey(v)] || 0) < settings.refresh_seconds * 1000;
let nsAt = {}; // connection -> when the namespace list was last read (it fills the namespace dropdown)
async function load(v, cached) { // resolves to false when the reload failed
  if (K8S_VIEWS.includes(v) && !kOk) return; // not connected: the page says so, nothing to load
  if (cached && isFresh(v)) return true;
  busy[v] = true;
  try {
    const ok = await loadView(v);
    if (ok) loadedAt[loadKey(v)] = Date.now();
    return ok;
  } finally { busy[v] = false; }
}
// the pages with the namespace dropdown need the namespace list once (it is read again when the Namespaces page is open,
// after a create or delete, or when the dropdown is opened more than a minute after the last read)
function ensureNamespaces() {
  if (!nsAt[cluster] || !data.namespaces.length) loadNamespaces().catch((e) => loadErr(e));
}
$('nsdd').addEventListener('toggle', (e) => { if (e.target.open && Date.now() - (nsAt[cluster] || 0) > 60000) loadNamespaces().catch(() => {}); }, true);
async function loadView(v) {
  if (v === 'nodes') return loadNodes();
  if (v === 'namespaces') return loadNamespaces().then(() => true, loadErr);
  if (['services', 'deployments', 'jobs', 'pods'].includes(v)) ensureNamespaces();
  if (v === 'pods') return loadPods();
  if (!LISTS[v]) return;
  const ns = nsKey(), c0 = cluster;
  try {
    const l = await api(LISTS[v].url(ns));
    if (ns !== nsKey() || c0 !== cluster) return; // namespace or connection changed while this was loading
    data[v] = tag(l, c0);
  } catch (e) { return loadErr(e); }
  render(v);
  return true;
}
const nsSel = multiSelect($('nsdd'), 'All namespaces', 'namespaces', (v) => setNs(v));
const statusSel = multiSelect($('statusdd'), 'All statuses', 'statuses', () => renderPods());
const ownerSel = multiSelect($('ownerdd'), 'All owners', 'owners', () => renderPods());
const NO_OWNER = 'None';

function setNs(list) {
  selected = list;
  saveSession();
  nsSel.set(list);
  data.services = []; data.deployments = []; data.jobs = []; pods = [];
  ['services', 'deployments', 'jobs', 'pods'].forEach(render);
  render('namespaces');
  if (['services', 'deployments', 'jobs', 'pods'].includes(view)) load(view); // only the page you are on; the others load when opened
}

const q = () => $('search').value.trim().toLowerCase();
const match = (texts) => !q() || texts.some((t) => String(t).toLowerCase().includes(q()));

// Tables draw 100 rows and add 100 more on "Show more" (the data is already loaded: the filters need all of it).
const PAGE = 100, shown = {}, shownSig = {};
function page(key, rows, sig) { // the rows to draw; starts over when the filters change
  if (shownSig[key] !== sig) { shownSig[key] = sig; shown[key] = PAGE; }
  return rows.slice(0, shown[key]);
}
const moreRow = (key, total, cols) => (total > shown[key]
  ? `<tr class="morerow"><td colspan="${cols}"><button type="button" class="sm" data-more="${key}">Show ${Math.min(PAGE, total - shown[key])} more (${total - shown[key]} left)</button></td></tr>` : '');
document.addEventListener('click', (e) => {
  const b = e.target.closest('button[data-more]');
  if (!b) return;
  const k = b.dataset.more;
  shown[k] += PAGE;
  if (k === 'esidx') renderEsIndices(); else render(k);
});
function render(v) {
  if (v === 'pods') return renderPods();
  if (v === 'nodes') return renderNodes();
  const L = LISTS[v];
  if (!L) return;
  const all = data[v], rows = all.filter((r) => match(L.text(r)));
  $(L.body).innerHTML = rows.length
    ? page(v, rows, cluster + '|' + q() + '|' + nsKey()).map((r) => `<tr>${L.row(r)}</tr>`).join('') + moreRow(v, rows.length, L.cols)
    : `<tr><td colspan="${L.cols}" class="muted">${emptyText(all.length)}</td></tr>`;
}

[['svcrows', 'services'], ['deprows', 'deployments']].forEach(([id, kind]) => $(id).addEventListener('click', async (e) => {
  const b = e.target.closest('button[data-w]');
  const r = b && data[kind].find((x) => `${x.namespace}/${x.name}` === b.dataset.k);
  if (!r) return;
  if (b.dataset.w === 'scale') return scaleDeployment(r);
  if (b.dataset.w === 'rollout') return rolloutRestart(r);
  if (b.dataset.w === 'logs') { klogs(r, kind); return openLogs(r, '', kind); } // every pod it selects, in one tab
  try {
    const { pod, details } = await api(kq(`/api/${kind}/${r.namespace}/${encodeURIComponent(r.name)}/pod`, r.cluster));
    const p = pods.find((x) => x.namespace === r.namespace && x.name === pod) || (details && { ...details, cluster: r.cluster }); // the Pods page may not have been opened
    if (!p) return toast(`could not find a pod for ${r.name}`);
    if (b.dataset.w === 'shell') { openShell(p); } else forward(p, kind === 'services' ? r : null);
  } catch (err) { toast(err.message); }
}));

$('jobrows').addEventListener('click', async (e) => {
  const b = e.target.closest('button[data-w]');
  const r = b && data.jobs.find((x) => `${x.namespace}/${x.name}` === b.dataset.k);
  if (!r) return;
  if (b.dataset.w === 'logs') { klogs(r, 'jobs'); return openLogs(r, '', 'jobs'); }
  if (b.dataset.w === 'trigger') {
    if (!await askConfirm(`Run job ${r.name} again? A new job is created from its spec.`, { title: 'Run job again', ok: 'Run' })) return;
    try {
      const j = await api(kq(`/api/jobs/${r.namespace}/${encodeURIComponent(r.name)}/trigger`, r.cluster), { method: 'POST' });
      ktoast(r.namespace, `get job ${r.name} -o yaml | kubectl create -f -   (a copy, renamed)`, `job.batch/${j.name} created`);
      load('jobs');
    } catch (err) { toast(err.message); }
    return;
  }
  if (!await askConfirm(`Delete job ${r.name}? Its pods are deleted with it.`, { title: 'Delete job', ok: 'Delete', danger: true })) return;
  try {
    await api(kq(`/api/jobs/${r.namespace}/${encodeURIComponent(r.name)}`, r.cluster), { method: 'DELETE' });
    ktoast(r.namespace, `delete job ${r.name}`, `job.batch "${r.name}" deleted`);
    load('jobs');
  } catch (err) { ktoast(r.namespace, `delete job ${r.name}`, err.message); }
});

$('nsrows').addEventListener('click', async (e) => {
  const b = e.target.closest('button[data-del]');
  if (!b) return;
  const name = b.dataset.del;
  const a = await ask({
    title: 'Delete namespace', ok: 'Delete', danger: true,
    message: `Delete namespace ${name} and everything in it (pods, services, secrets, volumes claims)? This cannot be undone.`,
    fields: [{ name: 'n', label: `Type ${name} to confirm`, required: true, pattern: name.replace(/[^a-z0-9-]/g, ''), hint: `type ${name}` }],
  });
  if (!a) return;
  const args = `delete namespace ${name}`;
  try {
    await api(kq(`/api/namespaces/${encodeURIComponent(name)}`, cluster), { method: 'DELETE' });
    ktoast('', args, `namespace "${name}" deleted`);
    if (selected.includes(name)) setNs(selected.filter((n) => n !== name)); else await loadNamespaces();
  } catch (err) { ktoast('', args, err.message); }
});
$('nscreate').onclick = async () => {
  const a = await ask({
    title: 'Create namespace', ok: 'Create',
    fields: [{ name: 'n', label: 'Name', required: true, maxlength: 63, pattern: '[a-z0-9]|[a-z0-9](?:[a-z0-9]|-)*[a-z0-9]', hint: 'lowercase letters, digits and -; starts and ends with a letter or digit' }],
  });
  if (!a) return;
  const name = a.n.trim(), args = `create namespace ${name}`;
  try {
    await api(kq('/api/namespaces', cluster), { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name }) });
    ktoast('', args, `namespace/${name} created`);
    await loadNamespaces();
  } catch (err) { ktoast('', args, err.message); }
};


// ---- nodes: the health of the whole cluster (not namespaced: the namespace filter does not apply)
async function loadNodes() {
  const c0 = cluster;
  try {
    const r = await api(kq('/api/nodes', c0));
    if (c0 !== cluster) return;
    data.nodes = tag(r.nodes, c0);
    data.nodesMeta = { metrics: r.metrics, requests: r.requests, err: '' };
  } catch (e) {
    if (c0 !== cluster) return;
    if (e.status === 403) { data.nodes = []; data.nodesMeta = { err: `this account may not list nodes (${e.message})` }; } // permanent: say so, do not retry noisily
    else return loadErr(e);
  }
  renderNodes();
  return true;
}
const pctOf = (a, b) => (b > 0 ? Math.round((a / b) * 100) : 0);
const ubar = (p) => `<div class="ubar ${p >= 90 ? 'bad' : p >= 70 ? 'warn' : ''}"><i style="width:${Math.min(p, 100)}%"></i></div>`;
// one resource cell: live use against what the node can allocate (when metrics-server is there), and what its pods request
function useCell(use, req, alloc, fmt, meta) {
  if (!alloc) return none;
  const p = use != null ? pctOf(use, alloc) : pctOf(req, alloc);
  const main = use != null ? `${fmt(use)} / ${fmt(alloc)} · ${p}%` : meta.requests ? `req ${fmt(req)} / ${fmt(alloc)} · ${p}%` : fmt(alloc);
  const sub = use != null && meta.requests ? `<small class="muted">requested ${fmt(req)} (${pctOf(req, alloc)}%)</small>` : '';
  return `${meta.requests || use != null ? ubar(p) : ''}<div class="usetxt">${main}</div>${sub}`;
}
function renderNodes() {
  const all = data.nodes, meta = data.nodesMeta || {};
  const rows = all.filter((n) => match([n.name, n.status, n.version, ...n.roles, ...n.pressure]));
  const sum = (f) => all.reduce((t, n) => t + (f(n) || 0), 0);
  const ready = all.filter((n) => n.status === 'Ready').length, bad = all.length - ready;
  const cpuA = sum((n) => n.cpu_alloc_milli), memA = sum((n) => n.mem_alloc_bytes), podsA = sum((n) => n.pods_alloc);
  const cpuU = sum((n) => n.cpu_use_milli), memU = sum((n) => n.mem_use_bytes);
  const tile = (label, value, subl, p) => `<div class="tile card"><span class="t">${label}</span><b>${value}</b>${p != null ? ubar(p) : ''}<small class="muted">${subl}</small></div>`;
  $('nodesum').innerHTML = all.length ? [
    tile('Nodes ready', `${ready} / ${all.length}`, bad ? `<span class="bad-t">${bad} not ready</span>` : 'all ready', null),
    tile('CPU', meta.metrics ? `${pctOf(cpuU, cpuA)}%` : `${pctOf(sum((n) => n.cpu_req_milli), cpuA)}%`, meta.metrics ? `${fmtCpu(cpuU)} of ${fmtCpu(cpuA)} in use` : meta.requests ? `${fmtCpu(sum((n) => n.cpu_req_milli))} of ${fmtCpu(cpuA)} requested` : `${fmtCpu(cpuA)} allocatable`, meta.metrics ? pctOf(cpuU, cpuA) : meta.requests ? pctOf(sum((n) => n.cpu_req_milli), cpuA) : null),
    tile('Memory', meta.metrics ? `${pctOf(memU, memA)}%` : `${pctOf(sum((n) => n.mem_req_bytes), memA)}%`, meta.metrics ? `${fmtBytes(memU)} of ${fmtBytes(memA)} in use` : meta.requests ? `${fmtBytes(sum((n) => n.mem_req_bytes))} of ${fmtBytes(memA)} requested` : `${fmtBytes(memA)} allocatable`, meta.metrics ? pctOf(memU, memA) : meta.requests ? pctOf(sum((n) => n.mem_req_bytes), memA) : null),
    tile('Pods', meta.requests ? `${sum((n) => n.pods)} / ${podsA}` : String(podsA), meta.requests ? 'running or pending, of capacity' : 'capacity', meta.requests ? pctOf(sum((n) => n.pods), podsA) : null),
  ].join('') : '';
  $('nodenote').textContent = meta.err ? '' : all.length ? `${meta.metrics ? '' : 'No metrics-server: showing what pods request, not live use. '}${meta.requests ? '' : 'The pod list is not readable: requests and pod counts are not shown.'}`.trim() : '';
  $('noderows').innerHTML = rows.length ? rows.map((n) => `<tr>
      <td class="name">${esc(n.name)}${n.roles.map((r) => ' ' + pill(r)).join('')}${n.unschedulable ? ' ' + pill('cordoned', 'bad') : ''}</td>
      <td>${pill(n.status, n.status === 'Ready' ? 'ok' : 'bad')}${n.pressure.map((p) => ' ' + pill(p, 'bad')).join('')}</td>
      <td class="ver">${esc(n.version)}</td>
      <td class="res">${useCell(n.cpu_use_milli, n.cpu_req_milli, n.cpu_alloc_milli, fmtCpu, meta)}</td>
      <td class="res">${useCell(n.mem_use_bytes, n.mem_req_bytes, n.mem_alloc_bytes, fmtBytes, meta)}</td>
      <td class="num">${meta.requests ? `${n.pods} / ${n.pods_alloc}` : n.pods_alloc}</td><td>${esc(n.age)}</td></tr>`).join('')
    : `<tr><td colspan="7" class="muted">${esc(meta.err || emptyText(all.length))}</td></tr>`;
}

// CPU and memory columns: only when the cluster has metrics-server (one 404 hides them for that connection)
const kmetrics = {}, podUse = {}; // connection -> true | false; "cluster|ns/name" -> { cpu_milli, mem_bytes }
const fmtCpu = (m) => (m < 1000 ? `${m}m` : `${(m / 1000).toFixed(2)}`);
const metricCells = (p) => { const u = podUse[`${p.cluster}|${p.namespace}/${p.name}`]; return `<td class="num m">${u ? fmtCpu(u.cpu_milli) : none}</td><td class="num m">${u ? fmtBytes(u.mem_bytes) : none}</td>`; };
const METRICS_SHOWN = matchMedia('(min-width: 1301px)'); // below this the CPU / Memory columns are hidden: do not fetch them
METRICS_SHOWN.addEventListener('change', (e) => { if (e.matches && view === 'pods' && kOk) load('pods'); });
async function loadPods() {
  const ns = nsKey(), c0 = cluster;
  const wantMetrics = METRICS_SHOWN.matches && (!poll.auto || poll.tick % 3 === 0); // CPU and memory move slowly: the automatic refresh asks every third time
  const metrics = !wantMetrics ? Promise.resolve(undefined) : kmetrics[c0] === false ? Promise.resolve(null)
    : api(kq(`/api/pods/metrics?namespace=${encodeURIComponent(ns)}`, c0)).catch((e) => { if (e.status === 404) kmetrics[c0] = false; return null; });
  try {
    const l = tag(await api(kq(`/api/pods?namespace=${encodeURIComponent(ns)}`, c0)), c0);
    if (ns !== nsKey() || c0 !== cluster) return; // the selection or connection changed while this was loading
    pods = l;
  } catch (e) {
    return loadErr(e);
  }
  const m = await metrics;
  if (m) { kmetrics[c0] = true; m.forEach((x) => (podUse[`${c0}|${x.namespace}/${x.name}`] = x)); }
  if (c0 !== cluster) return;
  if (m !== undefined) $('podsView').classList.toggle('nometrics', kmetrics[c0] === false || !m); // undefined: not asked this time, keep the columns as they are
  renderPods();
  return true;
}

function renderPods() {
  statusSel.options(pods.map((p) => p.phase));
  ownerSel.options(pods.map((p) => p.owner_kind ?? NO_OWNER));
  const rows = pods.map((p, i) => [p, i]).filter(([p]) =>
    (!statusSel.selected.length || statusSel.selected.includes(p.phase)) &&
    (!ownerSel.selected.length || ownerSel.selected.includes(p.owner_kind ?? NO_OWNER)) &&
    match([p.name, p.namespace, p.phase, p.owner_kind ?? '', ...p.images]));
  const sig = [cluster, q(), nsKey(), statusSel.selected, ownerSel.selected].join('|');
  $('pods').innerHTML = rows.length
    ? page('pods', rows, sig).map(([p, i]) => `<tr>
        <td class="name" title="${esc(p.images.join('\n'))}">${esc(p.name)}</td><td>${esc(p.namespace)}</td>
        <td>${esc(p.ready)}</td><td>${pill(p.phase, p.phase === 'Running' || p.phase === 'Succeeded' ? 'ok' : p.phase === 'Failed' || p.phase === 'Unknown' ? 'bad' : '')}</td>
        <td class="num">${p.restarts}</td>${metricCells(p)}<td>${esc(p.age)}</td>
        <td>${p.owner_kind ? pill(p.owner_kind) : none}</td>
        <td class="actions">
          <button data-a="logs" data-i="${i}" class="sm" title="Logs">${ICONS.logs}Logs</button>
          <button data-a="describe" data-i="${i}" class="sm" title="Describe">${ICONS.info}Describe</button>
          ${settings.read_only ? '' : `<button data-a="shell" data-i="${i}" class="sm" title="Shell">${ICONS.shell}Shell</button>
          <button data-a="fwd" data-i="${i}" class="sm" title="Forward">${ICONS.fwd}Forward</button>
          <button data-a="restart" data-i="${i}" class="sm danger" title="Restart">${ICONS.restart}Restart</button>`}
        </td></tr>`).join('') + moreRow('pods', rows.length, 10)
    : `<tr><td colspan="10" class="muted">${emptyText(pods.length)}</td></tr>`;
}
$('search').oninput = () => {
  view === 'forwards' ? renderForwards() : render(view);
};

$('pods').addEventListener('click', (e) => {
  const b = e.target.closest('button');
  if (!b || !b.dataset.a) return;
  const p = pods[b.dataset.i];
  ({ logs: (x) => { klogs(x); openLogs(x); }, describe: describePod, shell: openShell, fwd: forward, restart })[b.dataset.a](p);
});

// ---- deployments: scale and rollout restart
async function scaleDeployment(r) {
  const a = await ask({ title: `Scale ${r.name}`, ok: 'Scale', fields: [{ name: 'n', label: 'Replicas', type: 'number', min: 0, max: 10000, required: true, value: r.replicas, hint: `now ${r.replicas}` }] });
  if (!a) return;
  const n = parseInt(a.n, 10), args = `scale deploy/${r.name} --replicas=${n}`;
  try {
    await api(kq(`/api/deployments/${r.namespace}/${encodeURIComponent(r.name)}/scale`, r.cluster), { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ replicas: n }) });
    ktoast(r.namespace, args, `deployment.apps/${r.name} scaled`);
    load('deployments');
  } catch (e) { ktoast(r.namespace, args, e.message); }
}
async function rolloutRestart(r) {
  if (!await askConfirm(`Restart ${r.name}? Its pods are replaced one by one.`, { title: 'Rollout restart', ok: 'Restart', danger: true })) return;
  const args = `rollout restart deploy/${r.name}`;
  try {
    await api(kq(`/api/deployments/${r.namespace}/${encodeURIComponent(r.name)}/restart`, r.cluster), { method: 'POST' });
    ktoast(r.namespace, args, `deployment.apps/${r.name} restarted`);
    load('deployments');
  } catch (e) { ktoast(r.namespace, args, e.message); }
}

// ---- describe: status, why a container restarted, and the events, as JSON
async function describePod(p) {
  ktoast(p.namespace, `describe pod ${p.name}`);
  try { showJsonDialog(`${p.name}`, await api(kq(`/api/pods/${p.namespace}/${encodeURIComponent(p.name)}/describe`, p.cluster))); } catch (e) { toast(e.message); }
}

// ---- restart
async function restart(p) {
  const warn = p.owner_kind
    ? `Delete pod ${p.name}? ${p.owner_kind} will recreate it.`
    : `Pod ${p.name} has NO owner: deleting it is permanent, nothing recreates it. Delete anyway?`;
  if (!await askConfirm(warn, { title: 'Restart pod', ok: 'Delete pod', danger: true })) return;
  try {
    await api(kq(`/api/pods/${p.namespace}/${p.name}/restart`, p.cluster), { method: 'POST' });
    ktoast(p.namespace, `delete pod ${p.name}`, `pod "${p.name}" deleted`);
    setTimeout(loadPods, 1000);
  } catch (e) { ktoast(p.namespace, `delete pod ${p.name}`, e.message); }
}
