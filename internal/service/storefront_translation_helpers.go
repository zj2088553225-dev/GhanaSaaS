package service

import (
	"context"
	"strings"

	"saas/internal/models"
)

func (s *StorefrontService) productTranslations(
	ctx context.Context,
	products []models.SPU,
	requestedLocale string,
) (map[uint]map[string]models.ContentTranslation, error) {
	if s.translations == nil || len(products) == 0 {
		return map[uint]map[string]models.ContentTranslation{}, nil
	}
	ids := make([]uint, 0, len(products))
	for _, product := range products {
		ids = append(ids, product.ID)
	}
	return s.translations.GetTranslations(ctx, TranslationEntityProduct, ids, requestedLocale)
}

func (s *StorefrontService) shopProfileTranslations(
	ctx context.Context,
	profiles []models.ShopProfile,
	requestedLocale string,
) (map[uint]map[string]models.ContentTranslation, error) {
	if s.translations == nil || len(profiles) == 0 {
		return map[uint]map[string]models.ContentTranslation{}, nil
	}
	ids := make([]uint, 0, len(profiles))
	for _, profile := range profiles {
		ids = append(ids, profile.ID)
	}
	return s.translations.GetTranslations(ctx, TranslationEntityShopProfile, ids, requestedLocale)
}

func (s *StorefrontService) applyProductDetailTranslations(
	ctx context.Context,
	dto *ProductDetailDTO,
	productID uint,
	requestedLocale string,
) error {
	if s.translations == nil || dto == nil || productID == 0 {
		return nil
	}
	translations, err := s.translations.GetTranslations(ctx, TranslationEntityProduct, []uint{productID}, requestedLocale)
	if err != nil {
		return err
	}
	applyProductDetailTranslation(dto, translations[productID])
	return nil
}

func (s *StorefrontService) applyShopProfileTranslations(
	ctx context.Context,
	profile *models.ShopProfile,
	requestedLocale string,
) error {
	if s.translations == nil || profile == nil || profile.ID == 0 {
		return nil
	}
	translations, err := s.translations.GetTranslations(ctx, TranslationEntityShopProfile, []uint{profile.ID}, requestedLocale)
	if err != nil {
		return err
	}
	applyShopProfileTranslation(profile, translations[profile.ID])
	return nil
}

func (s *StorefrontService) applyCategoryTranslations(
	ctx context.Context,
	categories []models.Category,
	requestedLocale string,
) error {
	if s.translations == nil || len(categories) == 0 {
		return nil
	}
	ids := make([]uint, 0, len(categories))
	for _, category := range categories {
		ids = append(ids, category.ID)
	}
	translations, err := s.translations.GetTranslations(ctx, TranslationEntityCategory, ids, requestedLocale)
	if err != nil {
		return err
	}
	for index := range categories {
		categories[index].Name = translatedText(categories[index].Name, translations[categories[index].ID], TranslationFieldName)
	}
	return nil
}

func (s *StorefrontService) localizedPlatformCategory(
	ctx context.Context,
	category models.PlatformCategory,
	requestedLocale string,
) (*models.PlatformCategory, error) {
	if s.translations == nil || category.ID == 0 {
		return &category, nil
	}

	categories, err := s.repo.ListPlatformCategories(ctx, true)
	if err != nil {
		return nil, err
	}
	found := false
	for _, item := range categories {
		if item.ID == category.ID {
			found = true
			break
		}
	}
	if !found {
		categories = append(categories, category)
	}

	ids := make([]uint, 0, len(categories))
	for _, item := range categories {
		ids = append(ids, item.ID)
	}
	translations, err := s.translations.GetTranslations(ctx, TranslationEntityPlatformCategory, ids, requestedLocale)
	if err != nil {
		return nil, err
	}
	localized := localizePlatformCategories(categories, translations)
	for index := range localized {
		if localized[index].ID == category.ID {
			return &localized[index], nil
		}
	}
	return &category, nil
}

func applyProductListTranslations(dto *ProductListDTO, translations map[string]models.ContentTranslation) {
	if dto == nil {
		return
	}
	dto.Name = translatedText(dto.Name, translations, TranslationFieldName)
	dto.Description = translatedText(dto.Description, translations, TranslationFieldDescription)
}

func applyProductDetailTranslation(dto *ProductDetailDTO, translations map[string]models.ContentTranslation) {
	if dto == nil {
		return
	}
	dto.Name = translatedText(dto.Name, translations, TranslationFieldName)
	dto.Description = translatedText(dto.Description, translations, TranslationFieldDescription)
	dto.DetailHTML = translatedHTML(dto.DetailHTML, translations, TranslationFieldDetailHTML)
	dto.Material = translatedText(dto.Material, translations, TranslationFieldMaterial)
	dto.Dimensions = translatedText(dto.Dimensions, translations, TranslationFieldDimensions)
	dto.Weight = translatedText(dto.Weight, translations, TranslationFieldWeight)
}

func applyShopProfileTranslation(profile *models.ShopProfile, translations map[string]models.ContentTranslation) {
	if profile == nil {
		return
	}
	profile.ShopName = translatedText(profile.ShopName, translations, TranslationFieldShopName)
	profile.Description = translatedText(profile.Description, translations, TranslationFieldDescription)
	profile.Address = translatedText(profile.Address, translations, TranslationFieldAddress)
	profile.BusinessHours = translatedText(profile.BusinessHours, translations, TranslationFieldBusinessHours)
}

func localizePlatformCategories(
	categories []models.PlatformCategory,
	translations map[uint]map[string]models.ContentTranslation,
) []models.PlatformCategory {
	localized := make([]models.PlatformCategory, len(categories))
	copy(localized, categories)
	byID := make(map[uint]*models.PlatformCategory, len(localized))
	for index := range localized {
		localized[index].Name = translatedText(localized[index].Name, translations[localized[index].ID], TranslationFieldName)
		byID[localized[index].ID] = &localized[index]
	}

	pathMemo := make(map[uint]string, len(localized))
	var buildPath func(id uint, visiting map[uint]struct{}) string
	buildPath = func(id uint, visiting map[uint]struct{}) string {
		if path, ok := pathMemo[id]; ok {
			return path
		}
		category, ok := byID[id]
		if !ok {
			return ""
		}
		if _, ok := visiting[id]; ok {
			return strings.TrimSpace(category.Name)
		}
		visiting[id] = struct{}{}
		name := strings.TrimSpace(category.Name)
		if name == "" {
			name = strings.TrimSpace(category.Path)
		}
		path := name
		if category.ParentID != nil {
			parentPath := buildPath(*category.ParentID, visiting)
			if parentPath != "" && name != "" {
				path = parentPath + "/" + name
			} else if parentPath != "" {
				path = parentPath
			}
		}
		delete(visiting, id)
		pathMemo[id] = path
		return path
	}

	for index := range localized {
		path := buildPath(localized[index].ID, map[uint]struct{}{})
		if path != "" {
			localized[index].Path = path
		}
	}
	return localized
}

func translatedText(original string, translations map[string]models.ContentTranslation, fieldKey string) string {
	return ApplyTextTranslation(original, TranslationField(translations, fieldKey))
}

func translatedHTML(original string, translations map[string]models.ContentTranslation, fieldKey string) string {
	return ApplyHTMLTranslation(original, TranslationField(translations, fieldKey))
}
