package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"saas/internal/config"
	"saas/internal/models"
	"saas/internal/pkg/auth"
	"saas/internal/pkg/database"

	"gorm.io/gorm"
)

const (
	resetAdminPhoneEnv    = "RESET_ADMIN_PHONE"
	resetAdminPasswordEnv = "RESET_ADMIN_PASSWORD"
)

func main() {
	adminPhone, adminPassword, err := loadAdminCredentials(os.Getenv)
	if err != nil {
		log.Fatal(err)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	db, err := database.InitDB(cfg.Database)
	if err != nil {
		log.Fatalf("init database: %v", err)
	}

	passwordHash, err := auth.HashPassword(adminPassword)
	if err != nil {
		log.Fatalf("hash password: %v", err)
	}

	ctx := context.Background()
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		update := map[string]any{
			"tenant_id":     nil,
			"role":          models.UserRoleSuperAdmin,
			"password_hash": passwordHash,
			"status":        models.UserStatusActive,
		}

		result := tx.Model(&models.User{}).Where("phone = ?", adminPhone).Updates(update)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected > 0 {
			return nil
		}

		user := models.User{
			TenantID:     nil,
			Role:         models.UserRoleSuperAdmin,
			Phone:        adminPhone,
			PasswordHash: passwordHash,
			Status:       models.UserStatusActive,
		}
		return tx.Create(&user).Error
	}); err != nil {
		log.Fatalf("reset super admin: %v", err)
	}

	var user models.User
	if err := db.WithContext(ctx).Where("phone = ? AND role = ?", adminPhone, models.UserRoleSuperAdmin).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Fatalf("verify super admin: user was not created")
		}
		log.Fatalf("verify super admin: %v", err)
	}
	if user.TenantID != nil {
		log.Fatalf("verify super admin: tenant_id = %d, want NULL", *user.TenantID)
	}

	log.Printf("super admin reset succeeded: role=%s tenant_id=NULL", models.UserRoleSuperAdmin)
}

func loadAdminCredentials(getenv func(string) string) (string, string, error) {
	phone := strings.TrimSpace(getenv(resetAdminPhoneEnv))
	password := getenv(resetAdminPasswordEnv)
	if phone == "" || strings.TrimSpace(password) == "" {
		return "", "", fmt.Errorf("%s and %s are required", resetAdminPhoneEnv, resetAdminPasswordEnv)
	}
	if len([]byte(password)) < 12 {
		return "", "", fmt.Errorf("%s must be at least 12 bytes", resetAdminPasswordEnv)
	}
	for _, knownDefault := range []string{"123456", "admin", "admin123", "Admin@123456", "password", "password123"} {
		if strings.EqualFold(strings.TrimSpace(password), knownDefault) {
			return "", "", fmt.Errorf("%s uses a known default password", resetAdminPasswordEnv)
		}
	}
	return phone, password, nil
}
