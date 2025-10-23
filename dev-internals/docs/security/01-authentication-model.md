# OTP Authentication Model

**Purpose**: This document explains Kolabpad's OTP-based authentication system, security improvements, and access control mechanisms.

**Audience**: Developers implementing security features, security auditors, and system administrators.

---

## Table of Contents

1. [Authentication Model Overview](#authentication-model-overview)
2. [OTP Lifecycle](#otp-lifecycle)
3. [Dual-Check OTP Validation Pattern](#dual-check-otp-validation-pattern)
4. [Security Improvements Implemented](#security-improvements-implemented)
5. [OTP Generation and Strength](#otp-generation-and-strength)
6. [What We Intentionally DON'T Do](#what-we-intentionally-dont-do)
7. [Security Best Practices for Users](#security-best-practices-for-users)
8. [Known Limitations](#known-limitations)

---

## Authentication Model Overview

Kolabpad uses a **document-level access control** model based on OTP (One-Time Password) tokens. This design aligns with the project's philosophy of ephemeral, short-term collaboration without the overhead of user account management.

### Core Principles

**No User Accounts**
- Users are identified only within document sessions (assigned userId per connection)
- No registration, login, or password management
- No persistent user identity across sessions

**Document-Level Access Control**
- Each document can optionally be protected with an OTP
- OTP is a server-generated access token shared via URL
- Anyone with correct OTP can access the protected document
- Access is all-or-nothing: no read-only or admin roles

**URL-Based Distribution**
- OTP passed as query parameter: `#doc123?otp=xyz789abc`
- Shared via secure channels (Signal, Slack, email, etc.)
- Visible in browser history and logs

### Design Rationale

**Why OTP instead of user accounts?**
- Simplicity: No authentication infrastructure to maintain
- Ephemeral use case: Documents expire after 7 days
- Low barrier to entry: Users can collaborate immediately
- No password security concerns: Tokens are server-generated

**Trade-offs Accepted**
- No document "ownership" concept
- Anyone with OTP has full access (no fine-grained permissions)
- OTP leakage means document is compromised
- No audit trail of individual user actions (only connection-level tracking)

---

## OTP Lifecycle

The complete lifecycle of an OTP from creation to removal.

### Creating OTP Protection

```pseudocode
USER ACTION: Click "Enable OTP Protection" in sidebar
    ↓
CLIENT: Call POST /api/document/{id}/protect
    body: {
        user_id: <current user's ID>,
        user_name: <current user's name>
    }
    ↓
SERVER:
    1. Validate user is connected to document
    2. Generate cryptographically secure random token (12 characters)
    3. Store OTP in database (IMMEDIATE write, not lazy)
    4. Update in-memory document state
    5. Broadcast OTP message to ALL connected clients
    ↓
SERVER RESPONSE: { otp: "abc123xyz789" }
    ↓
CLIENT:
    1. Receive OTP from API response
    2. Update URL: window.location.hash = "#doc123?otp=abc123xyz789"
    3. Receive broadcast confirmation from WebSocket
    4. Show toast: "OTP protection enabled"
    ↓
OTHER CLIENTS:
    1. Receive OTP broadcast via WebSocket
    2. Update URL with new OTP
    3. Show toast: "Alice enabled OTP protection"
```

**Critical Implementation Detail**: The OTP is written to the database BEFORE updating in-memory state. This ensures that if the server crashes immediately after enabling protection, the document remains protected on restart.

### Using OTP to Access Protected Document

```pseudocode
USER ACTION: Open link https://app.com/#doc123?otp=abc123xyz789
    ↓
CLIENT:
    1. Extract documentId from hash: "doc123"
    2. Extract OTP from query params: "abc123xyz789"
    3. Connect WebSocket: /api/socket/doc123?otp=abc123xyz789
    ↓
SERVER: Dual-check validation (see next section)
    ↓
IF valid OTP or no OTP required:
    Accept WebSocket connection
    Send Identity message (assign userId)
    Send full document state
ELSE IF invalid OTP:
    Reject with HTTP 401 Unauthorized
    Connection never established
```

### Disabling OTP Protection

```pseudocode
USER ACTION: Click "Disable OTP Protection" in sidebar
    ↓
CLIENT: Call DELETE /api/document/{id}/protect
    body: {
        user_id: <current user's ID>,
        user_name: <current user's name>,
        otp: <CURRENT OTP TOKEN>  // Required for authorization!
    }
    ↓
SERVER:
    1. Validate user is connected to document
    2. Validate provided OTP matches current OTP (SECURITY CHECK)
    3. IF invalid OTP: Return 401 Unauthorized
    4. Remove OTP from database (IMMEDIATE write)
    5. Update in-memory state (set OTP to null)
    6. Broadcast OTP message with null value to ALL clients
    ↓
SERVER RESPONSE: 204 No Content
    ↓
CLIENT:
    1. Remove OTP from URL: window.location.hash = "#doc123"
    2. Receive broadcast confirmation from WebSocket
    3. Show toast: "OTP protection disabled"
    ↓
OTHER CLIENTS:
    1. Receive OTP broadcast with null value
    2. Remove OTP from URL
    3. Show toast: "Alice disabled OTP protection"
```

**Critical Security Feature**: Disabling OTP requires providing the current OTP. This prevents unauthorized users from removing protection if they gain temporary access.

### OTP Persistence

OTPs follow the same persistence strategy as document content (see [../architecture/03-persistence-strategy.md](../architecture/03-persistence-strategy.md)), with one critical exception:

**OTP changes are ALWAYS written immediately** (not lazy):
- Enable OTP: Immediate database write
- Disable OTP: Immediate database write
- Regular document edits: Lazy persistence (30s idle or 5min safety net)

**Rationale**: Security state must be durable immediately. If server crashes right after enabling OTP, document must remain protected on restart.

---

## Dual-Check OTP Validation Pattern

The dual-check pattern prevents **Denial of Service (DoS) attacks** where attackers repeatedly try to connect to documents with invalid OTPs, forcing the server to load documents into memory.

### The Problem

```pseudocode
// VULNERABLE APPROACH (what we DON'T do):
FUNCTION validateOTP(documentId, providedOTP):
    document = loadDocumentIntoMemory(documentId)  // Expensive!

    IF document.otp EXISTS AND document.otp != providedOTP:
        REJECT connection
        // But document already loaded into memory!

// ATTACK SCENARIO:
FOR i = 1 TO 10000:
    TRY connect to randomDocumentId with wrong OTP
    → Server loads 10,000 documents into memory
    → Memory exhaustion
    → Server crash
```

### The Solution: Dual-Check Pattern

```pseudocode
FUNCTION validateOTPAndConnect(documentId, providedOTP):
    // FAST PATH: Document already in memory (hot)
    IF document = activeDocuments.get(documentId):
        actualOTP = document.getOTP()  // From memory, instant

        IF actualOTP EXISTS AND actualOTP != providedOTP:
            REJECT "Invalid OTP"
            // No database read needed

        // Valid or no OTP required
        ACCEPT connection

    // SLOW PATH: Document not in memory (cold)
    ELSE:
        // CRITICAL: Check OTP BEFORE loading document
        persistedOTP = database.loadOTPOnly(documentId)  // Lightweight query

        IF persistedOTP EXISTS AND persistedOTP != providedOTP:
            REJECT "Invalid OTP"
            // Document NOT loaded into memory!
            // DoS attack prevented

        // Valid or no OTP - safe to load document
        document = loadDocumentIntoMemory(documentId)
        ACCEPT connection
```

### Implementation Details

The actual implementation in `pkg/server/server.go` performs this check:

```go
// Fast path: document already in memory
if val, ok := s.state.documents.Load(docID); ok {
    doc := val.(*Document)
    if otp := doc.Kolabpad.GetOTP(); otp != nil {
        if providedOTP != *otp {
            http.Error(w, "Invalid or missing OTP", http.StatusUnauthorized)
            return
        }
    }
} else {
    // Slow path: validate from database BEFORE loading
    if s.state.db != nil {
        if persisted, err := s.state.db.Load(docID); err == nil && persisted != nil && persisted.OTP != nil {
            if providedOTP != *persisted.OTP {
                http.Error(w, "Invalid or missing OTP", http.StatusUnauthorized)
                return
            }
        }
    }
}
```

### Performance Impact

**Without dual-check** (vulnerable):
- Attacker connects with wrong OTP to 1000 documents
- Result: 1000 documents loaded into memory (~250MB RAM)
- Cost: 1000 database reads + memory allocation

**With dual-check** (secure):
- Attacker connects with wrong OTP to 1000 documents
- Result: 1000 lightweight OTP-only queries, 0 documents loaded
- Cost: 1000 small database reads, no memory allocation

**Database Query Efficiency**: The OTP-only query reads only the `otp` column, not the full document text. For a 50KB document, this is a 200x reduction in data read.

---

## Security Improvements Implemented

Kolabpad's OTP system has evolved through several security iterations. This section documents known vulnerabilities and their fixes.

### Issue 1: Unauthenticated OTP Modification (Fixed)

**Problem** (pre-commit `c97bc05`):
```pseudocode
// OLD VULNERABLE API:
DELETE /api/document/{id}/protect
    body: {
        user_id: 123,
        user_name: "Alice"
    }

// Anyone could disable OTP without knowing current OTP!
// Attacker could remove protection then access document
```

**Solution** (implemented):
```pseudocode
// NEW SECURE API:
DELETE /api/document/{id}/protect
    body: {
        user_id: 123,
        user_name: "Alice",
        otp: "current-otp-token"  // REQUIRED!
    }

SERVER validates:
    IF provided OTP != stored OTP:
        RETURN 401 Unauthorized
        Log unauthorized attempt
```

**Impact**: Prevents attackers from removing protection without knowing the OTP.

### Issue 2: DoS via OTP Guessing (Fixed)

See [Dual-Check OTP Validation Pattern](#dual-check-otp-validation-pattern) above.

**Solution**: Implemented dual-check pattern to validate OTP before loading document into memory.

### Issue 3: No Audit Trail (Fixed)

**Problem**: When OTP changed, no record of who made the change.

**Solution**: All OTP messages now include attribution:

```json
{
  "OTP": {
    "otp": "abc123xyz789",
    "user_id": 42,
    "user_name": "Alice"
  }
}
```

**Benefits**:
- Client UI can show: "Alice enabled OTP protection" vs "You enabled OTP protection"
- Server logs include userId for security audits
- Can track which user performed administrative actions

### Issue 4: Race Conditions Between Memory and Database (Fixed)

**Problem**: OTP written to memory first, then database. If server crashed between writes, state would be inconsistent.

**Solution**: Database write happens BEFORE in-memory state update. Uses atomic transaction pattern:

```pseudocode
FUNCTION enableOTPProtection(documentId, userId, userName):
    BEGIN TRANSACTION
        1. Generate OTP token
        2. Write to database (commits immediately)
        3. IF database write succeeds:
            Update in-memory state
            Broadcast to clients
        4. ELSE:
            Return error, no state change
    END TRANSACTION
```

---

## OTP Generation and Strength

### Generation Algorithm

OTP tokens are generated using cryptographically secure random number generation:

```go
// From pkg/server/secret.go
func GenerateOTP() string {
    b := make([]byte, 9)
    rand.Read(b) // crypto/rand - cryptographically secure
    return base64.RawURLEncoding.EncodeToString(b)
}
```

### Token Properties

**Format**: URL-safe base64 encoding (no padding)
- Characters: `A-Z`, `a-z`, `0-9`, `-`, `_`
- Length: 12 characters
- Example: `abc123XYZ-_9`

**Entropy**: 9 bytes = 72 bits of entropy
- Possible combinations: 2^72 ≈ 4.7 × 10^21
- Brute force resistance: 4.7 sextillion possibilities

**Collision Probability**: Negligible
- For 1 million documents, probability of collision ≈ 10^-15

### URL-Safe Design

The base64 encoding uses URL-safe characters (RFC 4648):
- `+` replaced with `-`
- `/` replaced with `_`
- No padding `=` characters

This allows OTPs to be safely embedded in URLs without encoding:
```
https://app.com/#doc123?otp=abc123XYZ-_9
                              ↑ URL-safe characters
```

### Why Not More Characters?

**12 characters is sufficient because**:
- Documents are ephemeral (7-day expiration)
- No rate limiting per document (see Known Limitations)
- Collision probability negligible for expected scale
- Longer tokens are harder for users to verify/share

**Not designed for**:
- Long-term access control (use proper authentication)
- High-value targets (no brute-force protection)
- Compliance requirements (HIPAA, PCI-DSS, etc.)

---

## What We Intentionally DON'T Do

Kolabpad's security model makes deliberate trade-offs for simplicity and its ephemeral use case.

### No User Accounts

**Why Not**:
- Complexity: Registration, login, password reset, email verification
- Maintenance: Password policies, account recovery, GDPR compliance
- Friction: Users need to collaborate immediately, not after signing up

**Trade-offs**:
- Can't track document ownership across sessions
- Can't revoke access to specific users
- Can't implement fine-grained permissions
- No persistent reputation or identity

**Mitigation**: OTP provides sufficient access control for ephemeral collaboration.

### No Fine-Grained Permissions

**Why Not**:
- Complexity: Would need user accounts + role system
- Use case: Ephemeral collaboration doesn't need "viewer" vs "editor" roles
- Implementation: OT algorithm assumes all users can edit

**Trade-offs**:
- Can't have read-only users
- Can't have "admin" users who can remove others
- All users with OTP have equal access

**Future Consideration**: Could add read-only mode without authentication (useful for sharing results).

### No Two-Factor Authentication

**Why Not**:
- Overkill for ephemeral document sharing
- Would require user accounts + phone numbers/authenticator apps
- Documents auto-expire after 7 days (limited damage window)

**Trade-offs**:
- OTP alone could be leaked via screenshots, logs, URLs
- No defense against compromised OTP

**Mitigation**: Users should disable OTP when done collaborating.

### No Encryption at Rest

**Why Not**:
- Complexity: Key management, rotation, escrow
- Use case: Documents are not long-term sensitive data
- Performance: Encryption/decryption overhead on every access

**Trade-offs**:
- If attacker gets SQLite file, can read all documents
- OTPs stored in plain text in database

**Mitigation**: Don't use Kolabpad for sensitive data requiring encryption at rest.

### No Session Management

**Why Not**:
- Complexity: Session tokens, expiration, refresh tokens
- Use case: WebSocket connection itself is the "session"
- Stateless: Users can disconnect and reconnect freely

**Trade-offs**:
- No way to invalidate a specific connection
- No way to force re-authentication
- OTP remains valid until manually disabled

---

## Security Best Practices for Users

### When to Use OTP Protection

**Use OTP when**:
- Document contains team-internal information (not public)
- Sharing code snippets with API keys or configuration
- Collaborating on sensitive (but not regulated) content
- Want to prevent accidental public discovery

**Don't bother with OTP for**:
- Public documentation or tutorials
- Open-source code snippets
- Content you'd be comfortable tweeting

### How to Share OTPs Safely

**Good practices**:
✅ Share OTP links via encrypted channels (Signal, WhatsApp, Slack DMs)
✅ Share in private channels, not public forums
✅ Verify recipient before sharing
✅ Disable OTP when collaboration is complete

**Bad practices**:
❌ Posting OTP links in public Slack channels
❌ Tweeting or blogging OTP links
❌ Emailing OTP links to large distribution lists
❌ Screenshotting browser with OTP in URL bar

### OTP Lifecycle Management

**Enable OTP**:
- When first user with sensitive content joins
- Before sharing document link externally
- When transitioning from private to team collaboration

**Disable OTP**:
- When collaboration session is complete
- When document becomes public/non-sensitive
- Before document expiration (if intentionally abandoning)

**Rotate OTP** (enable new OTP):
- If you suspect OTP was leaked
- If unknown users appear in document
- After sharing in potentially compromised channel

### What NOT to Use Kolabpad For

**Never use for**:
- Medical records (HIPAA compliance required)
- Financial data (PCI-DSS compliance required)
- Legal documents (audit and retention requirements)
- Personally identifiable information (GDPR concerns)
- Passwords, API keys, production secrets (use proper secret management)
- Anything that cannot afford 30s-5min data loss on crash

**Good use cases**:
- Sharing `.env` files during development
- Quick code snippets during pair programming
- Collaborative troubleshooting notes
- Meeting notes (non-sensitive)
- Temporary data sharing (< 1 week)

---

## Known Limitations

These are security limitations that are **known and accepted** given Kolabpad's design goals.

### 1. No Rate Limiting

**Limitation**: No rate limiting on OTP validation attempts or protect/unprotect endpoints.

**Risk**:
- Brute force attacks possible (though 2^72 combinations makes this impractical)
- DoS via rapid protect/unprotect API calls
- Resource exhaustion from many simultaneous connection attempts

**Mitigation**:
- Deploy behind edge protection (Cloudflare, AWS Shield)
- Implement rate limiting at load balancer level
- Monitor for suspicious patterns

**Future Work**: Implement per-IP rate limiting (5 requests per minute recommended).

### 2. OTP in URL

**Limitation**: OTP passed as query parameter, visible in browser history and logs.

**Risk**:
- Browser history on shared computers
- Server access logs may capture OTP
- Screenshots with URL bar visible
- Browser sync services may store history

**Mitigation**:
- Educate users on safe sharing practices
- Use incognito/private browsing for sensitive documents
- Disable OTP when done

**Alternative Considered**: Passing OTP in headers, but this breaks URL-based sharing model.

### 3. No CSRF Protection

**Limitation**: REST API endpoints (`/api/document/{id}/protect`, etc.) lack CSRF tokens.

**Risk**: Cross-site request forgery attacks could enable/disable OTP if user is connected to document.

**Mitigation**:
- Requires user to be actively connected (WebSocket) to document
- Attacker must know documentId

**Future Work**: Add CSRF token validation for state-changing requests.

### 4. Plain Text Storage

**Limitation**: OTPs stored in database as plain text (SQLite `otp` column).

**Risk**: If attacker gains access to SQLite file, all OTPs are compromised.

**Mitigation**:
- File system permissions on database file
- Encrypted disk storage
- Don't use Kolabpad for compliance-requiring data

**Alternative Considered**: Hashing OTPs, but this breaks ability to broadcast OTP to clients.

### 5. No OTP Expiration

**Limitation**: OTPs remain valid until manually disabled or document expires (7 days).

**Risk**: Leaked OTP remains valid for full document lifetime.

**Mitigation**:
- Users should disable OTP when done
- Documents auto-expire after 7 days

**Future Work**: Add optional OTP expiration time (e.g., 24 hours).

---

## Related Documentation

- **Persistence Strategy**: [../architecture/03-persistence-strategy.md](../architecture/03-persistence-strategy.md) - Why OTP writes are immediate, not lazy
- **Security Considerations**: [02-security-considerations.md](02-security-considerations.md) - Threat model and broader security context
- **Broadcast System**: [../backend/02-broadcast-system.md](../backend/02-broadcast-system.md) - How OTP changes are broadcast to clients
- **WebSocket Protocol**: [../protocol/01-websocket-protocol.md](../protocol/01-websocket-protocol.md) - WebSocket connection flow and OTP validation
- **REST API**: [../protocol/02-rest-api.md](../protocol/02-rest-api.md) - API endpoints for OTP management

---

## Summary

Kolabpad's OTP authentication model provides **document-level access control** suitable for ephemeral, short-term collaboration:

**Strengths**:
- ✅ Simple to use (no user accounts)
- ✅ Cryptographically secure token generation
- ✅ DoS-resistant (dual-check pattern)
- ✅ Audit trail (userId attribution)
- ✅ Immediate persistence (survives crashes)

**Limitations**:
- ❌ No rate limiting
- ❌ OTP visible in URLs and logs
- ❌ No fine-grained permissions
- ❌ Plain text storage
- ❌ No CSRF protection

**Best For**: Internal team collaboration, code sharing, quick editing sessions (< 1 week).

**Not For**: Sensitive regulated data, long-term storage, compliance requirements.
