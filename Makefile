include $(wildcard .env)
export

.PHONY: dev-db dev-api dev-web web build test test-integration lint up down import-peaks

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

lint:
	@unformatted=$$(gofmt -l cmd internal web); if [ -n "$$unformatted" ]; then echo "Run gofmt on:"; echo "$$unformatted"; exit 1; fi
	go vet ./...
	go vet -tags integration ./...
	cd web && pnpm lint

up: ## Build and run app + db
	docker compose up --build -d

down:
	docker compose down

import-peaks: ## Load OSM peaks in a region: make import-peaks BBOX=minLon,minLat,maxLon,maxLat
	go run ./cmd/kipitrek import-peaks -bbox $(BBOX)
