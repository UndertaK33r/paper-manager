# paper-manager 后续规划

> 本文件记录代码清理（删除不可达的 pdf.js 阅读器、划词翻译、全文翻译后端等）之后的
> 优化方向与新功能思路。工作量标注：**S** ≈ 半天内，**M** ≈ 1–3 天，**L** ≈ 1 周以上。
> 每条都写明现状依据，避免"看起来该做"的空泛建议。

## 一、当前能力盘点

| 模块 | 现状 |
|---|---|
| 论文管理 | CRUD、分类/标签/合集、星标与阅读状态（unread→reading→read 循环）、分页/筛选/排序 |
| PDF | 上传、本地元数据提取、AI 补全、全文提取、原生 iframe 阅读、全宽/全屏、笔记与译文侧栏 |
| 笔记 | 自动保存（停止输入 1.2s）、乐观锁（`notes_updated_at` + `ErrConflict`）、导出全部笔记 |
| AI | 摘要、元数据提取、模型列表（从网关 `/models` 拉取）、粘贴文本翻译、RAG 问答 |
| 检索 | `SearchRelaxed`：title/authors/keywords/summary/fulltext 的 `LIKE` 模糊匹配 |
| 图谱 | 基于参考文献关系构图，带缓存（最多 500 篇） |
| 其他 | BibTeX 导出、三套主题（日间/黑夜/护眼）、移动端自适应、Basic 认证、四平台免安装包 |

## 二、本轮删除的代码（如需恢复，见提交历史）

| 删除项 | 原因 |
|---|---|
| `assets/pdfjs/`（313K + 1.0M worker）+ 阅读器代码 | 容器 `v-if="pdfJsOk"` 与 `pdfJsOk` 置真互为前置条件，路径不可达；实际一直用原生 iframe |
| 划词翻译（`selPopup`/`selResult`/`onPdfMouseUp`） | 依赖 pdf.js 文本层，随之上不可达 |
| 全文翻译后端（`/translate`、`/translation`、`SplitChunks`、译文列） | 前端无任何入口，从未被调用 |
| 冗余端点 `/file`、`/status`、`/toggle-star`、`/summary` | 与 `/pdf`、PATCH、`/summarize` 重复 |
| 死函数 | `HealthCheck`、`AllSettings`、`SetPaperCategory`、`SetPaperRead`、`UpdatePaper`、`PaperPDFPath`、`applyRelations`、`ensureTags/ensureCollections`、`tagIDs/collectionIDs`、`decodeJSONOnly`、`pdf.Extract/IsValidPDF/HasMeta`、`ai.Defaults` |
| 遗留主题 | `p3r-tokens.css`、`p3r-ui.css`、`p3r-tokens.json`（已解除引用，其 `.p3r-btn:hover` 会把按钮文字染成浅蓝） |

> 若想恢复"全文翻译"，**不要**恢复 `/translate` 的旧接口形态，按第四节第 1 条做成对照阅读更合适。

## 三、工程与性能（低风险，建议先做）

1. **列表页 N+1 查询**（S）— `ListPapers` 对每行调用 `loadRelations`，一次列表 = 1 + 2N 次查询
   （20 条即 41 次）。改为一次性 `WHERE paper_id IN (...)` 聚合，可显著降低首屏延迟。
2. **全文搜索上 FTS5**（M）— 现在 `LIKE '%关键词%'` 无法用索引，论文多了会全表扫描。
   SQLite 自带 FTS5，建 `papers_fts(title, authors, keywords, summary, fulltext)` 虚表 + 触发器同步即可。
3. **AI Key 加密存储**（S）— `settings` 表里 `ai_api_key` 是明文；用系统密钥链或
   `DATA_DIR/key`（0600）做 AES-GCM 加密，至少避免备份包直接泄露 Key。
4. **CI + 冒烟测试**（M）— GitHub Actions 跑 `go test ./... && go vet ./...`，
   再起隔离实例跑一组 Playwright 冒烟（上传 → 详情 → 笔记 → 删除），最后构建四平台产物。
   本轮验证用的脚本可直接改造复用。
5. **健康检查细化**（S）— `/api/health` 目前只有 `{"ok":true}`，可返回 DB 大小、WAL 帧数、
   上传目录可写、AI 配置状态，便于排障。
6. **一键备份/恢复**（S）— 目前只能手动拷贝 `data/`。做一个"导出 zip（含 db + uploads）"
   与"从 zip 恢复"，比文档里"先退出程序再复制"可靠得多。
7. **前端拆分**（M）— `app.js` 已 440 行且仍在增长，模板在 `index.html` 里。
   两个方向：保持零构建、按功能拆成多个 `<script>`；或引入 Vite + SFC（代价是发布流程多一步构建）。
8. **表格列宽抖动**（S）— 移除遗留主题后统计行/列宽会重排一次，可用 `table-layout: fixed` +
   列宽定义让它稳定。

## 四、阅读与笔记

1. **全文对照翻译（重做版）**（M）— 用已提取的 `fulltext` 分块翻译，左原文右译文、滚动同步，
   译文按段落存表（`paper_id, seq, src, dst`）。比旧的"划词翻译"更贴合原生阅读器的能力边界：
   原生 iframe 拿不到文本层，但后台有全文。
2. **笔记 Markdown 预览 + 全文搜索**（S/M）— 详情页已有 `md-mini.js`，笔记区加"预览/编辑"切换；
   搜索走同一套 FTS5。
3. **划线高亮与批注**（M）— 存 `paper_id + 段落序号 + 字符区间`，在笔记面板里做双向跳转。
   不要试图在原生 PDF 上做选区（拿不到文本层）。
4. **阅读进度**（S）— 记录"上次读到第几页/滚动位置"，列表页显示进度条；已有阅读状态可复用。

## 五、检索与知识组织

1. **向量检索 + 引用溯源**（L）— 现在 RAG 是 `LIKE` 取前 3 篇喂给模型，召回质量有限且答案无出处。
   本地小模型 embedding（如 `bge-small-zh`）+ SQLite 存向量，检索结果带"来自《论文 X》第 N 段"。
2. **批量导入**（M）— 支持 BibTeX/RIS 文件、DOI 列表批量入库；`internal/meta` 已有
   `EnrichByDOI`/`SearchByTitle` 可直接复用。
3. **重复论文合并**（M）— `FindDuplicate` 已能识别（DOI/标题），但命中后只能"仍然添加"。
   补一个"合并到已有条目"：迁移标签/笔记/PDF，删除重复行。
4. **AI 自动打标**（M）— 上传后按摘要建议标签/合集，用户确认即可，复用现有 AI 配置。

## 六、AI 能力

1. **多篇对比表**（M）— 勾选 2–5 篇，生成"方法/数据集/指标/结论"横向对比，输出 Markdown 表格。
2. **结构化抽取**（M）— 从全文抽方法、数据集、指标、局限，存成结构化字段，可筛选可排序。
3. **综述草稿**（L）— 基于选中的论文 + 自己的笔记生成初稿，引用必须带论文 ID。
4. **图谱增强**（M）— 现在只有引用关系；可叠加"共同作者/共同关键词/同分类"边，并支持点击节点跳详情。

## 七、体验细节（快速见效）

- **批量操作**（S）：列表多选 → 批量打标签/改状态/删除
- **快捷键**（S）：`j`/`k` 列表上下、`/` 聚焦搜索、`n` 新建笔记、`Esc` 关闭浮层（已有）
- **空状态引导**（S）：论文库为空时给"上传 PDF / 导入 BibTeX"两个入口
- **主题跟随系统**（S）：现在是三选一手动切换，可加"跟随系统"选项（`prefers-color-scheme`）
- **移动端**（S）：PDF 面板支持手势缩放提示；列表在 390px 下已可用

## 八、不建议做的事

- **不要重新引入 pdf.js 阅读器**：用户明确要原生阅读器，且当前 iframe 已满足阅读需求；
  划词翻译要另找文本层来源（见四.1）。
- **不要在检索层没升级前调 RAG 参数**：召回只有 3 篇 LIKE 结果，先解决召回再谈提示词。
- **不要为"可能的将来"保留无用代码**：本轮的教训是死代码会持续产生样式/行为泄漏。
