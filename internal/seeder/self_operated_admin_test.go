package seeder

import (
	"context"
	"testing"

	"saas/internal/models"
	"saas/internal/pkg/auth"

	"gorm.io/gorm"
)

type fakeSelfOperatedAdminStore struct {
	tenant       *models.Tenant
	admin        *models.User
	nextTenantID uint
	nextUserID   uint
}

func newFakeSelfOperatedAdminStore() *fakeSelfOperatedAdminStore {
	return &fakeSelfOperatedAdminStore{
		nextTenantID: 1,
		nextUserID:   1,
	}
}

func (s *fakeSelfOperatedAdminStore) FindSelfOperatedTenant(_ context.Context) (*models.Tenant, error) {
	if s.tenant == nil {
		return nil, gorm.ErrRecordNotFound
	}
	tenant := *s.tenant
	return &tenant, nil
}

func (s *fakeSelfOperatedAdminStore) CreateSelfOperatedTenant(_ context.Context, tenant *models.Tenant) error {
	tenant.ID = s.nextTenantID
	s.nextTenantID++
	cloned := *tenant
	s.tenant = &cloned
	return nil
}

func (s *fakeSelfOperatedAdminStore) UpdateSelfOperatedTenant(_ context.Context, tenant *models.Tenant) error {
	cloned := *tenant
	s.tenant = &cloned
	return nil
}

func (s *fakeSelfOperatedAdminStore) FindTenantAdmin(_ context.Context, tenantID uint) (*models.User, error) {
	if s.admin == nil || s.admin.TenantID == nil || *s.admin.TenantID != tenantID {
		return nil, gorm.ErrRecordNotFound
	}
	admin := *s.admin
	return &admin, nil
}

func (s *fakeSelfOperatedAdminStore) CreateTenantAdmin(_ context.Context, user *models.User) error {
	user.ID = s.nextUserID
	s.nextUserID++
	cloned := *user
	s.admin = &cloned
	return nil
}

func (s *fakeSelfOperatedAdminStore) UpdateTenantAdmin(_ context.Context, user *models.User) error {
	cloned := *user
	s.admin = &cloned
	return nil
}

func TestEnsureSelfOperatedTenantAdminCreatesAdminForExistingTenant(t *testing.T) {
	ctx := context.Background()
	store := newFakeSelfOperatedAdminStore()
	store.tenant = &models.Tenant{
		BaseModel: models.BaseModel{ID: 10},
		Name:      "Platform Self Operated",
		Type:      models.TenantTypeSelfOperated,
		Status:    models.TenantStatusActive,
	}

	result, err := ensureSelfOperatedTenantAdmin(ctx, store, selfOperatedAdminInput{
		Phone:    "13800000009",
		Password: "StrongPassword123!",
	}, func() (string, error) {
		t.Fatal("fallback password should not be used when password is configured")
		return "", nil
	})
	if err != nil {
		t.Fatalf("ensureSelfOperatedTenantAdmin returned error: %v", err)
	}
	if !result.Created || !result.PasswordConfigured {
		t.Fatalf("result = %+v, want created with configured password", result)
	}
	if result.TenantID != 10 || result.AdminID == 0 || result.AdminPhone != "13800000009" {
		t.Fatalf("result = %+v, want tenant 10 and configured phone", result)
	}
	if store.admin == nil || store.admin.TenantID == nil || *store.admin.TenantID != 10 {
		t.Fatalf("admin = %+v, want tenant 10", store.admin)
	}
	if store.admin.Role != models.UserRoleTenantAdmin || store.admin.Status != models.UserStatusActive {
		t.Fatalf("admin = %+v, want active tenant_admin", store.admin)
	}
	if !auth.CheckPasswordHash("StrongPassword123!", store.admin.PasswordHash) {
		t.Fatal("admin password hash does not match configured password")
	}
}

func TestEnsureSelfOperatedTenantAdminCreatesTenantWhenMissing(t *testing.T) {
	ctx := context.Background()
	store := newFakeSelfOperatedAdminStore()

	result, err := ensureSelfOperatedTenantAdmin(ctx, store, selfOperatedAdminInput{}, func() (string, error) {
		return "GeneratedPassword123!", nil
	})
	if err != nil {
		t.Fatalf("ensureSelfOperatedTenantAdmin returned error: %v", err)
	}
	if store.tenant == nil || store.tenant.Type != models.TenantTypeSelfOperated || store.tenant.Status != models.TenantStatusActive {
		t.Fatalf("tenant = %+v, want active self operated tenant", store.tenant)
	}
	if store.tenant.AppKey == "" || store.tenant.AppSecret == "" {
		t.Fatalf("tenant credentials were not assigned: %+v", store.tenant)
	}
	if store.admin == nil || store.admin.Phone != DefaultSelfOperatedAdminPhone {
		t.Fatalf("admin = %+v, want default self operated phone", store.admin)
	}
	if result.PasswordConfigured {
		t.Fatalf("PasswordConfigured = true, want false for generated fallback password")
	}
}

func TestEnsureSelfOperatedTenantAdminRepairsExistingPlatformTenant(t *testing.T) {
	ctx := context.Background()
	store := newFakeSelfOperatedAdminStore()
	tenantID := uint(10)
	store.tenant = &models.Tenant{
		BaseModel: models.BaseModel{ID: tenantID},
		Name:      PlatformSelfOperatedTenantName,
		Type:      models.TenantTypeThirdParty,
		Status:    models.TenantStatusDisabled,
	}

	result, err := ensureSelfOperatedTenantAdmin(ctx, store, selfOperatedAdminInput{}, func() (string, error) {
		return "GeneratedPassword123!", nil
	})
	if err != nil {
		t.Fatalf("ensureSelfOperatedTenantAdmin returned error: %v", err)
	}
	if result.TenantID != tenantID {
		t.Fatalf("tenant id = %d, want %d", result.TenantID, tenantID)
	}
	if store.tenant.Type != models.TenantTypeSelfOperated {
		t.Fatalf("tenant type = %q, want %q", store.tenant.Type, models.TenantTypeSelfOperated)
	}
	if store.tenant.Status != models.TenantStatusActive {
		t.Fatalf("tenant status = %q, want %q", store.tenant.Status, models.TenantStatusActive)
	}
	if store.admin == nil || store.admin.TenantID == nil || *store.admin.TenantID != tenantID {
		t.Fatalf("admin = %+v, want tenant %d", store.admin, tenantID)
	}
}

func TestEnsureSelfOperatedTenantAdminActivatesExistingAdminWithoutChangingPassword(t *testing.T) {
	ctx := context.Background()
	store := newFakeSelfOperatedAdminStore()
	tenantID := uint(10)
	passwordHash, err := auth.HashPassword("ExistingPassword123!")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	store.tenant = &models.Tenant{
		BaseModel: models.BaseModel{ID: tenantID},
		Name:      "Platform Self Operated",
		Type:      models.TenantTypeSelfOperated,
		Status:    models.TenantStatusActive,
	}
	store.admin = &models.User{
		BaseModel:    models.BaseModel{ID: 99},
		TenantID:     &tenantID,
		Role:         models.UserRoleTenantAdmin,
		Phone:        "13800000001",
		PasswordHash: passwordHash,
		Status:       models.UserStatusDisabled,
	}

	result, err := ensureSelfOperatedTenantAdmin(ctx, store, selfOperatedAdminInput{}, func() (string, error) {
		t.Fatal("fallback password should not be used for an existing admin")
		return "", nil
	})
	if err != nil {
		t.Fatalf("ensureSelfOperatedTenantAdmin returned error: %v", err)
	}
	if result.Created {
		t.Fatalf("Created = true, want false for existing admin")
	}
	if store.admin.Status != models.UserStatusActive {
		t.Fatalf("admin status = %q, want active", store.admin.Status)
	}
	if !auth.CheckPasswordHash("ExistingPassword123!", store.admin.PasswordHash) {
		t.Fatal("existing password hash was changed without configured password")
	}
}
