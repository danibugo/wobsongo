CREATE TABLE IF NOT EXISTS documents (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    modified_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ingested_at TIMESTAMPTZ,
    file_key TEXT NOT NULL,
    sha256 TEXT NOT NULL,
    title TEXT NOT NULL,
    filename TEXT NOT NULL,
    filetype TEXT NOT NULL,
    filesize BIGINT NOT NULL,
    page_count INTEGER NOT NULL,
    publisher_name TEXT NOT NULL DEFAULT '',
    publication_year INTEGER NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_documents_sha256 ON documents(sha256);
CREATE INDEX IF NOT EXISTS idx_documents_created_at ON documents(created_at);
