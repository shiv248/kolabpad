# Database Migrations

This directory contains SQL migration files for the Kolabpad database schema.

## How It Works

- Migrations are automatically applied on server startup in alphabetical order
- Each migration is tracked in the `schema_migrations` table
- Only pending migrations are applied (safe for existing databases)
- Migration files are embedded into the binary at compile time

## Naming Convention

**Format:** `{sequential_number}_{description}.sql`

**Examples:**
```
1_document.sql
2_add_timestamps.sql
3_add_index.sql
4_add_user_table.sql
```

### Rules:
1. **Sequential numbers** - Start at 1, increment by 1 for each new migration
2. **Descriptive name** - Use lowercase with underscores, describe what the migration does
3. **`.sql` extension** - Required for the migration system to find it

## Creating a New Migration

1. Determine the next migration number (count existing files + 1)
2. Create a new `.sql` file with the naming convention
3. Write your SQL migration (can contain multiple statements)
4. Test locally before deploying

**Example:** To add a `created_at` column to documents:

**File:** `2_add_created_at.sql`
```sql
-- Add created_at timestamp to document table
ALTER TABLE document ADD COLUMN created_at INTEGER;

-- Backfill existing documents with current timestamp
UPDATE document SET created_at = strftime('%s', 'now') WHERE created_at IS NULL;
```

## Migration Best Practices

### ✅ Do:
- Use `IF NOT EXISTS` for `CREATE TABLE` statements
- Add comments explaining what the migration does
- Test migrations on a copy of production data
- Make migrations idempotent when possible
- Keep migrations focused (one logical change per file)

### ❌ Don't:
- Skip version numbers
- Modify existing migration files after they've been deployed
- Drop tables or columns without careful consideration
- Use database-specific syntax (stick to standard SQLite)

## Migration Tracking

The `schema_migrations` table tracks applied migrations:

```sql
CREATE TABLE schema_migrations (
    version INTEGER PRIMARY KEY,
    filename TEXT NOT NULL,
    applied_at INTEGER NOT NULL  -- Unix timestamp
)
```

Query to see applied migrations:
```sql
SELECT version, filename, datetime(applied_at, 'unixepoch') as applied
FROM schema_migrations
ORDER BY version;
```

## Current Schema

### Version 1: Initial Schema
- **File:** `1_document.sql`
- **Description:** Creates the `document` table with id, text, language, and otp columns
- **Tables:** `document`
  - `id TEXT PRIMARY KEY` - Unique document identifier
  - `text TEXT NOT NULL` - Document content
  - `language TEXT` - Syntax highlighting language (nullable)
  - `otp TEXT` - One-time password for document protection (nullable, NULL = unprotected)

## Troubleshooting

### Migration fails with "table already exists"
- Use `CREATE TABLE IF NOT EXISTS` or `ALTER TABLE` carefully
- Check if the migration was partially applied

### Need to rollback a migration
- Currently no automatic rollback support
- Manually write a new migration to undo changes
- Or restore from backup and reapply correct migrations

### Fresh database vs existing database
- Fresh databases get all migrations in order
- Existing databases only get new migrations since last version
- Both end up with identical schema
