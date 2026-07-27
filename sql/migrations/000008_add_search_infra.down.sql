DROP INDEX IF EXISTS document_chunks_text_fts_idx;
DROP INDEX IF EXISTS document_chunks_embedding_idx;
ALTER TABLE document_chunks DROP COLUMN IF EXISTS text_fts;

DROP INDEX IF EXISTS atomic_knowledge_fts_idx;
DROP INDEX IF EXISTS atomic_knowledge_subject_idx;
DROP INDEX IF EXISTS atomic_knowledge_predicate_idx;
DROP INDEX IF EXISTS atomic_knowledge_object_idx;
DROP INDEX IF EXISTS atomic_knowledge_embedding_idx;
ALTER TABLE atomic_knowledge DROP COLUMN IF EXISTS fts;

DROP EXTENSION IF EXISTS pg_trgm;
