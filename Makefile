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

# --- Migrations (Atlas) ----------------------------------------------------
# Every target loads .env, so DATABASE_URL comes from one place.

## migrate-apply: apply pending migrations to DATABASE_URL
.PHONY: migrate-apply
migrate-apply:
	@$(LOAD_ENV); atlas migrate apply --env local

## migrate-status: show applied and pending migrations
.PHONY: migrate-status
migrate-status:
	@$(LOAD_ENV); atlas migrate status --env local

## migrate-new: create an empty migration file, e.g. make migrate-new name=add_x
.PHONY: migrate-new
migrate-new:
	@test -n "$(name)" || { echo "usage: make migrate-new name=<description>"; exit 1; }
	@$(LOAD_ENV); atlas migrate new $(name) --dir "file://db/migrations"
	@echo "Write the SQL, then run: make migrate-hash"

## migrate-hash: recompute atlas.sum after editing a migration file
.PHONY: migrate-hash
migrate-hash:
	@$(LOAD_ENV); atlas migrate hash --dir "file://db/migrations"

## migrate-validate: check the migration directory against atlas.sum
.PHONY: migrate-validate
migrate-validate:
	@$(LOAD_ENV); atlas migrate validate --env local

## migrate-fresh: DROP every table in DATABASE_URL and re-apply from scratch
.PHONY: migrate-fresh
migrate-fresh:
	@$(LOAD_ENV); psql "$$DATABASE_URL" -v ON_ERROR_STOP=1 -q -c 'DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public;'
	@$(LOAD_ENV); atlas migrate apply --env local

## verify-schema: assert the tenancy, stock and uniqueness invariants hold
.PHONY: verify-schema
verify-schema:
	@$(LOAD_ENV); psql "$$DATABASE_URL" -v ON_ERROR_STOP=1 -q -f db/verify/invariants.sql

# --- Code generation (sqlc) ------------------------------------------------

## sqlc: regenerate the database layer from db/queries
.PHONY: sqlc
sqlc:
	sqlc generate

## sqlc-check: fail if the generated code is out of date with the queries
.PHONY: sqlc-check
sqlc-check:
	sqlc diff

# --- Checks ----------------------------------------------------------------

## check: gofmt, vet, build and tests
.PHONY: check
check: sqlc-check
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }
	go vet ./...
	go build ./...
	go test ./...

## fmt: format all Go files
.PHONY: fmt
fmt:
	gofmt -w .
