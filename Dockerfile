# file: Dockerfile
# version: 1.2.1
# guid: audiobook-organizer-dockerfile-production

# Multi-stage production Dockerfile for audiobook-organizer
# Builds both Go backend and React frontend, serving both from a single container

# Stage 1: Build Go application
FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS go-builder

ARG TARGETOS
ARG TARGETARCH
ARG BUILDPLATFORM

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Copy dependency files first for better caching
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build the application
# CGO_ENABLED=0 for static binary
# -ldflags for smaller binary and version info
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
    -ldflags="-s -w -X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo 'dev')" \
    -o audiobook-organizer \
    .

# Stage 2: Build frontend
FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend-builder

WORKDIR /build/web

# Copy package files for better caching
COPY web/package*.json ./
RUN npm ci --prefer-offline --no-audit

# Copy frontend source
COPY web/ ./

# Build frontend for production
RUN npm run build

# Stage 3: Final production image
FROM alpine:3.20

# Install runtime dependencies (disable maintainer scripts to avoid QEMU trigger issues)
RUN apk add --no-cache --no-scripts \
    ca-certificates \
    tzdata \
    && update-ca-certificates || true \
    && addgroup -g 1000 audiobook \
    && adduser -D -u 1000 -G audiobook audiobook

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=go-builder --chown=audiobook:audiobook /build/audiobook-organizer /app/

# Copy frontend dist from builder
COPY --from=frontend-builder --chown=audiobook:audiobook /build/web/dist /app/web/dist

# Switch to non-root user
USER audiobook

# Expose the application port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the application
ENTRYPOINT ["/app/audiobook-organizer"]
CMD ["--help"]
