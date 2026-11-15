// Sidebar collapse and start-up.
// (Plain scripts share one global scope: index.html loads them in numeric order.)

// ---- last session: which connections were on, the active ones, the page, the namespace filter and the Redis /
// Elasticsearch tab are remembered in this browser. After a refresh the connections that were on are switched on
// again (see resumeSession); a connection that was off stays off, and a first start connects nothing.
const SESSION_KEY = 'k8ui.session';
const prevSession = (() => { try { return JSON.parse(localStorage.getItem(SESSION_KEY)) || {}; } catch { return {}; } })();
const sess = JSON.parse(JSON.stringify(prevSession));
const savedNs = (name) => (Array.isArray(prevSession.k8s?.ns?.[name]) ? prevSession.k8s.ns[name] : []);
let sessionArmed = false; // set by the first connect / disconnect of this page load: until then the saved session is left alone
const armSession = () => { sessionArmed = true; };
function saveSession() {
  // "on" = connected or still connecting; one that failed (a cluster that is down) stays remembered, so the next refresh retries it
  const isOn = (st, was) => st === 'connected' || st === 'connecting' || (st === 'failed' && was);
  const on = (kind) => ds[kind].conns.filter((c) => isOn(ds[kind].state[c.name]?.status, (prevSession[kind]?.on || []).includes(c.name))).map((c) => c.name);
  const k8on = clusters.filter((c) => isOn(kstate[c.name]?.status, (prevSession.k8s?.on || []).includes(c.name))).map((c) => c.name);
  syncPower();
  if (!sessionArmed && !k8on.length && !on('redis').length && !on('es').length) return; // nothing happened yet: keep the last session as it was
  if (k8on.length || on('redis').length || on('es').length || !['kconns', 'rconns', 'esconns'].includes(view)) sess.view = view; // with nothing on, the last real page is kept
  sess.k8s = { on: k8on, active: cluster || sess.k8s?.active, ns: { ...sess.k8s?.ns, ...(cluster ? { [cluster]: selected } : {}) } };
  for (const k of ['redis', 'es']) sess[k] = { on: on(k), active: ds[k].active || sess[k]?.active, tab: ds[k].active ? ds[k].tab : sess[k]?.tab };
  try { localStorage.setItem(SESSION_KEY, JSON.stringify(sess)); } catch {}
}
// a connection just switched on that was the active one last time: bring its page and tab back (only from a
// connection list page, the landing pages; it never moves you off a page you opened yourself)
const onLanding = () => view === 'kconns' || view === 'rconns' || view === 'esconns';
function resumeK8s(name) {
  if (name === prevSession.k8s?.active && onLanding() && K8S_VIEWS.includes(prevSession.view)) showView(prevSession.view);
}
function resumeDs(kind, name) {
  const d = ds[kind], p = prevSession[kind];
  if (name !== p?.active) return;
  if (d.pages.some(([t]) => t === p.tab)) d.tab = p.tab;
  if (onLanding() && prevSession.view === d.detail) showView(d.detail);
}

// After a refresh: switch on the connections that were on, make the same ones active and open the page you were on.
async function resumeSession(p = prevSession) {
  const k8 = (p.k8s?.on || []).filter((n) => clusters.some((c) => c.name === n && !c.error));
  const dsOn = Object.fromEntries(['redis', 'es'].map((k) => [k, (p[k]?.on || []).filter((n) => ds[k].conns.some((c) => c.name === n))]));
  if (!k8.length && !dsOn.redis.length && !dsOn.es.length) return;
  armSession();
  await Promise.all([...k8.map((n) => connectK8s(n)), ...['redis', 'es'].flatMap((k) => dsOn[k].map((n) => connectDs(k, n)))]);
  const up = k8.filter((n) => kstate[n].status === 'connected');
  const kActive = up.includes(p.k8s?.active) ? p.k8s.active : up[0];
  if (kActive) await setCluster(kActive);
  for (const k of ['redis', 'es']) {
    const live = dsConnected(k).map((c) => c.name), a = live.includes(p[k]?.active) ? p[k].active : live[0];
    if (a) { activateDs(k, a); if (ds[k].pages.some(([t]) => t === p[k]?.tab)) ds[k].tab = p[k].tab; }
  }
  const v = p.view;
  if (v && VIEWS[v] && v !== 'kconns' && (!K8S_VIEWS.includes(v) || kOk) && (v !== 'redis' || dsOk('redis')) && (v !== 'es' || dsOk('es'))) showView(v);
  else saveSession();
}

// Housekeeping at start-up, once the config is known: forget what no longer exists.
function pruneState() {
  try {
    // the session: only connections that are still in the config
    const known = (list) => new Set(list.map((c) => c.name));
    const k = known(clusters);
    if (sess.k8s?.ns) sess.k8s.ns = Object.fromEntries(Object.entries(sess.k8s.ns).filter(([n]) => k.has(n)));
    if (sess.k8s?.active && !k.has(sess.k8s.active)) delete sess.k8s.active;
    for (const kind of ['redis', 'es']) if (sess[kind]?.active && !known(ds[kind].conns).has(sess[kind].active)) delete sess[kind].active;
    const was = localStorage.getItem(SESSION_KEY);
    if (was && was !== JSON.stringify(sess)) localStorage.setItem(SESSION_KEY, JSON.stringify(sess)); // only rewrite what changed
    // divider marks belong to an open log tab: keep only those whose tab is saved as open
    const open = new Set((JSON.parse(localStorage.getItem('k8ui.logtabs')) || []).map((x) => `k8ui.dividers.${x.kind || 'pods'}:${x.namespace}/${x.pod}`));
    Object.keys(localStorage).filter((key) => key.startsWith('k8ui.dividers.') && !open.has(key)).forEach((key) => localStorage.removeItem(key));
  } catch {}
}
// Reset session (in Settings): every connection off AND the saved session forgotten (which connections were on, the page, filters,
// open tabs, port numbers), then a reload. Theme, Auto refresh and layout are preferences and stay.
$('startfresh').onclick = async () => {
  if (!await askConfirm('Switch every connection off and forget what was on, the page, filters, open tabs and port numbers? Open shells end. Theme and Auto refresh stay. Nothing in your clusters changes.', { title: 'Reset session', ok: 'Reset session', danger: true })) return;
  [...tabs.keys()].forEach(closeTab); // ends the shells
  try { Object.keys(localStorage).filter((k) => k === SESSION_KEY || k === 'k8ui.logtabs' || k === 'k8ui.forwards' || k.startsWith('k8ui.dividers.')).forEach((k) => localStorage.removeItem(k)); } catch {}
  location.reload();
};

// ---- sidebar: expanded or collapsed to the icons, remembered
function setSide(collapsed) {
  document.body.classList.toggle('collapsed', collapsed);
  try { localStorage.setItem('k8ui.side', collapsed ? '1' : ''); } catch {}
}
try { document.body.classList.toggle('collapsed', !!localStorage.getItem('k8ui.side')); } catch {} // apply only: nothing is written until you toggle it
$('sidetoggle').onclick = () => setSide(!document.body.classList.contains('collapsed'));

// ---- boot
(async () => {
  try {
    const info = await api('/api/info');
    settings = info;
    clusters = info.clusters;
    clusters.forEach((c) => (kstate[c.name] = { status: c.error ? 'failed' : 'idle', error: c.error || '' })); // nothing is contacted yet
    applyGroups();
    syncKubernetes();
    renderKNav();
    showView('kconns'); // the connection list: you connect, nothing does it for you
    initDs('redis', await api('/api/redis').catch(() => [])); // only the config: no server is contacted
    initDs('es', await api('/api/es').catch(() => []));
    pruneState();
    // started after the config is known: refresh_seconds is the poll interval
    setInterval(() => {
      if (document.hidden || !poll.enabled) return;
      if (poll.wait > 0) { poll.wait--; return; } // backing off after failures
      if (view === 'forwards') { loadForwards(); return; } // DevBuddy's own list: no cluster call
      if (busy[view]) return;
      poll.tick++; poll.auto = true;
      const done = (ok) => { poll.auto = false; if (ok === false) { poll.fails++; poll.wait = Math.min(2 ** poll.fails, 12) - 1; } else if (ok) poll.fails = 0; };
      load(view).then(done, () => done(false));
    }, settings.refresh_seconds * 1000);
    resumeSession(); // switches the connections on that were on before the refresh
  } catch (e) { toast(e.message); }
})();

[['ldcopy', 'ldclose'], ['rdcopy', 'rdclose'], ['escopy', 'esclose']].forEach(([c, x]) => dlgIcons(c, x));
$('rdedit').innerHTML = ICONS.edit; $('rdedit').title = 'Edit the value'; $('rdedit').className = 'icon';
$('nscreate').innerHTML = `${ICONS.plus} Create namespace`;
// Auto refresh on / off, and a refresh-now button for when it is off
$('autorefresh').checked = poll.enabled;
$('autorefresh').onchange = () => {
  poll.enabled = $('autorefresh').checked; poll.fails = poll.wait = 0;
  try { localStorage.setItem('k8ui.autorefresh', poll.enabled ? '1' : '0'); } catch {}
  toast(poll.enabled ? 'auto refresh on' : 'auto refresh off: use the refresh button');
};
$('refreshnow').innerHTML = ICON_REFRESH;
$('refreshnow').onclick = async () => {
  const b = $('refreshnow');
  b.classList.add('spin');
  poll.fails = poll.wait = 0;
  await (view === 'forwards' ? loadForwards() : load(view)).catch(() => {});
  b.classList.remove('spin');
};

// Settings: theme, auto refresh, disconnect all and reset open in a panel beside the Settings button
const openSettings = () => { $('refreshrow').title = `Reload the open page every ${settings.refresh_seconds} s (refresh_seconds in the config)`; $('settingsdlg').hidden = false; $('settingsbtn').classList.add('on'); };
function closeSettings() { $('settingsdlg').hidden = true; $('settingsbtn').classList.remove('on'); }
$('refreshrow').title = `Reload the open page every ${settings.refresh_seconds} s (refresh_seconds in the config)`;
$('themeico').innerHTML = ICONS.contrast; $('refreshico').innerHTML = ICON_REFRESH;
document.querySelector('#themeseg [data-theme=light]').innerHTML = ICONS.sun; document.querySelector('#themeseg [data-theme=dark]').innerHTML = ICONS.moon;
$('settingsbtn').onclick = () => ($('settingsdlg').hidden ? openSettings() : closeSettings());
$('setx').innerHTML = ICONS.close; $('setx').title = 'Close'; $('setx').onclick = closeSettings;
document.addEventListener('mousedown', (e) => { if (!$('settingsdlg').hidden && !e.target.closest('#settingsdlg, #settingsbtn, #askdlg')) closeSettings(); }); // a click elsewhere closes it
document.addEventListener('keydown', (e) => { if (e.key === 'Escape' && !$('settingsdlg').hidden && !document.querySelector('dialog[open]')) closeSettings(); });
$('startfresh').innerHTML = `${ICONS.restart} Reset session`;
// The power icon beside Settings is one switch: connections on -> click switches them all off (and remembers which
// were on); everything off -> click switches those back on, with the same active connection, filters and page.
// Nothing else is touched (page, filters, tabs and shells stay).
const liveOn = () => ({
  k8s: clusters.filter((c) => kstate[c.name]?.status === 'connected').map((c) => c.name),
  redis: dsConnected('redis').map((c) => c.name),
  es: dsConnected('es').map((c) => c.name),
});
const countOf = (o) => o.k8s.length + o.redis.length + o.es.length;
function syncPower() {
  const b = $('discall'), on = countOf(liveOn()) > 0, r = sess.resume;
  const back = r ? [...(r.k8s || []), ...(r.redis || []), ...(r.es || [])] : [];
  b.classList.toggle('on', on);
  b.setAttribute('aria-pressed', on);
  b.title = on ? 'Connections are on: click to switch every one off (your page, filters and tabs stay)' : back.length ? `All off: click to switch back on ${back.join(', ')}` : 'No connection is on';
}
$('discall').onclick = () => {
  const live = liveOn();
  if (countOf(live)) {
    sess.resume = live;
    const keepView = view; // disconnecting moves you to a connection list; the page you were on is what comes back
    armSession();
    live.k8s.forEach(disconnectK8s);
    live.redis.forEach((n) => disconnectDs('redis', n));
    live.es.forEach((n) => disconnectDs('es', n));
    if (!['kconns', 'rconns', 'esconns'].includes(keepView)) sess.view = keepView;
    saveSession();
    toast('all connections switched off');
    return;
  }
  const r = sess.resume;
  if (!r || !countOf(r)) return toast('no connection is on: switch one on in the sidebar');
  resumeSession({ ...sess, k8s: { ...sess.k8s, on: r.k8s || [] }, redis: { ...sess.redis, on: r.redis || [] }, es: { ...sess.es, on: r.es || [] } })
    .then(() => toast('connections switched back on'));
};
syncPower();
