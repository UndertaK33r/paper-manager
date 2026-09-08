function makeEmptyForm() {
  return { title: "", authors: "", year: "", venue: "", doi: "", keywords: "", link: "", summary: "", notes: "", categoryId: "", tags: "", collections: "", read: false, starred: false, useAI: true };
}

var Root = {
  data: function () {
    return {
      route: "list", token: localStorage.getItem("pm-token") || "", loginPass: "", loginError: "",
      theme: localStorage.getItem("pm-theme") || "", themeOpen: false,
      themeOptions: [{ v: "", t: "黑白极简" }, { v: "midnight", t: "深夜玻璃" }, { v: "paper", t: "纸墨衬线" }],
      stats: {}, categories: [], tags: [], collections: [],
      papers: [], total: 0, page: 1, pages: 1, pageSize: 20, loading: true,
      view: "table", search: "", statusFilter: "", categoryFilter: "", tagFilter: "", sort: "created", order: "desc",
      askOpen: false, question: "", answer: "", sources: [], asking: false,
      detail: {}, tagSelect: "", collectionSelect: "", fullWidth: false,
      pdfFullscreen: false, fsNotesMin: false, notesSavedAt: "",
      summaryExpanded: false, summaryOverflow: false,
      pdfJsOk: false, pdfJsFailed: false,
      selPopup: { show: false, x: 0, y: 0, text: "", loading: false }, selResult: null,
      fsTransMin: false, transHistory: [], instantSrc: "", instantLoading: false,
      showPaperModal: false, editingId: null, saving: false, form: makeEmptyForm(),
      aiExtracting: false, summarizing: false, pdfExtracting: false,
      showManage: false, newCategory: "", newTag: "", newCollection: "",
      showSettings: false, settings: { aiBaseUrl: "https://tokendance.space/gateway/v1", aiModel: "deepseek-v3.2", aiApiKey: "" }, hasApiKey: false, testingAI: false, aiTestResult: "",
      aiModels: [], aiModelsLoading: false,
      uploading: false, uploadProgress: 0, uploadResult: null,
      graphNodes: [], graphEdges: [], graphWarning: "", graphTimer: null, dragNode: null,
      toast: { show: false, msg: "", error: false }
    };
  },
  computed: {
    readPct: function () { if (!this.stats.total) return 0; return Math.round(this.stats.read / this.stats.total * 100); },
    statusOptions: function () { return [{v:"",t:"全部"},{v:"unread",t:"待读"},{v:"reading",t:"在读"},{v:"read",t:"已读"}]; },
    availableTags: function () { var self=this; return this.tags.filter(function(t){ return !(self.detail.tags||[]).some(function(x){return x.id===t.id;}); }); },
    availableCollections: function () { var self=this; return this.collections.filter(function(c){ return !(self.detail.collections||[]).some(function(x){return x.id===c.id;}); }); },
    renderedSummary: function () { return window.mdRender ? window.mdRender(this.detail.summary) : ""; },
    renderedAnswer: function () { return window.mdRender ? window.mdRender(this.answer) : ""; }
  },
  watch: {
    // 摘要变化（打开新论文 / AI 重新生成）时重置展开态并重新测量是否溢出
    "detail.summary": function () {
      var self = this;
      this.summaryExpanded = false;
      this.$nextTick(function () { self.checkSummaryOverflow(); });
    },
    // 全宽切换后容器宽度变化，重排 PDF 页面
    "fullWidth": function () {
      var self = this;
      this.$nextTick(function () { self.layoutPdfPages(true); });
    }
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
    pdfUrl: function (id) { return "/api/papers/" + id + "/pdf"; },
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
      else if (h.indexOf("#/graph") === 0) { this.route="graph"; this.loadGraph(false); }
      else if (h.indexOf("#/login") === 0) { this.route="login"; }
      else { this.route="list"; }
      if (this.route === "list") this.loadAll();
    },
    loadAll: async function () {
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
      add("category",this.categoryFilter); add("tags",this.tagFilter); add("sort",this.sort); add("order",this.order);
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
    openAdd: function () { this.editingId=null; this.form=makeEmptyForm(); if(this.$refs.pdfInput) this.$refs.pdfInput.value=""; this.showPaperModal=true; },
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
      if(!confirm("确定删除这篇论文吗？此操作不可恢复。")) return;
      try { await this.api("/api/papers/"+id,{method:"DELETE"}); this.notify("已删除"); this.go("list"); this.loadAll(); this.loadPapers(); } catch (e) { this.notify(e.message,true); }
    },
    openDetail: async function (id) {
      this.destroyPdfViewer(); this.pdfJsFailed=false;
      this.detail={}; this.route="detail"; window.location.hash="#/papers/"+id;
      this.transHistory=[]; this.selResult=null; this.selPopup.show=false; this.instantSrc="";
      try { this.detail=await this.api("/api/papers/"+id); this.tagSelect=""; this.collectionSelect=""; this.fsNotesMin=false; this.notesSavedAt=""; } catch (e) { this.notify(e.message,true); }
      if (this.detail.hasPdf) {
        var self=this; this.$nextTick(function(){ self.setupPdfViewer(); });
      }
    },
    // ---------- 划词翻译（PDF.js 文本层渲染 + 选区浮出翻译按钮） ----------
    setupPdfViewer: async function () {
      this.destroyPdfViewer();
      if (!this.detail.hasPdf || !window.pdfjsLib) return;
      var self=this;
      var container=this.$refs.pdfView;
      if (!container) return;
      pdfjsLib.GlobalWorkerOptions.workerSrc = "/assets/pdfjs/pdf.worker.min.js";
      var task = pdfjsLib.getDocument({ url: this.pdfUrl(this.detail.id) });
      this._pdf = { task: task, doc: null, pages: [], container: container, laid: false };
      try {
        var doc = await task.promise;
        if (!this._pdf || this._pdf.task !== task) return; // 期间已切换论文
        this._pdf.doc = doc;
        container.innerHTML = "";
        for (var n = 1; n <= doc.numPages; n++) {
          var wrap = document.createElement("div");
          wrap.className = "pjs-page";
          wrap.dataset.page = n;
          var canvas = document.createElement("canvas");
          var text = document.createElement("div");
          text.className = "pjs-text";
          wrap.appendChild(canvas); wrap.appendChild(text);
          container.appendChild(wrap);
          this._pdf.pages.push({ n: n, wrap: wrap, canvas: canvas, text: text, rendered: false, h0: 0 });
        }
        container.addEventListener("scroll", function () { self.selPopup.show = false; self.selResult = null; });
        this.pdfJsOk = true;
        await this.layoutPdfPages(false);
        var io = new IntersectionObserver(function (entries) {
          entries.forEach(function (en) {
            if (en.isIntersecting) self.renderPdfPage(+en.target.dataset.page);
          });
        }, { root: container, rootMargin: "500px 0px" });
        this._pdf.observer = io;
        this._pdf.pages.forEach(function (p) { io.observe(p.wrap); });
      } catch (e) {
        this.pdfJsFailed = true; // 回退浏览器内置阅读器
        this.pdfJsOk = false;
      }
    },
    destroyPdfViewer: function () {
      var P = this._pdf;
      if (P) {
        if (P.observer) P.observer.disconnect();
        if (P.task) { try { P.task.destroy(); } catch (e) {} }
        if (P.container) P.container.innerHTML = "";
      }
      this._pdf = null;
      this.pdfJsOk = false;
    },
    layoutPdfPages: async function (force) {
      var P = this._pdf; if (!P || !P.doc || !P.container) return;
      var self = this;
      var width = P.container.clientWidth - 20;
      if (width < 220) width = 220;
      if (P.laid && !force && Math.abs((P.width || 0) - width) < 8) return; // 宽度没变不重排
      P.width = width;
      // 取每页基准尺寸，设占位高度（避免渲染后布局跳动）
      for (var i = 0; i < P.pages.length; i++) {
        var p = P.pages[i];
        var page = await P.doc.getPage(p.n);
        var base = page.getViewport({ scale: 1 });
        p.h0 = base.height;
        var w0 = base.width;
        p.scale = width / w0;
        p.wrap.style.width = width + "px";
        p.wrap.style.height = Math.round(base.height * p.scale) + "px";
        p.rendered = false;
        p.canvas.width = 0; p.canvas.height = 0;
        p.text.innerHTML = "";
      }
      // 立即渲染当前视口内的页
      var st = P.container.scrollTop, vh = P.container.clientHeight;
      P.pages.forEach(function (p) {
        var top = p.wrap.offsetTop;
        if (top + p.wrap.offsetHeight > st - 600 && top < st + vh + 600) self.renderPdfPage(p.n);
      });
    },
    renderPdfPage: async function (n) {
      var P = this._pdf; if (!P || !P.doc) return;
      var p = P.pages[n - 1]; if (!p || p.rendered) return;
      p.rendered = true;
      try {
        var page = await P.doc.getPage(n);
        var width = P.container.clientWidth - 20;
        var scale = p.scale || (width / page.getViewport({ scale: 1 }).width);
        var dpr = Math.min(window.devicePixelRatio || 1, 2);
        var vp = page.getViewport({ scale: scale * dpr });
        p.canvas.width = Math.round(vp.width);
        p.canvas.height = Math.round(vp.height);
        await page.render({ canvasContext: p.canvas.getContext("2d"), viewport: vp }).promise;
        var tc = await page.getTextContent();
        p.text.innerHTML = "";
        var tl = pdfjsLib.renderTextLayer({ textContentSource: tc, container: p.text, viewport: page.getViewport({ scale: scale }) });
        await tl.promise;
      } catch (e) { p.rendered = false; }
    },
    onPdfMouseUp: function () {
      var self = this;
      setTimeout(function () {
        var sel = window.getSelection();
        if (!sel || sel.isCollapsed || !self._pdf || !self.pdfJsOk) { self.selPopup.show = false; return; }
        var text = sel.toString().trim();
        if (text.length < 2) { self.selPopup.show = false; return; }
        var node = sel.anchorNode;
        var el = node && (node.nodeType === 3 ? node.parentElement : node);
        if (!el || !el.closest || !el.closest(".pjs-text")) { self.selPopup.show = false; return; }
        var rect = sel.getRangeAt(0).getBoundingClientRect();
        if (!rect || (!rect.width && !rect.height)) { self.selPopup.show = false; return; }
        var x = Math.min(Math.max(rect.left, 8), window.innerWidth - 90);
        var y = rect.top - 46; if (y < 8) y = rect.bottom + 10;
        self.selPopup = { show: true, x: x, y: y, text: text.slice(0, 4000), loading: false };
        self.selResult = null;
      }, 10);
    },
    translateSelection: async function () {
      var text = this.selPopup.text;
      if (!text || this.selPopup.loading) return;
      this.selPopup.loading = true;
      var px = this.selPopup.x, py = this.selPopup.y;
      try {
        var res = await this.api("/api/ai/translate-text", { method: "POST", json: { text: text } });
        var out = (res && res.translation) || "";
        this.addToHistory(text, out);
        var x = Math.min(Math.max(px - 60, 8), Math.max(window.innerWidth - 452, 8));
        var y = Math.min(py + 42, window.innerHeight - 160);
        this.selResult = { x: x, y: y, html: window.mdRender ? window.mdRender(out) : out };
      } catch (e) { this.notify(e.message, true); }
      this.selPopup.show = false; this.selPopup.loading = false;
    },
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
    checkSummaryOverflow: function () {
      var el = this.$refs.summaryBody;
      if (!el) { this.summaryOverflow = false; return; }
      // 折叠态下 clientHeight 被限高，scrollHeight 是完整内容高
      this.summaryOverflow = el.scrollHeight > el.clientHeight + 8;
    },
    saveNotes: async function (silent) {
      if(!this.detail||!this.detail.id) return;
      try {
        var p=await this.api("/api/papers/"+this.detail.id,{method:"PATCH",json:{notes:this.detail.notes||""}});
        if(p && typeof p.notes==="string") this.detail.notes=p.notes;
        var t=new Date(); this.notesSavedAt=("0"+t.getHours()).slice(-2)+":"+("0"+t.getMinutes()).slice(-2);
        if(!silent) this.notify("笔记已保存");
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
    addDetailTag: async function () {
      var name=(this.tagSelect||"").trim(); if(!name||!this.detail.id) return;
      try { this.detail=await this.api("/api/papers/"+this.detail.id+"/tags",{method:"POST",json:{name:name}}); this.tagSelect=""; this.loadAll(); } catch(e){ this.notify(e.message,true); }
    },
    removeDetailTag: async function (id) { if(!this.detail.id) return; try { this.detail=await this.api("/api/papers/"+this.detail.id+"/tags/"+id,{method:"DELETE"}); this.loadAll(); } catch(e){ this.notify(e.message,true); } },
    addDetailCollection: async function () {
      var name=(this.collectionSelect||"").trim(); if(!name||!this.detail.id) return;
      try { this.detail=await this.api("/api/papers/"+this.detail.id+"/collections",{method:"POST",json:{name:name}}); this.collectionSelect=""; this.loadAll(); } catch(e){ this.notify(e.message,true); }
    },
    removeDetailCollection: async function (id) { if(!this.detail.id) return; try { this.detail=await this.api("/api/papers/"+this.detail.id+"/collections/"+id,{method:"DELETE"}); this.loadAll(); } catch(e){ this.notify(e.message,true); } },
    aiExtract: async function (id) { if(!this.hasApiKey){ this.notify("未配置 API Key，已跳过",true); return; } this.aiExtracting=true; try { var res=await this.api("/api/papers/"+id+"/ai-extract",{method:"POST"}); if(res&&res.skipped){ this.notify("未配置 API Key，已跳过"); } else { this.detail=res; this.notify("AI 提取完成"); } } catch(e){ this.notify(e.message,true); } this.aiExtracting=false; },
    summarize: async function (id) { if(!this.hasApiKey){ this.notify("未配置 API Key",true); return; } this.summarizing=true; try { var p=await this.api("/api/papers/"+id+"/summarize",{method:"POST"}); this.detail=p; this.notify("摘要已生成"); } catch(e){ this.notify(e.message,true); } this.summarizing=false; },
    reExtract: async function (id) { try { var p=await this.api("/api/papers/"+id+"/re-extract",{method:"POST"}); this.detail=p; this.notify("全文已重新提取"); } catch(e){ this.notify(e.message,true); } },
    reDetect: async function (id) { this.notify("正在重新识别元数据..."); try { var p=await this.api("/api/papers/"+id+"/re-detect",{method:"POST"}); this.detail=p; this.notify("元数据已重新识别"); this.loadPapers(); } catch(e){ this.notify(e.message,true); } },
    exportBib: function () { var q=[]; function add(k,v){ if(v!=="") q.push(k+"="+encodeURIComponent(v)); } add("search",this.search); add("status",this.statusFilter); add("category",this.categoryFilter); add("tags",this.tagFilter); window.open("/api/papers/export.bib?"+q.join("&"), "_blank"); },
    ask: async function () { if(!this.question.trim()) return; this.asking=true; this.answer=""; this.sources=[]; try { var res=await this.api("/api/ask",{method:"POST",json:{query:this.question}}); this.answer=res.answer; this.sources=res.sources||[]; } catch(e){ this.notify(e.message,true); } this.asking=false; },
    openManage: function () { this.showManage=true; },
    addCategory: async function () { if(!this.newCategory.trim()) return; try { await this.api("/api/categories",{method:"POST",json:{name:this.newCategory}}); this.newCategory=""; this.loadAll(); } catch(e){ this.notify(e.message,true); } },
    deleteCategory: async function (id) { if(!confirm("删除该分类？")) return; try { await this.api("/api/categories/"+id,{method:"DELETE"}); this.loadAll(); this.loadPapers(); } catch(e){ this.notify(e.message,true); } },
    addTag: async function () { if(!this.newTag.trim()) return; try { await this.api("/api/tags",{method:"POST",json:{name:this.newTag}}); this.newTag=""; this.loadAll(); } catch(e){ this.notify(e.message,true); } },
    deleteTag: async function (id) { if(!confirm("删除该标签？")) return; try { await this.api("/api/tags/"+id,{method:"DELETE"}); this.loadAll(); this.loadPapers(); } catch(e){ this.notify(e.message,true); } },
    addCollection: async function () { if(!this.newCollection.trim()) return; try { await this.api("/api/collections",{method:"POST",json:{name:this.newCollection}}); this.newCollection=""; this.loadAll(); } catch(e){ this.notify(e.message,true); } },
    deleteCollection: async function (id) { if(!confirm("删除该合集？")) return; try { await this.api("/api/collections/"+id,{method:"DELETE"}); this.loadAll(); this.loadPapers(); } catch(e){ this.notify(e.message,true); } },
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
      } catch (e) { this.notify(e.message, true); }
      this.aiModelsLoading = false;
    },
    saveSettings: async function () { var body={aiBaseUrl:this.settings.aiBaseUrl, aiModel:this.settings.aiModel}; if(this.settings.aiApiKey) body.aiApiKey=this.settings.aiApiKey; try { var res=await this.api("/api/settings",{method:"PUT",json:body}); this.hasApiKey=!!res.hasApiKey; this.settings.aiApiKey=""; this.notify("AI 设置已保存"); this.showSettings=false; } catch(e){ this.notify(e.message,true); } },
    clearApiKey: async function () { try { var res=await this.api("/api/settings",{method:"PUT",json:{clearApiKey:true,aiBaseUrl:this.settings.aiBaseUrl,aiModel:this.settings.aiModel}}); this.hasApiKey=!!res.hasApiKey; this.settings.aiApiKey=""; this.notify("API Key 已清除"); } catch(e){ this.notify(e.message,true); } },
    // ---------- 上传（XHR 进度，对齐老项目） ----------
    onDrop: function (ev) { var f=ev.dataTransfer && ev.dataTransfer.files && ev.dataTransfer.files[0]; if(f) this.doUpload(f); },
    onFileChange: function (ev) { var f=ev.target && ev.target.files && ev.target.files[0]; if(f) this.doUpload(f); },
    doUpload: function (file) {
      var self=this; var fd=new FormData(); fd.append("pdf",file,file.name); fd.append("useAI",this.hasApiKey?"1":"0");
      this.uploading=true; this.uploadProgress=0; this.uploadResult=null;
      var xhr=new XMLHttpRequest(); xhr.open("POST","/api/papers");
      if(this.token) xhr.setRequestHeader("Authorization","Bearer "+this.token);
      xhr.upload.onprogress=function(e){ if(e.lengthComputable) self.uploadProgress=Math.round(e.loaded/e.total*100); };
      xhr.onload=function(){
        self.uploading=false;
        var res=null; try{ res=JSON.parse(xhr.responseText); }catch(e){}
        if(xhr.status===201){ self.uploadResult=res; self.notify("收录完成"); self.loadAll(); self.loadPapers(); }
        else if(xhr.status===409 && res && res.duplicate){ if(confirm("检测到重复论文："+res.title+"。仍然添加吗？")){ fd.append("force","1"); self.doUpload(file); } else { self.notify("已取消",true); } }
        else { self.notify((res&&res.error)||("上传失败 "+xhr.status),true); }
      };
      xhr.onerror=function(){ self.uploading=false; self.notify("网络错误",true); };
      xhr.send(fd);
    },
    resetUpload: function () { this.uploadResult=null; this.uploadProgress=0; if(this.$refs.fileInput) this.$refs.fileInput.value=""; },
    // ---------- 图谱 ----------
    loadGraph: async function (force) {
      try { var res=await this.api("/api/graph"+(force?"?refresh=1":"")); this.graphWarning=res.warning||""; this.graphEdges=res.edges||[]; this.initSim(res.nodes||[]); } catch(e){ this.notify(e.message,true); }
    },
    initSim: function (nodes) {
      var cx=600, cy=360; var i, n=nodes.length;
      for(i=0;i<n;i++){ var ang=(i/Math.max(1,n))*Math.PI*2; nodes[i].x=cx+Math.cos(ang)*260; nodes[i].y=cy+Math.sin(ang)*260; nodes[i].vx=0; nodes[i].vy=0; }
      this.graphNodes=nodes; if(this.graphTimer) cancelAnimationFrame(this.graphTimer); var self=this;
      var step=function(){ self.simStep(); self.drawGraph(); if(!self.route||self.route==="graph") self.graphTimer=requestAnimationFrame(step); };
      step();
    },
    simStep: function () {
      var nodes=this.graphNodes; var i,j,dx,dy,d;
      for(i=0;i<nodes.length;i++){ for(j=i+1;j<nodes.length;j++){ dx=nodes[i].x-nodes[j].x; dy=nodes[i].y-nodes[j].y; d=Math.max(1,Math.sqrt(dx*dx+dy*dy)); var f=5000/(d*d); var fx=dx/d*f; var fy=dy/d*f; nodes[i].vx+=fx; nodes[i].vy+=fy; nodes[j].vx-=fx; nodes[j].vy-=fy; } }
      var byId={}; nodes.forEach(function(n){ byId[n.id]=n; });
      this.graphEdges.forEach(function(e){ var a=byId[e.source]; var b=byId[e.target]; if(!a||!b) return; dx=b.x-a.x; dy=b.y-a.y; d=Math.max(1,Math.sqrt(dx*dx+dy*dy)); var diff=(d-130)*0.025; var fx=dx/d*diff; var fy=dy/d*diff; a.vx+=fx; a.vy+=fy; b.vx-=fx; b.vy-=fy; });
      nodes.forEach(function(n){ n.vx+=(600-n.x)*0.002; n.vy+=(360-n.y)*0.002; n.vx*=0.86; n.vy*=0.86; var sp=Math.sqrt(n.vx*n.vx+n.vy*n.vy); if(sp>10){ n.vx=n.vx/sp*10; n.vy=n.vy/sp*10; } n.x+=n.vx; n.y+=n.vy; });
    },
    drawGraph: function () {
      var cv=this.$refs.graphCanvas; if(!cv) return; var ctx=cv.getContext("2d");
      ctx.clearRect(0,0,cv.width,cv.height);
      var byId={}; this.graphNodes.forEach(function(n){ byId[n.id]=n; });
      ctx.strokeStyle="rgba(17,17,17,0.4)"; ctx.lineWidth=1;
      this.graphEdges.forEach(function(e){ var a=byId[e.source]; var b=byId[e.target]; if(!a||!b) return; ctx.beginPath(); ctx.moveTo(a.x,a.y); ctx.lineTo(b.x,b.y); ctx.stroke(); });
      var self=this;
      this.graphNodes.forEach(function(n){
        ctx.beginPath(); ctx.arc(n.x,n.y, n.in_library?9:6, 0, Math.PI*2);
        ctx.fillStyle = n.in_library ? "#111111" : "#888888"; ctx.fill();
        ctx.strokeStyle="rgba(125,232,255,0.8)"; ctx.lineWidth=1; ctx.stroke();
        ctx.font="12px JetBrains Mono, monospace"; ctx.fillStyle="#111111";
        ctx.fillText((n.title||n.id).slice(0,24), n.x+12, n.y-6);
      });
    },
    onGraphDown: function (ev) {
      var pos=this.graphPos(ev); var mx=pos.x, my=pos.y;
      this.dragNode=null; for(var i=0;i<this.graphNodes.length;i++){ var n=this.graphNodes[i]; var dx=n.x-mx; var dy=n.y-my; if(dx*dx+dy*dy<400){ this.dragNode=n; break; } }
    },
    onGraphMove: function (ev) {
      if(!this.dragNode) return; var pos=this.graphPos(ev); this.dragNode.x=pos.x; this.dragNode.y=pos.y;
    },
    // 画布 CSS 尺寸与内部分辨率（1200x720）不一致时按比例换算，
    // 否则窄屏上点击/拖拽位置全部偏移
    graphPos: function (ev) {
      var cv=this.$refs.graphCanvas; var rect=cv.getBoundingClientRect();
      var p = (ev.touches && ev.touches[0]) ? ev.touches[0] : ev;
      return { x: (p.clientX-rect.left) * (cv.width/rect.width), y: (p.clientY-rect.top) * (cv.height/rect.height) };
    },
    onGraphTouchStart: function (ev) { ev.preventDefault(); this.onGraphDown(ev); },
    onGraphTouchMove: function (ev) { ev.preventDefault(); this.onGraphMove(ev); },
    onGraphTouchEnd: function (ev) { ev.preventDefault(); this.onGraphUp(ev); },
    onGraphUp: function (ev) {
      var n=this.dragNode; this.dragNode=null;
      if(n && n.id && n.id.indexOf("lib-")===0) { var id=parseInt(n.id.slice(4),10); if(id) this.openDetail(id); }
    },
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
    window.addEventListener("hashchange", function(){ self.parseHash(); });
    var fsSync=function(){
      self.syncFullscreen();
      self.$nextTick(function(){ self.layoutPdfPages(true); });
    };
    document.addEventListener("fullscreenchange", fsSync);
    document.addEventListener("webkitfullscreenchange", fsSync);
    // 划词翻译：mouseup 后检查选区
    document.addEventListener("mouseup", function(){ self.onPdfMouseUp(); });
    var rsz=function(){ self.$nextTick(function(){ self.layoutPdfPages(true); }); };
    window.addEventListener("resize", rsz);
    this.parseHash();
    this.loadAll().then(function(){ self.loadPapers(); });
  }
};

Root.template = document.getElementById("app-template").innerHTML;
var app = Vue.createApp(Root);
app.mount("#app");
