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
    let saved = '';
    try { saved = localStorage.getItem('k8ui.cluster') || ''; } catch {}
    cluster = clusters.some((c) => c.name === saved) ? saved : clusters[0]?.name ?? '';
    syncKubernetes();
    showView('namespaces'); // loads it when Kubernetes is connected, else says it is not
    if (kOk) loadAll().then(() => { restoreShells(); restoreLogTabs(); });
    initRedis();
    initEs();
    loadForwards();
    // started after the config is known: refresh_seconds is the poll interval
    setInterval(() => { if (!document.hidden) { if (view !== 'forwards' && !busy[view]) load(view); loadForwards(); } }, settings.refresh_seconds * 1000);
  } catch (e) { toast(e.message); }
})();
