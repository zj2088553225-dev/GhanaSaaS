package repository

import (
	"context"
	"errors"
	"time"

	"saas/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrAnonymousReservationDuplicate  = errors.New("anonymous reservation already exists")
	ErrAnonymousBrowserRateLimited    = errors.New("anonymous browser rate limited")
	ErrAnonymousIPRateLimited         = errors.New("anonymous IP rate limited")
	ErrAnonymousReservationNotPending = errors.New("anonymous reservation is not pending")
	ErrAnonymousReservationBadStatus  = errors.New("invalid anonymous reservation status transition")
)

type AnonymousReservationLimits struct {
	BrowserPerHour  int
	BrowserPerDay   int
	IPPerHour       int
	IPPerDay        int
	IPProductPerDay int
}

type AnonymousReservationAdminQuery struct {
	Page         int
	PageSize     int
	IdentityType *models.ReservationIdentityType
	Status       *models.ProductReservationStatus
	RiskFlagged  *bool
}

type AnonymousReservationRepository struct {
	db *gorm.DB
}

func NewAnonymousReservationRepository(db *gorm.DB) *AnonymousReservationRepository {
	return &AnonymousReservationRepository{db: db}
}

func (r *AnonymousReservationRepository) Create(
	ctx context.Context,
	reservation *models.ProductReservation,
	attempt *models.ReservationAttemptEvent,
	limits AnonymousReservationLimits,
	now time.Time,
) (*models.ProductReservation, bool, error) {
	var stored models.ProductReservation
	var duplicate bool
	var limitErr error
	now = now.UTC()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := expireAnonymousReservations(tx, now, 500); err != nil {
			return err
		}
		if reservation.ActiveIdentityKey != nil {
			if err := expireAnonymousReservationsMatching(
				tx, now, "active_identity_key = ?", *reservation.ActiveIdentityKey,
			); err != nil {
				return err
			}
		}
		if err := ensureReservationProductOpen(tx, reservation.ReservationProductID, now); err != nil {
			return err
		}

		if reservation.ActiveIdentityKey != nil {
			err := tx.Preload("ReservationProduct").
				Where("active_identity_key = ?", *reservation.ActiveIdentityKey).
				Take(&stored).Error
			if err == nil {
				duplicate = true
				if attempt != nil {
					attempt.Result = models.ReservationAttemptDuplicate
					reason := models.ReservationReasonAlreadyExists
					attempt.ReasonCode = &reason
					return tx.Create(attempt).Error
				}
				return nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}

		limitErr = anonymousReservationLimitError(tx, reservation, limits, now)
		if limitErr != nil {
			if errors.Is(limitErr, ErrAnonymousBrowserRateLimited) || errors.Is(limitErr, ErrAnonymousIPRateLimited) {
				return nil
			}
			return limitErr
		}

		result := tx.Omit(
			"VisitorID",
			"ContactType",
			"ContactHash",
			"ContactEncrypted",
			"ContactEncryptionVersion",
			"PhoneCountryCode",
			"PhoneLast4",
			"Provider",
			"ProviderReference",
			"VerificationChallengeID",
			"OTPRequestIdempotencyKey",
			"OTPVerifyIdempotencyKey",
			"CancelIdempotencyKey",
			"VerifiedAt",
			"CancelledAt",
			"ManageTokenHash",
		).Clauses(clause.OnConflict{DoNothing: true}).Create(reservation)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			if reservation.ActiveIdentityKey == nil {
				return errors.New("anonymous reservation conflict")
			}
			if err := tx.Preload("ReservationProduct").
				Where("active_identity_key = ?", *reservation.ActiveIdentityKey).
				Take(&stored).Error; err != nil {
				return err
			}
			duplicate = true
			if attempt != nil {
				attempt.Result = models.ReservationAttemptDuplicate
				reason := models.ReservationReasonAlreadyExists
				attempt.ReasonCode = &reason
				return tx.Create(attempt).Error
			}
			return nil
		}
		if err := tx.Preload("ReservationProduct").First(&stored, reservation.ID).Error; err != nil {
			return err
		}
		if attempt != nil {
			attempt.ReservationProductID = &stored.ReservationProductID
			attempt.Result = models.ReservationAttemptAccepted
			if err := tx.Create(attempt).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if limitErr != nil {
		return nil, false, limitErr
	}
	return &stored, duplicate, nil
}

func anonymousReservationLimitError(
	tx *gorm.DB,
	reservation *models.ProductReservation,
	limits AnonymousReservationLimits,
	now time.Time,
) error {
	rules := []struct {
		column    string
		hash      *string
		productID *uint64
		since     time.Time
		limit     int
		err       error
	}{
		{"browser_hash", reservation.BrowserHash, nil, now.Add(-time.Hour), limits.BrowserPerHour, ErrAnonymousBrowserRateLimited},
		{"browser_hash", reservation.BrowserHash, nil, now.Add(-24 * time.Hour), limits.BrowserPerDay, ErrAnonymousBrowserRateLimited},
		{"ip_hash", reservation.IPHash, nil, now.Add(-time.Hour), limits.IPPerHour, ErrAnonymousIPRateLimited},
		{"ip_hash", reservation.IPHash, nil, now.Add(-24 * time.Hour), limits.IPPerDay, ErrAnonymousIPRateLimited},
		{"ip_hash", reservation.IPHash, &reservation.ReservationProductID, now.Add(-24 * time.Hour), limits.IPProductPerDay, ErrAnonymousIPRateLimited},
	}
	for _, rule := range rules {
		count, err := anonymousAttemptCount(tx, rule.column, rule.hash, rule.productID, rule.since)
		if err != nil {
			return err
		}
		if count >= int64(rule.limit) {
			return rule.err
		}
	}
	return nil
}

func anonymousAttemptCount(
	tx *gorm.DB,
	column string,
	hash *string,
	productID *uint64,
	since time.Time,
) (int64, error) {
	if hash == nil || *hash == "" {
		return 0, nil
	}
	query := tx.Model(&models.ReservationAttemptEvent{}).
		Where("action = ? AND result = ? AND "+column+" = ? AND created_at >= ?",
			models.ReservationAttemptCreate, models.ReservationAttemptAccepted, *hash, since.UTC())
	if productID != nil {
		query = query.Where("reservation_product_id = ?", *productID)
	}
	var count int64
	err := query.Count(&count).Error
	return count, err
}

func (r *AnonymousReservationRepository) CreateAttemptEvent(ctx context.Context, event *models.ReservationAttemptEvent) error {
	return r.db.WithContext(ctx).Create(event).Error
}

func (r *AnonymousReservationRepository) ListByBrowser(
	ctx context.Context,
	browserHash string,
	now time.Time,
) ([]models.ProductReservation, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return expireAnonymousReservationsMatching(
			tx, now.UTC(), "identity_type = ? AND browser_hash = ?",
			models.ReservationIdentityAnonymous, browserHash,
		)
	})
	if err != nil {
		return nil, err
	}
	var reservations []models.ProductReservation
	err = r.db.WithContext(ctx).Preload("ReservationProduct").
		Where("identity_type = ? AND browser_hash = ?", models.ReservationIdentityAnonymous, browserHash).
		Order("created_at DESC, id DESC").
		Find(&reservations).Error
	return reservations, err
}

func (r *AnonymousReservationRepository) CancelByBrowser(
	ctx context.Context,
	reservationCode, browserHash string,
	now time.Time,
) (*models.ProductReservation, error) {
	var reservation models.ProductReservation
	now = now.UTC()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := expireAnonymousReservations(tx, now, 500); err != nil {
			return err
		}
		if err := expireAnonymousReservationsMatching(
			tx, now, "reservation_code = ? AND identity_type = ? AND browser_hash = ?",
			reservationCode, models.ReservationIdentityAnonymous, browserHash,
		); err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("ReservationProduct").
			Where("reservation_code = ? AND identity_type = ? AND browser_hash = ?",
				reservationCode, models.ReservationIdentityAnonymous, browserHash).
			Take(&reservation).Error; err != nil {
			return err
		}
		if reservation.Status != models.ProductReservationPending {
			return ErrAnonymousReservationNotPending
		}
		updates := map[string]any{
			"status": models.ProductReservationCancelled, "active_identity_key": nil,
			"cancelled_at": now, "updated_at": now,
		}
		return tx.Model(&models.ProductReservation{}).Where("id = ?", reservation.ID).Updates(updates).Error
	})
	if err != nil {
		return nil, err
	}
	reservation.Status = models.ProductReservationCancelled
	reservation.ActiveIdentityKey = nil
	reservation.CancelledAt = &now
	return &reservation, nil
}

func (r *AnonymousReservationRepository) ExpireDue(ctx context.Context, now time.Time, batchSize int) (int64, error) {
	var changed int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return expireAnonymousReservationsWithCount(tx, now.UTC(), batchSize, &changed)
	})
	return changed, err
}

func expireAnonymousReservations(tx *gorm.DB, now time.Time, batchSize int) error {
	var changed int64
	return expireAnonymousReservationsWithCount(tx, now, batchSize, &changed)
}

func expireAnonymousReservationsMatching(
	tx *gorm.DB,
	now time.Time,
	where string,
	args ...any,
) error {
	return tx.Model(&models.ProductReservation{}).
		Where(where, args...).
		Where("identity_type = ? AND status IN ? AND expires_at IS NOT NULL AND expires_at <= ?",
			models.ReservationIdentityAnonymous,
			[]models.ProductReservationStatus{models.ProductReservationPending, models.ProductReservationConfirmed},
			now.UTC(),
		).
		Updates(map[string]any{
			"status": models.ProductReservationExpired, "active_identity_key": nil, "updated_at": now.UTC(),
		}).Error
}

func expireAnonymousReservationsWithCount(tx *gorm.DB, now time.Time, batchSize int, changed *int64) error {
	if batchSize <= 0 || batchSize > 5000 {
		batchSize = 500
	}
	var ids []uint64
	query := tx.Model(&models.ProductReservation{}).
		Where("identity_type = ? AND status IN ? AND expires_at IS NOT NULL AND expires_at <= ?",
			models.ReservationIdentityAnonymous,
			[]models.ProductReservationStatus{models.ProductReservationPending, models.ProductReservationConfirmed},
			now.UTC()).
		Order("expires_at ASC, id ASC").
		Limit(batchSize)
	if tx.Dialector.Name() == "postgres" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
	}
	if err := query.Pluck("id", &ids).Error; err != nil || len(ids) == 0 {
		return err
	}
	result := tx.Model(&models.ProductReservation{}).
		Where("id IN ? AND identity_type = ? AND status IN ? AND expires_at <= ?",
			ids, models.ReservationIdentityAnonymous,
			[]models.ProductReservationStatus{models.ProductReservationPending, models.ProductReservationConfirmed},
			now.UTC()).
		Updates(map[string]any{
			"status": models.ProductReservationExpired, "active_identity_key": nil, "updated_at": now.UTC(),
		})
	if result.Error == nil && changed != nil {
		*changed = result.RowsAffected
	}
	return result.Error
}

func (r *AnonymousReservationRepository) AdminList(
	ctx context.Context,
	query AnonymousReservationAdminQuery,
) ([]models.ProductReservation, int64, error) {
	db := r.db.WithContext(ctx).Model(&models.ProductReservation{})
	if query.IdentityType != nil {
		db = db.Where("identity_type = ?", *query.IdentityType)
	}
	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
	}
	if query.RiskFlagged != nil {
		db = db.Where("risk_flagged = ?", *query.RiskFlagged)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var reservations []models.ProductReservation
	err := db.Preload("ReservationProduct").
		Order("created_at DESC, id DESC").
		Offset((query.Page - 1) * query.PageSize).
		Limit(query.PageSize).
		Find(&reservations).Error
	return reservations, total, err
}

func (r *AnonymousReservationRepository) AdminSetStatus(
	ctx context.Context,
	reservationCode string,
	status models.ProductReservationStatus,
	now time.Time,
) (*models.ProductReservation, error) {
	var reservation models.ProductReservation
	now = now.UTC()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := expireAnonymousReservations(tx, now, 500); err != nil {
			return err
		}
		if err := expireAnonymousReservationsMatching(
			tx, now, "reservation_code = ?", reservationCode,
		); err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("ReservationProduct").
			Where("reservation_code = ?", reservationCode).
			Take(&reservation).Error; err != nil {
			return err
		}
		if reservation.IdentityType != models.ReservationIdentityAnonymous {
			return ErrAnonymousReservationBadStatus
		}
		updates := map[string]any{"updated_at": now}
		switch status {
		case models.ProductReservationConfirmed:
			if reservation.Status != models.ProductReservationPending {
				return ErrAnonymousReservationBadStatus
			}
			updates["status"] = status
		case models.ProductReservationRejected, models.ProductReservationCancelled:
			if reservation.Status != models.ProductReservationPending && reservation.Status != models.ProductReservationConfirmed {
				return ErrAnonymousReservationBadStatus
			}
			updates["status"] = status
			updates["active_identity_key"] = nil
			if status == models.ProductReservationCancelled {
				updates["cancelled_at"] = now
			}
		default:
			return ErrAnonymousReservationBadStatus
		}
		if err := tx.Model(&models.ProductReservation{}).Where("id = ?", reservation.ID).Updates(updates).Error; err != nil {
			return err
		}
		reservation.Status = status
		if status == models.ProductReservationRejected || status == models.ProductReservationCancelled {
			reservation.ActiveIdentityKey = nil
		}
		return nil
	})
	return &reservation, err
}

func (r *AnonymousReservationRepository) AdminSetRisk(
	ctx context.Context,
	reservationCode string,
	flagged bool,
	now time.Time,
) (*models.ProductReservation, error) {
	var reservation models.ProductReservation
	var flaggedAt *time.Time
	if flagged {
		value := now.UTC()
		flaggedAt = &value
	}
	result := r.db.WithContext(ctx).Model(&models.ProductReservation{}).
		Where("reservation_code = ?", reservationCode).
		Updates(map[string]any{"risk_flagged": flagged, "risk_flagged_at": flaggedAt, "updated_at": now.UTC()})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if err := r.db.WithContext(ctx).Preload("ReservationProduct").
		Where("reservation_code = ?", reservationCode).Take(&reservation).Error; err != nil {
		return nil, err
	}
	return &reservation, nil
}

func (r *AnonymousReservationRepository) IdentityCounts(
	ctx context.Context,
	browserHash, ipHash string,
	since time.Time,
) (int64, int64, error) {
	var browserCount int64
	if browserHash != "" {
		if err := r.db.WithContext(ctx).Model(&models.ProductReservation{}).
			Where("browser_hash = ?", browserHash).Count(&browserCount).Error; err != nil {
			return 0, 0, err
		}
	}
	var ipCount int64
	if ipHash != "" {
		if err := r.db.WithContext(ctx).Model(&models.ProductReservation{}).
			Where("ip_hash = ? AND created_at >= ?", ipHash, since.UTC()).Count(&ipCount).Error; err != nil {
			return 0, 0, err
		}
	}
	return browserCount, ipCount, nil
}

func (r *AnonymousReservationRepository) WasRateLimited(
	ctx context.Context,
	browserHash, ipHash string,
	since time.Time,
) (bool, error) {
	query := r.db.WithContext(ctx).Model(&models.ReservationAttemptEvent{}).
		Where("result = ? AND reason_code IN ? AND created_at >= ?",
			models.ReservationAttemptBlocked,
			[]models.ReservationReasonCode{
				models.ReservationReasonBrowserRateLimited,
				models.ReservationReasonIPRateLimited,
			},
			since.UTC(),
		)
	switch {
	case browserHash != "" && ipHash != "":
		query = query.Where("browser_hash = ? OR ip_hash = ?", browserHash, ipHash)
	case browserHash != "":
		query = query.Where("browser_hash = ?", browserHash)
	case ipHash != "":
		query = query.Where("ip_hash = ?", ipHash)
	default:
		return false, nil
	}
	var count int64
	if err := query.Limit(1).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
