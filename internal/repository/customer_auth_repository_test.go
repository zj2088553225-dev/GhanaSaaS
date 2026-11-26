package repository

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"saas/internal/models"
	"saas/internal/pkg/auth"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestCustomerRepositoryResetPasswordIsAtomicAndSingleUse(t *testing.T) {
	dsn := fmt.Sprintf("file:customer-auth-repository-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.AutoMigrate(&models.Customer{}, &models.PasswordResetToken{}); err != nil {
		t.Fatalf("migrate customer auth tables: %v", err)
	}
	oldHash, err := auth.HashPassword("old-password")
	if err != nil {
		t.Fatalf("hash old password: %v", err)
	}
	customer := models.Customer{
		Email:        "buyer@example.com",
		PasswordHash: oldHash,
		Status:       models.CustomerStatusActive,
	}
	if err := db.Create(&customer).Error; err != nil {
		t.Fatalf("create customer: %v", err)
	}
	now := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	resetToken := models.PasswordResetToken{
		UserID:    customer.ID,
		TokenHash: strings.Repeat("a", 64),
		ExpiresAt: now.Add(30 * time.Minute),
	}
	if err := db.Create(&resetToken).Error; err != nil {
		t.Fatalf("create reset token: %v", err)
	}
	newHash, err := auth.HashPassword("new-password")
	if err != nil {
		t.Fatalf("hash new password: %v", err)
	}

	repo := NewCustomerRepository(db)
	if err := repo.ResetPassword(t.Context(), resetToken.TokenHash, newHash, now); err != nil {
		t.Fatalf("ResetPassword returned error: %v", err)
	}
	if err := db.First(&customer, customer.ID).Error; err != nil {
		t.Fatalf("reload customer: %v", err)
	}
	if !auth.CheckPasswordHash("new-password", customer.PasswordHash) {
		t.Fatal("new bcrypt password was not committed")
	}
	if err := db.First(&resetToken, resetToken.ID).Error; err != nil {
		t.Fatalf("reload reset token: %v", err)
	}
	if resetToken.UsedAt == nil || !resetToken.UsedAt.Equal(now) {
		t.Fatalf("used_at = %v, want %v", resetToken.UsedAt, now)
	}
	if err := repo.ResetPassword(t.Context(), resetToken.TokenHash, oldHash, now); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("reused token error = %v, want gorm.ErrRecordNotFound", err)
	}
}

func TestCustomerRepositoryRejectsExpiredResetTokenWithoutChangingPassword(t *testing.T) {
	dsn := fmt.Sprintf("file:customer-auth-expired-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.AutoMigrate(&models.Customer{}, &models.PasswordResetToken{}); err != nil {
		t.Fatalf("migrate customer auth tables: %v", err)
	}
	oldHash, err := auth.HashPassword("old-password")
	if err != nil {
		t.Fatalf("hash old password: %v", err)
	}
	customer := models.Customer{
		Email: "expired@example.com", PasswordHash: oldHash, Status: models.CustomerStatusActive,
	}
	if err := db.Create(&customer).Error; err != nil {
		t.Fatalf("create customer: %v", err)
	}
	now := time.Date(2026, 7, 30, 11, 0, 0, 0, time.UTC)
	resetToken := models.PasswordResetToken{
		UserID: customer.ID, TokenHash: strings.Repeat("b", 64), ExpiresAt: now.Add(-time.Second),
	}
	if err := db.Create(&resetToken).Error; err != nil {
		t.Fatalf("create expired reset token: %v", err)
	}
	newHash, err := auth.HashPassword("new-password")
	if err != nil {
		t.Fatalf("hash new password: %v", err)
	}

	err = NewCustomerRepository(db).ResetPassword(t.Context(), resetToken.TokenHash, newHash, now)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expired token error = %v, want gorm.ErrRecordNotFound", err)
	}
	if err := db.First(&customer, customer.ID).Error; err != nil {
		t.Fatalf("reload customer: %v", err)
	}
	if !auth.CheckPasswordHash("old-password", customer.PasswordHash) {
		t.Fatal("expired token changed the password")
	}
}
