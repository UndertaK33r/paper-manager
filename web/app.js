function makeEmptyForm() {
  return { title: "", authors: "", year: "", venue: "", doi: "", keywords: "", link: "", summary: "", notes: "", categoryId: "", tags: "", collections: "", read: false, starred: false, useAI: true };
}

var Root = {
  data: function () {
    return {
      stats: {}, categories: [], tags: [], collections: [],
      papers: [], total: 0, page: 1, pages: 1, pageSize: 20, loading: true,
      view: "table",
      filters: { search: "", category: "", tag: "", collection: "", yearFrom: "", yearTo: "", read: "", starred: "", sort: "created", order: "desc" },
      showPaperModal: false, editingId: null, saving: false, form: makeEmptyForm(),
      showDetail: false, detail: {}, tagSelect: "", collectionSelect: "", aiExtracting: false, summarizing: false, pdfExtracting: false,
      showManage: false, newCategory: "", newTag: "", newCollection: "",
      showSettings: false,
      settings: { aiBaseUrl: "https://api.deepseek.com/v1", aiModel: "deepseek-v4-flash", aiApiKey: "" },
      hasApiKey: false,
      toast: { show: false, msg: "", error: false }
    };
  },
  computed: {
    availableTags: function () {
      var self = this;
      return this.tags.filter(function (t) {
        return !(self.detail.tags || []).some(function (x) { return x.id === t.id; });
      });
    },
    availableCollections: function () {
      var self = this;
      return this.collections.filter(function (c) {
        return !(self.detail.collections || []).some(function (x) { return x.id === c.id; });
      });
    }
  },
  methods: {
    notify: function (msg, error) {
      var self = this;
      this.toast.show = true; this.toast.msg = msg; this.toast.error = !!error;
      clearTimeout(this._toastTimer);
      this._toastTimer = setTimeout(function () { self.toast.show = false; }, 2600);
    },
    api: async function (path, opts) {
      opts = opts || {}; opts.headers = opts.headers || {};
      if (opts.json) { opts.headers["Content-Type"] = "application/json"; opts.body = JSON.stringify(opts.json); delete opts.json; }
      var res = await fetch(path, opts);
      var data = null;
      try { data = await res.json(); } catch (e) {}
      if (!res.ok) {
        var err = new Error((data && data.error) || ("请求失败 " + res.status));
        err.status = res.status; err.data = data; throw err;
      }
      return data;
    },
    loadAll: async function () {
      try {
        var results = await Promise.all([
          this.api("/api/categories"), this.api("/api/tags"), this.api("/api/collections"), this.api("/api/stats"), this.api("/api/settings")
        ]);
        this.categories = results[0]; this.tags = results[1]; this.collections = results[2]; this.stats = results[3];
        var st = results[4];
        this.hasApiKey = !!st.hasApiKey;
        this.settings.aiBaseUrl = st.aiBaseUrl || this.settings.aiBaseUrl;
        this.settings.aiModel = st.aiModel || this.settings.aiModel;
      } catch (e) { this.notify(e.message, true); }
    },
    queryString: function () {
      var q = []; var f = this.filters;
      function add(k, v) { if (v !== null && v !== undefined && v !== "") q.push(k + "=" + encodeURIComponent(v)); }
      add("page", this.page); add("pageSize", this.pageSize); add("search", f.search);
      add("category", f.category); add("tags", f.tag); add("collections", f.collection);
      add("yearFrom", f.yearFrom); add("yearTo", f.yearTo); add("read", f.read); add("starred", f.starred);
      add("sort", f.sort); add("order", f.order);
      return "/api/papers?" + q.join("&");
    },
    loadPapers: async function () {
      this.loading = true;
      try {
        var res = await this.api(this.queryString());
        this.papers = res.papers || []; this.total = res.total; this.page = res.page;
        this.pages = Math.max(1, Math.ceil(res.total / res.pageSize));
      } catch (e) { this.notify(e.message, true); }
      this.loading = false;
    },
    onSearchInput: function () {
      var self = this;
      clearTimeout(this._searchTimer);
      this._searchTimer = setTimeout(function () { self.filters.search = self.filters.search.trim(); self.page = 1; self.loadPapers(); }, 300);
    },
    applyFilters: function () { this.page = 1; this.loadPapers(); },
    setView: function (v) { this.view = v; },
    goPage: function (p) { if (p < 1 || p > this.pages) return; this.page = p; this.loadPapers(); },
    openAdd: function () {
      this.editingId = null; this.form = makeEmptyForm(); this.pdfExtracting = false;
      if (this.$refs.pdfInput) this.$refs.pdfInput.value = "";
      this.showPaperModal = true;
    },
    openEdit: function (p) {
      this.editingId = p.id;
      this.form = {
        title: p.title || "", authors: p.authors || "", year: p.year || "", venue: p.venue || "",
        doi: p.doi || "", keywords: p.keywords || "", link: p.link || "", summary: p.summary || "",
        notes: p.notes || "", categoryId: p.categoryId || "",
        tags: (p.tags || []).map(function (t) { return t.name; }).join(", "),
        collections: (p.collections || []).map(function (c) { return c.name; }).join(", "),
        read: !!p.read, starred: !!p.starred, useAI: true
      };
      this.pdfExtracting = false;
      if (this.$refs.pdfInput) this.$refs.pdfInput.value = "";
      this.showDetail = false; this.showPaperModal = true;
    },
    closePaperModal: function () { this.showPaperModal = false; },

    onPdfChange: async function () {
      var el = this.$refs.pdfInput;
      if (!el || !el.files || !el.files[0]) return;
      this.pdfExtracting = true;
      var fd = new FormData();
      fd.append("pdf", el.files[0], el.files[0].name);
      fd.append("useAI", this.form.useAI ? "1" : "0");
      try {
        var m = await this.api("/api/papers/extract-pdf", { method: "POST", body: fd });
        if (!this.form.title) this.form.title = m.title || "";
        if (!this.form.authors) this.form.authors = m.authors || "";
        if (!this.form.year) this.form.year = m.year || "";
        if (!this.form.venue) this.form.venue = m.venue || "";
        if (!this.form.doi) this.form.doi = m.doi || "";
        if (!this.form.keywords) this.form.keywords = m.keywords || "";
        if (!this.form.summary) this.form.summary = m.summary || "";
        if (m.aiUsed) this.form.useAI = false;
        this.notify(m.aiUsed ? "已自动提取（AI），保存时不再重复请求" : "已提取 PDF 基础元数据");
      } catch (e) { this.notify(e.message, true); }
      this.pdfExtracting = false;
    },

    submitPaperForm: async function () {
      if (!this.form.title.trim()) { this.notify("标题必填", true); return; }
      var fd = new FormData();
      fd.append("title", this.form.title); fd.append("authors", this.form.authors);
      fd.append("year", this.form.year); fd.append("venue", this.form.venue);
      fd.append("doi", this.form.doi); fd.append("keywords", this.form.keywords);
      fd.append("link", this.form.link); fd.append("summary", this.form.summary);
      fd.append("notes", this.form.notes); fd.append("categoryId", this.form.categoryId);
      fd.append("read", this.form.read ? "1" : "0"); fd.append("starred", this.form.starred ? "1" : "0");
      fd.append("useAI", this.form.useAI ? "1" : "0");
      fd.append("tags", this.form.tags); fd.append("collections", this.form.collections);
      var fileEl = this.$refs.pdfInput;
      if (fileEl && fileEl.files && fileEl.files[0]) fd.append("pdf", fileEl.files[0], fileEl.files[0].name);
      var url = this.editingId ? ("/api/papers/" + this.editingId) : "/api/papers";
      var method = this.editingId ? "PUT" : "POST";
      this.saving = true;
      try {
        await this.api(url, { method: method, body: fd });
        this.notify("保存成功"); this.showPaperModal = false;
        await this.loadAll(); await this.loadPapers();
      } catch (e) {
        if (e.status === 409 && e.data && e.data.duplicate) {
          if (confirm("检测到重复论文：" + e.data.title + "。仍然添加吗？")) {
            fd.append("force", "1");
            try {
              await this.api(url, { method: method, body: fd });
              this.notify("已添加（确认保留重复项）"); this.showPaperModal = false;
              await this.loadAll(); await this.loadPapers();
            } catch (e2) { this.notify(e2.message, true); }
          }
        } else { this.notify(e.message, true); }
      }
      this.saving = false;
    },
    toggleRead: async function (id) {
      try {
        var p = await this.api("/api/papers/" + id + "/toggle-read", { method: "POST" });
        if (this.detail && this.detail.id === id) this.detail = p;
        await this.loadPapers();
      } catch (e) { this.notify(e.message, true); }
    },
    toggleStar: async function (id) {
      try {
        var p = await this.api("/api/papers/" + id + "/toggle-star", { method: "POST" });
        if (this.detail && this.detail.id === id) this.detail = p;
        await this.loadPapers();
      } catch (e) { this.notify(e.message, true); }
    },
    deletePaper: async function (id) {
      if (!confirm("确定删除这篇论文吗？此操作不可恢复。")) return;
      try {
        await this.api("/api/papers/" + id, { method: "DELETE" });
        this.notify("已删除"); this.showDetail = false;
        await this.loadAll(); await this.loadPapers();
      } catch (e) { this.notify(e.message, true); }
    },
    openDetail: async function (id) {
      this.showDetail = true; this.detail = {};
      try {
        this.detail = await this.api("/api/papers/" + id);
        this.tagSelect = ""; this.collectionSelect = "";
      } catch (e) { this.notify(e.message, true); }
    },
    closeDetail: function () { this.showDetail = false; this.detail = {}; },
    summarize: async function (id) {
      if (!this.hasApiKey) { this.notify("未配置 API Key，无法生成总结", true); return; }
      this.summarizing = true;
      try {
        var p = await this.api("/api/papers/" + id + "/summarize", { method: "POST" });
        this.detail = p;
        this.notify("AI 总结已生成");
        await this.loadPapers();
      } catch (e) { this.notify(e.message, true); }
      this.summarizing = false;
    },
    aiExtract: async function (id) {
      if (!this.hasApiKey) { this.notify("未配置 API Key，已跳过 AI 提取", true); return; }
      this.aiExtracting = true;
      try {
        var res = await this.api("/api/papers/" + id + "/ai-extract", { method: "POST" });
        if (res && res.skipped) { this.notify("未配置 API Key，已跳过"); }
        else { this.detail = res; this.notify("AI 提取完成"); await this.loadPapers(); }
      } catch (e) { this.notify(e.message, true); }
      this.aiExtracting = false;
    },
    addDetailTag: async function () {
      if (!this.tagSelect || !this.detail.id) return;
      try {
        this.detail = await this.api("/api/papers/" + this.detail.id + "/tags", { method: "POST", json: { tagId: Number(this.tagSelect) } });
        this.tagSelect = "";
      } catch (e) { this.notify(e.message, true); }
    },
    removeDetailTag: async function (tagId) {
      if (!this.detail.id) return;
      try { this.detail = await this.api("/api/papers/" + this.detail.id + "/tags/" + tagId, { method: "DELETE" }); }
      catch (e) { this.notify(e.message, true); }
    },
    addDetailCollection: async function () {
      if (!this.collectionSelect || !this.detail.id) return;
      try {
        this.detail = await this.api("/api/papers/" + this.detail.id + "/collections", { method: "POST", json: { collectionId: Number(this.collectionSelect) } });
        this.collectionSelect = "";
      } catch (e) { this.notify(e.message, true); }
    },
    removeDetailCollection: async function (colId) {
      if (!this.detail.id) return;
      try { this.detail = await this.api("/api/papers/" + this.detail.id + "/collections/" + colId, { method: "DELETE" }); }
      catch (e) { this.notify(e.message, true); }
    },
    saveNotes: async function () {
      if (!this.detail || !this.detail.id) return;
      var p = this.detail;
      var body = {
        title: p.title, authors: p.authors, year: p.year || 0, venue: p.venue, doi: p.doi,
        keywords: p.keywords, link: p.link, summary: p.summary, notes: p.notes,
        categoryId: p.categoryId, read: p.read, starred: p.starred,
        tags: (p.tags || []).map(function (t) { return t.id; }),
        collections: (p.collections || []).map(function (c) { return c.id; })
      };
      try {
        this.detail = await this.api("/api/papers/" + p.id, { method: "PUT", json: body });
        this.notify("笔记已保存"); await this.loadPapers();
      } catch (e) { this.notify(e.message, true); }
    },
    openManage: function () { this.showManage = true; },
    addCategory: async function () {
      if (!this.newCategory.trim()) return;
      try { await this.api("/api/categories", { method: "POST", json: { name: this.newCategory } }); this.newCategory = ""; this.notify("分类已添加"); await this.loadAll(); }
      catch (e) { this.notify(e.message, true); }
    },
    deleteCategory: async function (id) {
      if (!confirm("删除该分类？论文将变为未分类。")) return;
      try { await this.api("/api/categories/" + id, { method: "DELETE" }); this.notify("已删除"); await this.loadAll(); await this.loadPapers(); }
      catch (e) { this.notify(e.message, true); }
    },
    addTag: async function () {
      if (!this.newTag.trim()) return;
      try { await this.api("/api/tags", { method: "POST", json: { name: this.newTag } }); this.newTag = ""; this.notify("标签已添加"); await this.loadAll(); }
      catch (e) { this.notify(e.message, true); }
    },
    deleteTag: async function (id) {
      if (!confirm("删除该标签？")) return;
      try { await this.api("/api/tags/" + id, { method: "DELETE" }); this.notify("已删除"); await this.loadAll(); await this.loadPapers(); }
      catch (e) { this.notify(e.message, true); }
    },
    addCollection: async function () {
      if (!this.newCollection.trim()) return;
      try { await this.api("/api/collections", { method: "POST", json: { name: this.newCollection } }); this.newCollection = ""; this.notify("合集已添加"); await this.loadAll(); }
      catch (e) { this.notify(e.message, true); }
    },
    deleteCollection: async function (id) {
      if (!confirm("删除该合集？")) return;
      try { await this.api("/api/collections/" + id, { method: "DELETE" }); this.notify("已删除"); await this.loadAll(); await this.loadPapers(); }
      catch (e) { this.notify(e.message, true); }
    },
    openSettings: async function () {
      try { await this.loadAll(); } catch (e) {}
      this.showSettings = true;
    },
    saveSettings: async function () {
      var body = { aiBaseUrl: this.settings.aiBaseUrl, aiModel: this.settings.aiModel };
      if (this.settings.aiApiKey) body.aiApiKey = this.settings.aiApiKey;
      try {
        var res = await this.api("/api/settings", { method: "PUT", json: body });
        this.hasApiKey = !!res.hasApiKey; this.settings.aiApiKey = "";
        this.notify("AI 设置已保存"); this.showSettings = false;
      } catch (e) { this.notify(e.message, true); }
    },
    clearApiKey: async function () {
      try {
        var res = await this.api("/api/settings", { method: "PUT", json: { clearApiKey: true, aiBaseUrl: this.settings.aiBaseUrl, aiModel: this.settings.aiModel } });
        this.hasApiKey = !!res.hasApiKey; this.settings.aiApiKey = "";
        this.notify("API Key 已清除");
      } catch (e) { this.notify(e.message, true); }
    }
  },
  mounted: function () {
    var self = this;
    this.loadAll().then(function () { self.loadPapers(); });
  }
};

Root.template = document.getElementById("app-template").innerHTML;
var app = Vue.createApp(Root);
app.mount("#app");
