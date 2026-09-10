function makeEmptyForm() {
  return { title: "", authors: "", year: "", venue: "", doi: "", keywords: "", link: "", summary: "", notes: "", categoryId: "", tags: "", collections: "", read: false, starred: false, useAI: true };
}

var Root = {
  data: function () {
    return {
      route: "list", token: localStorage.getItem("pm-token") || "", loginPass: "", loginError: "",
      theme: localStorage.getItem("pm-theme") || "", themeOpen: false,
      themeOptions: [{ v: "", t: "日间" }, { v: "midnight", t: "黑夜" }, { v: "paper", t: "护眼" }],
      stats: {}, categories: [], tags: [], collections: [],
      papers: [], total: 0, page: 1, pages: 1, pageSize: 20, loading: true,
      view: "table", search: "", statusFilter: "", categoryFilter: "", tagFilter: "", collectionFilter: "", sort: "created", order: "desc",
      // 分类/标签/合集管理：内联改名
      taxEdit: { id: 0, kind: "", name: "" },
      // 合集页面
      currentCollectionId: 0, collectionPapers: [], collectionPapersMap: {},
      collectionPaperSearch: "", allPapersForCollection: [],
      // 详情页的设置分类
      categorySelect: "", newCategoryName: "",
      askOpen: false, question: "", answer: "", sources: [], asking: false,
      detail: {}, tagSelect: "", collectionSelect: "", fullWidth: false,
      pdfFullscreen: false, fsNotesMin: false, notesSavedAt: "",
      // 全屏阅读笔记浮窗：拖动位置 / 自定义尺寸 / 固定状态（持久化到 localStorage）
      fsNotesPos: null, fsNotesSize: null, fsNotesPinned: false, fsNotesDrag: false,
      // 笔记 Markdown 预览（编辑态/预览态，偏好持久化）
      notesPreview: false,
      // PDF 渲染与标注（pdf.js：画布 + 文字层 + 标注层）
      pdfLoading: false, pdfFailed: false, pdfError: "",
      // 标注：高亮 + 文字批注
      noteMode: false,
      annotations: [], annoColors: ["yellow", "green", "blue", "pink"],
      annoPopup: { show: false, x: 0, y: 0, start: 0, end: 0, quote: "" },
      annoEdit: { show: false, x: 0, y: 0, id: 0, color: "yellow", note: "" },
      // 划词翻译结果浮层
      transResult: { show: false, x: 0, y: 0, src: "", html: "", loading: false, error: "", rects: null },
      summaryExpanded: false, summaryOverflow: false,
      fsTransMin: false, transHistory: [], instantSrc: "", instantLoading: false,
      showPaperModal: false, editingId: null, saving: false, form: makeEmptyForm(),
      aiExtracting: false, summarizing: false, pdfExtracting: false,
      showManage: false, newCategory: "", newTag: "", newCollection: "",
      showTrash: false, trash: [], trashLoading: false, trashCount: 0,
      showSettings: false, settings: { aiBaseUrl: "https://tokendance.space/gateway/v1", aiModel: "deepseek-v3.2", aiApiKey: "" }, hasApiKey: false, testingAI: false, aiTestResult: "",
      aiModels: [], aiModelsLoading: false, customModel: false,
      uploading: false, uploadProgress: 0, uploadResult: null,
      toast: { show: false, msg: "", error: false }
    };
  },
  computed: {
    readPct: function () { if (!this.stats.total) return 0; return Math.round(this.stats.read / this.stats.total * 100); },
    statusOptions: function () { return [{v:"",t:"全部"},{v:"unread",t:"待读"},{v:"reading",t:"在读"},{v:"read",t:"已读"}]; },
    availableTags: function () { var self=this; return this.tags.filter(function(t){ return !(self.detail.tags||[]).some(function(x){return x.id===t.id;}); }); },
    availableCollections: function () { var self=this; return this.collections.filter(function(c){ return !(self.detail.collections||[]).some(function(x){return x.id===c.id;}); }); },
    renderedSummary: function () { return window.mdRender ? window.mdRender(this.detail.summary) : ""; },
    renderedAnswer: function () { return window.mdRender ? window.mdRender(this.answer) : ""; },
    uiBlocked: function () { return !!(this.showManage || this.showSettings || this.showPaperModal || this.showTrash); },
    // 合集页：当前打开的合集对象
    currentCollection: function () {
      var id = this.currentCollectionId;
      if (!id) return null;
      var found = null;
      (this.collections || []).forEach(function (c) { if (c.id === id) found = c; });
      return found;
    },
    // 合集页：可加入当前合集的候选（不在合集内 + 匹配搜索词）
    addableCandidates: function () {
      var kw = (this.collectionPaperSearch || "").trim().toLowerCase();
      if (!kw) return [];
      var inCol = {};
      this.collectionPapers.forEach(function (p) { inCol[p.id] = true; });
      return (this.allPapersForCollection || []).filter(function (p) {
        if (inCol[p.id]) return false;
        var hay = (p.title + " " + (p.authors || "")).toLowerCase();
        return hay.indexOf(kw) >= 0;
      });
    },
    // 分类/标签/合集的可选项：排除本篇已有的，用于「常用」一键添加
    availableCategories: function () {
      var self = this;
      return (this.categories || []).filter(function (c) {
        return !self.detail || c.name !== self.detail.category;
      });
    },
    unusedTags: function () {
      var used = {};
      ((this.detail && this.detail.tags) || []).forEach(function (t) { used[t.id] = true; });
      return (this.tags || []).filter(function (t) { return !used[t.id]; }).slice(0, 12);
    },
    unusedCollections: function () {
      var used = {};
      ((this.detail && this.detail.collections) || []).forEach(function (c) { used[c.id] = true; });
      return (this.collections || []).filter(function (c) { return !used[c.id]; }).slice(0, 8);
    },
    // 笔记的 Markdown 渲染（md-mini.js 已做 HTML 转义与 URL 白名单）
    renderedNotes: function () {
      var src = (this.detail && this.detail.notes) || "";
      return window.mdRender ? window.mdRender(src) : "";
    },
    // 拖动过就改用 left/top 定位；未拖动时保持 CSS 的底部居中默认位置。
    // 自定义尺寸只在展开态生效（收起态由 .fs-notes--min 的 width:auto 接管）。
    fsNotesStyle: function () {
      var s = {};
      if (this.fsNotesPos) { s.left = this.fsNotesPos.x + "px"; s.top = this.fsNotesPos.y + "px"; s.bottom = "auto"; s.transform = "none"; }
      if (this.fsNotesSize && !this.fsNotesMin) { s.width = this.fsNotesSize.w + "px"; s.height = this.fsNotesSize.h + "px"; }
      return s;
    }
  },
  watch: {
    // 摘要变化（打开新论文 / AI 重新生成）时重置展开态并重新测量是否溢出
    "detail.summary": function () {
      var self = this;
      this.summaryExpanded = false;
      this.$nextTick(function () { self.checkSummaryOverflow(); });
    },
    // 阅读浮窗出现/尺寸变化后，把已保存的位置拉回可视区
    "pdfFullscreen": function () { this.$nextTick(this.clampNotesPos); },
    "fullWidth": function () { this.$nextTick(this.clampNotesPos); },
    "fsNotesMin": function () { this.$nextTick(this.clampNotesPos); }
  },
  methods: {
    setTheme: function () {
      var t = this.theme || "";
      if (t) document.documentElement.setAttribute("data-theme", t);
      else document.documentElement.removeAttribute("data-theme");
      localStorage.setItem("pm-theme", t);
    },
    pickTheme: function (v) {
      this.theme = v;
      this.setTheme();
      this.themeOpen = false;
    },
    notify: function (msg, error) {
      var self=this; this.toast.show=true; this.toast.msg=msg; this.toast.error=!!error;
      clearTimeout(this._toastTimer); this._toastTimer=setTimeout(function(){ self.toast.show=false; }, 2600);
    },
    statusLabel: function (s) { return s==="read" ? "已读" : (s==="reading" ? "在读" : "待读"); },
    fmtDate: function (s) {
      if (!s) return "";
      var d = new Date(s); if (isNaN(d)) return s;
      var p = function (x) { return ("0" + x).slice(-2); };
      return d.getFullYear() + "-" + p(d.getMonth()+1) + "-" + p(d.getDate()) + " " + p(d.getHours()) + ":" + p(d.getMinutes());
    },
    // ---------- 笔记导出 ----------
    downloadText: function (filename, text) {
      var blob = new Blob([text], { type: "text/markdown;charset=utf-8" });
      var a = document.createElement("a");
      a.href = URL.createObjectURL(blob); a.download = filename; a.click();
      URL.revokeObjectURL(a.href);
    },
    exportNotes: function () {
      var p = this.detail; if (!p || !p.id) return;
      var lines = ["# " + (p.title || "未命名论文"), ""];
      var meta = [];
      if (p.authors) meta.push(p.authors);
      if (p.year) meta.push(p.year);
      if (p.venue) meta.push(p.venue);
      if (meta.length) lines.push(meta.join(" · "), "");
      lines.push("> 最后修改：" + (this.fmtDate(p.updatedAt) || "未知"), "");
      lines.push((p.notes || "").trim() || "（空）");
      this.downloadText("笔记-" + (p.title || p.id) + ".md", lines.join("\n"));
      this.notify("笔记已导出");
    },
    // 下载/iframe 无法自定义请求头，开启密码时把 token 拼进查询串
    authUrl: function (url) { if (!this.token) return url; return url + (url.indexOf("?") >= 0 ? "&" : "?") + "token=" + encodeURIComponent(this.token); },
    pdfUrl: function (id, download) { return this.authUrl("/api/papers/" + id + "/pdf" + (download ? "?download=1" : "")); },
    api: async function (path, opts) {
      opts = opts || {}; opts.headers = opts.headers || {};
      if (this.token) opts.headers["Authorization"] = "Bearer " + this.token;
      if (opts.json) { opts.headers["Content-Type"] = "application/json"; opts.body = JSON.stringify(opts.json); delete opts.json; }
      var res = await fetch(path, opts);
      if (res.status === 401) { if (this.token) { localStorage.removeItem("pm-token"); this.token=""; } this.route="login"; throw new Error("unauthorized"); }
      var data = null; try { data = await res.json(); } catch (e) {}
      if (!res.ok) { var err = new Error((data && data.error) || ("请求失败 " + res.status)); err.status=res.status; err.data=data; throw err; }
      return data;
    },
    go: function (r) { window.location.hash = r==="list" ? "#/" : ("#/" + r); this.route = r; window.scrollTo(0,0); },
    parseHash: function () {
      var h = window.location.hash || "#/";
      if (h.indexOf("#/papers/") === 0) { this.route="detail"; this.openDetail(parseInt(h.slice(9),10)); }
      else if (h.indexOf("#/upload") === 0) { this.route="upload"; }
      else if (h.indexOf("#/graph") === 0) { this.go("list"); } // 引用图谱暂不在前端展示（后端 /api/graph 保留）
      else if (h.indexOf("#/collections") === 0) { this.route = "collections"; this.loadCollectionsPage(h); }
      else if (h.indexOf("#/login") === 0) { this.route="login"; }
      else { this.route="list"; }
      if (this.route === "list") this.loadAll();
    },
    loadAll: async function () {
      this.loadTrashCount();
      try {
        var results = await Promise.all([this.api("/api/categories"), this.api("/api/tags"), this.api("/api/collections"), this.api("/api/stats"), this.api("/api/settings")]);
        this.categories=results[0]; this.tags=results[1]; this.collections=results[2]; this.stats=results[3];
        var st=results[4]; this.hasApiKey=!!st.hasApiKey; this.settings.aiBaseUrl=st.aiBaseUrl||this.settings.aiBaseUrl; this.settings.aiModel=st.aiModel||this.settings.aiModel;
      } catch (e) { if (e.status !== 401) this.notify(e.message, true); }
    },
    queryString: function () {
      var q=[];
      function add(k,v){ if(v!==null && v!==undefined && v!=="") q.push(k+"="+encodeURIComponent(v)); }
      add("page",this.page); add("pageSize",this.pageSize); add("search",this.search); add("status",this.statusFilter);
      add("category",this.categoryFilter); add("tags",this.tagFilter); add("collections",this.collectionFilter); add("sort",this.sort); add("order",this.order);
      return "/api/papers?"+q.join("&");
    },
    loadPapers: async function () {
      this.loading=true;
      try { var res=await this.api(this.queryString()); this.papers=res.papers||[]; this.total=res.total; this.page=res.page; this.pages=Math.max(1,Math.ceil(res.total/res.pageSize)); }
      catch (e) { if (e.status!==401) this.notify(e.message,true); }
      this.loading=false;
    },
    onSearch: function () { var self=this; clearTimeout(this._searchTimer); this._searchTimer=setTimeout(function(){ self.page=1; self.loadPapers(); }, 300); },
    applyFilters: function () { this.page=1; this.loadPapers(); },
    setStatus: function (s) { this.statusFilter=s; this.page=1; this.loadPapers(); },
    goPage: function (p) { if(p<1||p>this.pages) return; this.page=p; this.loadPapers(); },
    openEdit: function (p) {
      this.editingId=p.id;
      this.form={ title:p.title||"", authors:p.authors||"", year:p.year||"", venue:p.venue||"", doi:p.doi||"", keywords:p.keywords||"", link:p.link||"", summary:p.summary||"", notes:p.notes||"", categoryId:p.categoryId||"", tags:(p.tags||[]).map(function(t){return t.name;}).join(", "), collections:(p.collections||[]).map(function(c){return c.name;}).join(", "), read:!!p.read, starred:!!p.starred, useAI:true };
      if(this.$refs.pdfInput) this.$refs.pdfInput.value="";
      this.showPaperModal=true;
    },
    closePaperModal: function () { this.showPaperModal=false; },
    submitPaperForm: async function () {
      if (!this.form.title.trim()) { this.notify("标题必填", true); return; }
      var fd=new FormData();
      fd.append("title",this.form.title); fd.append("authors",this.form.authors); fd.append("year",this.form.year); fd.append("venue",this.form.venue);
      fd.append("doi",this.form.doi); fd.append("keywords",this.form.keywords); fd.append("link",this.form.link); fd.append("summary",this.form.summary); fd.append("notes",this.form.notes);
      fd.append("categoryId",this.form.categoryId); fd.append("read",this.form.read?"1":"0"); fd.append("starred",this.form.starred?"1":"0"); fd.append("useAI",this.form.useAI?"1":"0");
      fd.append("tags",this.form.tags); fd.append("collections",this.form.collections);
      var fileEl=this.$refs.pdfInput; if(fileEl && fileEl.files && fileEl.files[0]) fd.append("pdf",fileEl.files[0],fileEl.files[0].name);
      var url=this.editingId?("/api/papers/"+this.editingId):"/api/papers"; var method=this.editingId?"PUT":"POST";
      this.saving=true;
      try {
        await this.api(url,{method:method,body:fd});
        this.notify("保存成功"); this.showPaperModal=false;
        await this.loadAll(); await this.loadPapers();
      } catch (e) {
        if (e.status===409 && e.data && e.data.duplicate) {
          if (confirm("检测到重复论文："+e.data.title+"。仍然添加吗？")) {
            fd.append("force","1");
            try { await this.api(url,{method:method,body:fd}); this.notify("已添加"); this.showPaperModal=false; await this.loadAll(); await this.loadPapers(); } catch (e2) { this.notify(e2.message,true); }
          }
        } else { this.notify(e.message,true); }
      }
      this.saving=false;
    },
    onPdfChange: async function () {
      var el=this.$refs.pdfInput; if(!el||!el.files||!el.files[0]) return;
      this.pdfExtracting=true;
      var fd=new FormData(); fd.append("pdf",el.files[0],el.files[0].name); fd.append("useAI",this.form.useAI?"1":"0");
      try {
        var m=await this.api("/api/papers/extract-pdf",{method:"POST",body:fd});
        if(!this.form.title) this.form.title=m.title||""; if(!this.form.authors) this.form.authors=m.authors||"";
        if(!this.form.year) this.form.year=m.year||""; if(!this.form.venue) this.form.venue=m.venue||"";
        if(!this.form.doi) this.form.doi=m.doi||""; if(!this.form.keywords) this.form.keywords=m.keywords||""; if(!this.form.summary) this.form.summary=m.summary||"";
        if(m.aiUsed) this.form.useAI=false;
        this.notify(m.aiUsed?"已自动提取（AI），保存不再重复请求":"已提取 PDF 元数据");
      } catch (e) { this.notify(e.message,true); }
      this.pdfExtracting=false;
    },
    cycleStatus: async function (id) {
      try { await this.api("/api/papers/"+id+"/toggle-read",{method:"POST"}); await this.loadPapers(); } catch (e) { this.notify(e.message,true); }
    },
    deletePaper: async function (id) {
      if(!confirm("移入回收站？可在「管理 → 回收站」里恢复。")) return;
      try { await this.api("/api/papers/"+id,{method:"DELETE"}); this.notify("已移入回收站"); this.go("list"); this.loadAll(); this.loadPapers(); this.loadTrashCount(); } catch (e) { this.notify(e.message,true); }
    },
    // ---------- 回收站 ----------
    openTrash: function () { this.showTrash=true; this.loadTrash(); },
    loadTrash: async function () {
      this.trashLoading=true;
      try { var res=await this.api("/api/trash"); this.trash=res.papers||[]; this.trashCount=this.trash.length; }
      catch(e){ this.notify(e.message,true); }
      this.trashLoading=false;
    },
    loadTrashCount: async function () {
      try { var res=await this.api("/api/trash"); this.trashCount=(res.papers||[]).length; } catch(e){}
    },
    restorePaper: async function (id) {
      try { await this.api("/api/papers/"+id+"/restore",{method:"POST"}); this.notify("已恢复"); await this.loadTrash(); this.loadAll(); this.loadPapers(); } catch(e){ this.notify(e.message,true); }
    },
    purgePaper: async function (p) {
      if(!confirm("彻底删除《"+p.title+"》？PDF 文件会一起删掉，无法恢复。")) return;
      try { await this.api("/api/papers/"+p.id+"/purge",{method:"DELETE"}); this.notify("已彻底删除"); await this.loadTrash(); this.loadAll(); } catch(e){ this.notify(e.message,true); }
    },
    emptyTrash: async function () {
      if(!confirm("清空回收站？其中所有论文与 PDF 将被永久删除，无法恢复。")) return;
      try { var res=await this.api("/api/trash",{method:"DELETE"}); this.notify("已清空回收站（"+((res&&res.removed)||0)+" 个文件）"); await this.loadTrash(); this.loadAll(); } catch(e){ this.notify(e.message,true); }
    },
    openDetail: async function (id) {
      this.detail={}; this.route="detail";
      // 只有 hash 真的变化才赋值：否则会触发 hashchange → 再次 openDetail，
      // 把正在加载的 pdf.js 任务销毁掉（报 Worker was destroyed）
      if (window.location.hash !== "#/papers/" + id) window.location.hash = "#/papers/" + id;
      this.transHistory=[]; this.instantSrc="";
      this.readPages=[]; this.annotations=[]; this.readHasText=false;
      this.annoPopup.show=false; this.annoEdit.show=false; this.transResult.show=false;
      this.destroyPdf();
      try { this.detail=await this.api("/api/papers/"+id); this.tagSelect=""; this.collectionSelect=""; this.fsNotesMin=false; this.notesSavedAt=""; } catch (e) { this.notify(e.message,true); }
      await this.loadAnnotations(id);
      var self = this;
      this.$nextTick(function () { self.setupPdf(); });
    },
    // 点击 PDF 上的高亮（事件委托，绑定在容器上）
    onPdfContainerClick: function (ev) { this.onPdfClick(ev); },
    addToHistory: function (src, out) {
      this.transHistory.unshift({ src: src.length > 220 ? src.slice(0, 220) + "…" : src, html: window.mdRender ? window.mdRender(out) : out });
      if (this.transHistory.length > 30) this.transHistory.pop();
    },
    instantTranslate: async function () {
      var text=(this.instantSrc||"").trim(); if(!text||this.instantLoading) return;
      this.instantLoading=true;
      try {
        var res=await this.api("/api/ai/translate-text",{method:"POST",json:{text:text}});
        this.addToHistory(text,(res&&res.translation)||"");
        this.instantSrc="";
      }
      catch(e){ this.notify(e.message,true); }
      this.instantLoading=false;
    },
    // ---------- 笔记（全屏阅读悬浮窗与详情面板共用同一份数据） ----------
    // 浮窗拖动：按住标题栏移动；固定后禁止拖动。位置与固定状态存 localStorage。
    startNotesDrag: function (e) {
      if (this.fsNotesPinned || this.fsNotesDrag) return;
      if (e.button !== undefined && e.button !== 0) return;            // 只响应左键/触摸
      if (e.target.closest && e.target.closest("button, input, textarea, a, select")) return;
      var el = this.$refs.fsNotes;
      if (!el) return;
      var self = this;
      var rect = el.getBoundingClientRect();
      var offX = e.clientX - rect.left, offY = e.clientY - rect.top;
      this.fsNotesPos = { x: Math.round(rect.left), y: Math.round(rect.top) };
      this.fsNotesDrag = true;
      document.body.classList.add("fs-dragging");   // 拖动时禁用 iframe 命中与文本选择
      var move = function (ev) {
        var w = el.offsetWidth || 320, h = el.offsetHeight || 120;
        self.fsNotesPos = {
          x: Math.round(Math.min(Math.max(ev.clientX - offX, 8), Math.max(window.innerWidth - w - 8, 8))),
          y: Math.round(Math.min(Math.max(ev.clientY - offY, 8), Math.max(window.innerHeight - h - 8, 8)))
        };
      };
      var end = function () {
        window.removeEventListener("pointermove", move);
        window.removeEventListener("pointerup", end);
        window.removeEventListener("pointercancel", end);
        document.body.classList.remove("fs-dragging");
        self.fsNotesDrag = false;
        self.saveNotesPanelPrefs();
      };
      window.addEventListener("pointermove", move);
      window.addEventListener("pointerup", end);
      window.addEventListener("pointercancel", end);
      e.preventDefault();
    },
    // 右下角拖拽调整大小：宽度/高度分别限制在 [最小尺寸, 视口] 内
    startNotesResize: function (e) {
      if (this.fsNotesPinned || this.fsNotesDrag) return;
      if (e.button !== undefined && e.button !== 0) return;
      var el = this.$refs.fsNotes;
      if (!el) return;
      var self = this;
      var rect = el.getBoundingClientRect();
      var startX = e.clientX, startY = e.clientY, startW = rect.width, startH = rect.height;
      // 调整大小时把浮窗锚定到当前左上角，避免从默认底部居中位置"跳"
      this.fsNotesPos = { x: Math.round(rect.left), y: Math.round(rect.top) };
      this.fsNotesDrag = true;
      document.body.classList.add("fs-dragging");
      var move = function (ev) {
        var w = Math.round(Math.min(Math.max(startW + (ev.clientX - startX), 320), Math.max(window.innerWidth - 16, 320)));
        var h = Math.round(Math.min(Math.max(startH + (ev.clientY - startY), 140), Math.max(window.innerHeight - 16, 140)));
        self.fsNotesSize = { w: w, h: h };
        var pos = self.fsNotesPos;
        self.fsNotesPos = {
          x: Math.round(Math.min(Math.max(pos.x, 8), Math.max(window.innerWidth - w - 8, 8))),
          y: Math.round(Math.min(Math.max(pos.y, 8), Math.max(window.innerHeight - h - 8, 8)))
        };
      };
      var end = function () {
        window.removeEventListener("pointermove", move);
        window.removeEventListener("pointerup", end);
        window.removeEventListener("pointercancel", end);
        document.body.classList.remove("fs-dragging");
        self.fsNotesDrag = false;
        self.saveNotesPanelPrefs();
      };
      window.addEventListener("pointermove", move);
      window.addEventListener("pointerup", end);
      window.addEventListener("pointercancel", end);
      e.preventDefault();
      e.stopPropagation();
    },
    // ---------- PDF 渲染（pdf.js）+ 划段标注 ----------
    setupPdf: async function () {
      this.destroyPdf();
      this.pdfFailed = false;
      if (!this.detail || !this.detail.hasPdf) return;
      if (!window.pdfjsLib) { this.pdfFailed = true; return; }
      var self = this, el = this.$refs.pdfView;
      if (!el) return;
      this.pdfLoading = true;
      this.pdfError = "";
      pdfjsLib.GlobalWorkerOptions.workerSrc = "/assets/pdfjs/pdf.worker.min.js";
      var task = pdfjsLib.getDocument({ url: this.pdfUrl(this.detail.id) });
      var S = { task: task, doc: null, pages: [], el: el, width: 0, io: null };
      this._pdf = S;
      try {
        var doc = await task.promise;
        if (this._pdf !== S) return; // 期间切了论文
        S.doc = doc;
        el.innerHTML = "";
        for (var n = 1; n <= doc.numPages; n++) {
          var wrap = document.createElement("div");
          wrap.className = "pdf-page";
          wrap.dataset.page = n;
          var canvas = document.createElement("canvas");
          var text = document.createElement("div");
          text.className = "pdf-text";
          var anno = document.createElement("div");
          anno.className = "pdf-annos";
          wrap.appendChild(canvas); wrap.appendChild(text); wrap.appendChild(anno);
          el.appendChild(wrap);
          S.pages.push({ n: n, wrap: wrap, canvas: canvas, text: text, anno: anno, done: false, w: 0, h: 0 });
        }
        await this.layoutPdf(true);
        var io = new IntersectionObserver(function (entries) {
          entries.forEach(function (en) { if (en.isIntersecting) self.renderPdfPage(+en.target.dataset.page); });
        }, { root: el, rootMargin: "700px 0px" });
        S.io = io;
        S.pages.forEach(function (p) { io.observe(p.wrap); });
      } catch (e) {
        if (this._pdf !== S) return; // 已被新的一次渲染取代，忽略这次的失败
        this.pdfFailed = true;       // 回退浏览器原生阅读器
        this.pdfError = String((e && e.message) || e);
      }
      this.pdfLoading = false;
    },
    destroyPdf: function () {
      var S = this._pdf;
      if (S) {
        if (S.io) S.io.disconnect();
        if (S.task) { try { S.task.destroy(); } catch (e) {} }
        if (S.el) S.el.innerHTML = "";
      }
      this._pdf = null;
    },
    // 按容器宽度排布页面占位（避免渲染后跳动）
    layoutPdf: async function (force) {
      var S = this._pdf;
      if (!S || !S.doc || !S.el) return;
      var width = S.el.clientWidth - 20;
      if (width < 260) width = 260;
      if (!force && S.width && Math.abs(S.width - width) < 10) return;
      S.width = width;
      for (var i = 0; i < S.pages.length; i++) {
        var p = S.pages[i];
        var page = await S.doc.getPage(p.n);
        var base = page.getViewport({ scale: 1 });
        var scale = width / base.width;
        var vp = page.getViewport({ scale: scale });
        p.scale = scale; p.w = vp.width; p.h = vp.height;
        p.wrap.style.width = Math.round(vp.width) + "px";
        p.wrap.style.height = Math.round(vp.height) + "px";
        // pdf.js 文字层依赖这个 CSS 变量来定位 span
        p.wrap.style.setProperty("--scale-factor", scale);
        p.done = false;
        p.canvas.width = 0; p.canvas.height = 0;
        p.canvas.style.width = ""; p.canvas.style.height = "";
        p.text.innerHTML = "";
      }
      this.renderAllAnnotations();
      var st = S.el.scrollTop, vh = S.el.clientHeight;
      var self = this;
      S.pages.forEach(function (p) {
        var top = p.wrap.offsetTop;
        if (top + p.wrap.offsetHeight > st - 900 && top < st + vh + 900) self.renderPdfPage(p.n);
      });
    },
    renderPdfPage: async function (n) {
      var S = this._pdf;
      if (!S || !S.doc) return;
      var p = S.pages[n - 1];
      if (!p || p.done) return;
      p.done = true;
      try {
        var page = await S.doc.getPage(n);
        var vp = page.getViewport({ scale: p.scale });
        var dpr = Math.min(window.devicePixelRatio || 1, 2);
        p.canvas.width = Math.round(vp.width * dpr);
        p.canvas.height = Math.round(vp.height * dpr);
        p.canvas.style.width = Math.round(vp.width) + "px";
        p.canvas.style.height = Math.round(vp.height) + "px";
        var ctx = p.canvas.getContext("2d");
        await page.render({ canvasContext: ctx, viewport: vp, transform: dpr !== 1 ? [dpr, 0, 0, dpr, 0, 0] : null }).promise;
        if (this._pdf !== S) return;
        var tc = await page.getTextContent();
        p.text.innerHTML = "";
        await pdfjsLib.renderTextLayer({ textContentSource: tc, container: p.text, viewport: vp }).promise;
        this.renderPageAnnotations(n);
      } catch (e) { p.done = false; }
    },
    // ---------- 标注层 ----------
    renderPageAnnotations: function (n) {
      var S = this._pdf;
      if (!S) return;
      var p = S.pages[n - 1];
      if (!p) return;
      p.anno.innerHTML = "";
      var w = p.wrap.clientWidth || p.w, h = p.wrap.clientHeight || p.h;
      if (!w || !h) return;
      (this.annotations || []).forEach(function (a) {
        (a.rects || []).forEach(function (r) {
          if (r.p !== n) return;
          var d = document.createElement("div");
          d.className = "pdf-anno pdf-anno--" + a.color;
          d.style.left = (r.x * w) + "px";
          d.style.top = (r.y * h) + "px";
          d.style.width = Math.max(2, r.w * w) + "px";
          d.style.height = Math.max(2, r.h * h) + "px";
          d.dataset.id = a.id;
          d.title = a.note ? a.note : "点击编辑标注";
          p.anno.appendChild(d);
        });
      });
      // 文字批注框
      var self = this;
      (this.annotations || []).forEach(function (a) {
        if (a.kind !== "note" || a.page !== n) return;
        var box = document.createElement("div");
        box.className = "pdf-note pdf-note--" + a.color;
        box.dataset.id = a.id;
        box.style.left = ((a.x || 0) * w) + "px";
        box.style.top = ((a.y || 0) * h) + "px";
        var grip = document.createElement("div");
        grip.className = "pdf-note__grip";
        grip.title = "按住拖动";
        grip.textContent = "⠿";
        grip.addEventListener("pointerdown", function (ev) { self.startNoteDrag(ev, a.id, n); });
        var del = document.createElement("button");
        del.type = "button";
        del.className = "pdf-note__del";
        del.title = "删除批注";
        del.textContent = "×";
        del.addEventListener("click", function (ev) { ev.stopPropagation(); self.deleteAnnotation(a.id); });
        var ta = document.createElement("textarea");
        ta.className = "pdf-note__text";
        ta.placeholder = "写点什么…";
        ta.value = a.note || "";
        ta.addEventListener("change", function () { self.saveNoteText(a.id, ta.value); });
        ta.addEventListener("blur", function () { self.saveNoteText(a.id, ta.value); });
        ta.addEventListener("pointerdown", function (ev) { ev.stopPropagation(); });
        ta.addEventListener("mouseup", function (ev) { ev.stopPropagation(); });
        ta.addEventListener("click", function (ev) { ev.stopPropagation(); });
        box.appendChild(grip);
        box.appendChild(del);
        box.appendChild(ta);
        p.anno.appendChild(box);
      });
    },
    renderAllAnnotations: function () {
      var S = this._pdf;
      if (!S) return;
      for (var i = 1; i <= S.pages.length; i++) this.renderPageAnnotations(i);
    },
    // ---------- 文字批注框（点击页面放置，可编辑、拖动、删除） ----------
    toggleNoteMode: function () {
      this.noteMode = !this.noteMode;
      if (this.noteMode) this.notify("点击 PDF 页面任意位置放置批注框");
    },
    // 在指定页面、指定归一化位置新建批注框
    createNoteAt: async function (pageNo, px, py) {
      if (!this.detail || !this.detail.id) return;
      this.noteMode = false;
      try {
        var a = await this.api("/api/papers/" + this.detail.id + "/annotations", {
          method: "POST",
          json: { kind: "note", page: pageNo, x: px, y: py, color: "yellow" }
        });
        this.annotations = this.annotations.concat([a]);
        this.renderPageAnnotations(pageNo);
        var self = this;
        setTimeout(function () {
          var ta = document.querySelector('.pdf-note[data-id="' + a.id + '"] textarea');
          if (ta) ta.focus();
        }, 30);
      } catch (e) { this.notify(e.message, true); }
    },
    // 批注文字：只在提交时保存，且不重绘（避免打字时输入框被重建、光标丢失）
    saveNoteText: async function (id, text) {
      try {
        await this.api("/api/annotations/" + id, { method: "PATCH", json: { note: text } });
        this.annotations = this.annotations.map(function (a) {
          return a.id === id ? Object.assign({}, a, { note: text }) : a;
        });
      } catch (e) { this.notify(e.message, true); }
    },
    // 拖动批注框：拖动中只改样式，松手后保存归一化位置
    startNoteDrag: function (ev, id, pageNo) {
      var S = this._pdf, self = this;
      if (!S) return;
      var page = S.pages[pageNo - 1];
      var box = ev.currentTarget.closest(".pdf-note");
      if (!page || !box) return;
      ev.preventDefault();
      ev.stopPropagation();
      var boxRect = page.wrap.getBoundingClientRect();
      var startX = ev.clientX, startY = ev.clientY;
      var boxLeft = box.offsetLeft, boxTop = box.offsetTop;
      var move = function (e) {
        var maxX = page.wrap.clientWidth - box.offsetWidth;
        var maxY = page.wrap.clientHeight - box.offsetHeight;
        var nx = Math.min(Math.max(boxLeft + (e.clientX - startX), 0), Math.max(maxX, 0));
        var ny = Math.min(Math.max(boxTop + (e.clientY - startY), 0), Math.max(maxY, 0));
        box.style.left = nx + "px";
        box.style.top = ny + "px";
      };
      var end = function () {
        window.removeEventListener("pointermove", move);
        window.removeEventListener("pointerup", end);
        window.removeEventListener("pointercancel", end);
        document.body.classList.remove("fs-dragging");
        var px = Math.min(Math.max(box.offsetLeft / page.wrap.clientWidth, 0), 1);
        var py = Math.min(Math.max(box.offsetTop / page.wrap.clientHeight, 0), 1);
        self.annotations = self.annotations.map(function (a) {
          return a.id === id ? Object.assign({}, a, { x: px, y: py }) : a;
        });
        self.api("/api/annotations/" + id, { method: "PATCH", json: { x: px, y: py } })
          .catch(function (e) { self.notify(e.message, true); });
        _ = boxRect;
      };
      document.body.classList.add("fs-dragging");
      window.addEventListener("pointermove", move);
      window.addEventListener("pointerup", end);
      window.addEventListener("pointercancel", end);
    },
    // 选区 → 每页的归一化矩形（与栏数、缩放无关）
    selectionRects: function (range, S) {
      var rects = range.getClientRects();
      var raw = [];
      for (var i = 0; i < rects.length; i++) {
        var r = rects[i];
        if (r.width < 1 || r.height < 1) continue;
        var mid = r.top + r.height / 2;
        for (var j = 0; j < S.pages.length; j++) {
          var pg = S.pages[j], box = pg.wrap.getBoundingClientRect();
          if (mid < box.top || mid > box.bottom || box.height < 1) continue;
          raw.push({
            p: pg.n,
            x: Math.max(0, (r.left - box.left) / box.width),
            y: Math.max(0, (r.top - box.top) / box.height),
            w: Math.min(1, r.width / box.width),
            h: Math.min(1, r.height / box.height)
          });
          break;
        }
      }
      return mergeRects(raw);
    },
    onPdfMouseUp: function () {
      var self = this;
      if (this.annoEdit.show) return;
      setTimeout(function () {
        var S = self._pdf, sel = window.getSelection();
        if (!S || !sel || sel.isCollapsed) { self.annoPopup.show = false; return; }
        var range = sel.getRangeAt(0);
        if (!S.el.contains(range.commonAncestorContainer)) { self.annoPopup.show = false; return; }
        var text = sel.toString().replace(/\s+/g, " ").trim();
        if (text.length < 2) { self.annoPopup.show = false; return; }
        var rs = self.selectionRects(range, S);
        if (!rs.length) { self.annoPopup.show = false; return; }
        var box = range.getBoundingClientRect();
        var popW = 320, popH = 52;
        self.annoPopup = {
          show: true,
          x: Math.min(Math.max(box.left + Math.min(box.width, 300) / 2, popW / 2), Math.max(window.innerWidth - popW / 2, popW / 2)),
          // 选区靠近上下边缘时把浮层夹在视口内
          y: Math.min(Math.max(box.top - popH - 6, 8), Math.max(window.innerHeight - popH - 8, 8)),
          rects: rs, quote: text.slice(0, 4000)
        };
      }, 10);
    },
    // ---------- 划词翻译 ----------
    // 选中文字 → 翻译 → 结果浮层；译文可一键放到页面上作为批注框
    translateSelection: async function () {
      var pop = this.annoPopup;
      var text = (pop.quote || "").trim();
      if (!text) return;
      var px = Math.min(Math.max(pop.x, 210), Math.max(window.innerWidth - 210, 210));
      var py = Math.min(Math.max(pop.y + 46, 8), Math.max(window.innerHeight - 260, 8));
      this.annoPopup.show = false;
      this.transResult = {
        show: true, x: px, y: py, src: text.slice(0, 200),
        html: "", loading: true, error: "", rects: pop.rects || null
      };
      try {
        var res = await this.api("/api/ai/translate-text", { method: "POST", json: { text: text } });
        var out = (res && res.translation) || "";
        if (!this.transResult.show) return;
        this.transResult.loading = false;
        this.transResult.html = window.mdRender ? window.mdRender(out) : escHtml(out);
        this.addToHistory(text, out);
      } catch (e) {
        this.transResult.loading = false;
        this.transResult.error = e.message;
      }
    },
    // 把译文放到 PDF 上（选区下方）作为批注框
    translationToNote: async function () {
      var r = this.transResult;
      if (!r || !r.show || !this.detail.id) return;
      var rect = (r.rects && r.rects[0]) || { p: 1, x: 0.1, y: 0.1, h: 0.02 };
      var text = (r.html ? this.transResultText() : "").trim();
      if (!text) return;
      this.transResult.show = false;
      try {
        var a = await this.api("/api/papers/" + this.detail.id + "/annotations", {
          method: "POST",
          json: {
            kind: "note", page: rect.p,
            x: Math.min(rect.x, 0.85),
            y: Math.min(rect.y + (rect.h || 0.02) + 0.01, 0.94),
            color: "blue", note: text.slice(0, 2000)
          }
        });
        this.annotations = this.annotations.concat([a]);
        this.renderPageAnnotations(rect.p);
        this.notify("译文已作为批注放到页面上");
      } catch (e) { this.notify(e.message, true); }
    },
    // 取纯文本译文（浮层里渲染的是 HTML，这里转回文本用于批注）
    transResultText: function () {
      var el = document.createElement("div");
      el.innerHTML = this.transResult.html || "";
      return (el.textContent || "").trim();
    },
    onPdfClick: function (ev) {
      // 批注模式：点击页面任意位置放置批注框
      if (this.noteMode) {
        var t = ev.target;
        if (t && t.closest && (t.closest(".pdf-note") || t.closest(".pdf-anno"))) return;
        var pageEl = t && t.closest ? t.closest(".pdf-page") : null;
        var S = this._pdf;
        if (pageEl && S) {
          var rect = pageEl.getBoundingClientRect();
          var px = Math.min(Math.max((ev.clientX - rect.left) / rect.width, 0), 0.92);
          var py = Math.min(Math.max((ev.clientY - rect.top) / rect.height, 0), 0.95);
          this.createNoteAt(parseInt(pageEl.dataset.page, 10), px, py);
          return;
        }
      }
      var box = ev.target && ev.target.closest ? ev.target.closest(".pdf-anno") : null;
      if (!box) { this.annoEdit.show = false; return; }
      var id = parseInt(box.dataset.id, 10);
      var a = null;
      for (var i = 0; i < this.annotations.length; i++) { if (this.annotations[i].id === id) a = this.annotations[i]; }
      if (!a) return;
      var r = box.getBoundingClientRect();
      this.annoPopup.show = false;
      this.annoEdit = {
        show: true, id: a.id, color: a.color, note: a.note || "",
        x: Math.min(Math.max(r.left, 8), Math.max(window.innerWidth - 360, 8)),
        y: Math.min(r.bottom + 8, Math.max(window.innerHeight - 220, 8))
      };
    },
    loadAnnotations: async function (id) {
      try {
        var res = await this.api("/api/papers/" + id + "/annotations");
        if (!this.detail || this.detail.id !== id) return;
        this.annotations = res.annotations || [];
        this.renderAllAnnotations();
      } catch (e) { this.annotations = []; }
    },
    addAnnotation: async function (color) {
      var p = this.annoPopup;
      if (!p.show || !this.detail.id || !p.rects) return;
      this.annoPopup.show = false;
      try {
        var a = await this.api("/api/papers/" + this.detail.id + "/annotations", {
          method: "POST",
          json: { page: p.rects[0].p, rects: p.rects, quote: p.quote, color: color }
        });
        this.annotations = this.annotations.concat([a]).sort(function (x, y) {
          return x.page - y.page || x.id - y.id;
        });
        this.renderAllAnnotations();
        var sel = window.getSelection(); if (sel) sel.removeAllRanges();
        this.notify("已高亮");
      } catch (e) { this.notify(e.message, true); }
    },
    setAnnotationColor: async function (id, color) {
      try {
        await this.api("/api/annotations/" + id, { method: "PATCH", json: { color: color } });
        this.annotations = this.annotations.map(function (a) { return a.id === id ? Object.assign({}, a, { color: color }) : a; });
        this.annoEdit.color = color;
        this.renderAllAnnotations();
      } catch (e) { this.notify(e.message, true); }
    },
    saveAnnotationNote: async function (id, note) {
      try {
        await this.api("/api/annotations/" + id, { method: "PATCH", json: { note: note } });
        this.annotations = this.annotations.map(function (a) { return a.id === id ? Object.assign({}, a, { note: note }) : a; });
      } catch (e) { this.notify(e.message, true); }
    },
    deleteAnnotation: async function (id) {
      try {
        await this.api("/api/annotations/" + id, { method: "DELETE" });
        this.annotations = this.annotations.filter(function (a) { return a.id !== id; });
        this.annoEdit.show = false;
        this.renderAllAnnotations();
      } catch (e) { this.notify(e.message, true); }
    },
    // 点标注列表 → 滚到 PDF 对应位置并闪一下
    jumpToAnnotation: function (id) {
      var a = null;
      for (var i = 0; i < this.annotations.length; i++) { if (this.annotations[i].id === id) a = this.annotations[i]; }
      if (!a || !this._pdf) { this.notify("切到 PDF 视图才能定位", true); return; }
      var S = this._pdf;
      if (!S) return;
      var page = S.pages[(a.page || 1) - 1];
      if (page) page.wrap.scrollIntoView({ block: "start", behavior: "smooth" });
      var self2 = this;
      setTimeout(function () {
        var el = document.querySelector('.pdf-anno[data-id="' + id + '"]');
        if (!el) return;
        el.classList.add("pdf-anno--flash");
        setTimeout(function () { el.classList.remove("pdf-anno--flash"); }, 1400);
      }, 400);
    },
    // 把标注按 Markdown 追加到笔记
    insertAnnotationsIntoNotes: function () {
      if (!this.annotations.length) return;
      var lines = ["", "", "## 标注"];
      this.annotations.forEach(function (a) {
        var q = (a.quote || "").replace(/\s+/g, " ").trim();
        lines.push("- （第 " + (a.page || 1) + " 页）> " + q);
        if (a.note) lines.push("  - 备注：" + a.note);
      });
      var cur = (this.detail && this.detail.notes) || "";
      this.detail.notes = cur.replace(/\s+$/, "") + "\n" + lines.join("\n") + "\n";
      this.notify("已插入笔记，记得保存");
    },
    toggleNotesPreview: function () {
      this.notesPreview = !this.notesPreview;
      try { localStorage.setItem("pm-notes-preview", this.notesPreview ? "1" : "0"); } catch (e) {}
    },
    toggleNotesPin: function () {
      this.fsNotesPinned = !this.fsNotesPinned;
      this.saveNotesPanelPrefs();
      this.notify(this.fsNotesPinned ? "笔记框已固定（位置与大小锁定）" : "笔记框已取消固定");
    },
    // 双击复位：位置与尺寸都回到默认
    resetNotesLayout: function () {
      this.fsNotesPos = null;
      this.fsNotesSize = null;
      this.saveNotesPanelPrefs();
    },
    // 窗口变窄/变矮后把浮窗拉回可视区（位置与尺寸一起收紧）
    clampNotesPos: function () {
      if (this.fsNotesSize) {
        this.fsNotesSize = {
          w: Math.round(Math.min(this.fsNotesSize.w, Math.max(window.innerWidth - 16, 320))),
          h: Math.round(Math.min(this.fsNotesSize.h, Math.max(window.innerHeight - 16, 140)))
        };
      }
      if (!this.fsNotesPos) return;
      var el = this.$refs.fsNotes;
      var w = (el && el.offsetWidth) || (this.fsNotesSize ? this.fsNotesSize.w : 320);
      var h = (el && el.offsetHeight) || (this.fsNotesSize ? this.fsNotesSize.h : 120);
      this.fsNotesPos = {
        x: Math.round(Math.min(Math.max(this.fsNotesPos.x, 8), Math.max(window.innerWidth - w - 8, 8))),
        y: Math.round(Math.min(Math.max(this.fsNotesPos.y, 8), Math.max(window.innerHeight - h - 8, 8)))
      };
    },
    saveNotesPanelPrefs: function () {
      try { localStorage.setItem("pm-fsnotes", JSON.stringify({ pos: this.fsNotesPos, size: this.fsNotesSize, pinned: this.fsNotesPinned })); } catch (e) {}
    },
    loadNotesPanelPrefs: function () {
      try {
        var v = JSON.parse(localStorage.getItem("pm-fsnotes") || "null");
        if (!v) return;
        if (v.pos && typeof v.pos.x === "number" && typeof v.pos.y === "number") this.fsNotesPos = { x: v.pos.x, y: v.pos.y };
        if (v.size && typeof v.size.w === "number" && typeof v.size.h === "number") this.fsNotesSize = { w: v.size.w, h: v.size.h };
        this.fsNotesPinned = !!v.pinned;
      } catch (e) {}
    },
    checkSummaryOverflow: function () {
      var el = this.$refs.summaryBody;
      if (!el) { this.summaryOverflow = false; return; }
      // 折叠态下 clientHeight 被限高，scrollHeight 是完整内容高
      this.summaryOverflow = el.scrollHeight > el.clientHeight + 8;
    },
    saveNotes: async function (silent) {
      if(!this.detail||!this.detail.id) return;
      var id=this.detail.id, sent=this.detail.notes||"";
      try {
        var p=await this.api("/api/papers/"+id,{method:"PATCH",json:{notes:sent}});
        // 等待期间可能切到别的论文或继续输入：这两种情况都不能用旧响应覆盖
        if(this.detail&&this.detail.id===id){
          if(this.detail.notes===sent&&p&&typeof p.notes==="string") this.detail.notes=p.notes;
          var t=new Date(); this.notesSavedAt=("0"+t.getHours()).slice(-2)+":"+("0"+t.getMinutes()).slice(-2);
          if(!silent) this.notify("笔记已保存");
        }
      } catch(e){ if(!silent) this.notify(e.message,true); }
    },
    autosaveNotes: function () {
      var self=this; clearTimeout(this._notesTimer);
      this._notesTimer=setTimeout(function(){ self.saveNotes(true); }, 1200);
    },
    saveDetail: async function () {
      if(!this.detail||!this.detail.id) return;
      var p=this.detail;
      var body={ title:p.title, authors:p.authors, year:p.year||0, venue:p.venue, doi:p.doi, keywords:p.keywords, link:p.link, summary:p.summary, notes:p.notes, categoryId:p.categoryId, status:p.status, read:p.status==="read", starred:p.starred, tags:(p.tags||[]).map(function(t){return t.id;}), collections:(p.collections||[]).map(function(c){return c.id;}) };
      try { this.detail=await this.api("/api/papers/"+p.id,{method:"PUT",json:body}); this.notify("已保存"); this.loadAll(); this.loadPapers(); } catch (e) { this.notify(e.message,true); }
    },
    // 按名字添加：不存在时后端自动创建（EnsureTag / EnsureCollection）
    // 设置分类（可选新建名称）
    setDetailCategory: async function (useNew) {
      var name = useNew === true ? (this.newCategoryName || "").trim() : (this.categorySelect || "").trim();
      if (!name || !this.detail.id) {
        if (useNew === true) this.notify("请输入分类名称", true);
        return;
      }
      try {
        this.detail = await this.api("/api/papers/" + this.detail.id, { method: "PATCH", json: { categoryName: name } });
        this.newCategoryName = ""; this.categorySelect = "";
        await this.loadTaxonomy();
        this.notify("分类已设为「" + name + "」");
      } catch (e) { this.notify(e.message, true); }
    },
    clearDetailCategory: async function () {
      if (!this.detail.id) return;
      try {
        this.detail = await this.api("/api/papers/" + this.detail.id, { method: "PATCH", json: { categoryName: "" } });
        await this.loadTaxonomy();
        this.notify("已清除分类");
      } catch (e) { this.notify(e.message, true); }
    },
    addDetailTag: async function (preset) {
      // preset：点「常用」标签时直接传入名字
      var name = (typeof preset === "string" ? preset : (this.tagSelect || "")).trim();
      if (!name || !this.detail.id) return;
      try {
        this.detail = await this.api("/api/papers/" + this.detail.id + "/tags", { method: "POST", json: { name: name } });
        this.tagSelect = "";
        await this.loadTaxonomy();
      } catch (e) { this.notify(e.message, true); }
    },
    removeDetailTag: async function (id) {
      if (!this.detail.id) return;
      try { this.detail = await this.api("/api/papers/" + this.detail.id + "/tags/" + id, { method: "DELETE" }); await this.loadTaxonomy(); }
      catch (e) { this.notify(e.message, true); }
    },
    addDetailCollection: async function (preset) {
      var name = (typeof preset === "string" ? preset : (this.collectionSelect || "")).trim();
      if (!name || !this.detail.id) return;
      try {
        this.detail = await this.api("/api/papers/" + this.detail.id + "/collections", { method: "POST", json: { name: name } });
        this.collectionSelect = "";
        await this.loadTaxonomy();
      } catch (e) { this.notify(e.message, true); }
    },
    removeDetailCollection: async function (id) {
      if (!this.detail.id) return;
      try { this.detail = await this.api("/api/papers/" + this.detail.id + "/collections/" + id, { method: "DELETE" }); await this.loadTaxonomy(); }
      catch (e) { this.notify(e.message, true); }
    },
    aiExtract: async function (id) { if(!this.hasApiKey){ this.notify("未配置 API Key，已跳过",true); return; } this.aiExtracting=true; try { var res=await this.api("/api/papers/"+id+"/ai-extract",{method:"POST"}); if(res&&res.skipped){ this.notify("未配置 API Key，已跳过"); } else { this.detail=res; this.notify("AI 提取完成"); } } catch(e){ this.notify(e.message,true); } this.aiExtracting=false; },
    summarize: async function (id) { if(!this.hasApiKey){ this.notify("未配置 API Key",true); return; } this.summarizing=true; try { var p=await this.api("/api/papers/"+id+"/summarize",{method:"POST"}); this.detail=p; this.notify("摘要已生成"); } catch(e){ this.notify(e.message,true); } this.summarizing=false; },
    reExtract: async function (id) { try { var p=await this.api("/api/papers/"+id+"/re-extract",{method:"POST"}); this.detail=p; this.notify("全文已重新提取"); } catch(e){ this.notify(e.message,true); } },
    reDetect: async function (id) { this.notify("正在重新识别元数据..."); try { var p=await this.api("/api/papers/"+id+"/re-detect",{method:"POST"}); this.detail=p; this.notify("元数据已重新识别"); this.loadPapers(); } catch(e){ this.notify(e.message,true); } },
    exportBib: function () { var q=[]; function add(k,v){ if(v!=="") q.push(k+"="+encodeURIComponent(v)); } add("search",this.search); add("status",this.statusFilter); add("category",this.categoryFilter); add("tags",this.tagFilter); add("collections",this.collectionFilter); window.open(this.authUrl("/api/papers/export.bib?"+q.join("&")), "_blank"); },
    ask: async function () { if(!this.question.trim()) return; this.asking=true; this.answer=""; this.sources=[]; try { var res=await this.api("/api/ask",{method:"POST",json:{query:this.question}}); this.answer=res.answer; this.sources=res.sources||[]; } catch(e){ this.notify(e.message,true); } this.asking=false; },
    openManage: function () { this.showManage=true; },
    exportAllNotes: function () { window.open(this.authUrl("/api/papers/export/notes"), "_blank"); },
    exportBackup: function () { this.notify("正在生成备份，稍候会自动下载…"); window.open(this.authUrl("/api/backup"), "_blank"); },
    addCategory: async function () {
      var name = (this.newCategory || "").trim();
      if (!name) return;
      try { await this.api("/api/categories", { method: "POST", json: { name: name } }); this.newCategory = ""; await this.loadTaxonomy(); this.notify("分类已添加"); }
      catch (e) { this.notify(e.message, true); }
    },
    addTag: async function () {
      var name = (this.newTag || "").trim();
      if (!name) return;
      try { await this.api("/api/tags", { method: "POST", json: { name: name } }); this.newTag = ""; await this.loadTaxonomy(); this.notify("标签已添加"); }
      catch (e) { this.notify(e.message, true); }
    },
    addCollection: async function () {
      var name = (this.newCollection || "").trim();
      if (!name) return;
      try { await this.api("/api/collections", { method: "POST", json: { name: name } }); this.newCollection = ""; await this.loadTaxonomy(); this.notify("合集已添加"); }
      catch (e) { this.notify(e.message, true); }
    },
    // 分类/标签/合集列表：三个接口一起拉，避免分散在各处的重复请求
    loadTaxonomy: async function () {
      try {
        var res = await Promise.all([
          this.api("/api/categories"), this.api("/api/tags"), this.api("/api/collections")
        ]);
        this.categories = res[0] || [];
        this.tags = res[1] || [];
        this.collections = res[2] || [];
      } catch (e) { /* 保留原值 */ }
    },
    // ---------- 合集页面 ----------
    // 加载合集列表 + 每篇论文的合集归属（用于卡片预览），并处理 #/collections/{id}
    loadCollectionsPage: async function (hash) {
      try {
        var res = await Promise.all([
          this.api("/api/collections"),
          this.api("/api/papers?page=1&pageSize=100&sort=created&order=desc")
        ]);
        this.collections = res[0] || [];
        var papers = (res[1] && res[1].papers) || [];
        this.allPapersForCollection = papers;
        var map = {};
        papers.forEach(function (p) {
          (p.collections || []).forEach(function (c) {
            (map[c.id] = map[c.id] || []).push(p);
          });
        });
        this.collectionPapersMap = map;
      } catch (e) { this.notify(e.message, true); }

      var m = (hash || "").match(/^#\/collections\/(\d+)/);
      if (m) {
        var id = parseInt(m[1], 10);
        // 合集被删掉时回到列表
        var exists = (this.collections || []).some(function (c) { return c.id === id; });
        if (exists) {
          this.currentCollectionId = id;
          this.collectionPapers = this.collectionPapersMap[id] || [];
          this.refreshCollectionPapers(id);
          return;
        }
      }
      this.currentCollectionId = 0;
      this.collectionPapers = [];
      this.collectionPaperSearch = "";
    },
    // 单独拉一次该合集的论文（列表接口有 100 条上限，这里保证完整）
    refreshCollectionPapers: async function (id) {
      try {
        var res = await this.api("/api/papers?page=1&pageSize=100&collections=" + id);
        this.collectionPapers = (res && res.papers) || [];
      } catch (e) { this.notify(e.message, true); }
    },
    openCollection: function (c) {
      window.location.hash = "#/collections/" + c.id;
    },
    closeCollection: function () {
      window.location.hash = "#/collections";
    },
    addCollectionFromPage: async function () {
      var name = (this.newCollection || "").trim();
      if (!name) return;
      try {
        var c = await this.api("/api/collections", { method: "POST", json: { name: name } });
        this.newCollection = "";
        await this.loadCollectionsPage(window.location.hash);
        if (c && c.id) this.openCollection(c);
      } catch (e) { this.notify(e.message, true); }
    },
    // 改名（用输入框，避免原生 prompt 在部分环境被拦截）
    renameCollectionPrompt: async function (c) {
      var name = (window.prompt("合集名称", c.name) || "").trim();
      if (!name || name === c.name) return;
      try {
        await this.api("/api/collections/" + c.id, { method: "PATCH", json: { name: name, description: c.description || "" } });
        await this.loadCollectionsPage(window.location.hash);
        this.notify("已改名");
      } catch (e) { this.notify(e.message, true); }
    },
    editCollectionDesc: async function (c) {
      var desc = window.prompt("合集简介（可留空）", c.description || "");
      if (desc === null) return;
      try {
        await this.api("/api/collections/" + c.id, { method: "PATCH", json: { name: c.name, description: desc } });
        await this.loadCollectionsPage(window.location.hash);
        this.notify("简介已更新");
      } catch (e) { this.notify(e.message, true); }
    },
    addPaperToCurrentCollection: async function (p) {
      if (!this.currentCollectionId) return;
      try {
        await this.api("/api/papers/" + p.id + "/collections", { method: "POST", json: { CollectionID: this.currentCollectionId } });
        this.collectionPaperSearch = "";
        await this.loadCollectionsPage(window.location.hash);
        await this.refreshCollectionPapers(this.currentCollectionId);
        this.notify("已加入合集");
      } catch (e) { this.notify(e.message, true); }
    },
    removePaperFromCurrentCollection: async function (p) {
      if (!this.currentCollectionId) return;
      try {
        await this.api("/api/papers/" + p.id + "/collections/" + this.currentCollectionId, { method: "DELETE" });
        await this.loadCollectionsPage(window.location.hash);
        await this.refreshCollectionPapers(this.currentCollectionId);
        this.notify("已移出合集");
      } catch (e) { this.notify(e.message, true); }
    },
    // ---------- 改名（内联编辑） ----------
    renameStart: function (kind, item) { this.taxEdit = { id: item.id, kind: kind, name: item.name }; },
    cancelRename: function () { this.taxEdit = { id: 0, kind: "", name: "" }; },
    saveRename: async function (kind, id) {
      var name = (this.taxEdit.name || "").trim();
      if (!name) { this.notify("名称不能为空", true); return; }
      var url = kind === "category" ? "/api/categories/" : kind === "tag" ? "/api/tags/" : "/api/collections/";
      try {
        await this.api(url + id, { method: "PATCH", json: { name: name } });
        this.cancelRename();
        await this.loadTaxonomy();
        if (this.detail && this.detail.id) this.detail = await this.api("/api/papers/" + this.detail.id);
        this.notify("已改名");
      } catch (e) { this.notify(e.message, true); }
    },
    // ---------- 查看：把论文列表筛选到这一项 ----------
    viewTaxonomy: function (kind, item) {
      this.categoryFilter = ""; this.tagFilter = ""; this.collectionFilter = "";
      if (kind === "category") this.categoryFilter = String(item.id);
      else if (kind === "tag") this.tagFilter = String(item.id);
      else this.collectionFilter = String(item.id);
      this.showManage = false;
      this.route = "list";
      if (window.location.hash !== "#/papers") window.location.hash = "#/papers";
      this.applyFilters();
      this.notify("已按「" + item.name + "」筛选");
    },
    deleteCategory: async function (id, name, count) {
      var msg = "删除分类「" + (name || "") + "」？" +
        (count ? "\n其中 " + count + " 篇论文会变成「未分类」，论文本身不会被删除。" : "\n该分类下没有论文。");
      if (!window.confirm(msg)) return;
      try { await this.api("/api/categories/" + id, { method: "DELETE" }); await this.loadTaxonomy(); if (this.route === "list") this.loadPapers(); this.notify("分类已删除"); }
      catch (e) { this.notify(e.message, true); }
    },
    deleteTag: async function (id, name, count) {
      var msg = "删除标签「" + (name || "") + "」？" +
        (count ? "\n会从 " + count + " 篇论文上移除这个标签，论文本身不会被删除。" : "\n该标签还没有被使用。");
      if (!window.confirm(msg)) return;
      try { await this.api("/api/tags/" + id, { method: "DELETE" }); await this.loadTaxonomy(); if (this.route === "list") this.loadPapers(); this.notify("标签已删除"); }
      catch (e) { this.notify(e.message, true); }
    },
    deleteCollection: async function (id, name, count) {
      var msg = "删除合集「" + (name || "") + "」？" +
        (count ? "\n会从 " + count + " 篇论文上移除这个合集，论文本身不会被删除。" : "\n该合集里还没有论文。");
      if (!window.confirm(msg)) return;
      try { await this.api("/api/collections/" + id, { method: "DELETE" }); await this.loadTaxonomy(); if (this.route === "list") this.loadPapers(); this.notify("合集已删除"); }
      catch (e) { this.notify(e.message, true); }
    },
    testAI: async function () {
      this.testingAI = true; this.aiTestResult = "";
      try { var res = await this.api("/api/ai/test", { method: "POST" }); this.aiTestResult = "连接成功 · " + res.model; this.notify("AI 连接正常"); } catch (e) { this.aiTestResult = e.message; this.notify(e.message, true); }
      this.testingAI = false;
    },
    openSettings: function () { this.showSettings=true; this.aiTestResult=""; this.loadAll(); this.refreshAIModels(); },
    refreshAIModels: async function () {
      this.aiModelsLoading = true;
      try {
        var base = (this.settings.aiBaseUrl || "").trim();
        var res = await this.api("/api/ai/models" + (base ? "?base=" + encodeURIComponent(base) : ""));
        this.aiModels = res.models || [];
        if (res.baseUrl) this.settings.aiBaseUrl = res.baseUrl;
        if (res.warning) this.notify(res.warning, true);
      } catch (e) { this.notify(e.message, true); }
      this.aiModelsLoading = false;
      var cur = this.settings.aiModel || "";
      var inList = this.aiModels.some(function (m) { return m.id === cur; });
      this.customModel = !!cur && !inList; // 已存模型不在列表（网关换过/手输过）→ 手动输入模式
    },
    onModelChange: function () {
      if (this.settings.aiModel === "__custom__") {
        this.customModel = true;
        this.settings.aiModel = "";
      }
    },
    saveSettings: async function () { var body={aiBaseUrl:this.settings.aiBaseUrl, aiModel:this.settings.aiModel}; if(this.settings.aiApiKey) body.aiApiKey=this.settings.aiApiKey; try { var res=await this.api("/api/settings",{method:"PUT",json:body}); this.hasApiKey=!!res.hasApiKey; this.settings.aiApiKey=""; this.notify("AI 设置已保存"); this.showSettings=false; } catch(e){ this.notify(e.message,true); } },
    clearApiKey: async function () { try { var res=await this.api("/api/settings",{method:"PUT",json:{clearApiKey:true,aiBaseUrl:this.settings.aiBaseUrl,aiModel:this.settings.aiModel}}); this.hasApiKey=!!res.hasApiKey; this.settings.aiApiKey=""; this.notify("API Key 已清除"); } catch(e){ this.notify(e.message,true); } },
    // ---------- 上传（XHR 进度，对齐老项目） ----------
    onDrop: function (ev) { var f=ev.dataTransfer && ev.dataTransfer.files && ev.dataTransfer.files[0]; if(f) this.doUpload(f); },
    onFileChange: function (ev) { var f=ev.target && ev.target.files && ev.target.files[0]; if(f) this.doUpload(f); },
    doUpload: function (file, force) {
      var self=this; var fd=new FormData(); fd.append("pdf",file,file.name); fd.append("useAI",this.hasApiKey?"1":"0");
      if(force) fd.append("force","1");   // 必须建在本次 FormData 上：之前 append 到旧对象，导致重复上传无限弹窗
      this.uploading=true; this.uploadProgress=0; this.uploadResult=null;
      var xhr=new XMLHttpRequest(); xhr.open("POST","/api/papers");
      if(this.token) xhr.setRequestHeader("Authorization","Bearer "+this.token);
      xhr.upload.onprogress=function(e){ if(e.lengthComputable) self.uploadProgress=Math.round(e.loaded/e.total*100); };
      xhr.onload=function(){
        self.uploading=false;
        var res=null; try{ res=JSON.parse(xhr.responseText); }catch(e){}
        if(xhr.status===201){ self.uploadResult=res; self.notify("收录完成"); self.loadAll(); self.loadPapers(); }
        else if(xhr.status===409 && res && res.duplicate){ if(confirm("检测到重复论文："+res.title+"。仍然添加吗？")){ self.doUpload(file, true); } else { self.notify("已取消",true); } }
        else { self.notify((res&&res.error)||("上传失败 "+xhr.status),true); }
      };
      xhr.onerror=function(){ self.uploading=false; self.notify("网络错误",true); };
      xhr.send(fd);
    },
    resetUpload: function () { this.uploadResult=null; this.uploadProgress=0; if(this.$refs.fileInput) this.$refs.fileInput.value=""; },
    // PDF 阅读面板全屏（原模板绑定了该方法但从未实现）
    toggleFullscreen: function () {
      var el=this.$refs.pdfPanel; if(!el) return;
      var doc=document;
      var current=doc.fullscreenElement||doc.webkitFullscreenElement;
      if(current){ (doc.exitFullscreen||doc.webkitExitFullscreen).call(doc); return; }
      var req=el.requestFullscreen||el.webkitRequestFullscreen;
      if(req){ var r=req.call(el); if(r&&r.catch) r.catch(function(){}); }
      else this.notify("当前浏览器不支持全屏", true);
    },
    // 全屏状态跟踪：不依赖 :fullscreen 伪类（Safari 前缀支持不一致），
    // 直接由 fullscreenchange 事件驱动悬浮笔记窗的显隐
    // 进入全屏/全宽时自动切到阅读模式（有全文的前提下）
    toggleFullWidth: function () {
      this.fullWidth = !this.fullWidth;
      // 全宽/全屏后容器宽度变化，PDF 需要按新宽度重排（高亮会跟着重新定位）
      var self = this;
      this.$nextTick(function () { setTimeout(function () { self.layoutPdf(false); }, 120); });
    },
    syncFullscreen: function () {
      var doc=document;
      var on=!!(doc.fullscreenElement||doc.webkitFullscreenElement);
      this.pdfFullscreen=on;
      if(!on){ this.fsNotesMin=false; }
    },
    // ---------- 登录 ----------
    doLogin: async function () {
      var pass=this.loginPass; if(!pass) return;
      try {
        var res=await fetch("/api/health",{headers:{Authorization:"Bearer "+pass}});
        if(res.ok){ this.token=pass; localStorage.setItem("pm-token",pass); this.loginError=""; this.route="list"; this.loadAll(); this.loadPapers(); }
        else { this.loginError="密码错误"; }
      } catch(e){ this.loginError="无法连接服务器"; }
    },
    logout: function () { this.token=""; localStorage.removeItem("pm-token"); this.route="login"; },
  },
  mounted: function () {
    var self=this;
    this.setTheme(); // 恢复上次选择的主题
    this.loadNotesPanelPrefs(); // 恢复笔记浮窗位置与固定状态
    try { this.notesPreview = localStorage.getItem("pm-notes-preview") === "1"; } catch (e) {}
    window.addEventListener("hashchange", function(){ self.parseHash(); });
    window.addEventListener("resize", function(){ self.clampNotesPos(); });
    var fsSync=function(){
      self.syncFullscreen();
    };
    document.addEventListener("fullscreenchange", fsSync);
    document.addEventListener("webkitfullscreenchange", fsSync);
    // Esc 关闭最上层浮层（由内到外，一次只关一层）
    document.addEventListener("keydown", function(e){
      // Alt+T：翻译当前选中的文字（与浮层按钮等价）
      if (e.altKey && (e.key === "t" || e.key === "T")) {
        var sel = window.getSelection();
        if (sel && !sel.isCollapsed && self._pdf && self._pdf.el && self._pdf.el.contains(sel.anchorNode)) {
          e.preventDefault();
          self.onPdfMouseUp();
          setTimeout(function () { if (self.annoPopup.show) self.translateSelection(); }, 40);
        }
        return;
      }
      if (e.key !== "Escape") return;
      // 正在内联改名时先取消改名，不要顺手把整个管理弹窗关掉
      if (self.taxEdit && self.taxEdit.id) { self.cancelRename(); return; }
      if (self.transResult.show) { self.transResult.show = false; return; }
      if (self.annoPopup.show) { self.annoPopup.show = false; return; }
      if (self.pdfFullscreen) { self.pdfFullscreen = false; return; }
      if (self.showPaperModal) { self.showPaperModal = false; return; }
      if (self.showSettings) { self.showSettings = false; return; }
      if (self.showTrash) { self.showTrash = false; return; }
      if (self.showManage) { self.showManage = false; return; }
      if (self.askOpen) { self.askOpen = false; return; }
      if (self.themeOpen) { self.themeOpen = false; return; }
    });
    this.parseHash();
    // 列表路由的 parseHash 内部已经触发 loadAll；只有详情/上传/图谱等路由要在这里补一次，
    // 否则首屏会对 categories/tags/collections/stats/settings 各请求两遍
    if (this.route !== "list") this.loadAll();
    this.loadPapers();
  }
};

// 合并同页同行的矩形：一次选择会被 pdf.js 拆成很多 spans，逐行合并后重叠更自然
function mergeRects(rects) {
  if (!rects.length) return [];
  var byPage = {};
  rects.forEach(function (r) {
    (byPage[r.p] = byPage[r.p] || []).push(r);
  });
  var out = [];
  Object.keys(byPage).forEach(function (key) {
    var list = byPage[key].slice().sort(function (a, b) { return a.y - b.y || a.x - b.x; });
    var cur = null;
    list.forEach(function (r) {
      if (cur && Math.abs(cur.y - r.y) < 0.004 && Math.abs(cur.h - r.h) < 0.004 &&
          r.x <= cur.x + cur.w + 0.01) {
        var right = Math.max(cur.x + cur.w, r.x + r.w);
        cur.x = Math.min(cur.x, r.x);
        cur.w = right - cur.x;
        return;
      }
      if (cur) out.push(cur);
      cur = { p: r.p, x: r.x, y: r.y, w: r.w, h: r.h };
    });
    if (cur) out.push(cur);
  });
  return out;
}

// ---------- 阅读模式渲染辅助 ----------
// 全文是纯文本：先转义再拼 HTML，避免注入
function escHtml(s) {
  return String(s)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

// 把一个片段渲染为 HTML：按标注范围切分并包 <mark>
// 片段与标注都在「全文 UTF-16 偏移」坐标系里，与后端 SplitReading 一致
function renderSegmentHtml(seg, annos) {
  var base = seg.start;
  var text = seg.text || "";
  var end = base + text.length;
  var pieces = [];
  var pos = base;
  for (var i = 0; i < annos.length; i++) {
    var a = annos[i];
    var as = Math.max(a.start, base, pos);   // 不允许与已有标注重叠（防止嵌套 mark）
    var ae = Math.min(a.end, end);
    if (ae <= as) continue;
    if (as > pos) pieces.push(escHtml(text.slice(pos - base, as - base)));
    pieces.push('<mark class="anno anno--' + escHtml(a.color) + '" data-id="' + a.id + '">' +
      escHtml(text.slice(as - base, ae - base)) + '</mark>');
    pos = ae;
  }
  if (pos < end) pieces.push(escHtml(text.slice(pos - base)));
  return pieces.join("");
}

Root.template = document.getElementById("app-template").innerHTML;
// v-focus：元素出现时自动聚焦（改名输入框用），文本框顺便全选，改起来更快
var app = Vue.createApp(Root);
app.directive("focus", {
  mounted: function (el) {
    if (!el || !el.focus) return;
    el.focus();
    if (el.select && el.value !== undefined) { try { el.select(); } catch (e) {} }
  }
});
app.mount("#app");
