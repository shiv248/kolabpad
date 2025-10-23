# Security Considerations

**Purpose**: This document outlines Kolabpad's threat model, security trade-offs, and operational security guidance. It explains what Kolabpad protects against, what it doesn't, and how to deploy it securely.

**Audience**: System administrators, security engineers, and decision-makers evaluating Kolabpad for deployment.

---

## Table of Contents

1. [Threat Model](#threat-model)
2. [Acceptable Data Loss](#acceptable-data-loss)
3. [Transport Security](#transport-security)
4. [Database Security](#database-security)
5. [OTP Security](#otp-security)
6. [Memory Safety](#memory-safety)
7. [Denial of Service](#denial-of-service)
8. [What NOT to Use Kolabpad For](#what-not-to-use-kolabpad-for)
9. [Future Security Improvements](#future-security-improvements)

---

## Threat Model

Understanding what Kolabpad protects against—and what it doesn't—is critical for appropriate deployment.

### What Kolabpad Protects Against

**✅ Unauthorized Document Access**
- **Protection**: OTP-based access control
- **Mechanism**: Documents can be protected with cryptographically secure tokens
- **Scenario**: Alice shares document link with OTP only to Bob. Charlie cannot access without the OTP.

**✅ Tampering with Document Content**
- **Protection**: Server-authoritative Operational Transformation (OT)
- **Mechanism**: Server is single source of truth; clients cannot forge operations
- **Scenario**: Malicious client cannot inject operations with fake user IDs or out-of-sequence revisions.

**✅ DoS via OTP Guessing**
- **Protection**: Dual-check pattern (see [01-authentication-model.md](01-authentication-model.md))
- **Mechanism**: OTP validated before loading document into memory
- **Scenario**: Attacker tries 10,000 wrong OTPs; server rejects without memory allocation.

**✅ Basic Message Size Limits**
- **Protection**: WebSocket message size limits (25KB)
- **Mechanism**: Large malicious messages rejected
- **Scenario**: Attacker cannot send multi-gigabyte messages to exhaust memory.

### What Kolabpad Does NOT Protect Against

**❌ Sophisticated DoS Attacks**
- **Gap**: No rate limiting on most endpoints
- **Risk**: Attacker can exhaust resources via many legitimate requests
- **Mitigation Required**: Deploy behind edge protection (Cloudflare, AWS Shield, etc.)

**❌ Man-in-the-Middle Attacks**
- **Gap**: Kolabpad itself doesn't enforce HTTPS
- **Risk**: Unencrypted WebSocket (WS instead of WSS) exposes data in transit
- **Mitigation Required**: Deployment configuration MUST enforce HTTPS/WSS

**❌ Server Compromise**
- **Gap**: No encryption at rest
- **Risk**: If attacker gains filesystem access, all documents readable
- **Mitigation**: Operating system security, file permissions, encrypted disks

**❌ OTP Leakage**
- **Gap**: OTPs visible in URLs, browser history, server logs
- **Risk**: Shared computers, screenshot leaks, log aggregation exposure
- **Mitigation**: User education, secure sharing practices

**❌ Compliance Requirements**
- **Gap**: No HIPAA, GDPR, PCI-DSS controls
- **Risk**: Cannot be used for regulated data without extensive modifications
- **Mitigation**: Don't use Kolabpad for compliance-requiring data

**❌ CSRF Attacks**
- **Gap**: No CSRF token validation on REST endpoints
- **Risk**: Malicious website could enable/disable OTP if user connected to document
- **Mitigation**: Requires user to have active WebSocket connection (partial mitigation)

### Attacker Profiles

**Casual Attacker** (Low skill, opportunistic)
- **Protections**: OTP access control effective
- **Risks**: URL sharing in public forums

**Motivated Attacker** (Medium skill, targeted)
- **Protections**: Dual-check DoS prevention, server-authoritative OT
- **Risks**: No rate limiting, CSRF attacks, social engineering for OTP

**Advanced Attacker** (High skill, persistent)
- **Protections**: Cryptographically secure OTP generation
- **Risks**: Server compromise, traffic interception (if HTTPS not enforced), sophisticated DoS

### Trust Boundaries

```
[External Network]
        ↓
   [Load Balancer] ← HTTPS termination happens here
        ↓
   [Kolabpad Server]
        ├─ WebSocket connections (trust but verify)
        ├─ REST API (no authentication beyond OTP)
        └─ SQLite database (trusted storage)
```

**Key Assumptions**:
1. Server infrastructure is trusted and secure
2. TLS/HTTPS is properly configured at load balancer
3. File system permissions protect SQLite database
4. Network between load balancer and server is trusted
5. Users share OTPs via secure channels (not guaranteed)

---

## Acceptable Data Loss

Kolabpad's architecture deliberately accepts potential data loss in exchange for performance and simplicity. This is a **conscious design decision**, not a bug.

### By Design: Data Loss on Crash

**Loss Window**: 30 seconds to 5 minutes of recent edits

**Scenario**:
```
Timeline:
  T+0s:   User types "important data"
  T+10s:  More edits
  T+20s:  Server crash (power failure, OOM, etc.)
  T+21s:  Server restarts, loads from database

Result: Last 20 seconds of edits LOST
```

**Why This Trade-off?**

See [../architecture/03-persistence-strategy.md](../architecture/03-persistence-strategy.md) for full rationale:
- Memory-first architecture for real-time performance
- Lazy persistence (idle threshold: 30s, safety net: 5min)
- Reduces database writes from ~100/sec to ~100/min
- SQLite is backup, not source of truth

**Mitigation Strategies**:

1. **Graceful Shutdown** (Zero Loss)
   ```pseudocode
   ON SIGTERM (deployment, restart, shutdown):
       Stop accepting new connections
       Flush ALL active documents to database
       Wait for writes to complete
       Exit

   Result: Zero data loss on planned restarts
   ```

2. **Safety Net Persistence** (Max 5 Minutes Loss)
   - Persister writes to database every 5 minutes even if editing continuously
   - Worst case: Crash right before 5-minute mark = ~5min data loss

3. **User Expectations**
   - Kolabpad is for ephemeral, short-term collaboration
   - Users should save important content elsewhere
   - Not a replacement for Google Docs, Notion, or proper document storage

### Acceptable Risk Profile

**What this model is good for**:
- ✅ Quick code snippet sharing
- ✅ Collaborative troubleshooting notes (can re-type if lost)
- ✅ Pair programming session (both participants see same state)
- ✅ Temporary `.env` file sharing

**What this model is NOT good for**:
- ❌ Writing important documents (use Google Docs)
- ❌ Critical data entry (use proper database)
- ❌ Legal documents (audit requirements)
- ❌ Anything where 5 minutes of data loss is unacceptable

### Monitoring Data Loss

**Key Metric**: `persist_errors`
- Should be 0 in healthy system
- If > 0: Database issues, investigate immediately

**Other Indicators**:
- `shutdown_flush_count`: How many docs flushed on graceful shutdown
- `safety_net_persist_count`: How often safety net triggered (should be rare)
- `idle_persist_count`: Normal lazy persistence (should be most writes)

---

## Transport Security

Kolabpad transmits data over WebSocket connections and HTTP REST APIs. Proper transport security is **required** but not enforced by Kolabpad itself.

### HTTPS/WSS Requirement

**Critical**: Kolabpad MUST be deployed behind HTTPS termination.

**Why**:
- WebSocket connections upgrade from HTTP → WS or HTTPS → WSS
- Without HTTPS, all data transmitted in plain text
- OTP tokens visible to network observers
- Man-in-the-middle attacks possible

**Deployment Architecture**:
```
[Client Browser]
      ↓ HTTPS (TLS 1.2+)
[Load Balancer / Reverse Proxy]
      ↓ WebSocket Upgrade (WSS)
      ↓ HTTP (internal network, trusted)
[Kolabpad Server]
```

**Recommended Setup**:
- Use Let's Encrypt for free TLS certificates
- Configure Nginx or Caddy as reverse proxy
- Force HTTPS redirect (HTTP → HTTPS)
- Enable HSTS header: `Strict-Transport-Security: max-age=31536000`

### TLS Configuration

**Minimum Requirements**:
- TLS 1.2 or higher (TLS 1.3 preferred)
- Strong cipher suites (no RC4, no MD5, no export ciphers)
- Valid certificate from trusted CA
- Disable SSLv3, TLS 1.0, TLS 1.1

**Example Nginx Configuration**:
```nginx
server {
    listen 443 ssl http2;
    server_name kolabpad.example.com;

    ssl_certificate /etc/letsencrypt/live/kolabpad.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/kolabpad.example.com/privkey.pem;

    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers 'ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256';
    ssl_prefer_server_ciphers off;

    add_header Strict-Transport-Security "max-age=31536000" always;

    location / {
        proxy_pass http://localhost:3030;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

### CORS Configuration

**Current State**: Kolabpad needs CORS configuration for cross-origin requests.

**Recommendations**:
- **Development**: Allow `localhost` origins
- **Production**: Restrict to specific allowed origins (NO wildcards)

```go
// Example restrictive CORS policy
allowedOrigins := []string{
    "https://kolabpad.example.com",
    "https://app.kolabpad.example.com",
}
```

**Never do this in production**:
```go
// INSECURE: Allows any origin
Access-Control-Allow-Origin: *
```

### WebSocket Origin Validation

Validate WebSocket `Origin` header to prevent unauthorized cross-origin connections:

```pseudocode
ON WebSocket upgrade request:
    origin = request.header("Origin")

    IF origin NOT IN allowedOrigins:
        REJECT with 403 Forbidden
        LOG "Rejected WebSocket from unauthorized origin"
    ELSE:
        ACCEPT connection
```

---

## Database Security

Kolabpad uses SQLite as persistence layer. Understanding database security implications is important for deployment.

### SQLite Considerations

**Single-File Database**:
- Database is a single `.db` file on disk
- File location: Configurable via `DB_PATH` environment variable
- Default: `./data/kolabpad.db`

**No Network Attack Surface**:
- SQLite is in-process (no TCP port)
- Cannot be remotely exploited like PostgreSQL/MySQL
- Attack requires filesystem access

**File System Permissions**:
```bash
# Recommended permissions
chown kolabpad:kolabpad /app/data/kolabpad.db
chmod 600 /app/data/kolabpad.db

# Owner: read/write
# Group: none
# Others: none
```

### What's Stored in the Database

**Documents Table** (`pkg/database/migrations/1_document.sql`):
```sql
CREATE TABLE IF NOT EXISTS document (
    id TEXT PRIMARY KEY,        -- Document ID (visible in URL)
    text TEXT NOT NULL,         -- Plain text content (UNENCRYPTED)
    language TEXT,              -- Syntax highlighting language
    otp TEXT,                   -- OTP token (UNENCRYPTED, nullable)
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

**Sensitive Data**:
- ⚠️ Document text (plain text)
- ⚠️ OTP tokens (plain text)

**NOT Stored**:
- User accounts, passwords, emails
- Operation history (only final document text)
- User personal information

### No Encryption at Rest

**Current State**: Database is NOT encrypted.

**Implications**:
- Anyone with filesystem access can read all documents
- OTP tokens visible in plain text
- No protection against stolen database file

**Threat Scenarios**:
1. **Server Compromise**: Attacker gains shell access → reads database
2. **Backup Theft**: Attacker steals backup file → reads all documents
3. **Insider Threat**: Administrator with filesystem access → full read access

**Mitigations**:
- Encrypted disk storage (LUKS, dm-crypt, BitLocker)
- File system permissions (limit to kolabpad user only)
- Secure backup storage (encrypt backups separately)
- Don't store sensitive data in Kolabpad

**Future Enhancement**: SQLite Encryption Extension (SEE) or SQLCipher
- Transparent encryption/decryption
- Key management required
- Performance overhead (~5-15%)

### Database Backup Security

**Backup Strategy** (see [../development/03-deployment-monitoring.md](../development/03-deployment-monitoring.md)):
```bash
# Automated backup every 6 hours
sqlite3 /app/data/kolabpad.db ".backup '/backups/kolabpad-$(date +%Y%m%d-%H%M).db'"
```

**Security Considerations**:
- Backups contain same sensitive data as primary database
- Backup retention (7 days recommended)
- Backup location must be secured (same permissions as primary)
- Off-site backups should be encrypted in transit

**Recommended Backup Process**:
```bash
# Create backup
sqlite3 /app/data/kolabpad.db ".backup '/tmp/kolabpad-backup.db'"

# Encrypt backup
gpg --encrypt --recipient admin@example.com /tmp/kolabpad-backup.db

# Upload encrypted backup
aws s3 cp /tmp/kolabpad-backup.db.gpg s3://backups/

# Clean up
rm /tmp/kolabpad-backup.db /tmp/kolabpad-backup.db.gpg
```

### SQL Injection

**Status**: Not vulnerable to SQL injection.

**Why**:
- All queries use parameterized statements (prepared statements)
- No dynamic SQL string concatenation
- User input never directly interpolated into queries

**Example Safe Query** (from `pkg/database/database.go`):
```go
// SAFE: Uses parameterized query
_, err := d.db.Exec(
    "INSERT OR REPLACE INTO document (id, text, language, otp, updated_at) VALUES (?, ?, ?, ?, ?)",
    doc.ID, doc.Text, doc.Language, doc.OTP, time.Now(),
)
```

**Anti-Pattern** (not used in Kolabpad):
```go
// VULNERABLE: String concatenation
query := "SELECT * FROM document WHERE id = '" + userInput + "'"
```

---

## OTP Security

Beyond the authentication model (see [01-authentication-model.md](01-authentication-model.md)), OTP tokens have inherent security properties and limitations.

### OTP Token Properties Review

**Strength**: 12 characters, URL-safe base64
- Entropy: 72 bits
- Combinations: 2^72 ≈ 4.7 × 10^21
- Collision probability: Negligible

**Generation**: Cryptographically secure (`crypto/rand`)

**Lifetime**: Until manually disabled or document expires (7 days max)

### Attack Vectors and Mitigations

#### 1. Brute Force Attack

**Attack**: Attacker systematically tries all possible OTPs.

**Feasibility**:
- 2^72 combinations
- At 1 million attempts per second: Would take 149,000 years
- **Practically impossible** even without rate limiting

**Mitigation**: Strong cryptographic entropy makes brute force infeasible.

#### 2. URL Leakage

**Attack**: OTP exposed via URL in unintended contexts.

**Leakage Paths**:
- Browser history on shared computers
- Server access logs (`GET /api/socket/doc123?otp=xyz`)
- Referer header when clicking external links
- Screenshot with browser address bar visible
- Browser sync services (history synced to cloud)
- Link preview services (Slack, Discord unfurling URLs)

**Mitigations**:
- User education (private browsing, secure sharing)
- Disable OTP when collaboration complete
- Use `Referrer-Policy: no-referrer` header
- Strip query params from logs (server configuration)

**Example Log Sanitization**:
```go
// Log documentId but NOT otp parameter
log.Info("WebSocket connection",
    "documentId", docID,
    "hasOTP", otp != "",  // Boolean, not actual value
)
```

#### 3. Interception in Transit

**Attack**: Network observer captures OTP during transmission.

**Protection**: HTTPS/WSS encrypts all communication
- OTP in WebSocket connection URL: Encrypted by TLS
- OTP in REST API requests: Encrypted by TLS

**Failure Scenario**: If HTTPS not enforced, OTP visible to:
- Network administrators
- ISP
- Government surveillance
- Public WiFi attackers

**Mitigation**: MUST deploy with HTTPS (see [Transport Security](#transport-security)).

#### 4. Social Engineering

**Attack**: Attacker tricks user into sharing OTP.

**Example Scenarios**:
- Phishing: "Click this link to verify your document"
- Impersonation: "Hi, I'm from support, can you share your document link?"
- Public posting: User accidentally posts OTP link in public channel

**Mitigations**:
- User education
- UI warnings when enabling OTP
- Clear documentation on safe sharing practices

**No Technical Solution**: Social engineering attacks bypass technical controls.

#### 5. Cross-Site Request Forgery (CSRF)

**Attack**: Malicious website triggers OTP enable/disable on behalf of authenticated user.

**Current Vulnerability**:
```html
<!-- Attacker's malicious page -->
<script>
fetch('https://kolabpad.example.com/api/document/victim-doc/protect', {
    method: 'POST',
    credentials: 'include',  // Include user's session
    body: JSON.stringify({ user_id: 0, user_name: 'Attacker' })
});
</script>
```

**Partial Mitigation**: Requires user to have active WebSocket connection to document.

**Full Mitigation** (not implemented): CSRF tokens in API requests.

---

## Memory Safety

Kolabpad is written in Go, which provides memory safety guarantees absent in languages like C/C++.

### Go Language Benefits

**Automatic Memory Management**:
- Garbage collection prevents use-after-free
- No manual `malloc`/`free`
- No buffer overflows from pointer arithmetic

**Type Safety**:
- Strong static typing prevents type confusion
- No implicit pointer casts
- Compiler catches many bugs at compile time

**Array Bounds Checking**:
- Runtime panics on out-of-bounds access
- Cannot read/write beyond slice boundaries

**Safe String Handling**:
- Strings are immutable
- No null-terminated string issues
- UTF-8 handling built-in

### Concurrency Safety

Kolabpad uses goroutines and channels extensively. Proper synchronization is critical.

**Thread-Safe Data Structures**:
```go
// Active documents stored in sync.Map (thread-safe)
type State struct {
    documents *sync.Map
    db        *database.Database
}

// Each document has internal mutex
type Kolabpad struct {
    state *State
    mu    sync.RWMutex  // Protects state access
}
```

**Mutex Patterns**:
- Read locks for queries (`RLock()`)
- Write locks for modifications (`Lock()`)
- Deferred unlocks to prevent deadlocks: `defer mu.Unlock()`

**Atomic Operations**:
- Revision counters use atomic increment
- Timestamp updates use atomic store

**Goroutine Lifecycle**:
- Persister goroutines: 1 per active document
- WebSocket goroutines: 1 per connection
- Proper cleanup on disconnect (channels closed, goroutines exit)

### Race Condition Detection

**Development Tool**: `go run -race`
- Detects data races at runtime
- Should be run during testing

**Production**: Race detector not enabled (performance overhead)

**Known Race-Free Sections**:
- Document state access (mutex-protected)
- Broadcast distribution (channel-based)
- Database writes (single writer goroutine per document)

### Potential Memory Leaks

**Document Lifecycle**: Documents remain in memory until eviction (24h idle).

**Monitoring**:
- `documents_in_memory` metric
- `memory_usage_mb` metric
- Goroutine count

**Eviction Strategy** (from persistence spec):
- Documents idle for 24 hours: Flushed and removed from memory
- Documents with no users for extended time: Prioritized for eviction
- Graceful shutdown: All documents flushed

**Memory Growth Scenario**:
```
Problem: 10,000 documents created but never accessed again
Result: 10,000 × 250KB = 2.5GB memory usage
Solution: 24-hour eviction removes stale documents
```

---

## Denial of Service

DoS attacks aim to make Kolabpad unavailable to legitimate users.

### Mitigated Attack Vectors

**✅ OTP Guessing DoS** (Mitigated)
- **Attack**: Repeatedly connect with wrong OTP to load documents
- **Protection**: Dual-check pattern (see [01-authentication-model.md](01-authentication-model.md))
- **Result**: Invalid OTP rejected without loading document

**✅ Large Message DoS** (Mitigated)
- **Attack**: Send multi-gigabyte WebSocket messages
- **Protection**: 25KB message size limit
- **Result**: Oversized messages rejected

**✅ Slow Client DoS** (Mitigated)
- **Attack**: Connect but never read messages (buffer exhaustion)
- **Protection**: WebSocket write buffer limit (8KB)
- **Result**: Slow clients disconnected automatically

**✅ Malformed Message DoS** (Mitigated)
- **Attack**: Send invalid JSON to crash parser
- **Protection**: Safe JSON parsing with error handling
- **Result**: Malformed messages logged and ignored

### NOT Fully Mitigated

**❌ Connection Flood DoS**
- **Attack**: Open thousands of WebSocket connections
- **Current State**: No connection limit per IP
- **Impact**: Memory exhaustion, file descriptor exhaustion
- **Mitigation Needed**: Rate limiting at load balancer or edge protection

**❌ Rapid OTP Change DoS**
- **Attack**: Repeatedly enable/disable OTP (forces database writes)
- **Current State**: No rate limiting on `/api/document/{id}/protect` endpoints
- **Impact**: Database I/O exhaustion, disk space exhaustion (logs)
- **Mitigation Needed**: Rate limiting (e.g., 5 requests per minute per IP)

**❌ Document Creation DoS**
- **Attack**: Create thousands of new documents
- **Current State**: No limit on document creation
- **Impact**: Database growth, memory usage (if accessed)
- **Mitigation**: Document expiration (7 days) provides eventual cleanup

**❌ Bandwidth Exhaustion**
- **Attack**: Many connections sending/receiving continuous edits
- **Current State**: No bandwidth limits
- **Impact**: Network saturation
- **Mitigation Needed**: Edge protection (AWS Shield, Cloudflare)

### Recommended DoS Protections

**1. Edge Protection** (Highest Priority)
```
Deploy behind:
- Cloudflare (DDoS protection, rate limiting, caching)
- AWS Shield (DDoS protection)
- Google Cloud Armor
```

**2. Load Balancer Rate Limiting**
```nginx
# Nginx example
limit_req_zone $binary_remote_addr zone=protect_api:10m rate=5r/m;

location /api/document {
    limit_req zone=protect_api burst=2;
}
```

**3. Connection Limits**
```go
// Limit concurrent connections per IP
maxConnectionsPerIP := 10

if connectionCount[remoteIP] >= maxConnectionsPerIP {
    http.Error(w, "Too many connections", http.StatusTooManyRequests)
    return
}
```

**4. Monitoring and Alerting**
- Alert if `websocket_connections` > threshold
- Alert if `persist_errors` > 0 (database issues)
- Alert if `memory_usage_mb` growing unbounded

---

## What NOT to Use Kolabpad For

Kolabpad is designed for **ephemeral, short-term collaboration**. It is NOT suitable for many common use cases.

### Prohibited Use Cases

**❌ Medical Records (HIPAA)**
- **Why Not**: No encryption at rest, no access controls, no audit logs
- **Regulations**: HIPAA requires patient data protection, access logs, breach notification
- **Alternative**: Use HIPAA-compliant platforms (Epic, Cerner, Athenahealth)

**❌ Financial Data (PCI-DSS)**
- **Why Not**: No encryption, no PCI compliance, data loss acceptable
- **Regulations**: PCI-DSS requires cardholder data encryption, access controls, logging
- **Alternative**: Use PCI-compliant payment processors (Stripe, Square, PayPal)

**❌ Legal Documents**
- **Why Not**: Data loss possible (5min window), no audit trail, no version history
- **Requirements**: Legal documents need immutability, version control, audit trail
- **Alternative**: Use document management systems (NetDocuments, iManage)

**❌ Personally Identifiable Information (GDPR)**
- **Why Not**: No data subject rights (right to erasure, right to access), no consent management
- **Regulations**: GDPR requires data protection, consent, right to be forgotten
- **Alternative**: Use GDPR-compliant platforms with proper data controls

**❌ Long-Term Storage**
- **Why Not**: Documents expire after 7 days, memory-first architecture not optimized for cold storage
- **Requirements**: Long-term storage needs durability guarantees, backup/recovery
- **Alternative**: Use proper databases, cloud storage (S3, Google Drive, Dropbox)

**❌ Passwords, API Keys, Secrets**
- **Why Not**: No encryption at rest, OTP can leak, plain text in database
- **Requirements**: Secrets need encryption, access controls, rotation, auditing
- **Alternative**: Use secret management tools (HashiCorp Vault, AWS Secrets Manager, 1Password)

**❌ High-Availability Requirements**
- **Why Not**: Single-server architecture, no redundancy, acceptable crash-related data loss
- **Requirements**: Mission-critical systems need 99.99% uptime, redundancy, failover
- **Alternative**: Use distributed systems with proper HA design

### Acceptable Use Cases

**✅ Code Snippet Sharing**
- Quick sharing during pair programming
- Pasting error messages for debugging
- Sharing configuration examples

**✅ Collaborative Troubleshooting**
- Real-time note-taking during incidents
- Collecting diagnostic output
- Team brainstorming (non-sensitive)

**✅ Development Environment Sharing**
- Sharing `.env` files within team (development, not production)
- Sharing configuration snippets
- Temporary code collaboration

**✅ Meeting Notes (Non-Sensitive)**
- Collaborative note-taking during meetings
- Action item lists
- Brainstorming sessions

**✅ Temporary Data Sharing**
- Sharing data that's public or low-sensitivity
- Time-bound collaboration (< 7 days)
- Data that can afford 5min loss on crash

### Decision Matrix

| Use Case | Kolabpad? | Reason |
|----------|-----------|--------|
| Quick code snippets | ✅ Yes | Ephemeral, low sensitivity |
| `.env` files (dev) | ✅ Yes | Short-term, team-internal |
| `.env` files (prod) | ❌ No | Secrets require proper management |
| Meeting notes (public) | ✅ Yes | Non-sensitive, temporary |
| Meeting notes (confidential) | ❌ No | No access controls |
| Incident response notes | ✅ Yes | Real-time collaboration needed |
| Legal contracts | ❌ No | Requires audit trail |
| Patient data | ❌ No | HIPAA compliance required |
| Credit card numbers | ❌ No | PCI-DSS compliance required |
| API documentation | ✅ Yes | Non-sensitive, collaborative |
| Source code (open-source) | ✅ Yes | Public data, temporary sharing |
| Source code (proprietary) | ⚠️ Maybe | Use OTP protection, assess risk |

---

## Future Security Improvements

These enhancements would improve Kolabpad's security posture but are not currently implemented.

### High Priority

**1. Rate Limiting**
- **Goal**: Prevent brute force and DoS attacks
- **Implementation**: Per-IP rate limits on all endpoints
- **Example**: 5 requests/min for protect/unprotect, 100 connections/hour per IP

**2. CSRF Protection**
- **Goal**: Prevent cross-site request forgery
- **Implementation**: CSRF tokens in cookies, validated on state-changing requests
- **Affected Endpoints**: `/api/document/{id}/protect`, `/api/document/{id}/protect` (DELETE)

**3. Encryption at Rest**
- **Goal**: Protect data if database file stolen
- **Implementation**: SQLCipher or SQLite Encryption Extension
- **Trade-off**: ~10% performance overhead, key management complexity

### Medium Priority

**4. User Accounts and Authentication**
- **Goal**: Persistent identity, document ownership
- **Implementation**: Session-based or JWT authentication
- **Enables**: Fine-grained permissions, audit trails, owner-based access

**5. OTP Expiration**
- **Goal**: Limit blast radius of leaked OTP
- **Implementation**: Optional expiration time (e.g., 24 hours)
- **User Experience**: Must re-enable OTP after expiration

**6. Audit Logging**
- **Goal**: Track all security-relevant actions
- **Implementation**: Structured logs for document access, OTP changes, failed auth
- **Example**: `{"event": "otp_validation_failed", "documentId": "abc123", "ip": "1.2.3.4"}`

**7. Content Security Policy (CSP)**
- **Goal**: Prevent XSS attacks
- **Implementation**: CSP headers restricting script sources
- **Example**: `Content-Security-Policy: default-src 'self'; script-src 'self'`

### Low Priority (Nice to Have)

**8. Document Permissions (Owner, Editor, Viewer)**
- **Requires**: User accounts + authentication
- **Enables**: Read-only sharing, admin roles

**9. OTP Rotation**
- **Goal**: Invalidate old OTPs, generate new ones
- **Use Case**: Suspect OTP leaked, want to re-secure document

**10. Two-Factor Authentication**
- **Requires**: User accounts + phone/authenticator app
- **Overkill for**: Ephemeral collaboration use case

**11. End-to-End Encryption**
- **Goal**: Server cannot read document content
- **Complexity**: Key exchange, client-side encryption/decryption
- **Trade-off**: Server cannot perform search, server-side OT becomes difficult

**12. IP Whitelisting**
- **Goal**: Restrict document access to specific IPs
- **Use Case**: Corporate VPN-only access
- **Implementation**: Per-document IP whitelist

---

## Summary

Kolabpad's security model is **appropriate for ephemeral, low-sensitivity collaboration** but unsuitable for regulated, high-security, or long-term data storage use cases.

### Security Strengths
- ✅ OTP-based access control
- ✅ Cryptographically secure token generation
- ✅ DoS-resistant dual-check pattern
- ✅ Server-authoritative OT (no client tampering)
- ✅ Memory-safe Go implementation
- ✅ Audit trail with userId attribution

### Security Limitations
- ❌ No rate limiting (DDoS vulnerable)
- ❌ No encryption at rest
- ❌ No CSRF protection
- ❌ OTPs visible in URLs
- ❌ No user accounts or fine-grained permissions
- ❌ Data loss possible (5min window on crash)

### Deployment Requirements
- 🔒 **MUST**: Deploy behind HTTPS (TLS termination)
- 🔒 **MUST**: Secure file system permissions on database
- 🔒 **RECOMMENDED**: Edge DDoS protection (Cloudflare, AWS Shield)
- 🔒 **RECOMMENDED**: Rate limiting at load balancer
- 🔒 **RECOMMENDED**: Encrypted backups

### Use Case Fit
- ✅ **Good for**: Code snippets, dev collaboration, meeting notes, temporary sharing
- ❌ **Bad for**: HIPAA/PCI data, legal docs, secrets, long-term storage, mission-critical systems

---

## Related Documentation

- **Authentication Model**: [01-authentication-model.md](01-authentication-model.md) - OTP implementation details
- **WebSocket Protocol**: [../protocol/01-websocket-protocol.md](../protocol/01-websocket-protocol.md) - WebSocket message format and connection flow
- **REST API**: [../protocol/02-rest-api.md](../protocol/02-rest-api.md) - HTTP endpoints and security patterns
- **Persistence Strategy**: [../architecture/03-persistence-strategy.md](../architecture/03-persistence-strategy.md) - Why data loss is acceptable
- **Deployment Guide**: [../development/03-deployment-monitoring.md](../development/03-deployment-monitoring.md) - Secure deployment practices
- **System Overview**: [../architecture/01-system-overview.md](../architecture/01-system-overview.md) - High-level architecture

---

**Remember**: Kolabpad prioritizes simplicity and real-time performance over comprehensive security. Deploy accordingly, and educate users on appropriate use cases.
