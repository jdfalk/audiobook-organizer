# file: Dockerfile
# version: 2.1.0
# guid: audiobook-organizer-dockerfile-production

# Multi-stage production Dockerfile for audiobook-organizer
# Builds React frontend, embeds it into a statically-linked Go binary with
# CGO enabled (for SQLite FTS5 support), produces a minimal container.

# Stage 1: Build frontend
FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend-builder

WORKDIR /build/web

COPY web/package*.json ./
RUN npm ci --prefer-offline --no-audit

COPY web/ ./
RUN npm run build

# Stage 2: Build Go application with embedded frontend
# Uses native platform (no cross-compile) so CGO works without cross-toolchain.
FROM golang:1.25-alpine AS go-builder

WORKDIR /build

RUN apk add --no-cache git gcc musl-dev sqlite-dev ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .

# Copy built frontend into web/dist so go:embed picks it up
COPY --from=frontend-builder /build/web/dist ./web/dist

# Accept version from build arg (since .git is excluded via .dockerignore)
ARG APP_VERSION=dev

# Build statically-linked binary with CGO (for FTS5) and embedded frontend
RUN CGO_ENABLED=1 go build \
    -tags "embed_frontend fts5" \
    -ldflags="-s -w -linkmode external -extldflags '-static' -X main.version=${APP_VERSION}" \
    -o audiobook-organizer \
    .

# Stage 3: Minimal runtime image (scratch-compatible since binary is static)
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -g 1000 audiobook \
    && adduser -D -u 1000 -G audiobook audiobook

WORKDIR /app

COPY --from=go-builder --chown=audiobook:audiobook /build/audiobook-organizer /app/

# Default data directory
RUN mkdir -p /data && chown audiobook:audiobook /data
VOLUME /data

USER audiobook

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/audiobook-organizer"]
CMD ["serve", "--host", "0.0.0.0", "--db", "/data/audiobooks.pebble"]
