-- embedding is nullable: NULL means "not yet embedded". No ANN index
-- (HNSW/IVFFlat) is created here — deliberately deferred until a retrieval/
-- search feature actually exists to justify one. The vector extension is
-- already installed (migration 000004).
ALTER TABLE atomic_knowledge ADD COLUMN embedding vector(1536);
