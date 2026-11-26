package models

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type ReservationProductStatus string

const (
	ReservationProductStatusDraft     ReservationProductStatus = "draft"
	ReservationProductStatusScheduled ReservationProductStatus = "scheduled"
	ReservationProductStatusOpen      ReservationProductStatus = "open"
	ReservationProductStatusClosed    ReservationProductStatus = "closed"
	ReservationProductStatusArchived  ReservationProductStatus = "archived"
)

func (status ReservationProductStatus) IsValid() bool {
	switch status {
	case ReservationProductStatusDraft,
		ReservationProductStatusScheduled,
		ReservationProductStatusOpen,
		ReservationProductStatusClosed,
		ReservationProductStatusArchived:
		return true
	default:
		return false
	}
}

type ReservationHeatLevel string

const (
	ReservationHeatNew             ReservationHeatLevel = "new"
	ReservationHeatPopular         ReservationHeatLevel = "popular"
	ReservationHeatTrending        ReservationHeatLevel = "trending"
	ReservationHeatHighlyRequested ReservationHeatLevel = "highly_requested"
)

func (level ReservationHeatLevel) IsValid() bool {
	switch level {
	case ReservationHeatNew, ReservationHeatPopular, ReservationHeatTrending, ReservationHeatHighlyRequested:
		return true
	default:
		return false
	}
}

type ReservationSourceType string

const (
	ReservationSourceList   ReservationSourceType = "reservation_list"
	ReservationSourceDetail ReservationSourceType = "reservation_detail"
	ReservationSourceDialog ReservationSourceType = "reservation_dialog"
	ReservationSourceDirect ReservationSourceType = "direct"
)

func (source ReservationSourceType) IsValid() bool {
	switch source {
	case ReservationSourceList, ReservationSourceDetail, ReservationSourceDialog, ReservationSourceDirect:
		return true
	default:
		return false
	}
}

// ReservationProduct is a non-transactional product concept used only for
// demand testing. LinkedProductID is optional and does not copy price, stock,
// SKU, cart, order, or payment state from the linked SPU.
type ReservationProduct struct {
	ID                 uint64                   `gorm:"primaryKey" json:"id"`
	LinkedProductID    *uint                    `gorm:"index" json:"linked_product_id"`
	LinkedProduct      *SPU                     `gorm:"foreignKey:LinkedProductID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL" json:"-"`
	Slug               string                   `gorm:"type:varchar(120);not null;uniqueIndex:uidx_reservation_products_slug" json:"slug"`
	TitleEN            string                   `gorm:"type:varchar(200);not null" json:"title_en"`
	TitleZH            *string                  `gorm:"type:varchar(200)" json:"title_zh"`
	SummaryEN          string                   `gorm:"type:varchar(1000);not null;default:''" json:"summary_en"`
	SummaryZH          *string                  `gorm:"type:varchar(1000)" json:"summary_zh"`
	DescriptionEN      string                   `gorm:"type:text;not null;default:''" json:"description_en"`
	DescriptionZH      *string                  `gorm:"type:text" json:"description_zh"`
	MainImageURL       string                   `gorm:"type:varchar(512);not null;default:''" json:"main_image_url"`
	GalleryJSON        datatypes.JSON           `gorm:"type:jsonb;not null;default:'[]'" json:"gallery_json"`
	Status             ReservationProductStatus `gorm:"type:varchar(16);not null;index;check:chk_reservation_products_status,status IN ('draft','scheduled','open','closed','archived')" json:"status"`
	ReservationStartAt *time.Time               `gorm:"index" json:"reservation_start_at"`
	ReservationEndAt   *time.Time               `gorm:"index;check:chk_reservation_products_window,reservation_end_at IS NULL OR reservation_start_at IS NULL OR reservation_end_at > reservation_start_at" json:"reservation_end_at"`
	DisplayOrder       int                      `gorm:"not null;default:0;index" json:"display_order"`
	IsFeatured         bool                     `gorm:"not null;default:false;index" json:"is_featured"`
	HeatLevelOverride  *ReservationHeatLevel    `gorm:"type:varchar(24);check:chk_reservation_products_heat_override,heat_level_override IS NULL OR heat_level_override IN ('new','popular','trending','highly_requested')" json:"heat_level_override"`
	CreatedBy          uint                     `gorm:"not null;index" json:"created_by"`
	CreatedAt          time.Time                `gorm:"not null" json:"created_at"`
	UpdatedAt          time.Time                `gorm:"not null" json:"updated_at"`
}

func (product *ReservationProduct) BeforeSave(_ *gorm.DB) error {
	product.Slug = strings.ToLower(strings.TrimSpace(product.Slug))
	if product.ReservationStartAt != nil {
		value := product.ReservationStartAt.UTC()
		product.ReservationStartAt = &value
	}
	if product.ReservationEndAt != nil {
		value := product.ReservationEndAt.UTC()
		product.ReservationEndAt = &value
	}
	return nil
}

func (product *ReservationProduct) IsOpenAt(now time.Time) bool {
	if product == nil || product.Status != ReservationProductStatusOpen {
		return false
	}
	now = now.UTC()
	if product.ReservationStartAt != nil && now.Before(product.ReservationStartAt.UTC()) {
		return false
	}
	if product.ReservationEndAt != nil && !now.Before(product.ReservationEndAt.UTC()) {
		return false
	}
	return true
}

// ReservationInterest is deliberately a weak anonymous signal. VisitorID is
// the existing gc_visitor_id and is never promoted to an authenticated user.
type ReservationInterest struct {
	ID                   uint64                `gorm:"primaryKey" json:"id"`
	ReservationProductID uint64                `gorm:"not null;uniqueIndex:uidx_reservation_interests_product_visitor,priority:1;index" json:"reservation_product_id"`
	ReservationProduct   ReservationProduct    `gorm:"foreignKey:ReservationProductID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-"`
	VisitorID            string                `gorm:"type:char(36);not null;uniqueIndex:uidx_reservation_interests_product_visitor,priority:2;index" json:"visitor_id"`
	SessionID            *string               `gorm:"type:char(36)" json:"session_id"`
	IdempotencyKey       string                `gorm:"type:varchar(64);not null;uniqueIndex:uidx_reservation_interests_idempotency" json:"idempotency_key"`
	SourceType           ReservationSourceType `gorm:"type:varchar(32);not null;check:chk_reservation_interests_source_type,source_type IN ('reservation_list','reservation_detail','reservation_dialog','direct')" json:"source_type"`
	SourceID             *string               `gorm:"type:varchar(128)" json:"source_id"`
	CreatedAt            time.Time             `gorm:"not null;index" json:"created_at"`
}

type ReservationContactType string

const (
	ReservationContactSMS      ReservationContactType = "sms"
	ReservationContactWhatsApp ReservationContactType = "whatsapp"
)

func (contactType ReservationContactType) IsValid() bool {
	return contactType == ReservationContactSMS || contactType == ReservationContactWhatsApp
}

type ReservationVerificationStatus string

const (
	ReservationVerificationPending   ReservationVerificationStatus = "pending"
	ReservationVerificationVerified  ReservationVerificationStatus = "verified"
	ReservationVerificationExpired   ReservationVerificationStatus = "expired"
	ReservationVerificationFailed    ReservationVerificationStatus = "failed"
	ReservationVerificationCancelled ReservationVerificationStatus = "cancelled"
)

func (status ReservationVerificationStatus) IsValid() bool {
	switch status {
	case ReservationVerificationPending, ReservationVerificationVerified, ReservationVerificationExpired,
		ReservationVerificationFailed, ReservationVerificationCancelled:
		return true
	default:
		return false
	}
}

type ProductReservationStatus string

const (
	ProductReservationActive    ProductReservationStatus = "active"
	ProductReservationPending   ProductReservationStatus = "pending"
	ProductReservationConfirmed ProductReservationStatus = "confirmed"
	ProductReservationCancelled ProductReservationStatus = "cancelled"
	ProductReservationRejected  ProductReservationStatus = "rejected"
	ProductReservationExpired   ProductReservationStatus = "expired"
)

type ReservationIdentityType string

const (
	ReservationIdentityAnonymous     ReservationIdentityType = "anonymous"
	ReservationIdentityPhoneVerified ReservationIdentityType = "phone_verified"
)

const ReservationVerificationNotRequired ReservationVerificationStatus = "not_required"

// ProductReservation stores both verified-phone and anonymous reservation
// identities. Contact fields remain populated only for OTP reservations.
// Anonymous records store stable HMAC hashes, never the browser cookie token or
// the original IP address.
type ProductReservation struct {
	ID                       uint64                        `gorm:"primaryKey" json:"id"`
	ReservationCode          string                        `gorm:"type:varchar(48);not null;uniqueIndex:uidx_product_reservations_code" json:"reservation_code"`
	IdentityType             ReservationIdentityType       `gorm:"type:varchar(20);not null;default:'phone_verified';index;check:chk_product_reservations_identity_type,identity_type IN ('anonymous','phone_verified')" json:"identity_type"`
	ReservationProductID     uint64                        `gorm:"not null;uniqueIndex:uidx_product_reservations_product_contact,priority:1;index" json:"reservation_product_id"`
	ReservationProduct       ReservationProduct            `gorm:"foreignKey:ReservationProductID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-"`
	VisitorID                string                        `gorm:"type:char(36);index" json:"-"`
	BrowserHash              *string                       `gorm:"type:char(64);index" json:"-"`
	IPHash                   *string                       `gorm:"type:char(64);index" json:"-"`
	ActiveIdentityKey        *string                       `gorm:"type:char(64);uniqueIndex:uidx_product_reservations_active_identity" json:"-"`
	ContactType              ReservationContactType        `gorm:"type:varchar(16);check:chk_product_reservations_contact_type,contact_type IS NULL OR contact_type IN ('sms','whatsapp')" json:"-"`
	ContactHash              string                        `gorm:"type:char(64);uniqueIndex:uidx_product_reservations_product_contact,priority:2;index" json:"-"`
	ContactEncrypted         []byte                        `gorm:"type:bytea" json:"-"`
	ContactEncryptionVersion int                           `gorm:"check:chk_product_reservations_encryption_version,contact_encryption_version IS NULL OR contact_encryption_version > 0" json:"-"`
	PhoneCountryCode         string                        `gorm:"type:varchar(8)" json:"-"`
	PhoneLast4               string                        `gorm:"type:char(4)" json:"-"`
	VerificationStatus       ReservationVerificationStatus `gorm:"type:varchar(16);not null;index;check:chk_product_reservations_verification_status,verification_status IN ('not_required','pending','verified','expired','failed','cancelled')" json:"-"`
	Status                   ProductReservationStatus      `gorm:"type:varchar(16);not null;index;check:chk_product_reservations_status,status IN ('active','pending','confirmed','cancelled','rejected','expired');check:chk_product_reservations_state,(identity_type = 'phone_verified' AND ((verification_status = 'verified' AND status = 'active' AND verified_at IS NOT NULL AND cancelled_at IS NULL AND manage_token_hash IS NOT NULL) OR (verification_status = 'cancelled' AND status = 'cancelled' AND verified_at IS NOT NULL AND cancelled_at IS NOT NULL AND manage_token_hash IS NOT NULL) OR (verification_status IN ('pending','expired','failed') AND status = 'active' AND verified_at IS NULL AND cancelled_at IS NULL AND manage_token_hash IS NULL))) OR (identity_type = 'anonymous' AND verification_status = 'not_required' AND browser_hash IS NOT NULL AND ip_hash IS NOT NULL AND expires_at IS NOT NULL AND ((status IN ('pending','confirmed') AND active_identity_key IS NOT NULL AND cancelled_at IS NULL) OR (status IN ('cancelled','rejected','expired') AND active_identity_key IS NULL)))" json:"status"`
	Provider                 string                        `gorm:"type:varchar(32)" json:"-"`
	ProviderReference        *string                       `gorm:"type:varchar(128)" json:"-"`
	VerificationChallengeID  string                        `gorm:"type:varchar(64);uniqueIndex:uidx_product_reservations_challenge" json:"-"`
	OTPRequestIdempotencyKey string                        `gorm:"type:char(36);uniqueIndex:uidx_product_reservations_otp_request_idempotency" json:"-"`
	OTPVerifyIdempotencyKey  *string                       `gorm:"type:char(36);uniqueIndex:uidx_product_reservations_otp_verify_idempotency" json:"-"`
	CancelIdempotencyKey     *string                       `gorm:"type:char(36)" json:"-"`
	VerificationRequestedAt  time.Time                     `gorm:"not null;index" json:"-"`
	VerifiedAt               *time.Time                    `gorm:"index" json:"-"`
	CancelledAt              *time.Time                    `json:"-"`
	TurnstileVerifiedAt      *time.Time                    `json:"-"`
	VerificationAttemptCount int                           `gorm:"not null;default:0;check:chk_product_reservations_attempt_count,verification_attempt_count >= 0" json:"-"`
	ManageTokenHash          *string                       `gorm:"type:char(64);index" json:"-"`
	Quantity                 int                           `gorm:"not null;default:1;check:chk_product_reservations_quantity,quantity BETWEEN 1 AND 99" json:"quantity"`
	Note                     *string                       `gorm:"type:varchar(500)" json:"note,omitempty"`
	ExpiresAt                *time.Time                    `gorm:"index" json:"expires_at,omitempty"`
	RiskFlagged              bool                          `gorm:"not null;default:false;index" json:"risk_flagged"`
	RiskFlaggedAt            *time.Time                    `json:"risk_flagged_at,omitempty"`
	SourceType               ReservationSourceType         `gorm:"type:varchar(32);not null;check:chk_product_reservations_source_type,source_type IN ('reservation_list','reservation_detail','reservation_dialog','direct')" json:"-"`
	SourceID                 *string                       `gorm:"type:varchar(128)" json:"-"`
	CreatedAt                time.Time                     `gorm:"not null" json:"-"`
	UpdatedAt                time.Time                     `gorm:"not null" json:"-"`
}

func (reservation *ProductReservation) BeforeCreate(_ *gorm.DB) error {
	if strings.TrimSpace(reservation.ReservationCode) == "" {
		code, err := NewReservationCode()
		if err != nil {
			return err
		}
		reservation.ReservationCode = code
	}
	if reservation.IdentityType == "" {
		reservation.IdentityType = ReservationIdentityPhoneVerified
	}
	if reservation.Quantity == 0 {
		reservation.Quantity = 1
	}
	return nil
}

func (reservation *ProductReservation) BeforeSave(_ *gorm.DB) error {
	reservation.ReservationCode = strings.TrimSpace(reservation.ReservationCode)
	reservation.VisitorID = strings.ToLower(strings.TrimSpace(reservation.VisitorID))
	reservation.VerificationChallengeID = strings.TrimSpace(reservation.VerificationChallengeID)
	reservation.OTPRequestIdempotencyKey = strings.ToLower(strings.TrimSpace(reservation.OTPRequestIdempotencyKey))
	if reservation.OTPVerifyIdempotencyKey != nil {
		value := strings.ToLower(strings.TrimSpace(*reservation.OTPVerifyIdempotencyKey))
		reservation.OTPVerifyIdempotencyKey = &value
	}
	if reservation.CancelIdempotencyKey != nil {
		value := strings.ToLower(strings.TrimSpace(*reservation.CancelIdempotencyKey))
		reservation.CancelIdempotencyKey = &value
	}
	return nil
}

func NewReservationCode() (string, error) {
	value := make([]byte, 18)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return "GC-" + base64.RawURLEncoding.EncodeToString(value), nil
}

type ReservationAttemptAction string

const (
	ReservationAttemptInterest   ReservationAttemptAction = "interest"
	ReservationAttemptOTPRequest ReservationAttemptAction = "otp_request"
	ReservationAttemptOTPVerify  ReservationAttemptAction = "otp_verify"
	ReservationAttemptCancel     ReservationAttemptAction = "cancel"
	ReservationAttemptCreate     ReservationAttemptAction = "create"
)

type ReservationAttemptResult string

const (
	ReservationAttemptAccepted  ReservationAttemptResult = "accepted"
	ReservationAttemptDuplicate ReservationAttemptResult = "duplicate"
	ReservationAttemptBlocked   ReservationAttemptResult = "blocked"
	ReservationAttemptFailed    ReservationAttemptResult = "failed"
)

type ReservationReasonCode string

const (
	ReservationReasonRateLimited        ReservationReasonCode = "rate_limited"
	ReservationReasonProductUnavailable ReservationReasonCode = "product_unavailable"
	ReservationReasonInvalidRequest     ReservationReasonCode = "invalid_request"
	ReservationReasonInvalidPhone       ReservationReasonCode = "invalid_phone"
	ReservationReasonChannelDisabled    ReservationReasonCode = "channel_disabled"
	ReservationReasonTurnstileInvalid   ReservationReasonCode = "turnstile_invalid"
	ReservationReasonTurnstileTimeout   ReservationReasonCode = "turnstile_timeout"
	ReservationReasonResendCooldown     ReservationReasonCode = "resend_cooldown"
	ReservationReasonProviderError      ReservationReasonCode = "provider_error"
	ReservationReasonInvalidCode        ReservationReasonCode = "invalid_code"
	ReservationReasonMaxAttempts        ReservationReasonCode = "max_attempts"
	ReservationReasonChallengeExpired   ReservationReasonCode = "challenge_expired"
	ReservationReasonChallengeNotFound  ReservationReasonCode = "challenge_not_found"
	ReservationReasonAlreadyExists      ReservationReasonCode = "already_exists"
	ReservationReasonAlreadyCancelled   ReservationReasonCode = "already_cancelled"
	ReservationReasonBrowserRateLimited ReservationReasonCode = "browser_rate_limited"
	ReservationReasonIPRateLimited      ReservationReasonCode = "ip_rate_limited"
)

func (reason ReservationReasonCode) IsValid() bool {
	switch reason {
	case ReservationReasonRateLimited, ReservationReasonProductUnavailable, ReservationReasonInvalidRequest,
		ReservationReasonInvalidPhone, ReservationReasonChannelDisabled, ReservationReasonTurnstileInvalid,
		ReservationReasonTurnstileTimeout, ReservationReasonResendCooldown, ReservationReasonProviderError,
		ReservationReasonInvalidCode, ReservationReasonMaxAttempts, ReservationReasonChallengeExpired,
		ReservationReasonChallengeNotFound, ReservationReasonAlreadyExists, ReservationReasonAlreadyCancelled,
		ReservationReasonBrowserRateLimited, ReservationReasonIPRateLimited:
		return true
	default:
		return false
	}
}

// ReservationAttemptEvent is deliberately minimal. It stores only stable
// pseudonymous risk keys and aggregateable outcomes, never raw IPs, full phone
// numbers, OTPs, Turnstile tokens, request bodies, or provider responses.
type ReservationAttemptEvent struct {
	ID                   uint64                   `gorm:"primaryKey" json:"id"`
	ReservationProductID *uint64                  `gorm:"index" json:"reservation_product_id"`
	ReservationProduct   *ReservationProduct      `gorm:"foreignKey:ReservationProductID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL" json:"-"`
	VisitorID            *string                  `gorm:"type:char(36);index" json:"-"`
	ContactHash          *string                  `gorm:"type:char(64);index" json:"-"`
	IPRiskKey            *string                  `gorm:"type:char(64);index" json:"-"`
	BrowserHash          *string                  `gorm:"type:char(64);index" json:"-"`
	IPHash               *string                  `gorm:"type:char(64);index" json:"-"`
	Action               ReservationAttemptAction `gorm:"type:varchar(16);not null;index;check:chk_reservation_attempt_events_action,action IN ('interest','otp_request','otp_verify','cancel','create')" json:"action"`
	Result               ReservationAttemptResult `gorm:"type:varchar(16);not null;index;check:chk_reservation_attempt_events_result,result IN ('accepted','duplicate','blocked','failed')" json:"result"`
	ReasonCode           *ReservationReasonCode   `gorm:"type:varchar(32);check:chk_reservation_attempt_events_reason,reason_code IS NULL OR reason_code IN ('rate_limited','product_unavailable','invalid_request','invalid_phone','channel_disabled','turnstile_invalid','turnstile_timeout','resend_cooldown','provider_error','invalid_code','max_attempts','challenge_expired','challenge_not_found','already_exists','already_cancelled','browser_rate_limited','ip_rate_limited')" json:"reason_code"`
	CreatedAt            time.Time                `gorm:"not null;index" json:"created_at"`
}
