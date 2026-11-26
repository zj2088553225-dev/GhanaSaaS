package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"saas/internal/models"

	"gorm.io/gorm"
)

const maxCustomHeadHTMLLength = 10000

var (
	metaPixelIDPattern       = regexp.MustCompile(`^[0-9]{5,32}$`)
	dangerousURLPattern      = regexp.MustCompile(`(?is)\b(?:src|href|action)\s*=\s*(?:"\s*javascript:|'\s*javascript:|javascript:)`)
	dangerousHandlerPattern  = regexp.MustCompile(`(?is)\son[a-z]+\s*=`)
	dangerousHeadTagPattern  = regexp.MustCompile(`(?is)<\s*(?:base|object|embed)\b`)
	dangerousRefreshPattern  = regexp.MustCompile(`(?is)<meta\b[^>]*http-equiv\s*=\s*(?:"refresh"|'refresh'|refresh)`)
	dangerousDocumentPattern = regexp.MustCompile(`(?is)\bdocument\s*\.\s*(?:write|writeln)\s*\(`)
)

type StorefrontTrackingInput struct {
	MetaPixelEnabled  bool   `json:"meta_pixel_enabled"`
	MetaPixelID       string `json:"meta_pixel_id"`
	CustomHeadEnabled bool   `json:"custom_head_enabled"`
	CustomHeadHTML    string `json:"custom_head_html"`
}

type StorefrontTrackingDTO struct {
	MetaPixelEnabled  bool   `json:"meta_pixel_enabled"`
	MetaPixelID       string `json:"meta_pixel_id"`
	CustomHeadEnabled bool   `json:"custom_head_enabled"`
	CustomHeadHTML    string `json:"custom_head_html"`
}

type storefrontTrackingRepository interface {
	Get(ctx context.Context) (*models.StorefrontTrackingSetting, error)
	Save(ctx context.Context, setting models.StorefrontTrackingSetting) (*models.StorefrontTrackingSetting, error)
}

type StorefrontTrackingService struct {
	repo storefrontTrackingRepository
}

func NewStorefrontTrackingService(repo storefrontTrackingRepository) *StorefrontTrackingService {
	return &StorefrontTrackingService{repo: repo}
}

func (s *StorefrontTrackingService) GetAdmin(ctx context.Context) (*StorefrontTrackingDTO, error) {
	setting, err := s.repo.Get(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &StorefrontTrackingDTO{}, nil
	}
	if err != nil {
		return nil, err
	}
	return trackingDTO(setting), nil
}

func (s *StorefrontTrackingService) Save(
	ctx context.Context,
	input StorefrontTrackingInput,
) (*StorefrontTrackingDTO, error) {
	input.MetaPixelID = strings.TrimSpace(input.MetaPixelID)
	input.CustomHeadHTML = strings.TrimSpace(input.CustomHeadHTML)

	if input.MetaPixelID != "" && !metaPixelIDPattern.MatchString(input.MetaPixelID) {
		return nil, ErrInvalidInput
	}
	if input.MetaPixelEnabled && input.MetaPixelID == "" {
		return nil, ErrInvalidInput
	}
	if utf8.RuneCountInString(input.CustomHeadHTML) > maxCustomHeadHTMLLength {
		return nil, ErrInvalidInput
	}
	if containsDangerousHeadCode(input.CustomHeadHTML) {
		return nil, ErrInvalidInput
	}

	setting, err := s.repo.Save(ctx, models.StorefrontTrackingSetting{
		MetaPixelEnabled:  input.MetaPixelEnabled,
		MetaPixelID:       input.MetaPixelID,
		CustomHeadEnabled: input.CustomHeadEnabled,
		CustomHeadHTML:    input.CustomHeadHTML,
	})
	if err != nil {
		return nil, err
	}
	return trackingDTO(setting), nil
}

func (s *StorefrontTrackingService) GetPublic(ctx context.Context) (*StorefrontTrackingDTO, error) {
	setting, err := s.repo.Get(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &StorefrontTrackingDTO{}, nil
	}
	if err != nil {
		return nil, err
	}

	result := &StorefrontTrackingDTO{}
	if setting.MetaPixelEnabled && metaPixelIDPattern.MatchString(setting.MetaPixelID) {
		result.MetaPixelEnabled = true
		result.MetaPixelID = setting.MetaPixelID
	}
	if setting.CustomHeadEnabled && setting.CustomHeadHTML != "" {
		result.CustomHeadEnabled = true
		result.CustomHeadHTML = setting.CustomHeadHTML
	}
	return result, nil
}

func containsDangerousHeadCode(value string) bool {
	if value == "" {
		return false
	}
	return dangerousURLPattern.MatchString(value) ||
		dangerousHandlerPattern.MatchString(value) ||
		dangerousHeadTagPattern.MatchString(value) ||
		dangerousRefreshPattern.MatchString(value) ||
		dangerousDocumentPattern.MatchString(value)
}

func trackingDTO(setting *models.StorefrontTrackingSetting) *StorefrontTrackingDTO {
	return &StorefrontTrackingDTO{
		MetaPixelEnabled:  setting.MetaPixelEnabled,
		MetaPixelID:       setting.MetaPixelID,
		CustomHeadEnabled: setting.CustomHeadEnabled,
		CustomHeadHTML:    setting.CustomHeadHTML,
	}
}
