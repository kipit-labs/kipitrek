include $(wildcard .env)
export

# The `make dev-db` database, unless .env or the environment says otherwise.
DATABASE_URL ?= postgres://gpx:gpx@localhost:5432/gpx?sslmode=disable
GOLANGCI_LINT_VERSION := v2.15.0
# Ratchet: raise it when coverage goes up, never lower it.
COVERAGE_FLOOR := 75

.PHONY: dev-db dev-api dev-web web build test test-integration cover lint up down import-peaks

dev-db: ## Start PostGIS and Mailpit (inbox at http://localhost:8025)
	docker compose up -d db mailpit

dev-api: ## Run the API locally (serves the last frontend build)
	go run ./cmd/kipitrek

dev-web: ## Vite dev server with /api proxied to :8080
	cd web && pnpm dev

web: ## Build the frontend into web/dist (embedded in the Go binary)
	cd web && pnpm install --frozen-lockfile && pnpm build

build: web ## Build the single binary
	go build -o bin/kipitrek ./cmd/kipitrek

test:
	go test ./...
	cd web && pnpm test

test-integration: ## Needs `make dev-db`
	TEST_DATABASE_URL=$(DATABASE_URL) go test -tags integration ./internal/infrastructure/postgres/

cover: ## Unit + integration tests with coverage; fails under COVERAGE_FLOOR. Needs `make dev-db`
	TEST_DATABASE_URL=$(DATABASE_URL) go test -race -tags integration -coverpkg=./internal/... -coverprofile=cover.out ./...
	@go tool cover -func=cover.out | awk -v floor=$(COVERAGE_FLOOR) '/^total:/ { sub("%", "", $$3); print "coverage " $$3 "% (floor " floor "%)"; if ($$3 + 0 < floor) exit 1 }'

lint: ## gofmt, vet, layer rules (.golangci.yml), oxlint
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run
	cd web && pnpm lint

up: ## Build and run app + db
	docker compose up --build -d

down:
	docker compose down

import-peaks: ## Load OSM peaks in a region: make import-peaks BBOX=minLon,minLat,maxLon,maxLat
	go run ./cmd/kipitrek import-peaks -bbox $(BBOX)
