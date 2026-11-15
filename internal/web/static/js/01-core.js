// Shared helpers: DOM shortcut, toast, fetch wrapper, escaping, the checkbox dropdown.
// (Plain scripts share one global scope: index.html loads them in numeric order.)

const $ = (id) => document.getElementById(id);
let pods = [];
let selected = []; // namespaces to show; [] = every namespace
const nsKey = () => selected.join(',');
const nsLabel = () => (selected.length ? selected.join(', ') : 'all namespaces');
const inNs = (name) => !selected.length || selected.includes(name);
let settings = { refresh_seconds: 5, log_tail: 200, read_only: false };

function toast(msg) {
  const t = $('msg');
  t.textContent = msg;
  t.hidden = false;
  clearTimeout(toast.h);
  toast.h = setTimeout(() => (t.hidden = true), 5000);
}

async function api(path, opts) {
  const r = await fetch(path, opts);
  const body = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(body.error || r.statusText);
  return body;
}

const esc = (s) => String(s).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

const pill = (t, cls = '') => `<span class="pill ${cls}">${esc(t)}</span>`;
// image drops the registry path for the cell; the full name is the tooltip.
const image = (i) => `<span title="${esc(i)}">${esc(i.slice(i.lastIndexOf('/') + 1))}</span>`;
const none = '<span class="muted">—</span>';

// The namespace filter in the bar: "" = every namespace. Hidden on the Namespaces page, which lists them.
// multiSelect is a dropdown of checkboxes inside host; onChange gets the ticked values.
function multiSelect(host, allLabel, unit, onChange) {
  host.innerHTML = '<details class="dd"><summary></summary><div class="ddpanel"><button type="button" class="sm">Clear selection</button><div></div></div></details>';
  const dd = host.firstElementChild, sum = dd.querySelector('summary'), list = dd.querySelector('.ddpanel div');
  const ms = {
    selected: [],
    fmt: (v) => v, // how a value is shown
    cmp: undefined, // how values are sorted (default: as text)
    extra: {}, // value -> a count shown next to it
    extraUnit: 'keys',
    // options sets the choices (plus anything still ticked, so it can be unticked); the list is rebuilt only when it changed.
    options(values, extra = {}) {
      ms.extra = extra;
      ms.vals = values; // kept so a connection's choices can be parked and restored
      const all = [...new Set([...values, ...ms.selected])].sort(ms.cmp);
      const key = all.join('\n') + JSON.stringify(extra);
      if (key !== ms.key) {
        ms.key = key;
        list.innerHTML = all.map((v) => `<label><input type="checkbox" value="${esc(v)}"> ${esc(ms.fmt(v))}${v in ms.extra ? ` <span class="t">(${esc(ms.extra[v])} ${ms.extraUnit})</span>` : ''}</label>`).join('');
      }
      ms.sync();
    },
    set(values) { ms.selected = values; ms.sync(); },
    sync() {
      const n = ms.selected.length;
      sum.textContent = !n ? allLabel : n <= 2 ? ms.selected.map(ms.fmt).join(', ') : `${n} ${unit}`;
      list.querySelectorAll('input').forEach((c) => (c.checked = ms.selected.includes(c.value)));
    },
  };
  list.addEventListener('change', () => { ms.selected = [...list.querySelectorAll('input:checked')].map((c) => c.value); ms.sync(); onChange(ms.selected); });
  dd.querySelector('button').onclick = () => { ms.selected = []; ms.sync(); onChange([]); };
  document.addEventListener('click', (e) => { if (!dd.contains(e.target)) dd.open = false; });
  return ms;
}
