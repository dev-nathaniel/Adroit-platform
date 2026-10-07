-- ============================================================================
-- Adroit Platform Migration (012_add_service_source_type_and_archives.up.sql)
-- Make repo_url nullable in services to support non-git workloads (archives, pre-built images).
-- Add source_type, archive_url, image_url to services.
-- Allow builds to track archive and image sources.
-- ============================================================================

-- 1. Make repo_url nullable in services table
ALTER TABLE services ALTER COLUMN repo_url DROP NOT NULL;

-- 2. Add source_type enum column (git, archive, image)
ALTER TABLE services ADD COLUMN IF NOT EXISTS source_type VARCHAR(50) NOT NULL DEFAULT 'git';

-- 3. Add archive_url for zip / tar.gz code bundles
ALTER TABLE services ADD COLUMN IF NOT EXISTS archive_url TEXT;

-- 4. Add image_url for pre-built container image workloads (e.g. postgres:16, ghcr.io/my-app:v1)
ALTER TABLE services ADD COLUMN IF NOT EXISTS image_url TEXT;

-- 5. Add source type validation constraint
ALTER TABLE services DROP CONSTRAINT IF EXISTS chk_service_source_type;
ALTER TABLE services ADD CONSTRAINT chk_service_source_type CHECK (source_type IN ('git', 'archive', 'image'));

-- 6. Index for querying services by source type
CREATE INDEX IF NOT EXISTS idx_services_source_type ON services(source_type);

-- 7. Make commit_hash and branch nullable in builds table for non-git builds
ALTER TABLE builds ALTER COLUMN commit_hash DROP NOT NULL;
ALTER TABLE builds ALTER COLUMN branch DROP NOT NULL;

-- 8. Add source tracking to builds table
ALTER TABLE builds ADD COLUMN IF NOT EXISTS source_type VARCHAR(50) DEFAULT 'git';
ALTER TABLE builds ADD COLUMN IF NOT EXISTS archive_url TEXT;
