package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"saas/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrReservationIdempotencyConflict = errors.New("reservation idempotency key belongs to another request")
	ErrReservationProductNotOpen      = errors.New("reservation product is not open")
)

type ReservationProductRepository struct {
	db *gorm.DB
}

type ReservationProductListQuery struct {
	Page       int
	PageSize   int
	Search     string
	Featured   *bool
	Status     *models.ReservationProductStatus
	PublicOnly bool
	Now        time.Time
}

func NewReservationProductRepository(db *gorm.DB) *ReservationProductRepository {
	return &ReservationProductRepository{db: db}
}

func (r *ReservationProductRepository) Create(ctx context.Context, product *models.ReservationProduct) error {
	return r.db.WithContext(ctx).Create(product).Error
}

func (r *ReservationProductRepository) Update(ctx context.Context, product *models.ReservationProduct) error {
	updates := map[string]any{
		"linked_product_id": product.LinkedProductID, "slug": product.Slug, "title_en": product.TitleEN,
		"title_zh": product.TitleZH, "summary_en": product.SummaryEN, "summary_zh": product.SummaryZH,
		"description_en": product.DescriptionEN, "description_zh": product.DescriptionZH,
		"main_image_url": product.MainImageURL, "gallery_json": product.GalleryJSON, "status": product.Status,
		"reservation_start_at": product.ReservationStartAt, "reservation_end_at": product.ReservationEndAt,
		"display_order": product.DisplayOrder, "is_featured": product.IsFeatured,
		"heat_level_override": product.HeatLevelOverride, "updated_at": time.Now().UTC(),
	}
	result := r.db.WithContext(ctx).Model(&models.ReservationProduct{}).Where("id = ?", product.ID).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *ReservationProductRepository) Archive(ctx context.Context, id uint64) error {
	result := r.db.WithContext(ctx).Model(&models.ReservationProduct{}).
		Where("id = ? AND status <> ?", id, models.ReservationProductStatusArchived).
		Updates(map[string]any{"status": models.ReservationProductStatusArchived, "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		var count int64
		if err := r.db.WithContext(ctx).Model(&models.ReservationProduct{}).Where("id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return gorm.ErrRecordNotFound
		}
	}
	return nil
}

func (r *ReservationProductRepository) FindByID(ctx context.Context, id uint64) (*models.ReservationProduct, error) {
	var product models.ReservationProduct
	if err := r.db.WithContext(ctx).First(&product, id).Error; err != nil {
		return nil, err
	}
	return &product, nil
}

func (r *ReservationProductRepository) FindPublicByID(ctx context.Context, id uint64, now time.Time) (*models.ReservationProduct, error) {
	var product models.ReservationProduct
	err := publicReservationProductScope(r.db.WithContext(ctx), now).Where("id = ?", id).Take(&product).Error
	if err != nil {
		return nil, err
	}
	return &product, nil
}

func (r *ReservationProductRepository) FindPublicBySlug(ctx context.Context, slug string, now time.Time) (*models.ReservationProduct, error) {
	var product models.ReservationProduct
	err := publicReservationProductScope(r.db.WithContext(ctx), now).Where("slug = ?", slug).Take(&product).Error
	if err != nil {
		return nil, err
	}
	return &product, nil
}

func (r *ReservationProductRepository) List(ctx context.Context, query ReservationProductListQuery) ([]models.ReservationProduct, int64, error) {
	db := r.db.WithContext(ctx).Model(&models.ReservationProduct{})
	if query.PublicOnly {
		db = publicReservationProductScope(db, query.Now)
	}
	if query.Featured != nil {
		db = db.Where("is_featured = ?", *query.Featured)
	}
	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
	}
	if search := strings.ToLower(strings.TrimSpace(query.Search)); search != "" {
		pattern := "%" + search + "%"
		db = db.Where("LOWER(slug) LIKE ? OR LOWER(title_en) LIKE ? OR LOWER(COALESCE(title_zh, '')) LIKE ? OR LOWER(summary_en) LIKE ?", pattern, pattern, pattern, pattern)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var products []models.ReservationProduct
	err := db.Order("is_featured DESC, display_order ASC, created_at DESC, id DESC").
		Offset((query.Page - 1) * query.PageSize).Limit(query.PageSize).Find(&products).Error
	return products, total, err
}

func (r *ReservationProductRepository) LinkedProductExists(ctx context.Context, id uint) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.SPU{}).Where("id = ?", id).Count(&count).Error
	return count > 0, err
}

func (r *ReservationProductRepository) CountInterests(ctx context.Context, productID uint64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.ReservationInterest{}).
		Where("reservation_product_id = ?", productID).Count(&count).Error
	return count, err
}

func (r *ReservationProductRepository) CountActiveVerified(ctx context.Context, productID uint64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.ProductReservation{}).
		Where("reservation_product_id = ? AND verification_status = ? AND status = ?", productID, models.ReservationVerificationVerified, models.ProductReservationActive).
		Count(&count).Error
	return count, err
}

func (r *ReservationProductRepository) HasActiveVerifiedByVisitor(ctx context.Context, productID uint64, visitorID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.ProductReservation{}).
		Where("reservation_product_id = ? AND visitor_id = ? AND verification_status = ? AND status = ?", productID, visitorID, models.ReservationVerificationVerified, models.ProductReservationActive).
		Count(&count).Error
	return count > 0, err
}

func (r *ReservationProductRepository) Metrics(ctx context.Context, productID uint64) (ReservationProductMetrics, error) {
	metrics := ReservationProductMetrics{}
	queries := []struct {
		target *int64
		query  *gorm.DB
	}{
		{&metrics.UniqueInterests, r.db.WithContext(ctx).Model(&models.ReservationInterest{}).Where("reservation_product_id = ?", productID)},
		{&metrics.ReservationSubmissions, r.db.WithContext(ctx).Model(&models.ProductReservation{}).Where("reservation_product_id = ?", productID)},
		// A completed verification remains a funnel completion after the user
		// cancels. Active verified reservations are counted separately by
		// CountActiveVerified for storefront heat and current demand.
		{&metrics.VerifiedReservations, r.db.WithContext(ctx).Model(&models.ProductReservation{}).Where("reservation_product_id = ? AND verified_at IS NOT NULL", productID)},
		{&metrics.CancelledReservations, r.db.WithContext(ctx).Model(&models.ProductReservation{}).Where("reservation_product_id = ? AND status = ?", productID, models.ProductReservationCancelled)},
		{&metrics.BlockedAttempts, r.db.WithContext(ctx).Model(&models.ReservationAttemptEvent{}).Where("reservation_product_id = ? AND result = ?", productID, models.ReservationAttemptBlocked)},
	}
	for _, item := range queries {
		if err := item.query.Count(item.target).Error; err != nil {
			return ReservationProductMetrics{}, err
		}
	}
	return metrics, nil
}

func (r *ReservationProductRepository) ListReservationContacts(ctx context.Context, productID uint64) ([]models.ProductReservation, error) {
	var reservations []models.ProductReservation
	err := r.db.WithContext(ctx).
		Where("reservation_product_id = ? AND verified_at IS NOT NULL", productID).
		Order("verified_at DESC, id DESC").Find(&reservations).Error
	return reservations, err
}

func (r *ReservationProductRepository) HasInterest(ctx context.Context, productID uint64, visitorID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.ReservationInterest{}).
		Where("reservation_product_id = ? AND visitor_id = ?", productID, visitorID).Count(&count).Error
	return count > 0, err
}

func (r *ReservationProductRepository) CreateInterest(
	ctx context.Context,
	interest *models.ReservationInterest,
	attempt *models.ReservationAttemptEvent,
) (bool, error) {
	var created bool
	var outcomeErr error
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureReservationProductOpen(tx, interest.ReservationProductID, interest.CreatedAt); err != nil {
			return err
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(interest)
		if result.Error != nil {
			return result.Error
		}
		created = result.RowsAffected == 1
		if !created {
			var idempotent models.ReservationInterest
			err := tx.Where("idempotency_key = ?", interest.IdempotencyKey).Take(&idempotent).Error
			if err == nil {
				if idempotent.ReservationProductID != interest.ReservationProductID || idempotent.VisitorID != interest.VisitorID {
					outcomeErr = ErrReservationIdempotencyConflict
				}
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			} else {
				var duplicate models.ReservationInterest
				err = tx.Where("reservation_product_id = ? AND visitor_id = ?", interest.ReservationProductID, interest.VisitorID).Take(&duplicate).Error
				if err != nil {
					return err
				}
			}
		}
		if attempt != nil {
			if created {
				attempt.Result = models.ReservationAttemptAccepted
			} else {
				attempt.Result = models.ReservationAttemptDuplicate
				reason := models.ReservationReasonAlreadyExists
				attempt.ReasonCode = &reason
			}
			if err := tx.Create(attempt).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return created, outcomeErr
}

func (r *ReservationProductRepository) CreateAttemptEvent(ctx context.Context, event *models.ReservationAttemptEvent) error {
	return r.db.WithContext(ctx).Create(event).Error
}

func publicReservationProductScope(db *gorm.DB, now time.Time) *gorm.DB {
	now = now.UTC()
	return db.Where("status = ?", models.ReservationProductStatusOpen).
		Where("reservation_start_at IS NULL OR reservation_start_at <= ?", now).
		Where("reservation_end_at IS NULL OR reservation_end_at > ?", now)
}

type ReservationProductMetrics struct {
	UniqueInterests        int64
	ReservationSubmissions int64
	VerifiedReservations   int64
	CancelledReservations  int64
	BlockedAttempts        int64
}

type ReservationWorkflowRepository struct {
	db *gorm.DB
}

type ReservationFinalizeResult struct {
	Won             bool
	AlreadyVerified bool
}

func NewReservationWorkflowRepository(db *gorm.DB) *ReservationWorkflowRepository {
	return &ReservationWorkflowRepository{db: db}
}

func (r *ReservationWorkflowRepository) FindPublicProductByID(ctx context.Context, id uint64, now time.Time) (*models.ReservationProduct, error) {
	var product models.ReservationProduct
	if err := publicReservationProductScope(r.db.WithContext(ctx), now).Where("id = ?", id).Take(&product).Error; err != nil {
		return nil, err
	}
	return &product, nil
}

func (r *ReservationWorkflowRepository) FindByRequestIdempotency(ctx context.Context, key string) (*models.ProductReservation, error) {
	var reservation models.ProductReservation
	if err := r.db.WithContext(ctx).Where("otp_request_idempotency_key = ?", key).Take(&reservation).Error; err != nil {
		return nil, err
	}
	return &reservation, nil
}

func (r *ReservationWorkflowRepository) FindByProductContact(ctx context.Context, productID uint64, contactHash string) (*models.ProductReservation, error) {
	var reservation models.ProductReservation
	if err := r.db.WithContext(ctx).Where("reservation_product_id = ? AND contact_hash = ?", productID, contactHash).Take(&reservation).Error; err != nil {
		return nil, err
	}
	return &reservation, nil
}

func (r *ReservationWorkflowRepository) FindByChallenge(ctx context.Context, productID uint64, challengeID string) (*models.ProductReservation, error) {
	var reservation models.ProductReservation
	if err := r.db.WithContext(ctx).Where("reservation_product_id = ? AND verification_challenge_id = ?", productID, challengeID).Take(&reservation).Error; err != nil {
		return nil, err
	}
	return &reservation, nil
}

func (r *ReservationWorkflowRepository) UpsertPending(
	ctx context.Context,
	reservation *models.ProductReservation,
	attempt *models.ReservationAttemptEvent,
) (*models.ProductReservation, error) {
	var stored models.ProductReservation
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureReservationProductOpen(tx, reservation.ReservationProductID, reservation.VerificationRequestedAt); err != nil {
			return err
		}
		err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "reservation_product_id"}, {Name: "contact_hash"}},
			DoUpdates: clause.Assignments(map[string]any{
				"visitor_id": reservation.VisitorID, "contact_type": reservation.ContactType,
				"contact_encrypted": reservation.ContactEncrypted, "contact_encryption_version": reservation.ContactEncryptionVersion,
				"phone_country_code": reservation.PhoneCountryCode, "phone_last4": reservation.PhoneLast4,
				"verification_status": models.ReservationVerificationPending, "status": models.ProductReservationActive,
				"verified_at": nil, "cancelled_at": nil, "manage_token_hash": nil, "cancel_idempotency_key": nil,
				"provider":           reservation.Provider,
				"provider_reference": reservation.ProviderReference, "verification_challenge_id": reservation.VerificationChallengeID,
				"otp_request_idempotency_key": reservation.OTPRequestIdempotencyKey,
				"otp_verify_idempotency_key":  nil, "verification_requested_at": reservation.VerificationRequestedAt,
				"turnstile_verified_at": reservation.TurnstileVerifiedAt, "verification_attempt_count": 0,
				"source_type": reservation.SourceType, "source_id": reservation.SourceID, "updated_at": reservation.UpdatedAt,
			}),
		}).Create(reservation).Error
		if err != nil {
			return err
		}
		if err := tx.Where("reservation_product_id = ? AND contact_hash = ?", reservation.ReservationProductID, reservation.ContactHash).Take(&stored).Error; err != nil {
			return err
		}
		if attempt != nil {
			attempt.ReservationProductID = &stored.ReservationProductID
			if err := tx.Create(attempt).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return &stored, err
}

func (r *ReservationWorkflowRepository) RecordVerificationFailure(
	ctx context.Context,
	productID uint64,
	challengeID string,
	maxAttempts int,
	attempt *models.ReservationAttemptEvent,
) (*models.ProductReservation, error) {
	var reservation models.ProductReservation
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("reservation_product_id = ? AND verification_challenge_id = ?", productID, challengeID).Take(&reservation).Error; err != nil {
			return err
		}
		if reservation.VerificationStatus != models.ReservationVerificationPending {
			return nil
		}
		reservation.VerificationAttemptCount++
		updates := map[string]any{"verification_attempt_count": reservation.VerificationAttemptCount, "updated_at": time.Now().UTC()}
		if reservation.VerificationAttemptCount >= maxAttempts {
			reservation.VerificationStatus = models.ReservationVerificationFailed
			updates["verification_status"] = models.ReservationVerificationFailed
		}
		if err := tx.Model(&models.ProductReservation{}).Where("id = ?", reservation.ID).Updates(updates).Error; err != nil {
			return err
		}
		if attempt != nil {
			if err := tx.Create(attempt).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return &reservation, err
}

func (r *ReservationWorkflowRepository) MarkExpired(
	ctx context.Context,
	productID uint64,
	challengeID string,
	attempt *models.ReservationAttemptEvent,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.ProductReservation{}).
			Where("reservation_product_id = ? AND verification_challenge_id = ? AND verification_status = ?", productID, challengeID, models.ReservationVerificationPending).
			Updates(map[string]any{"verification_status": models.ReservationVerificationExpired, "updated_at": time.Now().UTC()})
		if result.Error != nil {
			return result.Error
		}
		if attempt != nil {
			return tx.Create(attempt).Error
		}
		return nil
	})
}

func (r *ReservationWorkflowRepository) FinalizeVerification(
	ctx context.Context,
	productID uint64,
	challengeID string,
	verifyIdempotencyKey string,
	manageTokenHash string,
	verifiedAt time.Time,
	attempt *models.ReservationAttemptEvent,
) (ReservationFinalizeResult, error) {
	result := ReservationFinalizeResult{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var reservation models.ProductReservation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("reservation_product_id = ? AND verification_challenge_id = ?", productID, challengeID).Take(&reservation).Error; err != nil {
			return err
		}
		if reservation.VerificationStatus == models.ReservationVerificationVerified && reservation.Status == models.ProductReservationActive {
			result.AlreadyVerified = true
			if attempt != nil {
				attempt.Result = models.ReservationAttemptDuplicate
				reason := models.ReservationReasonAlreadyExists
				attempt.ReasonCode = &reason
				return tx.Create(attempt).Error
			}
			return nil
		}
		if reservation.VerificationStatus != models.ReservationVerificationPending {
			return gorm.ErrRecordNotFound
		}
		if err := ensureReservationProductOpen(tx, productID, verifiedAt); err != nil {
			return err
		}
		updates := map[string]any{
			"verification_status": models.ReservationVerificationVerified,
			"status":              models.ProductReservationActive,
			"verified_at":         verifiedAt.UTC(), "cancelled_at": nil,
			"manage_token_hash": manageTokenHash, "otp_verify_idempotency_key": verifyIdempotencyKey,
			"updated_at": verifiedAt.UTC(),
		}
		if err := tx.Model(&models.ProductReservation{}).Where("id = ?", reservation.ID).Updates(updates).Error; err != nil {
			return err
		}
		result.Won = true
		if attempt != nil {
			return tx.Create(attempt).Error
		}
		return nil
	})
	return result, err
}

func (r *ReservationWorkflowRepository) Cancel(
	ctx context.Context,
	productID uint64,
	manageTokenHash string,
	idempotencyKey string,
	cancelledAt time.Time,
	attempt *models.ReservationAttemptEvent,
) (bool, error) {
	cancelled := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var reservation models.ProductReservation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("reservation_product_id = ? AND manage_token_hash = ?", productID, manageTokenHash).Take(&reservation).Error; err != nil {
			return err
		}
		if reservation.Status == models.ProductReservationCancelled {
			if attempt != nil {
				attempt.VisitorID = &reservation.VisitorID
				attempt.ContactHash = &reservation.ContactHash
				attempt.Result = models.ReservationAttemptDuplicate
				reason := models.ReservationReasonAlreadyCancelled
				attempt.ReasonCode = &reason
				return tx.Create(attempt).Error
			}
			return nil
		}
		updates := map[string]any{
			"verification_status": models.ReservationVerificationCancelled,
			"status":              models.ProductReservationCancelled,
			"cancelled_at":        cancelledAt.UTC(), "cancel_idempotency_key": idempotencyKey,
			"updated_at": cancelledAt.UTC(),
		}
		if err := tx.Model(&models.ProductReservation{}).Where("id = ?", reservation.ID).Updates(updates).Error; err != nil {
			return err
		}
		cancelled = true
		if attempt != nil {
			attempt.VisitorID = &reservation.VisitorID
			attempt.ContactHash = &reservation.ContactHash
			return tx.Create(attempt).Error
		}
		return nil
	})
	return cancelled, err
}

func (r *ReservationWorkflowRepository) CreateAttemptEvent(ctx context.Context, event *models.ReservationAttemptEvent) error {
	return r.db.WithContext(ctx).Create(event).Error
}

func ensureReservationProductOpen(tx *gorm.DB, productID uint64, at time.Time) error {
	var product models.ReservationProduct
	err := tx.Clauses(clause.Locking{Strength: "SHARE"}).
		Where("id = ? AND status = ?", productID, models.ReservationProductStatusOpen).
		Where("reservation_start_at IS NULL OR reservation_start_at <= ?", at.UTC()).
		Where("reservation_end_at IS NULL OR reservation_end_at > ?", at.UTC()).
		Take(&product).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrReservationProductNotOpen
	}
	return err
}
