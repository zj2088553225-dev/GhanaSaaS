package repository

import (
	"context"
	"errors"
	"time"

	"saas/internal/models"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SitePageTranslationMutation struct {
	Provided bool
	Content  datatypes.JSON
}

type SitePagePublishTranslationBuilder func(
	englishDraft datatypes.JSON,
	zhDraft datatypes.JSON,
) (datatypes.JSON, bool, error)

type SitePageRepository struct {
	db *gorm.DB
}

func NewSitePageRepository(db *gorm.DB) *SitePageRepository {
	return &SitePageRepository{db: db}
}

func (r *SitePageRepository) FindByKey(ctx context.Context, pageKey string) (*models.SitePage, error) {
	var page models.SitePage
	err := r.db.WithContext(ctx).Where("page_key = ?", pageKey).First(&page).Error
	if err != nil {
		return nil, err
	}
	return &page, nil
}

func (r *SitePageRepository) SaveDraftAtomic(
	ctx context.Context,
	pageKey string,
	title string,
	content datatypes.JSON,
	userID uint,
	translation SitePageTranslationMutation,
) (*models.SitePage, error) {
	var saved models.SitePage
	userIDPtr := userID

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.SitePage
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("page_key = ?", pageKey).First(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			page := models.SitePage{
				PageKey:      pageKey,
				Title:        title,
				DraftContent: content,
				Status:       models.SitePageStatusDraft,
				CreatedBy:    &userIDPtr,
				UpdatedBy:    &userIDPtr,
			}
			if err := tx.Omit(clause.Associations).Create(&page).Error; err != nil {
				return err
			}
			saved = page
		case err != nil:
			return err
		default:
			updates := map[string]any{
				"title":         title,
				"draft_content": content,
				"status":        models.SitePageStatusDraft,
				"updated_by":    &userIDPtr,
			}
			if err := tx.Model(&existing).Updates(updates).Error; err != nil {
				return err
			}
			if err := tx.Where("page_key = ?", pageKey).First(&saved).Error; err != nil {
				return err
			}
		}
		return saveSitePageTranslationMutation(tx, saved.ID, "draft_content", translation)
	})
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

func (r *SitePageRepository) PublishAtomic(
	ctx context.Context,
	pageKey string,
	userID uint,
	buildTranslation SitePagePublishTranslationBuilder,
) (*models.SitePage, error) {
	var saved models.SitePage
	userIDPtr := userID

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.SitePage
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("page_key = ?", pageKey).First(&existing).Error; err != nil {
			return err
		}

		var zhDraft datatypes.JSON
		var draftTranslation models.ContentTranslation
		err := tx.Where(
			"entity_type = ? AND entity_id = ? AND field_key = ? AND locale = ? AND status = ?",
			"site_page", existing.ID, "draft_content", "zh", models.ContentTranslationStatusPublished,
		).First(&draftTranslation).Error
		switch {
		case err == nil:
			zhDraft = append(datatypes.JSON(nil), draftTranslation.ValueJSON...)
		case errors.Is(err, gorm.ErrRecordNotFound):
		default:
			return err
		}
		publishedTranslation := SitePageTranslationMutation{Provided: true}
		if buildTranslation != nil {
			localized, hasTranslation, err := buildTranslation(existing.DraftContent, zhDraft)
			if err != nil {
				return err
			}
			if hasTranslation {
				publishedTranslation.Content = localized
			}
		}

		publishedAt := time.Now()
		updates := map[string]any{
			"published_content": existing.DraftContent,
			"status":            models.SitePageStatusPublished,
			"updated_by":        &userIDPtr,
			"published_at":      &publishedAt,
		}
		if err := tx.Model(&existing).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.Where("page_key = ?", pageKey).First(&saved).Error; err != nil {
			return err
		}
		return saveSitePageTranslationMutation(tx, saved.ID, "published_content", publishedTranslation)
	})
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

func saveSitePageTranslationMutation(
	tx *gorm.DB,
	pageID uint,
	fieldKey string,
	mutation SitePageTranslationMutation,
) error {
	if !mutation.Provided {
		return nil
	}
	if len(mutation.Content) == 0 || string(mutation.Content) == "null" {
		return tx.Where(
			"entity_type = ? AND entity_id = ? AND field_key = ? AND locale = ?",
			"site_page", pageID, fieldKey, "zh",
		).Delete(&models.ContentTranslation{}).Error
	}
	now := time.Now()
	translation := models.ContentTranslation{
		BaseModel:  models.BaseModel{CreatedAt: now, UpdatedAt: now},
		EntityType: "site_page",
		EntityID:   pageID,
		FieldKey:   fieldKey,
		Locale:     "zh",
		ValueJSON:  append(datatypes.JSON(nil), mutation.Content...),
		Status:     models.ContentTranslationStatusPublished,
	}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "entity_type"},
			{Name: "entity_id"},
			{Name: "field_key"},
			{Name: "locale"},
		},
		DoUpdates: clause.Assignments(map[string]any{
			"value_text": "",
			"value_html": "",
			"value_json": gorm.Expr("excluded.value_json"),
			"status":     models.ContentTranslationStatusPublished,
			"updated_at": now,
		}),
	}).Create(&translation).Error
}
