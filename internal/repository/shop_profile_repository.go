package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"saas/internal/models"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrDuplicateShopProfileSlug     = errors.New("duplicate shop profile slug")
	ErrShopProfileTenantUnavailable = errors.New("shop profile tenant is unavailable")
)

type ShopProfileTranslationMutation struct {
	Provided      bool
	ShopName      string
	Description   string
	Address       string
	BusinessHours string
}

type ShopProfileRepository struct {
	db *gorm.DB
}

func NewShopProfileRepository(db *gorm.DB) *ShopProfileRepository {
	return &ShopProfileRepository{db: db}
}

func (r *ShopProfileRepository) FindByTenant(ctx context.Context, tenantID uint) (*models.ShopProfile, error) {
	var profile models.ShopProfile
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).First(&profile).Error
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

func (r *ShopProfileRepository) SaveAtomic(
	ctx context.Context,
	tenantID uint,
	profile *models.ShopProfile,
	translation ShopProfileTranslationMutation,
) (*models.ShopProfile, error) {
	var saved models.ShopProfile
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tenant models.Tenant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND status = ?", tenantID, models.TenantStatusActive).
			First(&tenant).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrShopProfileTenantUnavailable
			}
			return err
		}
		if err := ensureUniqueShopSlug(tx, profile.Slug, tenantID); err != nil {
			return err
		}

		var existing models.ShopProfile
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("tenant_id = ?", tenantID).First(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			profile.TenantID = tenantID
			if err := tx.Omit(clause.Associations).Create(profile).Error; err != nil {
				if isUniqueViolation(err) {
					return ErrDuplicateShopProfileSlug
				}
				return err
			}
		case err != nil:
			return err
		default:
			updates := map[string]any{
				"slug":           profile.Slug,
				"shop_name":      profile.ShopName,
				"logo_url":       profile.LogoURL,
				"banner_url":     profile.BannerURL,
				"description":    profile.Description,
				"phone":          profile.Phone,
				"whatsapp":       profile.Whatsapp,
				"address":        profile.Address,
				"business_hours": profile.BusinessHours,
				"is_public":      profile.IsPublic,
			}
			if err := tx.Model(&existing).Updates(updates).Error; err != nil {
				if isUniqueViolation(err) {
					return ErrDuplicateShopProfileSlug
				}
				return err
			}
		}
		if err := tx.Where("tenant_id = ?", tenantID).First(&saved).Error; err != nil {
			return err
		}
		return saveShopProfileTranslationMutation(tx, saved.ID, translation)
	})
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

func ensureUniqueShopSlug(tx *gorm.DB, slug string, tenantID uint) error {
	var duplicate models.ShopProfile
	err := tx.Where("slug = ? AND tenant_id <> ?", slug, tenantID).First(&duplicate).Error
	switch {
	case err == nil:
		return ErrDuplicateShopProfileSlug
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil
	default:
		return err
	}
}

type shopProfileTranslationField struct {
	key   string
	value string
}

func saveShopProfileTranslationMutation(
	tx *gorm.DB,
	profileID uint,
	mutation ShopProfileTranslationMutation,
) error {
	if !mutation.Provided {
		return nil
	}
	fields := []shopProfileTranslationField{
		{key: "shop_name", value: strings.TrimSpace(mutation.ShopName)},
		{key: "description", value: strings.TrimSpace(mutation.Description)},
		{key: "address", value: strings.TrimSpace(mutation.Address)},
		{key: "business_hours", value: strings.TrimSpace(mutation.BusinessHours)},
	}
	now := time.Now()
	for _, field := range fields {
		if field.value == "" {
			if err := tx.Where(
				"entity_type = ? AND entity_id = ? AND field_key = ? AND locale = ?",
				"shop_profile", profileID, field.key, "zh",
			).Delete(&models.ContentTranslation{}).Error; err != nil {
				return err
			}
			continue
		}
		translation := models.ContentTranslation{
			BaseModel:  models.BaseModel{CreatedAt: now, UpdatedAt: now},
			EntityType: "shop_profile",
			EntityID:   profileID,
			FieldKey:   field.key,
			Locale:     "zh",
			ValueText:  field.value,
			Status:     models.ContentTranslationStatusPublished,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "entity_type"},
				{Name: "entity_id"},
				{Name: "field_key"},
				{Name: "locale"},
			},
			DoUpdates: clause.Assignments(map[string]any{
				"value_text": gorm.Expr("excluded.value_text"),
				"value_html": "",
				"value_json": nil,
				"status":     models.ContentTranslationStatusPublished,
				"updated_at": now,
			}),
		}).Create(&translation).Error; err != nil {
			return err
		}
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") && strings.Contains(message, "shop_profiles")
}
