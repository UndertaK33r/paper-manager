function makeEmptyForm() {
  return { title: "", authors: "", year: "", venue: "", doi: "", keywords: "", link: "", summary: "", notes: "", categoryId: "", tags: "", collections: "", read: false, starred: false, useAI: true };
}

var Root = {
  data: function () {
    return {
      route: "list", token: localStorage.getItem("pm-token") || "", loginPass: "", loginError: "",
      stats: {}, categories: [], tags: [], collections: [],
      papers: [], total: 0, page: 1, pages: 1, pageSize: 20, loading: true,
      view: "table", search: "", statusFilter: "", categoryFilter: "", tagFilter: "", sort: "created", order: "desc",
      askOpen: false, question: "", answer: "", sources: [], asking: false,
      detail: {}, tagSelect: "", collectionSelect: "", fullWidth: false,
      showPaperModal: false, editingId: null, saving: false, form: makeEmptyForm(),
      aiExtracting: false, summarizing: false, pdfExtracting: false,
      showManage: false, newCategory: "", newTag: "", newCollection: "",
      showSettings: false, settings: { aiBaseUrl: "https://api.deepseek.com/v1", aiModel: "deepseek-v4-flash", aiApiKey: "" }, hasApiKey: false, testingAI: false, aiTestResult: "",
      uploading: false, uploadProgress: 0, uploadResult: null,
      graphNodes: [], graphEdges: [], graphWarning: "", graphTimer: null, dragNode: null,
      toast: { show: false, msg: "", error: false }
    };
  },
  computed: {
    readPct: function () { if (!this.stats.total) return 0; return Math.round(this.stats.read / this.stats.total * 100); },
    statusOptions: function () { return [{v:"",t:"全部"},{v:"unread",t:"待读"},{v:"reading",t:"在读"},{v:"read",t:"已读"}]; },
    availableTags: function () { var self=this; return this.tags.filter(function(t){ return !(self.detail.tags||[]).some(function(x){return x.id===t.id;}); }); },
    availableCollections: function () { var self=this; return this.collections.filter(function(c){ return !(self.detail.collections||[]).some(function(x){return x.id===c.id;}); }); }
  },
  methods: {
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
      this.detail={}; this.route="detail"; window.location.hash="#/papers/"+id;
      try { this.detail=await this.api("/api/papers/"+id); this.tagSelect=""; this.collectionSelect=""; } catch (e) { this.notify(e.message,true); }
    },
    saveDetail: async function () {
      if(!this.detail||!this.detail.id) return;
      var p=this.detail;
      var body={ title:p.title, authors:p.authors, year:p.year||0, venue:p.venue, doi:p.doi, keywords:p.keywords, link:p.link, summary:p.summary, notes:p.notes, categoryId:p.categoryId, status:p.status, read:p.status==="read", starred:p.starred, tags:(p.tags||[]).map(function(t){return t.id;}), collections:(p.collections||[]).map(function(c){return c.id;}) };
      try { this.detail=await this.api("/api/papers/"+p.id,{method:"PUT",json:body}); this.notify("已保存"); this.loadAll(); this.loadPapers(); } catch (e) { this.notify(e.message,true); }
    },
    addDetailTag: async function () { if(!this.tagSelect||!this.detail.id) return; try { this.detail=await this.api("/api/papers/"+this.detail.id+"/tags",{method:"POST",json:{tagId:Number(this.tagSelect)}}); this.tagSelect=""; } catch(e){ this.notify(e.message,true); } },
    removeDetailTag: async function (id) { if(!this.detail.id) return; try { this.detail=await this.api("/api/papers/"+this.detail.id+"/tags/"+id,{method:"DELETE"}); } catch(e){ this.notify(e.message,true); } },
    addDetailCollection: async function () { if(!this.collectionSelect||!this.detail.id) return; try { this.detail=await this.api("/api/papers/"+this.detail.id+"/collections",{method:"POST",json:{collectionId:Number(this.collectionSelect)}}); this.collectionSelect=""; } catch(e){ this.notify(e.message,true); } },
    removeDetailCollection: async function (id) { if(!this.detail.id) return; try { this.detail=await this.api("/api/papers/"+this.detail.id+"/collections/"+id,{method:"DELETE"}); } catch(e){ this.notify(e.message,true); } },
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
    openSettings: function () { this.showSettings=true; this.aiTestResult=""; this.loadAll(); },
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
      var cv=this.$refs.graphCanvas; var rect=cv.getBoundingClientRect(); var mx=ev.clientX-rect.left; var my=ev.clientY-rect.top;
      this.dragNode=null; for(var i=0;i<this.graphNodes.length;i++){ var n=this.graphNodes[i]; var dx=n.x-mx; var dy=n.y-my; if(dx*dx+dy*dy<400){ this.dragNode=n; break; } }
    },
    onGraphMove: function (ev) { if(!this.dragNode) return; var cv=this.$refs.graphCanvas; var rect=cv.getBoundingClientRect(); this.dragNode.x=ev.clientX-rect.left; this.dragNode.y=ev.clientY-rect.top; },
    onGraphUp: function (ev) {
      var n=this.dragNode; this.dragNode=null;
      if(n && n.id && n.id.indexOf("lib-")===0) { var id=parseInt(n.id.slice(4),10); if(id) this.openDetail(id); }
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
    window.addEventListener("hashchange", function(){ self.parseHash(); });
    this.parseHash();
    this.loadAll().then(function(){ self.loadPapers(); });
  }
};

Root.template = document.getElementById("app-template").innerHTML;
var app = Vue.createApp(Root);
app.mount("#app");
