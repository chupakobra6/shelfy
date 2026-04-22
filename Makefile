APP_NAME := shelfy
SQLC_VERSION := v1.29.0
RUNTIME_BASE_IMAGE ?= shelfy-runtime-base:vosk-lib-0.3.45-small-ru-0.22
E2E_TRIAGE := go run ./cmd/e2e-triage

.DEFAULT_GOAL := help

.PHONY: help setup prepare runtime-base runtime-base-rebuild ensure-runtime-base generate dev api worker-pipeline worker-scheduler migrate test lint fmt fmt-check tidy up down logs logs-all logs-db e2e-last-failure e2e-trace-logs

help:
	@printf "Available commands:\n"
	@printf "  make setup            # go mod tidy\n"
	@printf "  make runtime-base     # build the shared runtime image once if it is missing\n"
	@printf "  make runtime-base-rebuild # force-rebuild the shared runtime image\n"
	@printf "  make dev              # start local docker compose stack in the background\n"
	@printf "  make down             # stop local docker compose stack\n"
	@printf "  make logs             # follow app logs (telegram-api, pipeline-worker, scheduler-worker)\n"
	@printf "  make logs-db          # follow postgres logs only\n"
	@printf "  make e2e-last-failure # build a compact failure pack from the latest tool failure\n"
	@printf "  make e2e-trace-logs   # slice recent logs by TRACE_ID/UPDATE_ID/JOB_ID/SCENARIO_LABEL\n"
	@printf "  make test             # go test ./...\n"
	@printf "  make lint             # fail on gofmt drift under cmd/ and internal/\n"
	@printf "  make generate         # regenerate sqlc code\n"
	@printf "  make api              # run telegram-api locally without docker\n"
	@printf "  make worker-pipeline  # run pipeline-worker locally without docker\n"
	@printf "  make worker-scheduler # run scheduler-worker locally without docker\n"
	@printf "  make migrate          # apply DB migrations\n"

prepare:
	@test -f .env || (echo ".env is required; copy .env.example to .env and fill secrets" && exit 1)
	@mkdir -p tmp models

setup:
	go mod tidy

runtime-base: ensure-runtime-base

runtime-base-rebuild:
	docker build -f Dockerfile.base -t $(RUNTIME_BASE_IMAGE) .

ensure-runtime-base:
	@if docker image inspect $(RUNTIME_BASE_IMAGE) >/dev/null 2>&1; then \
		echo "reusing existing runtime base image $(RUNTIME_BASE_IMAGE)"; \
	else \
		echo "runtime base image $(RUNTIME_BASE_IMAGE) is missing; building it once"; \
		docker build -f Dockerfile.base -t $(RUNTIME_BASE_IMAGE) .; \
	fi

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

fmt-check:
	@out="$$(gofmt -l ./cmd ./internal)"; \
	if [ -n "$$out" ]; then \
		echo "gofmt drift detected:"; \
		echo "$$out"; \
		exit 1; \
	fi

lint: fmt-check

dev: prepare ensure-runtime-base
	docker compose up --build -d

up: ensure-runtime-base
	docker compose up --build

down:
	docker compose down

logs:
	docker compose logs --tail=200 -f telegram-api pipeline-worker scheduler-worker

logs-all:
	docker compose logs --tail=200 -f

logs-db:
	docker compose logs --tail=200 -f postgres

e2e-last-failure:
	TOOL_ROOT="$(TOOL_ROOT)" PACK_ROOT="$(PACK_ROOT)" MAX_LINES_PER_SERVICE="$(MAX_LINES_PER_SERVICE)" $(E2E_TRIAGE) last-failure-pack

e2e-trace-logs:
	TOOL_ROOT="$(TOOL_ROOT)" MAX_LINES="$(MAX_LINES)" $(E2E_TRIAGE) trace-logs $(if $(TRACE_ID),--trace-id $(TRACE_ID),) $(if $(UPDATE_ID),--update-id $(UPDATE_ID),) $(if $(JOB_ID),--job-id $(JOB_ID),) $(if $(SCENARIO_LABEL),--scenario-label $(SCENARIO_LABEL),) $(if $(SERVICE),--service $(SERVICE),) $(if $(SINCE),--since $(SINCE),) $(if $(UNTIL),--until $(UNTIL),)
