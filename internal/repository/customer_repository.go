package repository

import (
	"context"
	"errors"
	"time"

	"saas/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CustomerRepository struct {
	db *gorm.DB
}

func NewCustomerRepository(db *gorm.DB) *CustomerRepository {
	return &CustomerRepository{db: db}
}

func (r *CustomerRepository) Create(ctx context.Context, customer *models.Customer) error {
	return r.db.WithContext(ctx).Create(customer).Error
}

func (r *CustomerRepository) FindByID(ctx context.Context, id uint) (*models.Customer, error) {
	var customer models.Customer
	if err := r.db.WithContext(ctx).First(&customer, id).Error; err != nil {
		return nil, err
	}
	return &customer, nil
}

func (r *CustomerRepository) FindByPhone(ctx context.Context, phone string) (*models.Customer, error) {
	var customer models.Customer
	if err := r.db.WithContext(ctx).Where("phone = ?", phone).First(&customer).Error; err != nil {
		return nil, err
	}
	return &customer, nil
}

func (r *CustomerRepository) FindByEmail(ctx context.Context, email string) (*models.Customer, error) {
	var customer models.Customer
	if err := r.db.WithContext(ctx).Where("email = ?", email).First(&customer).Error; err != nil {
		return nil, err
	}
	return &customer, nil
}

func (r *CustomerRepository) UpdateLastLoginAt(ctx context.Context, id uint, lastLoginAt time.Time) error {
	result := r.db.WithContext(ctx).
		Model(&models.Customer{}).
		Where("id = ?", id).
		Update("last_login_at", lastLoginAt)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *CustomerRepository) CreatePasswordResetToken(ctx context.Context, resetToken *models.PasswordResetToken) error {
	return r.db.WithContext(ctx).Create(resetToken).Error
}

func (r *CustomerRepository) ResetPassword(
	ctx context.Context,
	tokenHash string,
	passwordHash string,
	now time.Time,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var resetToken models.PasswordResetToken
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("token_hash = ? AND used_at IS NULL AND expires_at > ?", tokenHash, now).
			First(&resetToken).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return gorm.ErrRecordNotFound
		}
		if err != nil {
			return err
		}

		result := tx.Model(&models.Customer{}).
			Where("id = ?", resetToken.UserID).
			Update("password_hash", passwordHash)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}

		result = tx.Model(&models.PasswordResetToken{}).
			Where("id = ? AND used_at IS NULL", resetToken.ID).
			Update("used_at", now)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}
