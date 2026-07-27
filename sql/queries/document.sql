-- name: GetDocumentByID :one
SELECT * FROM documents WHERE id = $1;

-- name: GetDocumentBySHA256 :one
SELECT * FROM documents WHERE sha256 = $1;

-- name: CreateDocument :one
INSERT INTO documents (
    id, created_at, modified_at, ingested_at, file_key, sha256,
    title, filename, filetype, filesize, page_count, publisher_name, publication_year,
    language
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
) RETURNING *;

-- name: UpdateDocument :one
UPDATE documents SET
    modified_at = $2,
    ingested_at = $3,
    file_key = $4,
    sha256 = $5,
    title = $6,
    filename = $7,
    filetype = $8,
    filesize = $9,
    page_count = $10,
    publisher_name = $11,
    publication_year = $12,
    language = $13
WHERE id = $1
RETURNING *;

-- name: DeleteDocument :exec
DELETE FROM documents WHERE id = $1;

-- name: PaginateDocuments :many
SELECT * FROM documents ORDER BY created_at DESC LIMIT $1 OFFSET $2;

-- name: CountDocuments :one
SELECT count(*) FROM documents;
