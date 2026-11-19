# syntax=docker/dockerfile:1

# ---------- build stage ----------
FROM node:26-alpine AS builder

WORKDIR /app

# The lockfile is copied first so the dependency layer is cached independently
# of the source. npm ci installs exactly what the lockfile pins.
COPY package.json package-lock.json ./
RUN npm ci

COPY . .

# Vite inlines VITE_* variables at build time, so the API URL is a build
# argument rather than a runtime environment variable. It must be the URL the
# browser uses, which is a host address, not a Compose service name.
ARG VITE_API_BASE_URL=http://localhost:8081
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL

RUN npm run build

# ---------- runtime stage ----------
# Only the built static assets are served; Node is not present at runtime.
FROM nginx:1.29-alpine AS runtime

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/dist /usr/share/nginx/html

EXPOSE 80

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=5 \
  CMD wget -qO- http://127.0.0.1/ >/dev/null 2>&1 || exit 1

CMD ["nginx", "-g", "daemon off;"]
