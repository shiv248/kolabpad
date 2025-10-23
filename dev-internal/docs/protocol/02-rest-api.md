# REST API

**Purpose**: Document HTTP REST API endpoints for administrative operations in Kolabpad.

**Audience**: Developers integrating with Kolabpad, implementing clients, or building admin tools.

---

## Table of Contents

1. [API Overview](#api-overview)
2. [Endpoint: POST /api/document/{id}/protect](#endpoint-post-apidocumentidprotect)
3. [Endpoint: DELETE /api/document/{id}/protect](#endpoint-delete-apidocumentidprotect)
4. [Endpoint: GET /api/stats](#endpoint-get-apistats)
5. [Endpoint: GET /api/socket/{id}](#endpoint-get-apisocketid)
6. [Why OTP Uses REST, Not WebSocket](#why-otp-uses-rest-not-websocket)
7. [Error Handling](#error-handling)
8. [Security Considerations](#security-considerations)

---

## API Overview

Kolabpad exposes a REST API for administrative and security operations that don't require real-time communication.

**Design Principles**:
- **REST for admin operations**: OTP protection, stats, document retrieval
- **WebSocket for collaboration**: Real-time editing, cursor updates, presence
- **Hybrid approach**: REST triggers actions, WebSocket broadcasts updates

**Base URL**:
- Development: `http://localhost:3030`
- Production: `https://your-domain.com`

**Content Type**:
- Request: `application/json`
- Response: `application/json` or `text/plain` (depending on endpoint)

**Authentication**:
- Currently: None (future: JWT/session-based auth)
- OTP protection: Requires current OTP to modify protection status

---

## Endpoint: POST /api/document/{id}/protect

**Purpose**: Enable OTP (One-Time Password) protection for a document.

### Request

**HTTP Method**: `POST`

**URL**: `/api/document/{id}/protect`

**Path Parameters**:
- `{id}` (string): Document ID

**Request Body**:
```json
{
  "user_id": 1,
  "user_name": "Alice"
}
```

**Fields**:
- `user_id` (integer, required): User ID of the person enabling protection
- `user_name` (string, required): Display name of the user (for audit trail)

**Example**:
```http
POST /api/document/abc123/protect HTTP/1.1
Host: localhost:3030
Content-Type: application/json

{
  "user_id": 1,
  "user_name": "Alice"
}
```

### Response

**Success (200 OK)**:
```json
{
  "otp": "xyz789"
}
```

**Fields**:
- `otp` (string): Generated 6-character alphanumeric token

**Example**:
```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "otp": "abc123"
}
```

### Behavior

**What Happens**:

```pseudocode
1. Validate user is connected to the document
   IF user not connected:
       RETURN 403 Forbidden

2. Generate random 6-character OTP token
   otp = generateOTP()  // e.g., "abc123"

3. Write to database FIRST (atomicity)
   IF document exists in DB:
       UPDATE documents SET otp = {otp} WHERE id = {id}
   ELSE:
       INSERT INTO documents (id, otp) VALUES ({id}, {otp})

   IF database write fails:
       RETURN 500 Internal Server Error
       // Do NOT update memory or broadcast

4. Update in-memory document state
   document.setOTP(otp)

5. Broadcast to all connected clients via WebSocket
   BROADCAST OTPMsg {
       otp: otp,
       user_id: user_id,
       user_name: user_name
   }

6. Return OTP to caller
   RETURN { otp: otp }
```

**Critical Ordering**:
- Database write happens BEFORE memory update
- Prevents memory/DB desync on failure
- If DB fails, memory is NOT updated

**Why User Validation**:
- Prevents random users from protecting arbitrary documents
- Must be actively connected to enable protection
- Prevents DoS: protect many documents to force DB writes

### Client Usage

**TypeScript Example**:
```typescript
import { protectDocument } from './api/documents';

const { otp } = await protectDocument('abc123', userId, userName);

// Update URL with OTP
window.location.hash = `abc123?otp=${otp}`;

// User sees toast: "OTP protection enabled"
// Other users see toast: "{userName} enabled OTP protection"
```

**Workflow**:
1. User clicks "Enable Protection" in UI
2. Frontend calls `POST /api/document/{id}/protect`
3. Server generates OTP, stores in DB
4. Server broadcasts OTP to all clients via WebSocket
5. All clients update their URLs with `?otp={token}`

---

## Endpoint: DELETE /api/document/{id}/protect

**Purpose**: Disable OTP protection for a document.

### Request

**HTTP Method**: `DELETE`

**URL**: `/api/document/{id}/protect`

**Path Parameters**:
- `{id}` (string): Document ID

**Request Body**:
```json
{
  "user_id": 1,
  "user_name": "Alice",
  "otp": "abc123"
}
```

**Fields**:
- `user_id` (integer, required): User ID
- `user_name` (string, required): Display name
- `otp` (string, required): **Current OTP token** (for authorization)

**Example**:
```http
DELETE /api/document/abc123/protect HTTP/1.1
Host: localhost:3030
Content-Type: application/json

{
  "user_id": 1,
  "user_name": "Alice",
  "otp": "abc123"
}
```

### Response

**Success (204 No Content)**:
```http
HTTP/1.1 204 No Content
```

**Error (403 Forbidden)**:
```json
{
  "error": "Forbidden: invalid OTP"
}
```

### Behavior

**What Happens**:

```pseudocode
1. Validate user is connected to the document
   IF user not connected:
       RETURN 403 Forbidden

2. Validate provided OTP matches current OTP (SECURITY CRITICAL)
   currentOTP = document.getOTP()

   IF currentOTP is null:
       RETURN 400 Bad Request  // Document not protected

   IF providedOTP != currentOTP:
       RETURN 403 Forbidden  // Invalid OTP

3. Write to database FIRST (atomicity)
   UPDATE documents SET otp = NULL WHERE id = {id}

   IF database write fails:
       RETURN 500 Internal Server Error
       // Do NOT update memory or broadcast

4. Update in-memory document state
   document.setOTP(null)

5. Broadcast to all connected clients via WebSocket
   BROADCAST OTPMsg {
       otp: null,
       user_id: user_id,
       user_name: user_name
   }

6. Return success
   RETURN 204 No Content
```

**Why OTP Validation Required**:
- **Security**: Prevents anyone who knows the document ID from disabling protection
- **Authorization**: Only users with current OTP can change protection
- **Audit trail**: User ID and name tracked for who made the change

### Client Usage

**TypeScript Example**:
```typescript
import { unprotectDocument } from './api/documents';

const currentOtp = getOtpFromUrl();  // Extract from ?otp= query param

await unprotectDocument('abc123', userId, userName, currentOtp);

// Update URL (remove OTP)
window.location.hash = 'abc123';

// User sees toast: "OTP protection disabled"
// Other users see toast: "{userName} disabled OTP protection"
```

---

## Endpoint: GET /api/stats

**Purpose**: Retrieve server statistics and health metrics.

### Request

**HTTP Method**: `GET`

**URL**: `/api/stats`

**Query Parameters**: None

**Example**:
```http
GET /api/stats HTTP/1.1
Host: localhost:3030
```

### Response

**Success (200 OK)**:
```json
{
  "start_time": 1704067200,
  "num_documents": 5,
  "database_size": 12
}
```

**Fields**:
- `start_time` (integer): Unix timestamp when server started
- `num_documents` (integer): Number of active documents in memory
- `database_size` (integer): Total documents in database

**Example**:
```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "start_time": 1704067200,
  "num_documents": 5,
  "database_size": 12
}
```

### Behavior

**What Happens**:

```pseudocode
1. Count active documents in memory
   numDocs = 0
   FOR EACH document IN activeDocuments:
       numDocs++

2. Count documents in database
   dbSize = database.count()

3. Return stats
   RETURN {
       start_time: serverStartTime.unix(),
       num_documents: numDocs,
       database_size: dbSize
   }
```

**Use Cases**:
- Monitoring dashboards (Grafana, Datadog, etc.)
- Health checks (Kubernetes liveness probes)
- Operational visibility
- Capacity planning

### Client Usage

**cURL Example**:
```bash
curl http://localhost:3030/api/stats

# Output:
# {
#   "start_time": 1704067200,
#   "num_documents": 5,
#   "database_size": 12
# }
```

**Monitoring Script**:
```bash
#!/bin/bash
# Check if server has too many active documents

stats=$(curl -s http://localhost:3030/api/stats)
num_docs=$(echo $stats | jq .num_documents)

if [ $num_docs -gt 1000 ]; then
    echo "WARNING: $num_docs active documents (threshold: 1000)"
    # Send alert
fi
```

---

## Endpoint: GET /api/socket/{id}

**Purpose**: WebSocket upgrade endpoint for real-time collaboration.

### Request

**HTTP Method**: `GET` (upgrades to WebSocket)

**URL**: `/api/socket/{id}?otp={token}`

**Path Parameters**:
- `{id}` (string): Document ID

**Query Parameters**:
- `otp` (string, optional): OTP token if document is protected

**Headers**:
```http
Connection: Upgrade
Upgrade: websocket
Sec-WebSocket-Key: {base64-key}
Sec-WebSocket-Version: 13
```

**Example**:
```http
GET /api/socket/abc123?otp=xyz789 HTTP/1.1
Host: localhost:3030
Connection: Upgrade
Upgrade: websocket
Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==
Sec-WebSocket-Version: 13
```

### Response

**Success (101 Switching Protocols)**:
```http
HTTP/1.1 101 Switching Protocols
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Accept: {base64-accept-key}
```

**Then**: WebSocket protocol begins (see `protocol/01-websocket-protocol.md`)

**Unauthorized (401)**:
```http
HTTP/1.1 401 Unauthorized
Content-Type: text/plain

Invalid or missing OTP
```

### Behavior

**OTP Validation (Dual-Check Pattern)**:

```pseudocode
providedOTP = query parameter "otp"

// Fast path: Document in memory (hot)
IF document = activeDocuments.get(documentId):
    actualOTP = document.getOTP()

    IF actualOTP exists AND actualOTP != providedOTP:
        RETURN 401 Unauthorized  // Reject immediately (no DB read)

    // Valid or no OTP - accept connection
    ACCEPT WebSocket

// Slow path: Document not in memory (cold)
ELSE:
    // CRITICAL: Check OTP BEFORE loading document into memory
    persistedDoc = database.loadOTPOnly(documentId)

    IF persistedDoc.otp exists AND persistedDoc.otp != providedOTP:
        RETURN 401 Unauthorized  // Don't load into memory!

    // Valid or no OTP - safe to load document
    document = getOrCreateDocument(documentId)
    ACCEPT WebSocket
```

**Why Dual-Check**:
- **Hot path**: Fast rejection from memory (no DB read)
- **Cold path**: Check OTP BEFORE loading document (prevents DoS)
- **DoS prevention**: Attacker can't force server to load 1000 documents by guessing IDs

**See Also**:
- Full WebSocket protocol: `protocol/01-websocket-protocol.md`
- OTP security details: `security/01-authentication-model.md`

---

## Why OTP Uses REST, Not WebSocket

**Decision**: OTP enable/disable via REST API, broadcast via WebSocket.

**Rationale**:

1. **Security Features Need HTTP Middleware**
   - Future authentication: JWT/session validation
   - Rate limiting: Per-endpoint rate limits
   - CORS: Origin validation
   - CSRF protection: Token validation
   - WebSocket lacks standardized middleware patterns

2. **Administrative vs Collaborative Actions**
   ```
   Collaborative (WebSocket):
   - Real-time editing (Edit messages)
   - Cursor updates (CursorData)
   - User presence (UserInfo)
   - Language changes (SetLanguage)

   Administrative (REST):
   - Enable/disable protection
   - User authentication (future)
   - Document permissions (future)
   - Audit logs (future)
   ```

3. **Separation of Concerns**
   - REST: Request/response for admin operations
   - WebSocket: Real-time updates for all users
   - Clear boundary: "Who can do what"

4. **Audit Trail**
   - REST endpoints easier to log
   - Standard HTTP access logs
   - Request/response tracking
   - Error logging

5. **Backwards Compatibility**
   - Can add authentication to REST endpoints
   - Doesn't break existing WebSocket protocol
   - WebSocket remains stateless collaboration channel

**Pattern for Future Features**:
```
Admin action (requires auth):
  → REST API trigger (with auth check)
  → Database update
  → WebSocket broadcast (notify all users)

Collaborative action (no auth needed):
  → WebSocket message
  → In-memory update
  → WebSocket broadcast
```

**Example**:
```pseudocode
// Enable OTP protection

Client:
    POST /api/document/{id}/protect
        → Requires: User connected to document
        → Future: Requires authentication token

Server:
    1. Validate user (connected)
    2. Generate OTP
    3. Write to DB
    4. Update memory
    5. BROADCAST via WebSocket to all clients

All Clients:
    Receive OTP broadcast via WebSocket
    Update UI and URL
```

---

## Error Handling

### HTTP Status Codes

**200 OK**: Request succeeded
```json
{
  "otp": "abc123"
}
```

**204 No Content**: Request succeeded, no response body
```http
HTTP/1.1 204 No Content
```

**400 Bad Request**: Invalid request parameters
```json
{
  "error": "document is not OTP-protected"
}
```

**401 Unauthorized**: Invalid OTP or authentication failure
```json
{
  "error": "Invalid or missing OTP"
}
```

**403 Forbidden**: User not authorized
```json
{
  "error": "Forbidden: not connected to document"
}
```

**404 Not Found**: Endpoint doesn't exist
```json
{
  "error": "invalid endpoint"
}
```

**500 Internal Server Error**: Server-side error
```json
{
  "error": "internal error"
}
```

**503 Service Unavailable**: Database not enabled
```json
{
  "error": "database not enabled"
}
```

### Error Response Format

**Consistent Structure**:
```json
{
  "error": "Human-readable error message"
}
```

**Example Responses**:
```json
// User not connected
{
  "error": "Forbidden: not connected to document"
}

// Invalid OTP
{
  "error": "Forbidden: invalid OTP"
}

// Document not protected
{
  "error": "document is not OTP-protected"
}

// Database error
{
  "error": "internal error"
}
```

### Client Error Handling

**TypeScript Example**:
```typescript
try {
  const { otp } = await protectDocument(docId, userId, userName);
  console.log('Protected with OTP:', otp);
} catch (error) {
  if (error.status === 403) {
    showToast('You must be connected to enable protection');
  } else if (error.status === 500) {
    showToast('Server error. Please try again.');
  } else {
    showToast('Failed to enable protection');
  }
}
```

**Retry Strategy**:
```pseudocode
FUNCTION apiCallWithRetry(endpoint, maxRetries):
    attempts = 0

    WHILE attempts < maxRetries:
        TRY:
            response = fetch(endpoint)

            IF response.status == 500 OR response.status == 503:
                // Server error - retry with backoff
                attempts++
                wait exponentialBackoff(attempts)
                CONTINUE

            IF response.status >= 400:
                // Client error - don't retry
                THROW error

            RETURN response

        CATCH network error:
            attempts++
            wait exponentialBackoff(attempts)

    THROW "Max retries exceeded"
```

---

## Security Considerations

### Current Security

**What We Have**:
- ✅ OTP validation prevents unauthorized access
- ✅ User must be connected to enable/disable protection
- ✅ Current OTP required to disable protection
- ✅ Database writes are atomic (DB-first pattern)

**What We DON'T Have**:
- ❌ Rate limiting (implement at load balancer)
- ❌ User authentication (no login system)
- ❌ CSRF protection (future feature)
- ❌ API keys/tokens (future feature)

### Recommended Deployment Security

**1. HTTPS Required**:
```nginx
# Force HTTPS redirect
server {
    listen 80;
    return 301 https://$host$request_uri;
}
```

**2. CORS Configuration**:
```go
// Restrict origins in production
AllowedOrigins: []string{
    "https://kolabpad.example.com",
}
```

**3. Rate Limiting** (at load balancer):
```nginx
# Limit OTP endpoints to 5 requests per minute per IP
location /api/document/ {
    limit_req zone=otp_limit burst=5;
}
```

**4. Database Permissions**:
```bash
# SQLite file permissions (read/write for app user only)
chmod 600 /app/data/kolabpad.db
chown app:app /app/data/kolabpad.db
```

**5. Firewall Rules**:
```bash
# Only allow HTTP/HTTPS from internet
ufw allow 80/tcp
ufw allow 443/tcp
ufw deny from any to any port 3030  # Block direct access to app
```

### Future Security Enhancements

**1. User Authentication**:
```pseudocode
POST /api/document/{id}/protect
    Authorization: Bearer {jwt-token}

    Validate JWT token
    Extract user ID from token (don't trust client-provided user_id)
    Proceed with protection
```

**2. API Rate Limiting**:
```pseudocode
MIDDLEWARE rateLimit:
    key = clientIP + endpoint
    requests = redis.incr(key, expiry=60s)

    IF requests > threshold:
        RETURN 429 Too Many Requests
```

**3. CSRF Protection**:
```pseudocode
POST /api/document/{id}/protect
    X-CSRF-Token: {token}

    Validate CSRF token matches session
    Proceed with protection
```

**4. Audit Logging**:
```pseudocode
ON protect/unprotect:
    log.audit({
        action: "protect_document",
        user_id: user_id,
        document_id: doc_id,
        timestamp: now(),
        ip_address: client_ip,
        success: true
    })
```

---

## Related Documentation

- **WebSocket Protocol**: See `protocol/01-websocket-protocol.md` for real-time collaboration
- **Authentication Model**: See `security/01-authentication-model.md` for OTP security details
- **Backend Architecture**: See `backend/01-server-architecture.md` for server implementation
- **Security Considerations**: See `security/02-security-considerations.md` for threat model

---

## Implementation References

**Backend**:
- Route registration: `pkg/server/server.go` (see `NewServer` and `handleDocument`)
- OTP endpoints: `pkg/server/server.go` (see `handleProtectDocument` and `handleUnprotectDocument`)
- Stats endpoint: `pkg/server/server.go` (see `handleStats`)

**Frontend**:
- API client: `frontend/src/api/documents.ts`
- Type definitions: `frontend/src/types/api.ts`
- Usage examples: `frontend/src/hooks/useOTPSync.tsx`
