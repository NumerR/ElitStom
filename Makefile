APP     := dentacore-api
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

# Matches docker-compose.yml. Override for CI: `make test-integration DSN=postgres://...`.
DSN ?= postgres://dentacore:dentacore@localhost:5432/dentacore?sslmode=disable

.PHONY: run seed build test test-integration cover vet vet-integration fmt tidy db-up db-down migrate web-install web-dev web-build web-lint

run:
	go run ./cmd/api

# Demo accounts for local work; refuses to run when APP_ENV=production.
seed:
	go run ./cmd/seed

web-install:
	cd web && npm ci

# Vite proxies /api to the API on :8080, so run `make run` alongside.
web-dev:
	cd web && npm run dev

web-build:
	cd web && npm run build

web-lint:
	cd web && npm run lint

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(APP) ./cmd/api

test:
	go test -race ./...

# Needs a live server: `make db-up migrate`. The suite builds its own throwaway schema, so it
# never touches the tables the application uses.
test-integration:
	DENTACORE_TEST_DATABASE_URL=$(DSN) go test -race -count=1 -tags integration ./internal/repository/postgres/

vet:
	go vet ./...

vet-integration:
	go vet -tags integration ./...

cover:
	go test -race -covermode=atomic -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -n 1

fmt:
	gofmt -l -w .

tidy:
	go mod tidy

migrate:
	@for file in migrations/*.up.sql; do \
		echo "==> $$file"; \
		docker compose exec -T postgres psql -U dentacore -d dentacore -v ON_ERROR_STOP=1 < $$file || exit 1; \
	done

db-up:
	docker compose up -d postgres

db-down:
	docker compose down
