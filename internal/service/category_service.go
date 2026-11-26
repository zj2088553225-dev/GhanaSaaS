package service

import (
	"context"
	"errors"
	"strings"

	"saas/internal/models"

	"gorm.io/gorm"
)

type categoryRepository interface {
	Create(ctx context.Context, category *models.Category) error
	FindByIDWithTenant(ctx context.Context, tenantID uint, id uint) (*models.Category, error)
	FindByNameWithTenant(ctx context.Context, tenantID uint, name string) (*models.Category, error)
	ListByTenant(ctx context.Context, tenantID uint) ([]models.Category, error)
	CountChildrenWithTenant(ctx context.Context, tenantID uint, id uint) (int64, error)
	CountProductsWithTenant(ctx context.Context, tenantID uint, id uint) (int64, error)
	UpdateWithTenant(ctx context.Context, tenantID uint, category *models.Category) error
	DeleteWithTenant(ctx context.Context, tenantID uint, id uint) error
}

type CategoryService struct {
	repo categoryRepository
}

func NewCategoryService(repo categoryRepository) *CategoryService {
	return &CategoryService{repo: repo}
}

func (s *CategoryService) Create(ctx context.Context, tenantID uint, category *models.Category) error {
	if tenantID == 0 {
		return ErrForbidden
	}
	category.TenantID = tenantID
	category.Name = strings.TrimSpace(category.Name)
	if category.Name == "" {
		return ErrInvalidInput
	}
	if category.ParentID != nil {
		if _, err := s.FindByID(ctx, tenantID, *category.ParentID); err != nil {
			return err
		}
	}
	if _, err := s.repo.FindByNameWithTenant(ctx, tenantID, category.Name); err == nil {
		return ErrConflict
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return s.repo.Create(ctx, category)
}

func (s *CategoryService) FindByID(ctx context.Context, tenantID uint, id uint) (*models.Category, error) {
	if tenantID == 0 || id == 0 {
		return nil, ErrInvalidInput
	}
	category, err := s.repo.FindByIDWithTenant(ctx, tenantID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return category, err
}

func (s *CategoryService) List(ctx context.Context, tenantID uint) ([]models.Category, error) {
	if tenantID == 0 {
		return nil, ErrForbidden
	}
	return s.repo.ListByTenant(ctx, tenantID)
}

func (s *CategoryService) Update(ctx context.Context, tenantID uint, category *models.Category) error {
	if tenantID == 0 {
		return ErrForbidden
	}
	category.TenantID = tenantID
	category.Name = strings.TrimSpace(category.Name)
	if category.ID == 0 || category.Name == "" {
		return ErrInvalidInput
	}
	if _, err := s.FindByID(ctx, tenantID, category.ID); err != nil {
		return err
	}
	if category.ParentID != nil {
		if *category.ParentID == category.ID {
			return ErrInvalidInput
		}
		if _, err := s.FindByID(ctx, tenantID, *category.ParentID); err != nil {
			return err
		}
	}
	if existing, err := s.repo.FindByNameWithTenant(ctx, tenantID, category.Name); err == nil {
		if existing.ID != category.ID {
			return ErrConflict
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return s.repo.UpdateWithTenant(ctx, tenantID, category)
}

func (s *CategoryService) Delete(ctx context.Context, tenantID uint, id uint) error {
	if tenantID == 0 {
		return ErrForbidden
	}
	if _, err := s.FindByID(ctx, tenantID, id); err != nil {
		return err
	}
	childCount, err := s.repo.CountChildrenWithTenant(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if childCount > 0 {
		return ErrConflict
	}
	productCount, err := s.repo.CountProductsWithTenant(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if productCount > 0 {
		return ErrConflict
	}
	if err := s.repo.DeleteWithTenant(ctx, tenantID, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}
