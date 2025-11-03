# Deployment and Monitoring

**Purpose**: Guide for deploying Kolabpad to production and monitoring system health.

**Audience**: DevOps engineers, system administrators, on-call engineers.

---

## Table of Contents

1. [Deployment Architecture](#deployment-architecture)
2. [Pre-Deployment Checklist](#pre-deployment-checklist)
3. [Deployment Methods](#deployment-methods)
4. [Graceful Shutdown](#graceful-shutdown)
5. [Monitoring and Metrics](#monitoring-and-metrics)
6. [Alerting Rules](#alerting-rules)
7. [Logging](#logging)
8. [Backup and Recovery](#backup-and-recovery)
9. [Scaling Considerations](#scaling-considerations)
10. [Troubleshooting](#troubleshooting)

---

## Deployment Architecture

### Single-Server Design

Kolabpad is designed for **single-server deployment** (current design):

```
Internet
    ↓
[Load Balancer / Reverse Proxy]
    ├── HTTPS termination (TLS)
    ├── Rate limiting
    ├── DDoS protection
    └── Static file caching (optional)
    ↓
[Kolabpad Server]
    ├── Go backend (HTTP + WebSocket)
    ├── Frontend (static files in /dist)
    └── SQLite database (local file)
    ↓
[Persistent Storage]
    └── /data/kolabpad.db (bind mount or volume)
```

### Why Single-Server?

**Design philosophy**:

- **Simplicity**: No distributed state, no coordination overhead
- **Consistency**: CP from CAP theorem (consistent + partition-tolerant)
- **Performance**: All state in memory, zero network latency
- **Cost**: Single VM cheaper than distributed cluster

**Trade-offs accepted**:

- **Single point of failure**: Server crash = all connections lost
  - Mitigation: Graceful shutdown during deployments (zero data loss)
  - Mitigation: Fast restart (SQLite loads quickly)
- **Limited horizontal scaling**: Can't add servers dynamically
  - Mitigation: Vertical scaling sufficient for most use cases
  - Future: Sticky sessions + shared storage if needed

**When to consider multi-server**:

- More than 10,000 active documents simultaneously
- More than 100,000 WebSocket connections
- Geographic distribution required (multi-region)
- HA requirements exceed 99.9% uptime

---

## Pre-Deployment Checklist

### Build Checklist

```bash
✓ Build optimized binaries
  make build.all

✓ Verify frontend build
  ls -lh frontend/dist/  # Should see index.html, assets/

✓ Test production build locally
  ./bin/kolabpad-server  # Should serve on :3030

✓ Run all tests
  make test.all

✓ Check for security vulnerabilities
  go list -json -m all | nancy sleuth  # Or similar tool
  cd frontend && npm audit

✓ Tag release
  git tag v1.0.0
  git push --tags
```

### Infrastructure Checklist

```bash
✓ HTTPS certificate configured (Let's Encrypt recommended)
✓ Firewall rules (only 80/443 exposed to public)
✓ Persistent volume for database (/data)
✓ Log rotation configured (logrotate or similar)
✓ Monitoring agent installed (Prometheus, Datadog, etc.)
✓ Backup automation configured (see Backup section)
✓ Resource limits set (memory, CPU)
✓ DNS configured and tested
```

### Security Checklist

```bash
✓ HTTPS enforced (no HTTP in production)
✓ CORS origins restricted (not wildcard *)
✓ Rate limiting at load balancer level
✓ Database file permissions (0600, readable only by app user)
✓ Environment variables secured (not in logs)
✓ Server running as non-root user
✓ Security headers configured (CSP, HSTS, X-Frame-Options)
```

### Configuration Checklist

Environment variables for production:

```bash
✓ DOMAIN=domain.com                          # Required for Caddy/SSL
✓ EMAIL=you@example.com                      # Required for Let's Encrypt
✓ PORT=3030                                   # Internal port (Caddy proxies to this)
✓ BACKEND_LOG_LEVEL=info                     # Go server logs (not debug - too verbose)
✓ FRONTEND_LOG_LEVEL=error                   # Browser console (error only for production)
✓ EXPIRY_DAYS=7                              # Adjust based on use case
✓ SQLITE_URI=/data/kolabpad.db               # Persistent volume path
✓ CLEANUP_INTERVAL_HOURS=1                   # Default is fine
✓ MAX_DOCUMENT_SIZE_KB=256                   # Prevent abuse
✓ WS_READ_TIMEOUT_MINUTES=30                 # Disconnect idle clients
✓ WS_WRITE_TIMEOUT_SECONDS=10                # Disconnect slow clients
✓ BROADCAST_BUFFER_SIZE=16                   # Default is fine
```

---

## Deployment Methods

### Docker + Caddy Production Deployment (Recommended)

**New streamlined production deployment** with automatic HTTPS via Caddy reverse proxy.

**Quick start**:

```bash
# 1. Configure environment
cp .env.example .env
vim .env  # Set DOMAIN and EMAIL

# 2. Deploy with one command
make docker-prod-build

# 3. View logs
make docker-prod-logs
```

**What this does**:
- Builds Docker images with no cache (clean production build)
- Automatically injects current git SHA into frontend for versioning
- Starts Caddy reverse proxy on ports 80/443
- Caddy automatically obtains Let's Encrypt SSL certificate for your DOMAIN
- Kolabpad runs internally, only accessible via Caddy
- Automatic SSL renewal every 60 days

**Production Makefile commands**:

```bash
make docker-prod-build     # Clean build + deploy (auto git SHA injection)
make docker-prod-up        # Start containers (uses existing images)
make docker-prod-down      # Stop containers
make docker-prod-restart   # Restart without rebuilding
make docker-prod-logs      # Tail combined logs
```

**Architecture**:

```
Internet (port 443/80)
    ↓
[Caddy Container]
    ├── Automatic HTTPS (Let's Encrypt)
    ├── Security headers (HSTS, CSP, etc.)
    ├── Port 80 → 443 redirect
    └── Reverse proxy to Kolabpad
    ↓
[Kolabpad Container] (internal :3030)
    ├── Go backend
    ├── Frontend static files
    └── SQLite database
```

**Files**:
- `docker-compose.yml` - Base configuration
- `docker-compose.prod.yml` - Production overlay (adds Caddy)
- `Caddyfile` - Caddy configuration (reads DOMAIN and EMAIL from env)

**Environment requirements**:
- `DOMAIN`: Your domain name (e.g., domain.com)
- `EMAIL`: Email for Let's Encrypt notifications
- DNS must point to your server IP

---

### Docker Deployment (Manual)

For custom deployments or non-Caddy setups:

**Build image**:

```bash
# Build with version tag
docker build -t kolabpad:v1.0.0 .

# Tag as latest
docker tag kolabpad:v1.0.0 kolabpad:latest

# Push to registry (if using Docker Hub, AWS ECR, etc.)
docker push yourregistry/kolabpad:v1.0.0
```

**Run container**:

```bash
# Create persistent volume
docker volume create kolabpad-data

# Run container
docker run -d \
  --name kolabpad \
  -p 3030:3030 \
  -v kolabpad-data:/data \
  -e PORT=3030 \
  -e LOG_LEVEL=info \
  -e EXPIRY_DAYS=7 \
  --restart unless-stopped \
  kolabpad:v1.0.0

# View logs
docker logs -f kolabpad

# Check health
curl http://localhost:3030/api/stats
```

**Docker Compose**:

```yaml
# docker-compose.prod.yml
version: '3.8'

services:
  kolabpad:
    image: kolabpad:v1.0.0
    ports:
      - "3030:3030"
    environment:
      - PORT=3030
      - LOG_LEVEL=info
      - EXPIRY_DAYS=7
      - SQLITE_URI=/data/kolabpad.db
    volumes:
      - ./data:/data
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "--spider", "http://localhost:3030/api/stats"]
      interval: 30s
      timeout: 10s
      retries: 3

  # Optional: nginx reverse proxy
  nginx:
    image: nginx:alpine
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
      - ./certs:/etc/nginx/certs:ro
    depends_on:
      - kolabpad
```

### Systemd Service (Bare Metal)

**Service file** (`/etc/systemd/system/kolabpad.service`):

```ini
[Unit]
Description=Kolabpad Collaborative Editor
After=network.target

[Service]
Type=simple
User=kolabpad
Group=kolabpad
WorkingDirectory=/opt/kolabpad
EnvironmentFile=/opt/kolabpad/.env
ExecStart=/opt/kolabpad/bin/kolabpad-server
Restart=on-failure
RestartSec=5s

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/kolabpad/data

[Install]
WantedBy=multi-user.target
```

**Deploy**:

```bash
# Copy binary and files
sudo mkdir -p /opt/kolabpad/{bin,data,dist}
sudo cp bin/kolabpad-server /opt/kolabpad/bin/
sudo cp -r frontend/dist/* /opt/kolabpad/dist/
sudo cp .env.example /opt/kolabpad/.env

# Configure
sudo vim /opt/kolabpad/.env  # Set production values

# Create user
sudo useradd -r -s /bin/false kolabpad
sudo chown -R kolabpad:kolabpad /opt/kolabpad

# Install and start service
sudo systemctl daemon-reload
sudo systemctl enable kolabpad
sudo systemctl start kolabpad

# Check status
sudo systemctl status kolabpad
sudo journalctl -u kolabpad -f
```

### Cloud Platforms

**AWS (ECS + Fargate)**:

```yaml
# task-definition.json
{
  "family": "kolabpad",
  "containerDefinitions": [
    {
      "name": "kolabpad",
      "image": "yourregistry/kolabpad:v1.0.0",
      "portMappings": [{"containerPort": 3030}],
      "environment": [
        {"name": "PORT", "value": "3030"},
        {"name": "LOG_LEVEL", "value": "info"}
      ],
      "mountPoints": [
        {
          "sourceVolume": "data",
          "containerPath": "/data"
        }
      ]
    }
  ],
  "volumes": [
    {
      "name": "data",
      "efsVolumeConfiguration": {
        "fileSystemId": "fs-12345678"
      }
    }
  ]
}
```

**Google Cloud (Cloud Run)**:

```bash
# Build and push
gcloud builds submit --tag gcr.io/PROJECT_ID/kolabpad

# Deploy with persistent disk (requires mounting)
gcloud run deploy kolabpad \
  --image gcr.io/PROJECT_ID/kolabpad \
  --platform managed \
  --region us-central1 \
  --allow-unauthenticated \
  --set-env-vars LOG_LEVEL=info,EXPIRY_DAYS=7
```

**Kubernetes**:

```yaml
# deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kolabpad
spec:
  replicas: 1  # Single-server design
  selector:
    matchLabels:
      app: kolabpad
  template:
    metadata:
      labels:
        app: kolabpad
    spec:
      containers:
      - name: kolabpad
        image: kolabpad:v1.0.0
        ports:
        - containerPort: 3030
        env:
        - name: PORT
          value: "3030"
        - name: LOG_LEVEL
          value: "info"
        volumeMounts:
        - name: data
          mountPath: /data
      volumes:
      - name: data
        persistentVolumeClaim:
          claimName: kolabpad-pvc
```

---

## Graceful Shutdown

Kolabpad implements graceful shutdown to **prevent data loss** during deployments.

### How It Works

```pseudocode
ON SIGTERM received:
    logger.info("Graceful shutdown initiated")

    // 1. Stop accepting new connections
    httpServer.stopListening()

    // 2. Flush all active documents to database
    FOR EACH document IN activeDocuments:
        snapshot = document.snapshot()
        otp = document.getOTP()

        database.store(documentId, snapshot.text, snapshot.language, otp)
        logger.info("Flushed document", documentId)

        IF document.persisterCancel:
            document.persisterCancel()  // Stop background persister

        document.kill()  // Mark as inactive

    // 3. Close database connection
    database.close()

    logger.info("Graceful shutdown complete - flushed N documents")
    EXIT 0
```

**Implementation**: `cmd/server/main.go`

```go
// Handle graceful shutdown
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

go func() {
    <-sigChan
    logger.Info("Shutting down...")
    cancel()
    srv.Shutdown(ctx)
    os.Exit(0)
}()
```

### Zero-Downtime Deployments

**With load balancer**:

```pseudocode
DEPLOYMENT PROCESS:
    1. Deploy new version (new container/server)
    2. Health check passes on new version
    3. Load balancer routes NEW connections to new version
    4. Load balancer stops routing to old version (drain)
    5. Wait for old version connections to close naturally
    6. Send SIGTERM to old version
    7. Old version flushes all documents to database
    8. Old version exits cleanly
    9. New version serves all traffic

RESULT:
    - Zero connection drops
    - Zero data loss
    - Users may notice brief reconnect (WebSocket closes, auto-reconnects)
```

**Testing graceful shutdown**:

```bash
# Start server
./bin/kolabpad-server &
PID=$!

# Connect client and make edits
curl http://localhost:3030/api/socket/test123  # (WebSocket connection)

# Send SIGTERM
kill -TERM $PID

# Verify logs show:
# - "Shutting down..."
# - "Flushed document test123"
# - "Graceful shutdown complete"

# Verify database has latest data
sqlite3 ./data/kolabpad.db "SELECT id, text FROM documents WHERE id='test123';"
```

---

## Monitoring and Metrics

### Key Metrics

Kolabpad should expose metrics for monitoring system health. See [../architecture/03-persistence-strategy.md](../architecture/03-persistence-strategy.md) for detailed persistence metrics.

#### Performance Metrics

```
documents_in_memory                 # Current count of loaded documents
active_persisters_count             # Background persisters running
websocket_connections_total         # Active WebSocket connections
memory_usage_bytes                  # Process memory usage
goroutine_count                     # Active goroutines (detect leaks)

# Persistence metrics
db_writes_total                     # Total database writes
db_writes_per_minute                # Write rate (should be ~100 for 100 active docs)
db_reads_total                      # Total database reads (should be near 0)
persist_latency_seconds             # Time to write document (histogram)
```

#### Behavior Metrics

```
# Persistence reasons (counter per reason)
idle_persist_total                  # Writes triggered by 30s idle
safety_net_persist_total            # Writes triggered by 5min safety net
critical_write_total                # Immediate writes (OTP changes)
last_user_flush_total               # Writes when last user disconnects
eviction_flush_total                # Writes before memory eviction
shutdown_flush_total                # Writes during graceful shutdown
```

#### Health Metrics

```
persist_errors_total                # Database write failures
otp_validation_errors_total         # Invalid OTP attempts (potential attack)
document_evictions_total            # Documents removed from memory
document_expirations_total          # Documents deleted from database
uptime_seconds                      # Time since server started
```

### Metrics Endpoint

**Expose metrics** via `/api/stats` endpoint:

```json
GET /api/stats

{
  "documents_in_memory": 42,
  "active_persisters": 42,
  "websocket_connections": 108,
  "memory_usage_mb": 67.2,
  "goroutine_count": 156,
  "db_writes_total": 1234,
  "db_writes_per_minute": 2.1,
  "persist_errors": 0,
  "uptime_seconds": 86400
}
```

### Prometheus Integration

**Example Prometheus scrape config**:

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'kolabpad'
    scrape_interval: 15s
    static_configs:
      - targets: ['kolabpad:3030']
    metrics_path: '/api/stats'
```

**Implementation**: Expose metrics in Prometheus format if using Prometheus:

```go
// pkg/server/metrics.go
import "github.com/prometheus/client_golang/prometheus"

var (
    documentsInMemory = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "kolabpad_documents_in_memory",
            Help: "Number of documents currently in memory",
        },
    )

    dbWrites = prometheus.NewCounter(
        prometheus.CounterOpts{
            Name: "kolabpad_db_writes_total",
            Help: "Total number of database writes",
        },
    )
)
```

---

## Alerting Rules

### Critical Alerts (Page On-Call)

**Alert immediately** - system is degraded or data at risk:

```yaml
# Prometheus alerting rules
groups:
  - name: kolabpad_critical
    interval: 30s
    rules:
      # Database write failures
      - alert: DatabaseWriteFailures
        expr: rate(persist_errors_total[5m]) > 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Database writes failing"
          description: "Persist errors detected - data loss risk"

      # Memory exhaustion
      - alert: HighMemoryUsage
        expr: memory_usage_bytes / memory_limit_bytes > 0.8
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Memory usage above 80%"
          description: "Need to tune retention or add capacity"

      # Goroutine leak
      - alert: GoroutineLeakDetected
        expr: rate(goroutine_count[5m]) > 10
        for: 10m
        labels:
          severity: critical
        annotations:
          summary: "Goroutine count growing rapidly"
          description: "Possible persister not stopping correctly"

      # Server down
      - alert: ServerDown
        expr: up{job="kolabpad"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Kolabpad server is down"
          description: "Server not responding to health checks"
```

### Warning Alerts

**Alert but not urgent** - investigate during business hours:

```yaml
  - name: kolabpad_warnings
    interval: 1m
    rules:
      # Too many database writes (lazy persistence not working)
      - alert: HighDatabaseWriteRate
        expr: db_writes_per_minute > 10  # Expect ~1-2 for idle docs
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Database write rate unusually high"
          description: "Lazy persistence may not be working correctly"

      # Persister leak (more persisters than docs)
      - alert: PersisterLeakSuspected
        expr: active_persisters_count > documents_in_memory
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "More persisters than documents"
          description: "Persisters may not be stopping when users leave"

      # Potential DoS attack (many invalid OTP attempts)
      - alert: HighOTPValidationErrors
        expr: rate(otp_validation_errors_total[1m]) > 10
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Many invalid OTP attempts"
          description: "Possible DoS attack or misconfiguration"

      # Many documents in memory (may need capacity planning)
      - alert: HighDocumentCount
        expr: documents_in_memory > 1000
        for: 30m
        labels:
          severity: warning
        annotations:
          summary: "Over 1000 documents in memory"
          description: "Consider capacity planning or retention tuning"
```

---

## Logging

### Log Levels

Kolabpad has **separate log levels** for backend and frontend:

**Backend (Go server)** - Configure via `BACKEND_LOG_LEVEL`:
- **debug**: Verbose (every message, operation, state change)
- **info**: Standard operational events (**recommended for production**)
- **warn**: Warnings and errors only (good for production monitoring - captures abnormal events)
- **error**: Only errors (too quiet, not recommended - you won't see startup/requests)

**Frontend (Browser console)** - Configure via `FRONTEND_LOG_LEVEL`:
- **debug**: All console.log visible (development only)
- **info**: Info and error logs
- **error**: Only error logs (**recommended for production** - keeps console clean)

**Production recommendation**:
```bash
BACKEND_LOG_LEVEL=info      # See server operations in docker logs (or 'warn' for quieter logs)
FRONTEND_LOG_LEVEL=error    # Keep browser console quiet
```

### Key Log Events

**Server lifecycle**:

```
info: Starting Kolabpad server...
info: Port: 3030
info: Database: ./data/kolabpad.db
info: Listening on :3030
info: Shutting down...
info: Graceful shutdown complete - flushed 42 documents
```

**Document lifecycle**:

```
info: persister_started docId=abc123 userCount=1
info: persister_stopped docId=abc123 reason=last_user_disconnect
info: document_evicted docId=abc123 age=24h
info: document_expired docId=abc123 age=7d
```

**Persistence events**:

```
info: persisted docId=abc123 reason=idle_threshold revision=42
info: persisted docId=abc123 reason=safety_net revision=100
info: critical_write docId=abc123 type=otp_protect
info: flush docId=abc123 reason=last_disconnect
debug: persist_latency docId=abc123 duration=2.3ms
```

**Connection events**:

```
info: User 5 connected
info: User 5 disconnected
warn: User 5 disconnected forcefully
error: Disconnect reason: websocket: close 1006 (abnormal closure)
```

**OTP validation**:

```
info: otp_validation docId=abc123 source=memory result=valid
info: otp_validation docId=xyz789 source=database result=invalid
warn: otp_validation_rate_high ip=192.168.1.1 count=50
```

**Errors**:

```
error: database_write_failed docId=abc123 error="database locked"
error: websocket_write_timeout userId=5 duration=10s
error: panic_recovered error="runtime error" stack=...
```

### Log Aggregation

**Structured logging** recommended for production:

```go
// Use structured logger (e.g., zap, zerolog)
logger.Info("persisted",
    zap.String("docId", docId),
    zap.String("reason", "idle_threshold"),
    zap.Int("revision", revision),
    zap.Duration("latency", duration),
)

// Output (JSON format for parsing):
{"level":"info","ts":1234567890,"msg":"persisted","docId":"abc123","reason":"idle_threshold","revision":42,"latency":"2.3ms"}
```

**Log rotation** (if not using container logs):

```bash
# /etc/logrotate.d/kolabpad
/var/log/kolabpad/*.log {
    daily
    missingok
    rotate 7
    compress
    delaycompress
    notifempty
    create 0640 kolabpad kolabpad
    sharedscripts
    postrotate
        systemctl reload kolabpad
    endscript
}
```

---

## Backup and Recovery

### Database Backups

SQLite database is **single point of persistence** - backups are critical.

**Automated backup** (cron job):

```bash
#!/bin/bash
# /usr/local/bin/backup-kolabpad.sh

BACKUP_DIR="/backups/kolabpad"
DATE=$(date +%Y%m%d-%H%M)
DB_PATH="/data/kolabpad.db"

mkdir -p "$BACKUP_DIR"

# SQLite backup (safe even while server running)
sqlite3 "$DB_PATH" ".backup '$BACKUP_DIR/kolabpad-$DATE.db'"

# Compress
gzip "$BACKUP_DIR/kolabpad-$DATE.db"

# Keep only last 30 days
find "$BACKUP_DIR" -name "kolabpad-*.db.gz" -mtime +30 -delete

echo "Backup completed: kolabpad-$DATE.db.gz"
```

**Crontab**:

```cron
# Run backup every 6 hours
0 */6 * * * /usr/local/bin/backup-kolabpad.sh
```

### Recovery Procedure

```pseudocode
RECOVERY from backup:
    1. Stop server:
        systemctl stop kolabpad
        (or docker stop kolabpad)

    2. Replace database file:
        cp /backups/kolabpad/kolabpad-20250101-0600.db.gz /tmp/
        gunzip /tmp/kolabpad-20250101-0600.db.gz
        mv /tmp/kolabpad-20250101-0600.db /data/kolabpad.db
        chown kolabpad:kolabpad /data/kolabpad.db

    3. Start server:
        systemctl start kolabpad

    4. Verify:
        curl http://localhost:3030/api/stats
        # Check documents load correctly

EXPECTED BEHAVIOR after recovery:
    - Documents load from backup on first access
    - Any edits after backup time are lost (acceptable)
    - Users reconnect automatically (WebSocket reconnect logic)
```

### Disaster Recovery

**Complete data loss** (database corrupted or deleted):

```pseudocode
IF no backup available:
    1. Server starts with empty database
    2. Users who reconnect create fresh documents (empty)
    3. Data is lost (acceptable for ephemeral collaboration tool)

IF backup available but old (e.g., 24 hours):
    1. Restore backup (see above)
    2. Documents from last 24 hours may be missing
    3. Users can re-create documents as needed
    4. Communicate data loss to affected users
```

**Mitigation**:

- More frequent backups (every hour instead of every 6 hours)
- Offsite backup replication (S3, GCS)
- Point-in-time recovery (requires WAL mode in SQLite)

---

## Scaling Considerations

### Current Capacity Estimates

With memory-first architecture on single server:

```
Active Documents (memory usage):
    50 docs   → ~12 MB RAM    ✓ Comfortable
    100 docs  → ~25 MB RAM    ✓ Good
    500 docs  → ~125 MB RAM   ✓ Feasible
    1,000 docs → ~250 MB RAM  ✓ Possible (2GB VM)
    10,000 docs → ~2.5 GB RAM ⚠ Need tuning (8GB+ VM)
```

**Bottlenecks**:

1. **Memory** (primary): Documents stored in RAM
2. **SQLite writes** (secondary): Single-writer limitation
3. **CPU** (minor): OT operations, WebSocket message handling

### Vertical Scaling

**When to scale up**:

- More than 500 active documents
- Memory usage consistently above 70%
- DB write latency increasing (>100ms)

**How to scale up**:

```
Current:   2 vCPU, 2 GB RAM  →  Good for ~100 active docs
Upgrade:   4 vCPU, 8 GB RAM  →  Good for ~1,000 active docs
Upgrade:  16 vCPU, 32 GB RAM →  Good for ~10,000 active docs
```

### Horizontal Scaling (Future)

**Not currently supported** but possible with architecture changes:

```pseudocode
MULTI-SERVER ARCHITECTURE (future):
    [Load Balancer with sticky sessions]
        ↓
    [Server 1] [Server 2] [Server 3]
        ↓
    [Shared Storage: Redis for OTP, S3 for documents]

CHANGES REQUIRED:
    - Sticky sessions (route user to same server based on documentId hash)
    - Shared OTP storage (Redis instead of in-memory)
    - Shared document metadata (Redis for "which server has this doc")
    - Persistence to S3 instead of local SQLite
    - Inter-server communication (if same doc loaded on multiple servers)

COMPLEXITY INCREASE:
    - CAP theorem: Must choose AP (give up consistency) or CP (give up availability)
    - Network partitions: What if Redis unreachable?
    - Split-brain: What if two servers think they own same document?
```

**Recommendation**: Vertical scaling sufficient for most use cases. Only implement horizontal scaling if:

- Expected: >10,000 concurrent documents
- Budget: Can afford operational complexity
- Expertise: Team familiar with distributed systems

---

## Troubleshooting

### High Database Write Rate

**Symptom**: `db_writes_per_minute` metric is high (>10 for typical workload)

**Diagnosis**:

```bash
# Check persister metrics
curl http://localhost:3030/api/stats | jq '.idle_persist_total, .safety_net_persist_total'

# Check logs for persist reasons
journalctl -u kolabpad | grep "persisted" | tail -50

# Expected: Most writes should be "idle_threshold" or "safety_net"
# Problem: Too many "safety_net" writes means idle threshold not triggering
```

**Possible causes**:

1. **Users constantly typing** (safety net always triggers)
   - Solution: Acceptable, this is normal for active editing
2. **Persister not stopping** when users leave
   - Solution: Check `active_persisters` vs `documents_in_memory`
3. **Configuration issue** (idle threshold too low)
   - Solution: Review persistence thresholds in `pkg/server/kolabpad.go`

### Memory Growth

**Symptom**: `memory_usage_bytes` grows unbounded

**Diagnosis**:

```bash
# Check document count
curl http://localhost:3030/api/stats | jq '.documents_in_memory'

# Check eviction logs
journalctl -u kolabpad | grep "document_evicted"

# Check goroutine count (potential leak)
curl http://localhost:3030/debug/pprof/goroutine
```

**Possible causes**:

1. **Documents not being evicted** after 24h idle
   - Solution: Check cleanup task is running (`StartCleaner` in `main.go`)
2. **Goroutine leak** (persisters not stopping)
   - Solution: Check `active_persisters` vs `documents_in_memory`
   - Solution: Profile with `pprof`: `go tool pprof http://localhost:3030/debug/pprof/heap`
3. **Too many active documents** (legitimate load)
   - Solution: Increase server memory or tune eviction threshold

### Goroutine Leak

**Symptom**: `goroutine_count` grows continuously

**Diagnosis**:

```bash
# Get goroutine dump
curl http://localhost:3030/debug/pprof/goroutine?debug=2 > goroutines.txt

# Look for patterns (many similar goroutines)
grep -A 5 "persister" goroutines.txt | wc -l

# Expected: ~1 goroutine per active document + overhead
# Problem: If way more goroutines than documents
```

**Possible causes**:

1. **Persisters not stopped** when last user disconnects
   - Solution: Verify `persisterCancel()` called in `handleDisconnect`
2. **WebSocket goroutines leaked** (not cleaned up)
   - Solution: Check defer cleanup in connection handlers

### Database Lock Errors

**Symptom**: `error: database_write_failed error="database locked"`

**SQLite limitation**: Single writer at a time

**Diagnosis**:

```bash
# Check write rate
curl http://localhost:3030/api/stats | jq '.db_writes_per_minute'

# If >100 writes/min with many docs, may hit concurrency limit
```

**Solutions**:

1. **Enable WAL mode** (Write-Ahead Logging):

```go
// pkg/database/database.go
db.Exec("PRAGMA journal_mode=WAL;")

// Allows concurrent reads while writing
```

2. **Increase timeout**:

```go
db.SetConnMaxLifetime(30 * time.Second)
```

3. **Use connection pool** (SQLite allows multiple readers):

```go
db.SetMaxOpenConns(10)
db.SetMaxIdleConns(5)
```

### WebSocket Connection Drops

**Symptom**: Users frequently disconnected and reconnected

**Diagnosis**:

```bash
# Check timeout configuration
echo $WS_READ_TIMEOUT_MINUTES    # Should be 30
echo $WS_WRITE_TIMEOUT_SECONDS   # Should be 10

# Check logs for timeout messages
journalctl -u kolabpad | grep "timeout"

# Check network path (proxies, load balancer)
# Some proxies terminate idle WebSockets after 1 min
```

**Solutions**:

1. **WebSocket ping/pong** (keep-alive):
   - Client sends periodic ping (every 20s)
   - Server responds with pong
   - Prevents idle timeout

2. **Increase timeout** (if too aggressive):

```bash
WS_READ_TIMEOUT_MINUTES=60  # Allow 1 hour idle
```

3. **Load balancer configuration**:
   - Increase WebSocket idle timeout (if behind proxy)
   - Enable WebSocket support explicitly

---

## Health Checks

### HTTP Health Endpoint

```bash
# Basic health check
curl http://localhost:3030/api/stats

# Should return 200 OK with JSON body
# If 500 or no response, server is unhealthy
```

### Comprehensive Health Check

```pseudocode
HEALTH CHECK script:
    1. Check HTTP responds:
        curl http://localhost:3030/api/stats
        IF status != 200: CRITICAL

    2. Check metrics reasonable:
        stats = parseJSON(response)

        IF stats.persist_errors > 0: CRITICAL
        IF stats.memory_usage_mb > threshold: WARNING
        IF stats.goroutine_count > documents * 5: WARNING

    3. Check database accessible:
        sqlite3 /data/kolabpad.db "SELECT COUNT(*) FROM documents;"
        IF error: CRITICAL

    4. Check disk space:
        df -h /data
        IF usage > 90%: CRITICAL
```

---

## Next Steps

- **Development**: See [01-development-workflow.md](./01-development-workflow.md) for local setup
- **Testing**: See [02-testing-strategy.md](./02-testing-strategy.md) for testing before deployment
- **Persistence**: See [../architecture/03-persistence-strategy.md](../architecture/03-persistence-strategy.md) for detailed persistence monitoring

---

**Production Readiness**: Kolabpad is designed for straightforward deployment with minimal operational overhead. Follow this guide for reliable production operation.
