// Redis and Elasticsearch connections, shared: the sidebar tree (a parent per connection with an on/off switch,
// its pages as children), the Servers / Clusters list pages and the connect state.
// Nothing connects until you switch a connection on or press Connect; each connection keeps its own state.
// (Plain scripts share one global scope: index.html loads them in numeric order.)

const ds = {
  redis: {
    label: 'Redis', one: 'server', conns: [], state: {}, active: '', tab: 'keys', stash: {},
    pages: [['keys', 'Keys'], ['console', 'Console']],
    list: 'rconns', detail: 'redis', sec: 'rsec', nav: 'rnav', pill: 'rpill', rows: 'rconnrows',
    ping: (n) => api(`/api/redis/${encodeURIComponent(n)}/dbs`),
    cell: (st) => `${pill('connected', 'ok')} <span class="muted">${st.info.dbs.length ? st.info.dbs.reduce((t, d) => t + d.keys, 0) + ' keys · ' : ''}${st.ms} ms</span>`,
  },
  es: {
    label: 'Elasticsearch', one: 'cluster', conns: [], state: {}, active: '', tab: 'docs', stash: {},
    pages: [['docs', 'Documents'], ['indices', 'Indices'], ['query', 'Query']],
    list: 'esconns', detail: 'es', sec: 'esec', nav: 'enav', pill: 'epill', rows: 'esconnrows',
    ping: (n) => api(`/api/es/${encodeURIComponent(n)}/info`),
    cell: (st) => `${st.info.health ? pill(st.info.health, st.info.health === 'green' ? 'ok' : st.info.health === 'red' ? 'bad' : '') : pill('connected', 'ok')} <span class="muted">${esc(st.info.flavor)} ${esc(st.info.version)} · cluster ${esc(st.info.cluster)} · ${st.ms} ms</span>`,
  },
};
const dsConnected = (k) => ds[k].conns.filter((c) => ds[k].state[c.name].status === 'connected');
const dsOk = (k) => !!ds[k].active && ds[k].state[ds[k].active]?.status === 'connected';

function initDs(kind, list) { // at boot: only the config is read, no server is contacted
  const d = ds[kind];
  d.conns = list;
  list.forEach((c) => (d.state[c.name] = { status: 'idle', error: '' }));
  $(d.sec).hidden = !list.length;
  syncDsPill(kind);
  renderDsNav(kind);
}
function syncDsPill(kind) {
  const d = ds[kind], n = dsConnected(kind).length;
  $(d.pill).textContent = ''; $(d.pill).title = n ? `${n} connected` : 'not connected'; // shown as a coloured dot
  $(d.pill).className = 'pill ' + (n ? 'ok' : '');
}

// the sidebar: each connection is a parent with a switch; switched on, all its pages are its children
function renderDsNav(kind) {
  const d = ds[kind];
  $(d.nav).innerHTML = d.conns.map((c) => {
    const st = d.state[c.name], on = st.status === 'connected', busy = st.status === 'connecting';
    const closed = navClosed(`n:${kind}:${c.name}`);
    const head = `<div class="knh ${on ? 'on' : 'idle'}">
      <button type="button" class="kname" data-dh="${kind}" data-dn="${esc(c.name)}" title="${esc(c.addr)}"><span class="lbl">${esc(c.name)}</span></button>
      <button type="button" class="sw ${st.status}" role="switch" aria-checked="${on}" data-dstoggle="${kind}" data-dn="${esc(c.name)}" ${busy ? 'disabled' : ''} title="${on ? 'Connected: switch off to disconnect' : st.status === 'failed' ? 'Failed: ' + esc(st.error) : 'Switch on to connect'}"><i></i></button><span class="chev ${on && !closed ? 'up' : ''}" data-nodetoggle="n:${kind}:${esc(c.name)}">${ICONS.chev}</span>
    </div>`;
    const kids = on ? d.pages.map(([t, label]) => `<button type="button" class="kc" data-view="${d.detail}" data-ds="${kind}" data-dn="${esc(c.name)}" data-tab="${t}" title="${label}"><span class="ico">${ICONS[t]}</span><span class="lbl">${label}</span></button>`).join('') : '';
    return `<div class="kn ${on ? 'open' : ''}" data-dn="${esc(c.name)}">${head}${on ? `<div class="kids ${closed ? 'closed' : ''}">${kids}</div>` : ''}</div>`;
  }).join('');
  markNav();
}

// the Servers / Clusters list page
function renderDsConns(kind) {
  const d = ds[kind];
  const cell = (c) => {
    const st = d.state[c.name];
    if (st.status === 'connected') return d.cell(st);
    if (st.status === 'connecting') return '<span class="muted">connecting…</span>';
    if (st.status === 'failed') return `${pill('failed', 'bad')} <span class="muted" title="${esc(st.error)}">${esc((st.error || '').slice(0, 90))}</span>`;
    return `${pill('not connected')} <span class="muted">press Connect or switch it on</span>`;
  };
  $(d.rows).innerHTML = d.conns.map((c) => {
    const st = d.state[c.name];
    const acts = st.status === 'connected'
      ? `${d.pages.map(([t, label]) => ab(t, label, `data-a="${t}"`)).join('')}${ab('power', 'Disconnect', 'data-a="disconnect"')}`
      : ab('plug', 'Connect', `data-a="connect" ${st.status === 'connecting' ? 'disabled' : ''}`);
    return `<tr data-n="${esc(c.name)}"><td class="name">${esc(c.name)}</td><td style="word-break:break-all">${esc(c.addr)}${c.tls ? ' ' + pill('tls') : ''}</td><td>${cell(c)}</td><td class="actions">${acts}</td></tr>`;
  }).join('');
}
['redis', 'es'].forEach((kind) => $(ds[kind].rows).addEventListener('click', async (e) => {
  const b = e.target.closest('button[data-a]');
  if (!b) return;
  const name = b.closest('tr').dataset.n;
  if (b.dataset.a === 'disconnect') return disconnectDs(kind, name);
  if (b.dataset.a === 'connect') return toggleDs(kind, name);
  await openDs(kind, name, b.dataset.a); // one of its pages
}));

async function connectDs(kind, name) { // the only place that contacts a server before you use it
  const d = ds[kind], st = d.state[name];
  if (st.status === 'connecting') return false;
  Object.assign(st, { status: 'connecting', error: '' });
  renderDsConns(kind); renderDsNav(kind);
  const t0 = performance.now();
  try {
    st.info = await d.ping(name);
    Object.assign(st, { status: 'connected', ms: Math.round(performance.now() - t0), error: '' });
  } catch (e) {
    Object.assign(st, { status: 'failed', error: e.message });
  }
  syncDsPill(kind); renderDsConns(kind); renderDsNav(kind);
  return st.status === 'connected';
}
function disconnectDs(kind, name) { // drops that connection's loaded data
  const d = ds[kind];
  Object.assign(d.state[name], { status: 'idle', error: '' });
  delete d.stash[name];
  if (kind === 'redis' && rsh.has(name)) { rsh.get(name).el.remove(); rsh.delete(name); } // its console session ends
  if (d.active === name) {
    d.active = '';
    const next = dsConnected(kind)[0];
    if (next) activateDs(kind, next.name); else d.load(undefined);
  }
  syncDsPill(kind); renderDsNav(kind); renderDsConns(kind);
  if (view === d.detail && !dsOk(kind)) showView(d.list);
}
async function toggleDs(kind, name) { // the switch, or Connect: on = connect, off = disconnect
  const d = ds[kind];
  if (d.state[name].status === 'connected') return disconnectDs(kind, name);
  if (await connectDs(kind, name) && !d.active) activateDs(kind, name); // the first connected one becomes the active one
}
function activateDs(kind, name) { // park the active connection's data, bring this one's back
  const d = ds[kind];
  if (d.active === name) return;
  if (d.active) d.stash[d.active] = d.save();
  d.active = name;
  d.load(d.stash[name]);
}
async function openDs(kind, name, tab) { // one page of a connected server / cluster
  const d = ds[kind];
  if (d.state[name].status !== 'connected') return showView(d.list);
  activateDs(kind, name);
  d.tab = tab;
  showView(d.detail);
}
