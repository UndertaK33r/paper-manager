# Paper Manager 论文管理系统

一个基于 Go + SQLite 的个人论文管理系统，支持 PDF 上传与元数据提取、分类 / 标签 / 合集、搜索筛选排序、已读 / 收藏标记、浏览器内 PDF 预览，并通过 Docker 一键部署。

## 技术栈

- 后端：Go 1.24（标准库 net/http，无外部 Web 框架）
- 数据库：SQLite（pure-Go 驱动 modernc.org/sqlite，无 CGO 依赖）
- PDF 元数据：github.com/ledongthuc/pdf
- 前端：原生 HTML / CSS / JavaScript（响应式，无构建步骤）
- 部署：Docker / docker-compose

## 已实现功能（MVP）

- 论文 CRUD：添加（PDF 上传 + 元数据自动提取）、查看、编辑、删除
- 去重检测：DOI 精确匹配；无 DOI 时按「归一化标题 + 第一作者」匹配；重复时提示用户确认
- PDF 管理：上传、存储、流式接口浏览 / 下载、浏览器内预览（nội建 iframe）
- 分类（唯一分类）、标签（多对多）、合集（多对多，手动管理）
- 已读 / 未读、收藏 / 稍后读标记
- 搜索：标题 / 作者 / 关键词 / 期刊 / DOI 模糊搜索
- 筛选：分类、标签、合集、年份范围、已读状态、收藏状态
- 排序：按标题、年份、作者、添加时间、更新时间，升 / 降序
- 表格 / 卡片视图切换、分页
- 响应式界面：手机 / 平板 / 桌面均可使用
- 可选 Basic Auth 简单密码保护
- AI 接口预留：/api/papers/:id/summarize（二期接入）

## 快速开始（Docker）

```bash
docker compose up -d --build
# 访问 http://<实验室电脑IP>:8080
```

数据（数据库 + PDF 文件）保存在宿主机 `./data` 目录，备份时直接拷贝该目录即可。

## 本地开发

```bash
go run ./cmd/server
# 访问 http://localhost:8080
```

## 环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| PORT | 8080 | 监听端口 |
| DATA_DIR | ./data | 数据目录（SQLite + 上传文件） |
| UPLOAD_DIR | <DATA_DIR>/uploads | PDF 存储目录 |
| WEB_DIR | web | 前端静态目录 |
| MAX_UPLOAD_MB | 100 | PDF 上传大小上限（MB） |
| AUTH_USERNAME | 空 | 可选，启用 Basic Auth 时设置 |
| AUTH_PASSWORD | 空 | 可选，启用 Basic Auth 时设置 |

## API 概览

```
GET    /api/papers                        论文列表（search/category/tags/collections/yearFrom/yearTo/read/starred/sort/order/page/pageSize）
POST   /api/papers                        添加论文（multipart，字段 + 可选 pdf 文件）
GET    /api/papers/{id}                   论文详情
PUT    /api/papers/{id}                   更新论文（multipart 或 JSON）
DELETE /api/papers/{id}                   删除论文
GET    /api/papers/{id}/pdf               获取 PDF（?download=1 强制下载）
POST   /api/papers/{id}/toggle-read       切换已读
POST   /api/papers/{id}/toggle-star       切换收藏
POST   /api/papers/{id}/tags              添加标签 {tagId 或 name}
DELETE /api/papers/{id}/tags/{tagId}      移除标签
POST   /api/papers/{id}/collections       加入合集 {collectionId 或 name}
DELETE /api/papers/{id}/collections/{id}  移出合集
POST   /api/papers/{id}/summarize         AI 总结（二期，当前返回 501）

GET/POST/DELETE /api/categories[/{id}]    分类管理
GET/POST/DELETE /api/tags[/{id}]          标签管理
GET/POST/DELETE /api/collections[/{id}]   合集管理
GET    /api/stats                         统计数据
```

## 项目结构

```
paper-manager/
├── cmd/server/            # 入口
├── internal/
│   ├── api/               # HTTP 接口
│   ├── ai/                # AI 接口（预留）
│   ├── models/            # 数据模型
│   ├── pdf/               # PDF 元数据提取
│   └── store/             # SQLite 数据访问
├── web/                   # 前端静态文件
├── Dockerfile
├── docker-compose.yml
└── README.md
```

## 后续规划（二期）

- AI 一句话总结（OpenAI API 或本地模型）
- 富文本笔记
- 界面美化
- 批量导入（BibTeX 等）
- 数据统计图表
- AI 数据库问答
