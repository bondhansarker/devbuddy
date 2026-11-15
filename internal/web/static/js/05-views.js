// The Kubernetes connection picker, the page switcher and the theme.
// (Plain scripts share one global scope: index.html loads them in numeric order.)

// ---- Kubernetes connections. Nothing connects until you press Connect. Each connected cluster is its own
// node in the sidebar with its own pages, and keeps its own lists (kstore): pods of two clusters never mix.
const restored = {}; // connections whose open shells / log tabs were already reopened

function syncKubernetes() {
  const ci = clusterInfo();
  const n = clusters.filter((c) => kstate[c.name]?.status === 'connected').length;
  kOk = !!ci && kstate[ci.name]?.status === 'connected';
  kWhy = 'No Kubernetes cluster is connected. Open Clusters and press Connect, or switch one on in the sidebar.';
  $('kpill').textContent = n ? `${n} on` : 'off';
  $('kpill').className = 'pill ' + (n ? 'ok' : '');
  $('ctx').textContent = ci && kOk ? ci.name : 'not connected';
  $('ctx').title = ci ? ci.context : '';
  $('prod').hidden = !(ci && kOk && /prod/i.test(ci.name + ci.context));
}
function syncK8sBar() {
  const k = K8S_VIEWS.includes(view);
  $('knotice').hidden = !(k && !kOk);
  $('knoticetext').textContent = kWhy;
  if (view === 'kconns' || (k && !kOk)) { // nothing to filter or list here
    ['nsdd', 'statusdd', 'ownerdd', 'search'].forEach((id) => ($(id).hidden = true));
    if (k) $(view + 'View').hidden = true;
  }
}
$('knoticego').onclick = () => showView('kconns');

// the sidebar: each cluster is a parent with an on/off switch; switched on, it shows all its pages as children
function renderKNav() {
  $('knav').innerHTML = clusters.map((c) => {
    const st = kstate[c.name], on = st.status === 'connected', busy = st.status === 'connecting';
    const closed = navClosed('n:k8s:' + c.name);
    const head = `<div class="knh ${on ? 'on' : 'idle'}">
      <button type="button" class="kname" data-kh="${esc(c.name)}" title="${esc(c.context)}"><span class="chev" data-nodetoggle="n:k8s:${esc(c.name)}" data-r="k8s">${on && !closed ? '▾' : '▸'}</span><span class="ico">${esc(c.name.slice(0, 1).toUpperCase())}</span><span class="lbl">${esc(c.name)}</span></button>
      <button type="button" class="sw ${st.status}" role="switch" aria-checked="${on}" data-toggle="${esc(c.name)}" ${busy ? 'disabled' : ''} title="${on ? 'Connected: switch off to disconnect' : st.status === 'failed' ? 'Failed: ' + esc(st.error) : 'Switch on to connect'}"><i></i></button>
    </div>`;
    const kids = on ? K_PAGES.map(([v, label, letter]) => `<button type="button" class="kc" data-view="${v}" data-kc="${esc(c.name)}" title="${label}"><span class="ico">${letter}</span><span class="lbl">${label}</span> <span class="n" data-badge="${v}">${kbadges[c.name]?.[v] ?? ''}</span></button>`).join('') : '';
    return `<div class="kn ${on ? 'open' : ''}" data-kc="${esc(c.name)}">${head}${on ? `<div class="kids ${closed ? 'closed' : ''}">${kids}</div>` : ''}</div>`;
  }).join('');
  markNav();
}
function markNav() {
  document.querySelectorAll('nav .gn').forEach((b) => b.classList.toggle('on', b.dataset.gopen === view));
  document.querySelectorAll('nav button[data-view]').forEach((b) => {
    let on = b.dataset.view === view;
    if (b.dataset.kc) on = on && b.dataset.kc === cluster;
    if (b.dataset.ds) { const d = ds[b.dataset.ds]; on = on && b.dataset.dn === d.active && b.dataset.tab === d.tab; }
    b.classList.toggle('on', on);
  });
}

// the lists on screen belong to the active connection: park them, bring the other connection's back
function stashActive() {
  if (cluster) kstore[cluster] = { data, pods, selected, statuses: statusSel.selected, owners: ownerSel.selected };
}
function restoreActive(name) {
  const s = kstore[name];
  data = s?.data ?? { namespaces: [], services: [], deployments: [], jobs: [] };
  pods = s?.pods ?? [];
  selected = s?.selected ?? [];
  nsSel.set(selected);
  statusSel.set(s?.statuses ?? []);
  ownerSel.set(s?.owners ?? []);
  nsSel.options(data.namespaces.map((n) => n.name));
}
async function setCluster(name) { // make a connected cluster the active one
  if (name === cluster) return;
  stashActive();
  cluster = name;
  restoreActive(name);
  syncKubernetes();
  ['namespaces', 'services', 'deployments', 'jobs', 'pods'].forEach(render);
  if (!restored[name]) { // first time: load, then reopen this connection's shells and log tabs
    restored[name] = true;
    loadNamespaces().catch((e) => toast(e.message));
    loadAll().then(() => { restoreShells(); restoreLogTabs(); });
  }
}

// ---- the Connections page
function renderKconns() {
  const cell = (c) => {
    const st = kstate[c.name];
    if (st.status === 'connected') return `${pill('connected', 'ok')} <span class="muted">Kubernetes ${esc(st.version || '')} · ${st.ms} ms</span>`;
    if (st.status === 'connecting') return '<span class="muted">connecting…</span>';
    if (st.status === 'failed') return `${pill('failed', 'bad')} <span class="muted" title="${esc(st.error)}">${esc((st.error || '').slice(0, 90))}</span>`;
    return `${pill('not connected')} <span class="muted">press Connect or switch it on</span>`;
  };
  $('kconnrows').innerHTML = clusters.length ? clusters.map((c) => {
    const st = kstate[c.name];
    const acts = st.status === 'connected'
      ? '<button type="button" class="sm" data-a="namespaces">Namespaces</button><button type="button" class="sm" data-a="pods">Pods</button><button type="button" class="sm" data-a="disconnect">Disconnect</button>'
      : `<button type="button" class="sm" data-a="connect" ${st.status === 'connecting' ? 'disabled' : ''}>Connect</button>`;
    return `<tr data-n="${esc(c.name)}"><td class="name">${esc(c.name)}</td><td style="word-break:break-all">${esc(c.context)}</td><td>${cell(c)}</td><td class="actions">${acts}</td></tr>`;
  }).join('') : '<tr><td colspan="4" class="muted">no Kubernetes cluster in the config: add a "kubernetes" list</td></tr>';
}
async function connectK8s(name) { // the only place that talks to a cluster before you use it
  const st = kstate[name];
  if (st.status === 'connecting') return false;
  Object.assign(st, { status: 'connecting', error: '' });
  renderKconns(); renderKNav();
  const t0 = performance.now();
  try {
    const r = await api(kq('/api/kubernetes/ping', name));
    Object.assign(st, { status: 'connected', version: r.version, ms: Math.round(performance.now() - t0), error: '' });
  } catch (e) {
    Object.assign(st, { status: 'failed', error: e.message });
  }
  syncKubernetes(); renderKconns(); renderKNav();
  return st.status === 'connected';
}
function disconnectK8s(name) { // drops that connection's lists; its shells keep running until you close the tab
  Object.assign(kstate[name], { status: 'idle', error: '' });
  delete kstore[name]; delete kbadges[name]; delete restored[name];
  if (cluster === name) {
    cluster = ''; // the lists went with the connection
    const next = clusters.find((c) => kstate[c.name].status === 'connected');
    if (next) setCluster(next.name); else restoreActive('');
  }
  syncKubernetes(); renderKNav(); renderKconns();
  if (K8S_VIEWS.includes(view)) showView(kOk ? view : 'kconns');
}
$('kconnrows').addEventListener('click', async (e) => {
  const b = e.target.closest('button[data-a]');
  if (!b) return;
  const name = b.closest('tr').dataset.n;
  if (b.dataset.a === 'disconnect') return disconnectK8s(name);
  if (b.dataset.a === 'connect') {
    if (await connectK8s(name) && !kOk) await setCluster(name); // the first connection becomes the active one
    return;
  }
  await setCluster(name); // Namespaces / Pods of a connected cluster
  showView(b.dataset.a);
});

// ---- views and theme
const VIEWS = {
  kconns: ['Kubernetes clusters', () => 'The clusters from the config. Nothing connects until you switch one on.'],
  namespaces: ['Namespaces', () => 'The namespaces of the cluster. Open one to work in it.'],
  services: ['Services', () => `Services in ${nsLabel()}.`],
  deployments: ['Deployments', () => `Deployments in ${nsLabel()}.`],
  jobs: ['Jobs', () => `Jobs in ${nsLabel()}.`],
  pods: ['Pods', () => `Pods in ${nsLabel()}: logs, a shell, restart and port forward.`],
  esconns: ['Elasticsearch clusters', () => 'The clusters from the config. Nothing connects until you switch one on.'],
  es: ['Elasticsearch', () => 'Documents (search, view, delete), indices and read-only queries.'],
  rconns: ['Redis servers', () => 'The servers from the config. Nothing connects until you switch one on.'],
  redis: ['Redis', () => 'Keys (search, view, delete) and a command console.'],
  forwards: ['Port forwards', () => 'Active forwards to 127.0.0.1. They stop when the app exits.'],
};
function showView(v) {
  view = v;
  markNav();
  Object.keys(VIEWS).forEach((k) => ($(k + 'View').hidden = k !== v));
  const owner = K8S_VIEWS.includes(v) ? cluster : v === 'redis' ? ds.redis.active : v === 'es' ? ds.es.active : ''; // a connection's page is headed with its name
  $('secTitle').textContent = owner ? `${owner} › ${VIEWS[v][0]}` : VIEWS[v][0];
  $('secDesc').textContent = VIEWS[v][1]();
  $('nsdd').hidden = v === 'namespaces' || v === 'redis' || v === 'es' || v === 'kconns' || v === 'rconns' || v === 'esconns';
  $('search').placeholder = v === 'redis' ? 'Search keys: text, or a pattern like user:*' : 'Search';
  $('statusdd').hidden = $('ownerdd').hidden = v !== 'pods';
  $('search').value = '';
  if (v === 'redis' && !dsOk('redis')) return showView('rconns'); // no connected server: the list
  if (v === 'es' && !dsOk('es')) return showView('esconns');
  syncRedisBar();
  syncK8sBar();
  if (v === 'kconns') return renderKconns();
  if (v === 'rconns') return renderDsConns('redis');
  if (v === 'esconns') return renderDsConns('es');
  if (K8S_VIEWS.includes(v) && !kOk) return;
  if (v === 'forwards') loadForwards(); else if (v === 'redis') showRedis(); else if (v === 'es') showEs(); else { render(v); load(v); }
}
async function toggleK8s(name) { // the switch in the sidebar: on = connect, off = disconnect
  if (kstate[name].status === 'connected') return disconnectK8s(name);
  if (await connectK8s(name) && !kOk) await setCluster(name); // the first one connected becomes the active one
}
// ---- collapsing: whole groups (Kubernetes, Redis, Elasticsearch) and each connection; remembered
let navState = {};
try { navState = JSON.parse(localStorage.getItem('k8ui.nav')) || {}; } catch {}
const navClosed = (key) => !!navState[key];
function setNavClosed(key, closed) {
  navState[key] = closed;
  try { localStorage.setItem('k8ui.nav', JSON.stringify(navState)); } catch {}
}
function applyGroups() {
  document.querySelectorAll('nav .gbody').forEach((b) => b.classList.toggle('closed', navClosed('g:' + b.dataset.gbody)));
  document.querySelectorAll('nav .gt').forEach((b) => (b.textContent = navClosed('g:' + b.dataset.gtoggle) ? '▸' : '▾'));
}
document.querySelector('nav').addEventListener('click', async (e) => {
  const gt = e.target.closest('[data-gtoggle]');
  if (gt) { setNavClosed('g:' + gt.dataset.gtoggle, !navClosed('g:' + gt.dataset.gtoggle)); return applyGroups(); }
  const go = e.target.closest('[data-gopen]');
  if (go) return showView(go.dataset.gopen); // a group's name opens its overview
  const nt = e.target.closest('[data-nodetoggle]');
  if (nt) { // the arrow of a connected connection folds its pages
    const key = nt.dataset.nodetoggle, name = key.split(':').slice(2).join(':'), kind = key.split(':')[1];
    const connected = kind === 'k8s' ? kstate[name]?.status === 'connected' : ds[kind].state[name]?.status === 'connected';
    if (connected) { setNavClosed(key, !navClosed(key)); kind === 'k8s' ? renderKNav() : renderDsNav(kind); }
    return; // an idle connection has nothing to fold
  }
  const sw = e.target.closest('button[data-toggle]');
  if (sw) return toggleK8s(sw.dataset.toggle);
  const dsw = e.target.closest('button[data-dstoggle]');
  if (dsw) return toggleDs(dsw.dataset.dstoggle, dsw.dataset.dn);
  const dh = e.target.closest('button[data-dh]');
  if (dh) { // a server's / cluster's name: its first page when connected, else the list to connect
    const d = ds[dh.dataset.dh];
    return d.state[dh.dataset.dn].status === 'connected' ? openDs(dh.dataset.dh, dh.dataset.dn, d.pages[0][0]) : showView(d.list);
  }
  const dc = e.target.closest('button[data-ds]');
  if (dc) return openDs(dc.dataset.ds, dc.dataset.dn, dc.dataset.tab); // a page under a connection
  const h = e.target.closest('button[data-kh]');
  if (h) { // a cluster's name: its pods when connected, else the Clusters page
    if (kstate[h.dataset.kh].status !== 'connected') return showView('kconns');
    await setCluster(h.dataset.kh);
    return showView('pods');
  }
  const b = e.target.closest('button[data-view]');
  if (!b) return;
  if (b.dataset.kc) await setCluster(b.dataset.kc); // a page under a connection: that connection becomes the active one
  showView(b.dataset.view);
});

function applyTheme(t) {
  if (t === 'light' || t === 'dark') document.documentElement.dataset.theme = t;
  else delete document.documentElement.dataset.theme;
}
try { $('theme').value = localStorage.getItem('theme') || 'auto'; } catch {}
applyTheme($('theme').value);
$('theme').onchange = () => {
  applyTheme($('theme').value);
  try { localStorage.setItem('theme', $('theme').value); } catch {}
};
