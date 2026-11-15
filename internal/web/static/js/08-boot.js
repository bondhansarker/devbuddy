// Sidebar collapse and start-up.
// (Plain scripts share one global scope: index.html loads them in numeric order.)

// ---- sidebar: expanded or collapsed to the icons, remembered
function setSide(collapsed) {
  document.body.classList.toggle('collapsed', collapsed);
  $('sidetoggle').textContent = collapsed ? '»' : '«';
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
    syncKubernetes();
    renderKNav();
    showView('kconns'); // the connection list: you connect, nothing does it for you
    initRedis();
    initEs();
    loadForwards();
    // started after the config is known: refresh_seconds is the poll interval
    setInterval(() => { if (!document.hidden) { if (view !== 'forwards' && !busy[view]) load(view); loadForwards(); } }, settings.refresh_seconds * 1000);
  } catch (e) { toast(e.message); }
})();
