// Port forwards.
// (Plain scripts share one global scope: index.html loads them in numeric order.)

// ---- port-forward
// The ports of the last forward are remembered per workload (the pod name without its replica hash).
const FWDKEY = 'k8ui.forwards';
const fwdId = (p) => `${p.namespace}/${p.name.replace(/-[a-z0-9]{8,10}-[a-z0-9]{5}$/, '')}`;
const fwdMemory = () => { try { return JSON.parse(localStorage.getItem(FWDKEY)) || {}; } catch { return {}; } };
// svc: the service the forward was started from; its port is mapped to the pod's container port.
async function forward(p, svc) {
  const id = svc ? `${svc.namespace}/svc:${svc.name}` : fwdId(p);
  const last = fwdMemory()[id] || {};
  const def = last.container_port ?? p.ports[0]?.container_port ?? '';
  const hint = p.ports.map((x) => `${x.container_port}${x.name ? ' (' + x.name + ')' : ''}`).join(', ') || 'none declared';
  const lpDef = (svc ? last.service_port != null : last.container_port === def) ? last.local_port ?? '' : '';
  const r = await ask({
    title: svc ? `Forward service ${svc.name}` : `Forward ${p.name}`,
    message: svc ? `Via pod ${p.name}.` : '',
    ok: 'Forward',
    fields: [
      svc ? { name: 'sp', label: 'Service port', type: 'number', required: true, value: last.service_port ?? parseInt(svc.ports[0], 10) ?? '', hint: `ports: ${svc.ports.join(', ') || 'none'}` }
        : { name: 'cp', label: 'Container port', type: 'number', required: true, value: def, hint: `declared: ${hint}` },
      { name: 'lp', label: 'Local port', type: 'number', value: lpDef, hint: 'blank = same as the remote port, or a free one if taken' },
    ],
  });
  if (!r) return;
  let cp, sp;
  if (svc) {
    sp = parseInt(r.sp, 10);
    const target = svc.targets[sp];
    if (!target) return toast(`${svc.name} has no port ${sp}`);
    cp = /^\d+$/.test(target) ? +target : p.ports.find((x) => x.name === target)?.container_port;
    if (!cp) return toast(`cannot resolve target port ${target} on ${p.name}`);
  } else cp = parseInt(r.cp, 10);
  const lp = parseInt(r.lp, 10) || 0;
  try {
    const f = await api('/api/forwards', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ cluster: p.cluster, namespace: p.namespace, pod: p.name, container_port: cp, local_port: lp }),
    });
    try { localStorage.setItem(FWDKEY, JSON.stringify({ ...fwdMemory(), [id]: { container_port: f.container_port, local_port: f.local_port, service_port: sp } })); } catch {}
    ktoast(p.namespace, `port-forward pod/${p.name} ${f.local_port}:${f.container_port}`, `Forwarding from 127.0.0.1:${f.local_port} -> ${f.container_port}`);
    loadForwards();
  } catch (e) { ktoast(p.namespace, `port-forward pod/${p.name} ${lp || cp}:${cp}`, e.message); }
}
let forwards = [];
async function loadForwards() {
  forwards = await api('/api/forwards').catch(() => []);
  renderForwards();
}
function renderForwards() {
  $('fwcount').textContent = forwards.length;
  const l = forwards.filter((f) => inNs(f.namespace) && match([f.namespace, f.pod, f.local_port, f.container_port, f.error]));
  $('fwrows').innerHTML = !l.length ? `<tr><td colspan="4" class="muted">${emptyText(forwards.length)}</td></tr>` : l.map((f) => `<tr>
    <td><a href="http://localhost:${f.local_port}" target="_blank" rel="noopener" style="color:var(--accent)">localhost:${f.local_port}</a></td>
    <td>→ ${clusters.length > 1 ? esc(f.cluster) + ' · ' : ''}${esc(f.namespace)}/${esc(f.pod)}:${f.container_port}</td>
    <td class="muted">${esc(f.error || '')}</td>
    <td class="actions">${ab('power', 'Disconnect', `data-id="${esc(f.id)}"`, 'danger', true)}</td></tr>`).join('');
}
$('fwrows').addEventListener('click', async (e) => {
  const b = e.target.closest('button');
  if (!b) return;
  const f = forwards.find((x) => x.id === b.dataset.id);
  await api(`/api/forwards/${b.dataset.id}`, { method: 'DELETE' }).then(() => f && toast(`# disconnected port-forward localhost:${f.local_port} → ${f.namespace}/${f.pod}:${f.container_port}`), (x) => toast(x.message));
  loadForwards();
});
