// Redis: connections, keys, console.
// (Plain scripts share one global scope: index.html loads them in numeric order.)

// ---- redis: db indexes (several at once), SCAN search, view and delete keys
const rd = { keys: [], cursors: {}, sel: new Set(), gen: 0, err: '', conns: [], sort: '', dbsLoaded: false };
const rdbSel = multiSelect($('rdbdd'), 'Select a database', 'databases', () => redisDbsChanged());
rdbSel.fmt = (v) => `db${v}`;
rdbSel.cmp = (a, b) => a - b;
rdbSel.set(['0']);
const rsrv = () => ds.redis.active; // the active Redis connection
const rdbs = () => rdbSel.selected.map(Number); // the ticked db indexes
const rdb = () => rdbs()[0] ?? 0; // the console uses the first one
const rq = (extra = {}, db = rdb()) => new URLSearchParams({ db, ...extra });
const rid = (k) => `${k.db}\u0001${k.key}`;
const fmtTtl = (s) => (s < 0 ? '—' : s < 60 ? `${s}s` : s < 3600 ? `${Math.round(s / 60)}m` : s < 86400 ? `${Math.round(s / 3600)}h` : `${Math.round(s / 86400)}d`);

// Each Redis connection keeps its own keys, db choice, search, type filter and sort: parked here when you
// move to another connection, brought back when you return.
ds.redis.save = () => ({ keys: rd.keys, cursors: rd.cursors, sel: rd.sel, err: rd.err, sort: rd.sort, dbsLoaded: rd.dbsLoaded,
  dbs: rdbSel.selected, vals: rdbSel.vals, extra: rdbSel.extra, q: $('rkeysearch').value, type: $('rtype').value, tab: ds.redis.tab });
ds.redis.load = (s) => {
  rd.gen++; // answers still on their way for the previous connection are dropped
  Object.assign(rd, { keys: s?.keys ?? [], cursors: s?.cursors ?? {}, sel: s?.sel ?? new Set(), err: s?.err ?? '', sort: s?.sort ?? '', dbsLoaded: s?.dbsLoaded ?? false });
  rdbSel.options(s?.vals ?? [], s?.extra ?? {});
  rdbSel.set(s?.dbs ?? ['0']);
  $('rkeysearch').value = s?.q ?? ''; $('rtype').value = s?.type ?? '';
};
function syncRedisBar() {
  syncEsBar();
  const on = view === 'redis';
  $('rhead').hidden = !on; // connection name on top, the filters under it
  $('rdbdd').hidden = !on;
  $('search').hidden = on || view === 'es' || view === 'rconns' || view === 'esconns' || view === 'kconns'; // the key / document search lives in the table header
  if (!on) $('rdelsel').hidden = $('rexport').hidden = true;
}
// the Redis page of the active connection: Keys or Console (the sidebar picks which)
async function showRedis() {
  const name = rsrv(), c = ds.redis.conns.find((x) => x.name === name);
  $('rname').textContent = name; $('raddr').textContent = c ? c.addr : '';
  if (!rd.dbsLoaded) { rd.dbsLoaded = true; await loadRedisDBs(); }
  setRTab(ds.redis.tab);
}
async function loadRedisDBs() {
  const name = rsrv();
  try {
    const r = await api(`/api/redis/${encodeURIComponent(name)}/dbs`);
    if (name !== rsrv()) return; // another connection became active meanwhile
    const counts = Object.fromEntries(r.dbs.map((d) => [d.db, d.keys]));
    const idx = [...new Set([...Array(Math.min(r.databases, 16)).keys(), ...r.dbs.map((d) => d.db)])];
    rdbSel.options(idx.map(String), Object.fromEntries(Object.entries(counts).map(([k, v]) => [k, v])));
  } catch (e) { toast(e.message); }
}
function redisDbsChanged() {
  if (rtab === 'keys') loadRedisKeys(true);
}
// every Keys action shows the redis-cli command it is the same as, as a toast
const rq1 = (s) => (/[\s"']/.test(s) ? JSON.stringify(s) : s);
const rcli = (db, args) => `redis-cli -n ${db} ${args}`;
const rpat = (m) => (!m ? '*' : /[*?[\\]/.test(m) ? m : `*${m}*`);
const rtoast = (lines, result) => toast(`$ ${lines.join('\n$ ')}${result ? '\n' + result : ''}`);
// loadRedisKeys: reset = start over on every ticked db; else the next page of each db that has more.
async function loadRedisKeys(reset) {
  const gen = reset ? ++rd.gen : rd.gen;
  if (reset) { rd.keys = []; rd.cursors = {}; rd.sel.clear(); rd.err = ''; renderRedis(); }
  const dbs = rdbs().filter((d) => reset || (rd.cursors[d] ?? '0') !== '0');
  const match = $('rkeysearch').value.trim();
  if (dbs.length) rtoast(dbs.map((d) => rcli(d, `SCAN ${rd.cursors[d] ?? '0'} MATCH ${rq1(rpat(match))} COUNT 500${$('rtype').value ? ' TYPE ' + $('rtype').value : ''}`)));
  await Promise.all(dbs.map(async (d) => {
    try {
      const r = await api(`/api/redis/${encodeURIComponent(rsrv())}/keys?${rq({ match, type: $('rtype').value, cursor: rd.cursors[d] ?? '0', count: 200 }, d)}`);
      if (gen !== rd.gen) return; // the server, dbs or search changed while this was loading
      rd.keys = rd.keys.concat(r.keys.map((k) => ({ ...k, db: d })));
      rd.cursors[d] = r.cursor;
    } catch (e) { if (gen === rd.gen) rd.err = `db${d}: ${e.message}`; toast(`db${d}: ${e.message}`); }
  }));
  if (gen === rd.gen) renderRedis();
}
const moreRedis = () => rdbs().some((d) => (rd.cursors[d] ?? '0') !== '0');
function renderRedis() {
  const ro = settings.read_only, multi = rdbs().length > 1, cols = 4 + (multi ? 1 : 0) + 1;
  $('rdbth').hidden = !multi;
  // sorted by TTL when asked (no expiry counts as the longest); i stays the index in rd.keys for the handlers
  const ttl = (k) => (k.ttl < 0 ? Infinity : k.ttl), dir = rd.sort;
  const order = rd.keys.map((k, i) => [k, i]);
  if (dir) order.sort((a, b) => (dir === 'asc' ? 1 : -1) * (ttl(a[0]) === ttl(b[0]) ? 0 : ttl(a[0]) < ttl(b[0]) ? -1 : 1));
  $('rsorti').textContent = dir === 'asc' ? '▲' : dir === 'desc' ? '▼' : '⇅';
  $('rrows').innerHTML = rd.keys.length ? order.map(([k, i]) => `<tr>
      <td class="cb">${ro ? '' : `<input type="checkbox" data-i="${i}" ${rd.sel.has(rid(k)) ? 'checked' : ''}>`}</td>
      <td class="name" style="word-break:break-all">${esc(k.key)}</td>${multi ? `<td>db${k.db}</td>` : ''}<td>${pill(k.type)}</td><td class="num">${fmtTtl(k.ttl)}</td>
      <td class="actions">${ab('view', 'View', `data-a="view" data-i="${i}"`)}${ro ? '' : ab('trash', 'Delete', `data-a="del" data-i="${i}"`, 'danger')}</td></tr>`).join('')
    : `<tr><td colspan="${cols}" class="muted">${rd.err ? esc(rd.err) : !rdbs().length ? 'pick a database' : moreRedis() ? 'scanning…' : 'no keys match'}</td></tr>`;
  $('rmore').hidden = !moreRedis();
  $('rall').hidden = ro;
  $('rdelsel').hidden = ro || rtab !== 'keys' || !rd.sel.size || view !== 'redis';
  $('rexport').hidden = rtab !== 'keys' || view !== 'redis' || !rd.keys.length;
  $('rexport').innerHTML = `${ICONS.download} Export`; $('rexport').title = rd.sel.size ? `Export the ${rd.sel.size} selected keys with their values as JSON` : 'Export the listed keys (up to 200) with their values as JSON';
  $('rdelsel').innerHTML = `${ICONS.trash} ${rd.sel.size}`; $('rdelsel').title = `Delete selected (${rd.sel.size})`;
  $('rall').checked = rd.keys.length > 0 && rd.sel.size === rd.keys.length;
}
let rTimer;
const redisSearchSoon = () => { clearTimeout(rTimer); rTimer = setTimeout(() => loadRedisKeys(true), 350); };
$('rmore').onclick = () => loadRedisKeys(false);
$('rtype').onchange = () => loadRedisKeys(true); // the server filters by type
$('rttlth').onclick = () => { rd.sort = { '': 'asc', asc: 'desc', desc: '' }[rd.sort]; renderRedis(); }; // header click: soonest, longest, off
$('rkeysearch').oninput = redisSearchSoon;
$('rall').onchange = () => { rd.sel = $('rall').checked ? new Set(rd.keys.map(rid)) : new Set(); renderRedis(); };

// redisDelete removes {db, key} items, one call per db.
async function redisDelete(items) {
  let deleted = 0;
  for (const db of new Set(items.map((i) => i.db))) {
    const r = await api(`/api/redis/${encodeURIComponent(rsrv())}/delete`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ db, keys: items.filter((i) => i.db === db).map((i) => i.key) }),
    });
    deleted += r.deleted;
  }
  const gone = new Set(items.map(rid));
  rd.keys = rd.keys.filter((k) => !gone.has(rid(k)));
  gone.forEach((id) => rd.sel.delete(id));
  renderRedis();
  loadRedisDBs().then(() => {}); // the key counts changed
  rtoast([...new Set(items.map((i) => i.db))].map((db) => rcli(db, `UNLINK ${items.filter((i) => i.db === db).map((i) => rq1(i.key)).join(' ')}`).slice(0, 200)), `(integer) ${deleted}`);
}
const confirmDelete = (items) => {
  const dbs = [...new Set(items.map((i) => `db${i.db}`))].join(', ');
  return askConfirm(items.length === 1 ? `Delete key ${items[0].key} from ${rsrv()} ${dbs}?` : `Delete ${items.length} keys from ${rsrv()} (${dbs})?`, { title: items.length === 1 ? 'Delete key' : 'Delete keys', ok: 'Delete', danger: true });
};
// export: the selected keys (else the listed ones, up to 200) with their values, as one JSON file
$('rexport').onclick = async () => {
  const items = (rd.sel.size ? rd.keys.filter((k) => rd.sel.has(rid(k))) : rd.keys).slice(0, 200);
  if (!items.length) return;
  try {
    const out = [];
    for (const db of new Set(items.map((i) => i.db))) {
      const keys = items.filter((i) => i.db === db).map((i) => i.key);
      out.push(...await api(`/api/redis/${encodeURIComponent(rsrv())}/export`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ db, keys }) }));
    }
    download(`redis-${rsrv()}-${new Date().toISOString().slice(0, 10)}.json`, JSON.stringify(deepParse(out), null, 2), 'application/json');
    toast(`exported ${out.length} key${out.length === 1 ? '' : 's'}`);
  } catch (e) { toast(e.message); }
};
$('rdelsel').onclick = async () => {
  const items = rd.keys.filter((k) => rd.sel.has(rid(k)));
  if (items.length && await confirmDelete(items)) await redisDelete(items).catch((e) => toast(e.message));
};
$('rrows').addEventListener('change', (e) => {
  const c = e.target.closest('input[data-i]');
  if (!c) return;
  const id = rid(rd.keys[c.dataset.i]);
  c.checked ? rd.sel.add(id) : rd.sel.delete(id);
  renderRedis();
});
$('rrows').addEventListener('click', async (e) => {
  const b = e.target.closest('button[data-a]');
  if (!b) return;
  const k = rd.keys[b.dataset.i];
  if (b.dataset.a === 'view') return openRedisKey(k.key, k.db);
  if (await confirmDelete([k])) await redisDelete([k]).catch((x) => toast(x.message));
});

async function openRedisKey(key, db, quiet) {
  let v;
  try { v = await api(`/api/redis/${encodeURIComponent(rsrv())}/key?${rq({ key }, db)}`); } catch (e) { return toast(e.message); }
  // the value as indented JSON: a hash is an object, a list / set an array, a zset {member, score} pairs, a stream {id, fields}
  const value = v.type === 'string' ? v.string
    : v.type === 'hash' ? Object.fromEntries(v.items.map((i) => [i.k, i.v]))
    : v.type === 'zset' ? v.items.map((i) => ({ member: i.v, score: Number(i.k) }))
    : v.type === 'stream' ? v.items.map((i) => ({ id: i.k, fields: i.v }))
    : v.items.map((i) => i.v);
  let text = value;
  if (typeof value !== 'string' || /^[{[]/.test(value.trim())) { try { text = prettyJson(value); } catch { text = String(value); } }
  const body = `<pre>${esc(text)}</pre>`;
  const k = rq1(key), read = { string: `GET ${k}`, hash: `HSCAN ${k} 0 COUNT 500`, list: `LRANGE ${k} 0 499`, set: `SSCAN ${k} 0 COUNT 500`, zset: `ZRANGE ${k} 0 499 WITHSCORES`, stream: `XRANGE ${k} - + COUNT 500` }[v.type];
  if (!quiet) rtoast([rcli(db, `TYPE ${k}`), rcli(db, `TTL ${k}`), ...(read ? [rcli(db, read)] : [])]);
  $('rdtitle').textContent = v.key;
  $('rdbody').innerHTML = `<p class="muted">${pill(v.type)} ${v.type === 'string' ? `${v.size} bytes` : `${v.size} elements`} · TTL ${v.ttl < 0 ? 'none' : fmtTtl(v.ttl)} · ${esc(rsrv())} db${db}
    ${v.truncated ? ' · <strong>truncated: only the first part is shown</strong>' : ''}${v.binary ? ' · not valid UTF-8, shown lossy' : ''}</p>${body}`;
  const editable = v.type === 'string' && !v.truncated && !v.binary && !settings.read_only;
  $('rdedit').hidden = !editable;
  const changed = async () => { // show the new value in the modal
    const nv = await api(`/api/redis/${encodeURIComponent(rsrv())}/key?${rq({ key }, db)}`);
    const row = rd.keys.find((x) => x.key === key && x.db === db);
    if (row) { row.ttl = nv.ttl; renderRedis(); }
    await openRedisKey(key, db, true);
  };
  $('rdedit').onclick = async () => {
    const a = await ask({ title: `Edit ${key}`, ok: 'Save', fields: [{ name: 'v', label: 'Value (the TTL is kept)', type: 'textarea', rows: 12, value: v.string }] });
    if (!a) return;
    try {
      await api(`/api/redis/${encodeURIComponent(rsrv())}/set`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ db, key, value: a.v }) });
      rtoast([rcli(db, `SET ${rq1(key)} ${rq1(a.v.length > 60 ? a.v.slice(0, 60) + '…' : a.v)} KEEPTTL`)], 'OK');
      await changed();
    } catch (e) { toast(e.message); }
  };
  $('rdcopy').onclick = () => navigator.clipboard.writeText(text).then(() => toast('copied'), () => toast('copy failed'));
  $('redisdlg').showModal();
}
$('rdclose').onclick = () => $('redisdlg').close();

// ---- redis tabs: Keys and Console (redis-cli style)
let rtab = 'keys';
function setRTab(t) {
  rtab = t;
  ds.redis.tab = t;
  markNav();
  document.querySelectorAll('#redisView .rtabs button').forEach((b) => b.classList.toggle('on', b.dataset.t === t));
  $('rkeys').hidden = t !== 'keys'; $('rconsole').hidden = t !== 'console';
  syncRedisBar(); // the search box is for keys
  renderRedis();
  if (t === 'keys') loadRedisKeys(true);
  if (t === 'console') showConsole();
}
document.querySelector('#redisView .rtabs').addEventListener('click', (e) => { const b = e.target.closest('button[data-t]'); if (b) setRTab(b.dataset.t); });
// a different server or db: keys reload (above); the other tabs follow

// ---- the Redis console: a login-style terminal (xterm.js + a small line editor). One session per
// connection keeps its scrollback and history while you move around. Commands go to the server one by one.
const RCMDS = ('APPEND BITCOUNT BITPOS DBSIZE DECR DECRBY DEL DUMP ECHO EXISTS EXPIRE EXPIREAT EXPIRETIME FLUSHALL FLUSHDB GEOADD GEODIST GEOPOS GET GETBIT GETDEL GETEX GETRANGE GETSET HDEL HEXISTS HGET HGETALL HINCRBY HKEYS HLEN HMGET HMSET HSCAN HSET HSETNX HSTRLEN HVALS INCR INCRBY INCRBYFLOAT INFO KEYS LASTSAVE LINDEX LINSERT LLEN LPOP LPOS LPUSH LPUSHX LRANGE LREM LSET LTRIM MGET MSET MSETNX OBJECT PERSIST PEXPIRE PFADD PFCOUNT PING PTTL RANDOMKEY RENAME RENAMENX RPOP RPOPLPUSH RPUSH RPUSHX SADD SCAN SCARD SDIFF SET SETEX SETNX SETRANGE SINTER SISMEMBER SLOWLOG SMEMBERS SMOVE SPOP SRANDMEMBER SREM SSCAN STRLEN SUNION TIME TOUCH TTL TYPE UNLINK XADD XDEL XINFO XLEN XRANGE XREVRANGE XTRIM ZADD ZCARD ZCOUNT ZINCRBY ZRANGE ZRANGEBYSCORE ZRANK ZREM ZREVRANGE ZREVRANK ZSCAN ZSCORE CLIENT CONFIG MEMORY HELP CLEAR EXIT SELECT').split(' ');
const rsh = new Map(); // connection name -> { el, term, fit }

function makeShell(name) {
  const el = document.createElement('div');
  el.className = 'rterm';
  $('rconsole').append(el);
  const term = new Terminal({ cursorBlink: true, fontSize: 13 });
  const fit = new FitAddon.FitAddon();
  term.loadAddon(fit);
  term.open(el);
  new ResizeObserver(() => el.offsetHeight && fit.fit()).observe(el);
  const sh = { el, term, fit };
  const hkey = `k8ui.rhist.${name}`;
  let hist = [], buf = '', pos = 0, hpos = 0, busy = false, abort = null, queue = Promise.resolve();
  try { hist = JSON.parse(localStorage.getItem(hkey)) || []; } catch {}
  hpos = hist.length;
  const prompt = () => `\x1b[32m${name}\x1b[0m:\x1b[36mdb${rdb()}\x1b[0m> `;
  const redraw = () => term.write('\r\x1b[K' + prompt() + buf + (pos < buf.length ? `\x1b[${buf.length - pos}D` : ''));
  const newPrompt = () => { buf = ''; pos = 0; hpos = hist.length; term.write(prompt()); };
  const out = (text) => {
    for (const line of text.split('\n')) {
      const c = line.startsWith('(error)') ? '\x1b[31m' : line.startsWith('(hint)') ? '\x1b[33m' : line.startsWith('(nil)') || line.startsWith('(empty') ? '\x1b[90m' : '';
      term.write(c + line + (c ? '\x1b[0m' : '') + '\r\n');
    }
  };
  const common = (xs) => xs.reduce((a, b) => { let i = 0; while (i < a.length && i < b.length && a[i] === b[i]) i++; return a.slice(0, i); });
  const listOut = (xs) => { term.write('\r\n' + xs.slice(0, 40).join('  ') + (xs.length > 40 ? `  … (${xs.length} in all)` : '') + '\r\n'); redraw(); };

  async function complete() {
    const left = buf.slice(0, pos), at = left.lastIndexOf(' ') + 1, tok = left.slice(at);
    let cands = [];
    if (at === 0) cands = RCMDS.filter((c) => c.startsWith(tok.toUpperCase()));
    else if (tok && !/[*?[\\"']/.test(tok)) { // an argument: complete a key name from SCAN
      try {
        const r = await api(`/api/redis/${encodeURIComponent(name)}/keys?${rq({ match: tok + '*', count: 60 })}`);
        cands = r.keys.map((k) => k.key).filter((k) => k.startsWith(tok)).sort();
      } catch {}
    }
    if (!cands.length) return;
    const fill = cands.length === 1 ? cands[0] + ' ' : common(cands);
    const word = at === 0 && tok === tok.toLowerCase() ? fill.toLowerCase() : fill;
    if (word.length > tok.length) { buf = buf.slice(0, at) + word + buf.slice(pos); pos = at + word.length; redraw(); }
    else if (cands.length > 1) listOut(cands);
  }

  async function submit() {
    term.write('\r\n');
    const cmd = buf.trim();
    if (!cmd) return newPrompt();
    if (hist[hist.length - 1] !== cmd) hist.push(cmd);
    hist = hist.slice(-100);
    try { localStorage.setItem(hkey, JSON.stringify(hist)); } catch {}
    const word = cmd.split(/\s+/)[0].toLowerCase();
    const sel = cmd.match(/^select\s+(\d+)$/i); // SELECT never reaches the server (shared connections): it ticks that db in the filter bar
    if (['exit', 'quit', 'logout'].includes(word)) { term.write('logout\r\n'); buf = ''; return showView('rconns'); }
    if (word === 'clear') { term.clear(); return newPrompt(); }
    if (word === 'help') { out('Commands go to the server one at a time.\nTab completes commands and key names, up/down recall history, Ctrl+L clears, Ctrl+C cancels the line,\nSELECT n switches db, exit (or Ctrl+D on an empty line) goes back to the connections.'); return newPrompt(); }
    if (sel) {
      if (+sel[1] > 1023) out('(error) db index out of range');
      else {
        const have = [...document.querySelectorAll('#rdbdd input')].map((i) => i.value);
        rdbSel.set([String(+sel[1])]);
        rdbSel.options(have, rdbSel.extra); // adds the index to the list when it was not there (db 40, say)
        redisDbsChanged();
        out(`switched to db${+sel[1]}`);
      }
      return newPrompt();
    }
    if (/^(flushall|flushdb|keys|config\s+set)\b/i.test(cmd) && !await askConfirm(`Run ${word.toUpperCase()} on ${name} db${rdb()}?`, { title: 'Run command', ok: 'Run', danger: true })) { term.focus(); out('cancelled'); return newPrompt(); }
    busy = true; abort = new AbortController();
    try {
      const r = await api(`/api/redis/${encodeURIComponent(name)}/command`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, signal: abort.signal, body: JSON.stringify({ db: rdb(), command: cmd }),
      });
      out(r.output);
      toast(`$ ${rcli(rdb(), cmd.length > 120 ? cmd.slice(0, 120) + '…' : cmd)}`);
      if (/^(del|unlink|set|hset|flushdb|flushall|expire|rename|lpush|rpush|sadd|zadd)\b/i.test(cmd)) loadRedisKeys(true); // the key list changed
    } catch (err) { if (err.name !== 'AbortError') out('(refused) ' + err.message); }
    busy = false; abort = null;
    newPrompt();
  }

  async function key(d) {
    if (busy) { if (d === '\x03') { abort?.abort(); term.write('^C\r\n'); } return; }
    switch (d) {
      case '\r': return submit();
      case '\x03': term.write('^C\r\n'); return newPrompt();
      case '\x04': if (!buf) { term.write('logout\r\n'); return showView('rconns'); } return;
      case '\x0c': term.clear(); return redraw();
      case '\t': return complete();
      case '\x7f': if (pos > 0) { buf = buf.slice(0, pos - 1) + buf.slice(pos); pos--; redraw(); } return;
      case '\x1b[3~': if (pos < buf.length) { buf = buf.slice(0, pos) + buf.slice(pos + 1); redraw(); } return;
      case '\x01': case '\x1b[H': case '\x1bOH': pos = 0; return redraw();
      case '\x05': case '\x1b[F': case '\x1bOF': pos = buf.length; return redraw();
      case '\x1b[D': if (pos > 0) { pos--; term.write('\x1b[D'); } return;
      case '\x1b[C': if (pos < buf.length) { pos++; term.write('\x1b[C'); } return;
      case '\x0b': buf = buf.slice(0, pos); return redraw();
      case '\x15': buf = buf.slice(pos); pos = 0; return redraw();
      case '\x17': { const l = buf.slice(0, pos).replace(/\S+\s*$/, ''); buf = l + buf.slice(pos); pos = l.length; return redraw(); }
      case '\x1b[A': case '\x1b[B': {
        hpos = Math.max(0, Math.min(hist.length, hpos + (d === '\x1b[A' ? -1 : 1)));
        buf = hist[hpos] ?? ''; pos = buf.length;
        return redraw();
      }
    }
    if (d.startsWith('\x1b') || d < ' ') return; // other escape sequences and control keys
    buf = buf.slice(0, pos) + d + buf.slice(pos); pos += d.length;
    pos === buf.length ? term.write(d) : redraw();
  }
  // typed keys arrive one at a time; a paste arrives as one chunk: feed it char by char, in order
  term.onData((d) => {
    queue = queue.then(async () => {
      if (d.length > 1 && !d.startsWith('\x1b')) for (const ch of d.replace(/\r?\n/g, '\r')) await key(ch);
      else await key(d);
    });
  });

  const c = ds.redis.conns.find((x) => x.name === name);
  term.write(`\x1b[1m${name}\x1b[0m ${c ? '(' + c.addr + ')' : ''}  Tab completes, ↑↓ history, "help" for more, "exit" to leave\r\n`);
  term.write('connecting… ');
  api(`/api/redis/${encodeURIComponent(name)}/command`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ db: rdb(), command: 'PING' }) })
    .then(() => term.write('\x1b[32mconnected\x1b[0m\r\n'), (e) => term.write(`\x1b[31m${e.message}\x1b[0m\r\n`))
    .finally(newPrompt);
  return sh;
}
function showConsole() { // the session of the open connection; the others stay alive, hidden
  const name = rsrv();
  if (!rsh.has(name)) rsh.set(name, makeShell(name));
  rsh.forEach((sh, n) => (sh.el.hidden = n !== name));
  const sh = rsh.get(name);
  sh.fit.fit();
  sh.term.focus();
}
