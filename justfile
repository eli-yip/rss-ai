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

# Bring up the throwaway RSSHub used by the integration tests and wait for it.
[group('test')]
rsshub-up:
    docker compose -f compose.test.yaml up -d --wait

[group('test')]
rsshub-down:
    docker compose -f compose.test.yaml down -v

# Full integration flow: start RSSHub, run the gateway integration tests against
# it, then tear the stack down regardless of the test result.
[group('test')]
integration upstream="http://127.0.0.1:1200":
    just rsshub-up
    RSS_AI_TEST_UPSTREAM={{ upstream }} go test ./... ; status=$? ; just rsshub-down ; exit $status
