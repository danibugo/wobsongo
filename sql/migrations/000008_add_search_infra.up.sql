-- Hybrid retrieval infra: vector ANN indexes (deferred from 000004/000006
-- until a retrieval feature existed to justify them), full-text search over
-- chunk text and atomic-knowledge subject/predicate/object/note, and trigram
-- fuzzy matching over atomic-knowledge's structured fields.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

ALTER TABLE document_chunks ADD COLUMN text_fts tsvector
    GENERATED ALWAYS AS (to_tsvector('english', text)) STORED;
CREATE INDEX ON document_chunks USING GIN (text_fts);
CREATE INDEX ON document_chunks USING hnsw (embedding vector_cosine_ops);

ALTER TABLE atomic_knowledge ADD COLUMN fts tsvector
    GENERATED ALWAYS AS (
        to_tsvector('english', subject || ' ' || predicate || ' ' || object || ' ' || note)
    ) STORED;
CREATE INDEX ON atomic_knowledge USING GIN (fts);
CREATE INDEX ON atomic_knowledge USING GIN (subject gin_trgm_ops);
CREATE INDEX ON atomic_knowledge USING GIN (predicate gin_trgm_ops);
CREATE INDEX ON atomic_knowledge USING GIN (object gin_trgm_ops);
CREATE INDEX ON atomic_knowledge USING hnsw (embedding vector_cosine_ops);
