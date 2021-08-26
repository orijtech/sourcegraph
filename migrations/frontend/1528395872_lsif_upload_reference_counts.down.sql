BEGIN;

ALTER TABLE lsif_uploads DROP COLUMN expired;
ALTER TABLE lsif_uploads DROP COLUMN num_references;

COMMIT;
