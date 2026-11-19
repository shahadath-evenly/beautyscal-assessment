# Frontend commands. Run `make help` for the list.

SHELL := /bin/bash
COMPOSE := docker compose

.DEFAULT_GOAL := help

## help: list the available targets
.PHONY: help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'

# --- Docker ---------------------------------------------------------------

## up: build the client and serve it with nginx on http://localhost:5173
.PHONY: up
up:
	$(COMPOSE) up --build

## up-d: same as up, in the background
.PHONY: up-d
up-d:
	$(COMPOSE) up --build -d

## down: stop the container
.PHONY: down
down:
	$(COMPOSE) down

## logs: follow the nginx logs
.PHONY: logs
logs:
	$(COMPOSE) logs -f

## ps: show container status and health
.PHONY: ps
ps:
	$(COMPOSE) ps

# --- Host ------------------------------------------------------------------

## install: install dependencies from the lockfile
.PHONY: install
install:
	npm ci

## dev: run the Vite dev server on http://localhost:5173
.PHONY: dev
dev:
	npm run dev

## build: typecheck and produce a production build in dist/
.PHONY: build
build:
	npm run build

## preview: serve the production build locally
.PHONY: preview
preview:
	npm run preview

# --- Checks ----------------------------------------------------------------

## check: typecheck, lint and production build
.PHONY: check
check:
	npm run typecheck
	npm run lint
	npm run build
