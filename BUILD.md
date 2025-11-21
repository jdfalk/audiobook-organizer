<!-- file: BUILD.md -->
<!-- version: 1.0.0 -->
<!-- guid: 3c4d5e6f-7a8b-9c0d-1e2f-3a4b5c6d7e8f -->

# Build Instructions

## Standard Build (API Only)

By default, the application builds without embedding the frontend. This results
in a smaller binary that only serves the API:

```bash
go build -o audiobook-organizer
```

The server will display an API documentation page at the root (`/`) with
available endpoints.

## Build with Embedded Frontend

To embed the React frontend into the binary, use the `embed_frontend` build tag:

```bash
# First, build the frontend
cd web
npm install
npm run build
cd ..

# Then build the Go binary with embedded frontend
go build -tags embed_frontend -o audiobook-organizer
```

With this option, the compiled binary includes the entire React frontend from
`web/dist/`, allowing the application to serve both the API and web interface
from a single executable.

## Build Tags

- **No tags (default)**: API-only server with placeholder HTML page
  - Smaller binary size
  - Faster compilation
  - API documentation page at root

- **`embed_frontend`**: Full application with embedded web interface
  - Larger binary size (includes all frontend assets)
  - Self-contained single binary
  - Full React SPA served at root
  - Requires `web/dist/` to exist before building

## Cross-Platform Builds

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o audiobook-organizer-linux

# Linux with frontend
GOOS=linux GOARCH=amd64 go build -tags embed_frontend -o audiobook-organizer-linux

# macOS (Intel)
GOOS=darwin GOARCH=amd64 go build -o audiobook-organizer-macos-intel

# macOS (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o audiobook-organizer-macos-arm

# Windows
GOOS=windows GOARCH=amd64 go build -o audiobook-organizer.exe
```

## Implementation Details

The build system uses Go's build tags to conditionally compile different
versions:

- `internal/server/static_nonembed.go` - Compiles by default (no tags)
  - Serves placeholder HTML with API documentation
  - No embedded filesystem

- `internal/server/static_embed.go` - Compiles with `-tags embed_frontend`
  - Uses `//go:embed web/dist` to embed frontend
  - Serves React SPA from embedded filesystem
  - Handles SPA routing (all non-API routes serve index.html)

Both files implement the same `setupStaticFiles()` method, allowing the main
server code to remain unchanged regardless of build configuration.

## Development Workflow

### API Development (without frontend)

```bash
go run . serve
# or
go build && ./audiobook-organizer serve
```

### Full Stack Development

```bash
# Terminal 1: Run frontend dev server with hot reload
cd web
npm run dev

# Terminal 2: Run Go backend
go run . serve

# Frontend proxies API requests to backend
```

### Production Build

```bash
# Build frontend for production
cd web
npm run build
cd ..

# Build backend with embedded frontend
go build -tags embed_frontend -o audiobook-organizer

# Single binary includes everything
./audiobook-organizer serve
```

## Binary Size Comparison

Approximate sizes (may vary by platform):

- **API-only build**: ~15-20 MB
- **With embedded frontend**: ~20-30 MB (adds frontend assets)

The embedded frontend adds the following to the binary:

- React application bundle (~2-5 MB compressed)
- Static assets (images, fonts, etc.)
- index.html and other static files
