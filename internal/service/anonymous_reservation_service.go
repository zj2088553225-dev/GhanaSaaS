package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"saas/internal/models"
	"saas/internal/pkg/locale"
	"saas/internal/repository"

	"gorm.io/gorm"
)

const (
	maxAnonymousReservationNoteRunes = 500
	anonymousActiveIdentityDomain    = "reservation-active-identity:v1:"
)

var (
	ErrAnonymousReservationAlreadyExists = &contextualError{message: "reservation already exists", target: ErrConflict}
	ErrAnonymousReservationNotPending    = &contextualError{message: "only pending reservations can be cancelled", target: ErrConflict}
	ErrAnonymousProductNotReservable     = &contextualError{message: "product is not reservable", target: ErrConflict}
)

type AnonymousReservationRateLimitError struct {
	Scope             string
	RetryAfterSeconds int
}

func (e *AnonymousReservationRateLimitError) Error() string { return "too many reservation requests" }
func (e *AnonymousReservationRateLimitError) Unwrap() error { return ErrRateLimited }

type AnonymousReservationDuplicateError struct {
	Existing AnonymousReservationDTO
}

func (e *AnonymousReservationDuplicateError) Error() string {
	return ErrAnonymousReservationAlreadyExists.Error()
}
func (e *AnonymousReservationDuplicateError) Unwrap() error {
	return ErrAnonymousReservationAlreadyExists
}

type anonymousReservationRepository interface {
	Create(
		ctx context.Context,
		reservation *models.ProductReservation,
		attempt *models.ReservationAttemptEvent,
		limits repository.AnonymousReservationLimits,
		now time.Time,
	) (*models.ProductReservation, bool, error)
	CreateAttemptEvent(ctx context.Context, event *models.ReservationAttemptEvent) error
	ListByBrowser(ctx context.Context, browserHash string, now time.Time) ([]models.ProductReservation, error)
	CancelByBrowser(ctx context.Context, reservationCode, browserHash string, now time.Time) (*models.ProductReservation, error)
	ExpireDue(ctx context.Context, now time.Time, batchSize int) (int64, error)
	AdminList(ctx context.Context, query repository.AnonymousReservationAdminQuery) ([]models.ProductReservation, int64, error)
	AdminSetStatus(ctx context.Context, reservationCode string, status models.ProductReservationStatus, now time.Time) (*models.ProductReservation, error)
	AdminSetRisk(ctx context.Context, reservationCode string, flagged bool, now time.Time) (*models.ProductReservation, error)
	IdentityCounts(ctx context.Context, browserHash, ipHash string, since time.Time) (int64, int64, error)
	WasRateLimited(ctx context.Context, browserHash, ipHash string, since time.Time) (bool, error)
}

type AnonymousReservationOptions struct {
	Now        func() time.Time
	Turnstile  TurnstileVerifier
	TTL        time.Duration
	RateLimits repository.AnonymousReservationLimits
}

type AnonymousReservationService struct {
	repo       anonymousReservationRepository
	now        func() time.Time
	turnstile  TurnstileVerifier
	ttl        time.Duration
	rateLimits repository.AnonymousReservationLimits
	locks      *reservationKeyLocker
}

type AnonymousReservationCreateInput struct {
	ProductID      uint64
	Quantity       int
	Note           string
	TurnstileToken string
	BrowserHash    string
	IPHash         string
	RemoteIP       string
	Locale         string
}

type AnonymousReservationProductDTO struct {
	ID           uint64 `json:"id"`
	Slug         string `json:"slug"`
	Title        string `json:"title"`
	MainImageURL string `json:"main_image_url"`
}

type AnonymousReservationDTO struct {
	ReservationCode string                          `json:"reservation_code"`
	IdentityType    models.ReservationIdentityType  `json:"identity_type"`
	Status          models.ProductReservationStatus `json:"status"`
	Quantity        int                             `json:"quantity"`
	Note            *string                         `json:"note,omitempty"`
	CreatedAt       time.Time                       `json:"created_at"`
	ExpiresAt       *time.Time                      `json:"expires_at"`
	Product         AnonymousReservationProductDTO  `json:"product"`
}

type AnonymousReservationAdminDTO struct {
	ReservationCode          string                          `json:"reservation_code"`
	IdentityType             models.ReservationIdentityType  `json:"identity_type"`
	Status                   models.ProductReservationStatus `json:"status"`
	Quantity                 int                             `json:"quantity"`
	MaskedPhone              string                          `json:"masked_phone,omitempty"`
	Channel                  models.ReservationContactType   `json:"channel,omitempty"`
	BrowserHashPrefix        string                          `json:"browser_hash_prefix,omitempty"`
	IPHashPrefix             string                          `json:"ip_hash_prefix,omitempty"`
	BrowserReservationCount  int64                           `json:"browser_reservation_count"`
	RecentIPReservationCount int64                           `json:"recent_ip_reservation_count"`
	RateLimitFlagged         bool                            `json:"rate_limit_flagged"`
	ExpiresAt                *time.Time                      `json:"expires_at,omitempty"`
	RiskFlagged              bool                            `json:"risk_flagged"`
	RiskFlaggedAt            *time.Time                      `json:"risk_flagged_at,omitempty"`
	CreatedAt                time.Time                       `json:"created_at"`
	Product                  AnonymousReservationProductDTO  `json:"product"`
}

type AnonymousReservationAdminPage struct {
	Items    []AnonymousReservationAdminDTO `json:"items"`
	Total    int64                          `json:"total"`
	Page     int                            `json:"page"`
	PageSize int                            `json:"page_size"`
}

type AnonymousReservationAdminQuery struct {
	Page         int
	PageSize     int
	IdentityType string
	Status       string
	RiskFlagged  *bool
}

func NewAnonymousReservationService(
	repo anonymousReservationRepository,
	options AnonymousReservationOptions,
) (*AnonymousReservationService, error) {
	if repo == nil || options.Turnstile == nil {
		return nil, errors.New("anonymous reservation repository and Turnstile verifier are required")
	}
	if options.TTL <= 0 {
		return nil, errors.New("anonymous reservation TTL must be positive")
	}
	for _, value := range []int{
		options.RateLimits.BrowserPerHour,
		options.RateLimits.BrowserPerDay,
		options.RateLimits.IPPerHour,
		options.RateLimits.IPPerDay,
		options.RateLimits.IPProductPerDay,
	} {
		if value <= 0 {
			return nil, errors.New("anonymous reservation rate limits must be positive")
		}
	}
	clock := options.Now
	if clock == nil {
		clock = time.Now
	}
	return &AnonymousReservationService{
		repo: repo, now: clock, turnstile: options.Turnstile, ttl: options.TTL,
		rateLimits: options.RateLimits, locks: newReservationKeyLocker(),
	}, nil
}

func (s *AnonymousReservationService) Create(
	ctx context.Context,
	input AnonymousReservationCreateInput,
) (AnonymousReservationDTO, error) {
	note := strings.TrimSpace(input.Note)
	if input.ProductID == 0 || input.Quantity < 1 || input.Quantity > 99 ||
		utf8.RuneCountInString(note) > maxAnonymousReservationNoteRunes ||
		len(input.BrowserHash) != sha256.Size*2 || len(input.IPHash) != sha256.Size*2 {
		return AnonymousReservationDTO{}, ErrInvalidInput
	}
	now := s.now().UTC()
	turnstileRequestID, err := NewReservationTurnstileRequestID()
	if err != nil {
		return AnonymousReservationDTO{}, ErrServiceUnavailable
	}
	verification, err := s.turnstile.Verify(ctx, input.TurnstileToken, input.RemoteIP, turnstileRequestID)
	if err != nil {
		reason := models.ReservationReasonTurnstileInvalid
		result := models.ReservationAttemptBlocked
		if errors.Is(err, ErrServiceUnavailable) {
			reason = models.ReservationReasonTurnstileTimeout
			result = models.ReservationAttemptFailed
		}
		s.recordAttempt(ctx, anonymousReservationAttempt(
			input.ProductID, input.BrowserHash, input.IPHash, result, reason, now,
		))
		return AnonymousReservationDTO{}, err
	}

	unlockBrowser := s.locks.acquire("browser:" + input.BrowserHash)
	defer unlockBrowser()
	unlockIP := s.locks.acquire("ip:" + input.IPHash)
	defer unlockIP()

	activeIdentityKey := anonymousReservationActiveKey(input.ProductID, input.BrowserHash)
	expiresAt := now.Add(s.ttl)
	reservation := &models.ProductReservation{
		IdentityType:            models.ReservationIdentityAnonymous,
		ReservationProductID:    input.ProductID,
		BrowserHash:             optionalReservationString(input.BrowserHash),
		IPHash:                  optionalReservationString(input.IPHash),
		ActiveIdentityKey:       optionalReservationString(activeIdentityKey),
		VerificationStatus:      models.ReservationVerificationNotRequired,
		Status:                  models.ProductReservationPending,
		VerificationRequestedAt: now,
		TurnstileVerifiedAt:     &verification.VerifiedAt,
		Quantity:                input.Quantity,
		Note:                    optionalReservationString(note),
		ExpiresAt:               &expiresAt,
		SourceType:              models.ReservationSourceDirect,
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	stored, duplicate, err := s.repo.Create(
		ctx,
		reservation,
		anonymousReservationAttempt(
			input.ProductID, input.BrowserHash, input.IPHash,
			models.ReservationAttemptAccepted, "", now,
		),
		s.rateLimits,
		now,
	)
	if errors.Is(err, repository.ErrAnonymousBrowserRateLimited) {
		s.recordAttempt(ctx, anonymousReservationAttempt(
			input.ProductID, input.BrowserHash, input.IPHash,
			models.ReservationAttemptBlocked, models.ReservationReasonBrowserRateLimited, now,
		))
		return AnonymousReservationDTO{}, &AnonymousReservationRateLimitError{Scope: "browser", RetryAfterSeconds: 3600}
	}
	if errors.Is(err, repository.ErrAnonymousIPRateLimited) {
		s.recordAttempt(ctx, anonymousReservationAttempt(
			input.ProductID, input.BrowserHash, input.IPHash,
			models.ReservationAttemptBlocked, models.ReservationReasonIPRateLimited, now,
		))
		return AnonymousReservationDTO{}, &AnonymousReservationRateLimitError{Scope: "ip", RetryAfterSeconds: 3600}
	}
	if errors.Is(err, repository.ErrReservationProductNotOpen) {
		s.recordAttempt(ctx, anonymousReservationAttempt(
			input.ProductID, input.BrowserHash, input.IPHash,
			models.ReservationAttemptBlocked, models.ReservationReasonProductUnavailable, now,
		))
		return AnonymousReservationDTO{}, ErrAnonymousProductNotReservable
	}
	if err != nil {
		return AnonymousReservationDTO{}, mapReservationPersistenceError(err)
	}
	dto := anonymousReservationDTO(stored, input.Locale)
	if duplicate {
		return AnonymousReservationDTO{}, &AnonymousReservationDuplicateError{Existing: dto}
	}
	return dto, nil
}

func (s *AnonymousReservationService) Mine(
	ctx context.Context,
	browserHash, requestedLocale string,
) ([]AnonymousReservationDTO, error) {
	if len(browserHash) != sha256.Size*2 {
		return nil, ErrInvalidInput
	}
	reservations, err := s.repo.ListByBrowser(ctx, browserHash, s.now().UTC())
	if err != nil {
		return nil, err
	}
	result := make([]AnonymousReservationDTO, 0, len(reservations))
	for index := range reservations {
		result = append(result, anonymousReservationDTO(&reservations[index], requestedLocale))
	}
	return result, nil
}

func (s *AnonymousReservationService) Cancel(
	ctx context.Context,
	reservationCode, browserHash, requestedLocale string,
) (AnonymousReservationDTO, error) {
	reservationCode = strings.TrimSpace(reservationCode)
	if !validReservationCode(reservationCode) || len(browserHash) != sha256.Size*2 {
		return AnonymousReservationDTO{}, ErrInvalidInput
	}
	reservation, err := s.repo.CancelByBrowser(ctx, reservationCode, browserHash, s.now().UTC())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AnonymousReservationDTO{}, ErrNotFound
	}
	if errors.Is(err, repository.ErrAnonymousReservationNotPending) {
		return AnonymousReservationDTO{}, ErrAnonymousReservationNotPending
	}
	if err != nil {
		return AnonymousReservationDTO{}, err
	}
	return anonymousReservationDTO(reservation, requestedLocale), nil
}

func (s *AnonymousReservationService) ExpireDue(ctx context.Context, batchSize int) (int64, error) {
	return s.repo.ExpireDue(ctx, s.now().UTC(), batchSize)
}

func (s *AnonymousReservationService) AdminReservations(
	ctx context.Context,
	query AnonymousReservationAdminQuery,
) (*AnonymousReservationAdminPage, error) {
	if _, err := s.repo.ExpireDue(ctx, s.now().UTC(), 5000); err != nil {
		return nil, err
	}
	page, pageSize, _, err := normalizeReservationPage(query.Page, query.PageSize, "")
	if err != nil {
		return nil, err
	}
	var identityType *models.ReservationIdentityType
	if value := strings.TrimSpace(query.IdentityType); value != "" {
		parsed := models.ReservationIdentityType(value)
		if parsed != models.ReservationIdentityAnonymous && parsed != models.ReservationIdentityPhoneVerified {
			return nil, ErrInvalidInput
		}
		identityType = &parsed
	}
	var status *models.ProductReservationStatus
	if value := strings.TrimSpace(query.Status); value != "" {
		parsed := models.ProductReservationStatus(value)
		if !validReservationRecordStatus(parsed) {
			return nil, ErrInvalidInput
		}
		status = &parsed
	}
	reservations, total, err := s.repo.AdminList(ctx, repository.AnonymousReservationAdminQuery{
		Page: page, PageSize: pageSize, IdentityType: identityType, Status: status, RiskFlagged: query.RiskFlagged,
	})
	if err != nil {
		return nil, err
	}
	items := make([]AnonymousReservationAdminDTO, 0, len(reservations))
	for index := range reservations {
		item, itemErr := s.adminDTO(ctx, &reservations[index])
		if itemErr != nil {
			return nil, itemErr
		}
		items = append(items, item)
	}
	return &AnonymousReservationAdminPage{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *AnonymousReservationService) AdminSetStatus(
	ctx context.Context,
	reservationCode, status string,
) (AnonymousReservationAdminDTO, error) {
	reservationCode = strings.TrimSpace(reservationCode)
	parsed := models.ProductReservationStatus(strings.TrimSpace(status))
	if !validReservationCode(reservationCode) ||
		(parsed != models.ProductReservationConfirmed && parsed != models.ProductReservationRejected && parsed != models.ProductReservationCancelled) {
		return AnonymousReservationAdminDTO{}, ErrInvalidInput
	}
	reservation, err := s.repo.AdminSetStatus(ctx, reservationCode, parsed, s.now().UTC())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AnonymousReservationAdminDTO{}, ErrNotFound
	}
	if errors.Is(err, repository.ErrAnonymousReservationBadStatus) {
		return AnonymousReservationAdminDTO{}, ErrConflict
	}
	if err != nil {
		return AnonymousReservationAdminDTO{}, err
	}
	return s.adminDTO(ctx, reservation)
}

func (s *AnonymousReservationService) AdminSetRisk(
	ctx context.Context,
	reservationCode string,
	flagged bool,
) (AnonymousReservationAdminDTO, error) {
	reservationCode = strings.TrimSpace(reservationCode)
	if !validReservationCode(reservationCode) {
		return AnonymousReservationAdminDTO{}, ErrInvalidInput
	}
	reservation, err := s.repo.AdminSetRisk(ctx, reservationCode, flagged, s.now().UTC())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AnonymousReservationAdminDTO{}, ErrNotFound
	}
	if err != nil {
		return AnonymousReservationAdminDTO{}, err
	}
	return s.adminDTO(ctx, reservation)
}

func (s *AnonymousReservationService) adminDTO(
	ctx context.Context,
	reservation *models.ProductReservation,
) (AnonymousReservationAdminDTO, error) {
	browserHash := dereferenceReservationString(reservation.BrowserHash)
	ipHash := dereferenceReservationString(reservation.IPHash)
	browserCount, ipCount, err := s.repo.IdentityCounts(ctx, browserHash, ipHash, s.now().UTC().Add(-24*time.Hour))
	if err != nil {
		return AnonymousReservationAdminDTO{}, err
	}
	rateLimitFlagged, err := s.repo.WasRateLimited(ctx, browserHash, ipHash, s.now().UTC().Add(-24*time.Hour))
	if err != nil {
		return AnonymousReservationAdminDTO{}, err
	}
	item := AnonymousReservationAdminDTO{
		ReservationCode: reservation.ReservationCode, IdentityType: reservation.IdentityType,
		Status: reservation.Status, Quantity: reservation.Quantity,
		BrowserHashPrefix: hashPrefix(browserHash), IPHashPrefix: hashPrefix(ipHash),
		BrowserReservationCount: browserCount, RecentIPReservationCount: ipCount,
		RateLimitFlagged: rateLimitFlagged,
		ExpiresAt:        reservation.ExpiresAt, RiskFlagged: reservation.RiskFlagged,
		RiskFlaggedAt: reservation.RiskFlaggedAt, CreatedAt: reservation.CreatedAt,
		Product: anonymousReservationProductDTO(&reservation.ReservationProduct, "en"),
	}
	if reservation.IdentityType == models.ReservationIdentityPhoneVerified {
		item.MaskedPhone = MaskReservationPhone(reservation.PhoneCountryCode, reservation.PhoneLast4)
		item.Channel = reservation.ContactType
	}
	return item, nil
}

func (s *AnonymousReservationService) recordAttempt(ctx context.Context, event *models.ReservationAttemptEvent) {
	if event != nil {
		_ = s.repo.CreateAttemptEvent(ctx, event)
	}
}

func anonymousReservationAttempt(
	productID uint64,
	browserHash, ipHash string,
	result models.ReservationAttemptResult,
	reason models.ReservationReasonCode,
	now time.Time,
) *models.ReservationAttemptEvent {
	event := &models.ReservationAttemptEvent{
		BrowserHash: optionalReservationString(browserHash),
		IPHash:      optionalReservationString(ipHash),
		Action:      models.ReservationAttemptCreate,
		Result:      result,
		CreatedAt:   now.UTC(),
	}
	if productID != 0 {
		value := productID
		event.ReservationProductID = &value
	}
	if reason != "" {
		event.ReasonCode = &reason
	}
	return event
}

func anonymousReservationActiveKey(productID uint64, browserHash string) string {
	digest := sha256.Sum256([]byte(
		anonymousActiveIdentityDomain + strconv.FormatUint(productID, 10) + "\x00" + browserHash,
	))
	return hex.EncodeToString(digest[:])
}

func anonymousReservationDTO(reservation *models.ProductReservation, requestedLocale string) AnonymousReservationDTO {
	return AnonymousReservationDTO{
		ReservationCode: reservation.ReservationCode,
		IdentityType:    reservation.IdentityType,
		Status:          reservation.Status,
		Quantity:        reservation.Quantity,
		Note:            reservation.Note,
		CreatedAt:       reservation.CreatedAt.UTC(),
		ExpiresAt:       reservation.ExpiresAt,
		Product:         anonymousReservationProductDTO(&reservation.ReservationProduct, requestedLocale),
	}
}

func anonymousReservationProductDTO(product *models.ReservationProduct, requestedLocale string) AnonymousReservationProductDTO {
	title := product.TitleEN
	if locale.NormalizeLocale(requestedLocale) == "zh" && product.TitleZH != nil {
		title = *product.TitleZH
	}
	return AnonymousReservationProductDTO{
		ID: product.ID, Slug: product.Slug, Title: title, MainImageURL: product.MainImageURL,
	}
}

func validReservationCode(value string) bool {
	if len(value) < 20 || len(value) > 48 || !strings.HasPrefix(value, "GC-") {
		return false
	}
	for _, character := range value[3:] {
		if !((character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') ||
			(character >= 'a' && character <= 'z') || character == '-' || character == '_') {
			return false
		}
	}
	return true
}

func validReservationRecordStatus(status models.ProductReservationStatus) bool {
	switch status {
	case models.ProductReservationActive, models.ProductReservationPending, models.ProductReservationConfirmed,
		models.ProductReservationCancelled, models.ProductReservationRejected, models.ProductReservationExpired:
		return true
	default:
		return false
	}
}

func dereferenceReservationString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func hashPrefix(value string) string {
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}
