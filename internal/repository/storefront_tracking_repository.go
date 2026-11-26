package repository

import (
	"context"
	"errors"

	"saas/internal/models"

	"gorm.io/gorm"
)

type StorefrontTrackingRepository struct {
	db *gorm.DB
}

func NewStorefrontTrackingRepository(db *gorm.DB) *StorefrontTrackingRepository {
	return &StorefrontTrackingRepository{db: db}
}

func (r *StorefrontTrackingRepository) Get(ctx context.Context) (*models.StorefrontTrackingSetting, error) {
	var setting models.StorefrontTrackingSetting
	err := r.db.WithContext(ctx).Where("id = ?", 1).First(&setting).Error
	if err != nil {
		return nil, err
	}
	return &setting, nil
}

func (r *StorefrontTrackingRepository) Save(
	ctx context.Context,
	setting models.StorefrontTrackingSetting,
) (*models.StorefrontTrackingSetting, error) {
	var saved models.StorefrontTrackingSetting
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.StorefrontTrackingSetting
		err := tx.Where("id = ?", 1).First(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			setting.ID = 1
			if err := tx.Create(&setting).Error; err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			if err := tx.Model(&existing).Updates(map[string]any{
				"meta_pixel_enabled":  setting.MetaPixelEnabled,
				"meta_pixel_id":       setting.MetaPixelID,
				"custom_head_enabled": setting.CustomHeadEnabled,
				"custom_head_html":    setting.CustomHeadHTML,
			}).Error; err != nil {
				return err
			}
		}
		return tx.Where("id = ?", 1).First(&saved).Error
	})
	if err != nil {
		return nil, err
	}
	return &saved, nil
}
