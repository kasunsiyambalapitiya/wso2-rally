.PHONY: run build test test-integration tidy vet fmt lint docker-db docker-db-stop migrate-up

# Local MySQL DSN used by the DB-backed tests. Override on the command line:
#   make test TEST_DB_DSN='...'
TEST_DB_DSN ?= rally:rally@tcp(localhost:3306)/rally?parseTime=true&multiStatements=true

run:
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

# Unit tests. DB-backed tests skip themselves unless TEST_DB_DSN is exported.
test:
	go test ./...

# Every test including the DB-backed ones. Requires `make docker-db` first.
test-integration:
	TEST_DB_DSN='$(TEST_DB_DSN)' go test -tags=integration ./...

tidy:
	go mod tidy

vet:
	go vet ./...

fmt:
	gofmt -l -w .

lint: fmt vet

docker-db:
	docker compose up -d db

docker-db-stop:
	docker compose down

migrate-up:
	go run ./cmd/server -migrate
