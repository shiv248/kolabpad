# Multi-stage Dockerfile for Kolabpad
# Builds: Go WASM module → Frontend → Backend → Final image

# Stage 1: Build Go WASM module for browser
FROM golang:1.23-alpine AS wasm
WORKDIR /app

# Copy Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy OT library and WASM bridge
COPY pkg/ot ./pkg/ot
COPY cmd/ot-wasm-bridge ./cmd/ot-wasm-bridge

# Build WASM module
RUN GOOS=js GOARCH=wasm go build -o ot.wasm ./cmd/ot-wasm-bridge

# Copy Go WASM runtime
RUN cp $(go env GOROOT)/misc/wasm/wasm_exec.js .

# Stage 2: Build frontend (React + TypeScript)
FROM node:18-alpine AS frontend
WORKDIR /app

# Accept build arguments
ARG LOG_LEVEL=info
ARG VITE_SHA

# Install dependencies
COPY frontend/package*.json ./
RUN npm ci

# Copy frontend source
COPY frontend/ ./

# Copy WASM module and runtime from stage 1
COPY --from=wasm /app/ot.wasm ./public/
COPY --from=wasm /app/wasm_exec.js ./public/

# Copy Go server source for "Load Sample" feature
COPY pkg/server/kolabpad.go ../pkg/server/kolabpad.go

# Build frontend - ENV vars will be read by vite.config.ts from ARG
ENV LOG_LEVEL=${LOG_LEVEL}
ENV VITE_SHA=${VITE_SHA}
RUN npm run build

# Stage 3: Build Go backend server
FROM golang:1.23-alpine AS backend
WORKDIR /app

# Install build dependencies (gcc for CGO/SQLite)
RUN apk add --no-cache gcc musl-dev

# Copy Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy backend source
COPY cmd/server ./cmd/server
COPY pkg/ ./pkg/
COPY internal/ ./internal/

# Build server with optimizations
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o kolabpad-server ./cmd/server/

# Stage 4: Final runtime image
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy server binary from backend stage
COPY --from=backend /app/kolabpad-server .

# Copy frontend static files from frontend stage
COPY --from=frontend /app/dist ./dist

# Create data directory for SQLite
RUN mkdir -p /data

# Expose server port
EXPOSE 3030

# Environment variables
ENV PORT=3030
ENV EXPIRY_DAYS=7
ENV SQLITE_URI=/data/kolabpad.db

# Run server
CMD ["./kolabpad-server"]
