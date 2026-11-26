package service

import "saas/internal/models"

type ProductTranslationInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	DetailHTML  string `json:"detail_html"`
	Material    string `json:"material"`
	Dimensions  string `json:"dimensions"`
	Weight      string `json:"weight"`
}

type ShopProfileTranslationInput struct {
	ShopName      string `json:"shop_name"`
	Description   string `json:"description"`
	Address       string `json:"address"`
	BusinessHours string `json:"business_hours"`
}

type PlatformCategoryTranslationInput struct {
	Name string `json:"name"`
}

func productTranslationValues(input ProductTranslationInput) map[string]TranslationValueInput {
	return map[string]TranslationValueInput{
		TranslationFieldName:        {ValueText: input.Name},
		TranslationFieldDescription: {ValueText: input.Description},
		TranslationFieldDetailHTML:  {ValueHTML: sanitizeProductDetailHTML(input.DetailHTML)},
		TranslationFieldMaterial:    {ValueText: input.Material},
		TranslationFieldDimensions:  {ValueText: input.Dimensions},
		TranslationFieldWeight:      {ValueText: input.Weight},
	}
}

func productTranslationInputFromRows(rows map[string]models.ContentTranslation) ProductTranslationInput {
	return ProductTranslationInput{
		Name:        rows[TranslationFieldName].ValueText,
		Description: rows[TranslationFieldDescription].ValueText,
		DetailHTML:  rows[TranslationFieldDetailHTML].ValueHTML,
		Material:    rows[TranslationFieldMaterial].ValueText,
		Dimensions:  rows[TranslationFieldDimensions].ValueText,
		Weight:      rows[TranslationFieldWeight].ValueText,
	}
}

func shopProfileTranslationValues(input ShopProfileTranslationInput) map[string]TranslationValueInput {
	return map[string]TranslationValueInput{
		TranslationFieldShopName:      {ValueText: input.ShopName},
		TranslationFieldDescription:   {ValueText: input.Description},
		TranslationFieldAddress:       {ValueText: input.Address},
		TranslationFieldBusinessHours: {ValueText: input.BusinessHours},
	}
}

func shopProfileTranslationInputFromRows(rows map[string]models.ContentTranslation) ShopProfileTranslationInput {
	return ShopProfileTranslationInput{
		ShopName:      rows[TranslationFieldShopName].ValueText,
		Description:   rows[TranslationFieldDescription].ValueText,
		Address:       rows[TranslationFieldAddress].ValueText,
		BusinessHours: rows[TranslationFieldBusinessHours].ValueText,
	}
}

func platformCategoryTranslationValues(input PlatformCategoryTranslationInput) map[string]TranslationValueInput {
	return map[string]TranslationValueInput{
		TranslationFieldName: {ValueText: input.Name},
	}
}

func platformCategoryTranslationInputFromRows(rows map[string]models.ContentTranslation) PlatformCategoryTranslationInput {
	return PlatformCategoryTranslationInput{
		Name: rows[TranslationFieldName].ValueText,
	}
}
