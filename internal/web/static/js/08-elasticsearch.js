// Elasticsearch / OpenSearch: connections, documents, indices, query.
// (Plain scripts share one global scope: index.html loads them in numeric order.)

// ---- Elasticsearch / OpenSearch (read-only): connection list, then Documents, Indices and Query per connection
const es = { tab: 'docs', hits: [], total: 0, relation: 'eq', gen: 0, indices: [], err: '', sel: new Set(), loaded: false }; // state of the ACTIVE cluster
const esid = (h) => `${h.index}\u0001${h.id}`;
const essrv = () => ds.es.active; // the active Elasticsearch cluster
const esSel = multiSelect($('esidxdd'), 'All indices', 'indices', () => { if (es.tab === 'docs') loadEsDocs(true); });
esSel.extraUnit = 'docs';
const esIndexParam = () => esSel.selected.join(',');
const fmtBytes = (n) => (n < 1024 ? `${n} B` : n < 1048576 ? `${(n / 1024).toFixed(1)} KB` : n < 1073741824 ? `${(n / 1048576).toFixed(1)} MB` : `${(n / 1073741824).toFixed(2)} GB`);
const esUrl = (path, q = {}) => `/api/es/${encodeURIComponent(essrv())}/${path}?${new URLSearchParams(q)}`;

// Each cluster keeps its own documents, index choice, search text and indices: parked when you move to another.
ds.es.save = () => ({ hits: es.hits, total: es.total, relation: es.relation, took: es.took, indices: es.indices, err: es.err, sel: es.sel, loaded: es.loaded,
  idx: esSel.selected, vals: esSel.vals, extra: esSel.extra, q: $('esq').value, f: $('esidxfilter').value });
ds.es.load = (s) => {
  es.gen++; // answers still on their way for the previous cluster are dropped
  Object.assign(es, { hits: s?.hits ?? [], total: s?.total ?? 0, relation: s?.relation ?? 'eq', took: s?.took, indices: s?.indices ?? [], err: s?.err ?? '', sel: s?.sel ?? new Set(), loaded: s?.loaded ?? false });
  esSel.options(s?.vals ?? [], s?.extra ?? {});
  esSel.set(s?.idx ?? []);
  $('esq').value = s?.q ?? ''; $('esidxfilter').value = s?.f ?? '';
};
function syncEsBar() {
  const open = view === 'es';
  $('eshead').hidden = !open;
  $('esidxdd').hidden = !open || es.tab === 'query'; // the query tab names its own path
}
// the Elasticsearch page of the active cluster: Documents, Indices or Query (the sidebar picks which)
async function showEs() {
  const name = essrv(), c = ds.es.conns.find((x) => x.name === name);
  $('esname').textContent = name; $('esaddr').textContent = c ? c.addr : '';
  if (!es.loaded) { es.loaded = true; await loadEsIndices(); }
  setEsTab(ds.es.tab);
}
function setEsTab(t) {
  es.tab = t;
  ds.es.tab = t;
  markNav();
  document.querySelectorAll('#esdetail .rtabs button').forEach((b) => b.classList.toggle('on', b.dataset.t === t));
  $('esdocs').hidden = t !== 'docs'; $('esidx').hidden = t !== 'indices'; $('esquery').hidden = t !== 'query';
  syncEsBar();
  if (t === 'docs') loadEsDocs(true);
  if (t === 'indices') renderEsIndices();
}
document.querySelector('#esdetail .rtabs').addEventListener('click', (e) => { const b = e.target.closest('button[data-t]'); if (b) setEsTab(b.dataset.t); });

async function loadEsIndices() {
  const name = essrv();
  try {
    const list = await api(esUrl('indices'));
    if (name !== essrv()) return; // another cluster became active meanwhile
    es.indices = list;
    esSel.options(es.indices.map((i) => i.index), Object.fromEntries(es.indices.map((i) => [i.index, i.docs])));
  } catch (e) { es.indices = []; toast(e.message); }
}
function renderEsIndices() {
  const f = $('esidxfilter').value.trim().toLowerCase();
  const rows = es.indices.filter((i) => !f || i.index.toLowerCase().includes(f));
  $('esidxrows').innerHTML = rows.length ? page('esidx', rows, f).map((i) => `<tr><td class="name" style="word-break:break-all">${esc(i.index)}</td>
      <td>${pill(i.health || '—', i.health === 'green' ? 'ok' : i.health === 'red' ? 'bad' : '')}</td><td>${esc(i.status)}</td>
      <td class="num">${i.docs.toLocaleString()}</td><td class="num">${fmtBytes(i.bytes)}</td><td>${esc(i.pri)}p / ${esc(i.rep)}r</td>
      <td class="actions"><button type="button" class="sm" data-a="docs" data-i="${esc(i.index)}">Documents</button><button type="button" class="sm" data-a="map" data-i="${esc(i.index)}">Mapping</button></td></tr>`).join('') + moreRow('esidx', rows.length, 7)
    : `<tr><td colspan="7" class="muted">${es.indices.length ? 'no index matches' : 'no indices'}</td></tr>`;
}
$('esidxfilter').oninput = renderEsIndices;
$('esidxrows').addEventListener('click', async (e) => {
  const b = e.target.closest('button[data-a]');
  if (!b) return;
  if (b.dataset.a === 'docs') { esSel.set([b.dataset.i]); return setEsTab('docs'); }
  try { showEsDialog(`${b.dataset.i} mapping`, await api(esUrl('mapping', { index: b.dataset.i }))); } catch (err) { toast(err.message); }
});

// documents: Lucene query_string search, 50 per page, "Load more" pages on
let esTimer;
$('esq').oninput = () => { clearTimeout(esTimer); esTimer = setTimeout(() => loadEsDocs(true), 400); };
const esPreview = (src) => {
  const parts = [];
  for (const [k, v] of Object.entries(src || {})) {
    parts.push(`${k}: ${typeof v === 'object' && v !== null ? JSON.stringify(v) : v}`);
    if (parts.join('  ').length > 220) break;
  }
  const t = parts.join('   ');
  return t.length > 260 ? t.slice(0, 260) + '…' : t;
};
async function loadEsDocs(reset) {
  const gen = reset ? ++es.gen : es.gen;
  if (reset) { es.hits = []; es.total = 0; es.err = ''; es.sel.clear(); renderEsDocs(); }
  try {
    const r = await api(esUrl('search', { index: esIndexParam(), q: $('esq').value.trim(), from: es.hits.length, size: 50 }));
    if (gen !== es.gen) return;
    es.hits = es.hits.concat(r.hits); es.total = r.total; es.relation = r.relation; es.took = r.took;
  } catch (e) { if (gen === es.gen) es.err = e.message; }
  if (gen === es.gen) renderEsDocs();
}
function renderEsDocs() {
  const ro = settings.read_only;
  $('esrows').innerHTML = es.hits.length ? es.hits.map((h, i) => `<tr><td class="cb">${ro ? '' : `<input type="checkbox" data-c="${i}" ${es.sel.has(esid(h)) ? 'checked' : ''}>`}</td>
      <td class="name" style="word-break:break-all">${esc(h.id)}</td><td style="word-break:break-all">${esc(h.index)}</td>
      <td class="esprev">${esc(esPreview(h.source))}</td><td class="num">${h.score == null ? '—' : h.score.toFixed(2)}</td>
      <td class="actions"><button type="button" class="sm" data-a="view" data-i="${i}">View</button>${ro ? '' : `<button type="button" class="sm danger" data-a="del" data-i="${i}">Delete</button>`}</td></tr>`).join('')
    : `<tr><td colspan="6" class="muted">${es.err ? esc(es.err) : 'no documents match'}</td></tr>`;
  const more = es.hits.length < es.total && es.hits.length < 10000;
  $('esmore').hidden = !more;
  $('esall').hidden = ro;
  $('esall').checked = es.hits.length > 0 && es.sel.size === es.hits.length;
  $('esdelsel').hidden = ro || !es.sel.size;
  $('esdelsel').textContent = `Delete selected (${es.sel.size})`;
  $('esnote').textContent = es.total || es.hits.length ? `${es.relation === 'gte' ? '≥ ' : ''}${es.total.toLocaleString()} hits${es.took != null ? ` in ${es.took} ms` : ''} · showing ${es.hits.length}${es.hits.length >= 10000 && es.total > 10000 ? ' (Elasticsearch shows the first 10,000: narrow the search)' : ''}` : '';
}
$('esmore').onclick = () => loadEsDocs(false);
$('esall').onchange = () => { es.sel = $('esall').checked ? new Set(es.hits.map(esid)) : new Set(); renderEsDocs(); };
$('esrows').addEventListener('change', (e) => {
  const c = e.target.closest('input[data-c]');
  if (!c) return;
  const id = esid(es.hits[c.dataset.c]);
  c.checked ? es.sel.add(id) : es.sel.delete(id);
  renderEsDocs();
});
$('esrows').addEventListener('click', async (e) => {
  const b = e.target.closest('button[data-i]');
  if (!b) return;
  const h = es.hits[b.dataset.i];
  if (b.dataset.a === 'del') return deleteEsDocs([h]);
  try { showEsDialog(`${h.index} / ${h.id}`, await api(esUrl('doc', { index: h.index, id: h.id })), h); } catch (err) { toast(err.message); }
});
$('esdelsel').onclick = () => deleteEsDocs(es.hits.filter((h) => es.sel.has(esid(h))));

// delete documents by index and id (the server never deletes by pattern, and refuses it under read_only)
async function deleteEsDocs(docs) {
  if (!docs.length) return false;
  const idx = [...new Set(docs.map((d) => d.index))].join(', ');
  if (!confirm(docs.length === 1 ? `Delete document ${docs[0].id} from ${idx}?` : `Delete ${docs.length} documents (${idx}) from ${essrv()}?\n\nThis cannot be undone.`)) return false;
  try {
    const r = await api(`/api/es/${encodeURIComponent(essrv())}/delete`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ items: docs.map((d) => ({ index: d.index, id: d.id })) }),
    });
    const gone = new Set(docs.filter((d) => !r.failed.some((f) => f.index === d.index && f.id === d.id)).map(esid));
    es.hits = es.hits.filter((h) => !gone.has(esid(h)));
    es.total = Math.max(0, es.total - gone.size);
    gone.forEach((id) => es.sel.delete(id));
    renderEsDocs();
    loadEsIndices().then(() => {}); // the doc counts changed
    toast(`deleted ${r.deleted}${r.not_found ? `, ${r.not_found} were already gone` : ''}${r.failed.length ? `, ${r.failed.length} failed: ${r.failed[0].error}` : ''}`);
    return !r.failed.length;
  } catch (err) { toast(err.message); return false; }
}
function showEsDialog(title, obj, doc) {
  const text = JSON.stringify(obj, null, 2);
  $('estitle').textContent = title;
  $('esbody').innerHTML = `<pre>${esc(text)}</pre>`;
  $('escopy').onclick = () => navigator.clipboard.writeText(text).then(() => toast('copied'), () => toast('copy failed'));
  $('esdel').hidden = !doc || settings.read_only; // only a document can be deleted from here, not a mapping
  $('esdel').onclick = async () => { if (await deleteEsDocs([doc])) $('esdlg').close(); };
  $('esdlg').showModal();
}
$('esclose').onclick = () => $('esdlg').close();

// query: read-only REST calls (the server refuses writes)
async function runEsQuery() {
  const out = $('esout');
  out.textContent = 'running…';
  try {
    const r = await api(`/api/es/${encodeURIComponent(essrv())}/query`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ method: $('esqm').value, path: $('esqp').value.trim(), body: $('esqb').value }),
    });
    let t = r.body;
    try { t = JSON.stringify(JSON.parse(t), null, 2); } catch {}
    out.textContent = `HTTP ${r.status}\n\n${t}`;
    toast(`${essrv()}: ${$('esqm').value} ${$('esqp').value.trim()} → HTTP ${r.status}`);
  } catch (e) { out.textContent = `refused: ${e.message}`; }
}
$('esrun').onclick = runEsQuery;
$('esqb').addEventListener('keydown', (e) => { if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); runEsQuery(); } });
$('esqp').addEventListener('keydown', (e) => { if (e.key === 'Enter') runEsQuery(); });
