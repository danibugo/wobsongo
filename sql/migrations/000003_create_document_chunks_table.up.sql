CREATE TABLE IF NOT EXISTS document_chunks (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    sequence_number INTEGER NOT NULL,
    topics TEXT[] NOT NULL DEFAULT '{}',
    factuality_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    text TEXT NOT NULL DEFAULT '',
    page INTEGER NOT NULL DEFAULT 0,
    chapter TEXT NOT NULL DEFAULT '',
    layout_type TEXT NOT NULL,
    bounding_box DOUBLE PRECISION[] NOT NULL DEFAULT '{}',
    asset_url TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_document_chunks_document_sequence
    ON document_chunks(document_id, sequence_number);
CREATE INDEX IF NOT EXISTS idx_document_chunks_document_id ON document_chunks(document_id);
