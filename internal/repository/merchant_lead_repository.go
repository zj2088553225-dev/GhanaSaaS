package repository

import (
	"context"

	"saas/internal/models"

	"gorm.io/gorm"
)

type MerchantLeadRepository struct {
	db *gorm.DB
}

func NewMerchantLeadRepository(db *gorm.DB) *MerchantLeadRepository {
	return &MerchantLeadRepository{db: db}
}

func (r *MerchantLeadRepository) Create(ctx context.Context, lead *models.MerchantLead) error {
	return r.db.WithContext(ctx).Create(lead).Error
}

func (r *MerchantLeadRepository) List(ctx context.Context) ([]models.MerchantLead, error) {
	var leads []models.MerchantLead
	err := r.db.WithContext(ctx).
		Order("created_at DESC, id DESC").
		Find(&leads).Error
	return leads, err
}

func (r *MerchantLeadRepository) UpdateStatus(ctx context.Context, id uint, status models.MerchantLeadStatus) (*models.MerchantLead, error) {
	var lead models.MerchantLead
	if err := r.db.WithContext(ctx).First(&lead, id).Error; err != nil {
		return nil, err
	}
	lead.Status = status
	if err := r.db.WithContext(ctx).Save(&lead).Error; err != nil {
		return nil, err
	}
	return &lead, nil
}
