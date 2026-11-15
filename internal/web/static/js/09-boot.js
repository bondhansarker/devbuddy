// Sidebar collapse and start-up.
// (Plain scripts share one global scope: index.html loads them in numeric order.)

// ---- sidebar: expanded or collapsed to the icons, remembered
function setSide(collapsed) {
  document.body.classList.toggle('collapsed', collapsed);
  try { localStorage.setItem('k8ui.side', collapsed ? '1' : ''); } catch {}
}
try { setSide(!!localStorage.getItem('k8ui.side')); } catch {}
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
    loadForwards();
    // started after the config is known: refresh_seconds is the poll interval
    setInterval(() => {
      if (document.hidden || !poll.enabled) return;
      loadForwards(); // local: never reaches a cluster
      if (poll.wait > 0) { poll.wait--; return; } // backing off after failures
      if (view === 'forwards' || busy[view]) return;
      poll.tick++; poll.auto = true;
      const done = (ok) => { poll.auto = false; if (ok === false) { poll.fails++; poll.wait = Math.min(2 ** poll.fails, 12) - 1; } else if (ok) poll.fails = 0; };
      load(view).then(done, () => done(false));
    }, settings.refresh_seconds * 1000);
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
  await Promise.all([view === 'forwards' ? Promise.resolve() : load(view), loadForwards()]).catch(() => {});
  b.classList.remove('spin');
};
