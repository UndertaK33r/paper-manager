# Paper Manager 论文管理系统

一个基于 Go + SQLite 的个人论文管理系统。支持 PDF 上传后自动提取元数据（基础元数据 + 可选 AI 补充提取）、分类 / 标签 / 合集、搜索筛选排序、已读 / 收藏标记、浏览器内 PDF 预览，通过 Docker 一键部署。前端采用 **Vue 3** 重构，并使用 **P3R（Persona 3 Reload）冷蓝黑游戏 UI 美学**。

## 技术栈

- 后端：Go 1.24（标准库 net/http）
- 数据库：SQLite（pure-Go 驱动 modernc.org/sqlite，无 CGO）
- PDF 元数据：github.com/ledongthuc/pdf
- AI 提取：OpenAI 兼容 Chat Completions API（可配置 Base URL / Model / API Key）
- 前端：Vue 3（已内置 vendor，无需构建）+ P3R 设计系统（p3r-tokens.css / p3r-ui.css）
- 部署：Docker / docker-compose

## 已实现功能（MVP）

- 论文 CRUD：添加（PDF 上传 + 元数据自动提取）、查看、编辑、删除
- **选择 PDF 后表单自动填充**（无需先保存）：
  - 基础提取：标题（原文件名兜底）、作者、关键词（来自 PDF Info）
  - AI 补充提取（可选）：作者、年份、期刊/会议、DOI、关键词、一句话总结、建议分类
  - 已自动提取成功时，保存阶段不再重复发送 AI 请求
  - 未配置 API Key 时自动跳过 AI 请求，不会报错
- 去重检测：DOI 精确；无 DOI 时按「归一化标题 + 第一作者」；重复时提示确认
- PDF 管理：上传、存储、浏览器内预览、下载
- 分类（唯一分类）、标签（多对多）、合集（多对多）
- 已读 / 未读、收藏标记
- 搜索：标题 / 作者 / 关键词 / 期刊 / DOI 模糊
- 筛选：分类、标签、合集、年份、状态、收藏
- 排序：标题、年份、作者、添加/更新时间，升降序
- 表格 / 卡片视图、分页、响应式
- Basic Auth（可选）
- AI 设置页面：保存 OpenAI 兼容 API Key / Base URL / Model
- 手动「AI 提取」按钮：详情页对已有 PDF 重新提取

## 快速开始（Docker）

```bash
docker compose up -d --build
# 访问 http://<实验室电脑IP>:8080
```

数据保存于宿主机 `./data`，备份直接拷贝该目录。

## 本地开发

```bash
go run ./cmd/server
# 访问 http://localhost:8080
```

## AI 自动提取说明

1. 点击右上角「AI 设置」，填入 OpenAI 兼容 API Key（可选 Base URL、Model），保存。
2. 添加 / 编辑论文时勾选「使用 AI 自动提取剩余元数据」并上传 PDF。
3. 后端仅在 **已配置 API Key** 时发送 AI 请求；AI 失败不会阻断论文保存。
4. 详情页提供「AI 提取」按钮，可随时对已有 PDF 重新提取并补全空字段。

环境变量亦可直接注入：`AI_API_KEY`、`AI_BASE_URL`、`AI_MODEL`。

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
GET    /api/papers                        列表（search/category/tags/collections/yearFrom/yearTo/read/starred/sort/order/page/pageSize）
POST   /api/papers                        添加论文（multipart；useAI=1 时自动 AI 提取）
POST   /api/papers/extract-pdf            PDF 预览提取（返回元数据，不创建论文）
GET    /api/papers/{id}                   详情
PUT    /api/papers/{id}                   更新（multipart 或 JSON）
DELETE /api/papers/{id}                   删除
GET    /api/papers/{id}/pdf               获取 PDF（?download=1 下载）
POST   /api/papers/{id}/toggle-read       切换已读
POST   /api/papers/{id}/toggle-star       切换收藏
POST   /api/papers/{id}/ai-extract        手动 AI 提取（无 Key 时返回 skipped）
POST   /api/papers/{id}/summarize         AI 总结（预留）

GET/POST/DELETE /api/categories[/{id}]
GET/POST/DELETE /api/tags[/{id}]
GET/POST/DELETE /api/collections[/{id}]
GET    /api/stats
GET/PUT /api/settings                      AI 设置（aiBaseUrl/aiModel/aiApiKey/clearApiKey）
```

## 项目结构

```
paper-manager/
├── cmd/server/
├── internal/
│   ├── ai/                # OpenAI 兼容 AI 元数据提取
│   ├── api/               # HTTP 接口
│   ├── models/
│   ├── pdf/               # PDF 元数据 + 文本提取
│   └── store/             # SQLite（含 settings 表）
├── web/
│   ├── assets/            # Vue 3 vendor + P3R CSS
│   ├── index.html         # Vue 模板
│   ├── app.js             # Vue 应用
│   └── style.css
├── Dockerfile
└── docker-compose.yml
```
