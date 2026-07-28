BEGIN;

-- Supports grouping/paginating a document's chunks by page instead of by
-- sequence_number (sequence_number doesn't reflect true page order once
-- pictures/tables are present — see mapDoclingDocument).
CREATE INDEX IF NOT EXISTS idx_document_chunks_document_page
    ON document_chunks(document_id, page, sequence_number);

COMMIT;
