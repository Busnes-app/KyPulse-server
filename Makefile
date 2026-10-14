.PHONY: all build build-web test test-race test-postgres test-web tidy-check lint smoke ci run clean docker-build

all: build-web build

build-web:
	@echo "==> Building frontend web assets..."
	@cd web && npm ci && npm run build

build:
	@echo "==> Compiling kypulse binaries..."
	@go build -o kypulse ./cmd/server
	@go build -o kypulse-send ./cmd/kypulse-send

test:
	@echo "==> Running test suite..."
	@go test -v ./...

test-race:
	@echo "==> Running test suite with race detector..."
	@go test -race -covermode=atomic -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | tail -1

# Runs the same suite against PostgreSQL; needs a reachable server. The default DSN matches
# CI's throwaway one, e.g. docker run --rm -d -p 127.0.0.1:5432:5432 -e POSTGRES_USER=ci
# -e POSTGRES_PASSWORD=ci -e POSTGRES_DB=ci_test postgres:17-alpine
test-postgres:
	@echo "==> Running test suite against PostgreSQL..."
	@KYPULSE_TEST_POSTGRES_DSN="$${KYPULSE_TEST_POSTGRES_DSN:-postgres://ci:ci@127.0.0.1:5432/ci_test?sslmode=disable}" go test -count=1 ./...

test-web:
	@echo "==> Running frontend test suite..."
	@cd web && npm ci && npm test

# The same gate CI runs: a stale go.sum fails the build there, so fail here first.
tidy-check:
	@echo "==> Checking go.mod is tidy..."
	@go mod tidy
	@git diff --exit-code go.mod go.sum

lint:
	@echo "==> Checking formatting and vet..."
	@test -z "$$(gofmt -l $$(git ls-files '*.go'))" || { echo "gofmt needed:"; gofmt -l $$(git ls-files '*.go'); exit 1; }
	@go vet ./...

smoke: build
	@./scripts/smoke-test.sh

ci: tidy-check lint test-race test-web smoke
	@echo "==> Local CI checks passed"

run: build
	@./kypulse

docker-build:
	@docker build -t kypulse .

clean:
	@rm -rf kypulse kypulse-send web/dist web/node_modules data/ backups/ coverage.out
