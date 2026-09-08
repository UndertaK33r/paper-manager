# Paper Manager 论文管理系统

基于 Go + SQLite 的个人论文管理工具。上传 PDF 后自动提取元数据，支持分类、标签、搜索、笔记和 AI 辅助阅读。前端 Vue 3（已内置依赖，无需构建），支持 Docker 部署。

## 功能

- PDF 上传后自动提取标题 / 作者 / 年份 / 期刊 / DOI / 关键词，支持中文 PDF；可选 AI 补充提取
- 在线补全：DOI、Crossref、OpenAlex、arXiv
- 浏览器内阅读 PDF；选中英文段落可翻译（需配置 AI）
- 笔记：全屏阅读时可边读边记、自动保存，可导出为 Markdown
- 分类、标签、合集、阅读状态、收藏；搜索（含 PDF 全文）、筛选、排序、分页
- RAG 问论文库、AI 总结、BibTeX 导出、引用网络图谱
- 三套主题（日间 / 黑夜 / 护眼），移动端可用

## 快速开始（Docker）

```bash
docker compose up -d --build
# 访问 http://<机器IP>:8080
```

数据保存在 `./data` 目录，备份直接拷贝。

## 分享给别人

两种方式，按场景选：

1. **免安装分发**（各自使用各自的数据）：推荐 Mac/Linux 用户复制一行命令安装（自动下载并绕过 macOS Gatekeeper 拦截）：
   ```bash
   curl -fsSL https://github.com/UndertaK33r/paper-manager/releases/latest/download/install.sh | bash
   ```
   或者到 [Releases](https://github.com/UndertaK33r/paper-manager/releases) 下载对应系统的压缩包，解压后双击运行。Mac 会提示「已损坏 / 无法验证开发者」：打开 系统设置 → 隐私与安全性 → 底部点「仍要打开」，或终端执行 `xattr -cr <程序文件>`（详见压缩包内使用说明）。
   - 本地构建：`./build.sh`，产物在 `dist/`
   - 打 tag 推送（如 `git tag v1.0.0 && git push --tags`）会自动构建并发布 Release
2. **部署一份集中使用**（共享同一批论文，推荐实验室场景）：在一台常开的机器上 `docker compose up -d`，把 `http://<机器IP>:8080` 发给同学即可；跨网络可用 Tailscale 组网。

## 本地开发

```bash
go run ./cmd/server
# 访问 http://localhost:8080
```

需要 Go 1.24+。国内网络建议 `export GOPROXY=https://goproxy.cn,direct`。

## AI 设置

顶栏「AI 设置」里填 API Key 即可。默认接口地址为 `https://tokendance.space/gateway/v1`，可改成任意 OpenAI 兼容接口（如 `https://api.deepseek.com/v1`），模型可从接口拉取或手动输入。未配置 Key 时 AI 功能自动跳过。

也可用环境变量：`AI_API_KEY`、`AI_BASE_URL`、`AI_MODEL`。

## 环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| PORT | 8080 | 监听端口 |
| DATA_DIR | ./data | 数据目录 |
| UPLOAD_DIR | <DATA_DIR>/uploads | PDF 目录 |
| WEB_DIR | web | 前端静态目录 |
| MAX_UPLOAD_MB | 100 | 上传上限 |
| AUTH_USERNAME / AUTH_PASSWORD | 空 | 可选 Basic Auth |
| AI_API_KEY / AI_BASE_URL / AI_MODEL | 空 | AI 配置（也可在设置页保存） |

## API 概览

```
GET    /api/papers                        列表
POST   /api/papers                        添加论文（multipart，可带 PDF）
POST   /api/papers/extract-pdf            PDF 元数据预览提取
GET/PUT/PATCH/DELETE /api/papers/{id}     详情 / 更新 / 部分更新 / 删除
GET    /api/papers/{id}/pdf               获取 PDF（?download=1 下载）
POST   /api/papers/{id}/re-extract        重新提取全文
POST   /api/papers/{id}/re-detect         重新识别元数据
POST   /api/papers/{id}/ai-extract        手动 AI 提取
POST   /api/papers/{id}/summarize         AI 总结
POST   /api/papers/{id}/translate         全文翻译并存储
POST   /api/ai/translate-text             翻译一段文本
GET/POST/DELETE /api/categories[/{id}]    分类
GET/POST/DELETE /api/tags[/{id}]          标签
GET/POST/DELETE /api/collections[/{id}]   合集
GET    /api/stats                         统计
GET/PUT /api/settings                     AI 设置
GET    /api/ai/models                     模型列表
POST   /api/ai/test                       测试 AI 连接
GET    /api/papers/export.bib             导出 BibTeX
GET    /api/papers/export/notes           导出全部笔记（Markdown）
POST   /api/ask                           问论文库
GET    /api/graph                         引用网络
```

## 目录结构

```
paper-manager/
├── cmd/server/        # 入口
├── internal/
│   ├── ai/            # AI 客户端
│   ├── api/           # HTTP 处理器
│   ├── meta/          # 在线元数据补全
│   ├── models/        # 数据模型
│   ├── pdf/           # PDF 文本与元数据提取
│   └── store/         # SQLite 存储
├── web/               # 前端（Vue 3 + pdf.js，静态文件）
├── Dockerfile
└── docker-compose.yml
```

## 测试

```bash
go test ./internal/...
```
