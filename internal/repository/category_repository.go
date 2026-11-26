package repository

import (
	"context"
	"strings"

	"saas/internal/models"

	"gorm.io/gorm"
)

type CategoryRepository struct {
	db *gorm.DB
}

func NewCategoryRepository(db *gorm.DB) *CategoryRepository {
	return &CategoryRepository{db: db}
}

func (r *CategoryRepository) Create(ctx context.Context, category *models.Category) error {
	return r.db.WithContext(ctx).Create(category).Error
}

func (r *CategoryRepository) FindByIDWithTenant(ctx context.Context, tenantID uint, id uint) (*models.Category, error) {
	var category models.Category
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		First(&category, id).Error; err != nil {
		return nil, err
	}
	return &category, nil
}

func (r *CategoryRepository) FindByNameWithTenant(ctx context.Context, tenantID uint, name string) (*models.Category, error) {
	var category models.Category
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND LOWER(name) = LOWER(?)", tenantID, strings.TrimSpace(name)).
		First(&category).Error; err != nil {
		return nil, err
	}
	return &category, nil
}

func (r *CategoryRepository) ListByTenant(ctx context.Context, tenantID uint) ([]models.Category, error) {
	var categories []models.Category
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("sort ASC, id ASC").
		Find(&categories).Error; err != nil {
		return nil, err
	}
	return categories, nil
}

func (r *CategoryRepository) CountChildrenWithTenant(ctx context.Context, tenantID uint, id uint) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&models.Category{}).
		Where("tenant_id = ? AND parent_id = ?", tenantID, id).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *CategoryRepository) CountProductsWithTenant(ctx context.Context, tenantID uint, id uint) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&models.SPU{}).
		Where("tenant_id = ? AND category_id = ?", tenantID, id).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *CategoryRepository) UpdateWithTenant(ctx context.Context, tenantID uint, category *models.Category) error {
	return r.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", category.ID, tenantID).
		Updates(map[string]any{
			"parent_id": category.ParentID,
			"name":      category.Name,
			"sort":      category.Sort,
			"icon":      category.Icon,
			"is_active": category.IsActive,
		}).Error
}

func (r *CategoryRepository) DeleteWithTenant(ctx context.Context, tenantID uint, id uint) error {
	result := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Delete(&models.Category{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
