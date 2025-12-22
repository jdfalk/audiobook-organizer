<!-- file: web/README.md -->
<!-- version: 1.0.0 -->
<!-- guid: 0b1c2d3e-4f5a-6b7c-8d9e-0f1a2b3c4d5e -->

# Audiobook Organizer - Web Frontend

React-based web frontend for the Audiobook Organizer application.

## Technology Stack

- **React 18.2** - UI framework
- **TypeScript 5.3** - Type safety
- **Material-UI v5** - Component library
- **React Router v6** - Routing
- **Zustand** - State management
- **Vite** - Build tool and dev server
- **Vitest** - Testing framework
- **React Testing Library** - Component testing

## Development Setup

### Prerequisites

- Node.js 18+ and npm

### Install Dependencies

```bash
cd web/
npm install
```

### Development Server

```bash
npm run dev
```

The dev server runs on `http://localhost:5173` with API proxy to
`http://localhost:8080`.

### Build for Production

```bash
npm run build
```

Output goes to `dist/` directory.

### Run Tests

```bash
# Run tests once
npm test

# Run tests in watch mode
npm test -- --watch

# Run tests with UI
npm run test:ui
```

### Linting and Formatting

```bash
# Run ESLint
npm run lint

# Format code with Prettier
npm run format
```

## Project Structure

```
web/
├── src/
│   ├── components/       # Reusable components
│   │   ├── layout/       # Layout components (Sidebar, TopBar, etc.)
│   │   ├── ErrorBoundary.tsx
│   │   └── LoadingSpinner.tsx
│   ├── pages/            # Page components
│   │   ├── Dashboard.tsx
│   │   ├── Library.tsx
│   │   ├── Works.tsx
│   │   ├── FileManager.tsx
│   │   └── Settings.tsx
│   ├── lib/              # Utility libraries
│   │   └── api.ts        # API client
│   ├── stores/           # Zustand state stores
│   │   └── useAppStore.ts
│   ├── types/            # TypeScript type definitions
│   │   └── index.ts
│   ├── test/             # Test utilities
│   │   └── setup.ts
│   ├── App.tsx           # Root component
│   ├── main.tsx          # Entry point
│   └── theme.ts          # MUI theme configuration
├── index.html            # HTML entry point
├── package.json          # Dependencies and scripts
├── tsconfig.json         # TypeScript configuration
├── vite.config.ts        # Vite configuration
└── README.md             # This file
```

## API Integration

The frontend connects to the backend API running on port 8080. In development,
Vite proxies `/api` requests to `http://localhost:8080`.

Environment variables:

- `VITE_API_URL` - Override default API base URL (default: `/api/v1`)

## State Management

Uses Zustand for lightweight state management:

- `useAppStore` - Global app state (loading, notifications, errors)
- More stores will be added for entity-specific state

## Testing

- Component tests with React Testing Library
- Vitest for test runner
- Coverage reporting available via `npm test -- --coverage`

## Current Status

Phase 3 (React Frontend Foundation) is complete:

- ✅ Project structure and configuration
- ✅ Layout and navigation components
- ✅ Routing with React Router v6
- ✅ API client with error handling
- ✅ State management with Zustand
- ✅ Error boundary for global error handling
- ✅ Testing setup with Vitest
- ✅ ESLint and Prettier configuration

## Next Steps

Phase 4 will implement the Library Browser:

- Audiobook grid/list views
- Search and filtering
- Metadata editing
- Virtual scrolling for performance
