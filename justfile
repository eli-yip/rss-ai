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
