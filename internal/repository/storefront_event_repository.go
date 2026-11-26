package repository

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"saas/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type StorefrontEventRepository struct {
	db                   *gorm.DB
	platformMallTenantID uint
}

// storefrontEventWriteLogger keeps GORM's error and slow-query diagnostics but
// prevents interpolated analytics values from entering logs. Paths, referrers,
// visitor IDs, and session IDs can otherwise appear in a failed batch INSERT.
type storefrontEventWriteLogger struct {
	logger.Interface
}

func (storefrontEventWriteLogger) ParamsFilter(_ context.Context, sql string, _ ...interface{}) (string, []interface{}) {
	return sql, nil
}

func NewStorefrontEventRepository(db *gorm.DB, options ...PublicProductScopeOptions) *StorefrontEventRepository {
	repo := &StorefrontEventRepository{db: db}
	if len(options) > 0 {
		repo.platformMallTenantID = options[0].PlatformMallTenantID
	}
	return repo
}

func (r *StorefrontEventRepository) Create(ctx context.Context, event *models.StorefrontEvent) error {
	return r.writeDB(ctx).Transaction(func(tx *gorm.DB) error {
		acquired, err := acquireStorefrontEventThrottle(tx, event)
		if err != nil || !acquired {
			return err
		}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "event_id"}},
			DoNothing: true,
		}).Create(event).Error
	})
}

func (r *StorefrontEventRepository) CreateMany(ctx context.Context, events []*models.StorefrontEvent) (int, int, error) {
	if len(events) == 0 {
		return 0, 0, nil
	}
	accepted := 0
	duplicates := 0
	err := r.writeDB(ctx).Transaction(func(tx *gorm.DB) error {
		for _, event := range events {
			acquired, err := acquireStorefrontEventThrottle(tx, event)
			if err != nil {
				return err
			}
			if !acquired {
				duplicates++
				continue
			}
			result := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "event_id"}},
				DoNothing: true,
			}).Create(event)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				duplicates++
			} else {
				accepted++
			}
		}
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	return accepted, duplicates, nil
}

func acquireStorefrontEventThrottle(tx *gorm.DB, event *models.StorefrontEvent) (bool, error) {
	if strings.TrimSpace(event.ThrottleKey) == "" || event.ThrottleUntil.IsZero() {
		return true, nil
	}
	now := event.OccurredAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	result := tx.Exec(`
		INSERT INTO storefront_event_throttles
			(throttle_key, expires_at, created_at, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (throttle_key) DO UPDATE
		SET expires_at = EXCLUDED.expires_at,
		    updated_at = EXCLUDED.updated_at
		WHERE storefront_event_throttles.expires_at <= ?
	`, event.ThrottleKey, event.ThrottleUntil.UTC(), now, now, now)
	return result.RowsAffected == 1, result.Error
}

func (r *StorefrontEventRepository) writeDB(ctx context.Context) *gorm.DB {
	return r.db.Session(&gorm.Session{
		Logger: storefrontEventWriteLogger{Interface: r.db.Logger},
	}).WithContext(ctx)
}

func (r *StorefrontEventRepository) FindExistingEventIDs(ctx context.Context, eventIDs []string) (map[string]struct{}, error) {
	existing := make(map[string]struct{})
	if len(eventIDs) == 0 {
		return existing, nil
	}
	var found []string
	if err := r.db.WithContext(ctx).
		Model(&models.StorefrontEvent{}).
		Where("event_id IN ?", eventIDs).
		Pluck("event_id", &found).Error; err != nil {
		return nil, err
	}
	for _, eventID := range found {
		existing[eventID] = struct{}{}
	}
	return existing, nil
}

// FindPublicProductTenantID intentionally uses the same visibility conditions as
// the storefront catalog. Off-shelf products, disabled tenants, and private
// merchant shops cannot generate product-scoped analytics events.
func (r *StorefrontEventRepository) FindPublicProductTenantID(ctx context.Context, productID uint) (uint, error) {
	var product struct {
		TenantID uint
	}
	condition, args := publicProductSourceCondition("spus", "tenants", ProductSourceAll, r.platformMallTenantID)
	err := r.db.WithContext(ctx).
		Table("spus").
		Select("spus.tenant_id").
		Joins("JOIN tenants ON tenants.id = spus.tenant_id").
		Where("spus.id = ? AND spus.status = ? AND tenants.status = ?", productID, models.SPUStatusOnShelf, models.TenantStatusActive).
		Where(condition, args...).
		Take(&product).Error
	return product.TenantID, err
}

// FindPublicShopTenantID accepts either the stable numeric tenant ID or the
// public shop slug used by storefront routes. Both paths enforce public/active
// shop visibility before returning a tenant.
func (r *StorefrontEventRepository) FindPublicShopTenantID(ctx context.Context, identifier string) (uint, error) {
	identifier = strings.TrimSpace(identifier)
	query := r.db.WithContext(ctx).
		Model(&models.ShopProfile{}).
		Select("shop_profiles.tenant_id").
		Joins("JOIN tenants ON tenants.id = shop_profiles.tenant_id").
		Where("shop_profiles.is_public = ? AND tenants.status = ?", true, models.TenantStatusActive)

	if tenantID, err := strconv.ParseUint(identifier, 10, 64); err == nil && tenantID > 0 {
		query = query.Where("shop_profiles.tenant_id = ?", uint(tenantID))
	} else {
		query = query.Where("shop_profiles.slug = ?", identifier)
	}

	var profile struct {
		TenantID uint
	}
	err := query.Take(&profile).Error
	return profile.TenantID, err
}

func (r *StorefrontEventRepository) FindPlatformMallTenantID(ctx context.Context) (*uint, error) {
	query := r.db.WithContext(ctx).
		Model(&models.Tenant{}).
		Select("id").
		Where("status = ?", models.TenantStatusActive)
	if r.platformMallTenantID > 0 {
		query = query.Where("id = ?", r.platformMallTenantID)
	} else {
		query = query.Where("type = ? AND name = ?", models.TenantTypeSelfOperated, models.PlatformMallTenantName)
	}

	var tenant models.Tenant
	if err := query.Take(&tenant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	tenantID := tenant.ID
	return &tenantID, nil
}
