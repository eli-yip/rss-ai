# rss-ai

An AI title-rewriting gateway that sits in front of a single
[RSSHub](https://github.com/DIYgod/RSSHub) instance.

RSSHub item titles are sometimes hard to read. For **registered** path prefixes,
`rss-ai` fetches the feed from upstream, rewrites each item's title with an LLM,
caches the result, and returns a surgically-modified feed — only the `<title>`
changes; every other field is preserved byte-for-byte. **Unregistered** paths are
reverse-proxied through unchanged and uncached.

Point your RSS reader at the gateway instead of at RSSHub, and registered feeds
come back with cleaner, more readable titles.

## Highlights

- **Only the titles change** — enclosures, media, dates, and every other field
  come through exactly as RSSHub produced them, so podcasts and image feeds keep
  working.
- **You choose which feeds** — rewriting is opt-in per path prefix; everything
  else is passed straight through, untouched.
- **Tunable per feed** — give a prefix your own rewrite instructions, or rely on
  the sensible default. Some sources (like Telegram channels) get built-in
  handling.
- **No repeated LLM calls** — rewritten titles are cached, so a feed is only sent
  to the LLM when its titles actually change.
- **Never blocks your reader** — if a rewrite is slow, the original title is
  returned right away and the rewrite finishes in the background.

## Quick start (Docker)

A ready-to-use image is published as `eliyip/rss-ai`. Docker is the recommended
way to run the gateway.

```sh
# 1. Write a config.toml (DB DSN, upstream RSSHub, AI key, enabled prefixes)
cp config.example.toml config.toml   # then edit it

# 2. Run, mounting your config to /app/config.toml
docker run -d --name rss-ai \
  -p 8080:8080 \
  -v "$PWD/config.toml:/app/config.toml:ro" \
  eliyip/rss-ai:latest
```

Subscribe your reader to the gateway, e.g. `http://localhost:8080/telegram/channel/durov`.

See the usage docs below for a `compose` snippet and configuration details.

## Documentation

- **Usage** — [English](docs/USAGE.md) · [中文](docs/USAGE.zh.md)

## License

[MIT](LICENSE)
