package repository

import (
	"context"

	"saas/internal/models"
	tenantpkg "saas/internal/pkg/tenant"

	"gorm.io/gorm"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, user *models.User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

func (r *UserRepository) FindByID(ctx context.Context, id uint) (*models.User, error) {
	var user models.User
	if err := r.db.WithContext(ctx).First(&user, id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) FindByIDWithTenant(ctx context.Context, tenantID uint, id uint) (*models.User, error) {
	var user models.User
	db := tenantpkg.WithTenant(r.db.WithContext(ctx), tenantID)
	if err := db.First(&user, id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) FindByPhone(ctx context.Context, phone string) (*models.User, error) {
	var user models.User
	if err := r.db.WithContext(ctx).Where("phone = ?", phone).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) FindByPhoneWithRoles(ctx context.Context, phone string, roles ...models.UserRole) (*models.User, error) {
	var user models.User
	if err := r.db.WithContext(ctx).Where("phone = ? AND role IN ?", phone, roles).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) FindByPhoneWithTenant(ctx context.Context, tenantID uint, phone string) (*models.User, error) {
	var user models.User
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND phone = ?", tenantID, phone).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) List(ctx context.Context) ([]models.User, error) {
	var users []models.User
	if err := r.db.WithContext(ctx).Order("id ASC").Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func (r *UserRepository) ListByTenant(ctx context.Context, tenantID uint) ([]models.User, error) {
	var users []models.User
	db := tenantpkg.WithTenant(r.db.WithContext(ctx), tenantID)
	if err := db.Order("id ASC").Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func (r *UserRepository) ListByTenantAndRoles(ctx context.Context, tenantID uint, roles ...models.UserRole) ([]models.User, error) {
	var users []models.User
	db := tenantpkg.WithTenant(r.db.WithContext(ctx), tenantID)
	if err := db.Where("role IN ?", roles).Order("id ASC").Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func (r *UserRepository) Update(ctx context.Context, user *models.User) error {
	return r.db.WithContext(ctx).Save(user).Error
}

func (r *UserRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&models.User{}, id).Error
}

func (r *UserRepository) DeleteWithTenant(ctx context.Context, tenantID uint, id uint) error {
	db := tenantpkg.WithTenant(r.db.WithContext(ctx), tenantID)
	return db.Delete(&models.User{}, id).Error
}
