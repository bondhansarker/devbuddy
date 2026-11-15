// sidebar icons (inline SVG, drawn in currentColor)
const ICONS = {
  view: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2 12s3.6-7 10-7 10 7 10 7-3.6 7-10 7S2 12 2 12z"/><circle cx="12" cy="12" r="3"/></svg>',
  trash: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 6h18M8 6V4a1 1 0 0 1 1-1h6a1 1 0 0 1 1 1v2M6 6l1 14a2 2 0 0 0 2 2h6a2 2 0 0 0 2-2l1-14M10 11v6M14 11v6"/></svg>',
  play: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M7 4l13 8-13 8z"/></svg>',
  stop: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="6" y="6" width="12" height="12" rx="2"/></svg>',
  open: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h14M13 6l6 6-6 6"/></svg>',
  plug: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 2v6M15 2v6M6 8h12v4a6 6 0 0 1-12 0zM12 18v4"/></svg>',
  power: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2v10M6.4 6.4a8 8 0 1 0 11.2 0"/></svg>',
  braces: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M8 3H7a2 2 0 0 0-2 2v4a2 2 0 0 1-2 2 2 2 0 0 1 2 2v4a2 2 0 0 0 2 2h1M16 3h1a2 2 0 0 1 2 2v4a2 2 0 0 0 2 2 2 2 0 0 0-2 2v4a2 2 0 0 1-2 2h-1"/></svg>',
  copy: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="12" height="12" rx="2"/><path d="M5 15V5a2 2 0 0 1 2-2h8"/></svg>',
  close: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 6l12 12M18 6L6 18"/></svg>',
  restart: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 12a8 8 0 1 1-2.6-5.9"/><path d="M20 4v5h-5"/></svg>',
  logs: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5M8 12h8M8 15h5"/></svg>',
  shell: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="4" width="18" height="16" rx="3"/><path d="M3 9h18M7.5 12.5l3 2-3 2M13 17h3.5"/></svg>',
  fwd: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="10" cy="10" r="7"/><path d="M3 10h14M10 3c2.5 2.5 2.5 11.5 0 14M10 3c-2.5 2.5-2.5 11.5 0 14"/><path d="M15 18h6m-3-3l3 3-3 3"/></svg>',
  k8s: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12.0 1.5 L20.2 5.5 L22.2 14.3 L16.6 21.5 L7.4 21.5 L1.8 14.3 L3.8 5.5z"/><circle cx="12" cy="12" r="4.6"/><path d="M12.0 10.4L12.0 5.4M13.3 11.0L17.2 7.9M13.6 12.4L18.4 13.5M12.7 13.4L14.9 17.9M11.3 13.4L9.1 17.9M10.4 12.4L5.6 13.5M10.7 11.0L6.8 7.9"/></svg>',
  redis: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3l9 4.5-9 4.5-9-4.5z"/><path d="M3 12l9 4.5 9-4.5M3 16.5l9 4.5 9-4.5"/><circle cx="9.6" cy="7.6" r=".9" fill="currentColor"/><path d="M12.6 8.7l1.6-2.4 1.6 2.4z" fill="currentColor"/></svg>',
  es: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9.5"/><path d="M9.8 2.7L9 9.5M2.6 8.2L9 9.5l3 3 8.3-4.6M12 12.5l3 3 5.9.4M15 15.5l-.8 5.9"/></svg>',
  chev: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 9l6 6 6-6"/></svg>',
  namespaces: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="3" stroke-dasharray="3 3"/></svg>',
  services: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><circle cx="5" cy="5" r="2"/><circle cx="19" cy="5" r="2"/><circle cx="5" cy="19" r="2"/><circle cx="19" cy="19" r="2"/><path d="M7 7l3 3M17 7l-3 3M7 17l3-3M17 17l-3-3"/></svg>',
  deployments: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2l9 5-9 5-9-5z"/><path d="M3 12l9 5 9-5M3 17l9 5 9-5"/></svg>',
  jobs: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="13" r="8"/><path d="M12 9v4l2 2M9 2h6"/></svg>',
  pods: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 8l-9-5-9 5v8l9 5 9-5z"/><path d="M3 8l9 5 9-5M12 13v8"/></svg>',
  keys: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="8" cy="15" r="4"/><path d="M11 12l9-9M16 8l3 3"/></svg>',
  console: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="4" width="18" height="16" rx="2"/><path d="M7 10l3 2-3 2M13 15h4"/></svg>',
  docs: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z"/><path d="M14 3v6h6M8 13h8M8 17h8"/></svg>',
  indices: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><ellipse cx="12" cy="5" rx="8" ry="3"/><path d="M4 5v14c0 1.7 3.6 3 8 3s8-1.3 8-3V5M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3"/></svg>',
  query: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="7"/><path d="M21 21l-5-5"/></svg>',
  forwards: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h14M13 6l6 6-6 6"/></svg>',
};
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
  t.classList.toggle('cmd', /^[$#] /.test(msg)); // a kubectl command: monospace
  t.hidden = false;
  clearTimeout(toast.h);
  toast.h = setTimeout(() => (t.hidden = true), t.classList.contains('cmd') ? 8000 : 5000);
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
  host.innerHTML = '<details class="dd"><summary></summary><div class="ddpanel"><input type="search" class="ddq" placeholder="Search…" autocomplete="off"><button type="button" class="sm">Clear selection</button><div></div></div></details>';
  const dd = host.firstElementChild, sum = dd.querySelector('summary'), list = dd.querySelector('.ddpanel div'), q = dd.querySelector('.ddq');
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
      ms.draw();
    },
    // ticked choices come first, so they are always at the top; the rest follow
    draw() {
      const all = [...new Set([...(ms.vals || []), ...ms.selected])].sort(ms.cmp);
      const sel = new Set(ms.selected);
      all.splice(0, all.length, ...all.filter((v) => sel.has(v)), ...all.filter((v) => !sel.has(v)));
      const key = all.join('\n') + '|' + ms.selected.join('\n') + JSON.stringify(ms.extra);
      if (key !== ms.key) {
        ms.key = key;
        list.innerHTML = all.map((v) => `<label><input type="checkbox" value="${esc(v)}"> ${esc(ms.fmt(v))}${v in ms.extra ? ` <span class="t">(${esc(ms.extra[v])} ${ms.extraUnit})</span>` : ''}</label>`).join('');
      }
      ms.sync();
      ms.find();
    },
    find() { // the search box hides the choices that do not match
      const t = q.value.trim().toLowerCase();
      list.querySelectorAll('label').forEach((l) => (l.hidden = !!t && !l.querySelector('input').checked && !l.textContent.toLowerCase().includes(t)));
    },
    set(values) { ms.selected = values; ms.draw(); },
    sync() {
      const n = ms.selected.length;
      sum.textContent = !n ? allLabel : n <= 2 ? ms.selected.map(ms.fmt).join(', ') : `${n} ${unit}`;
      list.querySelectorAll('input').forEach((c) => (c.checked = ms.selected.includes(c.value)));
    },
  };
  q.oninput = ms.find;
  dd.addEventListener('toggle', () => { if (dd.open) { q.value = ''; ms.find(); q.focus(); } });
  list.addEventListener('change', () => { ms.selected = [...list.querySelectorAll('input:checked')].map((c) => c.value); ms.draw(); onChange(ms.selected); });
  dd.querySelector('button').onclick = () => { ms.selected = []; ms.draw(); onChange([]); };
  document.addEventListener('click', (e) => { if (!dd.contains(e.target)) dd.open = false; });
  return ms;
}

// JSON is always shown indented: strings that hold JSON are opened too, so nothing needs a second view
function deepParse(v) {
  if (typeof v === 'string') {
    const t = v.trim();
    if (/^[{[]/.test(t)) { try { return deepParse(JSON.parse(t)); } catch {} }
    return v;
  }
  if (Array.isArray(v)) return v.map(deepParse);
  if (v && typeof v === 'object') return Object.fromEntries(Object.entries(v).map(([k, x]) => [k, deepParse(x)]));
  return v;
}
const prettyJson = (v) => JSON.stringify(deepParse(v), null, 2);
// the copy / close icons every modal uses
function dlgIcons(copyId, closeId) {
  const c = $(copyId), x = $(closeId);
  c.className = x.className = 'icon'; c.innerHTML = ICONS.copy; x.innerHTML = ICONS.close; c.title = 'Copy'; x.title = 'Close';
}

// an icon-only action button; the label is its tooltip (and its accessible name)
const ab = (icon, label, attrs = '', cls = '') => `<button type="button" class="sm icon-btn ${cls}" title="${label}" aria-label="${label}" ${attrs}>${ICONS[icon]}</button>`;

// In-app dialogs instead of the browser's confirm() / prompt().
// ask({ title, message, fields: [{ name, label, value, type: 'number' | 'select' | 'text', options, hint, required }], ok, danger })
// resolves to the entered values ({} when there are no fields), or null when cancelled.
function ask({ title = '', message = '', fields = [], ok = 'OK', danger = false }) {
  return new Promise((resolve) => {
    const d = $('askdlg'), form = $('askform');
    $('asktitle').textContent = title;
    $('askx').innerHTML = ICONS.close; $('askx').title = 'Cancel';
    $('askbody').innerHTML = (message ? `<p class="askmsg">${esc(message)}</p>` : '') + fields.map((f) => `<label class="askf"><span>${esc(f.label)}</span>${
      f.type === 'select'
        ? `<select name="${esc(f.name)}">${f.options.map((o) => `<option${o === f.value ? ' selected' : ''}>${esc(o)}</option>`).join('')}</select>`
        : `<input name="${esc(f.name)}" type="${f.type || 'text'}" value="${esc(f.value ?? '')}" ${f.type === 'number' ? 'min="1" max="65535"' : ''} ${f.required ? 'required' : ''} autocomplete="off">`
    }${f.hint ? `<small class="muted">${esc(f.hint)}</small>` : ''}</label>`).join('');
    const okb = $('askok');
    okb.textContent = ok; okb.className = danger ? 'danger' : 'primary';
    let done = false;
    const finish = (v) => { if (done) return; done = true; d.close(); resolve(v); };
    form.onsubmit = (e) => { e.preventDefault(); finish(Object.fromEntries(fields.map((f) => [f.name, form.elements[f.name].value]))); };
    $('askcancel').onclick = $('askx').onclick = () => finish(null);
    d.oncancel = (e) => { e.preventDefault(); finish(null); }; // Esc
    d.showModal();
    (form.querySelector('input, select') || (danger ? $('askcancel') : okb)).focus();
  });
}
const askConfirm = (message, opts = {}) => ask({ message, ...opts }).then((r) => r !== null);
