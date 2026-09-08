# Paper Manager 论文管理系统

一个基于 Go + SQLite 的个人论文管理系统。支持 PDF 上传后自动提取元数据（字体感知的本地提取 + 在线补全 + 可选 AI 补充）、分类 / 标签 / 合集、搜索筛选排序、已读 / 收藏标记、浏览器内划词翻译阅读、AI 中文辅助阅读、笔记自动保存与导出，通过 Docker 一键部署。前端为 Vue 3（已内置 vendor，无需构建），内置三套可切换主题。

## 技术栈

- 后端：Go 1.24（标准库 net/http，无框架）
- 数据库：SQLite（pure-Go 驱动 modernc.org/sqlite，无 CGO）
- PDF：内置 pdf.js 阅读器（划词翻译）+ 自研字体感知文本提取
- AI：任意 OpenAI 兼容网关（默认词元跳动 tokendance，兼容 DeepSeek），元数据提取 / 总结 / RAG 问答 / 翻译
- 前端：Vue 3 + pdf.js + 自研迷你 Markdown 渲染器（均已 vendor，无需构建）
- 部署：Docker / docker-compose

## 功能总览

### 元数据自动提取
- **字体感知文本提取**：解析 ToUnicode CMap，正确处理 CID/Identity-H、子集字体与 /Differences 编码，中文 PDF 不再乱码；解析失败自动回退流扫描器
- **标题识别**：按页字号聚类（第一页最大字号优先），支持竖排中文逐字行合并、跨行标题拼接（作者行自动截断）；宁缺毋错——识别不可靠时返回空而不瞎填
- **在线补全**：DOI 嗅探 → OpenAlex → Crossref，arXiv 标题短语检索优先；Jaccard 对称相似度校验，DOI 对不上本地标题（多来自参考文献）即弃用
- **AI 补充提取**（可选）：作者 / 年份 / 期刊 / DOI / 关键词 / 一句话总结 / 建议分类；JSON 类型抖动宽容解析、截断自动修复
- **动态模型列表**：AI 设置里可编辑网关地址，一键拉取 `/models` 模型列表，也支持手输模型名
- 选择 PDF 后表单自动填充，保存时自动提取全文供搜索与 RAG

### 阅读
- **pdf.js 阅读器**：文字可选中；选中一段浮出「翻译」按钮，译文气泡就地显示；右侧划词翻译收集窗按序归档本次会话译文；渲染失败自动回退浏览器内置阅读器
- **全屏 / 全宽阅读**：底部悬浮笔记窗（停止输入 1.2s 自动保存，可收起）、右侧悬浮译文窗
- **即时翻译**：粘贴英文难句秒翻

### 笔记
- 全屏边读边记自动保存，详情页与悬浮窗共用同一份数据
- 笔记上方标注「最后修改」日期
- 导出：单篇 Markdown（含标题 / 作者 / 修改日期）；管理弹窗一键导出全部笔记（按修改时间倒序）

### 库管理
- 论文 CRUD、去重检测（DOI 精确 + 标题/作者归一化）
- 分类（唯一）、标签（多对多）、合集（多对多）；详情页输入即创建
- 阅读状态三档（待读 / 在读 / 已读）、收藏
- 搜索覆盖标题 / 作者 / 关键词 / 期刊 / DOI / **PDF 全文**，筛选 + 排序 + 分页，表格 / 卡片双视图
- BibTeX 导出（按当前筛选）；OpenAlex 引用网络知识图谱（Canvas，可拖拽）

### 界面
- **三套组件级主题**：日间（黑白极简，切角+五线谱音符）、黑夜（深蓝玻璃拟态+胶囊按钮+科技网格辉光）、护眼（揉皱宣纸质感+朱丝栏+篆印+衬线标题），右下角悬浮球切换
- 右下角悬浮球：「问」展开 RAG 问答浮窗（Markdown 回答 + 引用来源）、「◐」切换主题
- 移动端适配（断点重排 / 触屏目标 / iOS 防缩放），跟随系统深浅色

## 快速开始（Docker）

```bash
docker compose up -d --build
# 局域网访问 http://<机器IP>:8080
```

数据保存在宿主机 `./data`，备份直接拷贝该目录。

## 本地开发

```bash
go run ./cmd/server
# 访问 http://localhost:8080
```

需要 Go 1.24+。国内网络建议 `export GOPROXY=https://goproxy.cn,direct`。

## AI 配置

1. 右下角「◐」无关于 AI；点顶栏「AI 设置」：
   - **接口地址**：默认 `https://tokendance.space/gateway/v1`，可改为任意 OpenAI 兼容网关（如 `https://api.deepseek.com/v1`）
   - **模型**：点「刷新模型列表」从网关拉取，或直接输入
   - **API Key**：保存后即可使用 AI 提取 / 总结 / 问答 / 翻译
2. 未配置 Key 时 AI 功能自动跳过，不报错
3. 环境变量亦可注入：`AI_API_KEY`、`AI_BASE_URL`、`AI_MODEL`

## 环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| PORT | 8080 | 监听端口 |
| DATA_DIR | ./data | 数据目录 |
| UPLOAD_DIR | <DATA_DIR>/uploads | PDF 目录 |
| WEB_DIR | web | 前端静态目录 |
| MAX_UPLOAD_MB | 100 | 上传上限 |
| AUTH_USERNAME / AUTH_PASSWORD | 空 | 可选 Basic Auth |
| AI_API_KEY / AI_BASE_URL / AI_MODEL | 空 | 可选 AI 配置（也可在设置页保存） |

## API 概览

```
GET    /api/papers                        列表（search/category/tags/collections/yearFrom/yearTo/status/starred/sort/order/page/pageSize）
POST   /api/papers                        添加论文（multipart；useAI=1 时自动 AI 提取）
POST   /api/papers/extract-pdf            PDF 预览提取（返回元数据，不创建论文）
GET    /api/papers/{id}                   详情
PUT    /api/papers/{id}                   更新（multipart 或 JSON）
PATCH  /api/papers/{id}                   部分更新（未提供字段不覆盖）
DELETE /api/papers/{id}                   删除
GET    /api/papers/{id}/pdf               获取 PDF（?download=1 下载）
POST   /api/papers/{id}/status            设置状态 {status: unread/reading/read}
POST   /api/papers/{id}/re-extract        重新提取全文
POST   /api/papers/{id}/re-detect         重新识别元数据
POST   /api/papers/{id}/toggle-read       切换阅读状态
POST   /api/papers/{id}/toggle-star       切换收藏
POST   /api/papers/{id}/ai-extract        手动 AI 提取
POST   /api/papers/{id}/summarize         AI 总结
GET    /api/papers/{id}/translation       读取全文译文
POST   /api/papers/{id}/translate         分块翻译全文并存储
POST   /api/ai/translate-text             即时翻译一段文本
GET/POST/DELETE /api/categories[/{id}]    分类
GET/POST/DELETE /api/tags[/{id}]          标签
GET/POST/DELETE /api/collections[/{id}]   合集
GET    /api/stats                         统计
GET    /api/health                        健康检查
GET/PUT /api/settings                     AI 设置（baseUrl/model/apiKey）
GET    /api/ai/models                     拉取网关模型列表（?base= 可选）
POST   /api/ai/test                       测试 AI 连接
GET    /api/papers/export.bib             按筛选导出 BibTeX
GET    /api/papers/export/notes           导出全部笔记（Markdown 附件）
POST   /api/ask                           RAG 问论文库
GET    /api/graph                         OpenAlex 引用网络
```

## 项目结构

```
paper-manager/
├── cmd/server/            # 入口
├── internal/
│   ├── ai/                # OpenAI 兼容客户端（提取/总结/翻译，宽容 JSON 解析）
│   ├── meta/              # Crossref / OpenAlex / arXiv 在线补全（含相似度校验）
│   ├── api/               # HTTP 路由与处理器
│   ├── models/            # 数据模型
│   ├── pdf/               # 字体感知提取（cmap/pages/pdfx）+ 元数据嗅探
│   └── store/             # SQLite（papers/relations/settings）
├── web/
│   ├── assets/
│   │   ├── pdfjs/         # 内置 pdf.js（划词阅读器）
│   │   ├── md-mini.js     # 零依赖 Markdown 渲染器（防 XSS）
│   │   ├── themes.css     # 三套主题令牌（日间/黑夜/护眼）
│   │   └── vue.global.prod.js
│   ├── index.html         # Vue 模板
│   ├── app.js             # Vue 应用
│   └── style.css          # 全局样式（含响应式）
├── Dockerfile
└── docker-compose.yml
```

## 测试

```bash
go test ./internal/...
```

覆盖：ToUnicode CMap 解析、竖排标题合并、截断 JSON 修复、AI 字段类型抖动等。
