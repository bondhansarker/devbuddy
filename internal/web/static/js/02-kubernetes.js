// Kubernetes lists (namespaces, services, deployments, jobs, pods), their filters and row actions.
// (Plain scripts share one global scope: index.html loads them in numeric order.)

// Logs, Shell and Forward of a deployment or service act on the pods it selects (shell and forward on one ready pod).
const logsCell = (r) => {
  const k = esc(`${r.namespace}/${r.name}`);
  return `<td class="actions"><button type="button" class="sm" data-w="logs" data-k="${k}">Logs</button>${settings.read_only ? '' :
    `<button type="button" class="sm" data-w="shell" data-k="${k}">Shell</button><button type="button" class="sm" data-w="fwd" data-k="${k}">Forward</button>`}</td>`;
};
const jobCell = (r) => {
  const k = esc(`${r.namespace}/${r.name}`);
  return `<td class="actions"><button type="button" class="sm" data-w="logs" data-k="${k}">Logs</button>${settings.read_only ? '' : `<button type="button" class="sm" data-w="trigger" data-k="${k}" title="Run this job again as a new job">Trigger</button><button type="button" class="sm danger" data-w="delete" data-k="${k}">Delete</button>`}</td>`;
};
// ---- Kubernetes connections: several can be configured; every k8s call carries ?cluster=
let clusters = [], cluster = '', kOk = false, kWhy = '';
const K8S_VIEWS = ['namespaces', 'services', 'deployments', 'jobs', 'pods', 'forwards'];
const kq = (path, c = cluster) => `${path}${path.includes('?') ? '&' : '?'}cluster=${encodeURIComponent(c)}`;
const tag = (rows, c = cluster) => rows.map((r) => ({ ...r, cluster: c })); // every object remembers the connection it came from
const clusterInfo = () => clusters.find((c) => c.name === cluster);
// One entry per plain list view; pods have their own actions below.
const LISTS = {
  namespaces: { url: () => kq('/api/namespaces'), body: 'nsrows', badge: 'nscount', cols: 4,
    text: (r) => [r.name, r.status],
    row: (r) => `<td class="name">${esc(r.name)}${selected.includes(r.name) ? ' ' + pill('selected') : ''}</td>
      <td>${pill(r.status, r.status === 'Active' ? 'ok' : 'bad')}</td><td>${esc(r.age)}</td>
      <td class="actions"><button type="button" class="sm" data-open="${esc(r.name)}">Open</button></td>` },
  services: { url: (ns) => kq(`/api/services?namespace=${encodeURIComponent(ns)}`), body: 'svcrows', badge: 'svccount', cols: 8,
    text: (r) => [r.name, r.namespace, r.type, r.cluster_ip, r.external_ip ?? '', ...r.ports],
    row: (r) => `<td class="name">${esc(r.name)}</td><td>${esc(r.namespace)}</td><td>${pill(r.type)}</td><td>${esc(r.cluster_ip)}</td>
      <td class="name">${r.external_ip ? esc(r.external_ip) : none}</td><td>${r.ports.length ? esc(r.ports.join(', ')) : none}</td><td>${esc(r.age)}</td>${logsCell(r)}` },
  deployments: { url: (ns) => kq(`/api/deployments?namespace=${encodeURIComponent(ns)}`), body: 'deprows', badge: 'depcount', cols: 7,
    text: (r) => [r.name, r.namespace],
    row: (r) => `<td class="name">${esc(r.name)}</td><td>${esc(r.namespace)}</td><td>${esc(r.ready)}</td><td class="num">${r.up_to_date}</td><td class="num">${r.available}</td><td>${esc(r.age)}</td>${logsCell(r)}` },
  jobs: { url: (ns) => kq(`/api/jobs?namespace=${encodeURIComponent(ns)}`), body: 'jobrows', badge: 'jobcount', cols: 8,
    text: (r) => [r.name, r.namespace, r.status],
    row: (r) => `<td class="name">${esc(r.name)}</td><td>${esc(r.namespace)}</td><td class="num">${esc(r.completions)}</td>
      <td class="num">${r.active}</td><td class="num">${r.failed}</td><td>${pill(r.status, r.status === 'Complete' ? 'ok' : r.status === 'Failed' ? 'bad' : '')}</td><td>${esc(r.age)}</td>${jobCell(r)}` },
};
const emptyText = (n) => (n ? 'nothing matches the filter' : 'nothing here');
const data = { namespaces: [], services: [], deployments: [], jobs: [] };
let view = 'namespaces';

// loadNamespaces keeps the working namespace valid.
async function loadNamespaces() {
  const c0 = cluster;
  const list = tag(await api(kq('/api/namespaces', c0)), c0);
  if (c0 !== cluster) return; // the connection changed while this was loading
  data.namespaces = list;
  const names = list.map((n) => n.name);
  nsSel.selected = selected.filter((n) => names.includes(n)); // none = every namespace
  selected = nsSel.selected;
  nsSel.options(names);
  render('namespaces');
}

async function load(v) {
  if (K8S_VIEWS.includes(v) && !kOk) return; // not connected: the page says so, nothing to load
  busy[v] = true;
  try { await loadView(v); } finally { busy[v] = false; }
}
async function loadView(v) {
  if (v === 'namespaces') return loadNamespaces().catch((e) => toast(e.message));
  if (v === 'pods') return loadPods();
  if (!LISTS[v]) return;
  const ns = nsKey(), c0 = cluster;
  try {
    const l = await api(LISTS[v].url(ns));
    if (ns !== nsKey() || c0 !== cluster) return; // namespace or connection changed while this was loading
    data[v] = tag(l, c0);
  } catch (e) { return toast(e.message); }
  render(v);
}

const loadAll = () => !kOk ? Promise.resolve() : Promise.all(['services', 'deployments', 'jobs', 'pods'].map(load));

const nsSel = multiSelect($('nsdd'), 'All namespaces', 'namespaces', (v) => setNs(v));
const statusSel = multiSelect($('statusdd'), 'All statuses', 'statuses', () => renderPods());
const ownerSel = multiSelect($('ownerdd'), 'All owners', 'owners', () => renderPods());
const NO_OWNER = 'None';

function setNs(list) {
  selected = list;
  nsSel.set(list);
  data.services = []; data.deployments = []; data.jobs = []; pods = [];
  ['services', 'deployments', 'jobs', 'pods'].forEach(render);
  render('namespaces');
  renderForwards();
  loadAll();
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
const busy = {}; // a view whose last load has not finished is not loaded again by the refresh timer

function render(v) {
  if (v === 'pods') return renderPods();
  const L = LISTS[v];
  if (!L) return;
  const all = data[v], rows = all.filter((r) => match(L.text(r)));
  $(L.badge).textContent = all.length;
  $(L.body).innerHTML = rows.length
    ? page(v, rows, q() + '|' + nsKey()).map((r) => `<tr>${L.row(r)}</tr>`).join('') + moreRow(v, rows.length, L.cols)
    : `<tr><td colspan="${L.cols}" class="muted">${emptyText(all.length)}</td></tr>`;
}

[['svcrows', 'services'], ['deprows', 'deployments']].forEach(([id, kind]) => $(id).addEventListener('click', async (e) => {
  const b = e.target.closest('button[data-w]');
  const r = b && data[kind].find((x) => `${x.namespace}/${x.name}` === b.dataset.k);
  if (!r) return;
  if (b.dataset.w === 'logs') return openLogs(r, '', kind); // every pod it selects, in one tab
  try {
    const { pod } = await api(kq(`/api/${kind}/${r.namespace}/${encodeURIComponent(r.name)}/pod`, r.cluster));
    const p = pods.find((x) => x.namespace === r.namespace && x.name === pod);
    if (!p) return toast('pod list not loaded yet, try again');
    if (b.dataset.w === 'shell') openShell(p); else forward(p, kind === 'services' ? r : null);
  } catch (err) { toast(err.message); }
}));

$('jobrows').addEventListener('click', async (e) => {
  const b = e.target.closest('button[data-w]');
  const r = b && data.jobs.find((x) => `${x.namespace}/${x.name}` === b.dataset.k);
  if (!r) return;
  if (b.dataset.w === 'logs') return openLogs(r, '', 'jobs');
  if (b.dataset.w === 'trigger') {
    if (!confirm(`Run job ${r.name} again? A new job is created from its spec.`)) return;
    try {
      const j = await api(kq(`/api/jobs/${r.namespace}/${encodeURIComponent(r.name)}/trigger`, r.cluster), { method: 'POST' });
      toast(`started ${j.name}`);
      load('jobs');
    } catch (err) { toast(err.message); }
    return;
  }
  if (!confirm(`Delete job ${r.name}? Its pods are deleted with it.`)) return;
  try {
    await api(kq(`/api/jobs/${r.namespace}/${encodeURIComponent(r.name)}`, r.cluster), { method: 'DELETE' });
    toast(`${r.name} deleted`);
    load('jobs');
  } catch (err) { toast(err.message); }
});

$('nsrows').addEventListener('click', (e) => {
  const b = e.target.closest('button[data-open]');
  if (!b) return;
  setNs([b.dataset.open]);
  showView('pods');
});

async function loadPods() {
  const ns = nsKey(), c0 = cluster;
  try {
    const l = tag(await api(kq(`/api/pods?namespace=${encodeURIComponent(ns)}`, c0)), c0);
    if (ns !== nsKey() || c0 !== cluster) return; // the selection or connection changed while this was loading
    pods = l;
  } catch (e) {
    return toast(e.message);
  }
  renderPods();
}

function renderPods() {
  statusSel.options(pods.map((p) => p.phase));
  ownerSel.options(pods.map((p) => p.owner_kind ?? NO_OWNER));
  const rows = pods.map((p, i) => [p, i]).filter(([p]) =>
    (!statusSel.selected.length || statusSel.selected.includes(p.phase)) &&
    (!ownerSel.selected.length || ownerSel.selected.includes(p.owner_kind ?? NO_OWNER)) &&
    match([p.name, p.namespace, p.phase, p.owner_kind ?? '', ...p.images]));
  $('podcount').textContent = pods.length;
  const sig = [q(), nsKey(), statusSel.selected, ownerSel.selected].join('|');
  $('pods').innerHTML = rows.length
    ? page('pods', rows, sig).map(([p, i]) => `<tr>
        <td class="name">${esc(p.name)}</td><td>${esc(p.namespace)}</td>
        <td>${esc(p.ready)}</td><td>${pill(p.phase, p.phase === 'Running' || p.phase === 'Succeeded' ? 'ok' : p.phase === 'Failed' || p.phase === 'Unknown' ? 'bad' : '')}</td>
        <td class="name">${p.images.map(image).join('<br>')}</td>
        <td class="num">${p.restarts}</td><td>${esc(p.age)}</td>
        <td>${p.owner_kind ? pill(p.owner_kind) : none}</td>
        <td class="actions">
          <button data-a="logs" data-i="${i}" class="sm">Logs</button>
          ${settings.read_only ? '' : `<button data-a="shell" data-i="${i}" class="sm">Shell</button>
          <button data-a="fwd" data-i="${i}" class="sm">Forward</button>
          <button data-a="restart" data-i="${i}" class="sm danger">Restart</button>`}
        </td></tr>`).join('') + moreRow('pods', rows.length, 9)
    : `<tr><td colspan="9" class="muted">${emptyText(pods.length)}</td></tr>`;
}
$('search').oninput = () => {
  view === 'forwards' ? renderForwards() : render(view);
};

$('pods').addEventListener('click', (e) => {
  const b = e.target.closest('button');
  if (!b || !b.dataset.a) return;
  const p = pods[b.dataset.i];
  ({ logs: openLogs, shell: openShell, fwd: forward, restart })[b.dataset.a](p);
});

// ---- restart
async function restart(p) {
  const warn = p.owner_kind
    ? `Delete pod ${p.name}? ${p.owner_kind} will recreate it.`
    : `Pod ${p.name} has NO owner: deleting it is permanent, nothing recreates it. Delete anyway?`;
  if (!confirm(warn)) return;
  try {
    await api(kq(`/api/pods/${p.namespace}/${p.name}/restart`, p.cluster), { method: 'POST' });
    toast(`${p.name} deleted`);
    setTimeout(loadPods, 1000);
  } catch (e) { toast(e.message); }
}
