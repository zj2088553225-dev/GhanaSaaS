package database

import (
	"fmt"
	"strings"

	"saas/internal/models"

	"gorm.io/gorm"
)

const SKUCodeUniqueIndexName = "idx_skus_erp_sku_code_unique"

type ProductInvariantPreflightError struct {
	OrphanProductCount    int64
	OrphanProductIDs      []uint
	DuplicateSKUCodeCount int64
	DuplicateSKUCodes     []string
}

func (e *ProductInvariantPreflightError) Error() string {
	parts := make([]string, 0, 2)
	if e.OrphanProductCount > 0 {
		parts = append(parts, fmt.Sprintf(
			"found %d products with NULL, zero, or missing platform_category_id (sample SPU IDs: %v)",
			e.OrphanProductCount,
			e.OrphanProductIDs,
		))
	}
	if e.DuplicateSKUCodeCount > 0 {
		parts = append(parts, fmt.Sprintf(
			"found %d duplicated non-empty SKU codes (sample codes: %v)",
			e.DuplicateSKUCodeCount,
			e.DuplicateSKUCodes,
		))
	}
	return "product invariant preflight failed: " + strings.Join(parts, "; ") + ". Run docs/product-invariant-preflight.sql and repair data before restarting"
}

func PreflightProductInvariants(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("product invariant preflight: database is nil")
	}
	violations := &ProductInvariantPreflightError{}

	if db.Migrator().HasTable("spus") && db.Migrator().HasColumn("spus", "platform_category_id") {
		orphanQuery := db.Table("spus AS s")
		if db.Migrator().HasTable("platform_categories") {
			orphanQuery = orphanQuery.
				Joins("LEFT JOIN platform_categories AS pc ON pc.id = s.platform_category_id").
				Where("s.platform_category_id IS NULL OR s.platform_category_id = 0 OR pc.id IS NULL")
		} else {
			orphanQuery = orphanQuery.Where("1 = 1")
		}
		if err := orphanQuery.Count(&violations.OrphanProductCount).Error; err != nil {
			return fmt.Errorf("product invariant preflight orphan query: %w", err)
		}
		if violations.OrphanProductCount > 0 {
			if err := orphanQuery.Select("s.id").Order("s.id ASC").Limit(20).Scan(&violations.OrphanProductIDs).Error; err != nil {
				return fmt.Errorf("product invariant preflight orphan samples: %w", err)
			}
		}
	}

	if db.Migrator().HasTable("skus") && db.Migrator().HasColumn("skus", "erp_sku_code") {
		type duplicateSKUCode struct {
			Code  string
			Count int64
		}
		var duplicates []duplicateSKUCode
		if err := db.Table("skus").
			Select("erp_sku_code AS code, COUNT(*) AS count").
			Where("erp_sku_code IS NOT NULL AND erp_sku_code <> ''").
			Group("erp_sku_code").
			Having("COUNT(*) > 1").
			Order("erp_sku_code ASC").
			Limit(20).
			Scan(&duplicates).Error; err != nil {
			return fmt.Errorf("product invariant preflight SKU duplicate query: %w", err)
		}
		if len(duplicates) > 0 {
			countQuery := db.Table("(SELECT erp_sku_code FROM skus WHERE erp_sku_code IS NOT NULL AND erp_sku_code <> '' GROUP BY erp_sku_code HAVING COUNT(*) > 1) AS duplicate_codes")
			if err := countQuery.Count(&violations.DuplicateSKUCodeCount).Error; err != nil {
				return fmt.Errorf("product invariant preflight SKU duplicate count: %w", err)
			}
			violations.DuplicateSKUCodes = make([]string, 0, len(duplicates))
			for _, duplicate := range duplicates {
				violations.DuplicateSKUCodes = append(violations.DuplicateSKUCodes, duplicate.Code)
			}
		}
	}

	if violations.OrphanProductCount > 0 || violations.DuplicateSKUCodeCount > 0 {
		return violations
	}
	return nil
}

func EnsureProductInvariants(db *gorm.DB) error {
	if err := PreflightProductInvariants(db); err != nil {
		return err
	}
	if !db.Migrator().HasTable("spus") || !db.Migrator().HasTable("platform_categories") || !db.Migrator().HasTable("skus") {
		return fmt.Errorf("product invariant migration requires spus, skus, and platform_categories tables")
	}

	if db.Dialector.Name() == "postgres" {
		if err := db.Exec("ALTER TABLE spus ALTER COLUMN platform_category_id DROP DEFAULT").Error; err != nil {
			return fmt.Errorf("drop platform_category_id default: %w", err)
		}
		if err := db.Exec("ALTER TABLE spus ALTER COLUMN platform_category_id SET NOT NULL").Error; err != nil {
			return fmt.Errorf("set platform_category_id NOT NULL: %w", err)
		}
	}

	if !db.Migrator().HasConstraint(&models.SPU{}, "PlatformCategory") {
		if err := db.Migrator().CreateConstraint(&models.SPU{}, "PlatformCategory"); err != nil {
			return fmt.Errorf("create spus platform category foreign key: %w", err)
		}
	}
	if !db.Migrator().HasConstraint(&models.SPU{}, "PlatformCategory") {
		return fmt.Errorf("spus platform category foreign key was not created")
	}

	if err := db.Exec(
		"CREATE UNIQUE INDEX IF NOT EXISTS " + SKUCodeUniqueIndexName + " ON skus (erp_sku_code) WHERE erp_sku_code <> ''",
	).Error; err != nil {
		return fmt.Errorf("create global non-empty SKU code unique index: %w", err)
	}
	if !db.Migrator().HasIndex(&models.SKU{}, SKUCodeUniqueIndexName) {
		return fmt.Errorf("global non-empty SKU code unique index was not created")
	}
	return nil
}
