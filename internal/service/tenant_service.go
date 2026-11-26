package service

import (
	"context"
	"errors"

	"saas/internal/models"
	"saas/internal/pkg/credentials"
	"saas/internal/repository"

	"gorm.io/gorm"
)

type TenantService struct {
	repo *repository.TenantRepository
}

func NewTenantService(repo *repository.TenantRepository) *TenantService {
	return &TenantService{repo: repo}
}

func (s *TenantService) Create(ctx context.Context, tenant *models.Tenant) error {
	if tenant.Name == "" || tenant.Type == "" {
		return ErrInvalidInput
	}
	if tenant.Status == "" {
		tenant.Status = models.TenantStatusActive
	}
	if tenant.AppKey == "" {
		appKey, err := credentials.GenerateAppKey()
		if err != nil {
			return err
		}
		tenant.AppKey = appKey
	}
	if tenant.AppSecret == "" {
		appSecret, err := credentials.GenerateAppSecret()
		if err != nil {
			return err
		}
		tenant.AppSecret = appSecret
	}
	return s.repo.Create(ctx, tenant)
}

func (s *TenantService) FindByID(ctx context.Context, id uint) (*models.Tenant, error) {
	tenant, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return tenant, err
}

func (s *TenantService) List(ctx context.Context) ([]models.Tenant, error) {
	return s.repo.List(ctx)
}

func (s *TenantService) DeveloperCredentials(ctx context.Context, tenantID uint) (*models.Tenant, error) {
	if tenantID == 0 {
		return nil, ErrForbidden
	}
	return s.FindByID(ctx, tenantID)
}

func (s *TenantService) RegenerateAppSecret(ctx context.Context, tenantID uint) (*models.Tenant, error) {
	if tenantID == 0 {
		return nil, ErrForbidden
	}
	if _, err := s.FindByID(ctx, tenantID); err != nil {
		return nil, err
	}
	appSecret, err := credentials.GenerateAppSecret()
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdateAppSecret(ctx, tenantID, appSecret); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return s.FindByID(ctx, tenantID)
}

func (s *TenantService) Update(ctx context.Context, tenant *models.Tenant) error {
	if tenant.ID == 0 || tenant.Name == "" || tenant.Type == "" || tenant.Status == "" {
		return ErrInvalidInput
	}
	existing, err := s.FindByID(ctx, tenant.ID)
	if err != nil {
		return err
	}
	tenant.AppKey = existing.AppKey
	tenant.AppSecret = existing.AppSecret
	return s.repo.Update(ctx, tenant)
}

func (s *TenantService) Delete(ctx context.Context, id uint) error {
	if _, err := s.FindByID(ctx, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}
