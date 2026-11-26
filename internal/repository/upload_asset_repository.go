package repository

import (
	"context"
	"errors"

	"saas/internal/models"

	"gorm.io/gorm"
)

type UploadAssetRepository struct {
	db *gorm.DB
}

func NewUploadAssetRepository(db *gorm.DB) *UploadAssetRepository {
	return &UploadAssetRepository{db: db}
}

func (r *UploadAssetRepository) FindByHash(
	ctx context.Context,
	hash string,
	assetType string,
) (*models.UploadAsset, bool, error) {
	var asset models.UploadAsset
	err := r.db.WithContext(ctx).
		Where("hash = ? AND asset_type = ?", hash, assetType).
		First(&asset).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &asset, true, nil
}

func (r *UploadAssetRepository) Create(ctx context.Context, asset *models.UploadAsset) error {
	return r.db.WithContext(ctx).Create(asset).Error
}

func (r *UploadAssetRepository) UpdateByHash(ctx context.Context, asset *models.UploadAsset) error {
	result := r.db.WithContext(ctx).
		Model(&models.UploadAsset{}).
		Where("hash = ? AND asset_type = ?", asset.Hash, asset.AssetType).
		Updates(map[string]any{
			"url":       asset.URL,
			"file_name": asset.FileName,
			"mime_type": asset.MimeType,
			"file_size": asset.FileSize,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
