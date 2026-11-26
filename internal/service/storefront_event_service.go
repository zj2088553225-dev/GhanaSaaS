package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"saas/internal/models"
	"saas/internal/pkg/locale"

	"gorm.io/gorm"
)

const (
	MaxStorefrontEventBatchSize   = 50
	maxStorefrontEventIDLength    = 36
	maxStorefrontVisitorIDLength  = 64
	maxStorefrontSessionIDLength  = 64
	maxStorefrontEntityTypeLength = 40
	maxStorefrontEntityIDLength   = 128
	maxStorefrontSourceTypeLength = 40
	maxStorefrontSourceIDLength   = 128
	maxStorefrontPageTypeLength   = 24
	maxStorefrontPagePathLength   = 512
	maxStorefrontReferrerLength   = 1024
)

var (
	storefrontUUIDPattern       = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	storefrontBusinessIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
)

type StorefrontEventInput struct {
	EventID    string                      `json:"event_id"`
	EventName  models.StorefrontEventName  `json:"event_name"`
	VisitorID  string                      `json:"visitor_id"`
	SessionID  string                      `json:"session_id"`
	EntityType models.StorefrontEntityType `json:"entity_type"`
	EntityID   string                      `json:"entity_id"`
	ProductID  *uint                       `json:"product_id"`
	SourceType models.StorefrontSourceType `json:"source_type"`
	SourceID   string                      `json:"source_id"`
	PageType   models.StorefrontPageType   `json:"page_type"`
	PagePath   string                      `json:"page_path"`
	Locale     string                      `json:"locale"`
	Referrer   string                      `json:"referrer"`
	DeviceType models.StorefrontDeviceType `json:"device_type"`
}

type StorefrontEventBatchInput struct {
	Events []StorefrontEventInput `json:"events"`
}

type StorefrontEventBatchResult struct {
	Accepted   int `json:"accepted"`
	Duplicates int `json:"duplicates"`
}

type storefrontEventRepository interface {
	Create(ctx context.Context, event *models.StorefrontEvent) error
	CreateMany(ctx context.Context, events []*models.StorefrontEvent) (accepted int, duplicates int, err error)
	FindExistingEventIDs(ctx context.Context, eventIDs []string) (map[string]struct{}, error)
	FindPublicProductTenantID(ctx context.Context, productID uint) (uint, error)
	FindPublicShopTenantID(ctx context.Context, identifier string) (uint, error)
	FindPlatformMallTenantID(ctx context.Context) (*uint, error)
}

type StorefrontEventService struct {
	repo storefrontEventRepository
	now  func() time.Time
}

type StorefrontEventOptions struct {
	Now func() time.Time
}

func NewStorefrontEventService(repo storefrontEventRepository, options ...StorefrontEventOptions) *StorefrontEventService {
	service := &StorefrontEventService{repo: repo, now: time.Now}
	if len(options) > 0 && options[0].Now != nil {
		service.now = options[0].Now
	}
	return service
}

func (s *StorefrontEventService) Track(ctx context.Context, input StorefrontEventInput, trafficTypes ...models.StorefrontTrafficType) error {
	event, err := normalizeStorefrontEventInput(input)
	if err != nil {
		return err
	}
	existing, err := s.repo.FindExistingEventIDs(ctx, []string{event.EventID})
	if err != nil {
		return err
	}
	if _, duplicate := existing[event.EventID]; duplicate {
		return nil
	}

	tenantID, err := s.deriveStorefrontEventTenant(ctx, event)
	if err != nil {
		return err
	}
	event.TenantID = tenantID
	event.TrafficType = normalizedStorefrontTrafficType(trafficTypes)
	now := s.now().UTC()
	event.OccurredAt = now
	event.CreatedAt = now
	applyStorefrontPopularityThrottle(event, now)
	return s.repo.Create(ctx, event)
}

// TrackBatch uses atomic validation semantics: every input is normalized and
// scoped before the repository receives anything. One invalid event therefore
// rejects the entire batch without leaving partial analytics rows behind.
func (s *StorefrontEventService) TrackBatch(ctx context.Context, input StorefrontEventBatchInput, trafficTypes ...models.StorefrontTrafficType) (StorefrontEventBatchResult, error) {
	if len(input.Events) == 0 || len(input.Events) > MaxStorefrontEventBatchSize {
		return StorefrontEventBatchResult{}, ErrInvalidInput
	}

	events := make([]*models.StorefrontEvent, len(input.Events))
	eventIDs := make([]string, len(input.Events))
	for index, eventInput := range input.Events {
		event, err := normalizeStorefrontEventInput(eventInput)
		if err != nil {
			return StorefrontEventBatchResult{}, err
		}
		events[index] = event
		eventIDs[index] = event.EventID
	}
	existing, err := s.repo.FindExistingEventIDs(ctx, eventIDs)
	if err != nil {
		return StorefrontEventBatchResult{}, err
	}

	for _, event := range events {
		if _, duplicate := existing[event.EventID]; duplicate {
			continue
		}
		tenantID, err := s.deriveStorefrontEventTenant(ctx, event)
		if err != nil {
			return StorefrontEventBatchResult{}, err
		}
		event.TenantID = tenantID
	}

	now := s.now().UTC()
	trafficType := normalizedStorefrontTrafficType(trafficTypes)
	for _, event := range events {
		event.TrafficType = trafficType
		event.OccurredAt = now
		event.CreatedAt = now
		applyStorefrontPopularityThrottle(event, now)
	}
	accepted, duplicates, err := s.repo.CreateMany(ctx, events)
	if err != nil {
		return StorefrontEventBatchResult{}, err
	}
	return StorefrontEventBatchResult{Accepted: accepted, Duplicates: duplicates}, nil
}

func applyStorefrontPopularityThrottle(event *models.StorefrontEvent, now time.Time) {
	if event == nil || event.ProductID == nil || event.TenantID == nil {
		return
	}
	interval := storefrontPopularityThrottleInterval(event.EventName)
	if interval <= 0 {
		return
	}
	identity := strings.ToLower(strings.TrimSpace(event.VisitorID))
	if identity == "" {
		return
	}
	material := strings.Join([]string{
		"storefront-popularity-throttle:v1",
		strconv.FormatUint(uint64(*event.TenantID), 10),
		identity,
		strconv.FormatUint(uint64(*event.ProductID), 10),
		string(event.EventName),
	}, "\x00")
	digest := sha256.Sum256([]byte(material))
	event.ThrottleKey = hex.EncodeToString(digest[:])
	event.ThrottleUntil = now.UTC().Add(interval)
}

func storefrontPopularityThrottleInterval(eventName models.StorefrontEventName) time.Duration {
	switch eventName {
	case models.StorefrontEventProductView:
		return 30 * time.Second
	case models.StorefrontEventProductClick:
		return 10 * time.Second
	case models.StorefrontEventWhatsAppClick, models.StorefrontEventPhoneClick:
		return 60 * time.Second
	default:
		return 0
	}
}

func normalizeStorefrontEventInput(input StorefrontEventInput) (*models.StorefrontEvent, error) {
	input.EventID = strings.TrimSpace(input.EventID)
	input.VisitorID = strings.TrimSpace(input.VisitorID)
	input.SessionID = strings.TrimSpace(input.SessionID)
	input.EntityID = strings.TrimSpace(input.EntityID)
	input.SourceID = strings.TrimSpace(input.SourceID)
	input.PagePath = strings.TrimSpace(input.PagePath)
	input.Referrer = strings.TrimSpace(input.Referrer)

	if !validStorefrontUUID(input.EventID, maxStorefrontEventIDLength) ||
		!input.EventName.IsValid() ||
		utf8.RuneCountInString(string(input.EntityType)) > maxStorefrontEntityTypeLength ||
		utf8.RuneCountInString(string(input.SourceType)) > maxStorefrontSourceTypeLength ||
		utf8.RuneCountInString(string(input.PageType)) > maxStorefrontPageTypeLength {
		return nil, ErrInvalidInput
	}
	if !validStorefrontUUID(input.VisitorID, maxStorefrontVisitorIDLength) ||
		!validStorefrontUUID(input.SessionID, maxStorefrontSessionIDLength) {
		return nil, ErrInvalidInput
	}
	if !input.EntityType.IsValid() {
		return nil, ErrInvalidInput
	}
	if input.SourceType == "" {
		input.SourceType = models.StorefrontSourceDirect
	}
	if !input.SourceType.IsValid() {
		return nil, ErrInvalidInput
	}
	if input.PageType == "" {
		input.PageType = models.StorefrontPageOther
	}
	if !input.PageType.IsValid() {
		return nil, ErrInvalidInput
	}
	if input.DeviceType == "" {
		input.DeviceType = models.StorefrontDeviceUnknown
	}
	if !input.DeviceType.IsValid() {
		return nil, ErrInvalidInput
	}
	if !validOptionalStorefrontBusinessID(input.EntityID, maxStorefrontEntityIDLength) ||
		!validOptionalStorefrontBusinessID(input.SourceID, maxStorefrontSourceIDLength) {
		return nil, ErrInvalidInput
	}
	if input.ProductID != nil && *input.ProductID == 0 {
		return nil, ErrInvalidInput
	}

	pagePath, err := normalizeStorefrontPagePath(input.PagePath)
	if err != nil {
		return nil, err
	}
	referrer, err := normalizeStorefrontReferrer(input.Referrer)
	if err != nil {
		return nil, err
	}

	event := &models.StorefrontEvent{
		EventID:    strings.ToLower(input.EventID),
		EventName:  input.EventName,
		VisitorID:  input.VisitorID,
		SessionID:  input.SessionID,
		EntityType: input.EntityType,
		EntityID:   optionalStorefrontString(input.EntityID),
		ProductID:  input.ProductID,
		SourceType: input.SourceType,
		SourceID:   optionalStorefrontString(input.SourceID),
		PageType:   input.PageType,
		PagePath:   pagePath,
		Locale:     locale.NormalizeLocale(input.Locale),
		Referrer:   referrer,
		DeviceType: input.DeviceType,
	}
	if event.ProductID != nil && event.EntityType == "" {
		productEntityID := strconv.FormatUint(uint64(*event.ProductID), 10)
		event.EntityType = models.StorefrontEntityProduct
		event.EntityID = &productEntityID
	}

	if event.EventName.IsProductEvent() {
		if event.ProductID == nil || event.SourceID == nil {
			return nil, ErrInvalidInput
		}
		productEntityID := strconv.FormatUint(uint64(*event.ProductID), 10)
		event.EntityType = models.StorefrontEntityProduct
		event.EntityID = &productEntityID
	}
	if event.EventName.IsContentEvent() {
		if event.EntityType != models.StorefrontEntityMarketingContent || event.EntityID == nil ||
			event.SourceType != models.StorefrontSourceMarketingContent || event.SourceID == nil ||
			*event.EntityID != *event.SourceID {
			return nil, ErrInvalidInput
		}
	}
	if event.EntityType == models.StorefrontEntityProduct {
		if event.ProductID == nil || event.EntityID == nil || *event.EntityID != strconv.FormatUint(uint64(*event.ProductID), 10) {
			return nil, ErrInvalidInput
		}
	}
	if event.SourceType == models.StorefrontSourceShop && event.SourceID == nil {
		return nil, ErrInvalidInput
	}

	return event, nil
}

func normalizedStorefrontTrafficType(trafficTypes []models.StorefrontTrafficType) models.StorefrontTrafficType {
	if len(trafficTypes) > 0 && trafficTypes[0].IsValid() {
		return trafficTypes[0]
	}
	return models.StorefrontTrafficHuman
}

func (s *StorefrontEventService) deriveStorefrontEventTenant(ctx context.Context, event *models.StorefrontEvent) (*uint, error) {
	var tenantID *uint
	if event.ProductID != nil {
		resolved, err := s.repo.FindPublicProductTenantID(ctx, *event.ProductID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidInput
		}
		if err != nil {
			return nil, err
		}
		tenantID = uintPointer(resolved)
	}

	shopIdentifier := storefrontShopIdentifier(event)
	if shopIdentifier != "" {
		resolved, err := s.repo.FindPublicShopTenantID(ctx, shopIdentifier)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidInput
		}
		if err != nil {
			return nil, err
		}
		if tenantID != nil && *tenantID != resolved {
			return nil, ErrInvalidInput
		}
		tenantID = uintPointer(resolved)
	}

	if tenantID != nil {
		return tenantID, nil
	}
	return s.repo.FindPlatformMallTenantID(ctx)
}

func storefrontShopIdentifier(event *models.StorefrontEvent) string {
	if event.SourceType == models.StorefrontSourceShop && event.SourceID != nil {
		return *event.SourceID
	}
	if event.EntityType == models.StorefrontEntityShop && event.EntityID != nil {
		return *event.EntityID
	}
	return ""
}

func validStorefrontUUID(value string, maxLength int) bool {
	return value != "" && utf8.RuneCountInString(value) <= maxLength && storefrontUUIDPattern.MatchString(value)
}

func validOptionalStorefrontBusinessID(value string, maxLength int) bool {
	if value == "" {
		return true
	}
	return utf8.RuneCountInString(value) <= maxLength && storefrontBusinessIDPattern.MatchString(value)
}

func normalizeStorefrontPagePath(value string) (string, error) {
	if value == "" || !strings.HasPrefix(value, "/") {
		return "", ErrInvalidInput
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.IsAbs() || parsed.Opaque != "" || parsed.User != nil || parsed.Host != "" {
		return "", ErrInvalidInput
	}
	normalized, err := normalizeStorefrontURLPath(parsed)
	if err != nil || normalized == "" || !strings.HasPrefix(normalized, "/") ||
		utf8.RuneCountInString(normalized) > maxStorefrontPagePathLength {
		return "", ErrInvalidInput
	}
	return normalized, nil
}

func normalizeStorefrontReferrer(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Opaque != "" || parsed.User != nil || parsed.Host == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", ErrInvalidInput
	}
	normalizedPath, err := normalizeStorefrontURLPath(parsed)
	if err != nil {
		return "", ErrInvalidInput
	}
	normalized := parsed.Scheme + "://" + parsed.Host + normalizedPath
	if utf8.RuneCountInString(normalized) > maxStorefrontReferrerLength {
		return "", ErrInvalidInput
	}
	return normalized, nil
}

// normalizeStorefrontURLPath produces the analytics persistence representation,
// not a wire-format URL. Unicode remains readable, while ASCII characters that
// have reserved path meaning stay percent-escaped. Splitting the escaped path
// before PathUnescape preserves segment boundaries, so a%2Fb never becomes a/b.
func normalizeStorefrontURLPath(value *url.URL) (string, error) {
	escapedPath := value.EscapedPath()
	if escapedPath == "" {
		return "", nil
	}
	segments := strings.Split(escapedPath, "/")
	for index, segment := range segments {
		decoded, err := url.PathUnescape(segment)
		if err != nil || !utf8.ValidString(decoded) {
			return "", ErrInvalidInput
		}
		segments[index] = normalizeStorefrontURLPathSegment(decoded)
	}
	return strings.Join(segments, "/"), nil
}

func normalizeStorefrontURLPathSegment(value string) string {
	var normalized strings.Builder
	normalized.Grow(len(value))
	for _, current := range value {
		if current >= utf8.RuneSelf || isStorefrontURLPathCharacter(current) {
			normalized.WriteRune(current)
			continue
		}
		normalized.WriteString(url.PathEscape(string(current)))
	}
	return normalized.String()
}

// RFC 3986 pchar ASCII is safe inside a path segment. Delimiters such as '/',
// '?', '#', and '%' are intentionally absent and therefore remain escaped.
func isStorefrontURLPathCharacter(value rune) bool {
	if value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' {
		return true
	}
	switch value {
	case '-', '.', '_', '~', '!', '$', '&', '\'', '(', ')', '*', '+', ',', ';', '=', ':', '@':
		return true
	default:
		return false
	}
}

func optionalStorefrontString(value string) *string {
	if value == "" {
		return nil
	}
	result := value
	return &result
}

func uintPointer(value uint) *uint {
	result := value
	return &result
}
