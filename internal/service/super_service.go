package service

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"saas/internal/models"
	"saas/internal/pkg/auth"
	"saas/internal/pkg/credentials"

	"gorm.io/gorm"
)

type SuperService struct {
	db             *gorm.DB
	jwtSecret      string
	expireDuration time.Duration
}

type CreateTenantWithAdminInput struct {
	Name          string
	Type          models.TenantType
	Status        models.TenantStatus
	AdminPhone    string
	AdminPassword string
}

type TenantWithAdminResult struct {
	Tenant models.Tenant
	Admin  models.User
}

type GlobalDashboardSummary struct {
	TotalTenants int64 `json:"total_tenants"`
	TotalRevenue int64 `json:"total_revenue"`
	TotalOrders  int64 `json:"total_orders"`
	ActiveUsers  int64 `json:"active_users"`
}

func NewSuperService(db *gorm.DB, jwtSecret string, expireDuration time.Duration) *SuperService {
	return &SuperService{
		db:             db,
		jwtSecret:      jwtSecret,
		expireDuration: expireDuration,
	}
}

func (s *SuperService) CreateTenantWithAdmin(ctx context.Context, input CreateTenantWithAdminInput) (*TenantWithAdminResult, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.AdminPhone = strings.TrimSpace(input.AdminPhone)
	if input.Name == "" || input.AdminPhone == "" || input.AdminPassword == "" {
		return nil, ErrInvalidInput
	}
	if input.Type == "" {
		input.Type = models.TenantTypeThirdParty
	}
	if input.Status == "" {
		input.Status = models.TenantStatusActive
	}
	if !validTenantType(input.Type) || !validTenantStatus(input.Status) {
		return nil, ErrInvalidInput
	}

	result := &TenantWithAdminResult{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existingUser models.User
		err := tx.Where(
			"phone = ? AND role IN ?",
			input.AdminPhone,
			[]models.UserRole{
				models.UserRoleSuperAdmin,
				models.UserRoleTenantAdmin,
				models.UserRoleFinance,
				models.UserRolePicker,
			},
		).First(&existingUser).Error
		if err == nil {
			return ErrConflict
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		tenant := models.Tenant{
			Name:   input.Name,
			Type:   input.Type,
			Status: input.Status,
		}
		appKey, err := credentials.GenerateAppKey()
		if err != nil {
			return err
		}
		appSecret, err := credentials.GenerateAppSecret()
		if err != nil {
			return err
		}
		tenant.AppKey = appKey
		tenant.AppSecret = appSecret
		if err := tx.Create(&tenant).Error; err != nil {
			return err
		}

		passwordHash, err := auth.HashPassword(input.AdminPassword)
		if err != nil {
			return err
		}
		tenantID := tenant.ID
		admin := models.User{
			TenantID:     &tenantID,
			Role:         models.UserRoleTenantAdmin,
			Phone:        input.AdminPhone,
			PasswordHash: passwordHash,
			Status:       models.UserStatusActive,
		}
		if err := tx.Create(&admin).Error; err != nil {
			return err
		}

		result.Tenant = tenant
		result.Admin = admin
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *SuperService) ListTenants(ctx context.Context) ([]models.Tenant, error) {
	var tenants []models.Tenant
	if err := s.db.WithContext(ctx).Order("id ASC").Find(&tenants).Error; err != nil {
		return nil, err
	}
	return tenants, nil
}

func (s *SuperService) GlobalDashboardSummary(ctx context.Context) (*GlobalDashboardSummary, error) {
	summary := &GlobalDashboardSummary{}

	if err := s.db.WithContext(ctx).Model(&models.Tenant{}).Count(&summary.TotalTenants).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&models.Order{}).Count(&summary.TotalOrders).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).
		Model(&models.Order{}).
		Where("status <> ?", "cancelled").
		Select("COALESCE(SUM(total_amount), 0)").
		Scan(&summary.TotalRevenue).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).
		Model(&models.Customer{}).
		Count(&summary.ActiveUsers).Error; err != nil {
		return nil, err
	}

	return summary, nil
}

func (s *SuperService) ImpersonateTenantAdmin(ctx context.Context, superAdminID uint, tenantID uint) (*AuthResult, error) {
	if superAdminID == 0 || tenantID == 0 {
		return nil, ErrInvalidInput
	}

	var user models.User
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tenant models.Tenant
		if err := tx.First(&tenant, tenantID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if tenant.Status != models.TenantStatusActive {
			return ErrForbidden
		}

		err := tx.
			Where("tenant_id = ? AND role = ? AND status = ?", tenantID, models.UserRoleTenantAdmin, models.UserStatusActive).
			Order("id ASC").
			First(&user).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		password, err := credentials.GenerateAppSecret()
		if err != nil {
			return err
		}
		passwordHash, err := auth.HashPassword(password)
		if err != nil {
			return err
		}
		user = models.User{
			TenantID:     &tenantID,
			Role:         models.UserRoleTenantAdmin,
			Phone:        generatedTenantAdminPhone(tenantID),
			PasswordHash: passwordHash,
			Status:       models.UserStatusActive,
		}
		return tx.Create(&user).Error
	}); err != nil {
		return nil, err
	}

	token, err := auth.GenerateToken(s.jwtSecret, s.expireDuration, &user)
	if err != nil {
		return nil, err
	}

	log.Printf(
		"super admin impersonated tenant: super_admin_id=%d target_tenant_id=%d impersonated_user_id=%d",
		superAdminID,
		tenantID,
		user.ID,
	)

	return &AuthResult{
		Token: token,
		User: AuthUser{
			ID:       user.ID,
			TenantID: user.TenantID,
			Role:     user.Role,
			Phone:    user.Phone,
			Status:   user.Status,
		},
	}, nil
}

func generatedTenantAdminPhone(tenantID uint) string {
	return "tadmin-" + strconv.FormatUint(uint64(tenantID), 10)
}

func validTenantType(value models.TenantType) bool {
	return value == models.TenantTypeSelfOperated || value == models.TenantTypeThirdParty
}

func validTenantStatus(value models.TenantStatus) bool {
	return value == models.TenantStatusActive || value == models.TenantStatusDisabled
}
