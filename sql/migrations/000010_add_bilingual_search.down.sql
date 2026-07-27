DROP INDEX IF EXISTS atomic_knowledge_fts_fr_idx;
DROP INDEX IF EXISTS atomic_knowledge_fts_en_idx;
ALTER TABLE atomic_knowledge DROP COLUMN IF EXISTS fts_fr;
ALTER TABLE atomic_knowledge DROP COLUMN IF EXISTS fts_en;

DROP INDEX IF EXISTS document_chunks_text_fts_fr_idx;
DROP INDEX IF EXISTS document_chunks_text_fts_en_idx;
ALTER TABLE document_chunks DROP COLUMN IF EXISTS text_fts_fr;
ALTER TABLE document_chunks DROP COLUMN IF EXISTS text_fts_en;

-- Restore the old single-language columns/indexes (matching 000009's up.sql shape).
ALTER TABLE atomic_knowledge ADD COLUMN fts tsvector
    GENERATED ALWAYS AS (
        to_tsvector('english', subject || ' ' || predicate || ' ' || object || ' ' || note)
    ) STORED;
CREATE INDEX ON atomic_knowledge USING GIN (fts);

ALTER TABLE document_chunks ADD COLUMN text_fts tsvector
    GENERATED ALWAYS AS (to_tsvector('english', text)) STORED;
CREATE INDEX ON document_chunks USING GIN (text_fts);

ALTER TABLE atomic_knowledge DROP COLUMN IF EXISTS search_text_translated;
ALTER TABLE atomic_knowledge DROP COLUMN IF EXISTS language;

ALTER TABLE document_chunks DROP COLUMN IF EXISTS text_translated;
ALTER TABLE document_chunks DROP COLUMN IF EXISTS language;

ALTER TABLE documents DROP COLUMN IF EXISTS language;
