package seeder

import (
	"context"
	"strings"
	"testing"

	"saas/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestProductionSeederRequiresExplicitInitialCredentialsBeforeWrites(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv(InitialSuperAdminPhoneEnv, "")
	t.Setenv(InitialSuperAdminPasswordEnv, "")
	t.Setenv(InitialTenantAdminPhoneEnv, "")
	t.Setenv(InitialTenantAdminPasswordEnv, "")

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Tenant{}, &models.User{}); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}

	_, err = Run(context.Background(), db)
	if err == nil || !strings.Contains(err.Error(), "initial administrator credentials are required") {
		t.Fatalf("Run() error = %v, want missing credentials error", err)
	}
	var tenantCount int64
	if err := db.Model(&models.Tenant{}).Count(&tenantCount).Error; err != nil {
		t.Fatalf("count tenants: %v", err)
	}
	if tenantCount != 0 {
		t.Fatalf("tenant count = %d, want 0 after rejected seed", tenantCount)
	}
}

func TestProductionSeederRejectsKnownDefaultPassword(t *testing.T) {
	values := map[string]string{
		InitialSuperAdminPhoneEnv:     "13800000009",
		InitialSuperAdminPasswordEnv:  "Admin@123456",
		InitialTenantAdminPhoneEnv:    "13900000009",
		InitialTenantAdminPasswordEnv: "TenantPassword123!",
	}
	_, err := loadInitialAdminCredentials(func(key string) string { return values[key] }, "production")
	if err == nil || !strings.Contains(err.Error(), "known default password") {
		t.Fatalf("loadInitialAdminCredentials() error = %v, want known default password error", err)
	}
}

func TestProductionSeederAcceptsExplicitStrongCredentials(t *testing.T) {
	values := map[string]string{
		InitialSuperAdminPhoneEnv:     "13800000009",
		InitialSuperAdminPasswordEnv:  "SuperAdminPassword123!",
		InitialTenantAdminPhoneEnv:    "13900000009",
		InitialTenantAdminPasswordEnv: "TenantAdminPassword123!",
	}
	credentials, err := loadInitialAdminCredentials(func(key string) string { return values[key] }, "production")
	if err != nil {
		t.Fatalf("loadInitialAdminCredentials() returned error: %v", err)
	}
	if credentials.SuperAdminPhone != values[InitialSuperAdminPhoneEnv] || credentials.TenantAdminPhone != values[InitialTenantAdminPhoneEnv] {
		t.Fatalf("credentials = %+v, want configured phones", credentials)
	}
}
