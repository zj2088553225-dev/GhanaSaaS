package service

import (
	"context"
	"errors"
	"strings"

	"saas/internal/models"
	"saas/internal/pkg/auth"
	"saas/internal/repository"

	"gorm.io/gorm"
)

type UserService struct {
	repo *repository.UserRepository
}

type CreateEmployeeInput struct {
	Role     models.UserRole
	Phone    string
	Password string
}

func NewUserService(repo *repository.UserRepository) *UserService {
	return &UserService{repo: repo}
}

func (s *UserService) Create(ctx context.Context, user *models.User, password string) error {
	user.Phone = strings.TrimSpace(user.Phone)
	if user.Phone == "" || password == "" {
		return ErrInvalidInput
	}
	if err := normalizeUser(user); err != nil {
		return err
	}

	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	user.PasswordHash = passwordHash

	return s.repo.Create(ctx, user)
}

func (s *UserService) CreateEmployee(ctx context.Context, tenantID uint, input CreateEmployeeInput) (*models.User, error) {
	input.Phone = strings.TrimSpace(input.Phone)
	if tenantID == 0 || input.Phone == "" || input.Password == "" {
		return nil, ErrInvalidInput
	}
	if !isEmployeeRole(input.Role) {
		return nil, ErrInvalidInput
	}
	if err := s.ensureBackOfficePhoneAvailable(ctx, input.Phone); err != nil {
		return nil, err
	}

	user := models.User{
		TenantID: &tenantID,
		Role:     input.Role,
		Phone:    input.Phone,
		Status:   models.UserStatusActive,
	}
	if err := s.Create(ctx, &user, input.Password); err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *UserService) FindByID(ctx context.Context, id uint, tenantID *uint) (*models.User, error) {
	var (
		user *models.User
		err  error
	)
	if tenantID == nil {
		user, err = s.repo.FindByID(ctx, id)
	} else {
		user, err = s.repo.FindByIDWithTenant(ctx, *tenantID, id)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return user, err
}

func (s *UserService) List(ctx context.Context, tenantID *uint) ([]models.User, error) {
	if tenantID == nil {
		return s.repo.List(ctx)
	}
	return s.repo.ListByTenant(ctx, *tenantID)
}

func (s *UserService) ListEmployees(ctx context.Context, tenantID uint) ([]models.User, error) {
	if tenantID == 0 {
		return nil, ErrForbidden
	}
	return s.repo.ListByTenantAndRoles(ctx, tenantID, models.UserRoleFinance, models.UserRolePicker)
}

func (s *UserService) Update(ctx context.Context, user *models.User, password *string, tenantID *uint) error {
	user.Phone = strings.TrimSpace(user.Phone)
	if user.ID == 0 || user.Phone == "" || user.Status == "" {
		return ErrInvalidInput
	}
	if err := normalizeUser(user); err != nil {
		return err
	}

	existing, err := s.FindByID(ctx, user.ID, tenantID)
	if err != nil {
		return err
	}
	user.CreatedAt = existing.CreatedAt
	user.PasswordHash = existing.PasswordHash

	if password != nil {
		if *password == "" {
			return ErrInvalidInput
		}
		passwordHash, err := auth.HashPassword(*password)
		if err != nil {
			return err
		}
		user.PasswordHash = passwordHash
	}

	return s.repo.Update(ctx, user)
}

func (s *UserService) Delete(ctx context.Context, id uint, tenantID *uint) error {
	if _, err := s.FindByID(ctx, id, tenantID); err != nil {
		return err
	}
	if tenantID == nil {
		return s.repo.Delete(ctx, id)
	}
	return s.repo.DeleteWithTenant(ctx, *tenantID, id)
}

func normalizeUser(user *models.User) error {
	switch user.Role {
	case models.UserRoleSuperAdmin:
		user.TenantID = nil
	case models.UserRoleTenantAdmin, models.UserRoleFinance, models.UserRolePicker:
		if user.TenantID == nil {
			return ErrInvalidInput
		}
	default:
		return ErrInvalidInput
	}

	switch user.Status {
	case "":
		user.Status = models.UserStatusActive
	case models.UserStatusActive, models.UserStatusDisabled:
	default:
		return ErrInvalidInput
	}

	return nil
}

func (s *UserService) ensureBackOfficePhoneAvailable(ctx context.Context, phone string) error {
	_, err := s.repo.FindByPhoneWithRoles(
		ctx,
		phone,
		models.UserRoleSuperAdmin,
		models.UserRoleTenantAdmin,
		models.UserRoleFinance,
		models.UserRolePicker,
	)
	if err == nil {
		return ErrConflict
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return nil
}

func isEmployeeRole(role models.UserRole) bool {
	return role == models.UserRoleFinance || role == models.UserRolePicker
}
