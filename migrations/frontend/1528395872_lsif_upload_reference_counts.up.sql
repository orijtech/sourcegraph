BEGIN;

ALTER TABLE lsif_uploads ADD COLUMN expired boolean not null default false;
ALTER TABLE lsif_uploads ADD COLUMN num_references int;

-- TODO - add comments

COMMIT;
