# Build stage
FROM golang:1.23-alpine AS builder

# Install build dependencies (gcc for CGO/SQLite)
RUN apk add --no-cache gcc musl-dev

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build server
RUN CGO_ENABLED=1 go build -o kolabpad-server -ldflags="-s -w" ./cmd/server/

# Runtime stage
FROM alpine:latest

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/kolabpad-server .

# Create data directory for SQLite
RUN mkdir -p /data

# Expose port
EXPOSE 3030

# Run server
ENV PORT=3030
ENV EXPIRY_DAYS=7
ENV SQLITE_URI=/data/kolabpad.db

CMD ["./kolabpad-server"]
