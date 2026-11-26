package service

import (
	"context"
	"strings"

	"saas/internal/models"
	"saas/internal/pkg/locale"

	"gorm.io/datatypes"
)

type contentTranslationRepository interface {
	Upsert(ctx context.Context, translation *models.ContentTranslation) (*models.ContentTranslation, error)
	BatchUpsert(ctx context.Context, translations []models.ContentTranslation) error
	Delete(ctx context.Context, entityType string, entityID uint, fieldKey string, requestedLocale string) error
	GetEntityTranslations(ctx context.Context, entityType string, entityID uint, requestedLocale string) (map[string]models.ContentTranslation, error)
	GetTranslation(ctx context.Context, entityType string, entityID uint, fieldKey string, requestedLocale string) (*models.ContentTranslation, bool, error)
	GetTranslations(ctx context.Context, entityType string, entityIDs []uint, requestedLocale string) (map[uint]map[string]models.ContentTranslation, error)
}

type TranslationReader interface {
	GetTranslations(ctx context.Context, entityType string, entityIDs []uint, requestedLocale string) (map[uint]map[string]models.ContentTranslation, error)
}

type TranslationEditor interface {
	GetTranslationsForEntity(ctx context.Context, entityType string, entityID uint, requestedLocale string) (map[string]models.ContentTranslation, error)
	SaveTranslationsForEntity(ctx context.Context, entityType string, entityID uint, requestedLocale string, values map[string]TranslationValueInput) error
}

type TranslationBulkEditor interface {
	SaveTextTranslationsForEntities(ctx context.Context, entityType string, fieldKey string, requestedLocale string, values map[uint]string) error
}

type TranslationValueInput struct {
	ValueText string
	ValueHTML string
	ValueJSON datatypes.JSON
}

const (
	TranslationEntityProduct          = "product"
	TranslationEntityShopProfile      = "shop_profile"
	TranslationEntityCategory         = "category"
	TranslationEntityPlatformCategory = "platform_category"
	TranslationEntitySitePage         = "site_page"

	TranslationFieldName             = "name"
	TranslationFieldDescription      = "description"
	TranslationFieldDetailHTML       = "detail_html"
	TranslationFieldMaterial         = "material"
	TranslationFieldDimensions       = "dimensions"
	TranslationFieldWeight           = "weight"
	TranslationFieldShopName         = "shop_name"
	TranslationFieldAddress          = "address"
	TranslationFieldBusinessHours    = "business_hours"
	TranslationFieldDraftContent     = "draft_content"
	TranslationFieldPublishedContent = "published_content"
)

type translationFieldKind string

const (
	translationFieldText translationFieldKind = "text"
	translationFieldHTML translationFieldKind = "html"
	translationFieldJSON translationFieldKind = "json"
)

var allowedTranslationFields = map[string]map[string]translationFieldKind{
	TranslationEntityProduct: {
		TranslationFieldName:        translationFieldText,
		TranslationFieldDescription: translationFieldText,
		TranslationFieldDetailHTML:  translationFieldHTML,
		TranslationFieldMaterial:    translationFieldText,
		TranslationFieldDimensions:  translationFieldText,
		TranslationFieldWeight:      translationFieldText,
	},
	TranslationEntityShopProfile: {
		TranslationFieldShopName:      translationFieldText,
		TranslationFieldDescription:   translationFieldText,
		TranslationFieldAddress:       translationFieldText,
		TranslationFieldBusinessHours: translationFieldText,
	},
	TranslationEntityPlatformCategory: {
		TranslationFieldName: translationFieldText,
	},
	TranslationEntitySitePage: {
		TranslationFieldDraftContent:     translationFieldJSON,
		TranslationFieldPublishedContent: translationFieldJSON,
	},
}

type ContentTranslationService struct {
	repo contentTranslationRepository
}

func NewContentTranslationService(repo contentTranslationRepository) *ContentTranslationService {
	return &ContentTranslationService{repo: repo}
}

func (s *ContentTranslationService) GetTranslation(ctx context.Context, entityType string, entityID uint, fieldKey string, requestedLocale string) (*models.ContentTranslation, bool, error) {
	return s.repo.GetTranslation(ctx, entityType, entityID, fieldKey, requestedLocale)
}

func (s *ContentTranslationService) GetTranslations(ctx context.Context, entityType string, entityIDs []uint, requestedLocale string) (map[uint]map[string]models.ContentTranslation, error) {
	return s.repo.GetTranslations(ctx, entityType, entityIDs, requestedLocale)
}

func (s *ContentTranslationService) GetTranslationsForEntity(ctx context.Context, entityType string, entityID uint, requestedLocale string) (map[string]models.ContentTranslation, error) {
	if _, ok := allowedTranslationFields[entityType]; !ok {
		return nil, ErrInvalidInput
	}
	if entityID == 0 || !locale.IsSupportedLocale(requestedLocale) {
		return nil, ErrInvalidInput
	}
	return s.repo.GetEntityTranslations(ctx, entityType, entityID, requestedLocale)
}

func (s *ContentTranslationService) SaveTranslationsForEntity(ctx context.Context, entityType string, entityID uint, requestedLocale string, values map[string]TranslationValueInput) error {
	fields, ok := allowedTranslationFields[entityType]
	if !ok || entityID == 0 || !locale.IsSupportedLocale(requestedLocale) {
		return ErrInvalidInput
	}

	for fieldKey, input := range values {
		kind, ok := fields[fieldKey]
		if !ok {
			return ErrInvalidInput
		}
		translation := models.ContentTranslation{
			EntityType: entityType,
			EntityID:   entityID,
			FieldKey:   fieldKey,
			Locale:     requestedLocale,
			Status:     models.ContentTranslationStatusPublished,
		}
		empty := true
		switch kind {
		case translationFieldText:
			value := strings.TrimSpace(input.ValueText)
			translation.ValueText = value
			empty = value == ""
		case translationFieldHTML:
			value := strings.TrimSpace(input.ValueHTML)
			translation.ValueHTML = value
			empty = value == ""
		case translationFieldJSON:
			value := strings.TrimSpace(string(input.ValueJSON))
			translation.ValueJSON = input.ValueJSON
			empty = value == "" || value == "null"
		default:
			return ErrInvalidInput
		}

		if empty {
			if err := s.repo.Delete(ctx, entityType, entityID, fieldKey, requestedLocale); err != nil {
				return err
			}
			continue
		}
		if _, err := s.repo.Upsert(ctx, &translation); err != nil {
			return err
		}
	}
	return nil
}

func (s *ContentTranslationService) SaveTextTranslationsForEntities(ctx context.Context, entityType string, fieldKey string, requestedLocale string, values map[uint]string) error {
	fields, ok := allowedTranslationFields[entityType]
	if !ok || !locale.IsSupportedLocale(requestedLocale) || len(values) == 0 {
		return ErrInvalidInput
	}
	if fields[fieldKey] != translationFieldText {
		return ErrInvalidInput
	}

	rows := make([]models.ContentTranslation, 0, len(values))
	for entityID, value := range values {
		value = strings.TrimSpace(value)
		if entityID == 0 || value == "" {
			continue
		}
		rows = append(rows, models.ContentTranslation{
			EntityType: entityType,
			EntityID:   entityID,
			FieldKey:   fieldKey,
			Locale:     requestedLocale,
			ValueText:  value,
			Status:     models.ContentTranslationStatusPublished,
		})
	}
	if len(rows) == 0 {
		return nil
	}
	return s.repo.BatchUpsert(ctx, rows)
}

func ApplyTextTranslation(original string, translation *models.ContentTranslation) string {
	if translation == nil {
		return original
	}
	value := strings.TrimSpace(translation.ValueText)
	if value == "" {
		return original
	}
	return value
}

func ApplyHTMLTranslation(original string, translation *models.ContentTranslation) string {
	if translation == nil {
		return original
	}
	value := strings.TrimSpace(translation.ValueHTML)
	if value == "" {
		return original
	}
	return value
}

func ApplyJSONTranslation(original datatypes.JSON, translation *models.ContentTranslation) datatypes.JSON {
	if translation == nil || len(translation.ValueJSON) == 0 {
		return original
	}
	value := strings.TrimSpace(string(translation.ValueJSON))
	if value == "" || value == "null" {
		return original
	}
	return translation.ValueJSON
}

func TranslationField(translations map[string]models.ContentTranslation, fieldKey string) *models.ContentTranslation {
	if len(translations) == 0 {
		return nil
	}
	translation, ok := translations[fieldKey]
	if !ok {
		return nil
	}
	return &translation
}
