'use strict';

// The token arrives in the address and never leaves this page. A local port is
// not a secret — any site the browser visits can try to reach it — so the
// server asks for this on every call, and a cross-origin script cannot read it.
//
// It is read on every use rather than captured once: changing only the fragment
// does not reload the document, so a token pasted into an already-open tab
// would otherwise never be seen.
function token() {
  const fromHash = new URLSearchParams(location.hash.slice(1)).get('t');
  try {
    if (fromHash) sessionStorage.setItem('pixelita-token', fromHash);
    return fromHash || sessionStorage.getItem('pixelita-token') || '';
  } catch (e) {
    return fromHash || '';
  }
}

const $ = (id) => document.getElementById(id);

// Which files an operation has anything to say about. Applying the JPEG
// optimiser to a PNG is not an error worth reporting, it is a question that
// should not have been asked.
const HANDLES = {
  quant: ['.png'],
  webp: ['.png', '.jpg', '.jpeg'],
  jpeg: ['.jpg', '.jpeg'],
  resize: ['.png', '.jpg', '.jpeg', '.gif', '.webp'],
};

const OP_NAMES = { quant: 'палитра', webp: 'webp', jpeg: 'jpeg', resize: 'размер' };

const state = {
  hideIdle: false,
  path: '',
  root: '',
  op: 'quant',
  items: new Map(),   // path -> report item
  selected: new Set(),
  current: null,      // path shown in the inspector
  run: null,          // AbortController while a run is going
  dryRun: true,
  recursive: false,
  pick: '',           // where the folder picker is standing
  opts: {
    quant: { colors: 256, dither: 1, effort: 6 },
    webp: { quality: 90, mode: 'lossy' },
    jpeg: {},
    resize: { maxSide: 1920 },
  },
};

const handles = (path, op) => HANDLES[op].some((e) => path.toLowerCase().endsWith(e));

// --- talking to the server -------------------------------------------------

async function api(path, init = {}) {
  const headers = Object.assign({ 'X-Pixelita-Token': token() }, init.headers || {});
  const res = await fetch(path, Object.assign({}, init, { headers }));
  // A refused token is the one failure a person cannot guess at: the page looks
  // normal and nothing happens. Say it plainly instead.
  if (res.status === 403) {
    running(false, 'ключ не принят — откройте ссылку из терминала заново');
  }
  return res;
}

// ndjson turns a streaming response into an async iterator of objects. The
// point of streaming is that a folder of three hundred photographs takes
// minutes to measure honestly, and a progress bar that only moves at the end is
// not a progress bar.
async function* ndjson(res) {
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    let cut;
    while ((cut = buffer.indexOf('\n')) >= 0) {
      const line = buffer.slice(0, cut).trim();
      buffer = buffer.slice(cut + 1);
      if (line) yield JSON.parse(line);
    }
  }
}

// --- formatting ------------------------------------------------------------

function size(n) {
  if (!n) return '—';
  const unit = 1024;
  if (n < unit) return `${n} B`;
  const names = ['КБ', 'МБ', 'ГБ'];
  let div = unit, i = 0;
  while (n / div >= unit && i < names.length - 1) { div *= unit; i++; }
  return `${(n / div).toFixed(1)} ${names[i]}`;
}

const pct = (v) => `${v > 0 ? '−' : '+'}${Math.abs(Math.round(v))}%`;
const num = (v) => (v == null ? '' : v.toLocaleString('ru-RU'));

// --- the folder tree -------------------------------------------------------

async function loadTree() {
  const tree = $('tree');
  tree.innerHTML = '';
  await addLevel(tree, '', 0, null);
  selectFolder('');
}

async function addLevel(host, path, depth, afterNode) {
  const res = await api(`/api/dirs?path=${encodeURIComponent(path)}`);
  if (!res.ok) return;
  const data = await res.json();

  if (depth === 0) {
    state.root = data.rootPath || '';
    $('aboutRoot').textContent = state.root || '—';
    host.appendChild(folderNode(
      { name: data.root, path: '', images: data.images, children: data.dirs.length > 0 }, 0));
  }
  let anchor = afterNode || host.lastChild;
  for (const d of data.dirs) {
    const node = folderNode(d, depth + 1);
    anchor.after(node);
    anchor = node;
  }
}

function folderNode(d, depth) {
  const el = document.createElement('div');
  el.className = 'inst-tree-item';
  el.setAttribute('role', 'treeitem');
  el.setAttribute('aria-level', String(depth + 1));
  el.tabIndex = -1;
  // KIT GAP 6 — the tree wants its depth as an inline custom property, which is
  // the one place in the whole kit that forces an inline style on generated
  // markup. See docs/instrument.md.
  el.style.setProperty('--depth', depth);
  el.dataset.path = d.path;
  el.dataset.depth = depth;

  if (d.children) {
    const twist = document.createElement('span');
    twist.className = 'inst-tree-twist';
    el.appendChild(twist);
    el.setAttribute('aria-expanded', 'false');
  }
  el.append(d.name || '/');
  if (d.images) {
    const b = document.createElement('span');
    b.className = 'inst-badge inst-nav-count';
    b.textContent = d.images;
    el.append(' ', b);
  }

  el.addEventListener('click', async () => {
    if (d.children) {
      const open = el.getAttribute('aria-expanded') === 'true';
      el.setAttribute('aria-expanded', String(!open));
      if (open) {
        let n = el.nextElementSibling;
        while (n && Number(n.dataset.depth) > depth) {
          const next = n.nextElementSibling;
          n.remove();
          n = next;
        }
      } else {
        await addLevel($('tree'), d.path, depth, el);
      }
    }
    selectFolder(d.path);
  });
  return el;
}

function selectFolder(path) {
  state.path = path;
  for (const el of $('tree').children) {
    el.toggleAttribute('aria-selected', el.dataset.path === path);
  }
  renderCrumbs();
  state.items.clear();
  state.selected.clear();
  state.current = null;
  renderRows();
  renderInspector();
  rescan();
}

function renderCrumbs() {
  const parts = state.path ? state.path.split('/') : [];
  const html = ['<li><a href="#" data-go="">/</a></li>'];
  let acc = '';
  parts.forEach((p, i) => {
    acc = acc ? `${acc}/${p}` : p;
    html.push(i === parts.length - 1
      ? `<li><span aria-current="page">${p}</span></li>`
      : `<li><a href="#" data-go="${acc}">${p}</a></li>`);
  });
  $('crumbs').innerHTML = html.join('');
  for (const a of $('crumbs').querySelectorAll('[data-go]')) {
    a.onclick = (e) => { e.preventDefault(); selectFolder(a.dataset.go); };
  }
}

// --- choosing a folder anywhere on the machine -----------------------------

async function browse(path) {
  const res = await api(`/api/browse?path=${encodeURIComponent(path || '')}`);
  if (!res.ok) return;
  const data = await res.json();
  state.pick = data.path;

  $('pickPath').textContent = data.path || 'Диски';
  $('pickCount').textContent = data.path ? `картинок здесь: ${data.images || 0}` : '';
  $('pickChoose').disabled = !data.path;

  const list = $('pickList');
  list.innerHTML = '';
  if (data.path) {
    list.appendChild(pickRow('..', () => browse(data.up), 0));
  }
  for (const d of data.dirs) {
    list.appendChild(pickRow(d.name, () => browse(d.path), d.images));
  }
}

function pickRow(name, go, images) {
  const el = document.createElement('div');
  el.className = 'inst-tree-item';
  el.setAttribute('role', 'treeitem');
  el.tabIndex = -1;
  el.append(name);
  if (images) {
    const b = document.createElement('span');
    b.className = 'inst-badge inst-nav-count';
    b.textContent = images;
    el.append(' ', b);
  }
  el.onclick = go;
  return el;
}

async function chooseFolder() {
  if (!state.pick) return;
  const res = await api('/api/root', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path: state.pick }),
  });
  if (!res.ok) {
    running(false, 'не открылось: ' + (await res.text()).trim());
    return;
  }
  $('dlgPick').close();
  await loadTree();
}

// --- the options for the chosen operation ----------------------------------

function renderOptions() {
  const o = state.opts;
  const fields = {
    quant: `
      <label>цветов <input class="inst-input inst-input--sm" type="number" min="2" max="256"
             id="o-colors" value="${o.quant.colors}"></label>
      <label>дизеринг <input type="range" min="0" max="1" step="0.05"
             id="o-dither" value="${o.quant.dither}"></label>
      <label>усилие <input class="inst-input inst-input--sm" type="number" min="1" max="10"
             id="o-effort" value="${o.quant.effort}"></label>`,
    webp: `
      <label>качество <input class="inst-input inst-input--sm" type="number" min="1" max="100"
             id="o-quality" value="${o.webp.quality}"></label>
      <label>режим <span class="inst-select-wrap">
        <select class="inst-select inst-select--sm" id="o-mode">
          <option value="lossy">с потерями</option>
          <option value="lossless">без потерь</option>
          <option value="near-lossless">почти без потерь</option>
        </select></span></label>`,
    jpeg: `<span class="inst-toolbar-label">без потерь — настраивать нечего</span>`,
    resize: `
      <label>не шире <input class="inst-input inst-input--sm" type="number" min="1" max="20000"
             id="o-maxside" value="${o.resize.maxSide}"></label>`,
  }[state.op];

  $('options').innerHTML = `${fields}
    <span class="inst-toolbar-spacer"></span>
    <button class="inst-btn inst-btn--sm inst-btn--primary" type="button" id="apply">
      ${state.dryRun ? 'Измерить' : 'Применить'}</button>`;

  if (state.op === 'webp') $('o-mode').value = o.webp.mode;

  const bind = (id, set) => {
    const el = $(id);
    if (el) el.oninput = () => { set(el.value); refreshCandidate(); };
  };
  bind('o-colors', (v) => (o.quant.colors = +v));
  bind('o-dither', (v) => (o.quant.dither = +v));
  bind('o-effort', (v) => (o.quant.effort = +v));
  bind('o-quality', (v) => (o.webp.quality = +v));
  bind('o-mode', (v) => (o.webp.mode = v));
  bind('o-maxside', (v) => (o.resize.maxSide = +v));
  $('apply').onclick = applyToSelection;
  renderSelection();
}

// --- the table -------------------------------------------------------------

function renderRows() {
  const tbody = $('rows');
  const filter = $('filter').value.trim().toLowerCase();
  tbody.innerHTML = '';

  let shown = 0;
  for (const [path, it] of state.items) {
    if (filter && !path.toLowerCase().includes(filter)) continue;
    if (state.hideIdle && !handles(path, state.op)) continue;
    tbody.appendChild(rowFor(path, it));
    shown++;
  }
  $('empty').dataset.show = shown ? '' : '1';
  renderSelection();
}

function rowFor(path, it) {
  const m = it.metrics || {};
  const tr = document.createElement('tr');
  if (path === state.current) tr.setAttribute('aria-selected', 'true');
  if (!handles(path, state.op)) tr.classList.add('app-row-idle');

  const best = m.best || '';
  const outcome = it.status === 'failed'
    ? `<span class="inst-tag" data-tone="error">ошибка</span>`
    : best
      ? `<span class="inst-tag" data-tone="${best === 'quant' ? 'ok' : 'info'}">${best}</span>`
      : it.status === 'done' || it.status === 'would'
        ? `<span class="inst-tag" data-tone="ok">${it.status === 'done' ? 'записан' : 'готов'}</span>`
        : `<span class="inst-tag">${it.reason || '—'}</span>`;

  const colours = m.colours == null ? '' : num(m.colours) + (m.coloursExact === false ? '+' : '');

  tr.innerHTML = `
    <td class="inst-col-select"><label class="inst-checkbox">
      <input type="checkbox" ${state.selected.has(path) ? 'checked' : ''} aria-label="Выбрать ${path}"></label></td>
    <td><img class="app-thumb" loading="lazy" alt=""
             src="/api/thumb?path=${encodeURIComponent(path)}&w=120&t=${token()}"></td>
    <td class="app-name"><span class="inst-u-truncate" title="${path}">${path.split('/').pop()}</span></td>
    <td class="inst-num">${size(it.bytesBefore)}</td>
    <td>${m.colourType || ''}</td>
    <td class="inst-num">${m.size || ''}</td>
    <td class="inst-num">${colours}</td>
    <td>${outcome}</td>
    <td class="inst-num" ${it.gainPercent > 0 ? 'data-tone="ok"' : ''}>${
      it.bytesAfter ? pct(it.gainPercent) : ''}</td>`;

  tr.querySelector('input').onchange = (e) => {
    e.target.checked ? state.selected.add(path) : state.selected.delete(path);
    renderSelection();
  };
  tr.onclick = (e) => {
    if (e.target.closest('.inst-col-select')) return;
    state.current = path;
    renderRows();
    renderInspector();
  };
  return tr;
}

function renderSelection() {
  const n = state.selected.size;
  const fit = [...state.selected].filter((p) => handles(p, state.op)).length;
  const info = $('selInfo');

  if (!n) {
    info.textContent = 'ничего не выбрано';
    info.dataset.tone = '';
  } else if (fit === n) {
    info.textContent = `${n} выбрано`;
    info.dataset.tone = 'ok';
  } else {
    // Saying so beforehand is better than a run that quietly does less than the
    // count promised.
    info.textContent = `${n} выбрано · ${fit} подойдут`;
    info.dataset.tone = fit ? 'warn' : 'error';
  }

  $('selAll').checked = n > 0 && n === state.items.size;
  if ($('apply')) $('apply').disabled = fit === 0;
}

// --- the inspector ---------------------------------------------------------

function renderInspector() {
  const path = state.current;
  const body = $('insBody');
  if (!path) {
    $('insTitle').textContent = 'Инспектор';
    $('insTag').innerHTML = '';
    body.innerHTML = `<div class="inst-empty"><div class="inst-empty-title">Файл не выбран</div></div>`;
    return;
  }

  const it = state.items.get(path) || {};
  const m = it.metrics || {};
  $('insTitle').textContent = path.split('/').pop();
  $('insTag').innerHTML = m.best ? `<span class="inst-badge" data-tone="ok">${m.best}</span>` : '';

  const variants = [];
  if (m.quantBytes) variants.push(['квантизация', m.quantBytes, m.quantGain, m.quantPSNR]);
  if (m.webpBytes) variants.push(['webp', m.webpBytes, m.webpGain, null]);

  body.innerHTML = `
    <div class="app-shot" id="shot" title="Клик — сравнить">
      <img src="/api/thumb?path=${encodeURIComponent(path)}&w=560&t=${token()}" alt="Исходник">
      <img id="shotAfter" alt="Результат">
      <span class="app-shot-tag" id="shotTag">исходник</span>
    </div>

    <div class="inst-metric-row">
      <div class="inst-metric">
        <div class="inst-metric-label">Размер</div>
        <div class="inst-metric-value" id="mSize">${size(it.bytesBefore)}</div>
        <div class="inst-metric-delta" id="mDelta"></div>
      </div>
      <div class="inst-metric">
        <div class="inst-metric-label">Точность</div>
        <div class="inst-metric-value" id="mPsnr">—</div>
        <div class="inst-metric-delta" id="mPsnrNote"></div>
      </div>
    </div>

    <div class="inst-section">
      <div class="inst-section-head"><span class="inst-section-title">Разбор</span></div>
      <dl class="inst-kv">
        <dt>Формат</dt><dd>${m.format || '—'}${m.colourType ? ', ' + m.colourType : ''}</dd>
        <dt>Пиксели</dt><dd>${m.size || '—'}</dd>
        <dt>Цветов</dt><dd>${m.colours != null ? num(m.colours) + (m.coloursExact === false ? '+' : '') : '—'}</dd>
        <dt>Прозрачность</dt><dd>${m.hasAlpha ? 'есть' : 'нет'}</dd>
        <dt>Метаданные</dt><dd>${m.metadataBytes ? size(m.metadataBytes) : 'нет'}</dd>
      </dl>
    </div>

    ${variants.length ? `<div class="inst-section">
      <div class="inst-section-head"><span class="inst-section-title">Варианты</span></div>
      <table class="inst-table"><tbody>${variants.map(([name, bytes, gain, psnr]) => `
        <tr${m.best && name.startsWith(m.best) ? ' aria-selected="true"' : ''}>
          <td>${name}</td>
          <td class="inst-num">${size(bytes)}</td>
          <td class="inst-num" ${gain > 0 ? 'data-tone="ok"' : 'data-tone="error"'}>${pct(gain)}</td>
          <td class="inst-num">${psnr ? psnr.toFixed(1) + ' дБ' : ''}</td>
        </tr>`).join('')}</tbody></table></div>` : ''}`;

  const shot = $('shot');
  shot.onclick = () => {
    shot.dataset.show = shot.dataset.show === 'after' ? '' : 'after';
    $('shotTag').textContent = shot.dataset.show === 'after' ? 'результат' : 'исходник';
  };
  refreshCandidate();
}

// refreshCandidate asks the server for what the chosen operation would actually
// produce. It is the same encoder that would write the file, so the picture
// behind the blink is the file, not an impression of it.
let candidateSeq = 0;
async function refreshCandidate() {
  const path = state.current;
  const after = $('shotAfter');
  if (!path || !after) return;
  const seq = ++candidateSeq;

  if (!handles(path, state.op)) {
    after.removeAttribute('src');
    $('mPsnr').textContent = '—';
    $('mPsnrNote').textContent = 'не для этого файла';
    return;
  }

  const o = state.opts;
  const params = new URLSearchParams({ path, op: state.op });
  if (state.op === 'quant') {
    params.set('colors', o.quant.colors);
    params.set('dither', o.quant.dither);
    params.set('effort', o.quant.effort);
  } else if (state.op === 'webp') {
    params.set('quality', o.webp.quality);
    params.set('mode', o.webp.mode);
  } else if (state.op === 'resize') {
    params.set('width', o.resize.maxSide);
  }

  const res = await api(`/api/candidate?${params}`);
  if (!res.ok || seq !== candidateSeq) return;

  const before = Number(res.headers.get('X-Bytes-Before')) || 0;
  const bytes = Number(res.headers.get('X-Bytes-After')) || 0;
  const psnr = res.headers.get('X-PSNR');
  const blob = await res.blob();
  if (seq !== candidateSeq) return;

  after.src = URL.createObjectURL(blob);
  $('mSize').textContent = size(bytes);
  if (before) {
    const gain = (1 - bytes / before) * 100;
    const d = $('mDelta');
    d.textContent = `${pct(gain)} от ${size(before)}`;
    d.dataset.tone = gain > 0 ? 'ok' : 'error';
    d.dataset.dir = gain > 0 ? 'down' : 'up';
  }
  $('mPsnr').textContent = psnr ? `${Number(psnr).toFixed(1)} дБ` : 'без потерь';
  $('mPsnrNote').textContent = psnr ? '' : 'пиксели не изменились';
}

// --- runs ------------------------------------------------------------------

function running(on, label) {
  $('stState').innerHTML = on
    ? `<svg class="inst-spinner" viewBox="0 0 16 16" aria-hidden="true"><circle class="inst-spinner-track" cx="8" cy="8" r="6.5"/><circle class="inst-spinner-arc" cx="8" cy="8" r="6.5"/></svg>${label}`
    : `<svg class="inst-icon" aria-hidden="true"><use href="#i-check"/></svg>${label}`;
  $('stState').dataset.tone = on ? 'info' : 'ok';
  $('stStop').hidden = !on;
  $('rescan').disabled = on;
}

async function consume(res, total) {
  let done = 0;
  for await (const msg of ndjson(res)) {
    if (msg.type === 'start') {
      total = msg.files;
    } else if (msg.type === 'item') {
      state.items.set(msg.item.path, msg.item);
      if (msg.item.status === 'would' || msg.item.status === 'done') {
        state.selected.add(msg.item.path);
      }
      done = msg.done;
      running(true, `${done} из ${total}`);
      renderRows();
    } else if (msg.type === 'done') {
      showSummary(msg.summary);
    }
  }
}

// potential says what each tool would save across the whole set. The server
// sends the same figures as English notes for the command line; the interface
// speaks its own language and builds them from the items it already holds.
function potential() {
  const totals = { quant: [0, 0], webp: [0, 0] };
  for (const it of state.items.values()) {
    const m = it.metrics || {};
    for (const [key, bytes] of [['quant', m.quantBytes], ['webp', m.webpBytes]]) {
      if (!bytes || bytes >= it.bytesBefore) continue;
      totals[key][0]++;
      totals[key][1] += it.bytesBefore - bytes;
    }
  }
  return Object.entries(totals)
    .filter(([, [n]]) => n > 0)
    .map(([name, [n, saved]]) => `${name === 'quant' ? 'палитра' : 'webp'}: ${n} → −${size(saved)}`)
    .join(' · ');
}

function showSummary(s) {
  $('stFiles').textContent = `${s.files} файлов`;
  $('stSize').textContent = s.bytesBefore ? `${size(s.bytesBefore)} → ${size(s.bytesAfter)}` : '';
  $('stSaved').textContent = s.bytesBefore
    ? `сэкономлено ${size(s.bytesBefore - s.bytesAfter)} (${Math.round(s.gainPercent)}%)` : '';
  $('stNotes').textContent = potential();
  $('folderInfo').textContent = `${s.files} файлов · ${size(s.bytesBefore || 0)}`;
}

async function rescan() {
  if (state.run) state.run.abort();
  state.run = new AbortController();
  state.items.clear();
  state.selected.clear();
  renderRows();
  running(true, 'смотрю');

  try {
    const q = new URLSearchParams({ path: state.path });
    if (state.recursive) q.set('recursive', '1');
    const res = await api(`/api/scan?${q}`, { signal: state.run.signal });
    if (!res.ok) throw new Error(await res.text());
    await consume(res, 0);
    running(false, 'готово');
  } catch (e) {
    if (e.name !== 'AbortError') running(false, 'ошибка: ' + e.message);
  } finally {
    state.run = null;
  }
}

async function applyToSelection() {
  const paths = [...state.selected].filter((p) => handles(p, state.op));
  if (!paths.length || state.run) return;
  state.run = new AbortController();
  running(true, 'работаю');

  const o = state.opts;
  const body = Object.assign(
    { op: state.op, paths, dryRun: state.dryRun, minGain: 0, minPSNR: 0 },
    state.op === 'quant' ? o.quant
      : state.op === 'webp' ? o.webp
        : state.op === 'resize' ? { maxSide: o.resize.maxSide } : {});

  try {
    const res = await api('/api/apply', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      signal: state.run.signal,
    });
    if (!res.ok) throw new Error(await res.text());
    await consume(res, paths.length);
    running(false, state.dryRun ? 'измерено' : 'записано');
  } catch (e) {
    if (e.name !== 'AbortError') running(false, 'ошибка: ' + e.message);
  } finally {
    state.run = null;
  }
}

// --- wiring ----------------------------------------------------------------

function toggleMenuItem(el, get, set) {
  el.setAttribute('aria-checked', String(get()));
  el.onclick = () => {
    set(!get());
    el.setAttribute('aria-checked', String(get()));
    renderOptions();
    renderRows();
  };
}

function setOp(op) {
  state.op = op;
  for (const b of $('ops').children) {
    const on = b.dataset.op === op;
    b.setAttribute('aria-checked', String(on));
    b.tabIndex = on ? 0 : -1;
  }
  for (const b of document.querySelectorAll('[data-set-op]')) {
    b.setAttribute('aria-checked', String(b.dataset.setOp === op));
  }
  renderOptions();
  renderRows();
  renderInspector();
}

function select(which) {
  const all = [...state.items.keys()];
  state.selected = new Set(
    which === 'all' ? all
      : which === 'none' ? []
        : all.filter((p) => !state.selected.has(p)));
  renderRows();
}

function init() {
  for (const b of $('ops').children) {
    b.onclick = () => setOp(b.dataset.op);
  }
  for (const b of document.querySelectorAll('[data-set-op]')) {
    b.onclick = () => setOp(b.dataset.setOp);
  }
  $('filter').oninput = renderRows;
  $('rescan').onclick = rescan;
  $('selAll').onchange = (e) => {
    state.selected = e.target.checked ? new Set(state.items.keys()) : new Set();
    renderRows();
  };
  $('stStop').onclick = () => state.run && state.run.abort();

  const openPicker = () => { $('dlgPick').showModal(); browse(state.root); };
  $('pick').onclick = openPicker;
  $('mnOpen').onclick = openPicker;
  $('pickChoose').onclick = chooseFolder;
  $('mnRescan').onclick = rescan;
  $('mnApply').onclick = applyToSelection;
  $('mnStop').onclick = () => state.run && state.run.abort();
  $('mnAll').onclick = () => select('all');
  $('mnNone').onclick = () => select('none');
  $('mnInvert').onclick = () => select('invert');
  $('mnAbout').onclick = () => $('dlgAbout').showModal();
  $('mnQuit').onclick = async () => {
    await api('/api/quit', { method: 'POST' });
    document.body.innerHTML =
      '<div class="inst-empty"><div class="inst-empty-title">Сервер остановлен</div>' +
      '<div class="inst-empty-desc">Вкладку можно закрыть.</div></div>';
  };
  for (const b of document.querySelectorAll('[data-close]')) {
    b.onclick = () => b.closest('dialog').close();
  }

  for (const b of document.querySelectorAll('[data-theme-set]')) {
    b.onclick = () => (document.documentElement.dataset.theme = b.dataset.themeSet);
  }
  toggleMenuItem($('mnDry'), () => state.dryRun, (v) => (state.dryRun = v));
  toggleMenuItem($('mnRecursive'), () => state.recursive, (v) => (state.recursive = v));
  toggleMenuItem($('mnHideIdle'), () => state.hideIdle, (v) => (state.hideIdle = v));

  document.addEventListener('keydown', (e) => {
    const typing = /^(INPUT|TEXTAREA|SELECT)$/.test(e.target.tagName);
    if (e.key === 'F5') { e.preventDefault(); rescan(); return; }
    if (e.ctrlKey && e.key === 'o') { e.preventDefault(); openPicker(); return; }
    if (typing || document.querySelector('dialog[open]')) return;
    if (e.ctrlKey && e.key === 'a') { e.preventDefault(); select('all'); }
    if (e.key === 'Enter') applyToSelection();
    if (e.key === 'Escape' && state.run) state.run.abort();
  });

  renderOptions();
  loadTree();

  // The sprite has to live in the document: <use href="#i-…"> obeys the
  // same-origin rule and a reference to an external file silently draws
  // nothing.
  fetch('vendor/sprite.svg').then((r) => r.text()).then((svg) => {
    const holder = document.createElement('div');
    holder.hidden = true;
    holder.innerHTML = svg;
    document.body.prepend(holder);
  });
}

init();
