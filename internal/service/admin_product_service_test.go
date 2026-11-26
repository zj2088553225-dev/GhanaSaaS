package service

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"saas/internal/models"
	"saas/internal/repository"

	"gorm.io/gorm"
)

type fakeProductRepository struct {
	mu                 sync.Mutex
	nextSPU            uint
	nextSKU            uint
	products           map[uint]models.SPU
	tenants            map[uint]models.Tenant
	categories         map[uint]map[string]models.Category
	platformCategories map[uint]models.PlatformCategory
	existingSKUCodes   map[string]struct{}
	translations       *fakeTranslationReader
	orderItemRefs      map[uint]int64
	createErrors       []error
	createCalls        int
	translationError   error
}

func newFakeProductRepository() *fakeProductRepository {
	return &fakeProductRepository{
		nextSPU:  1,
		nextSKU:  1,
		products: make(map[uint]models.SPU),
		tenants: map[uint]models.Tenant{
			1001: {BaseModel: models.BaseModel{ID: 1001}, Status: models.TenantStatusActive},
			1002: {BaseModel: models.BaseModel{ID: 1002}, Status: models.TenantStatusActive},
			9001: {BaseModel: models.BaseModel{ID: 9001}, Status: models.TenantStatusActive},
		},
		platformCategories: map[uint]models.PlatformCategory{
			5000: {
				BaseModel: models.BaseModel{ID: 5000},
				Name:      "食品饮料",
				Path:      "食品饮料",
				Level:     1,
				Status:    models.PlatformCategoryStatusActive,
				IsLeaf:    false,
			},
			5001: {
				BaseModel: models.BaseModel{ID: 5001},
				ParentID:  uintPtr(5000),
				Name:      "碳酸饮料",
				Path:      "食品饮料/饮料/碳酸饮料",
				Level:     3,
				Status:    models.PlatformCategoryStatusActive,
				IsLeaf:    true,
			},
			5002: {
				BaseModel: models.BaseModel{ID: 5002},
				ParentID:  uintPtr(5000),
				Name:      "停用类目",
				Path:      "食品饮料/停用类目",
				Level:     2,
				Status:    models.PlatformCategoryStatusDisabled,
				IsLeaf:    true,
			},
			5003: {
				BaseModel: models.BaseModel{ID: 5003},
				Name:      "历史最末级",
				Path:      "历史最末级",
				Level:     1,
				Status:    models.PlatformCategoryStatusActive,
				IsLeaf:    false,
			},
		},
		categories: map[uint]map[string]models.Category{
			1001: {
				"饮料": {BaseModel: models.BaseModel{ID: 2001}, TenantID: 1001, Name: "饮料", IsActive: true},
			},
			1002: {
				"饮料": {BaseModel: models.BaseModel{ID: 3001}, TenantID: 1002, Name: "饮料", IsActive: true},
			},
			9001: {
				"饮料": {BaseModel: models.BaseModel{ID: 900101}, TenantID: 9001, Name: "饮料", IsActive: true},
			},
		},
		existingSKUCodes: make(map[string]struct{}),
		orderItemRefs:    make(map[uint]int64),
	}
}

func (r *fakeProductRepository) CreateAtomic(
	_ context.Context,
	tenantID uint,
	product *models.SPU,
	translation repository.ProductTranslationMutation,
	generateCodes repository.SKUCodeGenerator,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.createCalls++

	rollback := r.beginProductMutationLocked()
	committed := false
	var collisionCodes []string
	defer func() {
		if !committed {
			rollback()
			for _, code := range collisionCodes {
				r.existingSKUCodes[code] = struct{}{}
			}
		}
	}()
	if err := r.validateProductWriteScopeLocked(tenantID, product); err != nil {
		return err
	}
	if generateCodes != nil {
		if err := generateCodes(r.findSKUCodesLocked); err != nil {
			return err
		}
	}
	r.createProductLocked(product)
	if len(r.createErrors) > 0 {
		err := r.createErrors[0]
		r.createErrors = r.createErrors[1:]
		if errors.Is(err, repository.ErrDuplicateSKUCode) {
			for _, sku := range product.SKUs {
				collisionCodes = append(collisionCodes, sku.ERPSKUCode)
			}
		}
		return err
	}
	if err := r.saveProductTranslationLocked(product.ID, translation); err != nil {
		return err
	}
	committed = true
	return nil
}

func (r *fakeProductRepository) CreateManyAtomic(
	_ context.Context,
	tenantID uint,
	products []models.SPU,
	generateCodes repository.SKUCodeGenerator,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.createCalls++
	rollback := r.beginProductMutationLocked()
	committed := false
	var collisionCodes []string
	defer func() {
		if !committed {
			rollback()
			for _, code := range collisionCodes {
				r.existingSKUCodes[code] = struct{}{}
			}
		}
	}()
	for i := range products {
		if err := r.validateProductWriteScopeLocked(tenantID, &products[i]); err != nil {
			return err
		}
	}
	if generateCodes != nil {
		if err := generateCodes(r.findSKUCodesLocked); err != nil {
			return err
		}
	}
	for i := range products {
		r.createProductLocked(&products[i])
	}
	if len(r.createErrors) > 0 {
		err := r.createErrors[0]
		r.createErrors = r.createErrors[1:]
		if errors.Is(err, repository.ErrDuplicateSKUCode) {
			for i := range products {
				for _, sku := range products[i].SKUs {
					collisionCodes = append(collisionCodes, sku.ERPSKUCode)
				}
			}
		}
		return err
	}
	committed = true
	return nil
}

func (r *fakeProductRepository) FindByIDWithTenant(_ context.Context, tenantID uint, id uint) (*models.SPU, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	product, ok := r.products[id]
	if !ok || product.TenantID != tenantID {
		return nil, gorm.ErrRecordNotFound
	}
	cloned := cloneProduct(product)
	cloned.SKUs = activeSKUs(cloned.SKUs)
	return &cloned, nil
}

func (r *fakeProductRepository) ListByTenant(_ context.Context, query repository.AdminProductQuery) ([]models.SPU, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	items := make([]models.SPU, 0)
	for _, product := range r.products {
		if product.TenantID != query.TenantID {
			continue
		}
		if query.CategoryID != nil && (product.CategoryID == nil || *product.CategoryID != *query.CategoryID) {
			continue
		}
		if query.PlatformCategoryID != nil && product.PlatformCategoryID != *query.PlatformCategoryID {
			continue
		}
		if query.Status != nil && product.Status != *query.Status {
			continue
		}
		if keyword := strings.TrimSpace(query.Keyword); keyword != "" && !strings.Contains(strings.ToLower(product.Name), strings.ToLower(keyword)) {
			continue
		}
		items = append(items, cloneProduct(product))
	}
	sort.Slice(items, func(i int, j int) bool { return items[i].ID > items[j].ID })
	total := int64(len(items))
	start := (query.Page - 1) * query.PageSize
	if start >= len(items) {
		return []models.SPU{}, total, nil
	}
	end := start + query.PageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end], total, nil
}

func (r *fakeProductRepository) FindCategoryByIDWithTenant(_ context.Context, tenantID uint, id uint) (*models.Category, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, category := range r.categories[tenantID] {
		if category.ID == id {
			cloned := category
			return &cloned, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *fakeProductRepository) FindPlatformCategoryByID(_ context.Context, id uint) (*models.PlatformCategory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	category, ok := r.platformCategories[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	cloned := category
	return &cloned, nil
}

func (r *fakeProductRepository) FindPlatformCategoriesByPaths(_ context.Context, paths []string) (map[string]models.PlatformCategory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	lookup := make(map[string]models.PlatformCategory)
	wanted := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		wanted[strings.ToLower(strings.TrimSpace(path))] = struct{}{}
	}
	for _, category := range r.platformCategories {
		key := strings.ToLower(strings.TrimSpace(category.Path))
		if _, ok := wanted[key]; ok {
			lookup[key] = category
		}
	}
	return lookup, nil
}

func (r *fakeProductRepository) HasPlatformCategoryChildren(_ context.Context, id uint) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, category := range r.platformCategories {
		if category.ParentID != nil && *category.ParentID == id {
			return true, nil
		}
	}
	return false, nil
}

func (r *fakeProductRepository) FindCategoriesByName(_ context.Context, tenantID uint, names []string) (map[string]models.Category, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	categories := make(map[string]models.Category)
	tenantCategories := r.categories[tenantID]
	for _, name := range names {
		if category, ok := tenantCategories[strings.ToLower(strings.TrimSpace(name))]; ok {
			categories[strings.ToLower(strings.TrimSpace(name))] = category
		}
	}
	return categories, nil
}

func (r *fakeProductRepository) UpdateAtomic(
	_ context.Context,
	tenantID uint,
	product *models.SPU,
	translation repository.ProductTranslationMutation,
	generateCodes repository.SKUCodeGenerator,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rollback := r.beginProductMutationLocked()
	committed := false
	defer func() {
		if !committed {
			rollback()
		}
	}()

	existing, ok := r.products[product.ID]
	if !ok || existing.TenantID != tenantID {
		return gorm.ErrRecordNotFound
	}
	if err := r.validateProductWriteScopeLocked(tenantID, product); err != nil {
		return err
	}

	updated := cloneProduct(*product)
	existingByID := make(map[uint]models.SKU, len(existing.SKUs))
	for _, sku := range existing.SKUs {
		existingByID[sku.ID] = sku
	}

	skus := make([]models.SKU, 0, len(product.SKUs)+len(existing.SKUs))
	activeIDs := make(map[uint]struct{}, len(product.SKUs))
	for i := range product.SKUs {
		product.SKUs[i].ProductID = product.ID
		product.SKUs[i].IsActive = true
		if product.SKUs[i].ID == 0 {
			product.SKUs[i].ERPSKUCode = ""
		} else if existingSKU, ok := existingByID[product.SKUs[i].ID]; !ok || existingSKU.ProductID != product.ID {
			return gorm.ErrRecordNotFound
		} else {
			product.SKUs[i].ERPSKUCode = existingSKU.ERPSKUCode
		}
	}
	if generateCodes != nil {
		if err := generateCodes(r.findSKUCodesLocked); err != nil {
			return err
		}
	}
	for _, sku := range product.SKUs {
		sku.ProductID = product.ID
		sku.IsActive = true
		if sku.ID == 0 {
			sku.ID = r.nextSKU
			r.nextSKU++
		}
		activeIDs[sku.ID] = struct{}{}
		skus = append(skus, sku)
	}
	for _, sku := range existing.SKUs {
		if _, ok := activeIDs[sku.ID]; ok {
			continue
		}
		sku.IsActive = false
		skus = append(skus, sku)
	}
	updated.SKUs = skus
	r.products[product.ID] = cloneProduct(updated)
	if len(r.createErrors) > 0 {
		err := r.createErrors[0]
		r.createErrors = r.createErrors[1:]
		return err
	}
	if err := r.saveProductTranslationLocked(product.ID, translation); err != nil {
		return err
	}
	committed = true
	return nil
}

func (r *fakeProductRepository) UpdateStatusWithTenant(_ context.Context, tenantID uint, id uint, status models.SPUStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	product, ok := r.products[id]
	if !ok || product.TenantID != tenantID {
		return gorm.ErrRecordNotFound
	}
	product.Status = status
	r.products[id] = product
	return nil
}

func (r *fakeProductRepository) DeleteWithTenant(_ context.Context, tenantID uint, id uint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	product, ok := r.products[id]
	if !ok || product.TenantID != tenantID {
		return gorm.ErrRecordNotFound
	}
	for _, sku := range product.SKUs {
		if r.orderItemRefs[sku.ID] > 0 {
			return repository.ErrProductHasOrderRecords
		}
	}
	delete(r.products, id)
	return nil
}

func cloneProduct(product models.SPU) models.SPU {
	product.SliderImages = append([]byte(nil), product.SliderImages...)
	product.SKUs = append([]models.SKU(nil), product.SKUs...)
	return product
}

func activeSKUs(skus []models.SKU) []models.SKU {
	active := make([]models.SKU, 0, len(skus))
	for _, sku := range skus {
		if sku.IsActive {
			active = append(active, sku)
		}
	}
	return active
}

func (r *fakeProductRepository) beginProductMutationLocked() func() {
	products := make(map[uint]models.SPU, len(r.products))
	for id, product := range r.products {
		products[id] = cloneProduct(product)
	}
	codes := make(map[string]struct{}, len(r.existingSKUCodes))
	for code := range r.existingSKUCodes {
		codes[code] = struct{}{}
	}
	nextSPU := r.nextSPU
	nextSKU := r.nextSKU
	var translations []models.ContentTranslation
	if r.translations != nil {
		translations = append([]models.ContentTranslation(nil), r.translations.rows...)
	}
	return func() {
		r.products = products
		r.existingSKUCodes = codes
		r.nextSPU = nextSPU
		r.nextSKU = nextSKU
		if r.translations != nil {
			r.translations.rows = translations
		}
	}
}

func (r *fakeProductRepository) validateProductWriteScopeLocked(tenantID uint, product *models.SPU) error {
	tenant, ok := r.tenants[tenantID]
	if tenantID == 0 || product == nil || product.TenantID != tenantID || !ok || tenant.Status != models.TenantStatusActive {
		return repository.ErrProductTenantUnavailable
	}
	category, ok := r.platformCategories[product.PlatformCategoryID]
	if !ok || category.Status != models.PlatformCategoryStatusActive {
		return repository.ErrProductCategoryMissing
	}
	for _, candidate := range r.platformCategories {
		if candidate.ParentID != nil && *candidate.ParentID == category.ID {
			return repository.ErrProductCategoryMissing
		}
	}
	if product.CategoryID != nil {
		found := false
		for _, shopCategory := range r.categories[tenantID] {
			if shopCategory.ID == *product.CategoryID {
				found = true
				break
			}
		}
		if !found {
			return repository.ErrProductCategoryMissing
		}
	}
	return nil
}

func (r *fakeProductRepository) findSKUCodesLocked(codes []string) (map[string]struct{}, error) {
	wanted := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		wanted[code] = struct{}{}
	}
	existing := make(map[string]struct{})
	for code := range r.existingSKUCodes {
		if _, ok := wanted[code]; ok {
			existing[code] = struct{}{}
		}
	}
	for _, product := range r.products {
		for _, sku := range product.SKUs {
			if _, ok := wanted[sku.ERPSKUCode]; ok {
				existing[sku.ERPSKUCode] = struct{}{}
			}
		}
	}
	return existing, nil
}

func (r *fakeProductRepository) createProductLocked(product *models.SPU) {
	product.ID = r.nextSPU
	r.nextSPU++
	for i := range product.SKUs {
		product.SKUs[i].ID = r.nextSKU
		r.nextSKU++
		product.SKUs[i].ProductID = product.ID
		product.SKUs[i].IsActive = true
	}
	r.products[product.ID] = cloneProduct(*product)
}

func (r *fakeProductRepository) saveProductTranslationLocked(
	productID uint,
	mutation repository.ProductTranslationMutation,
) error {
	if mutation.Provided && r.translations != nil {
		if err := r.translations.SaveTranslationsForEntity(
			context.Background(), TranslationEntityProduct, productID, "zh",
			map[string]TranslationValueInput{
				TranslationFieldName:        {ValueText: mutation.Name},
				TranslationFieldDescription: {ValueText: mutation.Description},
				TranslationFieldDetailHTML:  {ValueHTML: mutation.DetailHTML},
				TranslationFieldMaterial:    {ValueText: mutation.Material},
				TranslationFieldDimensions:  {ValueText: mutation.Dimensions},
				TranslationFieldWeight:      {ValueText: mutation.Weight},
			},
		); err != nil {
			return err
		}
	}
	return r.translationError
}

func newAdminProductTestService() *AdminProductService {
	return NewAdminProductService(newFakeProductRepository())
}

func validAdminProductInput(name string) AdminProductInput {
	categoryID := uint(2001)
	return AdminProductInput{
		PlatformCategoryID: 5001,
		CategoryID:         &categoryID,
		Name:               name,
		Description:        "description",
		DetailHTML:         `<p>Detail</p><img src="/uploads/detail.webp"><script>alert(1)</script>`,
		MainImage:          "https://example.com/product.jpg",
		VideoURL:           "/uploads/videos/2026/06/product.mp4",
		Material:           "Cotton",
		Dimensions:         "30 × 20 × 10 cm",
		Weight:             "1.2 kg",
		Status:             models.SPUStatusOffShelf,
		SKUs: []AdminSKUInput{
			{
				Name:          "Default SKU",
				ERPSKUCode:    "SKU-001",
				ImageURL:      "/uploads/sku-default.webp",
				RetailPrice:   1299,
				OriginalPrice: 1599,
				SellableStock: 10,
			},
		},
	}
}

func TestAdminProductServiceCreateUpdateAndStatus(t *testing.T) {
	service := newAdminProductTestService()
	ctx := context.Background()

	created, err := service.Create(ctx, 1001, validAdminProductInput("Created Product"))
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if created.TenantID != 1001 {
		t.Fatalf("TenantID = %d, want 1001", created.TenantID)
	}
	if created.PlatformCategoryID != 5001 {
		t.Fatalf("PlatformCategoryID = %d, want 5001", created.PlatformCategoryID)
	}
	if created.CategoryID == nil || *created.CategoryID != 2001 {
		t.Fatalf("CategoryID = %v, want shop category 2001", created.CategoryID)
	}
	if created.Status != models.SPUStatusOffShelf {
		t.Fatalf("Status = %q, want %q", created.Status, models.SPUStatusOffShelf)
	}
	if !strings.Contains(created.DetailHTML, `<p>Detail</p>`) || !strings.Contains(created.DetailHTML, `/uploads/detail.webp`) || strings.Contains(created.DetailHTML, "script") {
		t.Fatalf("DetailHTML = %q, want sanitized detail HTML", created.DetailHTML)
	}
	if len(created.SKUs) != 1 || created.SKUs[0].RetailPrice != 1299 || created.SKUs[0].SellableStock != 10 {
		t.Fatalf("unexpected created SKUs: %+v", created.SKUs)
	}
	if created.SKUs[0].ImageURL != "/uploads/sku-default.webp" {
		t.Fatalf("created SKU image_url = %q", created.SKUs[0].ImageURL)
	}
	if created.VideoURL != "/uploads/videos/2026/06/product.mp4" {
		t.Fatalf("created VideoURL = %q", created.VideoURL)
	}
	if created.Material != "Cotton" || created.Dimensions != "30 × 20 × 10 cm" || created.Weight != "1.2 kg" {
		t.Fatalf("created product parameters = %q / %q / %q", created.Material, created.Dimensions, created.Weight)
	}
	if created.SKUs[0].ERPSKUCode == "" || created.SKUs[0].ERPSKUCode == "SKU-001" {
		t.Fatalf("created SKU code = %q, want system-generated code", created.SKUs[0].ERPSKUCode)
	}

	updateInput := validAdminProductInput("Updated Product")
	updateInput.Status = ""
	updateInput.SKUs[0].ID = created.SKUs[0].ID
	updateInput.SKUs[0].ERPSKUCode = "USER-CHANGED-SKU"
	updateInput.SKUs[0].RetailPrice = 2099
	updateInput.SKUs[0].SellableStock = 7
	updateInput.SKUs[0].ImageURL = "/uploads/sku-updated.webp"
	updated, err := service.Update(ctx, 1001, created.ID, updateInput)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if updated.Name != "Updated Product" {
		t.Fatalf("Name = %q, want Updated Product", updated.Name)
	}
	if updated.Status != models.SPUStatusOffShelf {
		t.Fatalf("Status = %q, want preserved off_shelf", updated.Status)
	}
	if len(updated.SKUs) != 1 || updated.SKUs[0].RetailPrice != 2099 || updated.SKUs[0].SellableStock != 7 {
		t.Fatalf("unexpected updated SKUs: %+v", updated.SKUs)
	}
	if updated.SKUs[0].ID != created.SKUs[0].ID {
		t.Fatalf("updated SKU ID = %d, want preserved %d", updated.SKUs[0].ID, created.SKUs[0].ID)
	}
	if updated.SKUs[0].ERPSKUCode != created.SKUs[0].ERPSKUCode {
		t.Fatalf("updated SKU code = %q, want preserved %q", updated.SKUs[0].ERPSKUCode, created.SKUs[0].ERPSKUCode)
	}
	if updated.SKUs[0].ImageURL != "/uploads/sku-updated.webp" {
		t.Fatalf("updated SKU image_url = %q", updated.SKUs[0].ImageURL)
	}

	updateInput.SKUs[0].ImageURL = ""
	updateInput.VideoURL = ""
	updateInput.Material = ""
	updateInput.Dimensions = ""
	updateInput.Weight = ""
	clearedImage, err := service.Update(ctx, 1001, created.ID, updateInput)
	if err != nil {
		t.Fatalf("Update clearing SKU image returned error: %v", err)
	}
	if clearedImage.SKUs[0].ImageURL != "" {
		t.Fatalf("cleared SKU image_url = %q, want empty", clearedImage.SKUs[0].ImageURL)
	}
	if clearedImage.VideoURL != "" {
		t.Fatalf("cleared VideoURL = %q, want empty", clearedImage.VideoURL)
	}
	if clearedImage.Material != "" || clearedImage.Dimensions != "" || clearedImage.Weight != "" {
		t.Fatalf("cleared parameters = %q / %q / %q", clearedImage.Material, clearedImage.Dimensions, clearedImage.Weight)
	}

	onShelf, err := service.UpdateStatus(ctx, 1001, created.ID, models.SPUStatusOnShelf)
	if err != nil {
		t.Fatalf("UpdateStatus returned error: %v", err)
	}
	if onShelf.Status != models.SPUStatusOnShelf {
		t.Fatalf("Status = %q, want %q", onShelf.Status, models.SPUStatusOnShelf)
	}

	page, err := service.List(ctx, 1001, AdminProductQuery{Page: -1, PageSize: 999})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if page.Page != 1 || page.PageSize != maxAdminProductPageSize || page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("unexpected page: %+v", page)
	}
}

func TestAdminProductServiceSavesTranslationsAndStorefrontUsesThem(t *testing.T) {
	productRepo := newFakeProductRepository()
	translations := &fakeTranslationReader{}
	productRepo.translations = translations
	adminService := NewAdminProductService(productRepo, AdminProductOptions{Translations: translations})
	ctx := context.Background()

	input := validAdminProductInput("English Phone")
	input.Status = models.SPUStatusOnShelf
	input.Translations = &ProductTranslationInput{
		Name:        "中文手机",
		Description: "中文描述",
		DetailHTML:  `<p>中文详情</p><script>alert(1)</script>`,
		Material:    "金属",
		Dimensions:  "10厘米",
		Weight:      "1公斤",
	}
	created, err := adminService.Create(ctx, 1001, input)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	loadedTranslations, err := adminService.TranslationFields(ctx, 1001, created.ID, "zh")
	if err != nil {
		t.Fatalf("TranslationFields returned error: %v", err)
	}
	if loadedTranslations.Name != "中文手机" || loadedTranslations.Description != "中文描述" {
		t.Fatalf("loaded translations = %+v, want zh fields", loadedTranslations)
	}
	if strings.Contains(loadedTranslations.DetailHTML, "script") {
		t.Fatalf("translation detail_html was not sanitized: %s", loadedTranslations.DetailHTML)
	}

	storefront := NewStorefrontService(&fakeStorefrontRepository{
		products: []models.SPU{*created},
		profiles: []models.ShopProfile{{
			BaseModel: models.BaseModel{ID: 801},
			TenantID:  1001,
			Slug:      "tenant-shop",
			ShopName:  "Tenant Shop",
			IsPublic:  true,
		}},
	}, StorefrontOptions{Translations: translations})
	page, err := storefront.Products(ctx, StorefrontProductQuery{Locale: "zh", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("storefront Products returned error: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Name != "中文手机" || page.Items[0].Description != "中文描述" {
		t.Fatalf("localized storefront item = %+v, want zh product content", page.Items)
	}

	clearInput := validAdminProductInput("English Phone Updated")
	clearInput.Status = models.SPUStatusOnShelf
	clearInput.SKUs[0].ID = created.SKUs[0].ID
	clearInput.Translations = &ProductTranslationInput{Name: "", Description: ""}
	updated, err := adminService.Update(ctx, 1001, created.ID, clearInput)
	if err != nil {
		t.Fatalf("Update clearing translation returned error: %v", err)
	}
	storefront = NewStorefrontService(&fakeStorefrontRepository{
		products: []models.SPU{*updated},
		profiles: []models.ShopProfile{{
			BaseModel: models.BaseModel{ID: 801},
			TenantID:  1001,
			Slug:      "tenant-shop",
			ShopName:  "Tenant Shop",
			IsPublic:  true,
		}},
	}, StorefrontOptions{Translations: translations})
	page, err = storefront.Products(ctx, StorefrontProductQuery{Locale: "zh", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("storefront Products after clear returned error: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Name != "English Phone Updated" {
		t.Fatalf("cleared translation item = %+v, want main table fallback", page.Items)
	}
}

func TestAdminProductServiceRejectsOtherTenantTranslationWrite(t *testing.T) {
	productRepo := newFakeProductRepository()
	translations := &fakeTranslationReader{}
	productRepo.translations = translations
	adminService := NewAdminProductService(productRepo, AdminProductOptions{Translations: translations})
	ctx := context.Background()

	input := validAdminProductInput("Tenant A Product")
	input.Translations = &ProductTranslationInput{Name: "租户A商品"}
	created, err := adminService.Create(ctx, 1001, input)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	otherTenantInput := validAdminProductInput("Other Tenant Write")
	otherTenantInput.Translations = &ProductTranslationInput{Name: "非法写入"}
	if _, err := adminService.Update(ctx, 2002, created.ID, otherTenantInput); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update with other tenant error = %v, want ErrNotFound", err)
	}

	loadedTranslations, err := adminService.TranslationFields(ctx, 1001, created.ID, "zh")
	if err != nil {
		t.Fatalf("TranslationFields returned error: %v", err)
	}
	if loadedTranslations.Name != "租户A商品" {
		t.Fatalf("translation after rejected write = %+v, want original tenant translation", loadedTranslations)
	}
}

func TestAdminProductServiceCreateGeneratesSKUCodeWhenEmpty(t *testing.T) {
	service := newAdminProductTestService()
	ctx := context.Background()
	input := validAdminProductInput("Generated SKU Product")
	input.SKUs[0].ERPSKUCode = ""

	created, err := service.Create(ctx, 1001, input)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if len(created.SKUs) != 1 {
		t.Fatalf("created SKUs = %+v, want one SKU", created.SKUs)
	}
	if !strings.HasPrefix(created.SKUs[0].ERPSKUCode, "SKU-1001-") {
		t.Fatalf("generated SKU code = %q, want tenant prefix", created.SKUs[0].ERPSKUCode)
	}
}

func TestAdminProductServiceRejectsUnsafeVideoURL(t *testing.T) {
	service := newAdminProductTestService()
	input := validAdminProductInput("Unsafe Video Product")
	input.VideoURL = "javascript:alert(1)"

	if _, err := service.Create(context.Background(), 1001, input); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Create error = %v, want ErrInvalidInput", err)
	}
}

func TestAdminProductServiceGeneratedSKUCodesAreUniqueWithinTenant(t *testing.T) {
	service := newAdminProductTestService()
	ctx := context.Background()

	first, err := service.Create(ctx, 1001, validAdminProductInput("First Product"))
	if err != nil {
		t.Fatalf("Create first product returned error: %v", err)
	}
	second, err := service.Create(ctx, 1001, validAdminProductInput("Second Product"))
	if err != nil {
		t.Fatalf("Create second product returned error: %v", err)
	}

	firstCode := first.SKUs[0].ERPSKUCode
	secondCode := second.SKUs[0].ERPSKUCode
	if firstCode == "" || secondCode == "" || firstCode == secondCode {
		t.Fatalf("generated SKU codes should be non-empty and unique, got %q and %q", firstCode, secondCode)
	}
}

func TestAdminProductServiceRetriesDatabaseSKUCodeConflict(t *testing.T) {
	repo := newFakeProductRepository()
	repo.createErrors = []error{repository.ErrDuplicateSKUCode}
	service := NewAdminProductService(repo)

	created, err := service.Create(context.Background(), 1001, validAdminProductInput("Concurrent SKU Product"))
	if err != nil {
		t.Fatalf("Create returned error after retryable SKU conflict: %v", err)
	}
	if repo.createCalls != 2 {
		t.Fatalf("create calls = %d, want 2", repo.createCalls)
	}
	if created.ID == 0 || len(created.SKUs) != 1 || created.SKUs[0].ID == 0 {
		t.Fatalf("created product was not persisted after retry: %+v", created)
	}
	if _, collided := repo.existingSKUCodes[created.SKUs[0].ERPSKUCode]; collided {
		t.Fatalf("retry reused collided SKU code %q", created.SKUs[0].ERPSKUCode)
	}
}

func TestAdminProductServiceStopsSKUCodeConflictRetriesAndRollsBack(t *testing.T) {
	repo := newFakeProductRepository()
	for attempt := 0; attempt < maxGeneratedSKUCodeAttempts; attempt++ {
		repo.createErrors = append(repo.createErrors, repository.ErrDuplicateSKUCode)
	}
	service := NewAdminProductService(repo)

	if _, err := service.Create(context.Background(), 1001, validAdminProductInput("Conflict Exhausted")); !errors.Is(err, ErrConflict) {
		t.Fatalf("Create error = %v, want ErrConflict", err)
	}
	if repo.createCalls != maxGeneratedSKUCodeAttempts {
		t.Fatalf("create calls = %d, want %d", repo.createCalls, maxGeneratedSKUCodeAttempts)
	}
	if len(repo.products) != 0 {
		t.Fatalf("products remained after exhausted retries: %+v", repo.products)
	}
}

func TestAdminProductServiceFakeRepositoryRollsBackTranslationFailure(t *testing.T) {
	repo := newFakeProductRepository()
	translations := &fakeTranslationReader{}
	repo.translations = translations
	repo.translationError = errors.New("forced fake translation failure")
	service := NewAdminProductService(repo, AdminProductOptions{Translations: translations})
	input := validAdminProductInput("Atomic Fake Product")
	input.Translations = &ProductTranslationInput{Name: "中文商品"}
	productsBefore := make(map[uint]models.SPU, len(repo.products))
	for id, product := range repo.products {
		productsBefore[id] = cloneProduct(product)
	}

	if _, err := service.Create(context.Background(), 1001, input); err == nil {
		t.Fatal("Create returned nil error for fake translation failure")
	}
	if !reflect.DeepEqual(repo.products, productsBefore) {
		t.Fatalf("fake products changed after rollback\nbefore=%+v\nafter=%+v", productsBefore, repo.products)
	}
	if len(translations.rows) != 0 {
		t.Fatalf("fake translations remained after rollback: %+v", translations.rows)
	}
}

func TestAdminProductServiceMapsConcurrentCategoryDeletionToInvalidInput(t *testing.T) {
	repo := newFakeProductRepository()
	repo.createErrors = []error{repository.ErrProductCategoryMissing}
	service := NewAdminProductService(repo)

	_, err := service.Create(context.Background(), 1001, validAdminProductInput("Deleted Category Product"))
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Create error = %v, want ErrInvalidInput", err)
	}
	if repo.createCalls != 1 {
		t.Fatalf("create calls = %d, want no retry for missing category", repo.createCalls)
	}
}

func TestAdminProductServiceUpdatePreservesExistingSKUCodeAndGeneratesNewSKUCode(t *testing.T) {
	service := newAdminProductTestService()
	ctx := context.Background()

	created, err := service.Create(ctx, 1001, validAdminProductInput("Editable Product"))
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	originalCode := created.SKUs[0].ERPSKUCode

	updateInput := validAdminProductInput("Editable Product")
	updateInput.SKUs = []AdminSKUInput{
		{
			ID:            created.SKUs[0].ID,
			Name:          "Default SKU",
			ERPSKUCode:    "USER-CHANGED-SKU",
			RetailPrice:   1299,
			OriginalPrice: 1599,
			SellableStock: 10,
		},
		{
			Name:          "New SKU",
			ERPSKUCode:    "USER-NEW-SKU",
			RetailPrice:   1399,
			OriginalPrice: 1699,
			SellableStock: 8,
		},
	}

	updated, err := service.Update(ctx, 1001, created.ID, updateInput)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if len(updated.SKUs) != 2 {
		t.Fatalf("updated SKUs = %+v, want two active SKUs", updated.SKUs)
	}
	if updated.SKUs[0].ERPSKUCode != originalCode {
		t.Fatalf("existing SKU code = %q, want preserved %q", updated.SKUs[0].ERPSKUCode, originalCode)
	}
	if updated.SKUs[1].ERPSKUCode == "" || updated.SKUs[1].ERPSKUCode == "USER-NEW-SKU" || updated.SKUs[1].ERPSKUCode == originalCode {
		t.Fatalf("new SKU code should be generated and unique: %+v", updated.SKUs[1])
	}
}

func TestAdminProductServiceGeneratedSKUCodesIncludeTenant(t *testing.T) {
	service := newAdminProductTestService()
	ctx := context.Background()

	tenantAProduct, err := service.Create(ctx, 1001, validAdminProductInput("Tenant A Product"))
	if err != nil {
		t.Fatalf("Create tenant A product returned error: %v", err)
	}
	tenantBInput := validAdminProductInput("Tenant B Product")
	tenantBInput.CategoryID = uintPtr(3001)
	tenantBProduct, err := service.Create(ctx, 1002, tenantBInput)
	if err != nil {
		t.Fatalf("Create tenant B product returned error: %v", err)
	}

	if !strings.HasPrefix(tenantAProduct.SKUs[0].ERPSKUCode, "SKU-1001-") {
		t.Fatalf("tenant A SKU code = %q, want tenant A prefix", tenantAProduct.SKUs[0].ERPSKUCode)
	}
	if !strings.HasPrefix(tenantBProduct.SKUs[0].ERPSKUCode, "SKU-1002-") {
		t.Fatalf("tenant B SKU code = %q, want tenant B prefix", tenantBProduct.SKUs[0].ERPSKUCode)
	}
	if tenantAProduct.SKUs[0].ERPSKUCode == tenantBProduct.SKUs[0].ERPSKUCode {
		t.Fatalf("tenant SKU codes should not collide: %q", tenantAProduct.SKUs[0].ERPSKUCode)
	}
}

func TestAdminProductServiceTenantIsolation(t *testing.T) {
	service := newAdminProductTestService()
	ctx := context.Background()

	tenantBInput := validAdminProductInput("Tenant B Product")
	tenantBInput.CategoryID = uintPtr(3001)
	tenantBProduct, err := service.Create(ctx, 1002, tenantBInput)
	if err != nil {
		t.Fatalf("Create tenant B product returned error: %v", err)
	}

	if _, err := service.FindByID(ctx, 1001, tenantBProduct.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("FindByID with wrong tenant error = %v, want ErrNotFound", err)
	}
	if _, err := service.Update(ctx, 1001, tenantBProduct.ID, validAdminProductInput("Cross Tenant Update")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update with wrong tenant error = %v, want ErrNotFound", err)
	}
	if _, err := service.UpdateStatus(ctx, 1001, tenantBProduct.ID, models.SPUStatusOnShelf); !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateStatus with wrong tenant error = %v, want ErrNotFound", err)
	}
	if err := service.Delete(ctx, 1001, tenantBProduct.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete with wrong tenant error = %v, want ErrNotFound", err)
	}

	unchanged, err := service.FindByID(ctx, 1002, tenantBProduct.ID)
	if err != nil {
		t.Fatalf("FindByID with owner tenant returned error: %v", err)
	}
	if unchanged.Name != "Tenant B Product" || unchanged.Status != models.SPUStatusOffShelf {
		t.Fatalf("cross-tenant operations modified product: %+v", unchanged)
	}
}

func TestAdminProductServiceDeletesProductWithoutOrderRecords(t *testing.T) {
	repo := newFakeProductRepository()
	service := NewAdminProductService(repo)
	ctx := context.Background()
	product, err := service.Create(ctx, 1001, validAdminProductInput("Deletable Product"))
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	skuID := product.SKUs[0].ID

	if err := service.Delete(ctx, 1001, product.ID); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if _, exists := repo.products[product.ID]; exists {
		t.Fatal("product still exists after deletion")
	}
	if repo.orderItemRefs[skuID] != 0 {
		t.Fatal("delete changed unrelated order item references")
	}
}

func TestAdminProductServiceRejectsDeleteWhenSKUHasOrderRecords(t *testing.T) {
	repo := newFakeProductRepository()
	service := NewAdminProductService(repo)
	ctx := context.Background()
	product, err := service.Create(ctx, 1001, validAdminProductInput("Ordered Product"))
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	skuID := product.SKUs[0].ID
	repo.orderItemRefs[skuID] = 2
	productBefore := cloneProduct(repo.products[product.ID])

	err = service.Delete(ctx, 1001, product.ID)
	if !errors.Is(err, ErrAdminProductHasOrderRecords) {
		t.Fatalf("Delete error = %v, want ErrAdminProductHasOrderRecords", err)
	}
	if err.Error() != "该商品已有订单记录，不能删除，请下架或归档" {
		t.Fatalf("Delete error = %q, want friendly message", err.Error())
	}
	productAfter, exists := repo.products[product.ID]
	if !exists {
		t.Fatal("product was deleted despite order records")
	}
	if len(productAfter.SKUs) != len(productBefore.SKUs) || productAfter.SKUs[0].ID != skuID {
		t.Fatalf("SKUs changed after rejected deletion: before=%+v after=%+v", productBefore.SKUs, productAfter.SKUs)
	}
	if repo.orderItemRefs[skuID] != 2 {
		t.Fatal("order item references changed after rejected deletion")
	}
}

func TestAdminProductServiceCanTakeOrderedProductOffShelf(t *testing.T) {
	repo := newFakeProductRepository()
	service := NewAdminProductService(repo)
	ctx := context.Background()
	product, err := service.Create(ctx, 1001, validAdminProductInput("Ordered On-Shelf Product"))
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := service.UpdateStatus(ctx, 1001, product.ID, models.SPUStatusOnShelf); err != nil {
		t.Fatalf("UpdateStatus on shelf returned error: %v", err)
	}
	repo.orderItemRefs[product.SKUs[0].ID] = 1

	updated, err := service.UpdateStatus(ctx, 1001, product.ID, models.SPUStatusOffShelf)
	if err != nil {
		t.Fatalf("UpdateStatus off shelf returned error: %v", err)
	}
	if updated.Status != models.SPUStatusOffShelf {
		t.Fatalf("status = %q, want off_shelf", updated.Status)
	}
}

func TestAdminProductServiceRejectsCategoryFromOtherTenant(t *testing.T) {
	service := newAdminProductTestService()
	ctx := context.Background()
	input := validAdminProductInput("Wrong Category Product")
	input.CategoryID = uintPtr(3001)

	if _, err := service.Create(ctx, 1001, input); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Create with other tenant category error = %v, want ErrInvalidInput", err)
	}
}

func TestAdminProductServiceValidatesPlatformCategory(t *testing.T) {
	service := newAdminProductTestService()
	ctx := context.Background()

	cases := []struct {
		name               string
		platformCategoryID uint
	}{
		{name: "missing platform category", platformCategoryID: 0},
		{name: "non leaf platform category", platformCategoryID: 5000},
		{name: "disabled platform category", platformCategoryID: 5002},
		{name: "unknown platform category", platformCategoryID: 9999},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			input := validAdminProductInput("Invalid Platform Category Product")
			input.PlatformCategoryID = tt.platformCategoryID
			if _, err := service.Create(ctx, 1001, input); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("Create error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestAdminProductServiceAllowsMostSpecificCategoryEvenWhenLeafFlagIsStale(t *testing.T) {
	service := newAdminProductTestService()
	ctx := context.Background()
	input := validAdminProductInput("Stale Leaf Flag Product")
	input.PlatformCategoryID = 5003
	input.CategoryID = nil

	created, err := service.Create(ctx, 1001, input)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if created.PlatformCategoryID != 5003 {
		t.Fatalf("platform_category_id = %d, want 5003", created.PlatformCategoryID)
	}
}

func TestAdminProductServiceAllowsEmptyShopCategory(t *testing.T) {
	service := newAdminProductTestService()
	ctx := context.Background()
	input := validAdminProductInput("No Shop Category Product")
	input.CategoryID = nil

	created, err := service.Create(ctx, 1001, input)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if created.PlatformCategoryID != 5001 {
		t.Fatalf("platform_category_id = %d, want 5001", created.PlatformCategoryID)
	}
	if created.CategoryID != nil {
		t.Fatalf("category_id = %v, want nil", created.CategoryID)
	}
}

func TestAdminProductServiceUpdateAllowsMissingShopCategory(t *testing.T) {
	service := newAdminProductTestService()
	ctx := context.Background()

	created, err := service.Create(ctx, 1001, validAdminProductInput("Product With Category"))
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	updateInput := validAdminProductInput("Product Without Category")
	updateInput.CategoryID = nil
	updateInput.SKUs[0].ID = created.SKUs[0].ID

	updated, err := service.Update(ctx, 1001, created.ID, updateInput)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if updated.CategoryID != nil {
		t.Fatalf("category_id = %v, want nil", updated.CategoryID)
	}
}

func TestAdminProductServiceRejectsNegativePriceAndStock(t *testing.T) {
	service := newAdminProductTestService()
	ctx := context.Background()

	cases := []struct {
		name   string
		mutate func(*AdminProductInput)
	}{
		{
			name: "negative retail price",
			mutate: func(input *AdminProductInput) {
				input.SKUs[0].RetailPrice = -1
			},
		},
		{
			name: "negative original price",
			mutate: func(input *AdminProductInput) {
				input.SKUs[0].OriginalPrice = -1
			},
		},
		{
			name: "negative stock",
			mutate: func(input *AdminProductInput) {
				input.SKUs[0].SellableStock = -1
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			input := validAdminProductInput("Invalid Product")
			tt.mutate(&input)
			if _, err := service.Create(ctx, 1001, input); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("Create error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestAdminProductServiceCanonicalizesSystemDefaultSKUName(t *testing.T) {
	for _, name := range []string{"", "默认规格", "默认SKU", "Default SKU", "Default Specification"} {
		t.Run(name, func(t *testing.T) {
			input := validAdminProductInput("Default SKU Product " + name)
			input.SKUs[0].Name = name
			created, err := newAdminProductTestService().Create(context.Background(), 1001, input)
			if err != nil {
				t.Fatalf("Create returned error: %v", err)
			}
			if got := created.SKUs[0].Name; got != canonicalDefaultSKUName {
				t.Fatalf("SKU name = %q, want %q", got, canonicalDefaultSKUName)
			}
		})
	}

	input := validAdminProductInput("Named SKU Product")
	input.SKUs[0].Name = "Wholesale Carton"
	created, err := newAdminProductTestService().Create(context.Background(), 1001, input)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if got := created.SKUs[0].Name; got != "Wholesale Carton" {
		t.Fatalf("real SKU name = %q, want preserved name", got)
	}
}

func TestAdminProductServiceRejectsInvalidTenantAndStatus(t *testing.T) {
	service := newAdminProductTestService()
	ctx := context.Background()

	if _, err := service.Create(ctx, 0, validAdminProductInput("No Tenant")); !errors.Is(err, ErrForbidden) {
		t.Fatalf("Create with zero tenant error = %v, want ErrForbidden", err)
	}

	product, err := service.Create(ctx, 1001, validAdminProductInput("Status Product"))
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := service.UpdateStatus(ctx, 1001, product.ID, models.SPUStatus("invalid")); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("UpdateStatus invalid status error = %v, want ErrInvalidInput", err)
	}
}
