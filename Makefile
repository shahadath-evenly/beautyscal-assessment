# Backend commands. Run `make help` for the list.
# Host targets load ./.env, so the same file configures Docker and host runs.

SHELL := /bin/bash
COMPOSE := docker compose
LOAD_ENV := set -a; [[ -f .env ]] && . ./.env; set +a

.DEFAULT_GOAL := help

## help: list the available targets
.PHONY: help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'

# --- Docker ---------------------------------------------------------------

## up: build and start PostgreSQL and the API in Docker
.PHONY: up
up:
	$(COMPOSE) up --build

## up-d: same as up, in the background
.PHONY: up-d
up-d:
	$(COMPOSE) up --build -d

## db: start only PostgreSQL, for running the API on the host
.PHONY: db
db:
	$(COMPOSE) up -d postgres

## down: stop the stack, keeping the database volume
.PHONY: down
down:
	$(COMPOSE) down

## down-volumes: stop the stack and DELETE the database volume
.PHONY: down-volumes
down-volumes:
	$(COMPOSE) down --volumes

## logs: follow the API and database logs
.PHONY: logs
logs:
	$(COMPOSE) logs -f

## ps: show service status and health
.PHONY: ps
ps:
	$(COMPOSE) ps

## seed-docker: run the seed command inside the Docker stack
.PHONY: seed-docker
seed-docker:
	$(COMPOSE) run --rm api seed

## psql: open a psql shell on the containerised database
.PHONY: psql
psql:
	$(COMPOSE) exec postgres sh -c 'psql -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"'

# --- Host ------------------------------------------------------------------

## api: run the API on the host, using DATABASE_URL from .env
.PHONY: api
api:
	@$(LOAD_ENV); go run ./cmd/api

## seed: run the seed command on the host, using DATABASE_URL from .env
.PHONY: seed
seed:
	@$(LOAD_ENV); go run ./cmd/seed

# --- Checks ----------------------------------------------------------------

## check: gofmt, vet, build and tests
.PHONY: check
check:
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }
	go vet ./...
	go build ./...
	go test ./...

## fmt: format all Go files
.PHONY: fmt
fmt:
	gofmt -w .
