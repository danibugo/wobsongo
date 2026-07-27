-- name: GetAtomicKnowledgeByID :one
SELECT * FROM atomic_knowledge WHERE id = $1;

-- name: CreateAtomicKnowledgeBatch :copyfrom
INSERT INTO atomic_knowledge (
    id, created_at, updated_at, document_id, document_chunk_id, truth_tier,
    topics, subject, predicate, object, note, marked_as_invalid, marked_as_irrelevant,
    category, language, search_text_translated
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
);

-- name: ListKnowledgeNeedingEmbedding :many
SELECT * FROM atomic_knowledge
WHERE document_id = $1 AND embedding IS NULL
ORDER BY created_at ASC;

-- name: UpdateAtomicKnowledgeEmbedding :exec
UPDATE atomic_knowledge SET embedding = $2, updated_at = $3 WHERE id = $1;
