package repository

import (
	"context"

	"saas/internal/models"

	"gorm.io/gorm"
)

type TenantRepository struct {
	db *gorm.DB
}

func NewTenantRepository(db *gorm.DB) *TenantRepository {
	return &TenantRepository{db: db}
}

func (r *TenantRepository) Create(ctx context.Context, tenant *models.Tenant) error {
	return r.db.WithContext(ctx).Create(tenant).Error
}

func (r *TenantRepository) FindByID(ctx context.Context, id uint) (*models.Tenant, error) {
	var tenant models.Tenant
	if err := r.db.WithContext(ctx).First(&tenant, id).Error; err != nil {
		return nil, err
	}
	return &tenant, nil
}

func (r *TenantRepository) List(ctx context.Context) ([]models.Tenant, error) {
	var tenants []models.Tenant
	if err := r.db.WithContext(ctx).Order("id ASC").Find(&tenants).Error; err != nil {
		return nil, err
	}
	return tenants, nil
}

func (r *TenantRepository) Update(ctx context.Context, tenant *models.Tenant) error {
	return r.db.WithContext(ctx).Save(tenant).Error
}

func (r *TenantRepository) UpdateAppSecret(ctx context.Context, id uint, appSecret string) error {
	result := r.db.WithContext(ctx).
		Model(&models.Tenant{}).
		Where("id = ?", id).
		Update("app_secret", appSecret)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *TenantRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&models.Tenant{}, id).Error
}
