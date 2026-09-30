-- ============================================================================
-- Adroit Platform Migration (008_add_domains_and_ingress.down.sql)
-- Revert custom_domains and ingress_routes tables
-- ============================================================================

DROP TABLE IF EXISTS ingress_routes CASCADE;
DROP TABLE IF EXISTS custom_domains CASCADE;
