package seeder

import (
	"context"
	"errors"
	"os"
	"strings"

	"saas/internal/models"
	"saas/internal/pkg/auth"
	"saas/internal/pkg/credentials"

	"gorm.io/gorm"
)

const (
	SelfOperatedAdminPhoneEnv      = "SELF_OPERATED_ADMIN_PHONE"
	SelfOperatedAdminPasswordEnv   = "SELF_OPERATED_ADMIN_PASSWORD"
	DefaultSelfOperatedAdminPhone  = "13800000001"
	PlatformSelfOperatedTenantName = "Platform Self Operated"
)

type SelfOperatedAdminResult struct {
	TenantID           uint
	AdminID            uint
	AdminPhone         string
	Created            bool
	PasswordConfigured bool
}

type selfOperatedAdminInput struct {
	Phone    string
	Password string
}

type selfOperatedAdminStore interface {
	FindSelfOperatedTenant(ctx context.Context) (*models.Tenant, error)
	CreateSelfOperatedTenant(ctx context.Context, tenant *models.Tenant) error
	UpdateSelfOperatedTenant(ctx context.Context, tenant *models.Tenant) error
	FindTenantAdmin(ctx context.Context, tenantID uint) (*models.User, error)
	CreateTenantAdmin(ctx context.Context, user *models.User) error
	UpdateTenantAdmin(ctx context.Context, user *models.User) error
}

type gormSelfOperatedAdminStore struct {
	db *gorm.DB
}

func EnsureSelfOperatedTenantAdmin(ctx context.Context, db *gorm.DB) (*SelfOperatedAdminResult, error) {
	return EnsureSelfOperatedTenantAdminWithCredentials(
		ctx,
		db,
		os.Getenv(SelfOperatedAdminPhoneEnv),
		os.Getenv(SelfOperatedAdminPasswordEnv),
	)
}

func EnsureSelfOperatedTenantAdminWithCredentials(ctx context.Context, db *gorm.DB, phone string, password string) (*SelfOperatedAdminResult, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return ensureSelfOperatedTenantAdmin(ctx, &gormSelfOperatedAdminStore{db: db}, selfOperatedAdminInput{
		Phone:    phone,
		Password: password,
	}, credentials.GenerateAppSecret)
}

func ensureSelfOperatedTenantAdmin(
	ctx context.Context,
	store selfOperatedAdminStore,
	input selfOperatedAdminInput,
	fallbackPassword func() (string, error),
) (*SelfOperatedAdminResult, error) {
	phone := strings.TrimSpace(input.Phone)
	if phone == "" {
		phone = DefaultSelfOperatedAdminPhone
	}
	passwordConfigured := input.Password != ""

	tenant, err := store.FindSelfOperatedTenant(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		tenant = &models.Tenant{
			Name:   PlatformSelfOperatedTenantName,
			Type:   models.TenantTypeSelfOperated,
			Status: models.TenantStatusActive,
		}
		if err := assignTenantCredentials(tenant); err != nil {
			return nil, err
		}
		if err := store.CreateSelfOperatedTenant(ctx, tenant); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if err := ensureSelfOperatedTenantReady(ctx, store, tenant); err != nil {
		return nil, err
	}

	admin, err := store.FindTenantAdmin(ctx, tenant.ID)
	if err == nil {
		admin.Status = models.UserStatusActive
		if passwordConfigured {
			passwordHash, err := auth.HashPassword(input.Password)
			if err != nil {
				return nil, err
			}
			admin.Phone = phone
			admin.PasswordHash = passwordHash
		}
		if err := store.UpdateTenantAdmin(ctx, admin); err != nil {
			return nil, err
		}
		return &SelfOperatedAdminResult{
			TenantID:           tenant.ID,
			AdminID:            admin.ID,
			AdminPhone:         admin.Phone,
			PasswordConfigured: passwordConfigured,
		}, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	password := input.Password
	if password == "" {
		password, err = fallbackPassword()
		if err != nil {
			return nil, err
		}
	}
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		return nil, err
	}

	tenantID := tenant.ID
	admin = &models.User{
		TenantID:     &tenantID,
		Role:         models.UserRoleTenantAdmin,
		Phone:        phone,
		PasswordHash: passwordHash,
		Status:       models.UserStatusActive,
	}
	if err := store.CreateTenantAdmin(ctx, admin); err != nil {
		return nil, err
	}

	return &SelfOperatedAdminResult{
		TenantID:           tenant.ID,
		AdminID:            admin.ID,
		AdminPhone:         admin.Phone,
		Created:            true,
		PasswordConfigured: passwordConfigured,
	}, nil
}

func ensureSelfOperatedTenantReady(ctx context.Context, store selfOperatedAdminStore, tenant *models.Tenant) error {
	changed := false
	if strings.TrimSpace(tenant.Name) == "" {
		tenant.Name = PlatformSelfOperatedTenantName
		changed = true
	}
	if tenant.Type != models.TenantTypeSelfOperated {
		tenant.Type = models.TenantTypeSelfOperated
		changed = true
	}
	if tenant.Status != models.TenantStatusActive {
		tenant.Status = models.TenantStatusActive
		changed = true
	}
	appKey := tenant.AppKey
	appSecret := tenant.AppSecret
	if err := assignTenantCredentials(tenant); err != nil {
		return err
	}
	if tenant.AppKey != appKey || tenant.AppSecret != appSecret {
		changed = true
	}
	if !changed {
		return nil
	}
	return store.UpdateSelfOperatedTenant(ctx, tenant)
}

func (s *gormSelfOperatedAdminStore) FindSelfOperatedTenant(ctx context.Context) (*models.Tenant, error) {
	var tenant models.Tenant
	if err := s.db.WithContext(ctx).
		Where("type = ? OR name = ?", models.TenantTypeSelfOperated, PlatformSelfOperatedTenantName).
		Order("id ASC").
		First(&tenant).Error; err != nil {
		return nil, err
	}
	return &tenant, nil
}

func (s *gormSelfOperatedAdminStore) CreateSelfOperatedTenant(ctx context.Context, tenant *models.Tenant) error {
	return s.db.WithContext(ctx).Create(tenant).Error
}

func (s *gormSelfOperatedAdminStore) UpdateSelfOperatedTenant(ctx context.Context, tenant *models.Tenant) error {
	return s.db.WithContext(ctx).Save(tenant).Error
}

func (s *gormSelfOperatedAdminStore) FindTenantAdmin(ctx context.Context, tenantID uint) (*models.User, error) {
	var user models.User
	if err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND role = ?", tenantID, models.UserRoleTenantAdmin).
		Order("id ASC").
		First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *gormSelfOperatedAdminStore) CreateTenantAdmin(ctx context.Context, user *models.User) error {
	return s.db.WithContext(ctx).Create(user).Error
}

func (s *gormSelfOperatedAdminStore) UpdateTenantAdmin(ctx context.Context, user *models.User) error {
	return s.db.WithContext(ctx).Save(user).Error
}
