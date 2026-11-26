package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"saas/internal/models"
	"saas/internal/pkg/locale"
	"saas/internal/repository"

	"gorm.io/gorm"
)

var (
	ErrReservationChannelDisabled  = &contextualError{message: "verification channel is unavailable", target: ErrInvalidInput}
	ErrReservationInvalidCode      = &contextualError{message: "invalid verification code", target: ErrInvalidInput}
	ErrReservationChallengeExpired = &contextualError{message: "verification challenge expired", target: ErrConflict}
	ErrReservationMaxAttempts      = &contextualError{message: "verification attempt limit reached", target: ErrRateLimited}
)

type reservationWorkflowRepository interface {
	FindPublicProductByID(ctx context.Context, id uint64, now time.Time) (*models.ReservationProduct, error)
	FindByRequestIdempotency(ctx context.Context, key string) (*models.ProductReservation, error)
	FindByProductContact(ctx context.Context, productID uint64, contactHash string) (*models.ProductReservation, error)
	FindByChallenge(ctx context.Context, productID uint64, challengeID string) (*models.ProductReservation, error)
	UpsertPending(ctx context.Context, reservation *models.ProductReservation, attempt *models.ReservationAttemptEvent) (*models.ProductReservation, error)
	RecordVerificationFailure(ctx context.Context, productID uint64, challengeID string, maxAttempts int, attempt *models.ReservationAttemptEvent) (*models.ProductReservation, error)
	MarkExpired(ctx context.Context, productID uint64, challengeID string, attempt *models.ReservationAttemptEvent) error
	FinalizeVerification(ctx context.Context, productID uint64, challengeID, verifyIdempotencyKey, manageTokenHash string, verifiedAt time.Time, attempt *models.ReservationAttemptEvent) (repository.ReservationFinalizeResult, error)
	Cancel(ctx context.Context, productID uint64, manageTokenHash, idempotencyKey string, cancelledAt time.Time, attempt *models.ReservationAttemptEvent) (bool, error)
	CreateAttemptEvent(ctx context.Context, event *models.ReservationAttemptEvent) error
}

type ReservationWorkflowOptions struct {
	Now                 func() time.Time
	Protector           *ReservationContactProtector
	Turnstile           TurnstileVerifier
	OTPProvider         OTPProvider
	Limiter             *ReservationMemoryLimiter
	RateLimits          ReservationRateLimits
	EnabledChannels     map[string]bool
	ChallengeTTL        time.Duration
	ResendCooldown      time.Duration
	MaxVerifyAttempts   int
	MinimumResponseTime time.Duration
}

type ReservationWorkflowService struct {
	repo                reservationWorkflowRepository
	now                 func() time.Time
	protector           *ReservationContactProtector
	turnstile           TurnstileVerifier
	otpProvider         OTPProvider
	limiter             *ReservationMemoryLimiter
	rateLimits          ReservationRateLimits
	enabledChannels     map[string]bool
	challengeTTL        time.Duration
	resendCooldown      time.Duration
	maxVerifyAttempts   int
	minimumResponseTime time.Duration
	requestLocks        *reservationKeyLocker
	verifyLocks         *reservationKeyLocker
}

type ReservationOTPRequestInput struct {
	VisitorID      string
	IdempotencyKey string
	ContactType    models.ReservationContactType
	PhoneCountry   string
	PhoneNumber    string
	TurnstileToken string
	SourceType     models.ReservationSourceType
	SourceID       string
	Locale         string
	IPRiskKey      string
	RemoteIP       string
}

type ReservationOTPRequestResult struct {
	ChallengeID        string `json:"challenge_id"`
	Status             string `json:"status"`
	ResendAfterSeconds int    `json:"resend_after_seconds"`
}

type ReservationOTPVerifyInput struct {
	ChallengeID    string
	Code           string
	IdempotencyKey string
	IPRiskKey      string
}

type ReservationOTPVerifyResult struct {
	Reserved    bool   `json:"reserved"`
	Status      string `json:"status"`
	ManageToken string `json:"manage_token,omitempty"`
}

type ReservationCancelInput struct {
	ManageToken    string
	IdempotencyKey string
	IPRiskKey      string
}

type ReservationCancelResult struct {
	Cancelled bool   `json:"cancelled"`
	Status    string `json:"status"`
}

type ReservationCapabilities struct {
	ReservationEnabled bool                            `json:"reservation_enabled"`
	ReservationMode    string                          `json:"reservation_mode"`
	OTPChannels        []models.ReservationContactType `json:"otp_channels"`
	TurnstileEnabled   bool                            `json:"turnstile_enabled"`
}

type reservationLimitRule struct {
	scope  string
	key    string
	limit  int
	window time.Duration
}

func NewReservationWorkflowService(repo reservationWorkflowRepository, options ReservationWorkflowOptions) (*ReservationWorkflowService, error) {
	if options.Protector == nil || options.Turnstile == nil || options.OTPProvider == nil {
		return nil, errors.New("reservation workflow security and providers are required")
	}
	if options.ChallengeTTL <= 0 || options.ResendCooldown <= 0 || options.MaxVerifyAttempts <= 0 {
		return nil, errors.New("reservation workflow TTL, cooldown, and attempts must be positive")
	}
	if options.MinimumResponseTime < 0 || options.MinimumResponseTime > 5*time.Second {
		return nil, errors.New("reservation workflow response floor must be between zero and five seconds")
	}
	clock := options.Now
	if clock == nil {
		clock = time.Now
	}
	channels := make(map[string]bool)
	for name, enabled := range options.EnabledChannels {
		if enabled {
			channels[strings.ToLower(strings.TrimSpace(name))] = true
		}
	}
	if len(channels) == 0 {
		return nil, errors.New("at least one reservation OTP channel is required")
	}
	return &ReservationWorkflowService{
		repo: repo, now: clock, protector: options.Protector, turnstile: options.Turnstile,
		otpProvider: options.OTPProvider, limiter: options.Limiter, rateLimits: options.RateLimits,
		enabledChannels: channels, challengeTTL: options.ChallengeTTL,
		resendCooldown: options.ResendCooldown, maxVerifyAttempts: options.MaxVerifyAttempts,
		minimumResponseTime: options.MinimumResponseTime,
		requestLocks:        newReservationKeyLocker(), verifyLocks: newReservationKeyLocker(),
	}, nil
}

func (s *ReservationWorkflowService) Capabilities() ReservationCapabilities {
	channels := make([]models.ReservationContactType, 0, 2)
	for _, channel := range []models.ReservationContactType{models.ReservationContactSMS, models.ReservationContactWhatsApp} {
		if s.enabledChannels[string(channel)] {
			channels = append(channels, channel)
		}
	}
	return ReservationCapabilities{
		ReservationEnabled: true,
		ReservationMode:    "otp",
		OTPChannels:        channels,
		TurnstileEnabled:   true,
	}
}

func (s *ReservationWorkflowService) RequestOTP(
	ctx context.Context,
	productID uint64,
	input ReservationOTPRequestInput,
) (ReservationOTPRequestResult, error) {
	responseStartedAt := time.Now()
	now := s.now().UTC()
	if productID == 0 {
		return ReservationOTPRequestResult{}, ErrInvalidInput
	}
	if err := s.checkLimits(
		reservationLimitRule{"otp_request_ingress_global", "global", s.rateLimits.OTPRequestGlobalPerMinute * 5, time.Minute},
		reservationLimitRule{"otp_request_ingress_ip", input.IPRiskKey, s.rateLimits.OTPRequestIPPerHour * 5, time.Hour},
	); err != nil {
		s.recordAttempt(ctx, reservationAttempt(productID, "", "", input.IPRiskKey, models.ReservationAttemptOTPRequest, models.ReservationAttemptBlocked, models.ReservationReasonRateLimited, now))
		return ReservationOTPRequestResult{}, err
	}

	visitorID, err := normalizeRequiredReservationUUID(input.VisitorID)
	if err != nil {
		s.recordAttempt(ctx, reservationAttempt(productID, "", "", input.IPRiskKey, models.ReservationAttemptOTPRequest, models.ReservationAttemptFailed, models.ReservationReasonInvalidRequest, now))
		return ReservationOTPRequestResult{}, err
	}
	idempotencyKey, err := normalizeRequiredReservationUUID(input.IdempotencyKey)
	if err != nil || len(strings.TrimSpace(input.TurnstileToken)) > maxTurnstileTokenLength {
		s.recordAttempt(ctx, reservationAttempt(productID, visitorID, "", input.IPRiskKey, models.ReservationAttemptOTPRequest, models.ReservationAttemptFailed, models.ReservationReasonInvalidRequest, now))
		return ReservationOTPRequestResult{}, ErrInvalidInput
	}
	if !input.ContactType.IsValid() || !s.enabledChannels[string(input.ContactType)] {
		s.recordAttempt(ctx, reservationAttempt(productID, visitorID, "", input.IPRiskKey, models.ReservationAttemptOTPRequest, models.ReservationAttemptBlocked, models.ReservationReasonChannelDisabled, now))
		return ReservationOTPRequestResult{}, ErrReservationChannelDisabled
	}
	sourceType := input.SourceType
	if sourceType == "" {
		sourceType = models.ReservationSourceDirect
	}
	if !sourceType.IsValid() || !validOptionalStorefrontBusinessID(strings.TrimSpace(input.SourceID), maxStorefrontSourceIDLength) {
		return ReservationOTPRequestResult{}, ErrInvalidInput
	}
	if _, err := s.repo.FindPublicProductByID(ctx, productID, now); errors.Is(err, gorm.ErrRecordNotFound) {
		s.recordAttempt(ctx, reservationAttempt(productID, visitorID, "", input.IPRiskKey, models.ReservationAttemptOTPRequest, models.ReservationAttemptBlocked, models.ReservationReasonProductUnavailable, now))
		return ReservationOTPRequestResult{}, ErrReservationProductUnavailable
	} else if err != nil {
		return ReservationOTPRequestResult{}, err
	}

	phone, err := NormalizeReservationPhone(input.PhoneNumber, input.PhoneCountry)
	if err != nil {
		s.recordAttempt(ctx, reservationAttempt(productID, visitorID, "", input.IPRiskKey, models.ReservationAttemptOTPRequest, models.ReservationAttemptFailed, models.ReservationReasonInvalidPhone, now))
		return ReservationOTPRequestResult{}, err
	}
	contactHash := s.protector.Hash(phone.E164)
	turnstileRequestID := ReservationTurnstileRequestID(productID, contactHash, idempotencyKey)
	if turnstileRequestID == "" {
		return ReservationOTPRequestResult{}, ErrInvalidInput
	}
	verification, err := s.turnstile.Verify(ctx, input.TurnstileToken, input.RemoteIP, turnstileRequestID)
	if err != nil {
		reason := models.ReservationReasonTurnstileInvalid
		result := models.ReservationAttemptBlocked
		if errors.Is(err, ErrServiceUnavailable) {
			reason = models.ReservationReasonTurnstileTimeout
			result = models.ReservationAttemptFailed
		}
		s.recordAttempt(ctx, reservationAttempt(productID, visitorID, contactHash, input.IPRiskKey, models.ReservationAttemptOTPRequest, result, reason, now))
		return ReservationOTPRequestResult{}, err
	}
	defer waitForReservationResponseFloor(ctx, responseStartedAt, s.minimumResponseTime)
	unlock := s.requestLocks.acquire(strconv.FormatUint(productID, 10) + "\x00" + contactHash)
	defer unlock()

	if existing, findErr := s.repo.FindByRequestIdempotency(ctx, idempotencyKey); findErr == nil {
		if existing.ReservationProductID != productID || existing.ContactHash != contactHash {
			return ReservationOTPRequestResult{}, ErrConflict
		}
		s.recordAttempt(ctx, reservationAttempt(productID, visitorID, contactHash, input.IPRiskKey, models.ReservationAttemptOTPRequest, models.ReservationAttemptDuplicate, models.ReservationReasonAlreadyExists, now))
		return ReservationOTPRequestResult{
			ChallengeID: existing.VerificationChallengeID, Status: "pending",
			ResendAfterSeconds: s.resendSeconds(existing.VerificationRequestedAt, now),
		}, nil
	} else if !errors.Is(findErr, gorm.ErrRecordNotFound) {
		return ReservationOTPRequestResult{}, findErr
	}

	var existingContact *models.ProductReservation
	if existing, findErr := s.repo.FindByProductContact(ctx, productID, contactHash); findErr == nil {
		existingContact = existing
		if elapsed := now.Sub(existing.VerificationRequestedAt); elapsed >= 0 && elapsed < s.resendCooldown {
			s.recordAttempt(ctx, reservationAttempt(productID, visitorID, contactHash, input.IPRiskKey, models.ReservationAttemptOTPRequest, models.ReservationAttemptBlocked, models.ReservationReasonResendCooldown, now))
			return s.opaquePendingResult(existing.VerificationChallengeID)
		}
	} else if !errors.Is(findErr, gorm.ErrRecordNotFound) {
		return ReservationOTPRequestResult{}, findErr
	}

	if err := s.checkLimits(
		reservationLimitRule{"otp_request_contact_30m", contactHash, s.rateLimits.OTPRequestContactPer30Min, 30 * time.Minute},
		reservationLimitRule{"otp_request_contact_day", contactHash, s.rateLimits.OTPRequestContactPerDay, 24 * time.Hour},
	); err != nil {
		s.recordAttempt(ctx, reservationAttempt(productID, visitorID, contactHash, input.IPRiskKey, models.ReservationAttemptOTPRequest, models.ReservationAttemptBlocked, models.ReservationReasonRateLimited, now))
		if existingContact != nil {
			return s.opaquePendingResult(existingContact.VerificationChallengeID)
		}
		return s.opaquePendingResult("")
	}
	if err := s.checkLimits(
		reservationLimitRule{"otp_request_visitor_hour", visitorID, s.rateLimits.OTPRequestVisitorPerHour, time.Hour},
		reservationLimitRule{"otp_request_ip_hour", input.IPRiskKey, s.rateLimits.OTPRequestIPPerHour, time.Hour},
		reservationLimitRule{"otp_request_global_minute", "global", s.rateLimits.OTPRequestGlobalPerMinute, time.Minute},
	); err != nil {
		s.recordAttempt(ctx, reservationAttempt(productID, visitorID, contactHash, input.IPRiskKey, models.ReservationAttemptOTPRequest, models.ReservationAttemptBlocked, models.ReservationReasonRateLimited, now))
		return ReservationOTPRequestResult{}, err
	}

	providerReference, err := s.otpProvider.StartVerification(ctx, phone.E164, input.ContactType, locale.NormalizeLocale(input.Locale))
	if err != nil {
		s.recordAttempt(ctx, reservationAttempt(productID, visitorID, contactHash, input.IPRiskKey, models.ReservationAttemptOTPRequest, models.ReservationAttemptFailed, models.ReservationReasonProviderError, now))
		return ReservationOTPRequestResult{}, ErrOTPProviderUnavailable
	}
	encrypted, encryptionVersion, err := s.protector.Encrypt(phone.E164)
	if err != nil {
		return ReservationOTPRequestResult{}, err
	}
	challengeID, err := NewReservationChallengeID()
	if err != nil {
		return ReservationOTPRequestResult{}, ErrServiceUnavailable
	}
	providerReference = strings.TrimSpace(providerReference)
	stored, err := s.repo.UpsertPending(ctx, &models.ProductReservation{
		ReservationProductID: productID, VisitorID: visitorID, ContactType: input.ContactType,
		ContactHash: contactHash, ContactEncrypted: encrypted, ContactEncryptionVersion: encryptionVersion,
		PhoneCountryCode: phone.CountryCode, PhoneLast4: phone.Last4,
		VerificationStatus: models.ReservationVerificationPending, Status: models.ProductReservationActive,
		Provider: s.otpProvider.Name(), ProviderReference: optionalReservationString(providerReference),
		VerificationChallengeID: challengeID, OTPRequestIdempotencyKey: idempotencyKey,
		VerificationRequestedAt: now, TurnstileVerifiedAt: &verification.VerifiedAt,
		SourceType: sourceType, SourceID: optionalReservationString(strings.TrimSpace(input.SourceID)),
		CreatedAt: now, UpdatedAt: now,
	}, reservationAttempt(productID, visitorID, contactHash, input.IPRiskKey, models.ReservationAttemptOTPRequest, models.ReservationAttemptAccepted, "", now))
	if err != nil {
		if errors.Is(err, repository.ErrReservationProductNotOpen) {
			s.recordAttempt(ctx, reservationAttempt(productID, visitorID, contactHash, input.IPRiskKey, models.ReservationAttemptOTPRequest, models.ReservationAttemptBlocked, models.ReservationReasonProductUnavailable, now))
		}
		return ReservationOTPRequestResult{}, mapReservationPersistenceError(err)
	}
	return ReservationOTPRequestResult{
		ChallengeID: stored.VerificationChallengeID, Status: "pending",
		ResendAfterSeconds: int(s.resendCooldown.Seconds()),
	}, nil
}

func waitForReservationResponseFloor(ctx context.Context, startedAt time.Time, minimum time.Duration) {
	remaining := minimum - time.Since(startedAt)
	if remaining <= 0 {
		return
	}
	timer := time.NewTimer(remaining)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}

func (s *ReservationWorkflowService) VerifyOTP(
	ctx context.Context,
	productID uint64,
	input ReservationOTPVerifyInput,
) (ReservationOTPVerifyResult, error) {
	now := s.now().UTC()
	challengeID, err := normalizeReservationChallengeID(input.ChallengeID)
	if productID == 0 || err != nil {
		return ReservationOTPVerifyResult{}, ErrInvalidInput
	}
	idempotencyKey, err := normalizeRequiredReservationUUID(input.IdempotencyKey)
	code := strings.TrimSpace(input.Code)
	if err != nil || len(code) < 4 || len(code) > 10 || !reservationDigitsOnly(code) {
		return ReservationOTPVerifyResult{}, ErrInvalidInput
	}
	if err := s.checkLimits(
		reservationLimitRule{"otp_verify_ingress_global", "global", s.rateLimits.OTPVerifyGlobalPerMinute * 5, time.Minute},
		reservationLimitRule{"otp_verify_ingress_ip", input.IPRiskKey, s.rateLimits.OTPVerifyIPPer10Min * 5, 10 * time.Minute},
	); err != nil {
		s.recordAttempt(ctx, reservationAttempt(productID, "", "", input.IPRiskKey, models.ReservationAttemptOTPVerify, models.ReservationAttemptBlocked, models.ReservationReasonRateLimited, now))
		return ReservationOTPVerifyResult{}, err
	}
	if _, err := s.repo.FindPublicProductByID(ctx, productID, now); errors.Is(err, gorm.ErrRecordNotFound) {
		return ReservationOTPVerifyResult{}, ErrReservationProductUnavailable
	} else if err != nil {
		return ReservationOTPVerifyResult{}, err
	}
	unlock := s.verifyLocks.acquire(strconv.FormatUint(productID, 10) + "\x00" + challengeID)
	defer unlock()
	reservation, err := s.repo.FindByChallenge(ctx, productID, challengeID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		s.recordAttempt(ctx, reservationAttempt(productID, "", "", input.IPRiskKey, models.ReservationAttemptOTPVerify, models.ReservationAttemptFailed, models.ReservationReasonChallengeNotFound, now))
		return ReservationOTPVerifyResult{}, ErrReservationInvalidCode
	}
	if err != nil {
		return ReservationOTPVerifyResult{}, err
	}
	if reservation.VerificationStatus == models.ReservationVerificationVerified && reservation.Status == models.ProductReservationActive {
		s.recordAttempt(ctx, reservationAttempt(productID, reservation.VisitorID, reservation.ContactHash, input.IPRiskKey, models.ReservationAttemptOTPVerify, models.ReservationAttemptDuplicate, models.ReservationReasonAlreadyExists, now))
		if reservation.OTPVerifyIdempotencyKey != nil && *reservation.OTPVerifyIdempotencyKey == idempotencyKey {
			manageToken, _, tokenErr := s.protector.IdempotentManageToken(challengeID, idempotencyKey)
			if tokenErr != nil {
				return ReservationOTPVerifyResult{}, ErrServiceUnavailable
			}
			return ReservationOTPVerifyResult{Reserved: true, Status: "verified", ManageToken: manageToken}, nil
		}
		return ReservationOTPVerifyResult{}, ErrConflict
	}
	if reservation.VerificationStatus != models.ReservationVerificationPending {
		return ReservationOTPVerifyResult{}, ErrConflict
	}
	if !now.Before(reservation.VerificationRequestedAt.Add(s.challengeTTL)) {
		attempt := reservationAttempt(productID, reservation.VisitorID, reservation.ContactHash, input.IPRiskKey, models.ReservationAttemptOTPVerify, models.ReservationAttemptBlocked, models.ReservationReasonChallengeExpired, now)
		if err := s.repo.MarkExpired(ctx, productID, challengeID, attempt); err != nil {
			return ReservationOTPVerifyResult{}, err
		}
		return ReservationOTPVerifyResult{}, ErrReservationChallengeExpired
	}
	if reservation.VerificationAttemptCount >= s.maxVerifyAttempts {
		s.recordAttempt(ctx, reservationAttempt(productID, reservation.VisitorID, reservation.ContactHash, input.IPRiskKey, models.ReservationAttemptOTPVerify, models.ReservationAttemptBlocked, models.ReservationReasonMaxAttempts, now))
		return ReservationOTPVerifyResult{}, ErrReservationMaxAttempts
	}
	if err := s.checkLimits(
		reservationLimitRule{"otp_verify_visitor_10m", reservation.VisitorID, s.rateLimits.OTPVerifyVisitorPer10Min, 10 * time.Minute},
		reservationLimitRule{"otp_verify_ip_10m", input.IPRiskKey, s.rateLimits.OTPVerifyIPPer10Min, 10 * time.Minute},
		reservationLimitRule{"otp_verify_global_minute", "global", s.rateLimits.OTPVerifyGlobalPerMinute, time.Minute},
	); err != nil {
		s.recordAttempt(ctx, reservationAttempt(productID, reservation.VisitorID, reservation.ContactHash, input.IPRiskKey, models.ReservationAttemptOTPVerify, models.ReservationAttemptBlocked, models.ReservationReasonRateLimited, now))
		return ReservationOTPVerifyResult{}, err
	}
	e164, err := s.protector.Decrypt(reservation.ContactEncrypted, reservation.ContactEncryptionVersion)
	if err != nil {
		return ReservationOTPVerifyResult{}, ErrServiceUnavailable
	}
	providerResult, err := s.otpProvider.CheckVerification(ctx, e164, code)
	if err != nil {
		s.recordAttempt(ctx, reservationAttempt(productID, reservation.VisitorID, reservation.ContactHash, input.IPRiskKey, models.ReservationAttemptOTPVerify, models.ReservationAttemptFailed, models.ReservationReasonProviderError, now))
		return ReservationOTPVerifyResult{}, ErrOTPProviderUnavailable
	}
	if !providerResult.Approved {
		result := models.ReservationAttemptFailed
		reason := models.ReservationReasonInvalidCode
		if reservation.VerificationAttemptCount+1 >= s.maxVerifyAttempts {
			result = models.ReservationAttemptBlocked
			reason = models.ReservationReasonMaxAttempts
		}
		updated, recordErr := s.repo.RecordVerificationFailure(
			ctx, productID, challengeID, s.maxVerifyAttempts,
			reservationAttempt(productID, reservation.VisitorID, reservation.ContactHash, input.IPRiskKey, models.ReservationAttemptOTPVerify, result, reason, now),
		)
		if recordErr != nil {
			return ReservationOTPVerifyResult{}, recordErr
		}
		if updated.VerificationAttemptCount >= s.maxVerifyAttempts {
			return ReservationOTPVerifyResult{}, ErrReservationMaxAttempts
		}
		return ReservationOTPVerifyResult{}, ErrReservationInvalidCode
	}
	manageToken, manageTokenHash, err := s.protector.IdempotentManageToken(challengeID, idempotencyKey)
	if err != nil {
		return ReservationOTPVerifyResult{}, err
	}
	finalized, err := s.repo.FinalizeVerification(
		ctx, productID, challengeID, idempotencyKey, manageTokenHash, now,
		reservationAttempt(productID, reservation.VisitorID, reservation.ContactHash, input.IPRiskKey, models.ReservationAttemptOTPVerify, models.ReservationAttemptAccepted, "", now),
	)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		current, findErr := s.repo.FindByChallenge(ctx, productID, challengeID)
		if findErr == nil && current.VerificationStatus == models.ReservationVerificationVerified && current.Status == models.ProductReservationActive {
			if current.OTPVerifyIdempotencyKey != nil && *current.OTPVerifyIdempotencyKey == idempotencyKey {
				return ReservationOTPVerifyResult{Reserved: true, Status: "verified", ManageToken: manageToken}, nil
			}
			return ReservationOTPVerifyResult{Reserved: true, Status: "verified"}, nil
		}
	}
	if errors.Is(err, repository.ErrReservationProductNotOpen) {
		s.recordAttempt(ctx, reservationAttempt(productID, reservation.VisitorID, reservation.ContactHash, input.IPRiskKey, models.ReservationAttemptOTPVerify, models.ReservationAttemptBlocked, models.ReservationReasonProductUnavailable, now))
		return ReservationOTPVerifyResult{}, ErrReservationProductUnavailable
	}
	if err != nil {
		return ReservationOTPVerifyResult{}, mapReservationPersistenceError(err)
	}
	if finalized.Won {
		return ReservationOTPVerifyResult{Reserved: true, Status: "verified", ManageToken: manageToken}, nil
	}
	return ReservationOTPVerifyResult{Reserved: true, Status: "verified"}, nil
}

func (s *ReservationWorkflowService) Cancel(
	ctx context.Context,
	productID uint64,
	input ReservationCancelInput,
) (ReservationCancelResult, error) {
	now := s.now().UTC()
	if err := s.checkLimits(
		reservationLimitRule{"reservation_cancel_ip_minute", input.IPRiskKey, s.rateLimits.CancelIPPerMinute, time.Minute},
		reservationLimitRule{"reservation_cancel_global_minute", "global", s.rateLimits.CancelGlobalPerMinute, time.Minute},
	); err != nil {
		s.recordAttempt(ctx, reservationAttempt(productID, "", "", input.IPRiskKey, models.ReservationAttemptCancel, models.ReservationAttemptBlocked, models.ReservationReasonRateLimited, now))
		return ReservationCancelResult{}, err
	}
	idempotencyKey, err := normalizeRequiredReservationUUID(input.IdempotencyKey)
	manageToken := strings.TrimSpace(input.ManageToken)
	if productID == 0 || err != nil || len(manageToken) < 32 || len(manageToken) > 128 {
		return ReservationCancelResult{}, ErrInvalidInput
	}
	cancelled, err := s.repo.Cancel(
		ctx, productID, HashReservationManageToken(manageToken), idempotencyKey, now,
		reservationAttempt(productID, "", "", input.IPRiskKey, models.ReservationAttemptCancel, models.ReservationAttemptAccepted, "", now),
	)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ReservationCancelResult{}, ErrNotFound
	}
	if err != nil {
		return ReservationCancelResult{}, err
	}
	_ = cancelled
	return ReservationCancelResult{Cancelled: true, Status: "cancelled"}, nil
}

func (s *ReservationWorkflowService) opaquePendingResult(challengeID string) (ReservationOTPRequestResult, error) {
	if challengeID == "" {
		var err error
		challengeID, err = NewReservationChallengeID()
		if err != nil {
			return ReservationOTPRequestResult{}, ErrServiceUnavailable
		}
	}
	return ReservationOTPRequestResult{
		ChallengeID:        challengeID,
		Status:             "pending",
		ResendAfterSeconds: int(s.resendCooldown.Seconds()),
	}, nil
}

func normalizeReservationChallengeID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) < 32 || len(value) > 64 {
		return "", ErrInvalidInput
	}
	for _, character := range value {
		if !((character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '-' || character == '_') {
			return "", ErrInvalidInput
		}
	}
	return value, nil
}

func (s *ReservationWorkflowService) checkLimits(rules ...reservationLimitRule) error {
	if s.limiter == nil {
		return nil
	}
	for _, rule := range rules {
		if rule.key == "" {
			continue
		}
		if allowed, retry := s.limiter.Allow(rule.scope, rule.key, rule.limit, rule.window); !allowed {
			return reservationRetryAfterError(retry)
		}
	}
	return nil
}

func (s *ReservationWorkflowService) resendSeconds(requestedAt, now time.Time) int {
	remaining := s.resendCooldown - now.Sub(requestedAt)
	if remaining <= 0 {
		return 0
	}
	seconds := int(remaining.Seconds())
	if seconds < 1 {
		return 1
	}
	return seconds
}

func (s *ReservationWorkflowService) recordAttempt(ctx context.Context, event *models.ReservationAttemptEvent) {
	if event != nil {
		_ = s.repo.CreateAttemptEvent(ctx, event)
	}
}

func reservationAttempt(
	productID uint64,
	visitorID, contactHash, ipRiskKey string,
	action models.ReservationAttemptAction,
	result models.ReservationAttemptResult,
	reason models.ReservationReasonCode,
	createdAt time.Time,
) *models.ReservationAttemptEvent {
	var productIDPointer *uint64
	if productID != 0 {
		value := productID
		productIDPointer = &value
	}
	event := &models.ReservationAttemptEvent{
		ReservationProductID: productIDPointer, VisitorID: optionalReservationString(visitorID),
		ContactHash: optionalReservationString(contactHash), IPRiskKey: optionalReservationString(ipRiskKey),
		Action: action, Result: result, CreatedAt: createdAt.UTC(),
	}
	if reason != "" {
		event.ReasonCode = &reason
	}
	return event
}

func reservationDigitsOnly(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}
