# Kolabpad

A real-time collaborative text editor powered by Operational Transformation. Multiple users can edit the same document simultaneously with instant synchronization and conflict-free merging.

## Features

- **Real-time collaborative editing** - Multiple users, one document, instant sync
- **Operational Transformation** - Conflict-free concurrent editing with proven OT algorithm
- **Monaco Editor** - Full-featured code editor (same engine as VS Code)
- **Optional OTP Protection** - Secure documents with one-time passwords
- **Persistent Storage** - SQLite database with smart lazy-write strategy
- **WebSocket Communication** - Low-latency bidirectional updates
- **Document Management** - Automatic expiry and cleanup of inactive documents
- **Multi-cursor Support** - See other users' cursors and selections in real-time
- **Syntax Highlighting** - 50+ languages supported via Monaco
- **Go + TypeScript Stack** - Fast backend, modern React frontend
- **WASM Integration** - OT operations run in browser via WebAssembly
- **Docker Ready** - Single-command deployment with Docker Compose

## Quick Start

### Development Mode

Run backend and frontend separately with hot reload:

```bash
# Terminal 1: Start backend server
make dev-backend

# Terminal 2: Start frontend dev server
make dev-frontend
```

Open your browser to `http://localhost:5173`

### Docker Mode

Production-like environment in a single container:

```bash
# Start with Docker Compose
docker-compose up -d

# Or using make
make docker-up
```

Open your browser to `http://localhost:3030`

## Architecture

```
kolabpad/
├── cmd/
│   ├── server/              # Main HTTP/WebSocket server
│   └── ot-wasm-bridge/      # WASM module for browser-side OT
├── pkg/
│   ├── server/              # WebSocket server & document management
│   ├── database/            # SQLite persistence layer
│   ├── ot/                  # Operational Transformation algorithm
│   └── logger/              # Logging utilities
├── internal/
│   └── protocol/            # Message type definitions
├── frontend/
│   ├── src/
│   │   ├── contexts/        # React context providers
│   │   ├── hooks/           # Custom React hooks
│   │   ├── services/        # WebSocket client, OT integration
│   │   ├── components/      # React UI components
│   │   └── types/           # TypeScript definitions
│   └── public/              # Static assets (includes WASM)
├── Dockerfile               # Multi-stage build: WASM → Frontend → Backend
└── docker-compose.yml       # Single-container deployment
```

## Environment Variables

Key configuration options (see `.env.example` for full list):

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `3030` | HTTP server port |
| `EXPIRY_DAYS` | `7` | Days before inactive documents are deleted |
| `SQLITE_URI` | `./data/kolabpad.db` | Database file path (empty = in-memory only) |
| `LOG_LEVEL` | `info` | Logging verbosity: `debug`, `info`, `error` |
| `MAX_DOCUMENT_SIZE_KB` | `256` | Maximum document size in kilobytes |

## API Endpoints

- `WebSocket /api/socket/{id}?otp={token}` - Real-time collaborative editing
- `POST /api/document/{id}/protect` - Enable OTP protection
- `DELETE /api/document/{id}/protect` - Disable OTP protection
- `GET /api/stats` - Server statistics and health metrics

## Development

### Prerequisites

- Go 1.23+
- Node.js 18+
- Docker & Docker Compose (optional)

### Setup

```bash
# One-time setup
make setup

# This will:
# 1. Create .env from .env.example
# 2. Install Go dependencies
# 3. Install frontend dependencies
# 4. Build WASM bridge
```

### Building

```bash
# Build everything (WASM + backend + frontend)
make build.all

# Build components individually
make build              # Backend only
make build.frontend     # Frontend only
make wasm-build         # WASM bridge only
```

### Testing

```bash
# Run all tests
make test.all

# Run backend tests
make test

# Run frontend tests
make test.frontend
```

### Documentation

Comprehensive documentation is available in `dev-internal/docs/`:

- **Architecture**: System design, OT algorithm, persistence strategy
- **Backend**: Server architecture, broadcast system
- **Frontend**: React architecture, state management, WebSocket integration
- **Protocol**: WebSocket messages, REST API
- **Security**: OTP authentication, threat model
- **Development**: Workflow, testing, deployment

## License

MIT

## Acknowledgments

Kolabpad was originally inspired by [Rustpad](https://github.com/ekzhang/rustpad) and implements the Operational Transformation algorithm with reference to:

- [ot.js](https://github.com/Operational-Transformation/ot.js) - JavaScript OT library
- [operational-transformation-go](https://github.com/shiv248/operational-transformation-go) - Go OT implementation
