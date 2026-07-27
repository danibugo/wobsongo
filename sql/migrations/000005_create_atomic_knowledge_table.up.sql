-- knowledge_extracted_at is nullable: NULL means "extraction not yet
-- attempted for this chunk". Set once extraction runs, even if it found
-- zero facts -- distinguishes "not yet processed" from "processed, found
-- nothing", mirroring the embedding IS NULL sentinel pattern.
ALTER TABLE document_chunks ADD COLUMN knowledge_extracted_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS atomic_knowledge (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    document_chunk_id UUID NOT NULL REFERENCES document_chunks(id) ON DELETE CASCADE,
    truth_tier INTEGER NOT NULL,
    topics TEXT[] NOT NULL DEFAULT '{}',
    subject TEXT NOT NULL,
    predicate TEXT NOT NULL,
    object TEXT NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    marked_as_invalid BOOLEAN NOT NULL DEFAULT false,
    marked_as_irrelevant BOOLEAN NOT NULL DEFAULT false
);

CREATE INDEX IF NOT EXISTS idx_atomic_knowledge_document_id ON atomic_knowledge(document_id);
CREATE INDEX IF NOT EXISTS idx_atomic_knowledge_document_chunk_id ON atomic_knowledge(document_chunk_id);
