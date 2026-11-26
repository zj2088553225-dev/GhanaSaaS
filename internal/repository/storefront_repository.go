package repository

import (
	"context"
	"strings"
	"time"

	"saas/internal/models"

	"gorm.io/gorm"
)

type ProductSource string

const (
	ProductSourceAll      ProductSource = "all"
	ProductSourceT6       ProductSource = "t6"
	ProductSourceMerchant ProductSource = "merchant"
)

type ProductQuery struct {
	CategoryID          *uint
	PlatformCategoryID  *uint
	PlatformCategoryIDs []uint
	TenantID            *uint
	Keyword             string
	Source              ProductSource
	IsSelfOperated      bool
	Sort                string
	Page                int
	PageSize            int
}

type PublicShopProfileQuery struct {
	Page     int
	PageSize int
}

type SitemapShopURL struct {
	Slug      string
	UpdatedAt time.Time
}

type SitemapProductURL struct {
	ID        uint
	Name      string
	UpdatedAt time.Time
}

type StorefrontRepository struct {
	db                   *gorm.DB
	platformMallTenantID uint
}

func NewStorefrontRepository(db *gorm.DB, options ...PublicProductScopeOptions) *StorefrontRepository {
	repo := &StorefrontRepository{db: db}
	if len(options) > 0 {
		repo.platformMallTenantID = options[0].PlatformMallTenantID
	}
	return repo
}

func (r *StorefrontRepository) ListActiveCategories(ctx context.Context) ([]models.Category, error) {
	var categories []models.Category
	condition, args := publicProductSourceCondition("spus", "tenants", ProductSourceAll, r.platformMallTenantID)
	visibleCategoryCondition := "EXISTS (SELECT 1 FROM spus JOIN tenants ON tenants.id = spus.tenant_id WHERE spus.category_id = categories.id AND spus.status = ? AND tenants.status = ? AND " + condition + ")"
	visibleCategoryArgs := []any{models.SPUStatusOnShelf, models.TenantStatusActive}
	visibleCategoryArgs = append(visibleCategoryArgs, args...)

	queryArgs := []any{true}
	queryArgs = append(queryArgs, visibleCategoryArgs...)
	queryArgs = append(queryArgs, true)

	err := r.db.WithContext(ctx).Raw(`
		WITH RECURSIVE visible_categories AS (
			SELECT categories.*
			FROM categories
			WHERE categories.is_active = ? AND `+visibleCategoryCondition+`
			UNION
			SELECT parent.*
			FROM categories parent
			JOIN visible_categories child ON child.parent_id = parent.id
			WHERE parent.is_active = ?
		)
		SELECT *
		FROM visible_categories
		ORDER BY sort ASC, id ASC
	`, queryArgs...).Scan(&categories).Error
	return categories, err
}

func (r *StorefrontRepository) ListProducts(ctx context.Context, query ProductQuery) ([]models.SPU, int64, error) {
	db := r.productBaseQuery(ctx, query)

	var total int64
	if err := db.Session(&gorm.Session{}).Model(&models.SPU{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var products []models.SPU
	err := applyProductSort(db.Session(&gorm.Session{}).
		Select("spus.*").
		Preload("SKUs", "is_active = ?", true), query.Sort).
		Limit(query.PageSize).
		Offset((query.Page - 1) * query.PageSize).
		Find(&products).Error
	if err != nil {
		return nil, 0, err
	}

	return products, total, nil
}

func applyProductSort(db *gorm.DB, sort string) *gorm.DB {
	switch strings.TrimSpace(strings.ToLower(sort)) {
	case "hot":
		return db.
			Order("(SELECT COALESCE(SUM(sellable_stock), 0) FROM skus WHERE skus.product_id = spus.id AND skus.is_active = true) DESC").
			Order("spus.id DESC")
	case "price_asc":
		return db.
			Order("(SELECT COALESCE(MIN(retail_price), 9223372036854775807) FROM skus WHERE skus.product_id = spus.id AND skus.is_active = true) ASC").
			Order("spus.id DESC")
	case "price_desc":
		return db.
			Order("(SELECT COALESCE(MIN(retail_price), 0) FROM skus WHERE skus.product_id = spus.id AND skus.is_active = true) DESC").
			Order("spus.id DESC")
	default:
		return db.Order("spus.id DESC")
	}
}

func (r *StorefrontRepository) FindPublicShopProfileBySlug(ctx context.Context, slug string) (*models.ShopProfile, error) {
	var profile models.ShopProfile
	err := r.db.WithContext(ctx).
		Model(&models.ShopProfile{}).
		Joins("JOIN tenants ON tenants.id = shop_profiles.tenant_id").
		Where("shop_profiles.slug = ? AND shop_profiles.is_public = ? AND tenants.status = ?", slug, true, models.TenantStatusActive).
		First(&profile).Error
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

func (r *StorefrontRepository) ListPublicShopProfiles(ctx context.Context, query PublicShopProfileQuery) ([]models.ShopProfile, int64, error) {
	db := r.db.WithContext(ctx).
		Model(&models.ShopProfile{}).
		Joins("JOIN tenants ON tenants.id = shop_profiles.tenant_id").
		Where("shop_profiles.is_public = ? AND tenants.status = ?", true, models.TenantStatusActive)

	var total int64
	if err := db.Session(&gorm.Session{}).Model(&models.ShopProfile{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var profiles []models.ShopProfile
	err := db.Session(&gorm.Session{}).
		Order("id DESC").
		Limit(query.PageSize).
		Offset((query.Page - 1) * query.PageSize).
		Find(&profiles).Error
	if err != nil {
		return nil, 0, err
	}
	return profiles, total, nil
}

func (r *StorefrontRepository) ListSitemapShops(ctx context.Context) ([]SitemapShopURL, error) {
	var shops []SitemapShopURL
	err := r.db.WithContext(ctx).
		Model(&models.ShopProfile{}).
		Select("shop_profiles.slug, shop_profiles.updated_at").
		Joins("JOIN tenants ON tenants.id = shop_profiles.tenant_id").
		Where("shop_profiles.is_public = ? AND shop_profiles.slug <> ? AND tenants.status = ?", true, "", models.TenantStatusActive).
		Order("shop_profiles.updated_at DESC, shop_profiles.id DESC").
		Find(&shops).Error
	return shops, err
}

func (r *StorefrontRepository) ListSitemapProducts(ctx context.Context) ([]SitemapProductURL, error) {
	var products []SitemapProductURL
	err := r.productBaseQuery(ctx, ProductQuery{Source: ProductSourceAll}).
		Select("spus.id, spus.name, spus.updated_at").
		Order("spus.updated_at DESC, spus.id DESC").
		Find(&products).Error
	return products, err
}

func (r *StorefrontRepository) FindProductByID(ctx context.Context, id uint) (*models.SPU, error) {
	var product models.SPU
	err := r.productBaseQuery(ctx, ProductQuery{Source: ProductSourceAll}).
		Select("spus.*").
		Preload("SKUs", "is_active = ?", true).
		Preload("Tenant").
		Where("spus.id = ?", id).
		First(&product).Error
	if err != nil {
		return nil, err
	}
	return &product, nil
}

func (r *StorefrontRepository) FindPublicShopProfileByTenantID(ctx context.Context, tenantID uint) (*models.ShopProfile, error) {
	var profile models.ShopProfile
	err := r.db.WithContext(ctx).
		Model(&models.ShopProfile{}).
		Joins("JOIN tenants ON tenants.id = shop_profiles.tenant_id").
		Where("shop_profiles.tenant_id = ? AND shop_profiles.is_public = ? AND tenants.status = ?", tenantID, true, models.TenantStatusActive).
		First(&profile).Error
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

func (r *StorefrontRepository) FindPlatformCategoryByID(ctx context.Context, id uint) (*models.PlatformCategory, error) {
	var category models.PlatformCategory
	if err := r.db.WithContext(ctx).First(&category, id).Error; err != nil {
		return nil, err
	}
	return &category, nil
}

func (r *StorefrontRepository) ListPlatformCategories(ctx context.Context, activeOnly bool) ([]models.PlatformCategory, error) {
	var categories []models.PlatformCategory
	db := r.db.WithContext(ctx)
	if activeOnly {
		db = db.Where("status = ?", models.PlatformCategoryStatusActive)
	}
	if err := db.Order("level ASC, sort_order ASC, id ASC").Find(&categories).Error; err != nil {
		return nil, err
	}
	return categories, nil
}

func (r *StorefrontRepository) ListPlatformCategoryIDsByRoot(ctx context.Context, id uint) ([]uint, error) {
	var root models.PlatformCategory
	if err := r.db.WithContext(ctx).First(&root, id).Error; err != nil {
		return nil, err
	}
	if root.IsLeaf {
		return []uint{id}, nil
	}

	var ids []uint
	err := r.db.WithContext(ctx).
		Model(&models.PlatformCategory{}).
		Where("id = ? OR path LIKE ?", id, root.Path+"/%").
		Pluck("id", &ids).Error
	return ids, err
}

func (r *StorefrontRepository) productBaseQuery(ctx context.Context, query ProductQuery) *gorm.DB {
	db := r.db.WithContext(ctx).Model(&models.SPU{}).Where("spus.status = ?", models.SPUStatusOnShelf)

	if query.CategoryID != nil {
		db = db.Where("spus.category_id = ?", *query.CategoryID)
	}
	if len(query.PlatformCategoryIDs) > 0 {
		db = db.Where("spus.platform_category_id IN ?", query.PlatformCategoryIDs)
	} else if query.PlatformCategoryID != nil {
		db = db.Where("spus.platform_category_id = ?", *query.PlatformCategoryID)
	}
	if query.TenantID != nil {
		db = db.Where("spus.tenant_id = ?", *query.TenantID)
	} else {
		db = r.applyPublicProductSourceScope(db, normalizedProductSource(query))
	}
	if strings.TrimSpace(query.Keyword) != "" {
		db = db.Where("spus.name ILIKE ?", "%"+strings.TrimSpace(query.Keyword)+"%")
	}

	return db
}

func normalizedProductSource(query ProductQuery) ProductSource {
	switch query.Source {
	case ProductSourceT6, ProductSourceMerchant:
		return query.Source
	case ProductSourceAll, "":
		if query.IsSelfOperated {
			return ProductSourceT6
		}
		return ProductSourceAll
	default:
		return ProductSourceAll
	}
}

func (r *StorefrontRepository) applyPublicProductSourceScope(db *gorm.DB, source ProductSource) *gorm.DB {
	db = db.
		Joins("JOIN tenants ON tenants.id = spus.tenant_id").
		Where("tenants.status = ?", models.TenantStatusActive)

	condition, args := publicProductSourceCondition("spus", "tenants", source, r.platformMallTenantID)
	return db.Where(condition, args...)
}
