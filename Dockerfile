# syntax=docker/dockerfile:1

# ---------- build stage ----------
# Pinned to the Go version in go.mod so the image never silently drifts.
FROM golang:1.26-alpine AS builder

WORKDIR /src

# Dependencies are copied first so the module layer is cached independently of
# the source, keeping rebuilds fast.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Static binaries: no cgo, so the runtime image needs no libc.
# -trimpath keeps build paths out of the binary; -s -w drop the symbol table.
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
 && go build -trimpath -ldflags="-s -w" -o /out/seed ./cmd/seed

# ---------- runtime stage ----------
# Alpine rather than scratch/distroless: it supplies the shell and wget the
# container health check needs, at a few megabytes.
FROM alpine:3.22 AS runtime

RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -H -u 10001 app

# Both commands ship in the image. `api` is the default command; `seed` is run
# on demand with `docker compose run --rm backend seed`, which replaces the
# command. Declared as CMD rather than ENTRYPOINT precisely so that works.
COPY --from=builder /out/api /out/seed /usr/local/bin/

# Migrations and queries travel with the image for later phases.
COPY --from=builder /src/db /app/db

USER app
WORKDIR /app

# Local default only. A platform that injects PORT (Railway, Cloud Run, Heroku)
# overrides it, because the config reads PORT first and falls back to HTTP_PORT.
ENV HTTP_PORT=8081
EXPOSE 8081

# The check resolves the port exactly the way the application does, so it
# follows an injected PORT instead of probing a port nothing is listening on.
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=5 \
  CMD wget -qO- "http://127.0.0.1:${PORT:-${HTTP_PORT:-8081}}/health" >/dev/null 2>&1 || exit 1

CMD ["api"]
