package database

import (
	"fmt"
	"strings"

	"saas/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	StorefrontEventIDUniqueIndexName = "idx_storefront_events_event_id_unique"
	storefrontEventBackfillBatchSize = 500
)

type StorefrontEventReliabilityPreflightError struct {
	MissingEventIDCount     int64
	DuplicateEventIDCount   int64
	DuplicateEventIDSamples []string
	InvalidTrafficTypeCount int64
}

func (e *StorefrontEventReliabilityPreflightError) Error() string {
	parts := make([]string, 0, 3)
	if e.MissingEventIDCount > 0 {
		parts = append(parts, fmt.Sprintf("%d events have a missing event_id", e.MissingEventIDCount))
	}
	if e.DuplicateEventIDCount > 0 {
		parts = append(parts, fmt.Sprintf(
			"%d event_id values are duplicated (samples: %v)",
			e.DuplicateEventIDCount,
			e.DuplicateEventIDSamples,
		))
	}
	if e.InvalidTrafficTypeCount > 0 {
		parts = append(parts, fmt.Sprintf("%d events have an invalid traffic_type", e.InvalidTrafficTypeCount))
	}
	return "storefront event reliability preflight failed: " + strings.Join(parts, "; ")
}

// PrepareStorefrontEventReliability upgrades an existing Phase 1 table without
// relying on pgcrypto or uuid-ossp. event_id starts nullable, historical rows
// are assigned UUIDs in bounded Go batches, then PostgreSQL receives NOT NULL
// and the database-enforced unique index. Historical traffic defaults to human
// because the original User-Agent was intentionally never stored.
func PrepareStorefrontEventReliability(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("storefront event reliability migration: database is nil")
	}
	if !db.Migrator().HasTable("storefront_events") {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if !tx.Migrator().HasColumn("storefront_events", "event_id") {
			if err := tx.Exec("ALTER TABLE storefront_events ADD COLUMN event_id varchar(36)").Error; err != nil {
				return fmt.Errorf("add nullable storefront event_id: %w", err)
			}
		}
		if !tx.Migrator().HasColumn("storefront_events", "traffic_type") {
			if err := tx.Exec("ALTER TABLE storefront_events ADD COLUMN traffic_type varchar(16) NOT NULL DEFAULT 'human'").Error; err != nil {
				return fmt.Errorf("add storefront traffic_type: %w", err)
			}
		}
		if err := tx.Table("storefront_events").
			Where("traffic_type IS NULL OR TRIM(traffic_type) = ''").
			Update("traffic_type", models.StorefrontTrafficHuman).Error; err != nil {
			return fmt.Errorf("backfill storefront traffic_type: %w", err)
		}
		if err := backfillStorefrontEventIDs(tx); err != nil {
			return err
		}
		if err := PreflightStorefrontEventReliability(tx); err != nil {
			return err
		}
		if tx.Dialector.Name() == "postgres" {
			if err := tx.Exec("ALTER TABLE storefront_events ALTER COLUMN event_id SET NOT NULL").Error; err != nil {
				return fmt.Errorf("set storefront event_id NOT NULL: %w", err)
			}
		}
		if err := tx.Exec(
			"CREATE UNIQUE INDEX IF NOT EXISTS " + StorefrontEventIDUniqueIndexName + " ON storefront_events (event_id)",
		).Error; err != nil {
			return fmt.Errorf("create storefront event_id unique index: %w", err)
		}
		unique, err := storefrontEventIDIndexIsUnique(tx)
		if err != nil {
			return err
		}
		if !unique {
			return fmt.Errorf("storefront event_id index %s exists but is not unique", StorefrontEventIDUniqueIndexName)
		}
		return nil
	})
}

func backfillStorefrontEventIDs(db *gorm.DB) error {
	for {
		var rowIDs []uint
		if err := db.Table("storefront_events").
			Select("id").
			Where("event_id IS NULL OR TRIM(event_id) = ''").
			Order("id ASC").
			Limit(storefrontEventBackfillBatchSize).
			Scan(&rowIDs).Error; err != nil {
			return fmt.Errorf("select storefront events requiring event_id backfill: %w", err)
		}
		if len(rowIDs) == 0 {
			return nil
		}
		for _, rowID := range rowIDs {
			if err := db.Table("storefront_events").
				Where("id = ? AND (event_id IS NULL OR TRIM(event_id) = '')", rowID).
				Update("event_id", uuid.NewString()).Error; err != nil {
				return fmt.Errorf("backfill storefront event_id for row %d: %w", rowID, err)
			}
		}
	}
}

// PreflightStorefrontEventReliability is read-only and can be used before an
// invariant is enforced to report actionable historical-data violations.
func PreflightStorefrontEventReliability(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("storefront event reliability preflight: database is nil")
	}
	if !db.Migrator().HasTable("storefront_events") ||
		!db.Migrator().HasColumn("storefront_events", "event_id") ||
		!db.Migrator().HasColumn("storefront_events", "traffic_type") {
		return nil
	}

	violations := &StorefrontEventReliabilityPreflightError{}
	if err := db.Table("storefront_events").
		Where("event_id IS NULL OR TRIM(event_id) = ''").
		Count(&violations.MissingEventIDCount).Error; err != nil {
		return fmt.Errorf("storefront event reliability missing event_id query: %w", err)
	}

	type duplicateEventID struct {
		EventID string
		Count   int64
	}
	var duplicateSamples []duplicateEventID
	duplicates := db.Table("storefront_events").
		Select("event_id, COUNT(*) AS count").
		Where("event_id IS NOT NULL AND TRIM(event_id) <> ''").
		Group("event_id").
		Having("COUNT(*) > 1")
	if err := db.Table("(?) AS duplicate_event_ids", duplicates).
		Count(&violations.DuplicateEventIDCount).Error; err != nil {
		return fmt.Errorf("storefront event reliability duplicate event_id count: %w", err)
	}
	if violations.DuplicateEventIDCount > 0 {
		if err := duplicates.Order("event_id ASC").Limit(20).Scan(&duplicateSamples).Error; err != nil {
			return fmt.Errorf("storefront event reliability duplicate event_id samples: %w", err)
		}
		for _, sample := range duplicateSamples {
			violations.DuplicateEventIDSamples = append(violations.DuplicateEventIDSamples, sample.EventID)
		}
	}

	if err := db.Table("storefront_events").
		Where("traffic_type NOT IN ? OR traffic_type IS NULL", []models.StorefrontTrafficType{
			models.StorefrontTrafficHuman,
			models.StorefrontTrafficBot,
		}).
		Count(&violations.InvalidTrafficTypeCount).Error; err != nil {
		return fmt.Errorf("storefront event reliability traffic_type query: %w", err)
	}

	if violations.MissingEventIDCount > 0 || violations.DuplicateEventIDCount > 0 || violations.InvalidTrafficTypeCount > 0 {
		return violations
	}
	return nil
}

func EnsureStorefrontEventReliability(db *gorm.DB) error {
	if err := PrepareStorefrontEventReliability(db); err != nil {
		return err
	}
	if !db.Migrator().HasTable("storefront_events") ||
		!db.Migrator().HasColumn("storefront_events", "event_id") ||
		!db.Migrator().HasColumn("storefront_events", "traffic_type") {
		return fmt.Errorf("storefront event reliability columns were not created")
	}
	if !db.Migrator().HasIndex(&models.StorefrontEvent{}, StorefrontEventIDUniqueIndexName) {
		return fmt.Errorf("storefront event_id unique index was not created")
	}
	unique, err := storefrontEventIDIndexIsUnique(db)
	if err != nil {
		return err
	}
	if !unique {
		return fmt.Errorf("storefront event_id index %s is not unique", StorefrontEventIDUniqueIndexName)
	}
	return PreflightStorefrontEventReliability(db)
}

func storefrontEventIDIndexIsUnique(db *gorm.DB) (bool, error) {
	switch db.Dialector.Name() {
	case "postgres":
		var unique bool
		err := db.Raw(`SELECT EXISTS (
    SELECT 1
    FROM pg_indexes
    WHERE schemaname = current_schema()
      AND tablename = 'storefront_events'
      AND indexname = ?
      AND indexdef ILIKE 'CREATE UNIQUE INDEX%'
)`, StorefrontEventIDUniqueIndexName).Scan(&unique).Error
		if err != nil {
			return false, fmt.Errorf("verify PostgreSQL storefront event_id unique index: %w", err)
		}
		return unique, nil
	case "sqlite":
		var indexes []struct {
			Name   string
			Unique int
		}
		if err := db.Raw("PRAGMA index_list('storefront_events')").Scan(&indexes).Error; err != nil {
			return false, fmt.Errorf("verify SQLite storefront event_id unique index: %w", err)
		}
		for _, index := range indexes {
			if index.Name == StorefrontEventIDUniqueIndexName {
				return index.Unique == 1, nil
			}
		}
		return false, nil
	default:
		return false, fmt.Errorf("cannot verify storefront event_id unique index for %s", db.Dialector.Name())
	}
}
