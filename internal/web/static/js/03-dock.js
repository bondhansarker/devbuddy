// The bottom dock: tabs, pod logs (SSE) and the pod shell (WebSocket + xterm.js).
// (Plain scripts share one global scope: index.html loads them in numeric order.)

// ---- dock: one tab per pod for logs and one for shell, kept alive while another tab is shown
const tabs = new Map(); // "logs:ns/pod" | "shell:ns/pod" -> { tab, pane, stop, fit }
let activeTab = null;

function activate(key) {
  activeTab = tabs.has(key) ? key : null;
  tabs.forEach((t, k) => { t.tab.classList.toggle('on', k === key); t.pane.hidden = k !== key; (k === key ? t.resume : t.pause)?.(); });
  $('dock').hidden = !tabs.size;
  document.body.classList.toggle('docked', tabs.size > 0);
  $('dockmax').hidden = !activeTab?.startsWith('shell'); // logs tabs carry their own full screen button in the toolbar
  tabs.get(key)?.fit?.();
}
function closeTab(key) {
  const t = tabs.get(key);
  if (!t) return;
  t.stop();
  t.kill?.();
  t.tab.remove();
  t.pane.remove();
  tabs.delete(key);
  saveLogTabs();
  activate([...tabs.keys()].pop());
}
$('tabs').addEventListener('click', (e) => {
  const tab = e.target.closest('.tab');
  if (!tab) return;
  if (e.target.classList.contains('x')) closeTab(tab.dataset.key); else { setMin(false); activate(tab.dataset.key); }
});

// openTab shows the existing tab for this pod and kind, or builds a new one (max one each per pod).
function openTab(kind, p, build) {
  const key = `${kind}:${p.cluster}/${p.namespace}/${p.name}`;
  setMin(false);
  if (tabs.has(key)) return activate(key);
  const tab = document.createElement('div');
  tab.className = 'tab';
  tab.dataset.key = key;
  tab.title = `${p.cluster} · ${p.namespace}/${p.name}`;
  const what = { logs: 'Logs', 'logs-deployments': 'Logs (deployment)', 'logs-services': 'Logs (service)', 'logs-jobs': 'Logs (job)' }[kind] || 'Shell';
  tab.innerHTML = `<span class="tl">${what === 'Shell' ? ICONS.shell : ICONS.logs}${what} · ${esc(p.name)}${clusters.length > 1 ? ` <span class="muted">(${esc(p.cluster)})</span>` : ''}</span><b class="x" title="Close">×</b>`;
  const pane = document.createElement('div');
  pane.className = 'pane';
  const t = { tab, pane, stop() {}, fit: null };
  tabs.set(key, t);
  $('tabs').append(tab);
  $('panes').append(pane);
  activate(key); // visible before build: xterm needs a size
  const opts = p.containers.map((c) => `<option>${esc(c)}</option>`).join('');
  pane.innerHTML = build.html(opts, p);
  build.start(p, pane, t);
}

// ---- logs (SSE)
// ---- structured log lines: split off the k8s timestamp, parse JSON logs, find the level
const LEVELS = [[/^(fatal|panic|crit\w*|err\w*)$/i, 'err'], [/^warn\w*$/i, 'warn'], [/^info\w*$/i, 'info'], [/^(debug|trace)$/i, 'dbg']];
const levelClass = (v) => (LEVELS.find(([re]) => re.test(String(v))) || [])[1] || '';
function parseLog(raw) {
  const m = raw.match(/^(\d{4}-\d\d-\d\dT[\d:.]+Z?)\s(.*)$/s);
  const ts = m ? m[1] : '', after = m ? m[2] : raw;
  const sm = after.match(/^\[([^\]\s]+)\]\s(.*)$/s); // "[pod]" tag of a merged deployment/service stream
  const src = sm ? sm[1] : '', rest = sm ? sm[2] : after;
  let obj = null;
  if (rest.trimStart().startsWith('{')) {
    try { const o = JSON.parse(rest); if (o && typeof o === 'object' && !Array.isArray(o)) obj = o; } catch {}
  }
  const lvKey = obj && ['level', 'severity', 'lvl'].find((k) => typeof obj[k] === 'string');
  const level = lvKey ? obj[lvKey] : (rest.match(/\b(FATAL|PANIC|ERROR|ERR|WARN(?:ING)?|INFO|DEBUG|TRACE)\b/i) || [])[1] || '';
  const msgKey = obj && ['msg', 'message'].find((k) => typeof obj[k] === 'string');
  return { ts, src, rest, obj, level, lvKey, msgKey };
}
const short = (v) => { const t = typeof v === 'string' ? v : JSON.stringify(v); return t.length > 80 ? t.slice(0, 80) + '…' : t; };
// highlight wraps every case-insensitive occurrence of q in the line's text in <mark>.
function highlight(d, q) {
  d.innerHTML = d._html;
  if (!q) return;
  const w = document.createTreeWalker(d, NodeFilter.SHOW_TEXT), nodes = [];
  while (w.nextNode()) nodes.push(w.currentNode);
  for (const n of nodes) {
    const lower = n.data.toLowerCase();
    let i = lower.indexOf(q), pos = 0;
    if (i < 0) continue;
    const f = document.createDocumentFragment();
    for (; i >= 0; i = lower.indexOf(q, pos)) {
      f.append(n.data.slice(pos, i));
      const m = document.createElement('mark');
      m.textContent = n.data.slice(i, i + q.length);
      f.append(m);
      pos = i + q.length;
    }
    f.append(n.data.slice(pos));
    n.replaceWith(f);
  }
}
function renderLine(raw) {
  const p = parseLog(raw), d = document.createElement('div');
  d._raw = raw;
  d.title = 'Double-click for details';
  d.className = 'line' + (levelClass(p.level) ? ' lv-' + levelClass(p.level) : '');
  let h = p.ts ? `<span class="ts">${esc(p.ts)}</span>` : '';
  if (p.src) h += `<span class="src" style="color:hsl(${[...p.src].reduce((a, c) => (a * 31 + c.charCodeAt(0)) % 360, 7)} 70% 70%)">${esc(p.src)}</span>`;
  if (p.obj) {
    const skip = new Set([p.lvKey, p.msgKey]);
    h += (p.level ? `<span class="lv">${esc(p.level)}</span>` : '') + (p.msgKey ? `<span class="msg">${esc(p.obj[p.msgKey])}</span>` : '') +
      Object.entries(p.obj).filter(([k]) => !skip.has(k)).map(([k, v]) => ` <span class="kv">${esc(k)}=${esc(short(v))}</span>`).join('');
  } else h += esc(p.rest);
  d.innerHTML = d._html = h;
  return d;
}
// A clicked line opens in a modal: JSON (also inside string fields) indented, anything else as plain text.
function showLogDetail(raw) {
  const p = parseLog(raw);
  let body = p.rest;
  if (/^[{[]/.test(p.rest.trim())) { try { body = prettyJson(JSON.parse(p.rest)); } catch {} }
  const meta = [p.ts, p.src].filter(Boolean).join(' · ');
  $('ldbody').innerHTML = `${meta ? `<p class="muted">${esc(meta)}</p>` : ''}<pre>${esc(body)}</pre>`;
  $('ldtitle').innerHTML = p.level ? pill(p.level, levelClass(p.level) === 'err' ? 'bad' : '') : 'Log entry';
  $('ldcopy').onclick = () => navigator.clipboard.writeText(body).then(() => toast('copied'), () => toast('copy failed'));
  $('logdetail').showModal();
}
$('ldclose').onclick = () => $('logdetail').close();

// Open Logs tabs are remembered in this browser, so a reload reopens them.
const LOGTABS = 'k8ui.logtabs';
function saveLogTabs() {
  const l = [...tabs.values()].filter((t) => t.log).map((t) => t.log);
  try { localStorage.setItem(LOGTABS, JSON.stringify(l)); } catch {}
}
function restoreLogTabs() {
  if (!pods.length && !data.deployments.length && !data.services.length) return; // lists failed: keep what is saved
  let l = [];
  try { l = JSON.parse(localStorage.getItem(LOGTABS)) || []; } catch {}
  l.forEach((x) => {
    const kind = x.kind || 'pods';
    const p = (kind === 'pods' ? pods : data[kind]).find((q) => q.cluster === x.cluster && q.namespace === x.namespace && q.name === x.pod);
    if (p) openLogs(p, x.container, kind);
  });
  saveLogTabs(); // drops tabs whose pod is gone
}
// kind: "pods" (one pod's container), or "deployments" / "services" (every pod they select, merged).
function openLogs(p, container, kind = 'pods') {
  if (kind !== 'pods') p = { ...p, containers: [] }; // the container select stays hidden: all containers
  openTab(kind === 'pods' ? 'logs' : `logs-${kind}`, p, {
    html: (opts) => `<div class="tools">
      <span class="grow"></span>
      <input class="q" placeholder="Search" autocomplete="off" style="width:220px"><span class="t cnt"></span>
      <button type="button" class="up sm" title="Previous match" hidden>↑</button><button type="button" class="down sm" title="Next match" hidden>↓</button>
      <button type="button" class="mark icon" title="Insert a divider line">${ICON_DIVIDER}</button>
      <button type="button" class="reload icon" title="Refresh the logs">${ICON_REFRESH}</button>
      ${fsButton()}
      <details class="menu"><summary>⋯</summary><div class="menupanel">
        <label class="t"${opts.split('</option>').length > 2 ? '' : ' hidden'}>container <select class="c">${opts}</select></label>
        <label class="t">tail <input class="tail" type="number" value="${settings.log_tail}" min="0" style="width:80px"></label>
        <label class="t"><input class="ts" type="checkbox"> timestamps</label>
        <label class="t"><input class="prev" type="checkbox"> previous container</label>
        <button type="button" class="pause">Pause</button><button type="button" class="dl">Download</button>
      </div></details></div>
      <div class="logbox"></div>`,
    start(p, pane, t) {
      const $$ = (s) => pane.querySelector(s);
      const box = $$('.logbox'), pauseBtn = $$('.pause');
      let es = null, paused = false, buf = [], lines = [], hits = [], cur = -1;
      const q = () => $$('.q').value.toLowerCase();
      const isHit = (d) => q() && d._raw.toLowerCase().includes(q());
      const count = () => {
        $$('.cnt').textContent = q() ? `${cur >= 0 ? cur + 1 : 0}/${hits.length}` : '';
        $$('.up').hidden = $$('.down').hidden = !hits.length; // arrows only when there is something to jump to
      };
      const refresh = () => {
        hits = []; cur = -1;
        lines.forEach((d) => {
          d.classList.remove('cur');
          const h = !!isHit(d);
          d.classList.toggle('hit', h);
          if (h) hits.push(d);
          if (h || d.querySelector('mark')) highlight(d, h ? q() : '');
        });
        count();
      };
      const find = (dir) => {
        hits = hits.filter((d) => d.isConnected);
        if (!hits.length) return count();
        cur = (cur + dir + hits.length) % hits.length;
        hits.forEach((d, i) => d.classList.toggle('cur', i === cur));
        hits[cur].scrollIntoView({ block: 'center' });
        count();
      };
      // Dividers are remembered (per tab, in this browser) as "after this log line": a refresh or reload
      // reads the logs again and puts each divider back after its line, if that line is still in the tail.
      const mkey = `k8ui.dividers.${kind}:${p.namespace}/${p.name}`;
      let marks = [], pending = [], fresh = null; // fresh: the divider just added, held at 70% of the view
      try { marks = JSON.parse(localStorage.getItem(mkey)) || []; } catch {}
      const dividerEl = (time) => Object.assign(document.createElement('div'), { className: 'divider', textContent: time });
      const append = (line) => {
        const atEnd = box.scrollTop + box.clientHeight >= box.scrollHeight - 20;
        const d = renderLine(line);
        box.append(d);
        if (fresh && d.getBoundingClientRect().bottom - fresh.getBoundingClientRect().bottom >= box.clientHeight * 0.3 - 20) {
          box.style.paddingBottom = ''; fresh = null; box.scrollTop = box.scrollHeight; // the view is full below the divider: follow the log again
        }
        lines.push(d);
        pending = pending.filter((m) => m.after !== line || (box.append(dividerEl(m.time)), false)); // place and drop the matched marks
        if (isHit(d)) { d.classList.add('hit'); highlight(d, q()); hits.push(d); count(); }
        if (lines.length > 10000) lines.shift().remove();
        if (atEnd) box.scrollTop = box.scrollHeight;
      };
      const stop = () => { if (es) es.close(); es = null; };
      if (container) $$('.c').value = container;
      const run = () => {
        stop();
        t.log = { cluster: p.cluster, namespace: p.namespace, pod: p.name, container: $$('.c').value, kind };
        saveLogTabs();
        box.textContent = ''; lines = []; hits = []; cur = -1; count();
        box.style.paddingBottom = ''; fresh = null;
        pending = marks.filter((m) => m.after !== '' || (box.append(dividerEl(m.time)), false));
        paused = false; buf = []; pauseBtn.textContent = 'Pause';
        const prm = new URLSearchParams({ container: $$('.c').value, tail: $$('.tail').value || 0, follow: 'true',
          timestamps: $$('.ts').checked, previous: $$('.prev').checked });
        prm.set('cluster', p.cluster);
        const e = es = new EventSource(`/api/${kind}/${p.namespace}/${p.name}/logs?${prm}`);
        e.onmessage = (m) => (paused ? buf.push(m.data) : append(m.data));
        // "end"/"error" events are final: close so EventSource does not retry forever.
        e.addEventListener('end', () => { append('[stream ended]'); e.close(); });
        e.addEventListener('error', (m) => { if (m.data) append('[error: ' + m.data + ']'); e.close(); });
        e.onerror = () => { if (!lines.length) append('[could not read logs]'); e.close(); };
      };
      ['.c', '.tail', '.ts', '.prev'].forEach((s) => ($$(s).onchange = run));
      box.ondblclick = (e) => {
        const d = e.target.closest('.line');
        if (d) { getSelection().removeAllRanges(); showLogDetail(d._raw); } // the double click also selected a word
      };
      $$('.q').oninput = refresh;
      // not `&&` as the handler's value: returning false would cancel every other keystroke
      $$('.q').onkeydown = (e) => { if (e.key === 'Enter') find(e.shiftKey ? -1 : 1); };
      $$('.reload').onclick = run;
      $$('.up').onclick = () => find(-1);
      $$('.down').onclick = () => find(1);
      pauseBtn.onclick = () => {
        paused = !paused;
        pauseBtn.textContent = paused ? 'Resume' : 'Pause';
        if (!paused) { buf.forEach(append); buf = []; }
      };
      $$('.mark').onclick = () => {
        const m = { after: lines.length ? lines[lines.length - 1]._raw : '', time: new Date().toLocaleTimeString() };
        marks = [...marks, m].slice(-50);
        try { localStorage.setItem(mkey, JSON.stringify(marks)); } catch {}
        fresh = dividerEl(m.time);
        box.append(fresh);
        // scroll the divider to 70% down the view; the padding gives it room, new lines fill the 30% below
        box.style.paddingBottom = Math.round(box.clientHeight * 0.3) + 'px';
        box.scrollTop += fresh.getBoundingClientRect().top - box.getBoundingClientRect().top - box.clientHeight * 0.7;
      };
      $$('.dl').onclick = () => {
        if (!lines.length) return toast('No logs to download');
        const a = document.createElement('a');
        a.href = URL.createObjectURL(new Blob([lines.map((d) => d._raw).join('\n') + '\n'], { type: 'text/plain' }));
        a.download = `${p.name}_${$$('.c').value}.log`;
        a.click();
        URL.revokeObjectURL(a.href);
      };
      // Only the tab you look at keeps its stream open: browsers allow about 6 connections per host,
      // and every followed log holds one. A hidden tab closes its stream and reads the logs again when shown.
      let idle = false;
      t.pause = () => { if (es) { stop(); idle = true; } };
      t.resume = () => { if (idle) { idle = false; run(); } };
      t.stop = stop;
      run();
    },
  });
}

// ---- shell (WebSocket + xterm.js)
// The shell keeps running on the server when this page goes away; closing the tab ends it.
async function openShell(p, container) {
  if (!container && !tabs.has(`shell:${p.cluster}/${p.namespace}/${p.name}`)) {
    container = p.containers[0];
    if (p.containers.length > 1) { // a pod with several containers: ask
      const r = await ask({ title: `Shell in ${p.name}`, fields: [{ name: 'c', label: 'Container', type: 'select', options: p.containers, value: container }], ok: 'Open shell' });
      container = r?.c;
    }
    if (!container) return;
    ktoast(p, p.namespace, `exec -it ${p.name} -c ${container} -- sh`);
  }
  openTab('shell', p, {
    html: () => '<div class="term"></div>',
    start(p, pane, t) {
      const el = pane.querySelector('.term');
      t.kill = () => api(`/api/shells?${new URLSearchParams({ cluster: p.cluster, namespace: p.namespace, pod: p.name, container })}`, { method: 'DELETE' }).catch(() => {});
      el.innerHTML = '';
      const term = new Terminal({ cursorBlink: true, fontSize: 13 });
      const fit = new FitAddon.FitAddon();
      term.loadAddon(fit);
      term.open(el);
      fit.fit();
      t.fit = () => el.offsetHeight && fit.fit();
      new ResizeObserver(t.fit).observe(el); // dock shown, tab switched or window resized
      const proto = location.protocol === 'https:' ? 'wss' : 'ws';
      const ws = new WebSocket(`${proto}://${location.host}/api/pods/${p.namespace}/${p.name}/exec?cluster=${encodeURIComponent(p.cluster)}&container=${encodeURIComponent(container)}`);
      ws.binaryType = 'arraybuffer';
      const enc = new TextEncoder();
      const sendSize = () => ws.readyState === 1 && ws.send(JSON.stringify({ cols: term.cols, rows: term.rows }));
      ws.onopen = () => { fit.fit(); sendSize(); if (tabs.get(activeTab) === t) term.focus(); }; // never steal focus from another tab's inputs
      ws.onmessage = (m) => term.write(new Uint8Array(m.data));
      ws.onclose = () => term.write('\r\n[disconnected]\r\n');
      term.onData((d) => ws.readyState === 1 && ws.send(enc.encode(d)));
      term.onResize(sendSize);
      t.stop = () => { ws.onclose = null; ws.close(); term.dispose(); };
    },
  });
}
// after a page reload: reopen a tab for every shell still running on the server
async function restoreShells() {
  if (!pods.length) return; // pod list failed: do not judge shells by it
  for (const sh of await api('/api/shells').catch(() => [])) {
    if (sh.cluster !== cluster) continue; // another connection's shell: it stays alive, its pods are not loaded now
    const p = pods.find((x) => x.namespace === sh.namespace && x.name === sh.pod);
    if (p) openShell(p, sh.container);
    else api(`/api/shells?${new URLSearchParams(sh)}`, { method: 'DELETE' }).catch(() => {});
  }
}
window.addEventListener('resize', () => tabs.get(activeTab)?.fit?.());
document.addEventListener('click', (e) => document.querySelectorAll('details.menu[open]').forEach((d) => d.contains(e.target) || (d.open = false)));
// Full screen: the dock covers the whole window (Esc or the button restores it).
const svg = (d) => `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="${d}"/></svg>`;
const ICON_EXPAND = svg('M14 3h7v7M21 3l-7 7M10 21H3v-7M3 21l7-7M3 10V3h7M3 3l7 7M21 14v7h-7M21 21l-7-7');
const ICON_COLLAPSE = svg('M3 10h7V3M10 10L3 3M21 10h-7V3M14 10l7-7M3 14h7v7M10 14l-7 7M21 14h-7v7M14 14l7 7');
const ICON_REFRESH = svg('M21 12a9 9 0 1 1-2.64-6.36M21 3v6h-6');
const ICON_DIVIDER = svg('M2 10h20M2 14h20M12 10V3M8 7l4-4 4 4M12 14v7M8 17l4 4 4-4');
const isFull = () => $('dock').classList.contains('full');
// the full screen button lives in every pane's toolbar
const fsButton = () => `<button type="button" class="fs" title="${isFull() ? 'Exit full screen' : 'Full screen'}">${isFull() ? ICON_COLLAPSE : ICON_EXPAND}</button>`;
function setFull(on) {
  $('dock').classList.toggle('full', on);
  document.querySelectorAll('.fs').forEach((b) => { b.innerHTML = on ? ICON_COLLAPSE : ICON_EXPAND; b.title = on ? 'Exit full screen' : 'Full screen'; });
  tabs.get(activeTab)?.fit?.();
}
$('dock').addEventListener('click', (e) => { if (e.target.closest('.fs')) setFull(!isFull()); });
$('dockmax').innerHTML = ICON_EXPAND;
// minimize: only the tab bar stays; close: every tab goes (their shells end)
const ICON_MIN = svg('M5 12h14'), ICON_RESTORE = svg('M6 15l6-6 6 6'), ICON_CLOSE = svg('M6 6l12 12M18 6L6 18');
function setMin(on) {
  if (on && isFull()) setFull(false);
  $('dock').classList.toggle('min', on);
  document.body.classList.toggle('docked-min', on);
  $('dockmin').innerHTML = on ? ICON_RESTORE : ICON_MIN;
  $('dockmin').title = on ? 'Restore' : 'Minimize';
  if (!on) tabs.get(activeTab)?.fit?.();
}
$('dockmin').innerHTML = ICON_MIN; $('dockclose').innerHTML = ICON_CLOSE;
$('dockmin').onclick = () => setMin(!$('dock').classList.contains('min'));
$('dockclose').onclick = async () => {
  if (tabs.size > 1 && !await askConfirm(`Close all ${tabs.size} tabs? Open shells end.`, { title: 'Close all tabs', ok: 'Close all', danger: true })) return;
  [...tabs.keys()].forEach(closeTab);
  setFull(false); setMin(false);
};
document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape' && $('dock').classList.contains('full') && !document.querySelector('dialog[open]')) setFull(false);
});

// Stretch: drag the top edge of the dock; the height is remembered.
const DOCKH = 'k8ui.dockh';
const setDockH = (px) => document.documentElement.style.setProperty('--dockh', px + 'px');
try { const h = parseInt(localStorage.getItem(DOCKH), 10); if (h) setDockH(Math.min(h, innerHeight - 80)); } catch {}
$('dockresize').onpointerdown = (e) => {
  e.preventDefault();
  const bar = $('dockresize'), clamp = (y) => Math.max(120, Math.min(innerHeight - y, innerHeight - 80));
  bar.setPointerCapture(e.pointerId);
  bar.onpointermove = (m) => setDockH(clamp(m.clientY));
  bar.onpointerup = (m) => {
    bar.onpointermove = bar.onpointerup = null;
    try { localStorage.setItem(DOCKH, String(clamp(m.clientY))); } catch {}
  };
};
