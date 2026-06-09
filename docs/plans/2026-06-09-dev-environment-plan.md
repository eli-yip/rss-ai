# Dev Environment (plan-0) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Spec:** `docs/spec/2026-06-09-rss-ai-gateway-design.md`
**Lessons:** `docs/lessons/2026-06-09-dev-environment-lessons.md` (append during execution)
**Status:** not started

**Goal:** Stand up a runnable, observable service skeleton — config, logging, DB, and an HTTP server with health checks — onto which later plans bolt the proxy and rewrite logic.

**Architecture:** A single binary (`cmd/rss-ai`, `urfave/cli/v3`) loads a TOML config, builds a stdout JSON/console logger (`mlog`), connects to a remote PostgreSQL via GORM and AutoMigrates the `item_titles` table, then serves an Echo v5 HTTP server exposing `/healthz` and `/readyz` with a `trace_id` middleware. No proxy or rewrite logic yet.

**Tech Stack:** Go 1.26, Echo/v5, GORM + PostgreSQL, go-toml/v2, urfave/cli/v3, zap. Tooling: just, golangci-lint v2, lefthook, dprint, goreleaser (conventions mirrored from `maestro-engine`, CI excluded).

---

## File Structure

| File | Responsibility |
|------|----------------|
| `go.mod` / `go.sum` | Module `github.com/eli-yip/rss-ai`, go 1.26. |
| `justfile` | Task runner: `run`, `build`, `lint`, `fmt`, `test`. |
| `.golangci.toml` | golangci-lint v2 config. |
| `.lefthook.toml` | pre-push runs `just lint`. |
| `dprint.json` | Markdown formatting. |
| `.goreleaser.yaml` | Single-binary build for `cmd/rss-ai`. |
| `.gitignore` | Append `config.toml`, `/dist/`. |
| `config.example.toml` | Committed config template (placeholders). |
| `pkg/ctxutil/ctxutil.go` | Typed context value helpers. |
| `pkg/mlog/zap.go` | `mlog.Logger` (zap wrapper), stdout, env-based encoder, `trace_id`. |
| `pkg/mlog/level.go` | `LogLevelFromString`. |
| `pkg/config/config.go` | `Config` struct, `Init`, global `C`. |
| `pkg/httputil/error.go` | `NewHTTPError`, Echo error handler. |
| `pkg/httputil/resp.go` | `Resp[T]`, `NewMessage`. |
| `pkg/store/store.go` | GORM connect, `ItemTitle` model, AutoMigrate, `Ping`. |
| `pkg/server/server.go` | Echo bootstrap, `/healthz`, `/readyz`, Start/Stop. |
| `pkg/server/middleware.go` | `traceIDMiddleware`, `requestLoggerMiddleware`. |
| `cmd/rss-ai/main.go` | CLI entry: wire config → logger → store → server. |

Tests live beside each package (`*_test.go`).

---

## Task 1: Module + tooling scaffolding

**Files:**
- Create: `go.mod` (via `go mod init`), `justfile`, `.golangci.toml`, `.lefthook.toml`, `dprint.json`, `.goreleaser.yaml`, `config.example.toml`
- Modify: `.gitignore`

- [ ] **Step 1: Initialize the module**

Run:
```bash
go mod init github.com/eli-yip/rss-ai
```

Then edit `go.mod` so the Go line reads exactly:
```
go 1.26
```

- [ ] **Step 2: Add core dependencies**

Run:
```bash
go get github.com/labstack/echo/v5@latest
go get github.com/urfave/cli/v3@latest
go get github.com/pelletier/go-toml/v2@latest
go get go.uber.org/zap@latest
go get gorm.io/gorm@latest gorm.io/driver/postgres@latest
go get github.com/stretchr/testify@latest
```

Expected: `go.mod` lists these under `require`; `go.sum` populated.

- [ ] **Step 3: Create `justfile`**

```just
default:
    @just --list

[group('dev')]
run *args:
    go run ./cmd/rss-ai {{ args }}

[group('build')]
build:
    goreleaser build --snapshot --clean --single-target

[group('lint')]
lint:
    dprint check
    go mod tidy -diff
    go vet ./...
    golangci-lint run -v --timeout 60s

[group('lint')]
fmt:
    dprint fmt
    go fmt ./...

[group('test')]
test:
    go test ./...
```

- [ ] **Step 4: Create `.golangci.toml`**

```toml
#:schema https://golangci-lint.run/jsonschema/golangci.jsonschema.json
version = "2"

[[linters.exclusions.rules]]
linters = [ "errcheck" ]
source = "^\\s*defer\\s+" # don't flag deferred calls returning error
```

- [ ] **Step 5: Create `.lefthook.toml`**

```toml
[pre-push.commands.lint]
run = "just lint"
```

- [ ] **Step 6: Create `dprint.json`**

```json
{
	"includes": ["**/*.md"],
	"plugins": [
		"https://plugins.dprint.dev/markdown-0.21.1.wasm"
	]
}
```

- [ ] **Step 7: Create `.goreleaser.yaml`**

```yaml
version: 2
project_name: rss-ai

before:
  hooks:
    - go mod tidy

builds:
  - id: rss-ai
    main: ./cmd/rss-ai
    binary: rss-ai
    env:
      - CGO_ENABLED=0
    goos: [linux, darwin]
    goarch: [amd64, arm64]
```

- [ ] **Step 8: Create `config.example.toml`**

```toml
[app]
env = "dev"            # dev | prod

[log]
level = "info"         # debug | info | error

[server]
addr = ":8080"

[db]
dsn = "postgres://user:pass@host:5432/rss_ai?sslmode=disable"

[upstream]
base_url = "https://rsshub.example.com"

[ai]
base_url = "https://api.openai.com/v1"
api_key  = "sk-replace-me"
model    = "gpt-4o-mini"
rpm      = 10000
request_timeout = "30s"

[gateway]
wait_timeout = "10s"

# Enabled prefixes (switch + allowlist). prompt is optional.
[handlers."/twitter"]
enabled = true
prompt  = "Rewrite this title to be clear and readable."
```

- [ ] **Step 9: Append to `.gitignore`**

Append these lines to the end of `.gitignore`:
```gitignore
# rss-ai
/config.toml
/dist/
```

- [ ] **Step 10: Commit**

```bash
git add -A
git commit -m "build: scaffold module, tooling, and config template"
```

---

## Task 2: ctxutil + mlog logging

**Files:**
- Create: `pkg/ctxutil/ctxutil.go`, `pkg/mlog/zap.go`, `pkg/mlog/level.go`, `pkg/mlog/level_test.go`

- [ ] **Step 1: Write the failing test** — `pkg/mlog/level_test.go`

```go
package mlog_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"

	"github.com/eli-yip/rss-ai/pkg/mlog"
)

func TestLogLevelFromString(t *testing.T) {
	lvl, err := mlog.LogLevelFromString("debug")
	require.NoError(t, err)
	require.Equal(t, zapcore.DebugLevel, lvl)

	lvl, err = mlog.LogLevelFromString("info")
	require.NoError(t, err)
	require.Equal(t, zapcore.InfoLevel, lvl)

	_, err = mlog.LogLevelFromString("bogus")
	require.Error(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/mlog/ -run TestLogLevelFromString -v`
Expected: FAIL (package/function undefined).

- [ ] **Step 3: Create `pkg/ctxutil/ctxutil.go`**

```go
package ctxutil

import "context"

// ValueOr extracts a typed value from a context, returning fallback if the key
// is missing or the stored type does not match.
func ValueOr[T any](ctx context.Context, key any, fallback T) T {
	v, ok := ctx.Value(key).(T)
	if !ok {
		return fallback
	}
	return v
}
```

- [ ] **Step 4: Create `pkg/mlog/level.go`**

```go
package mlog

import (
	"fmt"

	"go.uber.org/zap/zapcore"
)

// LogLevelFromString maps a config string to a zap level.
func LogLevelFromString(level string) (zapcore.Level, error) {
	levels := map[string]zapcore.Level{
		"debug": zapcore.DebugLevel,
		"info":  zapcore.InfoLevel,
		"error": zapcore.ErrorLevel,
	}
	if lvl, ok := levels[level]; ok {
		return lvl, nil
	}
	return zapcore.InfoLevel, fmt.Errorf("invalid log level: %s", level)
}
```

- [ ] **Step 5: Create `pkg/mlog/zap.go`**

```go
package mlog

import (
	"context"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/eli-yip/rss-ai/pkg/ctxutil"
)

type TraceIDKey struct{}

// TraceIDCtxKey is the context key under which the per-request trace id lives.
var TraceIDCtxKey = TraceIDKey{}

// CopyTraceID carries the trace id from one context to another — used when a
// background goroutine outlives the originating request.
func CopyTraceID(from, to context.Context) context.Context {
	return context.WithValue(to, TraceIDCtxKey, ctxutil.ValueOr[uint64](from, TraceIDCtxKey, 0))
}

type Logger struct {
	*zap.Logger
	level zap.AtomicLevel
}

func (l *Logger) SetLevel(level zapcore.Level) { l.level.SetLevel(level) }

// New builds a logger that writes to stdout. env "dev" uses a colored console
// encoder for readability; any other env (e.g. "prod") uses JSON so Alloy can
// ship structured lines to Loki. service and env are baked in as base fields.
func New(env string, level zapcore.Level) *Logger {
	encCfg := zap.NewProductionEncoderConfig()
	encCfg.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05.000")

	var enc zapcore.Encoder
	if env == "dev" {
		encCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		enc = zapcore.NewConsoleEncoder(encCfg)
	} else {
		enc = zapcore.NewJSONEncoder(encCfg)
	}

	atom := zap.NewAtomicLevelAt(level)
	core := zapcore.NewCore(enc, zapcore.Lock(os.Stdout), atom)
	zl := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel)).
		With(zap.String("service", "rss-ai"), zap.String("env", env))

	return &Logger{Logger: zl, level: atom}
}

// L returns a logger with the request's trace_id attached.
func L(ctx context.Context, logger *Logger) *Logger {
	return &Logger{
		Logger: logger.Logger.With(zap.Uint64("trace_id", ctxutil.ValueOr[uint64](ctx, TraceIDCtxKey, 0))),
		level:  logger.level,
	}
}

// S returns a logger tagged with a service/component name.
func S(logger *Logger, service string) *Logger {
	return &Logger{
		Logger: logger.Logger.With(zap.String("service", service)),
		level:  logger.level,
	}
}

func (l *Logger) With(fields ...zap.Field) *Logger {
	return &Logger{Logger: l.Logger.With(fields...), level: l.level}
}

// NewNop returns a no-op logger for tests.
func NewNop() *Logger {
	return &Logger{Logger: zap.NewNop(), level: zap.NewAtomicLevelAt(zapcore.InfoLevel)}
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./pkg/mlog/ -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/ctxutil pkg/mlog
git commit -m "feat(mlog): stdout logger with env-based encoding and trace_id"
```

---

## Task 3: Config package

**Files:**
- Create: `pkg/config/config.go`, `pkg/config/config_test.go`

- [ ] **Step 1: Write the failing test** — `pkg/config/config_test.go`

```go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eli-yip/rss-ai/pkg/config"
)

func TestInitLoadsConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[app]
env = "prod"
[log]
level = "debug"
[server]
addr = ":9090"
[db]
dsn = "postgres://localhost/test"
[upstream]
base_url = "https://rsshub.example.com"
[ai]
base_url = "https://api.openai.com/v1"
api_key = "sk-test"
model = "gpt-4o-mini"
rpm = 10000
request_timeout = "30s"
[gateway]
wait_timeout = "10s"
[handlers."/twitter"]
enabled = true
prompt = "rewrite"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	require.NoError(t, config.Init(path))

	require.Equal(t, "prod", config.C.App.Env)
	require.Equal(t, ":9090", config.C.Server.Addr)
	require.Equal(t, 10000, config.C.AI.RPM)
	require.Equal(t, "https://rsshub.example.com", config.C.Upstream.BaseURL)

	h, ok := config.C.Handlers["/twitter"]
	require.True(t, ok)
	require.True(t, h.Enabled)
	require.Equal(t, "rewrite", h.Prompt)
}

func TestInitCreatesDefaultWhenMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	require.NoError(t, config.Init(path))
	require.FileExists(t, path)
	require.Equal(t, "dev", config.C.App.Env)
	require.Equal(t, ":8080", config.C.Server.Addr)
	require.Equal(t, "info", config.C.Log.Level)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/config/ -v`
Expected: FAIL (package undefined).

- [ ] **Step 3: Create `pkg/config/config.go`**

```go
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

type App struct {
	Env string `toml:"env"` // dev | prod
}

type Log struct {
	Level string `toml:"level"`
}

type Server struct {
	Addr string `toml:"addr"`
}

type DB struct {
	DSN string `toml:"dsn"`
}

type Upstream struct {
	BaseURL string `toml:"base_url"`
}

type AI struct {
	BaseURL        string `toml:"base_url"`
	APIKey         string `toml:"api_key"`
	Model          string `toml:"model"`
	RPM            int    `toml:"rpm"`
	RequestTimeout string `toml:"request_timeout"`
}

type Gateway struct {
	WaitTimeout string `toml:"wait_timeout"`
}

type HandlerConfig struct {
	Enabled bool   `toml:"enabled"`
	Prompt  string `toml:"prompt"`
}

type Config struct {
	App      App                      `toml:"app"`
	Log      Log                      `toml:"log"`
	Server   Server                   `toml:"server"`
	DB       DB                       `toml:"db"`
	Upstream Upstream                 `toml:"upstream"`
	AI       AI                       `toml:"ai"`
	Gateway  Gateway                  `toml:"gateway"`
	Handlers map[string]HandlerConfig `toml:"handlers"`
}

// C is the loaded global config.
var C *Config

func defaultConfig() Config {
	return Config{
		App:     App{Env: "dev"},
		Log:     Log{Level: "info"},
		Server:  Server{Addr: ":8080"},
		AI:      AI{BaseURL: "https://api.openai.com/v1", RPM: 10000, RequestTimeout: "30s"},
		Gateway: Gateway{WaitTimeout: "10s"},
	}
}

// Init loads the TOML config at configPath into C. If the file is missing, a
// default config is written there first.
func Init(configPath string) error {
	if configPath == "" {
		configPath = "config.toml"
	}

	if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
		data, err := toml.Marshal(defaultConfig())
		if err != nil {
			return fmt.Errorf("marshal default config: %w", err)
		}
		if dir := filepath.Dir(configPath); dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("create config dir %s: %w", dir, err)
			}
		}
		if err := os.WriteFile(configPath, data, 0o644); err != nil {
			return fmt.Errorf("write default config %s: %w", configPath, err)
		}
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config %s: %w", configPath, err)
	}

	cfg := defaultConfig()
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse config %s: %w", configPath, err)
	}

	C = &cfg
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/config/ -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add pkg/config
git commit -m "feat(config): TOML config loader with defaults"
```

---

## Task 4: httputil (error + response)

**Files:**
- Create: `pkg/httputil/error.go`, `pkg/httputil/resp.go`, `pkg/httputil/resp_test.go`

- [ ] **Step 1: Write the failing test** — `pkg/httputil/resp_test.go`

```go
package httputil_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eli-yip/rss-ai/pkg/httputil"
)

func TestNewResp(t *testing.T) {
	r := httputil.NewResp("ok", 42)
	require.Equal(t, "ok", r.Message)
	require.Equal(t, 42, r.Data)
}

func TestNewMessage(t *testing.T) {
	m := httputil.NewMessage("hi")
	require.Equal(t, "hi", m.Message)
}

func TestNewHTTPError(t *testing.T) {
	err := httputil.NewHTTPError(404, "not found")
	require.Equal(t, 404, err.Code)
	require.Equal(t, "not found", err.Error())
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/httputil/ -v`
Expected: FAIL (package undefined).

- [ ] **Step 3: Create `pkg/httputil/resp.go`**

```go
package httputil

type Resp[T any] struct {
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type EmptyResp struct {
	Message string `json:"message"`
}

func NewResp[T any](message string, data T) Resp[T] {
	return Resp[T]{Message: message, Data: data}
}

func NewMessage(message string) EmptyResp { return EmptyResp{Message: message} }
```

- [ ] **Step 4: Create `pkg/httputil/error.go`**

```go
package httputil

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"
	"go.uber.org/zap"

	"github.com/eli-yip/rss-ai/pkg/mlog"
)

type ResponseError struct {
	Code    int
	Message string
}

func (e *ResponseError) Error() string { return e.Message }

func NewHTTPError(code int, message string) *ResponseError {
	return &ResponseError{Code: code, Message: message}
}

type EchoHTTPErrorHandler struct{ logger *mlog.Logger }

func NewEchoHTTPErrorHandler(logger *mlog.Logger) *EchoHTTPErrorHandler {
	return &EchoHTTPErrorHandler{logger: logger}
}

func (e *EchoHTTPErrorHandler) DefaultEchoHTTPErrorHandler(c *echo.Context, err error) {
	resp, _ := echo.UnwrapResponse(c.Response())
	if resp.Committed {
		return
	}

	logger := mlog.L(c.Request().Context(), e.logger)
	defer logger.Error("failed to handle request", zap.Error(err))

	code := http.StatusInternalServerError
	if responseErr, ok := errors.AsType[*ResponseError](err); ok {
		code = responseErr.Code
		_ = c.JSON(code, NewMessage(responseErr.Message))
		return
	}

	_ = c.JSON(code, NewMessage(http.StatusText(code)))
}
```

> Note: `errors.AsType[T]` and Echo v5's `*echo.Context` / `echo.UnwrapResponse` are
> the exact forms used in maestro-engine on Go 1.26. If the build reports a
> different v5 signature, fix to match and record it in the lessons file.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./pkg/httputil/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/httputil
git commit -m "feat(httputil): error handler and typed responses"
```

---

## Task 5: Store (GORM + item_titles)

**Files:**
- Create: `pkg/store/store.go`, `pkg/store/store_test.go`

- [ ] **Step 1: Write the failing test** — `pkg/store/store_test.go`

The DB lives on a remote instance, so this is an integration test guarded by an
env var; it skips when unset.

```go
package store_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eli-yip/rss-ai/pkg/store"
)

func TestStoreAutoMigrateAndPing(t *testing.T) {
	dsn := os.Getenv("RSS_AI_TEST_DSN")
	if dsn == "" {
		t.Skip("RSS_AI_TEST_DSN not set; skipping DB integration test")
	}

	st, err := store.New(dsn)
	require.NoError(t, err)
	require.NoError(t, st.Ping(context.Background()))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/store/ -v`
Expected: FAIL (package undefined). With `RSS_AI_TEST_DSN` unset it will not yet
even compile — that is the failure we want.

- [ ] **Step 3: Create `pkg/store/store.go`**

```go
package store

import (
	"context"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// ItemTitle is the cached AI-rewritten title for one feed item, keyed by the
// item's id (guid / atom id / link) within a handler prefix.
type ItemTitle struct {
	ID             string `gorm:"primaryKey;column:id"`
	Handler        string `gorm:"primaryKey;column:handler"`
	SourceTitle    string `gorm:"column:source_title"`
	RewrittenTitle string `gorm:"column:rewritten_title"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (ItemTitle) TableName() string { return "item_titles" }

type Store struct {
	db *gorm.DB
}

// New opens the PostgreSQL connection and runs AutoMigrate for the cache table.
func New(dsn string) (*Store, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := db.AutoMigrate(&ItemTitle{}); err != nil {
		return nil, fmt.Errorf("automigrate: %w", err)
	}
	return &Store{db: db}, nil
}

// Ping verifies the underlying connection is alive (used by /readyz).
func (s *Store) Ping(ctx context.Context) error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}
```

- [ ] **Step 4: Run test to verify it passes (or skips)**

Run: `go test ./pkg/store/ -v`
Expected: PASS — `SKIP` when `RSS_AI_TEST_DSN` is unset; full PASS when set to a
reachable Postgres DSN.

- [ ] **Step 5: Commit**

```bash
git add pkg/store
git commit -m "feat(store): gorm connection, item_titles model, automigrate"
```

---

## Task 6: HTTP server (health + middleware)

**Files:**
- Create: `pkg/server/server.go`, `pkg/server/middleware.go`, `pkg/server/server_test.go`, `pkg/server/middleware_internal_test.go`

- [ ] **Step 1: Write the failing test** — `pkg/server/server_test.go`

```go
package server_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eli-yip/rss-ai/pkg/mlog"
	"github.com/eli-yip/rss-ai/pkg/server"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func TestHealthz(t *testing.T) {
	srv := server.New(":0", fakePinger{}, mlog.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.Echo().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestReadyzOK(t *testing.T) {
	srv := server.New(":0", fakePinger{err: nil}, mlog.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	srv.Echo().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestReadyzDown(t *testing.T) {
	srv := server.New(":0", fakePinger{err: errors.New("db down")}, mlog.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	srv.Echo().ServeHTTP(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/server/ -run 'TestHealthz|TestReadyz' -v`
Expected: FAIL (package undefined).

- [ ] **Step 3: Create `pkg/server/middleware.go`**

```go
package server

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/labstack/echo/v5"
	"go.uber.org/zap"

	"github.com/eli-yip/rss-ai/pkg/mlog"
)

// traceIDMiddleware assigns a random trace id to each request's context so all
// downstream logs (including detached background work) can be correlated.
func traceIDMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			ctx := context.WithValue(c.Request().Context(), mlog.TraceIDCtxKey, rand.Uint64())
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}

// requestLoggerMiddleware emits one structured access line per request.
func requestLoggerMiddleware(logger *mlog.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			start := time.Now()
			err := next(c)

			l := mlog.L(c.Request().Context(), logger)
			resp, _ := echo.UnwrapResponse(c.Response())
			fields := []zap.Field{
				zap.String("method", c.Request().Method),
				zap.String("uri", c.Request().RequestURI),
				zap.Int("status", resp.Status),
				zap.Duration("latency", time.Since(start)),
			}
			if err != nil {
				fields = append(fields, zap.Error(err))
			}
			if resp.Status >= 500 {
				l.Error("request", fields...)
			} else {
				l.Info("request", fields...)
			}
			return err
		}
	}
}
```

- [ ] **Step 4: Create `pkg/server/server.go`**

```go
package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"go.uber.org/zap"

	"github.com/eli-yip/rss-ai/pkg/httputil"
	"github.com/eli-yip/rss-ai/pkg/mlog"
)

// Pinger reports whether a downstream dependency (the DB) is reachable.
type Pinger interface {
	Ping(ctx context.Context) error
}

type Server struct {
	echo       *echo.Echo
	addr       string
	pinger     Pinger
	httpServer *http.Server
	logger     *mlog.Logger
}

func New(addr string, pinger Pinger, logger *mlog.Logger) *Server {
	svcLogger := mlog.S(logger, "server")

	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = httputil.NewEchoHTTPErrorHandler(svcLogger).DefaultEchoHTTPErrorHandler
	e.Use(
		traceIDMiddleware(),
		requestLoggerMiddleware(svcLogger),
		middleware.Recover(),
	)

	s := &Server{echo: e, addr: addr, pinger: pinger, logger: svcLogger}
	e.GET("/healthz", s.handleHealthz)
	e.GET("/readyz", s.handleReadyz)
	return s
}

// Echo exposes the router for in-process testing.
func (s *Server) Echo() *echo.Echo { return s.echo }

func (s *Server) handleHealthz(c *echo.Context) error {
	return c.JSON(http.StatusOK, httputil.NewMessage("ok"))
}

func (s *Server) handleReadyz(c *echo.Context) error {
	if err := s.pinger.Ping(c.Request().Context()); err != nil {
		return httputil.NewHTTPError(http.StatusServiceUnavailable, "database not ready")
	}
	return c.JSON(http.StatusOK, httputil.NewMessage("ready"))
}

// Start binds the listener and serves in a background goroutine.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.addr, err)
	}
	s.httpServer = &http.Server{Handler: s.echo}

	go func() {
		if err := s.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("http serve error", zap.Error(err))
		}
	}()

	s.logger.Info("http server started", zap.String("addr", s.addr))
	return nil
}

// Stop gracefully shuts the server down.
func (s *Server) Stop(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}
```

- [ ] **Step 5: Write the trace-id internal test** — `pkg/server/middleware_internal_test.go`

```go
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"

	"github.com/eli-yip/rss-ai/pkg/ctxutil"
	"github.com/eli-yip/rss-ai/pkg/mlog"
)

func TestTraceIDMiddlewareSetsContext(t *testing.T) {
	e := echo.New()
	var seen bool
	h := traceIDMiddleware()(func(c *echo.Context) error {
		_, seen = ctxutil.ValueFrom(c.Request().Context(), mlog.TraceIDCtxKey)
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	require.NoError(t, h(c))
	require.True(t, seen, "trace id should be present in request context")
}
```

This test needs a `ValueFrom` helper. Add it to `pkg/ctxutil/ctxutil.go`:

```go
// ValueFrom extracts a typed value from a context. Returns the value and true
// if the key exists and the type matches, else the zero value and false.
func ValueFrom[T any](ctx context.Context, key any) (T, bool) {
	v, ok := ctx.Value(key).(T)
	return v, ok
}
```

- [ ] **Step 6: Run all server tests to verify they pass**

Run: `go test ./pkg/server/ ./pkg/ctxutil/ -v`
Expected: PASS. If `e.NewContext` has a different v5 signature, adjust the test
to match and note it in the lessons file.

- [ ] **Step 7: Commit**

```bash
git add pkg/server pkg/ctxutil
git commit -m "feat(server): echo bootstrap with health checks and trace_id"
```

---

## Task 7: CLI entry + wiring

**Files:**
- Create: `cmd/rss-ai/main.go`

- [ ] **Step 1: Create `cmd/rss-ai/main.go`**

```go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/urfave/cli/v3"
	"go.uber.org/zap"

	"github.com/eli-yip/rss-ai/pkg/config"
	"github.com/eli-yip/rss-ai/pkg/mlog"
	"github.com/eli-yip/rss-ai/pkg/server"
	"github.com/eli-yip/rss-ai/pkg/store"
)

func main() {
	app := &cli.Command{
		Name:  "rss-ai",
		Usage: "AI title-rewriting gateway in front of RSSHub",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Value:   "config.toml",
				Usage:   "path to config file",
			},
		},
		Action: run,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, c *cli.Command) error {
	if err := config.Init(c.String("config")); err != nil {
		return fmt.Errorf("init config: %w", err)
	}
	cfg := config.C

	level, err := mlog.LogLevelFromString(cfg.Log.Level)
	if err != nil {
		return fmt.Errorf("parse log level: %w", err)
	}
	logger := mlog.New(cfg.App.Env, level)
	logger.Info("starting rss-ai", zap.String("addr", cfg.Server.Addr))

	st, err := store.New(cfg.DB.DSN)
	if err != nil {
		return fmt.Errorf("init store: %w", err)
	}

	srv := server.New(cfg.Server.Addr, st, logger)
	if err := srv.Start(); err != nil {
		return fmt.Errorf("start server: %w", err)
	}

	<-ctx.Done()
	logger.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Stop(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", zap.Error(err))
	}
	logger.Info("bye")
	return nil
}
```

- [ ] **Step 2: Build the binary**

Run: `go build ./...`
Expected: builds with no errors.

- [ ] **Step 3: Verify it runs end-to-end (manual)**

Create a local `config.toml` from `config.example.toml` with a **reachable**
PostgreSQL DSN (RSSHub/AI values can stay as placeholders for plan-0), then:

```bash
go run ./cmd/rss-ai -c config.toml &
sleep 2
curl -fsS localhost:8080/healthz
curl -fsS localhost:8080/readyz
kill %1
```

Expected: `/healthz` → `{"message":"ok"}`; `/readyz` → `{"message":"ready"}`
(200) when the DB is reachable; structured logs print to stdout (colored console
in dev). On Ctrl+C the process logs `bye` and exits cleanly.

> If you have no reachable Postgres handy, `store.New` will fail at startup — that
> is expected and itself confirms wiring. Note the outcome in the lessons file.

- [ ] **Step 4: Commit**

```bash
git add cmd/rss-ai
git commit -m "feat(cmd): rss-ai entrypoint wiring config, logger, store, server"
```

---

## Task 8: Lint pass + progress update

**Files:**
- Modify: `docs/PROGRESS.md`

- [ ] **Step 1: Run the full lint**

Run: `just lint`
Expected: clean (or fix what it reports). If `autocorrect`/`dprint`/`golangci-lint`
are not installed, install them or run their underlying checks (`go vet ./...`,
`go test ./...`) and note the gap in the lessons file.

- [ ] **Step 2: Run the test suite**

Run: `go test ./...`
Expected: PASS (store test SKIPs without `RSS_AI_TEST_DSN`).

- [ ] **Step 3: Update `docs/PROGRESS.md`**

Mark plan-0 done and point at the next plan. Replace the "Next" section:

```markdown
## Done

- [x] Design spec written and approved — `docs/spec/2026-06-09-rss-ai-gateway-design.md`
- [x] Repository scaffolding: docs layout, `AGENTS.md`, git init
- [x] plan-0 dev environment — runnable skeleton (config, mlog, store, server, /healthz, /readyz)

## Next

- [ ] plan-1: reverse-proxy passthrough + prefix/handler resolution
```

- [ ] **Step 4: Consolidate the lessons file**

Per AGENTS.md, do the single consolidation pass on
`docs/lessons/2026-06-09-dev-environment-lessons.md`: dedupe, group, rewrite the
append-only notes into a clean organized form.

- [ ] **Step 5: Commit**

```bash
git add docs/PROGRESS.md docs/lessons/2026-06-09-dev-environment-lessons.md
git commit -m "docs: mark plan-0 complete and consolidate lessons"
```

---

## Self-Review Notes

- **Spec coverage (plan-0 scope):** config (§9, TOML-only) ✓; logging to stdout with
  env-based encoding and `trace_id` (§11.2–11.3) ✓; Loki base fields `service`/`env`
  baked in (§11.1) ✓; GORM + AutoMigrate of `item_titles` (§6, §12) ✓; `/healthz`,
  `/readyz` (§11.7) ✓; `cmd/rss-ai` + urfave/cli (§12) ✓; tooling mirrors maestro,
  CI excluded (§12) ✓. **Deferred to later plans (correctly out of scope here):**
  reverse-proxy passthrough, prefix/handler resolution, etree rewrite, singleflight,
  rate limiter, AI client, request.done aggregate event.
- **Placeholder scan:** none — every code/command step is concrete.
- **Type consistency:** `mlog.New`, `mlog.L/S`, `mlog.NewNop`, `config.C`,
  `store.New/Ping`, `server.New/Start/Stop/Echo`, `Pinger`, `httputil.NewHTTPError`/
  `NewMessage`/`NewResp` are used identically wherever referenced.
- **Known risk:** Echo v5 specifics (`*echo.Context`, `echo.UnwrapResponse`,
  `e.NewContext`) and `errors.AsType[T]` are taken verbatim from maestro-engine on
  Go 1.26; if a signature differs at build time, fix to match and record in lessons.
