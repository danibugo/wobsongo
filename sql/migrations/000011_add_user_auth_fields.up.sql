BEGIN;

-- The original users table (000001_init_schema) is vestigial: SERIAL id,
-- only email/created_at, no generated queries, no FK references from any
-- other table. Recreated UUID-keyed for consistency with documents/
-- atomic_knowledge, with the columns the web layer's auth actually needs.
DROP TABLE IF EXISTS users;

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    email VARCHAR(255) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    password_hash TEXT NOT NULL,
    role SMALLINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMIT;
