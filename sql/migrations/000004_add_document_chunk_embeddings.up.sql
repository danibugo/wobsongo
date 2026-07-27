CREATE EXTENSION IF NOT EXISTS vector;

-- embedding is nullable: NULL means "not yet embedded". No ANN index
-- (HNSW/IVFFlat) is created here — deliberately deferred until a retrieval/
-- search feature actually exists to justify one.
ALTER TABLE document_chunks ADD COLUMN embedding vector(1536);
