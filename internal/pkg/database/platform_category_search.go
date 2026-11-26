package database

import (
	"fmt"

	"gorm.io/gorm"
)

const (
	PlatformCategorySearchTextTrigramIndex = "idx_platform_category_search_indexes_search_text_trgm"
	PlatformCategorySearchOrderIndex       = "idx_platform_category_search_indexes_status_locale_order"
)

type PlatformCategorySearchInfrastructureStatus struct {
	TrigramExtension bool `json:"trigram_extension"`
	SearchTextGIN    bool `json:"search_text_gin"`
	OrderBTree       bool `json:"order_btree"`
}

func EnsurePlatformCategorySearchInfrastructure(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("platform category search infrastructure: nil database")
	}
	if db.Dialector.Name() != "postgres" {
		return nil
	}
	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS pg_trgm").Error; err != nil {
		return fmt.Errorf("enable required pg_trgm extension (database role needs CREATE privilege): %w", err)
	}
	if err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_platform_category_search_indexes_search_text_trgm
		ON platform_category_search_indexes
		USING gin (search_text gin_trgm_ops)
	`).Error; err != nil {
		return fmt.Errorf("create platform category search_text trigram index: %w", err)
	}
	if err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_platform_category_search_indexes_status_locale_order
		ON platform_category_search_indexes (status, locale, level, sort_order, category_id)
	`).Error; err != nil {
		return fmt.Errorf("create platform category search ordering index: %w", err)
	}
	return nil
}

func CheckPlatformCategorySearchInfrastructure(db *gorm.DB) (PlatformCategorySearchInfrastructureStatus, error) {
	status := PlatformCategorySearchInfrastructureStatus{}
	if db == nil {
		return status, fmt.Errorf("platform category search infrastructure: nil database")
	}
	if db.Dialector.Name() != "postgres" {
		return status, nil
	}
	if err := db.Raw(`SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_trgm')`).
		Scan(&status.TrigramExtension).Error; err != nil {
		return status, fmt.Errorf("check pg_trgm extension: %w", err)
	}
	type indexStatus struct {
		IndexName string
	}
	var indexes []indexStatus
	if err := db.Raw(`
		SELECT indexname AS index_name
		FROM pg_indexes
		WHERE schemaname = current_schema()
			AND tablename = 'platform_category_search_indexes'
			AND indexname IN (?, ?)
	`, PlatformCategorySearchTextTrigramIndex, PlatformCategorySearchOrderIndex).Scan(&indexes).Error; err != nil {
		return status, fmt.Errorf("check platform category search indexes: %w", err)
	}
	for _, index := range indexes {
		switch index.IndexName {
		case PlatformCategorySearchTextTrigramIndex:
			status.SearchTextGIN = true
		case PlatformCategorySearchOrderIndex:
			status.OrderBTree = true
		}
	}
	return status, nil
}

func (s PlatformCategorySearchInfrastructureStatus) Ready() bool {
	return s.TrigramExtension && s.SearchTextGIN && s.OrderBTree
}
