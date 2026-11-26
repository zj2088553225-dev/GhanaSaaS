package service

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"saas/internal/models"
	"saas/internal/repository"

	"gorm.io/gorm"
)

const (
	maxShopSlugLength          = 80
	maxShopNameLength          = 120
	maxShopURLLength           = 255
	maxShopDescriptionLength   = 2000
	maxShopPhoneLength         = 64
	maxShopWhatsappLength      = 64
	maxShopAddressLength       = 255
	maxShopBusinessHoursLength = 255
)

var shopSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type ShopProfileInput struct {
	Slug          string
	ShopName      string
	LogoURL       string
	BannerURL     string
	Description   string
	Phone         string
	Whatsapp      string
	Address       string
	BusinessHours string
	IsPublic      bool
	Translations  *ShopProfileTranslationInput
}

type ShopProfileDTO struct {
	ID            uint   `json:"id,omitempty"`
	TenantID      uint   `json:"tenant_id,omitempty"`
	Slug          string `json:"slug"`
	ShopName      string `json:"shop_name"`
	LogoURL       string `json:"logo_url"`
	BannerURL     string `json:"banner_url"`
	Description   string `json:"description"`
	Phone         string `json:"phone"`
	Whatsapp      string `json:"whatsapp"`
	Address       string `json:"address"`
	BusinessHours string `json:"business_hours"`
	IsPublic      bool   `json:"is_public"`
	Translations  struct {
		Zh ShopProfileTranslationInput `json:"zh"`
	} `json:"translations"`
}

type shopProfileRepository interface {
	FindByTenant(ctx context.Context, tenantID uint) (*models.ShopProfile, error)
	SaveAtomic(ctx context.Context, tenantID uint, profile *models.ShopProfile, translation repository.ShopProfileTranslationMutation) (*models.ShopProfile, error)
}

type ShopProfileService struct {
	repo         shopProfileRepository
	translations TranslationEditor
}

type ShopProfileOptions struct {
	Translations TranslationEditor
}

func NewShopProfileService(repo shopProfileRepository, options ...ShopProfileOptions) *ShopProfileService {
	s := &ShopProfileService{repo: repo}
	if len(options) > 0 {
		s.translations = options[0].Translations
	}
	return s
}

func (s *ShopProfileService) GetForTenant(ctx context.Context, tenantID uint) (*ShopProfileDTO, error) {
	if tenantID == 0 {
		return nil, ErrForbidden
	}

	profile, err := s.repo.FindByTenant(ctx, tenantID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &ShopProfileDTO{TenantID: tenantID}, nil
	}
	if err != nil {
		return nil, err
	}

	dto := toShopProfileDTO(profile, true)
	if err := s.applyShopProfileTranslationFields(ctx, &dto, profile.ID); err != nil {
		return nil, err
	}
	return &dto, nil
}

func (s *ShopProfileService) UpdateForTenant(ctx context.Context, tenantID uint, input ShopProfileInput) (*ShopProfileDTO, error) {
	if tenantID == 0 {
		return nil, ErrForbidden
	}

	profile, err := buildShopProfile(input)
	if err != nil {
		return nil, err
	}

	saved, err := s.repo.SaveAtomic(ctx, tenantID, profile, shopProfileTranslationMutation(input.Translations))
	if errors.Is(err, repository.ErrDuplicateShopProfileSlug) {
		return nil, ErrConflict
	}
	if errors.Is(err, repository.ErrShopProfileTenantUnavailable) {
		return nil, ErrForbidden
	}
	if err != nil {
		return nil, err
	}

	dto := toShopProfileDTO(saved, true)
	if err := s.applyShopProfileTranslationFields(ctx, &dto, saved.ID); err != nil {
		return nil, err
	}
	return &dto, nil
}

func (s *ShopProfileService) applyShopProfileTranslationFields(ctx context.Context, dto *ShopProfileDTO, profileID uint) error {
	if s.translations == nil || dto == nil || profileID == 0 {
		return nil
	}
	rows, err := s.translations.GetTranslationsForEntity(ctx, TranslationEntityShopProfile, profileID, "zh")
	if err != nil {
		return err
	}
	dto.Translations.Zh = shopProfileTranslationInputFromRows(rows)
	return nil
}

func buildShopProfile(input ShopProfileInput) (*models.ShopProfile, error) {
	slug := strings.TrimSpace(input.Slug)
	shopName := strings.TrimSpace(input.ShopName)
	logoURL := strings.TrimSpace(input.LogoURL)
	bannerURL := strings.TrimSpace(input.BannerURL)
	description := strings.TrimSpace(input.Description)
	phone := strings.TrimSpace(input.Phone)
	whatsapp := strings.TrimSpace(input.Whatsapp)
	address := strings.TrimSpace(input.Address)
	businessHours := strings.TrimSpace(input.BusinessHours)

	if !isValidShopSlug(slug) || shopName == "" {
		return nil, ErrInvalidInput
	}
	if len(slug) > maxShopSlugLength ||
		len(shopName) > maxShopNameLength ||
		len(logoURL) > maxShopURLLength ||
		len(bannerURL) > maxShopURLLength ||
		len(description) > maxShopDescriptionLength ||
		len(phone) > maxShopPhoneLength ||
		len(whatsapp) > maxShopWhatsappLength ||
		len(address) > maxShopAddressLength ||
		len(businessHours) > maxShopBusinessHoursLength {
		return nil, ErrInvalidInput
	}

	return &models.ShopProfile{
		Slug:          slug,
		ShopName:      shopName,
		LogoURL:       logoURL,
		BannerURL:     bannerURL,
		Description:   description,
		Phone:         phone,
		Whatsapp:      whatsapp,
		Address:       address,
		BusinessHours: businessHours,
		IsPublic:      input.IsPublic,
	}, nil
}

func shopProfileTranslationMutation(input *ShopProfileTranslationInput) repository.ShopProfileTranslationMutation {
	if input == nil {
		return repository.ShopProfileTranslationMutation{}
	}
	return repository.ShopProfileTranslationMutation{
		Provided:      true,
		ShopName:      strings.TrimSpace(input.ShopName),
		Description:   strings.TrimSpace(input.Description),
		Address:       strings.TrimSpace(input.Address),
		BusinessHours: strings.TrimSpace(input.BusinessHours),
	}
}

func isValidShopSlug(slug string) bool {
	return slug != "" && shopSlugPattern.MatchString(slug)
}

func toShopProfileDTO(profile *models.ShopProfile, includeAdminFields bool) ShopProfileDTO {
	if profile == nil {
		return ShopProfileDTO{}
	}

	dto := ShopProfileDTO{
		ID:            profile.ID,
		Slug:          profile.Slug,
		ShopName:      profile.ShopName,
		LogoURL:       profile.LogoURL,
		BannerURL:     profile.BannerURL,
		Description:   profile.Description,
		Phone:         profile.Phone,
		Whatsapp:      profile.Whatsapp,
		Address:       profile.Address,
		BusinessHours: profile.BusinessHours,
	}
	if includeAdminFields {
		dto.TenantID = profile.TenantID
		dto.IsPublic = profile.IsPublic
	}
	return dto
}
