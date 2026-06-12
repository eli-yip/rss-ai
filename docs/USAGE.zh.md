# 使用文档

`rss-ai` 放在你的 RSSHub 前面。对你选定的订阅源，它用 LLM 把条目标题改写得更易读，其余
内容则与 RSSHub 原样一致；没选的订阅源直接透传。

你只需把 RSS 阅读器指向 `rss-ai` 而非 RSSHub，就这么简单。

## 准备工作

- 一个 **RSSHub** 实例（你自己的，或你已经在用的）。
- 一个 **PostgreSQL** 数据库——`rss-ai` 把改写后的标题存在这里，避免对同一条目重复调用
  LLM。数据表会在首次运行时自动创建。
- 一个 **OpenAI 兼容**的 LLM 的 **API key**（OpenAI、本地模型等均可）。

## 用 Docker 部署

推荐的运行方式。镜像发布为 `eliyip/rss-ai`。

**1. 准备配置。** 复制示例并填写：

```sh
cp config.example.toml config.toml
```

至少要填上你的 RSSHub 地址、数据库、LLM key，以及要改写哪些订阅源（见下面的
[配置](#配置)）。

**2. 启动容器。** 把 `config.toml` 挂载到 `/app/config.toml`：

```sh
docker run -d --name rss-ai \
  -p 8080:8080 \
  -v "$PWD/config.toml:/app/config.toml:ro" \
  eliyip/rss-ai:latest
```

或用 Compose，连同它自带的 PostgreSQL 一起启动：

```yaml
services:
  rss-ai:
    image: eliyip/rss-ai:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - ./config.toml:/app/config.toml:ro
    depends_on:
      - db

  db:
    image: postgres:17-alpine
    restart: unless-stopped
    environment:
      POSTGRES_USER: rss_ai
      POSTGRES_PASSWORD: change-me
      POSTGRES_DB: rss_ai
    volumes:
      - rss-ai-db:/var/lib/postgresql/data

volumes:
  rss-ai-db:
```

同一个 Compose 网络里的服务可以用服务名互相访问，所以 `config.toml` 里的数据库 `dsn`
就写成：

```
postgres://rss_ai:change-me@db:5432/rss_ai?sslmode=disable
```

`upstream.base_url` 同理指向你的 RSSHub（若它也是同一 Compose 文件里的服务，如
`http://rsshub:1200`）。

## 配置

所有设置都在 `config.toml` 里。必填的几项：

```toml
[upstream]
base_url = "https://rsshub.example.com"   # 你的 RSSHub

[db]
dsn = "postgres://user:pass@host:5432/rss_ai?sslmode=disable"

[ai]
base_url = "https://api.openai.com/v1"
api_key  = "sk-..."
model    = "gpt-4o-mini"
```

其余设置一般用默认值即可：`[server] addr`（监听地址，默认 `:8080`）、`[ai] rpm` 与
`request_timeout`（限流与单次调用超时）、`[gateway] wait_timeout`（一次请求最多等多久；
超时就先返回原标题，改写在后台继续完成）、以及 `[app] env` / `[log] level`（日志）。

### 选择要改写的订阅源

**只有**你在这里列出并启用的路径前缀才会被改写；没列出的一律原样透传。

```toml
# 改写 /twitter 下的所有订阅源。
[handlers."/twitter"]
enabled = true
prompt  = "把这条推文标题改写得更易读……"  # 可选

# Telegram 频道标题常被截断，因此内置了专门处理。
[handlers."/telegram/channel"]
enabled = true
```

- `enabled` 是该前缀的开关。
- `prompt` 用来告诉 LLM 如何改写该前缀下的标题。可选——不写则使用合理的默认值。
- 前缀按**最长匹配**生效，所以 `/github/issue` 可以和更宽泛的 `/github` 用不同的设置。

## 订阅

用你在 RSSHub 上原本的路径，把 RSS 阅读器指向 `rss-ai`。若网关在 `localhost:8080`：

```
http://localhost:8080/telegram/channel/durov
```

已配置的订阅源会返回改写后的标题；其他路径的行为与 RSSHub 完全一致。有一点需要知道：
改写只作用于默认的 RSS 输出。若你请求 `?format=atom` 或 `?format=json`，订阅源会原样
透传、不改写。

## 健康检查

- `GET /healthz`——网关是否存活？
- `GET /readyz`——是否存活**且**能连上数据库？

适合用于容器健康检查和负载均衡。

## 从源码运行

如果你不想用 Docker（比如本地开发），把 `config.toml` 指向你的服务后运行：

```sh
go run ./cmd/rss-ai -c config.toml
```
