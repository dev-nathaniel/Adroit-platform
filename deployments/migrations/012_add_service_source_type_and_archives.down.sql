-- ============================================================================
-- Adroit Platform Migration (012_add_service_source_type_and_archives.down.sql)
-- Revert source_type, archive_url, image_url additions.
-- ============================================================================

-- 1. Revert builds columns
ALTER TABLE builds DROP COLUMN IF EXISTS archive_url;
ALTER TABLE builds DROP COLUMN IF EXISTS source_type;
UPDATE builds SET commit_hash = 'head' WHERE commit_hash IS NULL;
UPDATE builds SET branch = 'main' WHERE branch IS NULL;
ALTER TABLE builds ALTER COLUMN commit_hash SET NOT NULL;
ALTER TABLE builds ALTER COLUMN branch SET NOT NULL;

-- 2. Drop index and constraint
DROP INDEX IF EXISTS idx_services_source_type;
ALTER TABLE services DROP CONSTRAINT IF EXISTS chk_service_source_type;

-- 3. Drop columns from services
ALTER TABLE services DROP COLUMN IF EXISTS image_url;
ALTER TABLE services DROP COLUMN IF EXISTS archive_url;
ALTER TABLE services DROP COLUMN IF EXISTS source_type;

-- 4. Restore repo_url NOT NULL (backfilling empty string for nulls)
UPDATE services SET repo_url = '' WHERE repo_url IS NULL;
ALTER TABLE services ALTER COLUMN repo_url SET NOT NULL;
