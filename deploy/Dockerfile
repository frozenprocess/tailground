# syntax=docker/dockerfile:1

# Tailground is a static SPA plus a tiny short-link API. This image builds it in
# four stages — Go/WASM engine, Vite bundle, API binary, then an nginx runtime —
# serving web/dist and carrying the API binary for the sidecar container.
#
#   docker build -f deploy/Dockerfile -t ghcr.io/frozenprocess/tailground:latest .
#
# The Headscale module is not fetchable through the Go proxy, so the engine
# stage checks it out at the pinned SHA (kept in sync with
# .github/workflows/pages.yml) and points the `replace` directive at it.

# ---------- 1. engine: Go -> js/wasm ----------
FROM golang:1.27-bookworm AS engine

ARG HEADSCALE_REF=63123196cc21eeb301eec60bf05d5fe0a2bc4787

# ../headscale relative to the repo checkout at /src (matches go.mod's replace).
RUN git init -q /headscale \
 && git -C /headscale remote add origin https://github.com/juanfont/headscale.git \
 && git -C /headscale fetch -q --depth 1 origin "${HEADSCALE_REF}" \
 && git -C /headscale checkout -q FETCH_HEAD

WORKDIR /src
COPY . .

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    make vendor && make wasm

# ---------- 2. web: Vite build ----------
FROM node:20-bookworm AS web

WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci

COPY web/ ./
# Overlay the freshly built engine artifacts (gitignored in the repo).
COPY --from=engine /src/web/public/engine.wasm ./public/engine.wasm
COPY --from=engine /src/web/public/wasm_exec.js ./public/wasm_exec.js

# Served at the domain root, so the default base ("/") is correct.
RUN npm run build

# ---------- 3. api: short-share-link backend (separate module, pure Go) ----------
FROM golang:1.27-bookworm AS api

WORKDIR /src/server
COPY server/ ./
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/tailground-api .

# ---------- 4. runtime: nginx (non-root, listens on 8080) ----------
FROM nginxinc/nginx-unprivileged:1.27-alpine AS runtime

COPY deploy/nginx.conf /etc/nginx/conf.d/default.conf
COPY deploy/security-headers.conf /etc/nginx/security-headers.conf
COPY --from=web /src/web/dist /usr/share/nginx/html
# Run as a second container in the same pod (see deploy/k8s/deployment.yaml);
# nginx proxies /api/ to it on 127.0.0.1:8081.
COPY --from=api /out/tailground-api /usr/local/bin/tailground-api

EXPOSE 8080
