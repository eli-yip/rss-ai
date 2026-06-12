# Usage

`rss-ai` sits in front of your RSSHub. For the feeds you choose, it rewrites the
item titles with an LLM so they read better; everything else comes through exactly
as RSSHub produced it. Feeds you don't choose are passed straight through.

You point your RSS reader at `rss-ai` instead of at RSSHub, and that's it.

## What you need

- An **RSSHub** instance (your own, or one you already use).
- A **database** — `rss-ai` stores rewritten titles here so it doesn't call the
  LLM again for the same item. Use **PostgreSQL**, or **SQLite** if you'd rather
  not run a separate database (it's just a single file, no extra service). The
  table is created on first run either way.
- An **API key** for any **OpenAI-compatible** LLM (OpenAI, a local model, etc.).

## Deploy with Docker

The recommended way to run `rss-ai`. The image is published as `eliyip/rss-ai`.

**1. Create your config.** Copy the example and fill it in:

```sh
cp config.example.toml config.toml
```

At minimum, set your RSSHub URL, database, LLM key, and which feeds to rewrite
(see [Configuration](#configuration) below).

**2. Start the container.** Mount your `config.toml` at `/app/config.toml`:

```sh
docker run -d --name rss-ai \
  -p 8080:8080 \
  -v "$PWD/config.toml:/app/config.toml:ro" \
  eliyip/rss-ai:latest
```

Or with Compose, bringing up its own PostgreSQL alongside it:

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

Services on the same Compose network reach each other by name, so in
`config.toml` the database `dsn` becomes:

```
postgres://rss_ai:change-me@db:5432/rss_ai?sslmode=disable
```

Point `upstream.base_url` at your RSSHub the same way (e.g. `http://rsshub:1200`
if it's a service in the same Compose file).

## Configuration

Everything lives in `config.toml`. The essentials:

```toml
[upstream]
base_url = "https://rsshub.example.com"   # your RSSHub

[db]
dsn = "postgres://user:pass@host:5432/rss_ai?sslmode=disable"

[ai]
base_url = "https://api.openai.com/v1"
api_key  = "sk-..."
model    = "gpt-4o-mini"
```

The database driver is chosen from the `dsn` scheme — `postgres://…` (or
`postgresql://…`) for PostgreSQL, `sqlite:///path/to/rss-ai.db` for a SQLite
file. With SQLite, mount a volume so the file survives restarts, e.g.
`-v "$PWD/data:/data"` and `dsn = "sqlite:///data/rss-ai.db"`.

Other settings you can leave at their defaults: `[server] addr` (listen address,
default `:8080`), `[ai] rpm` and `request_timeout` (rate limit and per-call
timeout), `[gateway] wait_timeout` (how long a request waits for a rewrite before
returning the original title and finishing in the background), and `[app] env` /
`[log] level` (logging).

### Choosing which feeds get rewritten

A feed is rewritten **only if** you list its path prefix here and enable it.
Anything not listed is passed straight through, untouched.

```toml
# Rewrite every feed under /twitter.
[handlers."/twitter"]
enabled = true
prompt  = "Rewrite this tweet title to be more readable…"  # optional

# Telegram channel titles are often truncated, so there's built-in handling.
[handlers."/telegram/channel"]
enabled = true
```

- `enabled` is the on/off switch for that prefix.
- `prompt` lets you tell the LLM how to rewrite titles for that prefix. It's
  optional — leave it out and a sensible default is used.
- Prefixes match by **longest match**, so `/github/issue` can have different
  settings from a broader `/github`.

## Subscribe

Point your RSS reader at `rss-ai` using the same path you'd use on RSSHub. If the
gateway is at `localhost:8080`:

```
http://localhost:8080/telegram/channel/durov
```

A configured feed comes back with rewritten titles; any other path behaves
exactly like RSSHub. One thing to know: rewriting applies to the default RSS
output. If you request `?format=atom` or `?format=json`, the feed passes through
unchanged.

## Health checks

- `GET /healthz` — is the gateway up?
- `GET /readyz` — is it up _and_ can it reach the database?

Handy for container health checks and load balancers.

## Run from source

If you'd rather not use Docker — e.g. for local development — point `config.toml`
at your services and run:

```sh
go run ./cmd/rss-ai -c config.toml
```
