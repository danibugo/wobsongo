-- Bilingual (French/English) support: French must be first-class for the
-- Burkina Faso deployment. The 000009 FTS columns hardcoded
-- to_tsvector('english', ...), which mistokenizes French content and
-- provides zero real signal for French queries against non-French text.
-- language is a plain INTEGER (0=English, 1=French), matching this schema's
-- existing convention for other categorical fields (truth_tier, category)
-- rather than a native Postgres ENUM type.

ALTER TABLE documents ADD COLUMN language INTEGER NOT NULL DEFAULT 0;

ALTER TABLE document_chunks ADD COLUMN language INTEGER NOT NULL DEFAULT 0;
ALTER TABLE document_chunks ADD COLUMN text_translated TEXT;

ALTER TABLE atomic_knowledge ADD COLUMN language INTEGER NOT NULL DEFAULT 0;
ALTER TABLE atomic_knowledge ADD COLUMN search_text_translated TEXT;

-- Drop the old single-language (English-only) generated columns/indexes —
-- nothing should query them once the bilingual columns below exist.
DROP INDEX IF EXISTS document_chunks_text_fts_idx;
ALTER TABLE document_chunks DROP COLUMN IF EXISTS text_fts;

DROP INDEX IF EXISTS atomic_knowledge_fts_idx;
ALTER TABLE atomic_knowledge DROP COLUMN IF EXISTS fts;

-- Bilingual generated tsvector columns: a language=0 (English) row uses
-- `text`/subject+predicate+object+note verbatim for the English column and
-- the translated counterpart for French, and vice versa for language=1
-- (French) rows — so both columns are always meaningful regardless of which
-- language the source content is actually in.
ALTER TABLE document_chunks ADD COLUMN text_fts_en tsvector
    GENERATED ALWAYS AS (
        to_tsvector('english', CASE WHEN language = 0 THEN text ELSE COALESCE(text_translated, '') END)
    ) STORED;
ALTER TABLE document_chunks ADD COLUMN text_fts_fr tsvector
    GENERATED ALWAYS AS (
        to_tsvector('french', CASE WHEN language = 1 THEN text ELSE COALESCE(text_translated, '') END)
    ) STORED;
CREATE INDEX ON document_chunks USING GIN (text_fts_en);
CREATE INDEX ON document_chunks USING GIN (text_fts_fr);

ALTER TABLE atomic_knowledge ADD COLUMN fts_en tsvector
    GENERATED ALWAYS AS (
        to_tsvector('english', CASE WHEN language = 0
            THEN subject || ' ' || predicate || ' ' || object || ' ' || note
            ELSE COALESCE(search_text_translated, '') END)
    ) STORED;
ALTER TABLE atomic_knowledge ADD COLUMN fts_fr tsvector
    GENERATED ALWAYS AS (
        to_tsvector('french', CASE WHEN language = 1
            THEN subject || ' ' || predicate || ' ' || object || ' ' || note
            ELSE COALESCE(search_text_translated, '') END)
    ) STORED;
CREATE INDEX ON atomic_knowledge USING GIN (fts_en);
CREATE INDEX ON atomic_knowledge USING GIN (fts_fr);
