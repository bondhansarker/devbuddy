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
const kbadges = {}; // connection name -> { view: count } shown in the sidebar
const K8S_VIEWS = ['namespaces', 'services', 'deployments', 'jobs', 'pods']; // pages that need an active connection
const K_PAGES = [['namespaces', 'Namespaces'], ['services', 'Services'], ['deployments', 'Deployments'], ['jobs', 'Jobs'], ['pods', 'Pods']];
// every action shows the kubectl command it is the same as (and its outcome) as a toast
function ktoast(ns, args, result) {
  toast(`$ kubectl${ns ? ' -n ' + ns : ''} ${args}${result ? '\n' + result : ''}`);
}
const KTARGET = { pods: 'pod', deployments: 'deploy', services: 'svc', jobs: 'job' };
function klogs(r, kind = 'pods') { ktoast(r.namespace, `logs -f ${KTARGET[kind]}/${r.name} --tail=${settings.log_tail}${kind === 'pods' ? '' : ' --all-containers'}`); }
function setBadge(v, n) {
  (kbadges[cluster] ??= {})[v] = n;
  const el = document.querySelector(`#knav .kn[data-kc="${CSS.escape(cluster)}"] [data-badge="${v}"]`);
  if (el) el.textContent = n;
}
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
let data = { namespaces: [], services: [], deployments: [], jobs: [] }; // the lists of the ACTIVE connection (each connection keeps its own: see kstore)
let view = 'namespaces';
const PROTECTED_NS = (n) => n === 'default' || n.startsWith('kube-'); // the server refuses deleting these too

// loadNamespaces fills the namespace dropdown and the Namespaces page, and keeps the working namespace valid.
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
  $('nscreate').hidden = settings.read_only;
}

// The automatic refresh (see 09-boot.js): `auto` is set while it loads, `fails` counts failed refreshes in a row,
// `wait` is how many ticks to skip (backing off), `tick` counts refreshes (pod metrics only every third).
const poll = { auto: false, tick: 0, fails: 0, wait: 0, enabled: (() => { try { return localStorage.getItem('k8ui.autorefresh') !== '0'; } catch { return true; } })() }; // enabled: the Auto refresh checkbox, remembered
// a failed load: shown as a toast, except repeats during backoff (the first failure of a streak is shown)
const loadErr = (e) => { if (!(poll.auto && poll.fails)) toast(e.message); return false; };
async function load(v) { // resolves to false when the reload failed
  if (K8S_VIEWS.includes(v) && !kOk) return; // not connected: the page says so, nothing to load
  busy[v] = true;
  try { return await loadView(v); } finally { busy[v] = false; }
}
async function loadView(v) {
  if (v === 'namespaces') return loadNamespaces().then(() => true, loadErr);
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
  setBadge(v, all.length);
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
    const { pod } = await api(kq(`/api/${kind}/${r.namespace}/${encodeURIComponent(r.name)}/pod`, r.cluster));
    const p = pods.find((x) => x.namespace === r.namespace && x.name === pod);
    if (!p) return toast('pod list not loaded yet, try again');
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

// CPU and memory columns: only when the cluster has metrics-server (one 404 hides them for that connection)
const kmetrics = {}, podUse = {}; // connection -> true | false; "cluster|ns/name" -> { cpu_milli, mem_bytes }
const fmtCpu = (m) => (m < 1000 ? `${m}m` : `${(m / 1000).toFixed(2)}`);
const metricCells = (p) => { const u = podUse[`${p.cluster}|${p.namespace}/${p.name}`]; return `<td class="num m">${u ? fmtCpu(u.cpu_milli) : none}</td><td class="num m">${u ? fmtBytes(u.mem_bytes) : none}</td>`; };
async function loadPods() {
  const ns = nsKey(), c0 = cluster;
  const wantMetrics = !poll.auto || poll.tick % 3 === 0; // CPU and memory move slowly: the automatic refresh asks every third time
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
  setBadge('pods', pods.length);
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
