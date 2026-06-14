.PHONY: help build run test docker-up docker-down lint fmt migrate

help:
	@echo "Seoul Metro API - Development Commands"
	@echo "======================================"
	@echo "make build       - Build the binary"
	@echo "make run         - Run the server"
	@echo "make test        - Run tests"
	@echo "make lint        - Run linter"
	@echo "make fmt         - Format code"
	@echo "make docker-up   - Start Docker Compose services"
	@echo "make docker-down - Stop Docker Compose services"
	@echo "make clean       - Clean build artifacts"
	@echo "make deps        - Download Go dependencies"

build:
	go build -o metro .

run:
	go run .

test:
	go test -v ./...

lint:
	golangci-lint run ./...

fmt:
	go fmt ./...

docker-up:
	docker-compose up -d
	@echo "Services started. Waiting for health checks..."
	sleep 5
	docker-compose ps

docker-down:
	docker-compose down

docker-logs:
	docker-compose logs -f

clean:
	rm -f metro
	go clean

deps:
	go mod download
	go mod tidy

db-init:
	psql -h localhost -U postgres -d seoul_metro < init-db.sql

db-reset:
	docker-compose exec postgres psql -U postgres -d seoul_metro -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
	$(MAKE) db-init

all: clean deps build run
