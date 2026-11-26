package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"saas/internal/models"

	"gorm.io/gorm"
)

var ErrProductPopularityRequiresPostgres = errors.New("product popularity requires PostgreSQL")

type ProductPopularityScope string

const (
	ProductPopularityScopePublic ProductPopularityScope = "public"
	ProductPopularityScopeAll    ProductPopularityScope = "all"
	ProductPopularityScopeTenant ProductPopularityScope = "tenant"
)

type ProductPopularityQuery struct {
	Scope      ProductPopularityScope
	TenantID   *uint
	CategoryID *uint
	Limit      int
	ScoredAt   time.Time
}

type ProductPopularityRow struct {
	ProductID          uint    `gorm:"column:product_id"`
	TenantID           uint    `gorm:"column:tenant_id"`
	CategoryID         *uint   `gorm:"column:category_id"`
	PlatformCategoryID uint    `gorm:"column:platform_category_id"`
	Name               string  `gorm:"column:name"`
	MainImage          string  `gorm:"column:main_image"`
	MinPrice           int64   `gorm:"column:min_price"`
	MaxPrice           int64   `gorm:"column:max_price"`
	OriginalPrice      int64   `gorm:"column:original_price"`
	HotScore           float64 `gorm:"column:hot_score"`
	Views              int64   `gorm:"column:views"`
	Clicks             int64   `gorm:"column:clicks"`
	Contacts           int64   `gorm:"column:contacts"`
	InterestCount      int64   `gorm:"column:interest_count"`
	VerifiedCount      int64   `gorm:"column:verified_count"`
}

type ProductPopularityRepository struct {
	db                   *gorm.DB
	platformMallTenantID uint
}

func NewProductPopularityRepository(db *gorm.DB, options ...PublicProductScopeOptions) *ProductPopularityRepository {
	repository := &ProductPopularityRepository{db: db}
	if len(options) > 0 {
		repository.platformMallTenantID = options[0].PlatformMallTenantID
	}
	return repository
}

func (r *ProductPopularityRepository) ListPopularProducts(
	ctx context.Context,
	query ProductPopularityQuery,
) ([]ProductPopularityRow, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("product popularity repository is not configured")
	}
	if r.db.Dialector.Name() != "postgres" {
		return nil, ErrProductPopularityRequiresPostgres
	}

	candidateFilter, candidateArgs, err := r.candidateFilter(query)
	if err != nil {
		return nil, err
	}
	if query.ScoredAt.IsZero() {
		query.ScoredAt = time.Now().UTC()
	}

	args := []any{query.ScoredAt.UTC(), models.SPUStatusOnShelf, models.TenantStatusActive}
	args = append(args, candidateArgs...)
	args = append(args,
		models.StorefrontEventProductView,
		models.StorefrontEventProductClick,
		models.StorefrontEventWhatsAppClick,
		models.StorefrontEventPhoneClick,
		models.StorefrontEventProductView,
		models.StorefrontEventProductClick,
		models.StorefrontEventWhatsAppClick,
		models.StorefrontEventPhoneClick,
		models.StorefrontTrafficHuman,
		models.StorefrontEventProductView,
		models.StorefrontEventProductClick,
		models.StorefrontEventWhatsAppClick,
		models.StorefrontEventPhoneClick,
		models.ReservationProductStatusScheduled,
		models.ReservationProductStatusOpen,
		models.ReservationProductStatusClosed,
		models.ReservationIdentityPhoneVerified,
		models.ReservationVerificationVerified,
		models.ProductReservationActive,
		models.ReservationIdentityPhoneVerified,
		models.ReservationVerificationVerified,
		models.ProductReservationActive,
		models.ReservationIdentityPhoneVerified,
		models.ReservationVerificationCancelled,
		models.ProductReservationCancelled,
		models.ReservationIdentityPhoneVerified,
		models.ReservationVerificationVerified,
		models.ProductReservationActive,
		models.ReservationIdentityPhoneVerified,
		models.ReservationVerificationCancelled,
		models.ProductReservationCancelled,
		query.Limit,
	)

	rows := make([]ProductPopularityRow, 0)
	err = r.db.WithContext(ctx).Raw(`
WITH params AS (
    SELECT ?::timestamptz AS scored_at
),
candidate_products AS (
    SELECT
        spus.id,
        spus.tenant_id,
        spus.category_id,
        spus.platform_category_id,
        spus.name,
        spus.main_image,
        spus.created_at
    FROM spus
    JOIN tenants ON tenants.id = spus.tenant_id
    WHERE spus.status = ?
      AND tenants.status = ?
      AND `+candidateFilter+`
),
analytics_scores AS (
    SELECT
        e.product_id,
        COUNT(*) FILTER (WHERE e.event_name = ?) AS views,
        COUNT(*) FILTER (WHERE e.event_name = ?) AS clicks,
        COUNT(*) FILTER (WHERE e.event_name IN (?, ?)) AS contacts,
        SUM(
            CASE e.event_name
                WHEN ? THEN 1
                WHEN ? THEN 3
                WHEN ? THEN 10
                WHEN ? THEN 10
                ELSE 0
            END *
            CASE
                WHEN e.occurred_at >= params.scored_at - INTERVAL '7 days' THEN 1.0
                WHEN e.occurred_at >= params.scored_at - INTERVAL '30 days' THEN 0.5
                ELSE 0.2
            END
        ) AS score
    FROM storefront_events e
    JOIN candidate_products products ON products.id = e.product_id
    CROSS JOIN params
    WHERE e.traffic_type = ?
      AND e.tenant_id = products.tenant_id
      AND e.occurred_at <= params.scored_at
      AND e.event_name IN (?, ?, ?, ?)
    GROUP BY e.product_id
),
interest_scores AS (
    SELECT
        reservation_products.linked_product_id AS product_id,
        COUNT(*) AS interest_count,
        SUM(
            15 *
            CASE
                WHEN interests.created_at >= params.scored_at - INTERVAL '7 days' THEN 1.0
                WHEN interests.created_at >= params.scored_at - INTERVAL '30 days' THEN 0.5
                ELSE 0.2
            END
        ) AS score
    FROM reservation_interests interests
    JOIN reservation_products ON reservation_products.id = interests.reservation_product_id
    JOIN candidate_products products ON products.id = reservation_products.linked_product_id
    CROSS JOIN params
    WHERE reservation_products.status IN (?, ?, ?)
      AND interests.created_at <= params.scored_at
    GROUP BY reservation_products.linked_product_id
),
reservation_scores AS (
    SELECT
        reservation_products.linked_product_id AS product_id,
        COUNT(*) FILTER (
            WHERE reservations.identity_type = ?
              AND reservations.verification_status = ?
              AND reservations.status = ?
              AND reservations.cancelled_at IS NULL
              AND (reservations.expires_at IS NULL OR reservations.expires_at > params.scored_at)
        ) AS verified_count,
        SUM(
            CASE
                WHEN reservations.identity_type = ?
                  AND reservations.verification_status = ?
                  AND reservations.status = ?
                  AND reservations.cancelled_at IS NULL
                  AND (reservations.expires_at IS NULL OR reservations.expires_at > params.scored_at)
                    THEN 30
                WHEN reservations.identity_type = ?
                  AND reservations.verification_status = ?
                  AND reservations.status = ?
                  AND reservations.cancelled_at IS NOT NULL
                    THEN -10
                ELSE 0
            END *
            CASE
                WHEN reservations.created_at >= params.scored_at - INTERVAL '7 days' THEN 1.0
                WHEN reservations.created_at >= params.scored_at - INTERVAL '30 days' THEN 0.5
                ELSE 0.2
            END
        ) AS score
    FROM product_reservations reservations
    JOIN reservation_products ON reservation_products.id = reservations.reservation_product_id
    JOIN candidate_products products ON products.id = reservation_products.linked_product_id
    CROSS JOIN params
    WHERE reservations.created_at <= params.scored_at
      AND (
          (
              reservations.identity_type = ?
              AND reservations.verification_status = ?
              AND reservations.status = ?
              AND reservations.cancelled_at IS NULL
              AND (reservations.expires_at IS NULL OR reservations.expires_at > params.scored_at)
          ) OR (
              reservations.identity_type = ?
              AND reservations.verification_status = ?
              AND reservations.status = ?
              AND reservations.cancelled_at IS NOT NULL
          )
      )
    GROUP BY reservation_products.linked_product_id
),
sku_prices AS (
    SELECT
        skus.product_id,
        MIN(skus.retail_price) AS min_price,
        MAX(skus.retail_price) AS max_price,
        COALESCE(MIN(skus.original_price) FILTER (WHERE skus.original_price > 0), 0) AS original_price
    FROM skus
    JOIN candidate_products products ON products.id = skus.product_id
    WHERE skus.is_active = TRUE
    GROUP BY skus.product_id
),
ranked AS (
    SELECT
        products.id AS product_id,
        products.tenant_id,
        products.category_id,
        products.platform_category_id,
        products.name,
        products.main_image,
        products.created_at,
        COALESCE(prices.min_price, 0) AS min_price,
        COALESCE(prices.max_price, 0) AS max_price,
        COALESCE(prices.original_price, 0) AS original_price,
        ROUND((
            COALESCE(analytics.score, 0) +
            COALESCE(interests.score, 0) +
            COALESCE(reservations.score, 0)
        )::numeric, 2)::double precision AS hot_score,
        COALESCE(analytics.views, 0) AS views,
        COALESCE(analytics.clicks, 0) AS clicks,
        COALESCE(analytics.contacts, 0) AS contacts,
        COALESCE(interests.interest_count, 0) AS interest_count,
        COALESCE(reservations.verified_count, 0) AS verified_count,
        analytics.product_id IS NOT NULL
            OR interests.product_id IS NOT NULL
            OR reservations.product_id IS NOT NULL AS has_signal
    FROM candidate_products products
    LEFT JOIN analytics_scores analytics ON analytics.product_id = products.id
    LEFT JOIN interest_scores interests ON interests.product_id = products.id
    LEFT JOIN reservation_scores reservations ON reservations.product_id = products.id
    LEFT JOIN sku_prices prices ON prices.product_id = products.id
)
SELECT
    product_id,
    tenant_id,
    category_id,
    platform_category_id,
    name,
    main_image,
    min_price,
    max_price,
    original_price,
    hot_score,
    views,
    clicks,
    contacts,
    interest_count,
    verified_count
FROM ranked
ORDER BY hot_score DESC, has_signal DESC, created_at DESC, product_id DESC
LIMIT ?
`, args...).Scan(&rows).Error
	return rows, err
}

func (r *ProductPopularityRepository) candidateFilter(query ProductPopularityQuery) (string, []any, error) {
	conditions := make([]string, 0, 2)
	args := make([]any, 0, 4)
	switch query.Scope {
	case ProductPopularityScopePublic:
		condition, publicArgs := publicProductSourceCondition("spus", "tenants", ProductSourceAll, r.platformMallTenantID)
		conditions = append(conditions, condition)
		args = append(args, publicArgs...)
	case ProductPopularityScopeAll:
	case ProductPopularityScopeTenant:
		if query.TenantID == nil || *query.TenantID == 0 {
			return "", nil, errors.New("tenant popularity scope requires tenant id")
		}
		conditions = append(conditions, "spus.tenant_id = ?")
		args = append(args, *query.TenantID)
	default:
		return "", nil, errors.New("invalid product popularity scope")
	}
	if query.CategoryID != nil {
		if *query.CategoryID == 0 {
			return "", nil, errors.New("invalid product popularity category")
		}
		conditions = append(conditions, "spus.category_id = ?")
		args = append(args, *query.CategoryID)
	}
	if len(conditions) == 0 {
		return "TRUE", args, nil
	}
	return strings.Join(conditions, " AND "), args, nil
}
