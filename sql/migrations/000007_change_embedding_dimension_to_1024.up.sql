-- Switching the default embedding model to BGE-M3 (1024-dim, MIT-licensed)
-- instead of a Gemma-derived model (1536-dim, not OSI-approved). Existing
-- embeddings were produced by the old model and are meaningless at the new
-- dimension, so they're cleared here — EmbedChunksWorker/EmbedKnowledgeWorker
-- will naturally re-embed them (their "needs embedding" queries filter on
-- embedding IS NULL).
UPDATE document_chunks SET embedding = NULL WHERE embedding IS NOT NULL;
ALTER TABLE document_chunks ALTER COLUMN embedding TYPE vector(1024);

UPDATE atomic_knowledge SET embedding = NULL WHERE embedding IS NOT NULL;
ALTER TABLE atomic_knowledge ALTER COLUMN embedding TYPE vector(1024);
