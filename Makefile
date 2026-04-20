APP_NAME := shelfy
SQLC_VERSION := v1.29.0

.PHONY: setup prepare generate dev api worker-pipeline worker-scheduler migrate test lint fmt tidy up down logs logs-all logs-db

prepare:
	@test -f .env || (echo ".env is required; copy .env.example to .env and fill secrets" && exit 1)
	@mkdir -p tmp models

setup:
	go mod tidy

generate:
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) generate

tidy:
	go mod tidy

fmt:
	gofmt -w ./cmd ./internal

api:
	go run ./cmd/telegram-api

worker-pipeline:
	go run ./cmd/pipeline-worker

worker-scheduler:
	go run ./cmd/scheduler-worker

migrate:
	go run ./cmd/migrate

test:
	go test ./...

lint:
	go test ./...

dev: prepare
	docker compose up --build -d

up:
	docker compose up --build

down:
	docker compose down

logs:
	docker compose logs --tail=200 -f telegram-api pipeline-worker scheduler-worker

logs-all:
	docker compose logs --tail=200 -f

logs-db:
	docker compose logs --tail=200 -f postgres
