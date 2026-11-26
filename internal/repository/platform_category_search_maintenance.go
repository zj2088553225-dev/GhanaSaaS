package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"saas/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	PlatformCategorySearchRebuildDefaultBatchSize = 500
	PlatformCategorySearchRebuildMaxBatchSize     = 5000
	platformCategorySearchAdvisoryLockKey         = int64(73422193020260722)
)

var (
	ErrPlatformCategorySearchRebuildInProgress = errors.New("platform category search rebuild already in progress")
	ErrPlatformCategorySearchVerification      = errors.New("platform category search index verification failed")
)

type PlatformCategorySearchMaintenanceOptions struct {
	Apply      bool
	Verify     bool
	BatchSize  int
	CategoryID *uint
	TryLock    bool
}

type PlatformCategorySearchMaintenanceSummary struct {
	Categories          int64 `json:"categories"`
	ExpectedRows        int64 `json:"expected_rows"`
	Created             int64 `json:"created"`
	Updated             int64 `json:"updated"`
	Unchanged           int64 `json:"unchanged"`
	DeletedOrphans      int64 `json:"deleted_orphans"`
	MissingTranslations int64 `json:"missing_translations"`
	FallbackZhRows      int64 `json:"fallback_zh_rows"`
	InvalidRows         int64 `json:"invalid_rows"`
	Verified            bool  `json:"verified"`
}

func (s PlatformCategorySearchMaintenanceSummary) HasDrift() bool {
	return s.Created != 0 || s.Updated != 0 || s.DeletedOrphans != 0 || s.InvalidRows != 0
}

func (r *PlatformCategoryRepository) MaintainSearchIndex(
	ctx context.Context,
	options PlatformCategorySearchMaintenanceOptions,
) (PlatformCategorySearchMaintenanceSummary, error) {
	options.BatchSize = normalizePlatformCategorySearchBatchSize(options.BatchSize)
	var summary PlatformCategorySearchMaintenanceSummary
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if !options.Apply && tx.Dialector.Name() == "postgres" {
			if err := tx.Exec("SET TRANSACTION ISOLATION LEVEL REPEATABLE READ, READ ONLY").Error; err != nil {
				return fmt.Errorf("start read-only platform category search audit: %w", err)
			}
		}
		if options.Apply {
			if err := lockPlatformCategorySearchMaintenance(tx, options.TryLock); err != nil {
				return err
			}
		}
		var err error
		summary, err = maintainPlatformCategorySearchIndex(tx, options)
		if err != nil {
			return err
		}
		if !options.Verify {
			return nil
		}
		if !options.Apply {
			if summary.HasDrift() {
				return ErrPlatformCategorySearchVerification
			}
			summary.Verified = true
			return nil
		}
		verification, err := maintainPlatformCategorySearchIndex(tx, PlatformCategorySearchMaintenanceOptions{
			BatchSize:  options.BatchSize,
			CategoryID: options.CategoryID,
		})
		if err != nil {
			return fmt.Errorf("verify rebuilt platform category search index: %w", err)
		}
		if verification.HasDrift() {
			return fmt.Errorf("%w: created=%d updated=%d deleted_orphans=%d invalid_rows=%d",
				ErrPlatformCategorySearchVerification,
				verification.Created,
				verification.Updated,
				verification.DeletedOrphans,
				verification.InvalidRows,
			)
		}
		summary.Verified = true
		return nil
	})
	return summary, err
}

func (r *PlatformCategoryRepository) BeforeContentTranslationMutations(
	_ context.Context,
	tx *gorm.DB,
	mutations []ContentTranslationMutation,
) error {
	if len(platformCategorySearchTranslationMutationIDs(mutations)) == 0 {
		return nil
	}
	return lockPlatformCategorySearchMaintenance(tx, false)
}

func (r *PlatformCategoryRepository) AfterContentTranslationMutations(
	_ context.Context,
	tx *gorm.DB,
	mutations []ContentTranslationMutation,
) error {
	categoryIDs := platformCategorySearchTranslationMutationIDs(mutations)
	if len(categoryIDs) == 0 {
		return nil
	}
	return rebuildPlatformCategorySearchIndex(tx, categoryIDs)
}

func platformCategorySearchTranslationMutationIDs(mutations []ContentTranslationMutation) []uint {
	ids := make([]uint, 0, len(mutations))
	for _, mutation := range mutations {
		if mutation.EntityType != "platform_category" ||
			mutation.FieldKey != "name" ||
			mutation.Locale != "zh" ||
			mutation.EntityID == 0 {
			continue
		}
		ids = append(ids, mutation.EntityID)
	}
	return uniquePlatformCategoryIDs(ids)
}

func normalizePlatformCategorySearchBatchSize(value int) int {
	if value <= 0 {
		return PlatformCategorySearchRebuildDefaultBatchSize
	}
	if value > PlatformCategorySearchRebuildMaxBatchSize {
		return PlatformCategorySearchRebuildMaxBatchSize
	}
	return value
}

func lockPlatformCategorySearchMaintenance(tx *gorm.DB, try bool) error {
	if tx.Dialector.Name() != "postgres" {
		return nil
	}
	if !try {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", platformCategorySearchAdvisoryLockKey).Error; err != nil {
			return fmt.Errorf("lock platform category search maintenance: %w", err)
		}
		return nil
	}
	var acquired bool
	if err := tx.Raw("SELECT pg_try_advisory_xact_lock(?)", platformCategorySearchAdvisoryLockKey).
		Scan(&acquired).Error; err != nil {
		return fmt.Errorf("try platform category search maintenance lock: %w", err)
	}
	if !acquired {
		return ErrPlatformCategorySearchRebuildInProgress
	}
	return nil
}

func maintainPlatformCategorySearchIndex(
	tx *gorm.DB,
	options PlatformCategorySearchMaintenanceOptions,
) (PlatformCategorySearchMaintenanceSummary, error) {
	summary := PlatformCategorySearchMaintenanceSummary{}
	if options.CategoryID != nil && *options.CategoryID == 0 {
		return summary, gorm.ErrRecordNotFound
	}

	if options.CategoryID == nil {
		orphanQuery := tx.Table("platform_category_search_indexes AS search_index").
			Joins("LEFT JOIN platform_categories AS category ON category.id = search_index.category_id").
			Where("category.id IS NULL")
		if err := orphanQuery.Count(&summary.DeletedOrphans).Error; err != nil {
			return summary, fmt.Errorf("count orphan platform category search rows: %w", err)
		}
		if options.Apply && summary.DeletedOrphans > 0 {
			if err := tx.Exec(`
				DELETE FROM platform_category_search_indexes AS search_index
				WHERE NOT EXISTS (
					SELECT 1 FROM platform_categories AS category WHERE category.id = search_index.category_id
				)
			`).Error; err != nil {
				return summary, fmt.Errorf("delete orphan platform category search rows: %w", err)
			}
		}
	}

	lastID := uint(0)
	for {
		categoryIDs, err := platformCategorySearchTargetBatch(tx, options.CategoryID, lastID, options.BatchSize)
		if err != nil {
			return summary, err
		}
		if len(categoryIDs) == 0 {
			break
		}
		lastID = categoryIDs[len(categoryIDs)-1]
		summary.Categories += int64(len(categoryIDs))
		summary.ExpectedRows += int64(len(categoryIDs) * 2)

		expected, missingTranslations, fallbackZhRows, invalidRows, err := expectedPlatformCategorySearchRows(tx, categoryIDs)
		if err != nil {
			return summary, err
		}
		summary.MissingTranslations += missingTranslations
		summary.FallbackZhRows += fallbackZhRows
		summary.InvalidRows += invalidRows

		var existing []models.PlatformCategorySearchIndex
		if err := tx.Where("category_id IN ?", categoryIDs).Find(&existing).Error; err != nil {
			return summary, fmt.Errorf("load existing platform category search rows: %w", err)
		}
		existingByKey := make(map[string]models.PlatformCategorySearchIndex, len(existing))
		unexpectedLocales := make([]uint, 0)
		for _, row := range existing {
			if row.Locale != "en" && row.Locale != "zh" {
				summary.InvalidRows++
				unexpectedLocales = append(unexpectedLocales, row.ID)
				continue
			}
			existingByKey[platformCategorySearchRowKey(row.CategoryID, row.Locale)] = row
		}

		changed := make([]models.PlatformCategorySearchIndex, 0)
		for _, row := range expected {
			current, exists := existingByKey[platformCategorySearchRowKey(row.CategoryID, row.Locale)]
			switch {
			case !exists:
				summary.Created++
				changed = append(changed, row)
			case !samePlatformCategorySearchRow(current, row):
				summary.Updated++
				changed = append(changed, row)
			default:
				summary.Unchanged++
			}
		}
		if !options.Apply {
			continue
		}
		if len(unexpectedLocales) > 0 {
			if err := tx.Where("id IN ?", unexpectedLocales).Delete(&models.PlatformCategorySearchIndex{}).Error; err != nil {
				return summary, fmt.Errorf("delete invalid-locale platform category search rows: %w", err)
			}
		}
		if err := upsertPlatformCategorySearchRows(tx, changed); err != nil {
			return summary, err
		}
	}

	if options.CategoryID != nil && summary.Categories == 0 {
		return summary, gorm.ErrRecordNotFound
	}
	if summary.ExpectedRows != summary.Created+summary.Updated+summary.Unchanged {
		summary.InvalidRows += summary.ExpectedRows - (summary.Created + summary.Updated + summary.Unchanged)
	}
	return summary, nil
}

func platformCategorySearchTargetBatch(
	tx *gorm.DB,
	rootID *uint,
	lastID uint,
	batchSize int,
) ([]uint, error) {
	var ids []uint
	if rootID == nil {
		if err := tx.Model(&models.PlatformCategory{}).
			Where("id > ?", lastID).
			Order("id ASC").
			Limit(batchSize).
			Pluck("id", &ids).Error; err != nil {
			return nil, fmt.Errorf("read platform category search rebuild batch: %w", err)
		}
		return ids, nil
	}
	if err := tx.Raw(`
		WITH RECURSIVE subtree AS (
			SELECT id FROM platform_categories WHERE id = ?
			UNION
			SELECT child.id
			FROM platform_categories AS child
			INNER JOIN subtree AS parent ON child.parent_id = parent.id
		)
		SELECT id FROM subtree WHERE id > ? ORDER BY id ASC LIMIT ?
	`, *rootID, lastID, batchSize).Scan(&ids).Error; err != nil {
		return nil, fmt.Errorf("read platform category subtree search rebuild batch: %w", err)
	}
	return ids, nil
}

func expectedPlatformCategorySearchRows(
	tx *gorm.DB,
	categoryIDs []uint,
) ([]models.PlatformCategorySearchIndex, int64, int64, int64, error) {
	categories, err := findPlatformCategoriesWithAncestorsDB(tx, categoryIDs)
	if err != nil {
		return nil, 0, 0, 0, fmt.Errorf("load platform category ancestors for search rebuild: %w", err)
	}
	ancestorIDs := make([]uint, 0, len(categories))
	for _, category := range categories {
		ancestorIDs = append(ancestorIDs, category.ID)
	}
	translations, err := platformCategoryZhNameTranslationsWithDB(tx, ancestorIDs)
	if err != nil {
		return nil, 0, 0, 0, fmt.Errorf("load platform category translations for search rebuild: %w", err)
	}
	scope := make(map[uint]struct{}, len(categoryIDs))
	for _, id := range categoryIDs {
		scope[id] = struct{}{}
	}
	rows := buildPlatformCategorySearchIndexes(categories, translations, scope)

	byID := make(map[uint]models.PlatformCategory, len(categories))
	for _, category := range categories {
		byID[category.ID] = category
	}
	var missingTranslations int64
	var fallbackZhRows int64
	var invalidRows int64
	for _, id := range categoryIDs {
		category, exists := byID[id]
		if !exists || strings.TrimSpace(category.Name) == "" || !platformCategorySearchHierarchyValid(category, byID) {
			invalidRows++
			continue
		}
		if strings.TrimSpace(translations[id]) == "" {
			missingTranslations++
		}
		if platformCategoryZhPathUsesFallback(id, byID, translations) {
			fallbackZhRows++
		}
	}
	return rows, missingTranslations, fallbackZhRows, invalidRows, nil
}

func platformCategorySearchHierarchyValid(
	category models.PlatformCategory,
	byID map[uint]models.PlatformCategory,
) bool {
	if category.Level <= 0 {
		return false
	}
	visited := make(map[uint]struct{}, category.Level)
	current := category
	count := 0
	for {
		if _, exists := visited[current.ID]; exists {
			return false
		}
		visited[current.ID] = struct{}{}
		count++
		if current.ParentID == nil {
			return current.Level == 1 && count == category.Level
		}
		parent, exists := byID[*current.ParentID]
		if !exists || parent.Level != current.Level-1 || strings.TrimSpace(parent.Name) == "" {
			return false
		}
		current = parent
	}
}

func platformCategoryZhPathUsesFallback(
	id uint,
	byID map[uint]models.PlatformCategory,
	translations map[uint]string,
) bool {
	visited := make(map[uint]struct{})
	for id != 0 {
		if _, exists := visited[id]; exists {
			return true
		}
		visited[id] = struct{}{}
		category, exists := byID[id]
		if !exists {
			return true
		}
		if strings.TrimSpace(translations[id]) == "" {
			return true
		}
		if category.ParentID == nil {
			return false
		}
		id = *category.ParentID
	}
	return false
}

func platformCategorySearchRowKey(categoryID uint, locale string) string {
	return fmt.Sprintf("%d\x00%s", categoryID, locale)
}

func samePlatformCategorySearchRow(left, right models.PlatformCategorySearchIndex) bool {
	return left.CategoryID == right.CategoryID &&
		left.Locale == right.Locale &&
		left.Name == right.Name &&
		left.Path == right.Path &&
		left.SearchText == right.SearchText &&
		samePlatformCategoryParent(left.ParentID, right.ParentID) &&
		left.Level == right.Level &&
		left.Status == right.Status &&
		left.SortOrder == right.SortOrder
}

func upsertPlatformCategorySearchRows(tx *gorm.DB, rows []models.PlatformCategorySearchIndex) error {
	if len(rows) == 0 {
		return nil
	}
	now := time.Now()
	for index := range rows {
		rows[index].CreatedAt = now
		rows[index].UpdatedAt = now
	}
	if err := tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "category_id"}, {Name: "locale"}},
		DoUpdates: clause.Assignments(map[string]any{
			"name":        gorm.Expr("excluded.name"),
			"path":        gorm.Expr("excluded.path"),
			"search_text": gorm.Expr("excluded.search_text"),
			"parent_id":   gorm.Expr("excluded.parent_id"),
			"level":       gorm.Expr("excluded.level"),
			"status":      gorm.Expr("excluded.status"),
			"sort_order":  gorm.Expr("excluded.sort_order"),
			"updated_at":  now,
		}),
	}).CreateInBatches(rows, 1000).Error; err != nil {
		return fmt.Errorf("upsert platform category search rows: %w", err)
	}
	return nil
}
