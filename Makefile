# Whole-stack commands. Run `make help` for the list.
#
# Each app also has its own Makefile with the full set of targets for that
# side: `make -C backend help` and `make -C frontend help`. This file holds
# only what spans both.

SHELL := /bin/bash
COMPOSE := docker compose

.DEFAULT_GOAL := help

## help: list the available targets
.PHONY: help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'

# --- Docker ---------------------------------------------------------------

## up: build and start PostgreSQL, the API and the client
.PHONY: up
up:
	$(COMPOSE) up --build

## up-d: the same, in the background
.PHONY: up-d
up-d:
	$(COMPOSE) up -d --build

## backend-up: start only PostgreSQL and the API, so the seed can run first
.PHONY: backend-up
backend-up:
	$(COMPOSE) up -d --build postgres api

## seed: load the demo data and print the workspace IDs for .env
.PHONY: seed
seed:
	$(COMPOSE) run --rm api seed

## logs: follow the logs of every service
.PHONY: logs
logs:
	$(COMPOSE) logs -f

## ps: show the service status
.PHONY: ps
ps:
	$(COMPOSE) ps

## down: stop everything, keeping the database volume
.PHONY: down
down:
	$(COMPOSE) down

## down-volumes: stop everything and discard the database
.PHONY: down-volumes
down-volumes:
	$(COMPOSE) down -v

# --- Checks ---------------------------------------------------------------

## test: run the backend test suite
.PHONY: test
test:
	cd backend && go test ./...

## check: every check both sides have - format, vet, build, tests, typecheck, lint
.PHONY: check
check:
	$(MAKE) -C backend check
	$(MAKE) -C frontend check
