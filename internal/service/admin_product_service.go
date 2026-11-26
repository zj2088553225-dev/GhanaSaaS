package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"saas/internal/models"
	"saas/internal/repository"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	maxAdminProductPageSize        = 100
	maxProductDetailHTMLLength     = 2_000_000
	maxGeneratedSKUCodeAttempts    = 8
	generatedSKUCodeRandomBytes    = 3
	generatedSKUCodeSequenceFormat = "%04d"
	canonicalDefaultSKUName        = "Default SKU"
)

const AdminProductHasOrderRecordsMessageEN = "This product has order records and cannot be deleted. Please take it off shelf or archive it."

type AdminSKUInput struct {
	ID            uint
	Name          string
	ERPSKUCode    string
	ImageURL      string
	Specs         *datatypes.JSON
	RetailPrice   int64
	OriginalPrice int64
	SellableStock int64
}

type AdminProductInput struct {
	PlatformCategoryID uint
	CategoryID         *uint
	Name               string
	Description        string
	DetailHTML         string
	MainImage          string
	VideoURL           string
	Material           string
	Dimensions         string
	Weight             string
	SliderImages       *datatypes.JSON
	SpecTemplate       *datatypes.JSON
	Status             models.SPUStatus
	SKUs               []AdminSKUInput
	Translations       *ProductTranslationInput
}

type AdminProductQuery struct {
	CategoryID         *uint
	PlatformCategoryID *uint
	Keyword            string
	Status             *models.SPUStatus
	Page               int
	PageSize           int
}

type AdminProductPage struct {
	Items    []models.SPU
	Total    int64
	Page     int
	PageSize int
}

type adminProductRepository interface {
	CreateAtomic(ctx context.Context, tenantID uint, product *models.SPU, translation repository.ProductTranslationMutation, generateCodes repository.SKUCodeGenerator) error
	CreateManyAtomic(ctx context.Context, tenantID uint, products []models.SPU, generateCodes repository.SKUCodeGenerator) error
	FindByIDWithTenant(ctx context.Context, tenantID uint, id uint) (*models.SPU, error)
	ListByTenant(ctx context.Context, query repository.AdminProductQuery) ([]models.SPU, int64, error)
	FindPlatformCategoryByID(ctx context.Context, id uint) (*models.PlatformCategory, error)
	FindPlatformCategoriesByPaths(ctx context.Context, paths []string) (map[string]models.PlatformCategory, error)
	HasPlatformCategoryChildren(ctx context.Context, id uint) (bool, error)
	UpdateAtomic(ctx context.Context, tenantID uint, product *models.SPU, translation repository.ProductTranslationMutation, generateCodes repository.SKUCodeGenerator) error
	UpdateStatusWithTenant(ctx context.Context, tenantID uint, id uint, status models.SPUStatus) error
	DeleteWithTenant(ctx context.Context, tenantID uint, id uint) error
}

type AdminProductService struct {
	repo         adminProductRepository
	translations TranslationEditor
}

type AdminProductOptions struct {
	Translations TranslationEditor
}

func NewAdminProductService(repo adminProductRepository, options ...AdminProductOptions) *AdminProductService {
	s := &AdminProductService{repo: repo}
	if len(options) > 0 {
		s.translations = options[0].Translations
	}
	return s
}

func (s *AdminProductService) Create(ctx context.Context, tenantID uint, input AdminProductInput) (*models.SPU, error) {
	if tenantID == 0 {
		return nil, ErrForbidden
	}

	product, err := buildProductFromInput(tenantID, 0, input, models.SPUStatusOffShelf, nil, false)
	if err != nil {
		return nil, err
	}
	if err := s.createProductWithUniqueSKUCodes(ctx, tenantID, product, productTranslationMutation(input.Translations)); err != nil {
		return nil, err
	}

	return s.repo.FindByIDWithTenant(ctx, tenantID, product.ID)
}

func (s *AdminProductService) FindByID(ctx context.Context, tenantID uint, id uint) (*models.SPU, error) {
	if tenantID == 0 || id == 0 {
		return nil, ErrInvalidInput
	}

	product, err := s.repo.FindByIDWithTenant(ctx, tenantID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return product, err
}

func (s *AdminProductService) TranslationFields(ctx context.Context, tenantID uint, id uint, requestedLocale string) (ProductTranslationInput, error) {
	if s.translations == nil {
		return ProductTranslationInput{}, nil
	}
	if _, err := s.FindByID(ctx, tenantID, id); err != nil {
		return ProductTranslationInput{}, err
	}
	rows, err := s.translations.GetTranslationsForEntity(ctx, TranslationEntityProduct, id, requestedLocale)
	if err != nil {
		return ProductTranslationInput{}, err
	}
	return productTranslationInputFromRows(rows), nil
}

func (s *AdminProductService) List(ctx context.Context, tenantID uint, query AdminProductQuery) (*AdminProductPage, error) {
	if tenantID == 0 {
		return nil, ErrForbidden
	}
	query.Page, query.PageSize = normalizePagination(query.Page, query.PageSize)
	if query.Status != nil && !isValidSPUStatus(*query.Status) {
		return nil, ErrInvalidInput
	}

	products, total, err := s.repo.ListByTenant(ctx, repository.AdminProductQuery{
		TenantID:           tenantID,
		CategoryID:         query.CategoryID,
		PlatformCategoryID: query.PlatformCategoryID,
		Keyword:            query.Keyword,
		Status:             query.Status,
		Page:               query.Page,
		PageSize:           query.PageSize,
	})
	if err != nil {
		return nil, err
	}

	return &AdminProductPage{
		Items:    products,
		Total:    total,
		Page:     query.Page,
		PageSize: query.PageSize,
	}, nil
}

func (s *AdminProductService) Update(ctx context.Context, tenantID uint, id uint, input AdminProductInput) (*models.SPU, error) {
	if tenantID == 0 || id == 0 {
		return nil, ErrInvalidInput
	}

	existing, err := s.repo.FindByIDWithTenant(ctx, tenantID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	product, err := buildProductFromInput(tenantID, id, input, existing.Status, existing.SliderImages, true)
	if err != nil {
		return nil, err
	}
	newSKUs := newProductSKUs(product)
	if err := s.updateProductWithUniqueSKUCodes(ctx, tenantID, product, newSKUs, productTranslationMutation(input.Translations)); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return s.repo.FindByIDWithTenant(ctx, tenantID, id)
}

func (s *AdminProductService) UpdateStatus(ctx context.Context, tenantID uint, id uint, status models.SPUStatus) (*models.SPU, error) {
	if tenantID == 0 || id == 0 {
		return nil, ErrInvalidInput
	}
	if !isValidSPUStatus(status) {
		return nil, ErrInvalidInput
	}

	if err := s.repo.UpdateStatusWithTenant(ctx, tenantID, id, status); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return s.repo.FindByIDWithTenant(ctx, tenantID, id)
}

func (s *AdminProductService) Delete(ctx context.Context, tenantID uint, id uint) error {
	if tenantID == 0 || id == 0 {
		return ErrInvalidInput
	}

	if err := s.repo.DeleteWithTenant(ctx, tenantID, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		if errors.Is(err, repository.ErrProductHasOrderRecords) {
			return ErrAdminProductHasOrderRecords
		}
		return err
	}
	return nil
}

func buildProductFromInput(tenantID uint, id uint, input AdminProductInput, defaultStatus models.SPUStatus, defaultSliderImages datatypes.JSON, preserveSKUIDs bool) (*models.SPU, error) {
	name := strings.TrimSpace(input.Name)
	if tenantID == 0 || input.PlatformCategoryID == 0 || name == "" {
		return nil, ErrInvalidInput
	}
	categoryID := normalizeOptionalUint(input.CategoryID)

	status := input.Status
	if status == "" {
		status = defaultStatus
	}
	if !isValidSPUStatus(status) {
		return nil, ErrInvalidInput
	}

	sliderImages, err := normalizeSliderImages(input.SliderImages, defaultSliderImages)
	if err != nil {
		return nil, err
	}
	specTemplate, err := normalizeJSON(input.SpecTemplate, nil)
	if err != nil {
		return nil, err
	}

	skus, err := normalizeSKUInputs(input.SKUs, preserveSKUIDs)
	if err != nil {
		return nil, err
	}
	if len(input.DetailHTML) > maxProductDetailHTMLLength {
		return nil, ErrInvalidInput
	}
	if err := validateProductVideoURL(input.VideoURL); err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(input.Material)) > 255 ||
		len(strings.TrimSpace(input.Dimensions)) > 255 ||
		len(strings.TrimSpace(input.Weight)) > 255 {
		return nil, ErrInvalidInput
	}
	detailHTML := sanitizeProductDetailHTML(input.DetailHTML)

	return &models.SPU{
		BaseModel:          models.BaseModel{ID: id},
		TenantID:           tenantID,
		PlatformCategoryID: input.PlatformCategoryID,
		CategoryID:         categoryID,
		Name:               name,
		Description:        strings.TrimSpace(input.Description),
		DetailHTML:         detailHTML,
		MainImage:          strings.TrimSpace(input.MainImage),
		VideoURL:           strings.TrimSpace(input.VideoURL),
		Material:           strings.TrimSpace(input.Material),
		Dimensions:         strings.TrimSpace(input.Dimensions),
		Weight:             strings.TrimSpace(input.Weight),
		SliderImages:       sliderImages,
		SpecTemplate:       specTemplate,
		Status:             status,
		SKUs:               skus,
	}, nil
}

func validateProductVideoURL(value string) error {
	value = strings.TrimSpace(value)
	if len(value) > 512 {
		return ErrInvalidInput
	}
	if value == "" || strings.HasPrefix(value, "/uploads/videos/") ||
		strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "http://") {
		return nil
	}
	return ErrInvalidInput
}

func normalizeOptionalUint(value *uint) *uint {
	if value == nil || *value == 0 {
		return nil
	}
	return cloneUintPtr(value)
}

func normalizeSKUInputs(inputs []AdminSKUInput, preserveIDs bool) ([]models.SKU, error) {
	if len(inputs) == 0 {
		return nil, ErrInvalidInput
	}

	skus := make([]models.SKU, 0, len(inputs))
	for _, input := range inputs {
		name := strings.TrimSpace(input.Name)
		erpCode := strings.TrimSpace(input.ERPSKUCode)
		imageURL := strings.TrimSpace(input.ImageURL)
		if len(imageURL) > 512 || input.RetailPrice <= 0 || input.OriginalPrice < 0 || input.SellableStock < 0 {
			return nil, ErrInvalidInput
		}
		specs, err := normalizeJSON(input.Specs, nil)
		if err != nil {
			return nil, err
		}
		if skuSpecsEmpty(specs) && (name == "" || isSystemDefaultSKUName(name)) {
			name = canonicalDefaultSKUName
		}
		if name == "" {
			return nil, ErrInvalidInput
		}
		id := uint(0)
		if preserveIDs {
			id = input.ID
		}
		skus = append(skus, models.SKU{
			BaseModel:     models.BaseModel{ID: id},
			Name:          name,
			ERPSKUCode:    erpCode,
			ImageURL:      imageURL,
			Specs:         specs,
			RetailPrice:   input.RetailPrice,
			OriginalPrice: input.OriginalPrice,
			SellableStock: input.SellableStock,
			IsActive:      true,
		})
	}

	return skus, nil
}

func isSystemDefaultSKUName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "默认规格", "默认sku", "default sku", "default specification":
		return true
	default:
		return false
	}
}

func skuSpecsEmpty(specs datatypes.JSON) bool {
	if len(specs) == 0 {
		return true
	}
	var values map[string]json.RawMessage
	return json.Unmarshal(specs, &values) == nil && len(values) == 0
}

func resetSKUCodes(product *models.SPU) []*models.SKU {
	targets := make([]*models.SKU, 0, len(product.SKUs))
	for i := range product.SKUs {
		product.SKUs[i].ERPSKUCode = ""
		targets = append(targets, &product.SKUs[i])
	}
	return targets
}

func resetSKUCodesForProducts(products []models.SPU) []*models.SKU {
	targets := make([]*models.SKU, 0)
	for i := range products {
		for j := range products[i].SKUs {
			products[i].SKUs[j].ERPSKUCode = ""
			targets = append(targets, &products[i].SKUs[j])
		}
	}
	return targets
}

func newProductSKUs(product *models.SPU) []*models.SKU {
	newSKUs := make([]*models.SKU, 0)
	for i := range product.SKUs {
		if product.SKUs[i].ID == 0 {
			product.SKUs[i].ERPSKUCode = ""
			newSKUs = append(newSKUs, &product.SKUs[i])
		}
	}
	return newSKUs
}

func (s *AdminProductService) createProductWithUniqueSKUCodes(
	ctx context.Context,
	tenantID uint,
	product *models.SPU,
	translation repository.ProductTranslationMutation,
) error {
	for attempt := 0; attempt < maxGeneratedSKUCodeAttempts; attempt++ {
		if attempt > 0 {
			resetCreatedProductState(product)
		}
		targets := resetSKUCodes(product)
		err := s.repo.CreateAtomic(ctx, tenantID, product, translation, func(lookup repository.SKUCodeLookup) error {
			return generateSKUCodes(tenantID, targets, lookup)
		})
		if err == nil {
			return nil
		}
		if errors.Is(err, repository.ErrProductCategoryMissing) {
			return ErrInvalidInput
		}
		if errors.Is(err, repository.ErrProductTenantUnavailable) {
			return ErrForbidden
		}
		if !errors.Is(err, repository.ErrDuplicateSKUCode) {
			return err
		}
	}
	return ErrConflict
}

func (s *AdminProductService) createProductsWithUniqueSKUCodes(ctx context.Context, tenantID uint, products []models.SPU) error {
	for attempt := 0; attempt < maxGeneratedSKUCodeAttempts; attempt++ {
		if attempt > 0 {
			for index := range products {
				resetCreatedProductState(&products[index])
			}
		}
		targets := resetSKUCodesForProducts(products)
		err := s.repo.CreateManyAtomic(ctx, tenantID, products, func(lookup repository.SKUCodeLookup) error {
			return generateSKUCodes(tenantID, targets, lookup)
		})
		if err == nil {
			return nil
		}
		if errors.Is(err, repository.ErrProductCategoryMissing) {
			return ErrInvalidInput
		}
		if errors.Is(err, repository.ErrProductTenantUnavailable) {
			return ErrForbidden
		}
		if !errors.Is(err, repository.ErrDuplicateSKUCode) {
			return err
		}
	}
	return ErrConflict
}

func (s *AdminProductService) updateProductWithUniqueSKUCodes(
	ctx context.Context,
	tenantID uint,
	product *models.SPU,
	newSKUs []*models.SKU,
	translation repository.ProductTranslationMutation,
) error {
	for attempt := 0; attempt < maxGeneratedSKUCodeAttempts; attempt++ {
		for _, sku := range newSKUs {
			sku.BaseModel = models.BaseModel{}
			sku.ProductID = 0
			sku.ERPSKUCode = ""
		}
		err := s.repo.UpdateAtomic(ctx, tenantID, product, translation, func(lookup repository.SKUCodeLookup) error {
			return generateSKUCodes(tenantID, newSKUs, lookup)
		})
		if err == nil {
			return nil
		}
		if errors.Is(err, repository.ErrProductCategoryMissing) {
			return ErrInvalidInput
		}
		if errors.Is(err, repository.ErrProductTenantUnavailable) {
			return ErrForbidden
		}
		if !errors.Is(err, repository.ErrDuplicateSKUCode) {
			return err
		}
	}
	return ErrConflict
}

func resetCreatedProductState(product *models.SPU) {
	product.BaseModel = models.BaseModel{}
	for index := range product.SKUs {
		product.SKUs[index].BaseModel = models.BaseModel{}
		product.SKUs[index].ProductID = 0
	}
}

func generateSKUCodes(tenantID uint, targets []*models.SKU, lookup repository.SKUCodeLookup) error {
	if len(targets) == 0 {
		return nil
	}

	timestamp := generatedSKUTimestamp(time.Now().UTC())
	sequence := 1
	codes := make([]string, len(targets))
	for index := range targets {
		codes[index] = formatGeneratedSKUCode(tenantID, timestamp, sequence, "")
		sequence++
	}

	for attempt := 0; attempt < maxGeneratedSKUCodeAttempts; attempt++ {
		existingCodes, err := lookup(codes)
		if err != nil {
			return err
		}

		conflicts := generatedSKUCodeConflicts(codes, existingCodes)
		if len(conflicts) == 0 {
			for index, target := range targets {
				target.ERPSKUCode = codes[index]
			}
			return nil
		}

		for index := range conflicts {
			suffix, err := randomSKUCodeSuffix()
			if err != nil {
				return err
			}
			codes[index] = formatGeneratedSKUCode(tenantID, timestamp, sequence, suffix)
			sequence++
		}
	}

	return repository.ErrDuplicateSKUCode
}

func generatedSKUTimestamp(value time.Time) string {
	return value.Format("20060102150405") + fmt.Sprintf("%06d", value.Nanosecond()/1000)
}

func formatGeneratedSKUCode(tenantID uint, timestamp string, sequence int, suffix string) string {
	code := fmt.Sprintf("SKU-%d-%s-"+generatedSKUCodeSequenceFormat, tenantID, timestamp, sequence)
	if suffix != "" {
		code += "-" + suffix
	}
	return code
}

func generatedSKUCodeConflicts(codes []string, existingCodes map[string]struct{}) map[int]struct{} {
	conflicts := make(map[int]struct{})
	seen := make(map[string]struct{}, len(codes))
	for index, code := range codes {
		if _, exists := existingCodes[code]; exists {
			conflicts[index] = struct{}{}
			continue
		}
		if _, exists := seen[code]; exists {
			conflicts[index] = struct{}{}
			continue
		}
		seen[code] = struct{}{}
	}
	return conflicts
}

func randomSKUCodeSuffix() (string, error) {
	bytes := make([]byte, generatedSKUCodeRandomBytes)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(bytes)), nil
}

func productTranslationMutation(input *ProductTranslationInput) repository.ProductTranslationMutation {
	if input == nil {
		return repository.ProductTranslationMutation{}
	}
	return repository.ProductTranslationMutation{
		Provided:    true,
		Name:        strings.TrimSpace(input.Name),
		Description: strings.TrimSpace(input.Description),
		DetailHTML:  sanitizeProductDetailHTML(input.DetailHTML),
		Material:    strings.TrimSpace(input.Material),
		Dimensions:  strings.TrimSpace(input.Dimensions),
		Weight:      strings.TrimSpace(input.Weight),
	}
}

func normalizeSliderImages(input *datatypes.JSON, fallback datatypes.JSON) (datatypes.JSON, error) {
	return normalizeJSON(input, fallback)
}

func normalizeJSON(input *datatypes.JSON, fallback datatypes.JSON) (datatypes.JSON, error) {
	if input == nil {
		return cloneJSON(fallback), nil
	}
	if len(*input) > 0 && !json.Valid(*input) {
		return nil, ErrInvalidInput
	}
	return cloneJSON(*input), nil
}

func cloneJSON(input datatypes.JSON) datatypes.JSON {
	if len(input) == 0 {
		return nil
	}
	return append(datatypes.JSON(nil), input...)
}

func normalizePagination(page int, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > maxAdminProductPageSize {
		pageSize = maxAdminProductPageSize
	}
	return page, pageSize
}

func isValidSPUStatus(status models.SPUStatus) bool {
	return status == models.SPUStatusOnShelf || status == models.SPUStatusOffShelf
}
