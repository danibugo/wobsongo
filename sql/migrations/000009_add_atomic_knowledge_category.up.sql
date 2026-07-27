ALTER TABLE atomic_knowledge ADD COLUMN category INTEGER NOT NULL DEFAULT 0; -- 0 = clinical

-- Dev-only system, no production data yet — delete the pre-existing facts
-- outright (not just tag them) whose predicate matches the
-- bibliographic/authorship/administrative patterns validated against this
-- deployment's real extracted data, to align existing data with the new
-- "never persist confident metadata" policy applied to extraction going
-- forward.
DELETE FROM atomic_knowledge
WHERE predicate ILIKE '%associated with%'
   OR predicate ILIKE '%affiliated%'
   OR predicate ILIKE '%author%'
   OR predicate ILIKE '%published%'
   OR predicate ILIKE '%DOI%'
   OR predicate ILIKE '%URI%'
   OR predicate ILIKE '%volume%'
   OR predicate ILIKE '%page range%'
   OR predicate ILIKE '%member of%'
   OR predicate ILIKE '%editor%'
   OR predicate ILIKE '%reviewer%'
   OR predicate ILIKE '%chair%'
   OR predicate ILIKE '%located%'
   OR predicate ILIKE '%supported%';
