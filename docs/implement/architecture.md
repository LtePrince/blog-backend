# 后端实现架构

`cmd/main.go` 用 Cobra 接收启动参数，Fx 组装配置、SQLite 连接、业务模块和 Gin 服务。开发配置监听回环地址的 8080 端口。生产配置将数据库放在独立的数据目录，内容仓库由 `content.repo_dir` 指定。[启动入口](../../cmd/main.go) · [开发配置](../../configs/config_dev.yaml) · [生产配置](../../configs/config_prod.yaml)

## 请求与模块边界

| 层 | 职责 |
|---|---|
| `internal/server` | 注册路由、绑定 URI / query / JSON、执行 Bearer 鉴权并统一包装响应。普通 API 成功返回 `{code:"OK", message, data}`；参数绑定失败为 400，服务错误通常为 500。未配置管理令牌时写接口返回 503，缺少或错用令牌时返回 401。 |
| `internal/blog`、`internal/column` | `service` 校验请求并转换输入，`repository` 通过接口执行查询和事务，`schema` 定义 GORM 模型。时间使用 Unix 秒，公开 JSON 使用 snake_case。 |
| `internal/content` | 在内容根目录内读取合法文件，计算正文 SHA-256、处理相对图片引用；同时供博客、专栏及静态资源路由使用。 |
| `internal/moments`、`internal/system` | 前者读 YAML，后者通过系统状态接口采集主机指标，两者独立于博客和专栏的数据库事务。 |

## HTTP 契约

业务接口统一位于 `/api/v1`。健康检查 `GET /health` 和静态图片 `GET/HEAD /static/*filepath` 位于前缀之外。[路由注册](../../internal/server/http.go) · [响应与绑定](../../internal/server/service.go)

| 资源 | 读取 | 管理端写入 |
|---|---|---|
| 博客 | `GET /blogs`、`/blogs/recent`、`/blogs/:id`、`/tags` | `POST /blogs`、`PUT/DELETE /blogs/:id` |
| 专栏 | `GET /columns`、`/columns/tags`、`/columns/:slug` | `POST /columns`、`PUT/DELETE /columns/:slug` |
| 单章 | `GET /columns/:slug/chapters/:chapter` | 在同一路径使用 PUT 发布、DELETE 移除 |
| 动态和统计 | `GET /moments`、`/stats`、`/system/stats` | 只读 |

`GET /api/v1/stats` 返回 `post_count`、`column_count`、`tag_count` 和 `last_updated_at`。一次只读查询汇总全库，取博客有效时间与专栏 `updated_at` 的最大值，单位为 Unix 秒。旧博客沿用列表排序的创建时间、文章日期回退规则；无有效时间返回 0。

列表接受 `page_no`、`page_size`、`filter_title`、`filter_tag` 等参数，响应含 `total` 与 `item_list`。单章 PUT 传 `file`、`title`、`content_hash` 和 `following`（本地目录中排在该章之后的 slug）。服务层核对从文件名派生的 slug；仓储层在同一事务中读服务器正文、校验哈希、定位章节顺序、保存快照并重算专栏时间。校验失败会回滚。专栏 PUT 仍支持显式提交整份章节列表；仅更新专栏信息时应省略 `chapters`。[请求类型](../../internal/column/service/columns.go) · [单章事务](../../internal/column/repository/chapter.go)

## 存储与升级

| SQLite 表 | 数据与关系 |
|---|---|
| `blogs` | 博客 ID、标题、摘要、路径、日期、封面、作者、创建和更新时间，以及 `content_hash`。正文位于内容目录的 `posts/<dir>/index.md`。 |
| `columns` | 专栏 ID、唯一 slug、标题、摘要、作者、目录路径与时间。 |
| `column_chapters` | 通过 `column_id` 关联专栏；保存 slug、标题、Markdown 文件名、排序、创建和更新时间、`content_hash` 及当前 `published_markdown` 快照。每次手动更新覆盖该章的已发布版本。 |
| `tags`、`blog_tags`、`column_tags` | 共享标签及两种多对多关联，支持独立查询博客和专栏的标签计数。 |

启动时先执行 GORM 表结构迁移，再为旧博客补哈希，为缺少快照的既有章节读取服务器 Markdown、建立首份快照并保留原章节时间。表结构迁移失败会阻止启动；正文回填失败会写日志，服务仍可能通过健康检查。核查升级时应同时检查已登记章节的哈希及阅读接口。恢复缺失文件后重启可补齐待迁移行。[博客仓储](../../internal/blog/repository/repository.go) · [章节迁移](../../internal/column/repository/repository.go)

在临时 SQLite 库和临时正文目录中运行 `go test ./...` 可验证发布、排序、迁移和静态资源访问。生产工作流由推送 `main` 触发，构建二进制、切换发布目录并通过 PM2 启动或重载。发布验收应对照提交与工作流记录，再读取受影响的公开接口。[回归用例](../../internal/server/content_test.go) · [部署工作流](../../.github/workflows/main.yaml) · [功能设计](../design/features.md)
