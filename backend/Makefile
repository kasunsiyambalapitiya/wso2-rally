.PHONY: run build test test-integration tidy vet fmt lint docker-db docker-db-stop migrate-up

# Local MySQL DSN used by the DB-backed tests. Override on the command line:
#   make test TEST_DB_DSN='...'
TEST_DB_DSN ?= rally:rally@tcp(localhost:3306)/rally?parseTime=true&multiStatements=true

# Compose command. Rancher Desktop in containerd mode has no dockerd, so the
# docker CLI is present but cannot connect; nerdctl speaks compose to
# containerd instead. The probe is `docker info`, which needs a live daemon —
# `docker compose version` answers from the CLI alone and would always pass.
COMPOSE ?= $(shell docker info >/dev/null 2>&1 && echo "docker compose" \
	|| (command -v nerdctl >/dev/null 2>&1 && echo "nerdctl compose") \
	|| echo "docker compose")

run:
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

# Unit tests. DB-backed tests skip themselves unless TEST_DB_DSN is exported.
test:
	go test ./...

# Every test including the DB-backed ones. Requires `make docker-db` first.
#
# -count=1 defeats the test cache: a cached "ok" from a run without a database
# would report success for tests that actually skipped.
test-integration:
	TEST_DB_DSN='$(TEST_DB_DSN)' go test -tags=integration -count=1 ./...

tidy:
	go mod tidy

vet:
	go vet ./...

fmt:
	gofmt -l -w .

lint: fmt vet

docker-db:
	$(COMPOSE) up -d db

docker-db-stop:
	$(COMPOSE) down

migrate-up:
	go run ./cmd/server -migrate
