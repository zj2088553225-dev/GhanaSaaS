-- Read-only checks. Run with psql and ON_ERROR_STOP before deploying the
-- release that adds the product category FK and global non-empty SKU code index.
-- This file returns a non-zero psql status when an invariant is violated.

\set ON_ERROR_STOP on

SELECT EXISTS (
    SELECT 1
    FROM spus AS s
    LEFT JOIN platform_categories AS pc ON pc.id = s.platform_category_id
    WHERE s.platform_category_id IS NULL
       OR s.platform_category_id = 0
       OR pc.id IS NULL
) AS has_orphan_products \gset

\if :has_orphan_products
\echo 'ERROR: orphan spus.platform_category_id rows must be repaired before deployment'

-- Products whose platform category cannot satisfy the new FK.
SELECT
    s.id AS spu_id,
    s.tenant_id,
    s.platform_category_id
FROM spus AS s
LEFT JOIN platform_categories AS pc ON pc.id = s.platform_category_id
WHERE s.platform_category_id IS NULL
   OR s.platform_category_id = 0
   OR pc.id IS NULL
ORDER BY s.id;
\quit 20
\endif

SELECT EXISTS (
    SELECT 1
    FROM skus
    WHERE erp_sku_code IS NOT NULL
      AND erp_sku_code <> ''
    GROUP BY erp_sku_code
    HAVING COUNT(*) > 1
) AS has_duplicate_sku_codes \gset

\if :has_duplicate_sku_codes
\echo 'ERROR: duplicate non-empty SKU codes must be repaired before deployment'

-- Non-empty SKU codes that cannot satisfy the global unique index.
-- New codes contain the tenant ID, so global uniqueness does not merge tenant
-- namespaces. Historical empty codes remain outside the partial index.
SELECT
    erp_sku_code,
    COUNT(*) AS duplicate_count,
    ARRAY_AGG(id ORDER BY id) AS sku_ids
FROM skus
WHERE erp_sku_code IS NOT NULL
  AND erp_sku_code <> ''
GROUP BY erp_sku_code
HAVING COUNT(*) > 1
ORDER BY erp_sku_code;
\quit 21
\endif

\echo 'Product invariant preflight passed: no orphan category references or duplicate non-empty SKU codes.'

-- Verification after a successful application startup/migration.
SELECT conname, pg_get_constraintdef(oid)
FROM pg_constraint
WHERE conrelid = 'spus'::regclass
  AND conname = 'fk_spus_platform_category';

SELECT indexname, indexdef
FROM pg_indexes
WHERE schemaname = current_schema()
  AND tablename = 'skus'
  AND indexname = 'idx_skus_erp_sku_code_unique';
