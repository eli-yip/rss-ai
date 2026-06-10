# RSS-AI 网关设计

**日期：** 2026-06-09
**状态：** 设计待审

## 1. 目标

在 RSSHub 前面放一层网关。RSSHub 返回的条目标题有时不够易读，网关对**已注册**的来源用 AI（LLM）重写标题，使其更易读、更准确，并缓存结果以避免重复调用 AI。

源自 [goal.md](../../../goal.md) 的三条核心要求：

1. 所有流量都经过该网关。
2. 路径若注册了处理器 → 处理并缓存到数据库，再返回。
3. 路径若未注册处理器 → 不缓存，直接透传。

## 2. 整体架构

网关是**单个 RSSHub 实例前面的一层反向代理**。

```
RSS 阅读器 ──▶ rss-ai 网关 ──▶ RSSHub（单实例，配置 base URL）
                  │
                  ├─ 已启用前缀：拉取上游 → AI 改标题 → 改 XML → 返回
                  └─ 未启用前缀：纯透传（头/query/响应/状态码原样），不缓存
```

- 上游为**单个** RSSHub 实例，base URL 走配置。
- 未注册路径走纯反向代理，不解析、不缓存。
- 唯一的"处理"语义是 **AI 重写条目标题**；其余字段一律原样保留。

## 3. 请求处理流程

按路径**最长前缀**匹配 config 中已启用的前缀：

```
请求路径
   │
   ├─ 匹配到「已启用」前缀？
   │      否 → 纯透传，不缓存
   │      是 ↓
   │
   ├─ registry 有该前缀的「专属 handler」？
   │      是 → 专属 handler
   │      否 → 通用 handler（AI 改标题）
   │
   └─ 拉上游 → 逐条目改标题 → 缓存标题映射 → 改 XML → 返回
```

条目来源**每次请求都从上游实时拉取**（保证新增/删除条目正确反映）；被缓存的只是"AI 改写后的标题"，不是条目本身。

**输出格式处理：** RSSHub 默认输出 **RSS 2.0**（XML），`?format=atom` 为 Atom（同为 XML），`?format=json` 为 JSON Feed。

- 默认（RSS 2.0）与 Atom 均为 XML → 正常走改标题流程（`etree` 可解析）。
- 显式 `format=json` → 即使前缀已启用，也**直接透传不改**（手术式替换基于 XML，不处理 JSON）。

绝大多数请求是默认 RSS 2.0，会被完整处理；JSON 是少数显式指定的边缘情况。

## 4. 处理器解析（三层）

职责划分：**配置文件管"开不开"，代码管"怎么处理"。**

- **config（TOML）** — 唯一的开关 + 白名单。前缀 → `{ enabled, prompt?, 其他参数? }`。
- **handler registry（代码内）** — 专属 handler 通过 `init()` 自动注册到全局 registry，key 为前缀。用于标题结构特殊、不只是"换 prompt"就能搞定的源。
- **通用 handler（代码内）** — 默认行为"AI 改写标题"，启用但无专属 handler 时兜底，覆盖绝大多数源。

**匹配规则：** 最长前缀匹配。config 同时有 `/github` 和 `/github/issue` 时，`/github/issue/123` 命中更具体的 `/github/issue`；registry 查找用同一个命中的前缀 key。

**prompt 优先级：** `config 中该前缀的 prompt` > `通用 handler 内置默认 prompt`。专属 handler 通常自带逻辑/prompt，但 config 若写了 prompt 仍优先覆盖（运维不改代码即可调整）。

## 5. 标题改写与缓存

### 5.1 改写方式：保真的外科手术式替换

只改标题，其余 XML 一字不动。**用保留完整 DOM 的 XML 库（`beevik/etree`）**，而非字符串/正则手术：

1. 加载整棵文档树。
2. 定位每个 item 的 `title` 节点。
3. 只替换其文本。
4. 序列化回去。

附件（enclosure）、媒体（media）、自定义命名空间、category、author、pubDate、guid 等全部原样保留——避免重新生成 feed 时丢字段（对播客/图片类订阅源是硬伤）。

> 注：因采用手术式替换，技术栈里原列的 `gorilla/feeds` 不再需要。

### 5.2 条目标识

`id` 取值优先级：RSS `<guid>` → Atom `<id>` → 回退 `<link>`。

### 5.3 缓存逻辑

每个 item，按 `(id, handler)` 查库：

- 命中且 `source_title` 一致 → 用 `rewritten_title`。
- 未命中，或上游改了标题（`source_title` 不一致）→ 调 AI 重写，写入/更新该行。

数据库存的是**标题映射**，不是条目本身，表很小、逻辑简单。

## 6. 数据模型（PostgreSQL + GORM）

```
item_titles
─────────────────────────────────
id              TEXT   -- guid / atom id / link 回退
handler         TEXT   -- 命中的前缀 key
source_title    TEXT   -- 原始标题（校验上游是否改过）
rewritten_title TEXT   -- AI 重写后的标题
created_at      TIMESTAMPTZ
updated_at      TIMESTAMPTZ
─────────────────────────────────
主键：(id, handler)
```

写库为应用层"有则更新、无则插入"（GORM）。**不使用 DB 层 upsert/冲突约束**——靠 singleflight 保证每个 `(id, handler)` 同时只有一个 writer（见 §7）。

## 7. 并发设计

进程内机制，**不引入 Redis**。三件套 + 一个关键陷阱处理：

### 7.1 同条目去重（singleflight）

`golang.org/x/sync/singleflight`，key = `handler|id`。并发请求遇到同一篇未缓存文章时，只真正发起**一次** AI 调用，其余复用结果。同时充当写库的序列化机制。

### 7.2 AI 限流（rate limiter）

`golang.org/x/time/rate` 令牌桶，**10,000 RPM**（≈167 token/s，带少量 burst）。突然冒出大量新文章时排队消化，不打爆上游/账单。

### 7.3 关键陷阱：超时回退 ≠ 取消后台任务

请求超时只是"**这次不等了**"，重写**不能跟着请求一起取消**，否则后台永远补不齐。两条 context 分离：

- **AI 调用 context**：独立、生命周期更长（background + 自带超时，如 30s），**不**挂在请求 context 上。
- **请求侧**：带 **10s** 超时的 `select`，等到结果就用，超时就拿原始标题先返回；goroutine 在后台跑完并写库。

配合 §7.1：后台 goroutine 还在跑时 singleflight key 一直占用，下一个请求直接复用在途调用——超时与后台衔接天然对上。

### 7.4 失败处理

AI 调用失败 → 用原始标题返回，**不写库**，下次请求再试。绝不把失败/原始标题当成功结果缓存。

## 8. AI 集成

- 通过**任意 OpenAI 兼容接口**调用，模型/endpoint/key 走配置。
- 输入：条目原始标题（+ 处理器 prompt）。输出：重写后的标题。
- prompt 解析见 §4。

## 9. 配置（TOML 草图）

Config is **TOML-only** (no env-var overrides). `config.toml` holds secrets
(AI key, DB DSN) and is **gitignored**; a placeholder `config.example.toml` is
committed. On startup, if the file is missing it is created from defaults
(maestro pattern). Loaded with `go-toml/v2` into a global `config.C`.

```toml
[app]
env = "dev"            # dev | prod —— 决定日志编码，并作为 Loki 的 env label

[log]
level = "info"

[db]
dsn = "postgres://user:pass@host:5432/rss_ai?sslmode=disable"

[upstream]
base_url = "https://rsshub.example.com"

[ai]
base_url = "https://api.openai.com/v1"
api_key  = "..."
model    = "..."
rpm      = 10000
request_timeout = "30s"

[gateway]
wait_timeout = "10s"   # 请求侧等待超时

# 前缀启用表（开关 + 白名单）
[handlers."/twitter"]
enabled = true
prompt  = "把这条推文标题改写得更易读……"   # 可选，缺省用通用 handler 默认 prompt

[handlers."/github/issue"]
enabled = true
# 无 prompt → 用默认 prompt；若代码里注册了专属 handler 则用之
```

## 10. 技术栈

- **Go 1.26**，module `github.com/eli-yip/rss-ai`
- **Echo/v5**（HTTP），`httputil`（`NewHTTPError` / `Resp[T]`，命名 struct 响应）
- **GORM + PostgreSQL**（标题映射缓存，**AutoMigrate** 建表）
- **beevik/etree**（保真 XML 改写，替代 gorilla/feeds）
- **golang.org/x/sync/singleflight**（同条目去重）
- **golang.org/x/time/rate**（AI 限流）
- **resty/v3**（HTTP 客户端：上游抓取、OpenAI 兼容接口）
- **go-toml/v2**（配置）；**urfave/cli/v3**（CLI，入口 `cmd/rss-ai`）
- **zap + lumberjack**（日志，封装为 `mlog`）
- 反向代理：标准库 `net/http/httputil`（透传）
- ~~Redis~~（本期不用）
- ~~gorilla/feeds~~（手术式替换后不需要）

约定全面参考 `maestro-engine`（工具链、config/mlog/httputil 模式），CI 除外。

## 11. 可观测性

后端只有 **Loki（经 Alloy 采集）+ Grafana**——无 Prometheus/Tempo。因此可观测性
**以结构化日志为中心**，"指标"和"追踪"都从日志派生（LogQL 聚合 + Grafana 看板）。

### 11.1 标签低基数，细节进字段

- **Loki labels（仅这几个，低基数）**：`service="rss-ai"`、`env`(dev/prod)、`level`。
- **高基数信息进 JSON 日志体**（LogQL `| json` 过滤）：`handler`、`path`、`item_id`、
  `upstream_url`、`status`、`trace_id` 等。**绝不**把 `handler`/`item_id` 设为 Loki label。

### 11.2 传输

应用把日志以 **JSON 打到 stdout**，Alloy 抓容器 stdout → Loki。`env=dev` 时用彩色
console 编码（本地可读），`env=prod` 时用 JSON 编码；两者都走 stdout。

### 11.3 关联 ID

复用 `mlog` 的 `trace_id`（ctx 注入）。Echo 中间件给每个请求分配 `trace_id`，透传到
上游抓取、缓存、AI 调用，**以及超时后继续的后台重写 goroutine**（后台任务沿用同一
`trace_id`，使一次请求引发的全部动作——包括它返回后才完成的重写——可串联。）

### 11.4 结构化事件（同时是指标源）

每事件一行 JSON，字段固定：

| 事件                         | 级别       | 关键字段                                                                                                                    |
| ---------------------------- | ---------- | --------------------------------------------------------------------------------------------------------------------------- |
| `request.done`（每请求一条） | INFO       | `path, handler, mode(handled\|passthrough), status, duration_ms, item_count, cache_hits, cache_misses, ai_calls, timed_out` |
| `upstream.fetch`             | INFO/WARN  | `upstream_url, status, duration_ms, bytes`                                                                                  |
| `ai.rewrite`                 | INFO/WARN  | `handler, item_id, duration_ms, ok, err`                                                                                    |
| `rewrite.background_done`    | INFO       | `handler, item_id, waited_ms`                                                                                               |
| `ratelimit.wait`             | DEBUG/INFO | `waited_ms`                                                                                                                 |
| `error`                      | ERROR      | `where, err`（带堆栈）                                                                                                      |

逐条目命中/未命中明细走 **DEBUG**；INFO 只出每请求一条聚合 `request.done`，避免日志量
随条目数爆炸。

### 11.5 级别约定

- **INFO**：正常请求/抓取/重写完成。
- **WARN**：超时回退（返回原始标题）、AI 单次失败将重试、上游非 2xx。
- **ERROR**：彻底失败、DB 错误、配置错误。
- **DEBUG**：逐条目缓存命中、singleflight 合并、限流细节。

### 11.6 Grafana 看板（基于 LogQL 派生）

- 缓存命中率 `cache_hits / (cache_hits + cache_misses)`。
- AI 调用速率（盯 10k RPM 上限）与 AI 错误率。
- 请求延迟分位 p50/p95/p99（`quantile_over_time` over `duration_ms`）。
- 超时回退率（`timed_out=true` 占比）。
- handled vs passthrough 占比、按 `handler` 的 Top-N 请求量。
- 上游延迟/错误率。

### 11.7 健康检查

`/healthz`（存活）、`/readyz`（含 DB ping）。运维用，不进 Loki。

## 12. 开发环境与工具链

- **入口**：`cmd/rss-ai`，`urfave/cli/v3` 根命令启动网关。单体服务。
- **依赖服务**：PostgreSQL 与上游 RSSHub **均连远程/已有实例**（走 `config.toml`），
  本项目**不提供 compose.dev.yaml**。本地开发即 `go run ./cmd/rss-ai` + 指向远程的配置。
- **工具链**（参考 maestro，CI 除外）：`just`（justfile）任务；`golangci-lint` v2 +
  `autocorrect` + `dprint`(markdown) + `go mod tidy -diff` 组成 `just lint`；`lefthook`
  pre-push 跑 `just lint`；`goreleaser` 构建。
- **测试**：本项目逻辑纯软件，**遵循 TDD、可自由跑测试**（不沿用 maestro 的 "never run
  tests" 规则）。
- **配置**：仅 TOML；`config.toml` 含密钥、**gitignore**；提交 `config.example.toml`。

## 13. 测试策略

- 处理器解析：最长前缀匹配、启用/未启用、专属 vs 通用、prompt 优先级。
- XML 改写：保真性（附件/媒体/命名空间不丢）、CDATA/转义、缺 guid 回退 link。
- 缓存：命中、source_title 变化触发重写、未命中写库。
- 并发：singleflight 去重（同 key 只一次 AI）、限流、超时回退后后台补齐且不缓存原始标题、AI 失败不写库。
- 透传：未注册路径头/query/状态码原样、不缓存。

## 14. 非目标 / YAGNI

- 不缓存上游响应（透传不缓存；注册路径每次实时拉上游）。
- 不引入 Redis。
- 不做运行时增删处理器的管理界面（注册改 config + 重启）。
- 不支持多上游。
- 不处理标题以外的字段（正文/翻译/摘要等暂不做）。
- 不重新生成 feed（仅手术式改标题）。
- 不引入 Prometheus/Tempo——指标与追踪全部从日志派生。
- 配置不做环境变量覆盖（仅 TOML）。
