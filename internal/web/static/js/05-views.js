// The Kubernetes connection picker, the page switcher and the theme.
// (Plain scripts share one global scope: index.html loads them in numeric order.)

// ---- the Kubernetes connection: state, picker, and the "not connected" notice
function syncKubernetes() {
  const ci = clusterInfo();
  kOk = !!ci && !ci.error;
  kWhy = !clusters.length ? 'Add a "kubernetes" entry to the config to connect a cluster.' : ci?.error ? `${ci.name}: ${ci.error}` : '';
  $('kpill').textContent = kOk ? (clusters.length > 1 ? `${clusters.length} connections` : 'connected') : 'not connected';
  $('kpill').className = 'pill ' + (kOk ? 'ok' : 'bad');
  $('ctx').textContent = ci ? ci.name : 'not connected';
  $('ctx').title = ci ? ci.context : '';
  $('prod').hidden = !(ci && /prod/i.test(ci.name + ci.context));
  $('kcluster').innerHTML = clusters.map((c) => `<option value="${esc(c.name)}" title="${esc(c.context)}">${esc(c.name)}${c.error ? ' (not connected)' : ''}</option>`).join('');
  $('kcluster').value = cluster;
}
function syncK8sBar() {
  const k = K8S_VIEWS.includes(view);
  $('kcluster').hidden = !(k && clusters.length);
  $('knotice').hidden = !(k && !kOk);
  $('knoticetext').textContent = kWhy;
  if (k && !kOk) { // nothing to filter or list: hide the controls and the table
    ['nsdd', 'statusdd', 'ownerdd', 'search'].forEach((id) => ($(id).hidden = true));
    $(view + 'View').hidden = true;
  }
}
async function setCluster(name) {
  cluster = name;
  try { localStorage.setItem('k8ui.cluster', name); } catch {}
  selected = []; nsSel.set([]);
  data.namespaces = []; data.services = []; data.deployments = []; data.jobs = []; pods = [];
  syncKubernetes();
  ['namespaces', 'services', 'deployments', 'jobs', 'pods'].forEach(render);
  showView(view); // loads the open page for this connection, or shows the notice
  if (kOk) { loadNamespaces().catch((e) => toast(e.message)); loadAll(); }
}
$('kcluster').onchange = () => setCluster($('kcluster').value);

// ---- views and theme
const VIEWS = {
  namespaces: ['Namespaces', () => `The namespaces of ${cluster || 'the cluster'}. Open one to work in it.`],
  services: ['Services', () => `Services in ${nsLabel()}.`],
  deployments: ['Deployments', () => `Deployments in ${nsLabel()}.`],
  jobs: ['Jobs', () => `Jobs in ${nsLabel()}.`],
  pods: ['Pods', () => `Pods in ${nsLabel()}: logs, a shell, restart and port forward.`],
  es: ['Elasticsearch', () => 'Elasticsearch / OpenSearch clusters from the config (read-only): documents, indices and read-only queries.'],
  redis: ['Redis', () => 'Redis connections from the config. Open one for its keys and a command console.'],
  forwards: ['Port forwards', () => 'Active forwards to 127.0.0.1. They stop when the app exits.'],
};
function showView(v) {
  view = v;
  document.querySelectorAll('nav button').forEach((b) => b.classList.toggle('on', b.dataset.view === v));
  Object.keys(VIEWS).forEach((k) => ($(k + 'View').hidden = k !== v));
  $('secTitle').textContent = VIEWS[v][0];
  $('secDesc').textContent = VIEWS[v][1]();
  $('nsdd').hidden = v === 'namespaces' || v === 'redis' || v === 'es';
  $('search').placeholder = v === 'redis' ? 'Search keys: text, or a pattern like user:*' : 'Search';
  $('statusdd').hidden = $('ownerdd').hidden = v !== 'pods';
  $('search').value = '';
  syncRedisBar();
  syncK8sBar();
  if (K8S_VIEWS.includes(v) && !kOk) return;
  if (v === 'forwards') loadForwards(); else if (v === 'redis') showRedis(); else if (v === 'es') showEs(); else { render(v); load(v); }
}
document.querySelector('nav').addEventListener('click', (e) => {
  const b = e.target.closest('button');
  if (b) showView(b.dataset.view);
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
