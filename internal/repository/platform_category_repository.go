package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"saas/internal/models"
	"saas/internal/pkg/categorysearch"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrPlatformCategoryTreeHasProducts = errors.New("platform category tree contains products")
	ErrPlatformCategoryImportCycle     = errors.New("platform category import would create a cycle")
	ErrPlatformCategoryMutationCycle   = errors.New("platform category mutation would create a cycle")
	ErrPlatformCategoryDuplicate       = errors.New("platform category already exists under parent")
)

type PlatformCategoryMutation struct {
	ParentID   *uint
	Name       string
	SortOrder  int
	Status     models.PlatformCategoryStatus
	ZhName     string
	ZhProvided bool
}

type PlatformCategoryImportOperation struct {
	ID         *uint
	Path       string
	ParentPath string
	Name       string
	ZhName     string
	ZhProvided bool
	Level      int
	SortOrder  int
	Status     models.PlatformCategoryStatus
	Explicit   bool
}

type PlatformCategoryImportResult struct {
	Created   int
	Updated   int
	Unchanged int
}

type PlatformCategoryRepository struct {
	db *gorm.DB
}

func NewPlatformCategoryRepository(db *gorm.DB) *PlatformCategoryRepository {
	return &PlatformCategoryRepository{db: db}
}

func (r *PlatformCategoryRepository) CreateAtomic(
	ctx context.Context,
	mutation PlatformCategoryMutation,
) (*models.PlatformCategory, error) {
	var saved models.PlatformCategory
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPlatformCategorySearchMaintenance(tx, false); err != nil {
			return err
		}
		var parent *models.PlatformCategory
		if mutation.ParentID != nil {
			var current models.PlatformCategory
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, *mutation.ParentID).Error; err != nil {
				return err
			}
			parent = &current
		}
		if err := ensurePlatformCategoryNameAvailable(tx, nil, mutation.ParentID, mutation.Name); err != nil {
			return err
		}

		category := models.PlatformCategory{
			ParentID:  clonePlatformCategoryID(mutation.ParentID),
			Name:      mutation.Name,
			SortOrder: mutation.SortOrder,
			Status:    mutation.Status,
			IsLeaf:    true,
		}
		category.Path, category.Level = platformCategoryMutationPathAndLevel(parent, mutation.Name)
		if err := tx.Create(&category).Error; err != nil {
			return err
		}
		if category.ParentID != nil {
			result := tx.Model(&models.PlatformCategory{}).
				Where("id = ?", *category.ParentID).
				Update("is_leaf", false)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return gorm.ErrRecordNotFound
			}
		}
		if err := savePlatformCategoryZhNameTranslation(tx, category.ID, mutation.ZhProvided, mutation.ZhName); err != nil {
			return err
		}
		if err := rebuildPlatformCategorySearchIndex(tx, []uint{category.ID}); err != nil {
			return err
		}
		if err := tx.First(&saved, category.ID).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

func (r *PlatformCategoryRepository) FindByID(ctx context.Context, id uint) (*models.PlatformCategory, error) {
	var category models.PlatformCategory
	if err := r.db.WithContext(ctx).First(&category, id).Error; err != nil {
		return nil, err
	}
	return &category, nil
}

func (r *PlatformCategoryRepository) FindByNameWithParent(ctx context.Context, parentID *uint, name string) (*models.PlatformCategory, error) {
	var category models.PlatformCategory
	db := r.db.WithContext(ctx).Where("LOWER(name) = LOWER(?)", strings.TrimSpace(name))
	if parentID == nil {
		db = db.Where("parent_id IS NULL")
	} else {
		db = db.Where("parent_id = ?", *parentID)
	}
	if err := db.First(&category).Error; err != nil {
		return nil, err
	}
	return &category, nil
}

func (r *PlatformCategoryRepository) List(ctx context.Context, activeOnly bool) ([]models.PlatformCategory, error) {
	var categories []models.PlatformCategory
	db := r.db.WithContext(ctx)
	if activeOnly {
		db = db.Where("status = ?", models.PlatformCategoryStatusActive)
	}
	if err := db.Order("level ASC, sort_order ASC, id ASC").Find(&categories).Error; err != nil {
		return nil, err
	}
	return categories, nil
}

func (r *PlatformCategoryRepository) ListByMaxLevel(ctx context.Context, activeOnly bool, maxLevel int) ([]models.PlatformCategory, error) {
	if maxLevel <= 0 {
		return r.List(ctx, activeOnly)
	}
	var categories []models.PlatformCategory
	db := r.db.WithContext(ctx).Where("level <= ?", maxLevel)
	if activeOnly {
		db = db.Where("status = ?", models.PlatformCategoryStatusActive)
	}
	if err := db.Order("level ASC, sort_order ASC, id ASC").Find(&categories).Error; err != nil {
		return nil, err
	}
	if err := r.populateHasChildren(ctx, categories, activeOnly); err != nil {
		return nil, err
	}
	return categories, nil
}

func (r *PlatformCategoryRepository) ListChildren(
	ctx context.Context,
	parentID *uint,
	activeOnly bool,
) ([]models.PlatformCategory, error) {
	var categories []models.PlatformCategory
	db := r.db.WithContext(ctx)
	if parentID == nil {
		db = db.Where("parent_id IS NULL")
	} else {
		db = db.Where("parent_id = ?", *parentID)
	}
	if activeOnly {
		db = db.Where("status = ?", models.PlatformCategoryStatusActive)
	}
	if err := db.Order("sort_order ASC, id ASC").Find(&categories).Error; err != nil {
		return nil, err
	}
	if err := r.populateHasChildren(ctx, categories, activeOnly); err != nil {
		return nil, err
	}
	return categories, nil
}

func (r *PlatformCategoryRepository) AttachBilingualDisplay(
	ctx context.Context,
	categories []models.PlatformCategory,
) ([]models.PlatformCategory, error) {
	if len(categories) == 0 {
		return categories, nil
	}
	ids := make([]uint, 0, len(categories))
	for _, category := range categories {
		ids = append(ids, category.ID)
	}
	ids = uniquePlatformCategoryIDs(ids)

	var indexes []models.PlatformCategorySearchIndex
	if err := r.db.WithContext(ctx).
		Where("category_id IN ? AND locale = ?", ids, "zh").
		Find(&indexes).Error; err != nil {
		return nil, err
	}
	byCategoryID := make(map[uint]models.PlatformCategorySearchIndex, len(indexes))
	for _, index := range indexes {
		byCategoryID[index.CategoryID] = index
	}

	missingFallback := false
	for i := range categories {
		index, ok := byCategoryID[categories[i].ID]
		if ok && strings.TrimSpace(index.Name) != "" && strings.TrimSpace(index.Path) != "" {
			categories[i].ZhName = index.Name
			categories[i].ZhPath = index.Path
			continue
		}
		missingFallback = true
	}
	if !missingFallback {
		return categories, nil
	}

	fallback, err := r.platformCategoryBilingualDisplayFallback(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range categories {
		if strings.TrimSpace(categories[i].ZhName) != "" && strings.TrimSpace(categories[i].ZhPath) != "" {
			continue
		}
		if display, ok := fallback[categories[i].ID]; ok {
			categories[i].ZhName = display.Name
			categories[i].ZhPath = display.Path
			continue
		}
		categories[i].ZhName = strings.TrimSpace(categories[i].Name)
		categories[i].ZhPath = platformCategorySearchDisplayPath(categories[i].Path)
	}
	return categories, nil
}

type platformCategoryBilingualDisplay struct {
	Name string
	Path string
}

func (r *PlatformCategoryRepository) platformCategoryBilingualDisplayFallback(
	ctx context.Context,
	requestedIDs []uint,
) (map[uint]platformCategoryBilingualDisplay, error) {
	result := make(map[uint]platformCategoryBilingualDisplay)
	requestedIDs = uniquePlatformCategoryIDs(requestedIDs)
	if len(requestedIDs) == 0 {
		return result, nil
	}
	categories, err := findPlatformCategoriesWithAncestorsDB(r.db.WithContext(ctx), requestedIDs)
	if err != nil {
		return nil, err
	}
	ancestorIDs := make([]uint, 0, len(categories))
	for _, category := range categories {
		ancestorIDs = append(ancestorIDs, category.ID)
	}
	zhNames, err := r.platformCategoryZhNameTranslations(ctx, ancestorIDs)
	if err != nil {
		return nil, err
	}
	requested := make(map[uint]struct{}, len(requestedIDs))
	for _, id := range requestedIDs {
		requested[id] = struct{}{}
	}
	for _, index := range buildPlatformCategorySearchIndexes(categories, zhNames, requested) {
		if index.Locale != "zh" {
			continue
		}
		if _, ok := requested[index.CategoryID]; !ok {
			continue
		}
		result[index.CategoryID] = platformCategoryBilingualDisplay{Name: index.Name, Path: index.Path}
	}
	return result, nil
}

func (r *PlatformCategoryRepository) Search(
	ctx context.Context,
	keyword string,
	limit int,
	activeOnly bool,
	requestedLocale string,
) ([]models.PlatformCategory, error) {
	query, err := categorysearch.Parse(keyword)
	if err != nil {
		return nil, err
	}
	if len(query.Tokens) == 0 || limit <= 0 {
		return []models.PlatformCategory{}, nil
	}
	if limit > platformCategoryRepositorySearchMaxLimit {
		limit = platformCategoryRepositorySearchMaxLimit
	}
	categoryIDs, err := searchPlatformCategoryIDs(
		r.db.WithContext(ctx),
		query,
		limit,
		activeOnly,
		normalizePlatformCategorySearchLocale(requestedLocale),
	)
	if err != nil {
		return nil, err
	}
	categories, err := r.findCategoriesWithAncestors(ctx, categoryIDs)
	if err != nil {
		return nil, err
	}
	if err := r.populateHasChildren(ctx, categories, activeOnly); err != nil {
		return nil, err
	}
	return categories, nil
}

func normalizePlatformCategorySearchLocale(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "en" || value == "zh" {
		return value
	}
	return "all"
}

func (r *PlatformCategoryRepository) findCategoriesWithAncestors(ctx context.Context, categoryIDs []uint) ([]models.PlatformCategory, error) {
	return findPlatformCategoriesWithAncestorsDB(r.db.WithContext(ctx), categoryIDs)
}

func findPlatformCategoriesWithAncestorsDB(db *gorm.DB, categoryIDs []uint) ([]models.PlatformCategory, error) {
	if len(categoryIDs) == 0 {
		return []models.PlatformCategory{}, nil
	}
	var categories []models.PlatformCategory
	err := db.Raw(`
		WITH RECURSIVE ancestors AS (
			SELECT id, created_at, updated_at, parent_id, name, path, level, sort_order, status, is_leaf
			FROM platform_categories
			WHERE id IN ?
			UNION
			SELECT parent.id, parent.created_at, parent.updated_at, parent.parent_id, parent.name, parent.path,
				parent.level, parent.sort_order, parent.status, parent.is_leaf
			FROM platform_categories AS parent
			INNER JOIN ancestors AS child ON child.parent_id = parent.id
		)
		SELECT DISTINCT id, created_at, updated_at, parent_id, name, path, level, sort_order, status, is_leaf
		FROM ancestors
		ORDER BY level ASC, sort_order ASC, id ASC
	`, categoryIDs).Scan(&categories).Error
	return categories, err
}

func (r *PlatformCategoryRepository) populateHasChildren(
	ctx context.Context,
	categories []models.PlatformCategory,
	activeOnly bool,
) error {
	if len(categories) == 0 {
		return nil
	}
	ids := make([]uint, 0, len(categories))
	for _, category := range categories {
		ids = append(ids, category.ID)
	}
	var parentIDs []uint
	db := r.db.WithContext(ctx).
		Model(&models.PlatformCategory{}).
		Where("parent_id IN ?", ids)
	if activeOnly {
		db = db.Where("status = ?", models.PlatformCategoryStatusActive)
	}
	if err := db.Distinct("parent_id").Pluck("parent_id", &parentIDs).Error; err != nil {
		return err
	}
	withChildren := make(map[uint]struct{}, len(parentIDs))
	for _, parentID := range parentIDs {
		withChildren[parentID] = struct{}{}
	}
	for i := range categories {
		_, exists := withChildren[categories[i].ID]
		hasChildren := exists
		categories[i].HasChildren = &hasChildren
	}
	return nil
}

func (r *PlatformCategoryRepository) UpdateAtomic(
	ctx context.Context,
	id uint,
	mutation PlatformCategoryMutation,
) (*models.PlatformCategory, error) {
	var saved models.PlatformCategory
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPlatformCategorySearchMaintenance(tx, false); err != nil {
			return err
		}
		var current models.PlatformCategory
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, id).Error; err != nil {
			return err
		}
		oldParentID := clonePlatformCategoryID(current.ParentID)
		oldPath := current.Path
		oldLevel := current.Level

		var parent *models.PlatformCategory
		if mutation.ParentID != nil {
			if *mutation.ParentID == id {
				return ErrPlatformCategoryMutationCycle
			}
			var candidate models.PlatformCategory
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&candidate, *mutation.ParentID).Error; err != nil {
				return err
			}
			parent = &candidate
		}
		wouldCycle, err := platformCategoryMutationWouldCreateCycle(id, mutation.ParentID, tx)
		if err != nil {
			return err
		}
		if wouldCycle {
			return ErrPlatformCategoryMutationCycle
		}
		if err := ensurePlatformCategoryNameAvailable(tx, &id, mutation.ParentID, mutation.Name); err != nil {
			return err
		}

		newPath, newLevel := platformCategoryMutationPathAndLevel(parent, mutation.Name)
		updates := map[string]any{
			"parent_id":  clonePlatformCategoryID(mutation.ParentID),
			"name":       mutation.Name,
			"path":       newPath,
			"level":      newLevel,
			"sort_order": mutation.SortOrder,
			"status":     mutation.Status,
		}
		result := tx.Model(&models.PlatformCategory{}).Where("id = ?", id).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}

		if oldPath != newPath || oldLevel != newLevel {
			oldPrefix := oldPath + "/"
			levelDelta := newLevel - oldLevel
			if err := tx.Model(&models.PlatformCategory{}).
				Where("substr(path, 1, length(?)) = ?", oldPrefix, oldPrefix).
				Updates(map[string]any{
					"path":  gorm.Expr("? || substr(path, length(?) + 1)", newPath, oldPath),
					"level": gorm.Expr("level + ?", levelDelta),
				}).Error; err != nil {
				return err
			}
		}

		if err := updatePlatformCategoryLeaf(tx, oldParentID); err != nil {
			return err
		}
		if err := updatePlatformCategoryLeaf(tx, mutation.ParentID); err != nil {
			return err
		}
		if err := savePlatformCategoryZhNameTranslation(tx, id, mutation.ZhProvided, mutation.ZhName); err != nil {
			return err
		}
		if err := rebuildPlatformCategorySearchIndex(tx, []uint{id}); err != nil {
			return err
		}
		if err := tx.First(&saved, id).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

func (r *PlatformCategoryRepository) DeleteTree(ctx context.Context, category *models.PlatformCategory) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPlatformCategorySearchMaintenance(tx, false); err != nil {
			return err
		}
		var categories []models.PlatformCategory
		if err := tx.Select("id", "parent_id").
			Find(&categories).Error; err != nil {
			return err
		}
		subtreeIDs := platformCategorySubtreeIDs(categories, category.ID)
		if len(subtreeIDs) == 0 {
			return gorm.ErrRecordNotFound
		}
		var parentID *uint
		for _, current := range categories {
			if current.ID == category.ID {
				parentID = clonePlatformCategoryID(current.ParentID)
				break
			}
		}

		var productCount int64
		if err := tx.Model(&models.SPU{}).
			Where("platform_category_id IN ?", subtreeIDs).
			Count(&productCount).Error; err != nil {
			return err
		}
		if productCount > 0 {
			return ErrPlatformCategoryTreeHasProducts
		}

		if err := tx.Where("category_id IN ?", subtreeIDs).
			Delete(&models.PlatformCategorySearchIndex{}).Error; err != nil {
			return err
		}
		if err := tx.Where("entity_type = ? AND entity_id IN ?", "platform_category", subtreeIDs).
			Delete(&models.ContentTranslation{}).Error; err != nil {
			return err
		}
		result := tx.Where("id IN ?", subtreeIDs).Delete(&models.PlatformCategory{})
		if result.Error != nil {
			if isPlatformCategoryProductForeignKeyViolation(result.Error) {
				return ErrPlatformCategoryTreeHasProducts
			}
			return result.Error
		}
		if result.RowsAffected != int64(len(subtreeIDs)) {
			return gorm.ErrRecordNotFound
		}
		return updatePlatformCategoryLeaf(tx, parentID)
	})
}

func isPlatformCategoryProductForeignKeyViolation(err error) bool {
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && pgError.Code == "23503" {
		constraint := strings.ToLower(pgError.ConstraintName)
		table := strings.ToLower(pgError.TableName)
		detail := strings.ToLower(pgError.Detail)
		return constraint == "fk_spus_platform_category" ||
			(strings.Contains(constraint, "spus") && strings.Contains(constraint, "platform_category")) ||
			table == "spus" ||
			(strings.Contains(detail, "spus") && strings.Contains(detail, "platform_category"))
	}
	return strings.Contains(strings.ToLower(err.Error()), "foreign key constraint failed")
}

func (r *PlatformCategoryRepository) ListSubtreeIDs(ctx context.Context, rootID uint) ([]uint, error) {
	var categories []models.PlatformCategory
	if err := r.db.WithContext(ctx).
		Select("id", "parent_id").
		Find(&categories).Error; err != nil {
		return nil, err
	}
	return platformCategorySubtreeIDs(categories, rootID), nil
}

func platformCategorySubtreeIDs(categories []models.PlatformCategory, rootID uint) []uint {
	childrenByParent := make(map[uint][]uint, len(categories))
	exists := false
	for _, category := range categories {
		if category.ID == rootID {
			exists = true
		}
		if category.ParentID != nil {
			childrenByParent[*category.ParentID] = append(childrenByParent[*category.ParentID], category.ID)
		}
	}
	if !exists {
		return nil
	}

	ids := make([]uint, 0)
	queue := []uint{rootID}
	visited := make(map[uint]struct{})
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if _, exists := visited[id]; exists {
			continue
		}
		visited[id] = struct{}{}
		ids = append(ids, id)
		queue = append(queue, childrenByParent[id]...)
	}
	return ids
}

func clonePlatformCategoryID(value *uint) *uint {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func (r *PlatformCategoryRepository) Import(ctx context.Context, operations []PlatformCategoryImportOperation) (PlatformCategoryImportResult, error) {
	result := PlatformCategoryImportResult{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPlatformCategorySearchMaintenance(tx, false); err != nil {
			return err
		}
		var existing []models.PlatformCategory
		if err := tx.Order("level ASC, id ASC").Find(&existing).Error; err != nil {
			return err
		}

		byPath := make(map[string]models.PlatformCategory, len(existing)+len(operations))
		byID := make(map[uint]models.PlatformCategory, len(existing))
		for _, category := range existing {
			byPath[platformCategoryImportKey(category.Path)] = category
			byID[category.ID] = category
		}

		for _, operation := range operations {
			key := platformCategoryImportKey(operation.Path)
			var parentID *uint
			if operation.ParentPath != "" {
				parent, ok := byPath[platformCategoryImportKey(operation.ParentPath)]
				if !ok {
					return gorm.ErrRecordNotFound
				}
				parentValue := parent.ID
				parentID = &parentValue
			}
			category, exists := platformCategoryImportExistingCategory(operation, key, byPath, byID)
			if exists {
				if platformCategoryImportWouldCreateCycle(category.ID, parentID, byID) {
					return ErrPlatformCategoryImportCycle
				}
				if target, occupied := byPath[key]; occupied && target.ID != category.ID {
					return gorm.ErrDuplicatedKey
				}
				oldPath := category.Path
				oldLevel := category.Level
				updates := make(map[string]any)
				if !samePlatformCategoryParent(category.ParentID, parentID) {
					updates["parent_id"] = parentID
				}
				if category.Name != operation.Name {
					updates["name"] = operation.Name
				}
				if category.Path != operation.Path {
					updates["path"] = operation.Path
				}
				if category.Level != operation.Level {
					updates["level"] = operation.Level
				}
				if operation.Explicit {
					if category.SortOrder != operation.SortOrder {
						updates["sort_order"] = operation.SortOrder
					}
					if category.Status != operation.Status {
						updates["status"] = operation.Status
					}
				}
				if len(updates) > 0 {
					if err := tx.Model(&models.PlatformCategory{}).
						Where("id = ?", category.ID).
						Updates(updates).Error; err != nil {
						return err
					}
					if oldPath != "" && oldPath != operation.Path {
						levelDelta := operation.Level - oldLevel
						oldPrefix := oldPath + "/"
						if err := tx.Model(&models.PlatformCategory{}).
							Where("substr(path, 1, length(?)) = ?", oldPrefix, oldPrefix).
							Updates(map[string]any{
								"path":  gorm.Expr("? || substr(path, length(?) + 1)", operation.Path, oldPath),
								"level": gorm.Expr("level + ?", levelDelta),
							}).
							Error; err != nil {
							return err
						}
						updatePlatformCategoryImportDescendants(byPath, byID, oldPath, operation.Path, levelDelta)
						delete(byPath, platformCategoryImportKey(oldPath))
					}
					category.ParentID = parentID
					category.Name = operation.Name
					category.Path = operation.Path
					category.Level = operation.Level
					if operation.Explicit {
						category.SortOrder = operation.SortOrder
						category.Status = operation.Status
						result.Updated++
					}
					byID[category.ID] = category
					byPath[key] = category
				} else if operation.Explicit {
					result.Unchanged++
				}
				continue
			}

			newCategory := models.PlatformCategory{
				ParentID:  parentID,
				Name:      operation.Name,
				Path:      operation.Path,
				Level:     operation.Level,
				SortOrder: operation.SortOrder,
				Status:    operation.Status,
				IsLeaf:    true,
			}
			if err := tx.Create(&newCategory).Error; err != nil {
				return err
			}
			byID[newCategory.ID] = newCategory
			byPath[key] = newCategory
			result.Created++
		}

		if err := tx.Exec(`
			UPDATE platform_categories AS parent
			SET is_leaf = NOT EXISTS (
				SELECT 1 FROM platform_categories AS child WHERE child.parent_id = parent.id
			)
		`).Error; err != nil {
			return err
		}
		if err := savePlatformCategoryImportTranslations(tx, operations, byPath, byID); err != nil {
			return err
		}
		return rebuildPlatformCategorySearchIndex(tx, nil)
	})
	return result, err
}

func (r *PlatformCategoryRepository) RebuildSearchIndex(ctx context.Context, rootIDs []uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPlatformCategorySearchMaintenance(tx, false); err != nil {
			return err
		}
		return rebuildPlatformCategorySearchIndex(tx, rootIDs)
	})
}

func rebuildPlatformCategorySearchIndex(tx *gorm.DB, rootIDs []uint) error {
	if len(rootIDs) == 0 {
		_, err := maintainPlatformCategorySearchIndex(tx, PlatformCategorySearchMaintenanceOptions{
			Apply:     true,
			BatchSize: PlatformCategorySearchRebuildDefaultBatchSize,
		})
		return err
	}
	for _, rootID := range uniquePlatformCategoryIDs(rootIDs) {
		rootID := rootID
		if _, err := maintainPlatformCategorySearchIndex(tx, PlatformCategorySearchMaintenanceOptions{
			Apply:      true,
			BatchSize:  PlatformCategorySearchRebuildDefaultBatchSize,
			CategoryID: &rootID,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *PlatformCategoryRepository) DeleteSearchIndexes(ctx context.Context, categoryIDs []uint) error {
	categoryIDs = uniquePlatformCategoryIDs(categoryIDs)
	if len(categoryIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Where("category_id IN ?", categoryIDs).
		Delete(&models.PlatformCategorySearchIndex{}).Error
}

func (r *PlatformCategoryRepository) platformCategoryZhNameTranslations(
	ctx context.Context,
	categoryIDs []uint,
) (map[uint]string, error) {
	return platformCategoryZhNameTranslationsWithDB(r.db.WithContext(ctx), categoryIDs)
}

func platformCategoryZhNameTranslationsWithDB(
	db *gorm.DB,
	categoryIDs []uint,
) (map[uint]string, error) {
	result := make(map[uint]string)
	categoryIDs = uniquePlatformCategoryIDs(categoryIDs)
	if len(categoryIDs) == 0 {
		return result, nil
	}
	var rows []models.ContentTranslation
	if err := db.
		Where("entity_type = ? AND entity_id IN ? AND field_key = ? AND locale = ? AND status = ?",
			"platform_category", categoryIDs, "name", "zh", models.ContentTranslationStatusPublished).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		value := strings.TrimSpace(row.ValueText)
		if value != "" {
			result[row.EntityID] = value
		}
	}
	return result, nil
}

func buildPlatformCategorySearchIndexes(
	categories []models.PlatformCategory,
	zhNames map[uint]string,
	scopeIDs map[uint]struct{},
) []models.PlatformCategorySearchIndex {
	if len(categories) == 0 || len(scopeIDs) == 0 {
		return nil
	}
	byID := make(map[uint]models.PlatformCategory, len(categories))
	for _, category := range categories {
		byID[category.ID] = category
	}
	enPathMemo := make(map[uint]string, len(categories))
	zhPathMemo := make(map[uint]string, len(categories))
	var localizedPath func(id uint, zh bool, visiting map[uint]struct{}) string
	localizedPath = func(id uint, zh bool, visiting map[uint]struct{}) string {
		memo := enPathMemo
		if zh {
			memo = zhPathMemo
		}
		if path, ok := memo[id]; ok {
			return path
		}
		category, ok := byID[id]
		if !ok {
			return ""
		}
		if _, ok := visiting[id]; ok {
			return strings.TrimSpace(category.Name)
		}
		visiting[id] = struct{}{}
		name := strings.TrimSpace(category.Name)
		if zh {
			name = strings.TrimSpace(platformCategorySearchName(category, zhNames))
		}
		if name == "" {
			name = strings.TrimSpace(category.Name)
		}
		path := name
		if category.ParentID != nil {
			parentPath := localizedPath(*category.ParentID, zh, visiting)
			if parentPath != "" && name != "" {
				path = parentPath + " > " + name
			} else if parentPath != "" {
				path = parentPath
			}
		}
		delete(visiting, id)
		memo[id] = path
		return path
	}

	now := time.Now()
	rows := make([]models.PlatformCategorySearchIndex, 0, len(scopeIDs)*2)
	for _, category := range categories {
		if _, ok := scopeIDs[category.ID]; !ok {
			continue
		}
		enName := strings.TrimSpace(category.Name)
		enPath := strings.TrimSpace(localizedPath(category.ID, false, map[uint]struct{}{}))
		zhName := strings.TrimSpace(platformCategorySearchName(category, zhNames))
		zhPathValue := strings.TrimSpace(localizedPath(category.ID, true, map[uint]struct{}{}))
		rows = append(rows, models.PlatformCategorySearchIndex{
			BaseModel:  models.BaseModel{CreatedAt: now, UpdatedAt: now},
			CategoryID: category.ID,
			Locale:     "en",
			Name:       enName,
			Path:       enPath,
			SearchText: strings.Join(nonEmptyPlatformCategorySearchParts(
				enName,
				enPath,
				strings.ReplaceAll(enPath, " > ", "/"),
			), " "),
			ParentID:  clonePlatformCategoryID(category.ParentID),
			Level:     category.Level,
			Status:    category.Status,
			SortOrder: category.SortOrder,
		}, models.PlatformCategorySearchIndex{
			BaseModel:  models.BaseModel{CreatedAt: now, UpdatedAt: now},
			CategoryID: category.ID,
			Locale:     "zh",
			Name:       zhName,
			Path:       zhPathValue,
			SearchText: strings.Join(nonEmptyPlatformCategorySearchParts(
				zhName,
				zhPathValue,
				strings.ReplaceAll(zhPathValue, " > ", "/"),
				enName,
				enPath,
				strings.ReplaceAll(enPath, " > ", "/"),
			), " "),
			ParentID:  clonePlatformCategoryID(category.ParentID),
			Level:     category.Level,
			Status:    category.Status,
			SortOrder: category.SortOrder,
		})
	}
	return rows
}

func platformCategorySearchName(category models.PlatformCategory, zhNames map[uint]string) string {
	if name := strings.TrimSpace(zhNames[category.ID]); name != "" {
		return name
	}
	return strings.TrimSpace(category.Name)
}

func platformCategorySearchDisplayPath(path string) string {
	return strings.ReplaceAll(strings.TrimSpace(path), "/", " > ")
}

func nonEmptyPlatformCategorySearchParts(parts ...string) []string {
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func uniquePlatformCategoryIDs(values []uint) []uint {
	seen := make(map[uint]struct{}, len(values))
	result := make([]uint, 0, len(values))
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func platformCategoryImportExistingCategory(
	operation PlatformCategoryImportOperation,
	key string,
	byPath map[string]models.PlatformCategory,
	byID map[uint]models.PlatformCategory,
) (models.PlatformCategory, bool) {
	if operation.ID != nil && *operation.ID > 0 {
		if category, exists := byID[*operation.ID]; exists {
			return category, true
		}
	}
	category, exists := byPath[key]
	return category, exists
}

func platformCategoryImportWouldCreateCycle(
	categoryID uint,
	parentID *uint,
	byID map[uint]models.PlatformCategory,
) bool {
	if parentID == nil {
		return false
	}
	currentID := *parentID
	visited := make(map[uint]struct{})
	for currentID != 0 {
		if currentID == categoryID {
			return true
		}
		if _, exists := visited[currentID]; exists {
			return true
		}
		visited[currentID] = struct{}{}
		current, exists := byID[currentID]
		if !exists || current.ParentID == nil {
			return false
		}
		currentID = *current.ParentID
	}
	return false
}

func savePlatformCategoryImportTranslations(
	tx *gorm.DB,
	operations []PlatformCategoryImportOperation,
	byPath map[string]models.PlatformCategory,
	byID map[uint]models.PlatformCategory,
) error {
	values := make(map[uint]string)
	for _, operation := range operations {
		if !operation.ZhProvided {
			continue
		}
		var category models.PlatformCategory
		var exists bool
		if operation.ID != nil && *operation.ID > 0 {
			category, exists = byID[*operation.ID]
		}
		if !exists {
			category, exists = byPath[platformCategoryImportKey(operation.Path)]
		}
		if !exists {
			return gorm.ErrRecordNotFound
		}
		values[category.ID] = strings.TrimSpace(operation.ZhName)
	}
	if len(values) == 0 {
		return nil
	}

	now := time.Now()
	rows := make([]models.ContentTranslation, 0, len(values))
	for categoryID, value := range values {
		if value == "" {
			if err := tx.Where(
				"entity_type = ? AND entity_id = ? AND field_key = ? AND locale = ?",
				"platform_category", categoryID, "name", "zh",
			).Delete(&models.ContentTranslation{}).Error; err != nil {
				return err
			}
			continue
		}
		rows = append(rows, models.ContentTranslation{
			BaseModel:  models.BaseModel{CreatedAt: now, UpdatedAt: now},
			EntityType: "platform_category",
			EntityID:   categoryID,
			FieldKey:   "name",
			Locale:     "zh",
			ValueText:  value,
			Status:     models.ContentTranslationStatusPublished,
		})
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "entity_type"},
			{Name: "entity_id"},
			{Name: "field_key"},
			{Name: "locale"},
		},
		DoUpdates: clause.Assignments(map[string]any{
			"value_text": gorm.Expr("excluded.value_text"),
			"value_html": "",
			"value_json": nil,
			"status":     models.ContentTranslationStatusPublished,
			"updated_at": now,
		}),
	}).CreateInBatches(rows, 500).Error
}

func updatePlatformCategoryImportDescendants(
	byPath map[string]models.PlatformCategory,
	byID map[uint]models.PlatformCategory,
	oldPath string,
	newPath string,
	levelDelta int,
) {
	updates := make([]models.PlatformCategory, 0)
	for key, category := range byPath {
		if !strings.HasPrefix(category.Path, oldPath+"/") {
			continue
		}
		delete(byPath, key)
		category.Path = newPath + strings.TrimPrefix(category.Path, oldPath)
		category.Level += levelDelta
		updates = append(updates, category)
	}
	for _, category := range updates {
		byPath[platformCategoryImportKey(category.Path)] = category
		byID[category.ID] = category
	}
}

func platformCategoryImportKey(path string) string {
	return strings.ToLower(strings.TrimSpace(path))
}

func samePlatformCategoryParent(left, right *uint) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func ensurePlatformCategoryNameAvailable(
	tx *gorm.DB,
	currentID *uint,
	parentID *uint,
	name string,
) error {
	query := tx.Model(&models.PlatformCategory{}).Where("LOWER(name) = LOWER(?)", strings.TrimSpace(name))
	if parentID == nil {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *parentID)
	}
	if currentID != nil {
		query = query.Where("id <> ?", *currentID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrPlatformCategoryDuplicate
	}
	return nil
}

func platformCategoryMutationPathAndLevel(parent *models.PlatformCategory, name string) (string, int) {
	if parent == nil {
		return name, 1
	}
	return parent.Path + "/" + name, parent.Level + 1
}

func platformCategoryMutationWouldCreateCycle(id uint, parentID *uint, tx *gorm.DB) (bool, error) {
	if parentID == nil {
		return false, nil
	}
	currentID := *parentID
	visited := make(map[uint]struct{})
	for currentID != 0 {
		if currentID == id {
			return true, nil
		}
		if _, exists := visited[currentID]; exists {
			return true, nil
		}
		visited[currentID] = struct{}{}
		var current models.PlatformCategory
		if err := tx.Select("id", "parent_id").First(&current, currentID).Error; err != nil {
			return false, err
		}
		if current.ParentID == nil {
			return false, nil
		}
		currentID = *current.ParentID
	}
	return false, nil
}

func savePlatformCategoryZhNameTranslation(tx *gorm.DB, categoryID uint, provided bool, value string) error {
	if !provided {
		return nil
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return tx.Where(
			"entity_type = ? AND entity_id = ? AND field_key = ? AND locale = ?",
			"platform_category", categoryID, "name", "zh",
		).Delete(&models.ContentTranslation{}).Error
	}
	now := time.Now()
	translation := models.ContentTranslation{
		BaseModel:  models.BaseModel{CreatedAt: now, UpdatedAt: now},
		EntityType: "platform_category",
		EntityID:   categoryID,
		FieldKey:   "name",
		Locale:     "zh",
		ValueText:  value,
		Status:     models.ContentTranslationStatusPublished,
	}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "entity_type"},
			{Name: "entity_id"},
			{Name: "field_key"},
			{Name: "locale"},
		},
		DoUpdates: clause.Assignments(map[string]any{
			"value_text": gorm.Expr("excluded.value_text"),
			"value_html": "",
			"value_json": nil,
			"status":     models.ContentTranslationStatusPublished,
			"updated_at": now,
		}),
	}).Create(&translation).Error
}

func updatePlatformCategoryLeaf(tx *gorm.DB, parentID *uint) error {
	if parentID == nil {
		return nil
	}
	var count int64
	if err := tx.Model(&models.PlatformCategory{}).
		Where("parent_id = ?", *parentID).
		Count(&count).Error; err != nil {
		return err
	}
	return tx.Model(&models.PlatformCategory{}).
		Where("id = ?", *parentID).
		Update("is_leaf", count == 0).Error
}
