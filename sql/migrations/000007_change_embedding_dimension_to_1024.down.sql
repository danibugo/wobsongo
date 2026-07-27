UPDATE document_chunks SET embedding = NULL WHERE embedding IS NOT NULL;
ALTER TABLE document_chunks ALTER COLUMN embedding TYPE vector(1536);

UPDATE atomic_knowledge SET embedding = NULL WHERE embedding IS NOT NULL;
ALTER TABLE atomic_knowledge ALTER COLUMN embedding TYPE vector(1536);
