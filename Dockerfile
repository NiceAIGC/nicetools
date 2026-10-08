# syntax=docker/dockerfile:1
FROM node:22-bookworm-slim AS frontend
WORKDIR /build
RUN corepack enable && corepack prepare pnpm@10.11.0 --activate
COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY tsconfig.json vite.config.ts index.html ./
COPY public ./public
COPY src ./src
RUN pnpm build

FROM golang:1.26-bookworm AS backend
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
WORKDIR /build/backend
COPY backend/go.mod backend/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY backend ./
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go test ./... && CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /nicetools ./cmd/server

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 nicetools \
    && useradd --uid 10001 --gid nicetools --no-create-home nicetools \
    && mkdir -p /app/data && chown nicetools:nicetools /app/data
WORKDIR /app
COPY --from=backend /nicetools /app/nicetools
COPY --from=frontend /build/dist /app/dist
COPY src/tools/catalog.json /app/catalog.json
ENV PORT=8080 DB_PATH=/app/data/nicetools.db STATIC_DIR=/app/dist TOOL_CATALOG=/app/catalog.json GIN_MODE=release
USER nicetools
EXPOSE 8080
VOLUME ["/app/data"]
HEALTHCHECK --interval=10s --timeout=5s --start-period=15s --retries=6 CMD ["/app/nicetools", "healthcheck"]
ENTRYPOINT ["/app/nicetools"]
