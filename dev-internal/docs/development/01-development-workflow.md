# Development Workflow

**Purpose**: Guide developers on setting up, running, building, and contributing to Kolabpad.

**Audience**: New contributors, developers setting up local environment, maintainers.

---

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Project Structure](#project-structure)
3. [Getting Started](#getting-started)
4. [Running Locally](#running-locally)
5. [Development Workflow](#development-workflow)
6. [Building for Production](#building-for-production)
7. [Environment Variables](#environment-variables)
8. [Code Style and Quality](#code-style-and-quality)
9. [Making Changes](#making-changes)
10. [Common Tasks](#common-tasks)

---

## Prerequisites

Before starting development, ensure you have the following installed:

- **Go 1.23+** (backend development and WASM bridge)
- **Node.js 18+** (frontend development)
- **Docker** and **Docker Compose** (optional, for containerized development)
- **Make** (for build automation)
- **Git** (version control)

**Verify installations**:

```bash
go version      # Should show 1.23 or higher
node --version  # Should show 18 or higher
docker --version
make --version
```

---

## Project Structure

```
kolabpad/
├── cmd/                    # Go entry points
│   ├── server/            # Main HTTP/WebSocket server
│   └── ot-wasm-bridge/    # OT algorithm WASM bridge for browser
├── pkg/                   # Go packages (public API)
│   ├── server/           # Server implementation
│   ├── database/         # SQLite persistence layer
│   ├── ot/               # Operational Transformation algorithm
│   └── logger/           # Logging utilities
├── internal/              # Internal Go packages (private)
│   └── protocol/         # WebSocket message definitions
├── frontend/              # TypeScript + React frontend
│   ├── src/
│   │   ├── contexts/     # React context providers
│   │   ├── hooks/        # Custom React hooks
│   │   ├── api/          # REST API client
│   │   ├── services/     # WebSocket and business logic
│   │   ├── components/   # React UI components
│   │   └── types/        # TypeScript type definitions
│   ├── public/           # Static assets
│   └── dist/             # Build output (gitignored)
├── data/                  # SQLite database files (gitignored)
├── Makefile              # Build automation
├── Dockerfile            # Multi-stage production build
├── docker-compose.yml    # Docker development environment
├── .env.example          # Environment variable template
├── go.mod                # Go dependencies
└── README.md
```

---

## Getting Started

### One-Time Setup

Run the automated setup command for new developers:

```bash
make setup
```

This command will:
1. Create `.env` file from `.env.example` (if not exists)
2. Install Go dependencies (`go mod download`)
3. Install frontend dependencies (`npm ci`)
4. Build WASM bridge for browser-side OT operations

After setup completes, you'll see:

```
✅ Setup complete! Start developing:
   Terminal 1: make dev-backend
   Terminal 2: make dev-frontend
```

### Manual Setup

If you prefer manual setup:

```bash
# 1. Copy environment configuration
cp .env.example .env

# 2. Install backend dependencies
make install

# 3. Install frontend dependencies
make install.frontend

# 4. Build WASM bridge
make wasm-build
```

---

## Running Locally

Kolabpad has separate backend and frontend dev servers that run concurrently.

### Development Servers

**Option 1: Two terminals (recommended for active development)**

```bash
# Terminal 1: Backend server
make dev-backend

# Terminal 2: Frontend dev server (with hot reload)
make dev-frontend
```

- Backend runs on `http://localhost:3030` (WebSocket + REST API)
- Frontend runs on `http://localhost:5173` (Vite dev server)
- Vite proxies API requests to backend automatically

**Option 2: Docker Compose (recommended for testing production-like environment)**

```bash
# Start all services in background
make docker-up

# View logs
make docker-logs

# Stop services
make docker-down

# Restart with rebuild
make docker-restart

# Restart with debug logging
make docker-restart.debug
```

Docker mode:
- Runs on `http://localhost:3030` (single port, like production)
- Frontend served as static files by Go server
- SQLite database persisted in `./data/` volume

### Accessing the Application

Once running, open your browser:

- **Development mode**: `http://localhost:5173`
- **Docker mode**: `http://localhost:3030`

Create or access a document by adding a path:
- New random document: `http://localhost:5173` (auto-redirects)
- Specific document: `http://localhost:5173/#abc123`
- Protected document: `http://localhost:5173/#abc123?otp=xyz789`

---

## Development Workflow

### Hot Reload

**Frontend**:
- Vite dev server provides instant hot module replacement (HMR)
- Changes to `.tsx`, `.ts`, `.css` files reload automatically
- State preserved across reloads when possible

**Backend**:
- Changes require manual restart: `Ctrl+C` then `make dev-backend`
- For automatic reload: Install [Air](https://github.com/cosmtrek/air) and configure `.air.toml`

### Database

**Location**:
- Development: `./data/kolabpad.db` (SQLite file)
- Docker: `/data/kolabpad.db` (mounted volume)

**Operations**:

```bash
# Reset database (delete and restart server)
rm ./data/kolabpad.db
make dev-backend

# Inspect database (requires sqlite3)
sqlite3 ./data/kolabpad.db
sqlite> .tables
sqlite> SELECT id, language, created_at FROM documents;
sqlite> .exit
```

**Migrations**:
- Migrations run automatically on server startup
- Migration code: `pkg/database/migrations.go`
- Schema changes require new migration functions

### Logging

**Backend**:

```bash
# Set log level in .env
LOG_LEVEL=debug  # Options: debug, info, error

# Or override for single run
LOG_LEVEL=debug make dev-backend
```

**Log levels**:
- `debug`: Verbose output (all operations, message handling)
- `info`: Standard operational messages (connections, persistence, errors)
- `error`: Only error messages

**Frontend**:
- Browser console: Open DevTools (F12)
- React DevTools: Install browser extension for component inspection
- Network tab: Monitor WebSocket messages and REST API calls

---

## Building for Production

### Build Everything

```bash
# Build all components (WASM + backend + frontend)
make build.all
```

This runs:
1. `make wasm-build` - Compiles OT algorithm to WebAssembly
2. `make build` - Builds Go server binary to `bin/kolabpad-server`
3. `make build.frontend` - Builds optimized frontend to `frontend/dist/`

### Individual Builds

**Backend only**:

```bash
make build

# Output: bin/kolabpad-server
# Run: ./bin/kolabpad-server
```

**Frontend only**:

```bash
make build.frontend

# Output: frontend/dist/
# Serve: Any static file server
```

**WASM bridge only**:

```bash
make wasm-build

# Output: frontend/public/ot.wasm
#         frontend/public/wasm_exec.js
```

### Docker Image

```bash
# Build Docker image (multi-stage build)
docker build -t kolabpad:latest .

# Run container
docker run -p 3030:3030 -v ./data:/data kolabpad:latest

# With environment variables
docker run -p 3030:3030 \
  -e LOG_LEVEL=debug \
  -e EXPIRY_DAYS=14 \
  -v ./data:/data \
  kolabpad:latest
```

**Multi-stage Dockerfile stages**:
1. **WASM**: Compiles Go OT library to WebAssembly
2. **Frontend**: Builds React app with Vite (includes WASM from stage 1)
3. **Backend**: Compiles Go server with CGO for SQLite
4. **Final**: Alpine-based runtime image (only binary + static files)

---

## Environment Variables

Configuration is managed via environment variables. All variables have sensible defaults.

### Backend Variables

Defined in `.env` file:

```bash
# Server Configuration
PORT=3030                          # HTTP server port
LOG_LEVEL=info                     # Logging verbosity: debug|info|error

# Document Configuration
EXPIRY_DAYS=7                      # Days before inactive docs are deleted
SQLITE_URI=./data/kolabpad.db      # Database file path (empty = in-memory only)
CLEANUP_INTERVAL_HOURS=1           # How often to run expiry cleanup
MAX_DOCUMENT_SIZE_KB=256           # Maximum document size (prevents abuse)

# WebSocket Configuration
WS_READ_TIMEOUT_MINUTES=30         # Disconnect idle clients after N minutes
WS_WRITE_TIMEOUT_SECONDS=10        # Timeout when sending to slow clients
BROADCAST_BUFFER_SIZE=16           # Channel buffer size per connection
```

**Special cases**:

- **In-memory mode**: Omit `SQLITE_URI` or set to empty string (no persistence)
- **Docker**: `SQLITE_URI` is hardcoded to `/data/kolabpad.db` in `docker-compose.yml`

### Frontend Variables

Frontend uses Vite environment variables (prefix with `VITE_`):

```bash
# Development only (vite.config.ts handles proxy)
VITE_API_URL=http://localhost:3030
```

**Production build**:
- Frontend expects backend on same origin (no VITE_API_URL needed)
- WebSocket connects to `/api/socket/{id}`
- REST API calls go to `/api/`

---

## Code Style and Quality

### Go (Backend)

**Formatting**:

```bash
# Format all Go code
make lint

# Runs: go fmt ./...
#       go vet ./...
```

**Guidelines**:
- Use `go fmt` for consistent formatting (no tabs vs spaces debates)
- Run `go vet` to catch common mistakes
- Follow [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- Package names: lowercase, single word (e.g., `server`, `database`, not `serverUtils`)
- Exported functions: Document with comment starting with function name

**Example**:

```go
// NewServer creates a new HTTP server with the provided configuration.
// The database parameter can be nil to run in memory-only mode.
func NewServer(db *database.Database, maxSize int) *Server {
    // ...
}
```

### TypeScript (Frontend)

**Formatting and Linting**:

```bash
# Format code with Prettier
make format.frontend

# Lint with ESLint
make lint.frontend

# Fix auto-fixable lint errors
cd frontend && npm run lint:fix

# Type check
cd frontend && npm run check
```

**Guidelines**:
- Prettier enforces formatting (configured in `package.json`)
- ESLint catches code quality issues
- TypeScript strict mode enabled (no implicit `any`)
- Use functional components with hooks (no class components)
- Prefer explicit types over inference for function signatures

**Example**:

```typescript
// Good: Explicit types on parameters and return
function calculateHue(userId: number, totalUsers: number): number {
    return (userId * 360) / totalUsers;
}

// Avoid: Implicit any
function calculateHue(userId, totalUsers) {
    return (userId * 360) / totalUsers;
}
```

### Import Organization

**Go**: Use `goimports` (handles grouping automatically):

```go
import (
    // Standard library
    "context"
    "fmt"

    // External dependencies
    "nhooyr.io/websocket"

    // Internal packages
    "github.com/shiv248/kolabpad/pkg/database"
)
```

**TypeScript**: Prettier plugin handles sorting (configured):

```typescript
// Automatic sorting via @trivago/prettier-plugin-sort-imports
import { useEffect, useState } from "react";
import debounce from "lodash.debounce";
import { useSession } from "@/contexts/SessionProvider";
```

---

## Making Changes

### Typical Workflow

```pseudocode
WORKFLOW for making changes:
    1. Create feature branch:
        git checkout -b feature/my-feature

    2. Make code changes:
        - Edit files in appropriate directory
        - Follow code style guidelines

    3. Test locally:
        - Run dev servers (make dev-backend, make dev-frontend)
        - Test manually in browser
        - Run automated tests (see [02-testing-strategy.md])

    4. Format and lint:
        make lint              # Backend
        make format.frontend   # Frontend
        make lint.frontend     # Frontend

    5. Commit changes:
        git add .
        git commit -m "feat: add feature description"

    6. Push and create pull request:
        git push origin feature/my-feature
        # Create PR on GitHub
```

### Commit Message Conventions

Follow [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` New feature
- `fix:` Bug fix
- `docs:` Documentation changes
- `refactor:` Code restructuring (no behavior change)
- `test:` Adding or updating tests
- `chore:` Maintenance tasks (dependencies, config)

**Examples**:

```bash
git commit -m "feat: add OTP protection for documents"
git commit -m "fix: resolve race condition in persister shutdown"
git commit -m "refactor: extract hook logic from DocumentProvider"
git commit -m "docs: update deployment guide with monitoring section"
```

### Testing Before Commit

Always test your changes before committing:

```bash
# Run all tests
make test.all

# Backend tests
make test

# Frontend tests
make test.frontend

# Manual smoke test
# 1. Start dev servers
# 2. Create document
# 3. Test collaboration (open in multiple tabs)
# 4. Test your specific change
```

See [02-testing-strategy.md](./02-testing-strategy.md) for comprehensive testing guidance.

---

## Common Tasks

### Add a New Go Package

```bash
# 1. Create directory
mkdir pkg/mypackage

# 2. Create Go file
cat > pkg/mypackage/myfile.go <<EOF
package mypackage

// MyFunction does something useful
func MyFunction() {
    // Implementation
}
EOF

# 3. Import in other code
# import "github.com/shiv248/kolabpad/pkg/mypackage"

# 4. Run tests
go test ./pkg/mypackage/...
```

### Add a New Frontend Component

```bash
# 1. Create component file
cat > frontend/src/components/MyComponent.tsx <<EOF
import { FC } from "react";

interface MyComponentProps {
    message: string;
}

export const MyComponent: FC<MyComponentProps> = ({ message }) => {
    return <div>{message}</div>;
};
EOF

# 2. Import and use
# import { MyComponent } from "./components/MyComponent";
```

### Update Dependencies

**Backend**:

```bash
# Update all dependencies
go get -u ./...
go mod tidy

# Update specific dependency
go get -u github.com/mattn/go-sqlite3

# Verify changes
make test
```

**Frontend**:

```bash
cd frontend

# Check outdated packages
npm outdated

# Update all dependencies (semver-safe)
npm update

# Update to latest (including breaking changes)
npm install <package>@latest

# Verify changes
npm test
npm run build
```

### Add Environment Variable

1. **Add to `.env.example`** with documentation:

```bash
# My new feature configuration
MY_FEATURE_ENABLED=true
```

2. **Add to `cmd/server/main.go`** Config struct:

```go
type Config struct {
    // ... existing fields
    MyFeatureEnabled bool
}

// In main():
config := Config{
    // ... existing fields
    MyFeatureEnabled: getEnvBool("MY_FEATURE_ENABLED", true),
}
```

3. **Document in this file** (Environment Variables section)

4. **Add to Docker** if needed (`docker-compose.yml`, `Dockerfile`)

### Clean Build Artifacts

```bash
# Clean everything
make clean.all

# Clean backend only
make clean

# Clean frontend only
make clean.frontend

# Clean Docker environment (removes containers + images)
make docker-clean
```

---

## Troubleshooting

### Backend won't start

**Issue**: `Failed to initialize database`

```bash
# Check database file permissions
ls -la ./data/kolabpad.db

# Remove corrupted database
rm ./data/kolabpad.db

# Restart server (will recreate)
make dev-backend
```

**Issue**: `Port already in use`

```bash
# Find process using port 3030
lsof -i :3030

# Kill process
kill -9 <PID>

# Or change port in .env
echo "PORT=3031" >> .env
```

### Frontend won't start

**Issue**: `npm install` fails

```bash
# Clear npm cache
cd frontend
rm -rf node_modules package-lock.json
npm cache clean --force
npm install
```

**Issue**: WebSocket connection fails

```bash
# Ensure backend is running
curl http://localhost:3030/api/stats

# Check browser console for errors
# Verify proxy configuration in vite.config.ts
```

### WASM not loading

**Issue**: `Failed to load WASM module`

```bash
# Rebuild WASM bridge
make wasm-build

# Verify output files exist
ls -la frontend/public/ot.wasm
ls -la frontend/public/wasm_exec.js

# Restart frontend dev server
make dev-frontend
```

### Tests failing

**Issue**: Backend tests fail

```bash
# Run with verbose output
go test -v ./...

# Run specific package
go test -v ./pkg/ot/...

# Run specific test
go test -v -run TestTransform ./pkg/ot/
```

**Issue**: Frontend tests fail

```bash
cd frontend

# Run with verbose output
npm test -- --reporter=verbose

# Run specific test file
npm test -- useLanguageSync.test

# Update snapshots (if needed)
npm test -- -u
```

---

## Next Steps

- **Testing**: See [02-testing-strategy.md](./02-testing-strategy.md) for testing guidelines
- **Deployment**: See [03-deployment-monitoring.md](./03-deployment-monitoring.md) for production deployment
- **Architecture**: See [../architecture/01-system-overview.md](../architecture/01-system-overview.md) for system design
- **Protocol**: See [../protocol/01-websocket-protocol.md](../protocol/01-websocket-protocol.md) for message format

---

**Questions or Issues?**

- Check existing documentation in `dev-internal/docs/`
- Review GitHub Issues for known problems
- Ask in team chat or open a discussion

**Contributing**: All contributions welcome! Follow the workflow above and maintain code quality standards.
