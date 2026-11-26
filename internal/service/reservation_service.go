package service

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"saas/internal/models"
	"saas/internal/pkg/locale"
	"saas/internal/repository"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	maxReservationPageSize          = 100
	maxReservationSearchLength      = 100
	maxReservationDescriptionLength = 2_000_000
	maxReservationGalleryImages     = 12
	reservationInterestWeight       = 5
)

var (
	reservationSlugPattern           = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	ErrReservationProductUnavailable = &contextualError{
		message: "reservation product is not open",
		target:  ErrConflict,
	}
)

type reservationProductRepository interface {
	Create(ctx context.Context, product *models.ReservationProduct) error
	Update(ctx context.Context, product *models.ReservationProduct) error
	Archive(ctx context.Context, id uint64) error
	FindByID(ctx context.Context, id uint64) (*models.ReservationProduct, error)
	FindPublicByID(ctx context.Context, id uint64, now time.Time) (*models.ReservationProduct, error)
	FindPublicBySlug(ctx context.Context, slug string, now time.Time) (*models.ReservationProduct, error)
	List(ctx context.Context, query repository.ReservationProductListQuery) ([]models.ReservationProduct, int64, error)
	LinkedProductExists(ctx context.Context, id uint) (bool, error)
	CountInterests(ctx context.Context, productID uint64) (int64, error)
	CountActiveVerified(ctx context.Context, productID uint64) (int64, error)
	HasInterest(ctx context.Context, productID uint64, visitorID string) (bool, error)
	HasActiveVerifiedByVisitor(ctx context.Context, productID uint64, visitorID string) (bool, error)
	CreateInterest(ctx context.Context, interest *models.ReservationInterest, attempt *models.ReservationAttemptEvent) (bool, error)
	CreateAttemptEvent(ctx context.Context, event *models.ReservationAttemptEvent) error
	Metrics(ctx context.Context, productID uint64) (repository.ReservationProductMetrics, error)
	ListReservationContacts(ctx context.Context, productID uint64) ([]models.ProductReservation, error)
}

type ReservationHeatThresholds struct {
	PopularMin         int64
	TrendingMin        int64
	HighlyRequestedMin int64
}

type ReservationServiceOptions struct {
	Now            func() time.Time
	HeatThresholds ReservationHeatThresholds
	Limiter        *ReservationMemoryLimiter
	RateLimits     ReservationRateLimits
}

type ReservationService struct {
	repo           reservationProductRepository
	now            func() time.Time
	heatThresholds ReservationHeatThresholds
	limiter        *ReservationMemoryLimiter
	rateLimits     ReservationRateLimits
}

type ReservationProductInput struct {
	LinkedProductID    *uint
	Slug               string
	TitleEN            string
	TitleZH            *string
	SummaryEN          string
	SummaryZH          *string
	DescriptionEN      string
	DescriptionZH      *string
	MainImageURL       string
	Gallery            []string
	Status             models.ReservationProductStatus
	ReservationStartAt *time.Time
	ReservationEndAt   *time.Time
	DisplayOrder       int
	IsFeatured         bool
	HeatLevelOverride  *models.ReservationHeatLevel
	CreatedBy          uint
}

type ReservationPublicQuery struct {
	Page      int
	PageSize  int
	Search    string
	Featured  *bool
	Locale    string
	VisitorID string
}

type ReservationAdminQuery struct {
	Page     int
	PageSize int
	Search   string
	Status   *models.ReservationProductStatus
}

type ReservationViewerState struct {
	Interested       bool `json:"interested"`
	VerifiedReserved bool `json:"verified_reserved"`
}

type ReservationProductPublicDTO struct {
	ID                 uint64                      `json:"id"`
	Slug               string                      `json:"slug"`
	Title              string                      `json:"title"`
	TitleEN            string                      `json:"title_en"`
	TitleZH            *string                     `json:"title_zh"`
	Summary            string                      `json:"summary"`
	SummaryEN          string                      `json:"summary_en"`
	SummaryZH          *string                     `json:"summary_zh"`
	Description        string                      `json:"description"`
	DescriptionEN      string                      `json:"description_en"`
	DescriptionZH      *string                     `json:"description_zh"`
	MainImageURL       string                      `json:"main_image_url"`
	Gallery            []string                    `json:"gallery"`
	ReservationStartAt *time.Time                  `json:"reservation_start_at"`
	ReservationEndAt   *time.Time                  `json:"reservation_end_at"`
	IsFeatured         bool                        `json:"is_featured"`
	HeatLevel          models.ReservationHeatLevel `json:"heat_level"`
	ViewerState        ReservationViewerState      `json:"viewer_state"`
}

type ReservationProductAdminDTO struct {
	ID                 uint64                          `json:"id"`
	LinkedProductID    *uint                           `json:"linked_product_id"`
	Slug               string                          `json:"slug"`
	TitleEN            string                          `json:"title_en"`
	TitleZH            *string                         `json:"title_zh"`
	SummaryEN          string                          `json:"summary_en"`
	SummaryZH          *string                         `json:"summary_zh"`
	DescriptionEN      string                          `json:"description_en"`
	DescriptionZH      *string                         `json:"description_zh"`
	MainImageURL       string                          `json:"main_image_url"`
	Gallery            []string                        `json:"gallery"`
	Status             models.ReservationProductStatus `json:"status"`
	ReservationStartAt *time.Time                      `json:"reservation_start_at"`
	ReservationEndAt   *time.Time                      `json:"reservation_end_at"`
	DisplayOrder       int                             `json:"display_order"`
	IsFeatured         bool                            `json:"is_featured"`
	HeatLevelOverride  *models.ReservationHeatLevel    `json:"heat_level_override"`
	AutomaticHeatLevel models.ReservationHeatLevel     `json:"automatic_heat_level"`
	EffectiveHeatLevel models.ReservationHeatLevel     `json:"effective_heat_level"`
	ExactInterestCount int64                           `json:"exact_interest_count"`
	ExactVerifiedCount int64                           `json:"exact_verified_count"`
	CreatedBy          uint                            `json:"created_by"`
	CreatedAt          time.Time                       `json:"created_at"`
	UpdatedAt          time.Time                       `json:"updated_at"`
}

type ReservationProductPublicPage struct {
	Items    []ReservationProductPublicDTO `json:"items"`
	Total    int64                         `json:"total"`
	Page     int                           `json:"page"`
	PageSize int                           `json:"page_size"`
}

type ReservationProductAdminPage struct {
	Items    []ReservationProductAdminDTO `json:"items"`
	Total    int64                        `json:"total"`
	Page     int                          `json:"page"`
	PageSize int                          `json:"page_size"`
}

type ReservationInterestInput struct {
	VisitorID      string
	SessionID      string
	IdempotencyKey string
	SourceType     models.ReservationSourceType
	SourceID       string
	IPRiskKey      string
}

type ReservationInterestResult struct {
	Interested bool `json:"interested"`
	Created    bool `json:"created"`
}

type ReservationProductAnalyticsDTO struct {
	Impressions            int64                        `json:"impressions"`
	PageViews              int64                        `json:"page_views"`
	UniqueInterests        int64                        `json:"unique_interests"`
	ReservationSubmissions int64                        `json:"reservation_submissions"`
	VerifiedReservations   int64                        `json:"verified_reservations"`
	VerificationRate       float64                      `json:"verification_rate"`
	CancelledReservations  int64                        `json:"cancelled_reservations"`
	BlockedAttempts        int64                        `json:"blocked_attempts"`
	Contacts               []ReservationContactAdminDTO `json:"contacts"`
}

type ReservationContactAdminDTO struct {
	MaskedPhone string                          `json:"masked_phone"`
	Channel     models.ReservationContactType   `json:"channel"`
	VerifiedAt  *time.Time                      `json:"verified_at"`
	Status      models.ProductReservationStatus `json:"status"`
}

func NewReservationService(repo reservationProductRepository, options ...ReservationServiceOptions) *ReservationService {
	service := &ReservationService{
		repo: repo,
		now:  time.Now,
		heatThresholds: ReservationHeatThresholds{
			PopularMin: 5, TrendingMin: 20, HighlyRequestedMin: 50,
		},
	}
	if len(options) > 0 {
		if options[0].Now != nil {
			service.now = options[0].Now
		}
		if validReservationHeatThresholds(options[0].HeatThresholds) {
			service.heatThresholds = options[0].HeatThresholds
		}
		service.limiter = options[0].Limiter
		service.rateLimits = options[0].RateLimits
	}
	return service
}

func (s *ReservationService) CreateProduct(ctx context.Context, input ReservationProductInput) (*ReservationProductAdminDTO, error) {
	product, err := s.productFromInput(ctx, 0, input)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, product); err != nil {
		return nil, mapReservationPersistenceError(err)
	}
	return s.adminDTO(ctx, product)
}

func (s *ReservationService) UpdateProduct(ctx context.Context, id uint64, input ReservationProductInput) (*ReservationProductAdminDTO, error) {
	if id == 0 {
		return nil, ErrInvalidInput
	}
	existing, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	input.CreatedBy = existing.CreatedBy
	product, err := s.productFromInput(ctx, id, input)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, product); err != nil {
		return nil, mapReservationPersistenceError(err)
	}
	updated, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, mapReservationPersistenceError(err)
	}
	return s.adminDTO(ctx, updated)
}

func (s *ReservationService) ArchiveProduct(ctx context.Context, id uint64) error {
	if id == 0 {
		return ErrInvalidInput
	}
	return mapReservationPersistenceError(s.repo.Archive(ctx, id))
}

func (s *ReservationService) AdminProduct(ctx context.Context, id uint64) (*ReservationProductAdminDTO, error) {
	if id == 0 {
		return nil, ErrInvalidInput
	}
	product, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, mapReservationPersistenceError(err)
	}
	return s.adminDTO(ctx, product)
}

func (s *ReservationService) AdminProducts(ctx context.Context, query ReservationAdminQuery) (*ReservationProductAdminPage, error) {
	page, pageSize, search, err := normalizeReservationPage(query.Page, query.PageSize, query.Search)
	if err != nil {
		return nil, err
	}
	if query.Status != nil && !query.Status.IsValid() {
		return nil, ErrInvalidInput
	}
	products, total, err := s.repo.List(ctx, repository.ReservationProductListQuery{
		Page: page, PageSize: pageSize, Search: search, Status: query.Status,
	})
	if err != nil {
		return nil, err
	}
	items := make([]ReservationProductAdminDTO, 0, len(products))
	for index := range products {
		item, dtoErr := s.adminDTO(ctx, &products[index])
		if dtoErr != nil {
			return nil, dtoErr
		}
		items = append(items, *item)
	}
	return &ReservationProductAdminPage{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *ReservationService) PublicProducts(ctx context.Context, query ReservationPublicQuery) (*ReservationProductPublicPage, error) {
	page, pageSize, search, err := normalizeReservationPage(query.Page, query.PageSize, query.Search)
	if err != nil {
		return nil, err
	}
	visitorID, err := normalizeOptionalReservationUUID(query.VisitorID)
	if err != nil {
		return nil, err
	}
	products, total, err := s.repo.List(ctx, repository.ReservationProductListQuery{
		Page: page, PageSize: pageSize, Search: search, Featured: query.Featured, PublicOnly: true, Now: s.now().UTC(),
	})
	if err != nil {
		return nil, err
	}
	items := make([]ReservationProductPublicDTO, 0, len(products))
	for index := range products {
		item, dtoErr := s.publicDTO(ctx, &products[index], query.Locale, visitorID)
		if dtoErr != nil {
			return nil, dtoErr
		}
		items = append(items, *item)
	}
	return &ReservationProductPublicPage{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *ReservationService) PublicProduct(ctx context.Context, slug, requestedLocale, visitorID string) (*ReservationProductPublicDTO, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if !reservationSlugPattern.MatchString(slug) || len(slug) > 120 {
		return nil, ErrInvalidInput
	}
	normalizedVisitor, err := normalizeOptionalReservationUUID(visitorID)
	if err != nil {
		return nil, err
	}
	product, err := s.repo.FindPublicBySlug(ctx, slug, s.now().UTC())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.publicDTO(ctx, product, requestedLocale, normalizedVisitor)
}

func (s *ReservationService) RecordInterest(ctx context.Context, productID uint64, input ReservationInterestInput) (ReservationInterestResult, error) {
	// Apply the non-user-controlled IP/global boundaries before parsing the
	// visitor UUID so rotating invalid visitor IDs cannot bypass ingress limits.
	if s.limiter != nil {
		for _, rule := range []reservationLimitRule{
			{"interest_ip_minute", input.IPRiskKey, s.rateLimits.InterestIPPerMinute, time.Minute},
			{"interest_global_minute", "global", s.rateLimits.InterestGlobalPerMinute, time.Minute},
		} {
			if rule.key == "" {
				continue
			}
			if allowed, retry := s.limiter.Allow(rule.scope, rule.key, rule.limit, rule.window); !allowed {
				s.recordInterestAttempt(ctx, productID, "", input.IPRiskKey, models.ReservationAttemptBlocked, models.ReservationReasonRateLimited)
				return ReservationInterestResult{}, reservationRetryAfterError(retry)
			}
		}
	}
	if productID == 0 {
		return ReservationInterestResult{}, ErrInvalidInput
	}
	visitorID, err := normalizeRequiredReservationUUID(input.VisitorID)
	if err != nil {
		return ReservationInterestResult{}, err
	}
	sessionID, err := normalizeOptionalReservationUUID(input.SessionID)
	if err != nil {
		return ReservationInterestResult{}, err
	}
	idempotencyKey, err := normalizeRequiredReservationUUID(input.IdempotencyKey)
	if err != nil {
		return ReservationInterestResult{}, err
	}
	sourceType := input.SourceType
	if sourceType == "" {
		sourceType = models.ReservationSourceDirect
	}
	if !sourceType.IsValid() || !validOptionalStorefrontBusinessID(strings.TrimSpace(input.SourceID), maxStorefrontSourceIDLength) {
		return ReservationInterestResult{}, ErrInvalidInput
	}
	if _, err := s.repo.FindPublicByID(ctx, productID, s.now().UTC()); errors.Is(err, gorm.ErrRecordNotFound) {
		s.recordInterestAttempt(ctx, productID, visitorID, input.IPRiskKey, models.ReservationAttemptBlocked, models.ReservationReasonProductUnavailable)
		return ReservationInterestResult{}, ErrReservationProductUnavailable
	} else if err != nil {
		return ReservationInterestResult{}, err
	}
	if s.limiter != nil {
		for _, rule := range []reservationLimitRule{
			{"interest_visitor_minute", visitorID, s.rateLimits.InterestVisitorPerMinute, time.Minute},
		} {
			if rule.key == "" {
				continue
			}
			if allowed, retry := s.limiter.Allow(rule.scope, rule.key, rule.limit, rule.window); !allowed {
				s.recordInterestAttempt(ctx, productID, visitorID, input.IPRiskKey, models.ReservationAttemptBlocked, models.ReservationReasonRateLimited)
				return ReservationInterestResult{}, reservationRetryAfterError(retry)
			}
		}
	}

	interest := &models.ReservationInterest{
		ReservationProductID: productID,
		VisitorID:            visitorID,
		SessionID:            optionalReservationString(sessionID),
		IdempotencyKey:       idempotencyKey,
		SourceType:           sourceType,
		SourceID:             optionalReservationString(strings.TrimSpace(input.SourceID)),
		CreatedAt:            s.now().UTC(),
	}
	created, err := s.repo.CreateInterest(ctx, interest, reservationAttempt(
		productID, visitorID, "", input.IPRiskKey, models.ReservationAttemptInterest,
		models.ReservationAttemptAccepted, "", s.now().UTC(),
	))
	if errors.Is(err, repository.ErrReservationIdempotencyConflict) {
		return ReservationInterestResult{}, ErrConflict
	}
	if errors.Is(err, repository.ErrReservationProductNotOpen) {
		s.recordInterestAttempt(ctx, productID, visitorID, input.IPRiskKey, models.ReservationAttemptBlocked, models.ReservationReasonProductUnavailable)
		return ReservationInterestResult{}, ErrReservationProductUnavailable
	}
	if err != nil {
		return ReservationInterestResult{}, mapReservationPersistenceError(err)
	}
	return ReservationInterestResult{Interested: true, Created: created}, nil
}

func (s *ReservationService) recordInterestAttempt(
	ctx context.Context,
	productID uint64,
	visitorID, ipRiskKey string,
	result models.ReservationAttemptResult,
	reason models.ReservationReasonCode,
) {
	_ = s.repo.CreateAttemptEvent(ctx, reservationAttempt(
		productID, visitorID, "", ipRiskKey, models.ReservationAttemptInterest, result, reason, s.now().UTC(),
	))
}

func (s *ReservationService) ProductAnalytics(ctx context.Context, productID uint64) (*ReservationProductAnalyticsDTO, error) {
	if productID == 0 {
		return nil, ErrInvalidInput
	}
	if _, err := s.repo.FindByID(ctx, productID); err != nil {
		return nil, mapReservationPersistenceError(err)
	}
	metrics, err := s.repo.Metrics(ctx, productID)
	if err != nil {
		return nil, err
	}
	contacts, err := s.repo.ListReservationContacts(ctx, productID)
	if err != nil {
		return nil, err
	}
	contactDTOs := make([]ReservationContactAdminDTO, 0, len(contacts))
	for index := range contacts {
		contactDTOs = append(contactDTOs, ReservationContactAdminDTO{
			MaskedPhone: MaskReservationPhone(contacts[index].PhoneCountryCode, contacts[index].PhoneLast4),
			Channel:     contacts[index].ContactType, VerifiedAt: contacts[index].VerifiedAt, Status: contacts[index].Status,
		})
	}
	verificationRate := float64(0)
	if metrics.ReservationSubmissions > 0 {
		verificationRate = float64(metrics.VerifiedReservations) / float64(metrics.ReservationSubmissions)
	}
	// Exposure and visit counts remain zero until a dedicated reservation event
	// source exists. Existing Storefront Analytics meanings are not changed.
	return &ReservationProductAnalyticsDTO{
		UniqueInterests: metrics.UniqueInterests, ReservationSubmissions: metrics.ReservationSubmissions,
		VerifiedReservations: metrics.VerifiedReservations, VerificationRate: verificationRate,
		CancelledReservations: metrics.CancelledReservations, BlockedAttempts: metrics.BlockedAttempts,
		Contacts: contactDTOs,
	}, nil
}

func (s *ReservationService) productFromInput(ctx context.Context, id uint64, input ReservationProductInput) (*models.ReservationProduct, error) {
	if input.CreatedBy == 0 {
		return nil, ErrForbidden
	}
	slug := strings.ToLower(strings.TrimSpace(input.Slug))
	titleEN := strings.TrimSpace(input.TitleEN)
	if !reservationSlugPattern.MatchString(slug) || len(slug) > 120 || titleEN == "" || utf8.RuneCountInString(titleEN) > 200 {
		return nil, ErrInvalidInput
	}
	titleZH, ok := normalizeOptionalReservationText(input.TitleZH, 200)
	if !ok || utf8.RuneCountInString(strings.TrimSpace(input.SummaryEN)) > 1000 {
		return nil, ErrInvalidInput
	}
	summaryZH, ok := normalizeOptionalReservationText(input.SummaryZH, 1000)
	if !ok || len(input.DescriptionEN) > maxReservationDescriptionLength {
		return nil, ErrInvalidInput
	}
	descriptionZH, ok := normalizeOptionalReservationHTML(input.DescriptionZH)
	if !ok {
		return nil, ErrInvalidInput
	}
	mainImageURL := strings.TrimSpace(input.MainImageURL)
	if len(mainImageURL) > 512 || (mainImageURL != "" && !isAllowedProductDetailImage(mainImageURL)) {
		return nil, ErrInvalidInput
	}
	gallery, err := normalizeReservationGallery(input.Gallery)
	if err != nil {
		return nil, err
	}
	status := input.Status
	if status == "" {
		status = models.ReservationProductStatusDraft
	}
	if !status.IsValid() || (status == models.ReservationProductStatusScheduled && input.ReservationStartAt == nil) {
		return nil, ErrInvalidInput
	}
	startAt := utcReservationTime(input.ReservationStartAt)
	endAt := utcReservationTime(input.ReservationEndAt)
	if startAt != nil && endAt != nil && !endAt.After(*startAt) {
		return nil, ErrInvalidInput
	}
	if input.DisplayOrder < -1_000_000 || input.DisplayOrder > 1_000_000 {
		return nil, ErrInvalidInput
	}
	override := input.HeatLevelOverride
	if override != nil && !override.IsValid() {
		return nil, ErrInvalidInput
	}
	linkedProductID := normalizeOptionalUint(input.LinkedProductID)
	if linkedProductID != nil {
		exists, existsErr := s.repo.LinkedProductExists(ctx, *linkedProductID)
		if existsErr != nil {
			return nil, existsErr
		}
		if !exists {
			return nil, ErrInvalidInput
		}
	}

	galleryJSON, err := json.Marshal(gallery)
	if err != nil {
		return nil, err
	}
	return &models.ReservationProduct{
		ID: id, LinkedProductID: linkedProductID, Slug: slug, TitleEN: titleEN, TitleZH: titleZH,
		SummaryEN: strings.TrimSpace(input.SummaryEN), SummaryZH: summaryZH,
		DescriptionEN: sanitizeProductDetailHTML(input.DescriptionEN), DescriptionZH: descriptionZH,
		MainImageURL: mainImageURL, GalleryJSON: datatypes.JSON(galleryJSON), Status: status,
		ReservationStartAt: startAt, ReservationEndAt: endAt, DisplayOrder: input.DisplayOrder,
		IsFeatured: input.IsFeatured, HeatLevelOverride: override, CreatedBy: input.CreatedBy,
	}, nil
}

func (s *ReservationService) adminDTO(ctx context.Context, product *models.ReservationProduct) (*ReservationProductAdminDTO, error) {
	interestCount, err := s.repo.CountInterests(ctx, product.ID)
	if err != nil {
		return nil, err
	}
	verifiedCount, err := s.repo.CountActiveVerified(ctx, product.ID)
	if err != nil {
		return nil, err
	}
	automatic := reservationAutomaticHeatLevel(verifiedCount, interestCount, s.heatThresholds)
	effective := automatic
	if product.HeatLevelOverride != nil {
		effective = *product.HeatLevelOverride
	}
	return &ReservationProductAdminDTO{
		ID: product.ID, LinkedProductID: product.LinkedProductID, Slug: product.Slug,
		TitleEN: product.TitleEN, TitleZH: product.TitleZH, SummaryEN: product.SummaryEN, SummaryZH: product.SummaryZH,
		DescriptionEN: product.DescriptionEN, DescriptionZH: product.DescriptionZH, MainImageURL: product.MainImageURL,
		Gallery: reservationGallery(product.GalleryJSON), Status: product.Status,
		ReservationStartAt: product.ReservationStartAt, ReservationEndAt: product.ReservationEndAt,
		DisplayOrder: product.DisplayOrder, IsFeatured: product.IsFeatured, HeatLevelOverride: product.HeatLevelOverride,
		AutomaticHeatLevel: automatic, EffectiveHeatLevel: effective, ExactInterestCount: interestCount,
		ExactVerifiedCount: verifiedCount, CreatedBy: product.CreatedBy, CreatedAt: product.CreatedAt, UpdatedAt: product.UpdatedAt,
	}, nil
}

func (s *ReservationService) publicDTO(ctx context.Context, product *models.ReservationProduct, requestedLocale, visitorID string) (*ReservationProductPublicDTO, error) {
	interestCount, err := s.repo.CountInterests(ctx, product.ID)
	if err != nil {
		return nil, err
	}
	verifiedCount, err := s.repo.CountActiveVerified(ctx, product.ID)
	if err != nil {
		return nil, err
	}
	heat := reservationAutomaticHeatLevel(verifiedCount, interestCount, s.heatThresholds)
	if product.HeatLevelOverride != nil {
		heat = *product.HeatLevelOverride
	}
	viewer := ReservationViewerState{}
	if visitorID != "" {
		viewer.Interested, err = s.repo.HasInterest(ctx, product.ID, visitorID)
		if err != nil {
			return nil, err
		}
		viewer.VerifiedReserved, err = s.repo.HasActiveVerifiedByVisitor(ctx, product.ID, visitorID)
		if err != nil {
			return nil, err
		}
	}
	title, summary, description := product.TitleEN, product.SummaryEN, product.DescriptionEN
	if locale.NormalizeLocale(requestedLocale) == "zh" {
		if product.TitleZH != nil {
			title = *product.TitleZH
		}
		if product.SummaryZH != nil {
			summary = *product.SummaryZH
		}
		if product.DescriptionZH != nil {
			description = *product.DescriptionZH
		}
	}
	return &ReservationProductPublicDTO{
		ID: product.ID, Slug: product.Slug,
		Title: title, TitleEN: product.TitleEN, TitleZH: product.TitleZH,
		Summary: summary, SummaryEN: product.SummaryEN, SummaryZH: product.SummaryZH,
		Description: description, DescriptionEN: product.DescriptionEN, DescriptionZH: product.DescriptionZH,
		MainImageURL: product.MainImageURL, Gallery: reservationGallery(product.GalleryJSON),
		ReservationStartAt: product.ReservationStartAt, ReservationEndAt: product.ReservationEndAt,
		IsFeatured: product.IsFeatured, HeatLevel: heat, ViewerState: viewer,
	}, nil
}

func reservationAutomaticHeatLevel(verified, interests int64, thresholds ReservationHeatThresholds) models.ReservationHeatLevel {
	score := verified*reservationInterestWeight + interests
	if verified >= thresholds.HighlyRequestedMin || score >= thresholds.HighlyRequestedMin*reservationInterestWeight {
		return models.ReservationHeatHighlyRequested
	}
	if verified >= thresholds.TrendingMin || score >= thresholds.TrendingMin*reservationInterestWeight {
		return models.ReservationHeatTrending
	}
	if verified >= thresholds.PopularMin || score >= thresholds.PopularMin*reservationInterestWeight {
		return models.ReservationHeatPopular
	}
	return models.ReservationHeatNew
}

func validReservationHeatThresholds(value ReservationHeatThresholds) bool {
	return value.PopularMin > 0 && value.TrendingMin > value.PopularMin && value.HighlyRequestedMin > value.TrendingMin
}

func normalizeReservationPage(page, pageSize int, search string) (int, int, string, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > maxReservationPageSize {
		pageSize = maxReservationPageSize
	}
	search = strings.TrimSpace(search)
	if utf8.RuneCountInString(search) > maxReservationSearchLength {
		return 0, 0, "", ErrInvalidInput
	}
	return page, pageSize, search, nil
}

func normalizeOptionalReservationText(value *string, max int) (*string, bool) {
	if value == nil {
		return nil, true
	}
	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return nil, true
	}
	if utf8.RuneCountInString(normalized) > max {
		return nil, false
	}
	return &normalized, true
}

func normalizeOptionalReservationHTML(value *string) (*string, bool) {
	if value == nil {
		return nil, true
	}
	if len(*value) > maxReservationDescriptionLength {
		return nil, false
	}
	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return nil, true
	}
	sanitized := sanitizeProductDetailHTML(normalized)
	return &sanitized, true
}

func normalizeReservationGallery(values []string) ([]string, error) {
	if len(values) > maxReservationGalleryImages {
		return nil, ErrInvalidInput
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 512 || !isAllowedProductDetailImage(value) {
			return nil, ErrInvalidInput
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func reservationGallery(value datatypes.JSON) []string {
	if len(value) == 0 {
		return []string{}
	}
	var gallery []string
	if json.Unmarshal(value, &gallery) != nil || gallery == nil {
		return []string{}
	}
	return gallery
}

func normalizeRequiredReservationUUID(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if !validStorefrontUUID(value, maxStorefrontVisitorIDLength) {
		return "", ErrInvalidInput
	}
	return value, nil
}

func normalizeOptionalReservationUUID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	return normalizeRequiredReservationUUID(value)
}

func optionalReservationString(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}

func utcReservationTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	utc := value.UTC()
	return &utc
}

func mapReservationPersistenceError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	if errors.Is(err, repository.ErrReservationProductNotOpen) {
		return ErrReservationProductUnavailable
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrConflict
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unique constraint") || strings.Contains(message, "duplicate key") || strings.Contains(message, "unique failed") {
		return ErrConflict
	}
	return err
}
