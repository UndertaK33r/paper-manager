
var state = {
  page: 1, pageSize: 20, search: '', category: '', tag: '', collection: '',
  yearFrom: '', yearTo: '', read: '', starred: '', sort: 'created', order: 'desc',
  view: 'table', total: 0, pages: 1, papers: [],
  categories: [], tags: [], collections: [], detail: null, editingId: null
};

function $(id) { return document.getElementById(id); }

function esc(s) {
  if (s === null || s === undefined) return '';
  return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

function toast(msg, isError) {
  var t = $('toast');
  t.textContent = msg;
  t.className = 'toast' + (isError ? ' error' : '');
  clearTimeout(toast._timer);
  toast._timer = setTimeout(function () { t.className = 'toast hidden'; }, 2600);
}

function show(id) { $(id).classList.remove('hidden'); }
function hide(id) { $(id).classList.add('hidden'); }

async function api(path, opts) {
  opts = opts || {};
  opts.headers = opts.headers || {};
  if (opts.json) {
    opts.headers['Content-Type'] = 'application/json';
    opts.body = JSON.stringify(opts.json);
    delete opts.json;
  }
  var res = await fetch(path, opts);
  var data = null;
  try { data = await res.json(); } catch (e) { /* no body */ }
  if (!res.ok) {
    var err = new Error((data && data.error) || ('请求失败 ' + res.status));
    err.status = res.status;
    err.data = data;
    throw err;
  }
  return data;
}

function queryString() {
  var q = [];
  function add(k, v) { if (v !== null && v !== undefined && v !== '') q.push(k + '=' + encodeURIComponent(v)); }
  add('page', state.page); add('pageSize', state.pageSize); add('search', state.search);
  add('category', state.category); add('tags', state.tag); add('collections', state.collection);
  add('yearFrom', state.yearFrom); add('yearTo', state.yearTo);
  add('read', state.read); add('starred', state.starred);
  add('sort', state.sort); add('order', state.order);
  return '/api/papers?' + q.join('&');
}

async function loadAll() {
  try {
    var results = await Promise.all([
      api('/api/categories'), api('/api/tags'), api('/api/collections'), api('/api/stats')
    ]);
    state.categories = results[0]; state.tags = results[1]; state.collections = results[2];
    renderSelects(); renderStats(results[3]);
  } catch (e) { toast(e.message, true); }
}

function renderStats(s) {
  var html = '';
  html += '<div class="stat"><div class="num">' + esc(s.total) + '</div><div class="label">论文总数</div></div>';
  html += '<div class="stat"><div class="num">' + esc(s.unread) + '</div><div class="label">未读</div></div>';
  html += '<div class="stat"><div class="num">' + esc(s.read) + '</div><div class="label">已读</div></div>';
  html += '<div class="stat"><div class="num">' + esc(s.starred) + '</div><div class="label">收藏</div></div>';
  html += '<div class="stat"><div class="num">' + esc(s.categories) + '</div><div class="label">分类</div></div>';
  $('stats').innerHTML = html;
}

function renderSelects() {
  var cf = $('categoryFilter');
  var html = '<option value="">全部分类</option>';
  for (var i = 0; i < state.categories.length; i++) {
    var c = state.categories[i];
    html += '<option value="' + c.id + '">' + esc(c.name) + ' (' + c.paperCount + ')</option>';
  }
  cf.innerHTML = html;
  cf.value = state.category;

  var tf = $('tagFilter');
  var th = '<option value="">全部标签</option>';
  for (var j = 0; j < state.tags.length; j++) {
    var t = state.tags[j];
    th += '<option value="' + t.id + '">' + esc(t.name) + ' (' + t.paperCount + ')</option>';
  }
  tf.innerHTML = th; tf.value = state.tag;

  var cof = $('collectionFilter');
  var oh = '<option value="">全部合集</option>';
  for (var k = 0; k < state.collections.length; k++) {
    var o = state.collections[k];
    oh += '<option value="' + o.id + '">' + esc(o.name) + ' (' + o.paperCount + ')</option>';
  }
  cof.innerHTML = oh; cof.value = state.collection;

  var fcat = $('fCategory');
  var fh = '<option value="">未分类</option>';
  for (var m = 0; m < state.categories.length; m++) {
    var cc = state.categories[m];
    fh += '<option value="' + cc.id + '">' + esc(cc.name) + '</option>';
  }
  fcat.innerHTML = fh;
}

async function loadPapers() {
  try {
    var res = await api(queryString());
    state.total = res.total;
    state.page = res.page;
    state.papers = res.papers || [];
    state.pages = Math.max(1, Math.ceil(res.total / res.pageSize));
    renderPapers();
    renderPagination();
  } catch (e) { toast(e.message, true); }
}

function statusBadges(p) {
  var h = '';
  if (p.read) h += '<span class="badge read">已读</span> ';
  if (p.starred) h += '<span class="badge star">★ 收藏</span> ';
  return h;
}

function tagChips(p) {
  var h = '';
  for (var i = 0; i < (p.tags || []).length; i++) {
    h += '<span class="chip">' + esc(p.tags[i].name) + '</span> ';
  }
  return h;
}

function actionButtons(p) {
  var h = '';
  h += '<button class="btn small" onclick="openDetail(' + p.id + ')">详情</button> ';
  h += '<button class="btn small" onclick="openEditById(' + p.id + ')">编辑</button> ';
  h += '<button class="btn small" onclick="toggleRead(' + p.id + ')">' + (p.read ? '标未读' : '标已读') + '</button> ';
  h += '<button class="btn small" onclick="toggleStar(' + p.id + ')">' + (p.starred ? '取消收藏' : '收藏') + '</button> ';
  h += '<button class="btn small danger" onclick="deletePaper(' + p.id + ')">删除</button>';
  return h;
}

function renderPapers() {
  var area = $('papersArea');
  if (!state.papers.length) {
    area.innerHTML = '<div class="empty">暂无论文，点击右上角「添加论文」开始吧</div>';
    return;
  }
  if (state.view === 'card') {
    var h = '<div class="cards">';
    for (var i = 0; i < state.papers.length; i++) {
      var p = state.papers[i];
      h += '<div class="paper-card">';
      h += '<div class="t" onclick="openDetail(' + p.id + ')">' + esc(p.title) + '</div>';
      h += '<div class="muted">' + esc(p.authors || '未知作者') + (p.year ? ' · ' + p.year : '') + '</div>';
      h += '<div>' + statusBadges(p) + (p.categoryName ? '<span class="badge cat">' + esc(p.categoryName) + '</span>' : '') + '</div>';
      h += '<div class="chips">' + tagChips(p) + '</div>';
      if (p.summary) h += '<div class="muted">' + esc(p.summary) + '</div>';
      h += '<div class="actions">' + actionButtons(p) + '</div>';
      h += '</div>';
    }
    h += '</div>';
    area.innerHTML = h;
  } else {
    var t = '<table class="papers"><thead><tr><th>标题</th><th>作者</th><th>年份</th><th>分类</th><th>标签</th><th>状态</th><th>摘要</th><th>操作</th></tr></thead><tbody>';
    for (var j = 0; j < state.papers.length; j++) {
      var pp = state.papers[j];
      t += '<tr>';
      t += '<td class="title-cell" onclick="openDetail(' + pp.id + ')">' + esc(pp.title) + '</td>';
      t += '<td class="ellipsis">' + esc(pp.authors || '') + '</td>';
      t += '<td>' + esc(pp.year || '') + '</td>';
      t += '<td>' + esc(pp.categoryName || '') + '</td>';
      t += '<td class="chips">' + tagChips(pp) + '</td>';
      t += '<td>' + statusBadges(pp) + '</td>';
      t += '<td class="ellipsis">' + esc(pp.summary || '') + '</td>';
      t += '<td><div class="actions">' + actionButtons(pp) + '</div></td>';
      t += '</tr>';
    }
    t += '</tbody></table>';
    area.innerHTML = t;
  }
}

function renderPagination() {
  var h = '';
  h += '<button class="btn small" ' + (state.page <= 1 ? 'disabled' : '') + ' onclick="goPage(' + (state.page - 1) + ')">上一页</button>';
  h += '<span class="muted">第 ' + state.page + ' / ' + state.pages + ' 页 · 共 ' + state.total + ' 篇</span>';
  h += '<button class="btn small" ' + (state.page >= state.pages ? 'disabled' : '') + ' onclick="goPage(' + (state.page + 1) + ')">下一页</button>';
  $('pagination').innerHTML = h;
}

function goPage(p) {
  if (p < 1 || p > state.pages) return;
  state.page = p;
  loadPapers();
}

function fillCategoryOptions(selectEl, selected) {
  var h = '<option value="">未分类</option>';
  for (var i = 0; i < state.categories.length; i++) {
    var c = state.categories[i];
    h += '<option value="' + c.id + '"' + (String(c.id) === String(selected) ? ' selected' : '') + '>' + esc(c.name) + '</option>';
  }
  selectEl.innerHTML = h;
}

function openAdd() {
  state.editingId = null;
  $('paperModalTitle').textContent = '添加论文';
  $('paperForm').reset();
  if ($('fPDF')) $('fPDF').value = '';
  fillCategoryOptions($('fCategory'), '');
  show('paperModal');
}

function openEditById(id) {
  var p = null;
  for (var i = 0; i < state.papers.length; i++) {
    if (state.papers[i].id === id) { p = state.papers[i]; break; }
  }
  if (p) openEdit(p);
}

function openEdit(p) {
  state.editingId = p.id;
  $('paperModalTitle').textContent = '编辑论文';
  $('fTitle').value = p.title || '';
  $('fAuthors').value = p.authors || '';
  $('fYear').value = p.year || '';
  $('fVenue').value = p.venue || '';
  $('fDOI').value = p.doi || '';
  $('fKeywords').value = p.keywords || '';
  $('fLink').value = p.link || '';
  $('fSummary').value = p.summary || '';
  $('fNotes').value = p.notes || '';
  $('fTags').value = (p.tags || []).map(function (t) { return t.name; }).join(', ');
  $('fCollections').value = (p.collections || []).map(function (c) { return c.name; }).join(', ');
  $('fRead').checked = !!p.read;
  $('fStarred').checked = !!p.starred;
  fillCategoryOptions($('fCategory'), p.categoryId);
  show('paperModal');
  hide('detailModal');
}

async function submitPaperForm(ev) {
  ev.preventDefault();
  var fd = new FormData($('paperForm'));
  var file = $('fPDF').files && $('fPDF').files[0];
  if (file) fd.delete('pdf'), fd.append('pdf', file, file.name);
  var url = state.editingId ? '/api/papers/' + state.editingId : '/api/papers';
  var method = state.editingId ? 'PUT' : 'POST';
  try {
    await api(url, { method: method, body: fd });
    hide('paperModal');
    toast('保存成功');
    await loadAll();
    await loadPapers();
  } catch (e) {
    if (e.status === 409 && e.data && e.data.duplicate) {
      var ok = confirm('检测到重复论文：' + e.data.title + '。仍然添加吗？');
      if (ok) {
        fd.append('force', '1');
        try {
          await api(url, { method: method, body: fd });
          hide('paperModal');
          toast('已添加（确认保留重复项）');
          await loadAll();
          await loadPapers();
        } catch (e2) { toast(e2.message, true); }
      }
    } else {
      toast(e.message, true);
    }
  }
}

async function toggleRead(id) {
  try {
    var p = await api('/api/papers/' + id + '/toggle-read', { method: 'POST' });
    if (state.detail && state.detail.id === id) { state.detail = p; fillDetail(p); }
    await loadPapers();
  } catch (e) { toast(e.message, true); }
}

async function toggleStar(id) {
  try {
    var p = await api('/api/papers/' + id + '/toggle-star', { method: 'POST' });
    if (state.detail && state.detail.id === id) { state.detail = p; fillDetail(p); }
    await loadPapers();
  } catch (e) { toast(e.message, true); }
}

async function deletePaper(id) {
  if (!confirm('确定删除这篇论文吗？此操作不可恢复。')) return;
  try {
    await api('/api/papers/' + id, { method: 'DELETE' });
    toast('已删除');
    hide('detailModal');
    await loadAll();
    await loadPapers();
  } catch (e) { toast(e.message, true); }
}

function metaItem(k, v) {
  return '<div class="meta-item"><div class="k">' + k + '</div><div class="v">' + (v ? esc(v) : '<span class="muted">—</span>') + '</div></div>';
}

function fillDetail(p) {
  state.detail = p;
  $('detailTitle').textContent = p.title || '';
  var h = '';
  h += metaItem('作者', p.authors);
  h += metaItem('年份', p.year);
  h += metaItem('期刊/会议', p.venue);
  h += metaItem('DOI', p.doi);
  h += metaItem('关键词', p.keywords);
  h += metaItem('链接', p.link);
  h += metaItem('分类', p.categoryName);
  h += metaItem('已读', p.read ? '是' : '否');
  h += metaItem('收藏', p.starred ? '是' : '否');
  $('detailMeta').innerHTML = h;

  var s = '';
  if (p.summary) { s = '<div class="muted">' + esc(p.summary) + '</div>'; }
  else { s = '<div class="muted">暂无 AI 总结（预留接口，后续版本自动生成）</div>'; }
  $('detailSummary').innerHTML = '<h3>总结</h3>' + s;

  var a = '';
  a += '<button class="btn small" onclick="toggleRead(' + p.id + ')">' + (p.read ? '标为未读' : '标为已读') + '</button> ';
  a += '<button class="btn small" onclick="toggleStar(' + p.id + ')">' + (p.starred ? '取消收藏' : '加入收藏') + '</button> ';
  a += '<button class="btn small" onclick="openEdit(state.detail)">编辑</button> ';
  a += '<a class="btn small" target="_blank" href="/api/papers/' + p.id + '/pdf" ' + (p.hasPdf ? '' : 'style="display:none"') + '>新窗口打开</a> ';
  a += '<a class="btn small" target="_blank" href="/api/papers/' + p.id + '/pdf?download=1" ' + (p.hasPdf ? '' : 'style="display:none"') + '>下载 PDF</a> ';
  a += '<button class="btn small danger" onclick="deletePaper(' + p.id + ')">删除</button>';
  $('detailActions').innerHTML = a;

  var pdf = '';
  if (p.hasPdf) {
    pdf = '<iframe src="/api/papers/' + p.id + '/pdf" title="PDF 预览"></iframe>';
  } else {
    pdf = '<div class="empty">该论文暂无 PDF 文件，可在编辑时上传</div>';
  }
  $('pdfWrap').innerHTML = pdf;

  var th = '';
  for (var i = 0; i < (p.tags || []).length; i++) {
    th += '<span class="chip x" onclick="removeDetailTag(' + p.tags[i].id + ')">' + esc(p.tags[i].name) + ' ×</span> ';
  }
  $('tagChips').innerHTML = th || '<span class="muted">暂无标签</span>';

  var ch = '';
  for (var j = 0; j < (p.collections || []).length; j++) {
    ch += '<span class="chip coll x" onclick="removeDetailCollection(' + p.collections[j].id + ')">' + esc(p.collections[j].name) + ' ×</span> ';
  }
  $('collectionChips').innerHTML = ch || '<span class="muted">暂无合集</span>';

  var ds = $('detailTagSelect');
  var dh = '<option value="">选择标签</option>';
  for (var k = 0; k < state.tags.length; k++) {
    var t = state.tags[k];
    var exists = (p.tags || []).some(function (x) { return x.id === t.id; });
    if (!exists) dh += '<option value="' + t.id + '">' + esc(t.name) + '</option>';
  }
  ds.innerHTML = dh;

  var cs = $('detailCollectionSelect');
  var oh = '<option value="">选择合集</option>';
  for (var m = 0; m < state.collections.length; m++) {
    var c = state.collections[m];
    var inIt = (p.collections || []).some(function (x) { return x.id === c.id; });
    if (!inIt) oh += '<option value="' + c.id + '">' + esc(c.name) + '</option>';
  }
  cs.innerHTML = oh;

  $('detailNotes').value = p.notes || '';
}

async function openDetail(id) {
  try {
    var p = await api('/api/papers/' + id);
    fillDetail(p);
    show('detailModal');
    loadAll();
  } catch (e) { toast(e.message, true); }
}

async function addDetailTag() {
  var sel = $('detailTagSelect');
  var id = Number(sel.value);
  if (!id || !state.detail) return;
  try {
    var p = await api('/api/papers/' + state.detail.id + '/tags', { method: 'POST', json: { tagId: id } });
    fillDetail(p);
  } catch (e) { toast(e.message, true); }
}

async function removeDetailTag(tagId) {
  if (!state.detail) return;
  try {
    var p = await api('/api/papers/' + state.detail.id + '/tags/' + tagId, { method: 'DELETE' });
    fillDetail(p);
  } catch (e) { toast(e.message, true); }
}

async function addDetailCollection() {
  var sel = $('detailCollectionSelect');
  var id = Number(sel.value);
  if (!id || !state.detail) return;
  try {
    var p = await api('/api/papers/' + state.detail.id + '/collections', { method: 'POST', json: { collectionId: id } });
    fillDetail(p);
  } catch (e) { toast(e.message, true); }
}

async function removeDetailCollection(colId) {
  if (!state.detail) return;
  try {
    var p = await api('/api/papers/' + state.detail.id + '/collections/' + colId, { method: 'DELETE' });
    fillDetail(p);
  } catch (e) { toast(e.message, true); }
}

async function saveNotes() {
  if (!state.detail) return;
  var p = state.detail;
  var body = {
    title: p.title, authors: p.authors, year: p.year || 0, venue: p.venue, doi: p.doi,
    keywords: p.keywords, link: p.link, summary: p.summary, notes: $('detailNotes').value,
    categoryId: p.categoryId, read: p.read, starred: p.starred,
    tags: (p.tags || []).map(function (t) { return t.id; }),
    collections: (p.collections || []).map(function (c) { return c.id; })
  };
  try {
    var updated = await api('/api/papers/' + p.id, { method: 'PUT', json: body });
    state.detail = updated;
    fillDetail(updated);
    toast('笔记已保存');
    await loadPapers();
  } catch (e) { toast(e.message, true); }
}

function renderManage() {
  var ch = '';
  for (var i = 0; i < state.categories.length; i++) {
    var c = state.categories[i];
    ch += '<li><span class="name">' + esc(c.name) + '</span><span class="cnt">' + c.paperCount + ' 篇</span><button class="btn small danger" onclick="deleteCategory(' + c.id + ')">删除</button></li>';
  }
  $('categoryList').innerHTML = ch || '<li><span class="muted">暂无分类</span></li>';

  var th = '';
  for (var j = 0; j < state.tags.length; j++) {
    var t = state.tags[j];
    th += '<li><span class="name">' + esc(t.name) + '</span><span class="cnt">' + t.paperCount + ' 篇</span><button class="btn small danger" onclick="deleteTag(' + t.id + ')">删除</button></li>';
  }
  $('tagList').innerHTML = th || '<li><span class="muted">暂无标签</span></li>';

  var oh = '';
  for (var k = 0; k < state.collections.length; k++) {
    var o = state.collections[k];
    oh += '<li><span class="name">' + esc(o.name) + '</span><span class="cnt">' + o.paperCount + ' 篇</span><button class="btn small danger" onclick="deleteCollection(' + o.id + ')">删除</button></li>';
  }
  $('collectionList').innerHTML = oh || '<li><span class="muted">暂无合集</span></li>';
}

async function openManage() {
  try {
    await loadAll();
    renderManage();
    show('manageModal');
  } catch (e) { toast(e.message, true); }
}

async function addCategory() {
  var name = $('newCategory').value.trim();
  if (!name) return;
  try {
    await api('/api/categories', { method: 'POST', json: { name: name } });
    $('newCategory').value = '';
    toast('分类已添加');
    await loadAll(); renderManage(); await loadPapers();
  } catch (e) { toast(e.message, true); }
}

async function deleteCategory(id) {
  if (!confirm('删除该分类？论文将变为未分类。')) return;
  try {
    await api('/api/categories/' + id, { method: 'DELETE' });
    toast('已删除');
    await loadAll(); renderManage(); await loadPapers();
  } catch (e) { toast(e.message, true); }
}

async function addTag() {
  var name = $('newTag').value.trim();
  if (!name) return;
  try {
    await api('/api/tags', { method: 'POST', json: { name: name } });
    $('newTag').value = '';
    toast('标签已添加');
    await loadAll(); renderManage(); await loadPapers();
  } catch (e) { toast(e.message, true); }
}

async function deleteTag(id) {
  if (!confirm('删除该标签？')) return;
  try {
    await api('/api/tags/' + id, { method: 'DELETE' });
    toast('已删除');
    await loadAll(); renderManage(); await loadPapers();
  } catch (e) { toast(e.message, true); }
}

async function addCollection() {
  var name = $('newCollection').value.trim();
  if (!name) return;
  try {
    await api('/api/collections', { method: 'POST', json: { name: name } });
    $('newCollection').value = '';
    toast('合集已添加');
    await loadAll(); renderManage(); await loadPapers();
  } catch (e) { toast(e.message, true); }
}

async function deleteCollection(id) {
  if (!confirm('删除该合集？')) return;
  try {
    await api('/api/collections/' + id, { method: 'DELETE' });
    toast('已删除');
    await loadAll(); renderManage(); await loadPapers();
  } catch (e) { toast(e.message, true); }
}

function bindEvents() {
  $('addBtn').addEventListener('click', openAdd);
  $('manageBtn').addEventListener('click', openManage);
  $('paperForm').addEventListener('submit', submitPaperForm);
  $('saveNotesBtn').addEventListener('click', saveNotes);
  $('addTagBtn').addEventListener('click', addDetailTag);
  $('addCollectionBtn').addEventListener('click', addDetailCollection);
  $('addCategoryBtn').addEventListener('click', addCategory);
  $('addTagGlobalBtn').addEventListener('click', addTag);
  $('addCollectionGlobalBtn').addEventListener('click', addCollection);

  var closeBtns = document.querySelectorAll('[data-close]');
  for (var i = 0; i < closeBtns.length; i++) {
    closeBtns[i].addEventListener('click', function () { hide(this.getAttribute('data-close')); });
  }

  var searchTimer = null;
  $('searchInput').addEventListener('input', function () {
    clearTimeout(searchTimer);
    var v = this.value;
    searchTimer = setTimeout(function () { state.search = v.trim(); state.page = 1; loadPapers(); }, 300);
  });
  $('categoryFilter').addEventListener('change', function () { state.category = this.value; state.page = 1; loadPapers(); });
  $('tagFilter').addEventListener('change', function () { state.tag = this.value; state.page = 1; loadPapers(); });
  $('collectionFilter').addEventListener('change', function () { state.collection = this.value; state.page = 1; loadPapers(); });
  $('yearFrom').addEventListener('change', function () { state.yearFrom = this.value.trim(); state.page = 1; loadPapers(); });
  $('yearTo').addEventListener('change', function () { state.yearTo = this.value.trim(); state.page = 1; loadPapers(); });
  $('readFilter').addEventListener('change', function () { state.read = this.value; state.page = 1; loadPapers(); });
  $('starFilter').addEventListener('change', function () { state.starred = this.value; state.page = 1; loadPapers(); });
  $('sortSelect').addEventListener('change', function () { state.sort = this.value; loadPapers(); });
  $('orderSelect').addEventListener('change', function () { state.order = this.value; loadPapers(); });
  $('tableViewBtn').addEventListener('click', function () { state.view = 'table'; this.classList.add('active'); $('cardViewBtn').classList.remove('active'); renderPapers(); });
  $('cardViewBtn').addEventListener('click', function () { state.view = 'card'; this.classList.add('active'); $('tableViewBtn').classList.remove('active'); renderPapers(); });

  document.addEventListener('click', function (ev) {
    if (ev.target.classList && ev.target.classList.contains('modal')) ev.target.classList.add('hidden');
  });
}

bindEvents();
loadAll();
loadPapers();
