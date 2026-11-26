package service

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"strings"
	"testing"
	"time"

	"saas/internal/models"
	"saas/internal/pkg/locale"
	"saas/internal/repository"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type fakeStorefrontRepository struct {
	categories           []models.Category
	platformCategories   map[uint]models.PlatformCategory
	tenants              map[uint]models.Tenant
	profiles             []models.ShopProfile
	products             []models.SPU
	lastProductQuery     repository.ProductQuery
	platformMallTenantID uint
}

type fakeTranslationReader struct {
	rows      []models.ContentTranslation
	calls     []fakeTranslationCall
	bulkCalls int
}

type fakeTranslationCall struct {
	EntityType string
	EntityIDs  []uint
	Locale     string
}

func (r *fakeTranslationReader) GetTranslations(_ context.Context, entityType string, entityIDs []uint, requestedLocale string) (map[uint]map[string]models.ContentTranslation, error) {
	r.calls = append(r.calls, fakeTranslationCall{
		EntityType: entityType,
		EntityIDs:  append([]uint(nil), entityIDs...),
		Locale:     requestedLocale,
	})

	ids := make(map[uint]struct{}, len(entityIDs))
	for _, id := range entityIDs {
		if id > 0 {
			ids[id] = struct{}{}
		}
	}
	localePriority := []string{locale.NormalizeLocale(requestedLocale)}
	if localePriority[0] != locale.DefaultLocale {
		localePriority = append(localePriority, locale.DefaultLocale)
	}
	priorityByLocale := make(map[string]int, len(localePriority))
	for index, value := range localePriority {
		priorityByLocale[value] = index
	}

	result := make(map[uint]map[string]models.ContentTranslation)
	selectedPriority := make(map[uint]map[string]int)
	for _, row := range r.rows {
		if row.EntityType != entityType {
			continue
		}
		if _, ok := ids[row.EntityID]; !ok {
			continue
		}
		if row.Status != "" && row.Status != models.ContentTranslationStatusPublished {
			continue
		}
		priority, ok := priorityByLocale[row.Locale]
		if !ok || !fakeTranslationHasValue(row) {
			continue
		}
		if result[row.EntityID] == nil {
			result[row.EntityID] = make(map[string]models.ContentTranslation)
			selectedPriority[row.EntityID] = make(map[string]int)
		}
		currentPriority, exists := selectedPriority[row.EntityID][row.FieldKey]
		if !exists || priority < currentPriority {
			result[row.EntityID][row.FieldKey] = row
			selectedPriority[row.EntityID][row.FieldKey] = priority
		}
	}
	return result, nil
}

func (r *fakeTranslationReader) GetTranslationsForEntity(_ context.Context, entityType string, entityID uint, requestedLocale string) (map[string]models.ContentTranslation, error) {
	result := make(map[string]models.ContentTranslation)
	localeValue := locale.NormalizeLocale(requestedLocale)
	for _, row := range r.rows {
		if row.EntityType != entityType || row.EntityID != entityID || row.Locale != localeValue {
			continue
		}
		if row.Status != "" && row.Status != models.ContentTranslationStatusPublished {
			continue
		}
		if fakeTranslationHasValue(row) {
			result[row.FieldKey] = row
		}
	}
	return result, nil
}

func (r *fakeTranslationReader) SaveTranslationsForEntity(_ context.Context, entityType string, entityID uint, requestedLocale string, values map[string]TranslationValueInput) error {
	localeValue := locale.NormalizeLocale(requestedLocale)
	for fieldKey, input := range values {
		valueText := strings.TrimSpace(input.ValueText)
		valueHTML := strings.TrimSpace(input.ValueHTML)
		valueJSON := strings.TrimSpace(string(input.ValueJSON))
		empty := valueText == "" && valueHTML == "" && (valueJSON == "" || valueJSON == "null")
		next := r.rows[:0]
		for _, row := range r.rows {
			if row.EntityType == entityType && row.EntityID == entityID && row.FieldKey == fieldKey && row.Locale == localeValue {
				continue
			}
			next = append(next, row)
		}
		r.rows = next
		if empty {
			continue
		}
		r.rows = append(r.rows, models.ContentTranslation{
			EntityType: entityType,
			EntityID:   entityID,
			FieldKey:   fieldKey,
			Locale:     localeValue,
			ValueText:  valueText,
			ValueHTML:  valueHTML,
			ValueJSON:  input.ValueJSON,
			Status:     models.ContentTranslationStatusPublished,
		})
	}
	return nil
}

func (r *fakeTranslationReader) SaveTextTranslationsForEntities(ctx context.Context, entityType string, fieldKey string, requestedLocale string, values map[uint]string) error {
	r.bulkCalls++
	for entityID, value := range values {
		if err := r.SaveTranslationsForEntity(ctx, entityType, entityID, requestedLocale, map[string]TranslationValueInput{
			fieldKey: {ValueText: value},
		}); err != nil {
			return err
		}
	}
	return nil
}

func fakeTranslationHasValue(row models.ContentTranslation) bool {
	return strings.TrimSpace(row.ValueText) != "" ||
		strings.TrimSpace(row.ValueHTML) != "" ||
		(strings.TrimSpace(string(row.ValueJSON)) != "" && strings.TrimSpace(string(row.ValueJSON)) != "null")
}

func (r *fakeStorefrontRepository) ListActiveCategories(_ context.Context) ([]models.Category, error) {
	return append([]models.Category(nil), r.categories...), nil
}

func (r *fakeStorefrontRepository) ListProducts(_ context.Context, query repository.ProductQuery) ([]models.SPU, int64, error) {
	r.lastProductQuery = query
	items := make([]models.SPU, 0)
	for _, product := range r.products {
		if product.Status != models.SPUStatusOnShelf {
			continue
		}
		if query.TenantID != nil && product.TenantID != *query.TenantID {
			continue
		}
		if query.TenantID == nil && !r.matchesPublicProductSource(product, query) {
			continue
		}
		if query.CategoryID != nil && (product.CategoryID == nil || *product.CategoryID != *query.CategoryID) {
			continue
		}
		if len(query.PlatformCategoryIDs) > 0 {
			if !containsUint(query.PlatformCategoryIDs, product.PlatformCategoryID) {
				continue
			}
		} else if query.PlatformCategoryID != nil && product.PlatformCategoryID != *query.PlatformCategoryID {
			continue
		}
		if keyword := strings.TrimSpace(query.Keyword); keyword != "" && !strings.Contains(strings.ToLower(product.Name), strings.ToLower(keyword)) {
			continue
		}
		items = append(items, cloneProduct(product))
	}

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

func (r *fakeStorefrontRepository) FindProductByID(_ context.Context, id uint) (*models.SPU, error) {
	for _, product := range r.products {
		if product.ID == id && product.Status == models.SPUStatusOnShelf && r.matchesPublicProductSource(product, repository.ProductQuery{Source: repository.ProductSourceAll}) {
			cloned := cloneProduct(product)
			cloned.Tenant = r.tenantForProduct(product)
			return &cloned, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *fakeStorefrontRepository) FindPublicShopProfileByTenantID(_ context.Context, tenantID uint) (*models.ShopProfile, error) {
	for _, profile := range r.profiles {
		if profile.TenantID == tenantID && profile.IsPublic && r.isTenantActive(tenantID) {
			cloned := profile
			return &cloned, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *fakeStorefrontRepository) FindPlatformCategoryByID(_ context.Context, id uint) (*models.PlatformCategory, error) {
	category, ok := r.platformCategories[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	cloned := category
	return &cloned, nil
}

func (r *fakeStorefrontRepository) ListPlatformCategories(_ context.Context, activeOnly bool) ([]models.PlatformCategory, error) {
	items := make([]models.PlatformCategory, 0, len(r.platformCategories))
	for _, category := range r.platformCategories {
		if activeOnly && category.Status != "" && category.Status != models.PlatformCategoryStatusActive {
			continue
		}
		items = append(items, category)
	}
	return items, nil
}

func (r *fakeStorefrontRepository) ListPlatformCategoryIDsByRoot(_ context.Context, id uint) ([]uint, error) {
	category, ok := r.platformCategories[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	if category.IsLeaf {
		return []uint{id}, nil
	}

	ids := []uint{id}
	prefix := category.Path + "/"
	for _, candidate := range r.platformCategories {
		if candidate.ID != id && strings.HasPrefix(candidate.Path, prefix) {
			ids = append(ids, candidate.ID)
		}
	}
	return ids, nil
}

func (r *fakeStorefrontRepository) FindPublicShopProfileBySlug(_ context.Context, slug string) (*models.ShopProfile, error) {
	for _, profile := range r.profiles {
		if profile.Slug == slug && profile.IsPublic && r.isTenantActive(profile.TenantID) {
			cloned := profile
			return &cloned, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *fakeStorefrontRepository) ListPublicShopProfiles(_ context.Context, query repository.PublicShopProfileQuery) ([]models.ShopProfile, int64, error) {
	items := make([]models.ShopProfile, 0)
	for _, profile := range r.profiles {
		if profile.IsPublic && r.isTenantActive(profile.TenantID) {
			items = append(items, profile)
		}
	}

	total := int64(len(items))
	start := (query.Page - 1) * query.PageSize
	if start >= len(items) {
		return []models.ShopProfile{}, total, nil
	}
	end := start + query.PageSize
	if end > len(items) {
		end = len(items)
	}
	return append([]models.ShopProfile(nil), items[start:end]...), total, nil
}

func (r *fakeStorefrontRepository) ListSitemapShops(_ context.Context) ([]repository.SitemapShopURL, error) {
	items := make([]repository.SitemapShopURL, 0)
	for _, profile := range r.profiles {
		if profile.IsPublic && strings.TrimSpace(profile.Slug) != "" && r.isTenantActive(profile.TenantID) {
			items = append(items, repository.SitemapShopURL{Slug: profile.Slug, UpdatedAt: profile.UpdatedAt})
		}
	}
	return items, nil
}

func (r *fakeStorefrontRepository) ListSitemapProducts(_ context.Context) ([]repository.SitemapProductURL, error) {
	items := make([]repository.SitemapProductURL, 0)
	for _, product := range r.products {
		if product.Status != models.SPUStatusOnShelf {
			continue
		}
		if !r.matchesPublicProductSource(product, repository.ProductQuery{Source: repository.ProductSourceAll}) {
			continue
		}
		items = append(items, repository.SitemapProductURL{ID: product.ID, Name: product.Name, UpdatedAt: product.UpdatedAt})
	}
	return items, nil
}

func (r *fakeStorefrontRepository) matchesPublicProductSource(product models.SPU, query repository.ProductQuery) bool {
	tenant := r.tenantForProduct(product)
	if tenant.Status != models.TenantStatusActive {
		return false
	}

	source := query.Source
	if source == "" || source == repository.ProductSourceAll {
		if query.IsSelfOperated {
			source = repository.ProductSourceT6
		} else {
			source = repository.ProductSourceAll
		}
	}

	isT6 := models.IsPlatformMallTenantWithID(tenant, r.platformMallTenantID)
	hasPublicShop := r.hasPublicShopProfile(product.TenantID)
	switch source {
	case repository.ProductSourceT6:
		return isT6
	case repository.ProductSourceMerchant:
		return !isT6 && hasPublicShop
	default:
		return isT6 || hasPublicShop
	}
}

func (r *fakeStorefrontRepository) tenantForProduct(product models.SPU) models.Tenant {
	if tenant, ok := r.tenants[product.TenantID]; ok {
		return tenant
	}
	if product.Tenant.ID != 0 || product.Tenant.Name != "" || product.Tenant.Type != "" || product.Tenant.Status != "" {
		tenant := product.Tenant
		if tenant.ID == 0 {
			tenant.ID = product.TenantID
		}
		if tenant.Status == "" {
			tenant.Status = models.TenantStatusActive
		}
		return tenant
	}
	return models.Tenant{
		BaseModel: models.BaseModel{ID: product.TenantID},
		Type:      models.TenantTypeThirdParty,
		Status:    models.TenantStatusActive,
	}
}

func (r *fakeStorefrontRepository) isTenantActive(tenantID uint) bool {
	tenant, ok := r.tenants[tenantID]
	return !ok || tenant.Status == "" || tenant.Status == models.TenantStatusActive
}

func (r *fakeStorefrontRepository) hasPublicShopProfile(tenantID uint) bool {
	for _, profile := range r.profiles {
		if profile.TenantID == tenantID && profile.IsPublic && r.isTenantActive(tenantID) {
			return true
		}
	}
	return false
}

func TestStorefrontServiceShopProductsAreScopedBySlugTenant(t *testing.T) {
	tenantAID := uint(1001)
	tenantBID := uint(2002)
	service := NewStorefrontService(&fakeStorefrontRepository{
		profiles: []models.ShopProfile{
			{TenantID: tenantAID, Slug: "ghana-market", ShopName: "Ghana Market", IsPublic: true},
			{TenantID: tenantBID, Slug: "lagos-shop", ShopName: "Lagos Shop", IsPublic: true},
		},
		products: []models.SPU{
			{
				BaseModel:  models.BaseModel{ID: 1},
				TenantID:   tenantAID,
				CategoryID: uintPtr(10),
				Name:       "Tenant A Product",
				Status:     models.SPUStatusOnShelf,
				SKUs:       []models.SKU{{RetailPrice: 1000, IsActive: true}},
			},
			{
				BaseModel:  models.BaseModel{ID: 2},
				TenantID:   tenantBID,
				CategoryID: uintPtr(10),
				Name:       "Tenant B Product",
				Status:     models.SPUStatusOnShelf,
				SKUs:       []models.SKU{{RetailPrice: 2000, IsActive: true}},
			},
			{
				BaseModel:  models.BaseModel{ID: 3},
				TenantID:   tenantAID,
				CategoryID: uintPtr(10),
				Name:       "Tenant A Off Shelf Product",
				Status:     models.SPUStatusOffShelf,
				SKUs:       []models.SKU{{RetailPrice: 3000, IsActive: true}},
			},
		},
	})

	page, err := service.ShopProducts(context.Background(), "ghana-market", StorefrontProductQuery{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ShopProducts returned error: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("page = %+v, want exactly one tenant A product", page)
	}
	if page.Items[0].ID != 1 {
		t.Fatalf("product = %+v, want tenant A product", page.Items[0])
	}
}

func TestStorefrontServiceProductsFilterByPlatformCategoryAndLimit(t *testing.T) {
	phoneCategoryID := uint(5001)
	computerCategoryID := uint(5002)
	repo := &fakeStorefrontRepository{
		profiles: []models.ShopProfile{
			{TenantID: 1001, Slug: "phone-shop", ShopName: "Phone Shop", IsPublic: true},
		},
		products: []models.SPU{
			{
				BaseModel:          models.BaseModel{ID: 1},
				TenantID:           1001,
				PlatformCategoryID: phoneCategoryID,
				Name:               "Phone Screen",
				Status:             models.SPUStatusOnShelf,
				SKUs:               []models.SKU{{RetailPrice: 1200, IsActive: true}},
			},
			{
				BaseModel:          models.BaseModel{ID: 2},
				TenantID:           1001,
				PlatformCategoryID: phoneCategoryID,
				Name:               "Phone Battery",
				Status:             models.SPUStatusOnShelf,
				SKUs:               []models.SKU{{RetailPrice: 900, IsActive: true}},
			},
			{
				BaseModel:          models.BaseModel{ID: 3},
				TenantID:           1001,
				PlatformCategoryID: phoneCategoryID,
				Name:               "Hidden Phone",
				Status:             models.SPUStatusOffShelf,
				SKUs:               []models.SKU{{RetailPrice: 1000, IsActive: true}},
			},
			{
				BaseModel:          models.BaseModel{ID: 4},
				TenantID:           1001,
				PlatformCategoryID: computerCategoryID,
				Name:               "Laptop Bag",
				Status:             models.SPUStatusOnShelf,
				SKUs:               []models.SKU{{RetailPrice: 3000, IsActive: true}},
			},
		},
	}
	service := NewStorefrontService(repo)

	page, err := service.Products(context.Background(), StorefrontProductQuery{
		PlatformCategoryID: &phoneCategoryID,
		Sort:               "hot",
		Page:               1,
		PageSize:           1,
	})
	if err != nil {
		t.Fatalf("Products returned error: %v", err)
	}
	if page.Total != 2 || len(page.Items) != 1 || page.Items[0].PlatformCategoryID != phoneCategoryID {
		t.Fatalf("page = %+v, want one visible phone item with total 2", page)
	}
	payload, err := json.Marshal(page.Items[0])
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	if strings.Contains(string(payload), "tenant_id") {
		t.Fatalf("product list payload leaked tenant_id: %s", string(payload))
	}
	if repo.lastProductQuery.Sort != "hot" {
		t.Fatalf("repo sort = %q, want hot", repo.lastProductQuery.Sort)
	}
}

func TestStorefrontServiceProductsApplyLocalizedContentInBatch(t *testing.T) {
	translations := &fakeTranslationReader{
		rows: []models.ContentTranslation{
			{EntityType: TranslationEntityProduct, EntityID: 1, FieldKey: TranslationFieldName, Locale: "zh", ValueText: "中文手机"},
			{EntityType: TranslationEntityProduct, EntityID: 1, FieldKey: TranslationFieldDescription, Locale: "zh", ValueText: "中文描述"},
			{EntityType: TranslationEntityProduct, EntityID: 3, FieldKey: TranslationFieldName, Locale: "en", ValueText: "English fallback"},
		},
	}
	service := NewStorefrontService(&fakeStorefrontRepository{
		profiles: []models.ShopProfile{
			{TenantID: 1001, Slug: "phone-shop", ShopName: "Phone Shop", IsPublic: true},
		},
		products: []models.SPU{
			{BaseModel: models.BaseModel{ID: 1}, TenantID: 1001, Name: "Phone", Description: "Original description", Status: models.SPUStatusOnShelf},
			{BaseModel: models.BaseModel{ID: 2}, TenantID: 1001, Name: "Charger", Description: "Main charger", Status: models.SPUStatusOnShelf},
			{BaseModel: models.BaseModel{ID: 3}, TenantID: 1001, Name: "Cable", Status: models.SPUStatusOnShelf},
		},
	}, StorefrontOptions{Translations: translations})

	page, err := service.Products(context.Background(), StorefrontProductQuery{Locale: "zh", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("Products returned error: %v", err)
	}
	if page.Items[0].Name != "中文手机" || page.Items[0].Description != "中文描述" {
		t.Fatalf("localized first item = %+v, want zh name and description", page.Items[0])
	}
	if page.Items[1].Name != "Charger" || page.Items[1].Description != "Main charger" {
		t.Fatalf("fallback item = %+v, want main table fallback", page.Items[1])
	}
	if page.Items[2].Name != "English fallback" {
		t.Fatalf("en fallback item = %+v, want English fallback", page.Items[2])
	}
	productCalls := 0
	for _, call := range translations.calls {
		if call.EntityType == TranslationEntityProduct {
			productCalls++
		}
	}
	if productCalls != 1 {
		t.Fatalf("product translation calls = %d, want one batch call", productCalls)
	}
}

func TestStorefrontServiceProductsBadLocaleFallsBackToEnglishTranslation(t *testing.T) {
	service := NewStorefrontService(&fakeStorefrontRepository{
		profiles: []models.ShopProfile{
			{TenantID: 1001, Slug: "phone-shop", ShopName: "Phone Shop", IsPublic: true},
		},
		products: []models.SPU{
			{BaseModel: models.BaseModel{ID: 1}, TenantID: 1001, Name: "Original Phone", Status: models.SPUStatusOnShelf},
		},
	}, StorefrontOptions{Translations: &fakeTranslationReader{
		rows: []models.ContentTranslation{
			{EntityType: TranslationEntityProduct, EntityID: 1, FieldKey: TranslationFieldName, Locale: "en", ValueText: "English Phone"},
			{EntityType: TranslationEntityProduct, EntityID: 1, FieldKey: TranslationFieldName, Locale: "zh", ValueText: "中文手机"},
		},
	}})

	page, err := service.Products(context.Background(), StorefrontProductQuery{Locale: "bad", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("Products returned error: %v", err)
	}
	if page.Items[0].Name != "English Phone" {
		t.Fatalf("bad locale product name = %q, want English fallback", page.Items[0].Name)
	}
}

func TestStorefrontServiceProductsFilterByParentPlatformCategory(t *testing.T) {
	parentID := uint(5001)
	screenID := uint(5002)
	batteryID := uint(5003)
	computerID := uint(6001)
	repo := &fakeStorefrontRepository{
		profiles: []models.ShopProfile{
			{TenantID: 1001, Slug: "parts-shop", ShopName: "Parts Shop", IsPublic: true},
		},
		platformCategories: map[uint]models.PlatformCategory{
			parentID: {
				BaseModel: models.BaseModel{ID: parentID},
				Name:      "Phone Parts",
				Path:      "Phone Parts",
				IsLeaf:    false,
			},
			screenID: {
				BaseModel: models.BaseModel{ID: screenID},
				ParentID:  &parentID,
				Name:      "Screens",
				Path:      "Phone Parts/Screens",
				IsLeaf:    true,
			},
			batteryID: {
				BaseModel: models.BaseModel{ID: batteryID},
				ParentID:  &parentID,
				Name:      "Batteries",
				Path:      "Phone Parts/Batteries",
				IsLeaf:    true,
			},
			computerID: {
				BaseModel: models.BaseModel{ID: computerID},
				Name:      "Computer",
				Path:      "Computer",
				IsLeaf:    true,
			},
		},
		products: []models.SPU{
			{
				BaseModel:          models.BaseModel{ID: 1},
				TenantID:           1001,
				PlatformCategoryID: screenID,
				Name:               "Phone Screen",
				Status:             models.SPUStatusOnShelf,
				SKUs:               []models.SKU{{RetailPrice: 1200, IsActive: true}},
			},
			{
				BaseModel:          models.BaseModel{ID: 2},
				TenantID:           1001,
				PlatformCategoryID: batteryID,
				Name:               "Phone Battery",
				Status:             models.SPUStatusOnShelf,
				SKUs:               []models.SKU{{RetailPrice: 900, IsActive: true}},
			},
			{
				BaseModel:          models.BaseModel{ID: 3},
				TenantID:           1001,
				PlatformCategoryID: computerID,
				Name:               "Laptop Bag",
				Status:             models.SPUStatusOnShelf,
				SKUs:               []models.SKU{{RetailPrice: 3000, IsActive: true}},
			},
		},
	}
	service := NewStorefrontService(repo)

	parentPage, err := service.Products(context.Background(), StorefrontProductQuery{
		PlatformCategoryID: &parentID,
		Page:               1,
		PageSize:           20,
	})
	if err != nil {
		t.Fatalf("Products parent category returned error: %v", err)
	}
	if parentPage.Total != 2 || len(parentPage.Items) != 2 {
		t.Fatalf("parent page = %+v, want two child category products", parentPage)
	}
	if !containsUint(repo.lastProductQuery.PlatformCategoryIDs, screenID) || !containsUint(repo.lastProductQuery.PlatformCategoryIDs, batteryID) {
		t.Fatalf("expanded category IDs = %+v, want child IDs", repo.lastProductQuery.PlatformCategoryIDs)
	}

	leafPage, err := service.Products(context.Background(), StorefrontProductQuery{
		PlatformCategoryID: &screenID,
		Page:               1,
		PageSize:           20,
	})
	if err != nil {
		t.Fatalf("Products leaf category returned error: %v", err)
	}
	if leafPage.Total != 1 || len(leafPage.Items) != 1 || leafPage.Items[0].ID != 1 {
		t.Fatalf("leaf page = %+v, want screen product only", leafPage)
	}

	keywordPage, err := service.Products(context.Background(), StorefrontProductQuery{
		PlatformCategoryID: &parentID,
		Keyword:            "battery",
		Page:               1,
		PageSize:           20,
	})
	if err != nil {
		t.Fatalf("Products keyword parent category returned error: %v", err)
	}
	if keywordPage.Total != 1 || len(keywordPage.Items) != 1 || keywordPage.Items[0].ID != 2 {
		t.Fatalf("keyword parent page = %+v, want battery product only", keywordPage)
	}
}

func TestStorefrontServiceProductsFilterBySecondLevelIncludesDeepDescendantsAndSource(t *testing.T) {
	rootID := uint(7001)
	secondLevelID := uint(7002)
	leafID := uint(7003)
	otherLeafID := uint(7004)
	t6TenantID := uint(1001)
	merchantTenantID := uint(2002)
	hiddenMerchantTenantID := uint(3003)
	repo := &fakeStorefrontRepository{
		platformMallTenantID: t6TenantID,
		tenants: map[uint]models.Tenant{
			t6TenantID: {
				BaseModel: models.BaseModel{ID: t6TenantID},
				Name:      "T6",
				Type:      models.TenantTypeSelfOperated,
				Status:    models.TenantStatusActive,
			},
			merchantTenantID: {
				BaseModel: models.BaseModel{ID: merchantTenantID},
				Name:      "Public Vendor",
				Type:      models.TenantTypeThirdParty,
				Status:    models.TenantStatusActive,
			},
			hiddenMerchantTenantID: {
				BaseModel: models.BaseModel{ID: hiddenMerchantTenantID},
				Name:      "Hidden Vendor",
				Type:      models.TenantTypeThirdParty,
				Status:    models.TenantStatusActive,
			},
		},
		profiles: []models.ShopProfile{
			{TenantID: merchantTenantID, Slug: "public-vendor", ShopName: "Public Vendor", IsPublic: true},
			{TenantID: hiddenMerchantTenantID, Slug: "hidden-vendor", ShopName: "Hidden Vendor", IsPublic: false},
		},
		platformCategories: map[uint]models.PlatformCategory{
			rootID: {
				BaseModel: models.BaseModel{ID: rootID},
				Name:      "Electronics",
				Path:      "Electronics",
				IsLeaf:    false,
			},
			secondLevelID: {
				BaseModel: models.BaseModel{ID: secondLevelID},
				ParentID:  &rootID,
				Name:      "Mobile Accessories",
				Path:      "Electronics/Mobile Accessories",
				IsLeaf:    false,
			},
			leafID: {
				BaseModel: models.BaseModel{ID: leafID},
				ParentID:  &secondLevelID,
				Name:      "Phone Cases",
				Path:      "Electronics/Mobile Accessories/Phone Cases",
				IsLeaf:    true,
			},
			otherLeafID: {
				BaseModel: models.BaseModel{ID: otherLeafID},
				ParentID:  &rootID,
				Name:      "Laptops",
				Path:      "Electronics/Laptops",
				IsLeaf:    true,
			},
		},
		products: []models.SPU{
			{BaseModel: models.BaseModel{ID: 1}, TenantID: t6TenantID, PlatformCategoryID: leafID, Name: "T6 Phone Case", Status: models.SPUStatusOnShelf},
			{BaseModel: models.BaseModel{ID: 2}, TenantID: merchantTenantID, PlatformCategoryID: leafID, Name: "Vendor Phone Case", Status: models.SPUStatusOnShelf},
			{BaseModel: models.BaseModel{ID: 3}, TenantID: hiddenMerchantTenantID, PlatformCategoryID: leafID, Name: "Hidden Phone Case", Status: models.SPUStatusOnShelf},
			{BaseModel: models.BaseModel{ID: 4}, TenantID: t6TenantID, PlatformCategoryID: otherLeafID, Name: "T6 Laptop", Status: models.SPUStatusOnShelf},
		},
	}
	service := NewStorefrontService(repo)

	t6Page, err := service.Products(context.Background(), StorefrontProductQuery{
		Source:             "t6",
		PlatformCategoryID: &secondLevelID,
		Page:               1,
		PageSize:           20,
	})
	if err != nil {
		t.Fatalf("Products t6 second-level category returned error: %v", err)
	}
	if got := productIDs(t6Page.Items); !sameUintSet(got, []uint{1}) {
		t.Fatalf("t6 second-level product IDs = %+v, want deep descendant product 1", got)
	}

	merchantPage, err := service.Products(context.Background(), StorefrontProductQuery{
		Source:             "merchant",
		PlatformCategoryID: &secondLevelID,
		Page:               1,
		PageSize:           20,
	})
	if err != nil {
		t.Fatalf("Products merchant second-level category returned error: %v", err)
	}
	if got := productIDs(merchantPage.Items); !sameUintSet(got, []uint{2}) {
		t.Fatalf("merchant second-level product IDs = %+v, want public merchant deep descendant product 2", got)
	}
	if !containsUint(repo.lastProductQuery.PlatformCategoryIDs, secondLevelID) || !containsUint(repo.lastProductQuery.PlatformCategoryIDs, leafID) {
		t.Fatalf("expanded second-level category IDs = %+v, want parent and deep leaf", repo.lastProductQuery.PlatformCategoryIDs)
	}
}

func TestStorefrontServiceProductsFilterBySource(t *testing.T) {
	t6TenantID := uint(100)
	otherSelfOperatedTenantID := uint(101)
	merchantTenantID := uint(200)
	hiddenMerchantTenantID := uint(201)
	phoneCategoryID := uint(5001)
	toolsCategoryID := uint(5002)

	repo := &fakeStorefrontRepository{
		tenants: map[uint]models.Tenant{
			t6TenantID: {
				BaseModel: models.BaseModel{ID: t6TenantID},
				Name:      models.PlatformMallTenantName,
				Type:      models.TenantTypeSelfOperated,
				Status:    models.TenantStatusActive,
			},
			otherSelfOperatedTenantID: {
				BaseModel: models.BaseModel{ID: otherSelfOperatedTenantID},
				Name:      "YunFei Store",
				Type:      models.TenantTypeSelfOperated,
				Status:    models.TenantStatusActive,
			},
			merchantTenantID: {
				BaseModel: models.BaseModel{ID: merchantTenantID},
				Name:      "Vendor Store",
				Type:      models.TenantTypeThirdParty,
				Status:    models.TenantStatusActive,
			},
			hiddenMerchantTenantID: {
				BaseModel: models.BaseModel{ID: hiddenMerchantTenantID},
				Name:      "Hidden Vendor",
				Type:      models.TenantTypeThirdParty,
				Status:    models.TenantStatusActive,
			},
		},
		profiles: []models.ShopProfile{
			{TenantID: otherSelfOperatedTenantID, Slug: "yunfei-store", ShopName: "YunFei Store", IsPublic: true},
			{TenantID: merchantTenantID, Slug: "vendor-store", ShopName: "Vendor Store", IsPublic: true},
			{TenantID: hiddenMerchantTenantID, Slug: "hidden-vendor", ShopName: "Hidden Vendor", IsPublic: false},
		},
		products: []models.SPU{
			{BaseModel: models.BaseModel{ID: 1}, TenantID: t6TenantID, PlatformCategoryID: phoneCategoryID, Name: "T6 Phone", Status: models.SPUStatusOnShelf},
			{BaseModel: models.BaseModel{ID: 2}, TenantID: merchantTenantID, PlatformCategoryID: phoneCategoryID, Name: "Vendor Phone", Status: models.SPUStatusOnShelf},
			{BaseModel: models.BaseModel{ID: 3}, TenantID: hiddenMerchantTenantID, PlatformCategoryID: phoneCategoryID, Name: "Hidden Vendor Phone", Status: models.SPUStatusOnShelf},
			{BaseModel: models.BaseModel{ID: 4}, TenantID: t6TenantID, PlatformCategoryID: toolsCategoryID, Name: "T6 Tool", Status: models.SPUStatusOnShelf},
			{BaseModel: models.BaseModel{ID: 5}, TenantID: otherSelfOperatedTenantID, PlatformCategoryID: toolsCategoryID, Name: "YunFei Tool", Status: models.SPUStatusOnShelf},
		},
	}
	service := NewStorefrontService(repo)

	allPage, err := service.Products(context.Background(), StorefrontProductQuery{Source: "all", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("Products all source returned error: %v", err)
	}
	if got := productIDs(allPage.Items); !sameUintSet(got, []uint{1, 2, 4, 5}) {
		t.Fatalf("all source product IDs = %+v, want T6 plus public shop products", got)
	}

	t6Page, err := service.Products(context.Background(), StorefrontProductQuery{Source: "t6", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("Products t6 source returned error: %v", err)
	}
	if got := productIDs(t6Page.Items); !sameUintSet(got, []uint{1, 4}) {
		t.Fatalf("t6 source product IDs = %+v, want official T6 products only", got)
	}

	merchantPage, err := service.Products(context.Background(), StorefrontProductQuery{Source: "merchant", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("Products merchant source returned error: %v", err)
	}
	if got := productIDs(merchantPage.Items); !sameUintSet(got, []uint{2, 5}) {
		t.Fatalf("merchant source product IDs = %+v, want public non-T6 products only", got)
	}

	t6PhonePage, err := service.Products(context.Background(), StorefrontProductQuery{
		Source:             "t6",
		PlatformCategoryID: &phoneCategoryID,
		Page:               1,
		PageSize:           20,
	})
	if err != nil {
		t.Fatalf("Products t6 category source returned error: %v", err)
	}
	if got := productIDs(t6PhonePage.Items); !sameUintSet(got, []uint{1}) {
		t.Fatalf("t6 phone product IDs = %+v, want T6 phone only", got)
	}

	merchantKeywordPage, err := service.Products(context.Background(), StorefrontProductQuery{
		Source:   "merchant",
		Keyword:  "vendor",
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("Products merchant keyword source returned error: %v", err)
	}
	if got := productIDs(merchantKeywordPage.Items); !sameUintSet(got, []uint{2}) {
		t.Fatalf("merchant keyword product IDs = %+v, want public vendor product only", got)
	}
}

func TestStorefrontServiceCategoryTreeAppliesLocalizedNames(t *testing.T) {
	rootID := uint(10)
	childID := uint(11)
	service := NewStorefrontService(&fakeStorefrontRepository{
		categories: []models.Category{
			{BaseModel: models.BaseModel{ID: rootID}, Name: "Electronics", IsActive: true},
			{BaseModel: models.BaseModel{ID: childID}, ParentID: &rootID, Name: "Phones", IsActive: true},
		},
	}, StorefrontOptions{Translations: &fakeTranslationReader{
		rows: []models.ContentTranslation{
			{EntityType: TranslationEntityCategory, EntityID: rootID, FieldKey: TranslationFieldName, Locale: "zh", ValueText: "电子产品"},
			{EntityType: TranslationEntityCategory, EntityID: childID, FieldKey: TranslationFieldName, Locale: "en", ValueText: "Mobile Phones"},
		},
	}})

	tree, err := service.CategoryTreeWithLocale(context.Background(), "zh")
	if err != nil {
		t.Fatalf("CategoryTreeWithLocale returned error: %v", err)
	}
	if len(tree) != 1 || tree[0].Name != "电子产品" {
		t.Fatalf("tree = %+v, want localized root", tree)
	}
	if len(tree[0].Children) != 1 || tree[0].Children[0].Name != "Mobile Phones" {
		t.Fatalf("children = %+v, want en fallback child", tree[0].Children)
	}
}

func TestNormalizeProductSortPreservesSupportedPriceSorts(t *testing.T) {
	tests := map[string]string{
		"price_asc":  "price_asc",
		"PRICE_DESC": "price_desc",
		"hot":        "hot",
		"unknown":    "latest",
		"":           "latest",
	}

	for input, want := range tests {
		if got := normalizeProductSort(input, true); got != want {
			t.Fatalf("normalizeProductSort(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeProductSortHidesPriceSortsWhenPriceHidden(t *testing.T) {
	for _, input := range []string{"price_asc", "PRICE_DESC"} {
		if got := normalizeProductSort(input, false); got != "latest" {
			t.Fatalf("normalizeProductSort(%q, false) = %q, want latest", input, got)
		}
	}
	if got := normalizeProductSort("hot", false); got != "hot" {
		t.Fatalf("normalizeProductSort(hot, false) = %q, want hot", got)
	}
}

func TestStorefrontServiceProductDetailReturnsSpecsSkusAndAggregates(t *testing.T) {
	service := NewStorefrontService(&fakeStorefrontRepository{
		profiles: []models.ShopProfile{
			{TenantID: 1001, Slug: "dress-shop", ShopName: "Dress Shop", IsPublic: true},
		},
		platformCategories: map[uint]models.PlatformCategory{
			5001: {
				BaseModel: models.BaseModel{ID: 5001},
				Name:      "连衣裙",
				Path:      "女装/连衣裙",
			},
		},
		products: []models.SPU{
			{
				BaseModel:          models.BaseModel{ID: 10},
				TenantID:           1001,
				PlatformCategoryID: 5001,
				Name:               "Summer Dress",
				Description:        "Light fabric",
				DetailHTML:         `<p>Long detail</p><img src="/uploads/detail.webp">`,
				MainImage:          "/uploads/main.webp",
				VideoURL:           "/uploads/videos/2026/06/product.mp4",
				Material:           "Cotton",
				Dimensions:         "30 × 20 × 10 cm",
				Weight:             "1.2 kg",
				SliderImages:       jsonValue(`["/uploads/side.webp"]`),
				SpecTemplate:       jsonValue(`[{"name":"Size","values":["S","M"]},{"name":"Color","values":["Blue","Green"]}]`),
				Status:             models.SPUStatusOnShelf,
				SKUs: []models.SKU{
					{
						BaseModel:     models.BaseModel{ID: 101},
						ProductID:     10,
						Name:          "S / Blue",
						ERPSKUCode:    "SKU-1001-001",
						ImageURL:      "/uploads/sku-blue.webp",
						Specs:         jsonValue(`{"Size":"S","Color":"Blue"}`),
						RetailPrice:   8000,
						OriginalPrice: 10000,
						SellableStock: 3,
						IsActive:      true,
					},
					{
						BaseModel:     models.BaseModel{ID: 102},
						ProductID:     10,
						Name:          "M / Green",
						ERPSKUCode:    "SKU-1001-002",
						Specs:         jsonValue(`{"Size":"M","Color":"Green"}`),
						RetailPrice:   9000,
						OriginalPrice: 11000,
						SellableStock: 2,
						IsActive:      true,
					},
				},
			},
		},
	})

	detail, err := service.ProductDetail(context.Background(), 10)
	if err != nil {
		t.Fatalf("ProductDetail returned error: %v", err)
	}
	if len(detail.SKUs) != 2 {
		t.Fatalf("skus = %+v, want 2", detail.SKUs)
	}
	if detail.SKUs[0].ImageURL != "/uploads/sku-blue.webp" {
		t.Fatalf("sku image_url = %q", detail.SKUs[0].ImageURL)
	}
	if !strings.Contains(string(detail.Specs), "Size") || !strings.Contains(string(detail.Specs), "Color") {
		t.Fatalf("specs = %s, want Size and Color dimensions", string(detail.Specs))
	}
	if detail.MinPrice != 8000 || detail.MaxPrice != 9000 || detail.OriginalPrice != 10000 {
		t.Fatalf("prices = min %d max %d original %d, want 8000/9000/10000", detail.MinPrice, detail.MaxPrice, detail.OriginalPrice)
	}
	if detail.TotalStock != 5 {
		t.Fatalf("total_stock = %d, want 5", detail.TotalStock)
	}
	if detail.DetailHTML != `<p>Long detail</p><img src="/uploads/detail.webp">` {
		t.Fatalf("detail_html = %q, want storefront detail HTML", detail.DetailHTML)
	}
	if detail.VideoURL != "/uploads/videos/2026/06/product.mp4" {
		t.Fatalf("video_url = %q", detail.VideoURL)
	}
	if detail.Material != "Cotton" || detail.Dimensions != "30 × 20 × 10 cm" || detail.Weight != "1.2 kg" {
		t.Fatalf("product parameters = %q / %q / %q", detail.Material, detail.Dimensions, detail.Weight)
	}
	if detail.PlatformCategory == nil || detail.PlatformCategory.Path != "女装/连衣裙" {
		t.Fatalf("platform_category = %+v, want path", detail.PlatformCategory)
	}
	if detail.Source != string(repository.ProductSourceMerchant) || detail.ShopName != "Dress Shop" || detail.ShopSlug != "dress-shop" {
		t.Fatalf("product source/shop = %q/%q/%q, want merchant Dress Shop", detail.Source, detail.ShopName, detail.ShopSlug)
	}
	if len(detail.Images) != 2 || detail.Images[0] != "/uploads/main.webp" || detail.Images[1] != "/uploads/side.webp" {
		t.Fatalf("images = %+v, want main plus slider image", detail.Images)
	}
	payload, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	if strings.Contains(string(payload), "tenant_id") {
		t.Fatalf("detail payload = %s, should not expose tenant_id", string(payload))
	}
}

func TestStorefrontServiceProductDetailAppliesLocalizedTextAndHTML(t *testing.T) {
	platformCategoryID := uint(5001)
	parentCategoryID := uint(5000)
	service := NewStorefrontService(&fakeStorefrontRepository{
		profiles: []models.ShopProfile{
			{BaseModel: models.BaseModel{ID: 701}, TenantID: 1001, Slug: "dress-shop", ShopName: "Dress Shop", IsPublic: true},
		},
		platformCategories: map[uint]models.PlatformCategory{
			parentCategoryID: {
				BaseModel: models.BaseModel{ID: parentCategoryID},
				Name:      "Fashion",
				Path:      "Fashion",
				IsLeaf:    false,
				Status:    models.PlatformCategoryStatusActive,
			},
			platformCategoryID: {
				BaseModel: models.BaseModel{ID: platformCategoryID},
				ParentID:  &parentCategoryID,
				Name:      "Dresses",
				Path:      "Fashion/Dresses",
				Level:     2,
				IsLeaf:    true,
				Status:    models.PlatformCategoryStatusActive,
			},
		},
		products: []models.SPU{
			{
				BaseModel:          models.BaseModel{ID: 10},
				TenantID:           1001,
				PlatformCategoryID: platformCategoryID,
				Name:               "Summer Dress",
				Description:        "Light fabric",
				DetailHTML:         `<p>Original detail</p>`,
				Material:           "Cotton",
				Dimensions:         "30 x 20 x 10 cm",
				Weight:             "1.2 kg",
				Status:             models.SPUStatusOnShelf,
				SKUs:               []models.SKU{{BaseModel: models.BaseModel{ID: 101}, ProductID: 10, Name: "S / Blue", RetailPrice: 8000, SellableStock: 3, IsActive: true}},
			},
		},
	}, StorefrontOptions{Translations: &fakeTranslationReader{
		rows: []models.ContentTranslation{
			{EntityType: TranslationEntityProduct, EntityID: 10, FieldKey: TranslationFieldName, Locale: "zh", ValueText: "夏日连衣裙"},
			{EntityType: TranslationEntityProduct, EntityID: 10, FieldKey: TranslationFieldDescription, Locale: "zh", ValueText: "轻薄面料"},
			{EntityType: TranslationEntityProduct, EntityID: 10, FieldKey: TranslationFieldDetailHTML, Locale: "zh", ValueHTML: `<p>中文详情</p>`},
			{EntityType: TranslationEntityProduct, EntityID: 10, FieldKey: TranslationFieldMaterial, Locale: "zh", ValueText: "棉"},
			{EntityType: TranslationEntityProduct, EntityID: 10, FieldKey: TranslationFieldDimensions, Locale: "zh", ValueText: "30 x 20 x 10 厘米"},
			{EntityType: TranslationEntityProduct, EntityID: 10, FieldKey: TranslationFieldWeight, Locale: "zh", ValueText: " "},
			{EntityType: TranslationEntityShopProfile, EntityID: 701, FieldKey: TranslationFieldShopName, Locale: "zh", ValueText: "连衣裙店"},
			{EntityType: TranslationEntityPlatformCategory, EntityID: parentCategoryID, FieldKey: TranslationFieldName, Locale: "zh", ValueText: "时尚"},
			{EntityType: TranslationEntityPlatformCategory, EntityID: platformCategoryID, FieldKey: TranslationFieldName, Locale: "zh", ValueText: "连衣裙"},
		},
	}})

	detail, err := service.ProductDetailWithLocale(context.Background(), 10, "zh")
	if err != nil {
		t.Fatalf("ProductDetailWithLocale returned error: %v", err)
	}
	if detail.Name != "夏日连衣裙" || detail.Description != "轻薄面料" || detail.DetailHTML != `<p>中文详情</p>` {
		t.Fatalf("localized product detail = %+v, want zh text and HTML", detail)
	}
	if detail.Material != "棉" || detail.Dimensions != "30 x 20 x 10 厘米" || detail.Weight != "1.2 kg" {
		t.Fatalf("localized params = %q/%q/%q, want zh material/dimensions and original weight fallback", detail.Material, detail.Dimensions, detail.Weight)
	}
	if detail.ShopName != "连衣裙店" {
		t.Fatalf("shop_name = %q, want localized shop name", detail.ShopName)
	}
	if detail.PlatformCategory == nil || detail.PlatformCategory.Path != "时尚/连衣裙" {
		t.Fatalf("platform_category = %+v, want localized path", detail.PlatformCategory)
	}
	if detail.SKUs[0].Name != "S / Blue" || detail.SKUs[0].ID != 101 {
		t.Fatalf("sku changed by translation: %+v", detail.SKUs[0])
	}
}

func TestStorefrontServiceProductDetailEmptyHTMLTranslationFallsBackToEnglishThenOriginal(t *testing.T) {
	service := NewStorefrontService(&fakeStorefrontRepository{
		profiles: []models.ShopProfile{
			{TenantID: 1001, Slug: "detail-shop", ShopName: "Detail Shop", IsPublic: true},
		},
		products: []models.SPU{
			{
				BaseModel:  models.BaseModel{ID: 11},
				TenantID:   1001,
				Name:       "Original Product",
				DetailHTML: `<p>Original detail</p>`,
				Status:     models.SPUStatusOnShelf,
			},
			{
				BaseModel:  models.BaseModel{ID: 12},
				TenantID:   1001,
				Name:       "Original Product 2",
				DetailHTML: `<p>Original detail 2</p>`,
				Status:     models.SPUStatusOnShelf,
			},
		},
	}, StorefrontOptions{Translations: &fakeTranslationReader{
		rows: []models.ContentTranslation{
			{EntityType: TranslationEntityProduct, EntityID: 11, FieldKey: TranslationFieldDetailHTML, Locale: "zh", ValueHTML: " "},
			{EntityType: TranslationEntityProduct, EntityID: 11, FieldKey: TranslationFieldDetailHTML, Locale: "en", ValueHTML: `<p>English detail</p>`},
			{EntityType: TranslationEntityProduct, EntityID: 12, FieldKey: TranslationFieldDetailHTML, Locale: "zh", ValueHTML: " "},
		},
	}})

	withEnglishFallback, err := service.ProductDetailWithLocale(context.Background(), 11, "zh")
	if err != nil {
		t.Fatalf("ProductDetailWithLocale returned error: %v", err)
	}
	if withEnglishFallback.DetailHTML != `<p>English detail</p>` {
		t.Fatalf("detail_html = %q, want English fallback", withEnglishFallback.DetailHTML)
	}

	withOriginalFallback, err := service.ProductDetailWithLocale(context.Background(), 12, "zh")
	if err != nil {
		t.Fatalf("ProductDetailWithLocale returned error: %v", err)
	}
	if withOriginalFallback.DetailHTML != `<p>Original detail 2</p>` {
		t.Fatalf("detail_html = %q, want original fallback", withOriginalFallback.DetailHTML)
	}
}

func TestStorefrontServiceProductDetailMarksT6MallSource(t *testing.T) {
	t6TenantID := uint(100)
	service := NewStorefrontService(&fakeStorefrontRepository{
		tenants: map[uint]models.Tenant{
			t6TenantID: {
				BaseModel: models.BaseModel{ID: t6TenantID},
				Name:      models.PlatformMallTenantName,
				Type:      models.TenantTypeSelfOperated,
				Status:    models.TenantStatusActive,
			},
		},
		products: []models.SPU{
			{
				BaseModel: models.BaseModel{ID: 50},
				TenantID:  t6TenantID,
				Name:      "Official T6 Product",
				Status:    models.SPUStatusOnShelf,
				SKUs:      []models.SKU{{RetailPrice: 1000, IsActive: true}},
			},
		},
	})

	detail, err := service.ProductDetail(context.Background(), 50)
	if err != nil {
		t.Fatalf("ProductDetail returned error: %v", err)
	}
	if detail.Source != string(repository.ProductSourceT6) || detail.ShopName != "T6 Mall" || detail.ShopSlug != "" {
		t.Fatalf("product source/shop = %q/%q/%q, want T6 Mall", detail.Source, detail.ShopName, detail.ShopSlug)
	}
}

func TestStorefrontServiceProductDetailUsesConfiguredT6TenantID(t *testing.T) {
	t6TenantID := uint(555)
	repo := &fakeStorefrontRepository{
		platformMallTenantID: t6TenantID,
		tenants: map[uint]models.Tenant{
			t6TenantID: {
				BaseModel: models.BaseModel{ID: t6TenantID},
				Name:      "Production T6 Mall",
				Type:      models.TenantTypeSelfOperated,
				Status:    models.TenantStatusActive,
			},
		},
		products: []models.SPU{
			{
				BaseModel: models.BaseModel{ID: 51},
				TenantID:  t6TenantID,
				Name:      "Configured T6 Product",
				Status:    models.SPUStatusOnShelf,
				SKUs:      []models.SKU{{RetailPrice: 1000, IsActive: true}},
			},
		},
	}
	service := NewStorefrontService(repo, StorefrontOptions{PriceVisible: true, PlatformMallTenantID: t6TenantID})

	detail, err := service.ProductDetail(context.Background(), 51)
	if err != nil {
		t.Fatalf("ProductDetail returned error: %v", err)
	}
	if detail.Source != string(repository.ProductSourceT6) || detail.ShopName != "T6 Mall" {
		t.Fatalf("product source/shop = %q/%q, want configured T6 Mall", detail.Source, detail.ShopName)
	}
}

func TestStorefrontServiceHidesPricesWhenDisabled(t *testing.T) {
	repo := &fakeStorefrontRepository{
		profiles: []models.ShopProfile{
			{TenantID: 1001, Slug: "price-shop", ShopName: "Price Shop", IsPublic: true},
		},
		products: []models.SPU{
			{
				BaseModel: models.BaseModel{ID: 60},
				TenantID:  1001,
				Name:      "Hidden Price Product",
				Status:    models.SPUStatusOnShelf,
				SKUs: []models.SKU{
					{BaseModel: models.BaseModel{ID: 601}, RetailPrice: 1800, OriginalPrice: 2200, SellableStock: 4, IsActive: true},
					{BaseModel: models.BaseModel{ID: 602}, RetailPrice: 2500, OriginalPrice: 3000, SellableStock: 5, IsActive: true},
				},
			},
		},
	}
	service := NewStorefrontService(repo, StorefrontOptions{
		PriceVisible: false,
		Translations: &fakeTranslationReader{
			rows: []models.ContentTranslation{
				{EntityType: TranslationEntityProduct, EntityID: 60, FieldKey: TranslationFieldName, Locale: "zh", ValueText: "隐藏价格商品"},
			},
		},
	})

	page, err := service.Products(context.Background(), StorefrontProductQuery{Locale: "zh", Sort: "price_asc", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("Products returned error: %v", err)
	}
	if repo.lastProductQuery.Sort != "latest" {
		t.Fatalf("repo sort = %q, want price sort downgraded to latest", repo.lastProductQuery.Sort)
	}
	if len(page.Items) != 1 || page.Items[0].MinPrice != 0 || page.Items[0].MaxPrice != 0 || page.Items[0].OriginalPrice != 0 {
		t.Fatalf("list item prices = %+v, want hidden zeros", page.Items)
	}
	if page.Items[0].Name != "隐藏价格商品" {
		t.Fatalf("localized name = %q, want zh translation", page.Items[0].Name)
	}

	detail, err := service.ProductDetailWithLocale(context.Background(), 60, "zh")
	if err != nil {
		t.Fatalf("ProductDetail returned error: %v", err)
	}
	if detail.MinPrice != 0 || detail.MaxPrice != 0 || detail.OriginalPrice != 0 {
		t.Fatalf("detail prices = min %d max %d original %d, want hidden zeros", detail.MinPrice, detail.MaxPrice, detail.OriginalPrice)
	}
	for _, sku := range detail.SKUs {
		if sku.RetailPrice != 0 || sku.OriginalPrice != 0 {
			t.Fatalf("sku prices = %+v, want hidden zeros", detail.SKUs)
		}
	}
}

func TestStorefrontProductListKeepsMainImageAndOmitsVideo(t *testing.T) {
	dto := toProductListDTO(models.SPU{
		BaseModel: models.BaseModel{ID: 10},
		Name:      "Video Product",
		MainImage: "/uploads/images/main.webp",
		VideoURL:  "/uploads/videos/2026/06/product.mp4",
		Status:    models.SPUStatusOnShelf,
		SKUs: []models.SKU{{
			RetailPrice:   1000,
			OriginalPrice: 1200,
			IsActive:      true,
		}},
	}, true)
	if dto.MainImage != "/uploads/images/main.webp" {
		t.Fatalf("main_image = %q, want product image", dto.MainImage)
	}
	payload, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	if strings.Contains(string(payload), "video_url") || strings.Contains(string(payload), "product.mp4") {
		t.Fatalf("list payload = %s, should not include product video", string(payload))
	}
}

func TestStorefrontServiceProductDetailOffShelfReturnsNotFound(t *testing.T) {
	service := NewStorefrontService(&fakeStorefrontRepository{
		products: []models.SPU{
			{
				BaseModel: models.BaseModel{ID: 99},
				TenantID:  1001,
				Name:      "Hidden Product",
				Status:    models.SPUStatusOffShelf,
				SKUs:      []models.SKU{{RetailPrice: 1000, IsActive: true}},
			},
		},
	})

	if _, err := service.ProductDetail(context.Background(), 99); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ProductDetail error = %v, want ErrNotFound", err)
	}
}

func uintPtr(value uint) *uint {
	return &value
}

func jsonValue(value string) datatypes.JSON {
	return datatypes.JSON([]byte(value))
}

func containsUint(values []uint, expected uint) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func productIDs(products []ProductListDTO) []uint {
	ids := make([]uint, 0, len(products))
	for _, product := range products {
		ids = append(ids, product.ID)
	}
	return ids
}

func sameUintSet(actual []uint, expected []uint) bool {
	if len(actual) != len(expected) {
		return false
	}
	for _, value := range expected {
		if !containsUint(actual, value) {
			return false
		}
	}
	return true
}

func TestStorefrontServicePublicShopsOnlyReturnsPublishedProfiles(t *testing.T) {
	service := NewStorefrontService(&fakeStorefrontRepository{
		profiles: []models.ShopProfile{
			{TenantID: 1001, Slug: "public-shop", ShopName: "Public Shop", IsPublic: true},
			{TenantID: 2002, Slug: "hidden-shop", ShopName: "Hidden Shop", IsPublic: false},
		},
	})

	page, err := service.PublicShops(context.Background(), StorefrontShopQuery{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("PublicShops returned error: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("page = %+v, want one public shop", page)
	}
	if page.Items[0].Slug != "public-shop" {
		t.Fatalf("shop = %+v, want public-shop", page.Items[0])
	}
}

func TestStorefrontServicePublicShopProfileAppliesLocalizedFields(t *testing.T) {
	service := NewStorefrontService(&fakeStorefrontRepository{
		profiles: []models.ShopProfile{
			{
				BaseModel:     models.BaseModel{ID: 801},
				TenantID:      1001,
				Slug:          "public-shop",
				ShopName:      "Public Shop",
				Description:   "Original shop description",
				Address:       "Accra",
				BusinessHours: "9 AM - 6 PM",
				IsPublic:      true,
			},
			{
				BaseModel:   models.BaseModel{ID: 802},
				TenantID:    1002,
				Slug:        "fallback-shop",
				ShopName:    "Fallback Shop",
				Description: "Fallback main description",
				IsPublic:    true,
			},
		},
	}, StorefrontOptions{Translations: &fakeTranslationReader{
		rows: []models.ContentTranslation{
			{EntityType: TranslationEntityShopProfile, EntityID: 801, FieldKey: TranslationFieldShopName, Locale: "zh", ValueText: "公开商家"},
			{EntityType: TranslationEntityShopProfile, EntityID: 801, FieldKey: TranslationFieldDescription, Locale: "zh", ValueText: "中文店铺描述"},
			{EntityType: TranslationEntityShopProfile, EntityID: 801, FieldKey: TranslationFieldAddress, Locale: "zh", ValueText: "阿克拉"},
			{EntityType: TranslationEntityShopProfile, EntityID: 801, FieldKey: TranslationFieldBusinessHours, Locale: "zh", ValueText: "上午9点至下午6点"},
		},
	}})

	profile, err := service.PublicShopProfileWithLocale(context.Background(), "public-shop", "zh")
	if err != nil {
		t.Fatalf("PublicShopProfileWithLocale returned error: %v", err)
	}
	if profile.ShopName != "公开商家" || profile.Description != "中文店铺描述" || profile.Address != "阿克拉" || profile.BusinessHours != "上午9点至下午6点" {
		t.Fatalf("localized shop profile = %+v, want zh fields", profile)
	}

	fallback, err := service.PublicShopProfileWithLocale(context.Background(), "fallback-shop", "zh")
	if err != nil {
		t.Fatalf("PublicShopProfileWithLocale fallback returned error: %v", err)
	}
	if fallback.ShopName != "Fallback Shop" || fallback.Description != "Fallback main description" {
		t.Fatalf("fallback shop profile = %+v, want main table fields", fallback)
	}
}

func TestStorefrontServicePublicShopsApplyTranslationsInBatch(t *testing.T) {
	translations := &fakeTranslationReader{
		rows: []models.ContentTranslation{
			{EntityType: TranslationEntityShopProfile, EntityID: 901, FieldKey: TranslationFieldShopName, Locale: "zh", ValueText: "第一家店"},
			{EntityType: TranslationEntityShopProfile, EntityID: 902, FieldKey: TranslationFieldDescription, Locale: "zh", ValueText: "第二家店描述"},
		},
	}
	service := NewStorefrontService(&fakeStorefrontRepository{
		profiles: []models.ShopProfile{
			{BaseModel: models.BaseModel{ID: 901}, TenantID: 1001, Slug: "shop-one", ShopName: "Shop One", Description: "Original One", IsPublic: true},
			{BaseModel: models.BaseModel{ID: 902}, TenantID: 1002, Slug: "shop-two", ShopName: "Shop Two", Description: "Original Two", IsPublic: true},
		},
	}, StorefrontOptions{Translations: translations})

	page, err := service.PublicShops(context.Background(), StorefrontShopQuery{Locale: "zh", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("PublicShops returned error: %v", err)
	}
	if page.Items[0].ShopName != "第一家店" {
		t.Fatalf("first shop = %+v, want translated shop name", page.Items[0])
	}
	if page.Items[1].ShopName != "Shop Two" || page.Items[1].Description != "第二家店描述" {
		t.Fatalf("second shop = %+v, want mixed fallback and translated description", page.Items[1])
	}
	shopCalls := 0
	for _, call := range translations.calls {
		if call.EntityType == TranslationEntityShopProfile {
			shopCalls++
		}
	}
	if shopCalls != 1 {
		t.Fatalf("shop translation calls = %d, want one batch call", shopCalls)
	}
}

func TestStorefrontServiceShopProductsApplyProductTranslations(t *testing.T) {
	service := NewStorefrontService(&fakeStorefrontRepository{
		profiles: []models.ShopProfile{
			{BaseModel: models.BaseModel{ID: 901}, TenantID: 1001, Slug: "shop-one", ShopName: "Shop One", IsPublic: true},
		},
		products: []models.SPU{
			{BaseModel: models.BaseModel{ID: 31}, TenantID: 1001, Name: "Shop Product", Status: models.SPUStatusOnShelf},
		},
	}, StorefrontOptions{Translations: &fakeTranslationReader{
		rows: []models.ContentTranslation{
			{EntityType: TranslationEntityProduct, EntityID: 31, FieldKey: TranslationFieldName, Locale: "zh", ValueText: "店铺商品"},
		},
	}})

	page, err := service.ShopProducts(context.Background(), "shop-one", StorefrontProductQuery{Locale: "zh", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ShopProducts returned error: %v", err)
	}
	if page.Total != 1 || page.Items[0].Name != "店铺商品" {
		t.Fatalf("shop products = %+v, want translated product", page)
	}
}

func TestStorefrontServiceHiddenShopReturnsNotFound(t *testing.T) {
	service := NewStorefrontService(&fakeStorefrontRepository{
		profiles: []models.ShopProfile{
			{TenantID: 1001, Slug: "hidden-shop", ShopName: "Hidden Shop", IsPublic: false},
		},
	})

	if _, err := service.PublicShopProfile(context.Background(), "hidden-shop"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("PublicShopProfile error = %v, want ErrNotFound", err)
	}
	if _, err := service.ShopProducts(context.Background(), "hidden-shop", StorefrontProductQuery{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ShopProducts error = %v, want ErrNotFound", err)
	}
}

func TestBuildProductSlug(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{name: "Cruiser Motorcycle", want: "cruiser-motorcycle"},
		{name: "42MM Two-Color PVC Pipe Cutter", want: "42mm-two-color-pvc-pipe-cutter"},
		{name: "Phones & Accessories", want: "phones-and-accessories"},
		{name: "中文商品", want: "product"},
		{name: "", want: "product"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := BuildProductSlug(tt.name); got != tt.want {
				t.Fatalf("BuildProductSlug(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestStorefrontServiceSitemapIncludesOnlyPublicShopsAndOnShelfProducts(t *testing.T) {
	updatedAt := time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)
	service := NewStorefrontService(&fakeStorefrontRepository{
		tenants: map[uint]models.Tenant{
			3003: {
				BaseModel: models.BaseModel{ID: 3003},
				Name:      models.PlatformMallTenantName,
				Type:      models.TenantTypeSelfOperated,
				Status:    models.TenantStatusActive,
			},
		},
		profiles: []models.ShopProfile{
			{BaseModel: models.BaseModel{UpdatedAt: updatedAt}, TenantID: 1001, Slug: "public-shop", ShopName: "Public Shop", IsPublic: true},
			{BaseModel: models.BaseModel{UpdatedAt: updatedAt}, TenantID: 2002, Slug: "hidden-shop", ShopName: "Hidden Shop", IsPublic: false},
		},
		products: []models.SPU{
			{BaseModel: models.BaseModel{ID: 10, UpdatedAt: updatedAt}, TenantID: 1001, Name: "Cruiser Motorcycle", Status: models.SPUStatusOnShelf},
			{BaseModel: models.BaseModel{ID: 11, UpdatedAt: updatedAt}, TenantID: 1001, Status: models.SPUStatusOffShelf},
			{BaseModel: models.BaseModel{ID: 20, UpdatedAt: updatedAt}, TenantID: 2002, Status: models.SPUStatusOnShelf},
			{BaseModel: models.BaseModel{ID: 30, UpdatedAt: updatedAt}, TenantID: 3003, Status: models.SPUStatusOnShelf},
			{BaseModel: models.BaseModel{ID: 40, UpdatedAt: updatedAt}, TenantID: 3003, Name: "42MM Two-Color PVC Pipe Cutter & Kit", Status: models.SPUStatusOnShelf},
		},
	})

	sitemap, err := service.SitemapXML(context.Background(), "https://www.ghanachinatown.com")
	if err != nil {
		t.Fatalf("SitemapXML returned error: %v", err)
	}

	body := string(sitemap)
	for _, expected := range []string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
		`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">`,
		`<loc>https://www.ghanachinatown.com/en</loc>`,
		`<loc>https://www.ghanachinatown.com/zh</loc>`,
		`<loc>https://www.ghanachinatown.com/en/mall</loc>`,
		`<loc>https://www.ghanachinatown.com/zh/mall</loc>`,
		`<loc>https://www.ghanachinatown.com/en/mall?source=t6</loc>`,
		`<loc>https://www.ghanachinatown.com/zh/mall?source=t6</loc>`,
		`<loc>https://www.ghanachinatown.com/en/mall?source=merchant</loc>`,
		`<loc>https://www.ghanachinatown.com/zh/mall?source=merchant</loc>`,
		`<loc>https://www.ghanachinatown.com/en/merchant/join</loc>`,
		`<loc>https://www.ghanachinatown.com/zh/merchant/join</loc>`,
		`<loc>https://www.ghanachinatown.com/en/shops/public-shop</loc>`,
		`<loc>https://www.ghanachinatown.com/zh/shops/public-shop</loc>`,
		`<loc>https://www.ghanachinatown.com/en/products/cruiser-motorcycle-10</loc>`,
		`<loc>https://www.ghanachinatown.com/zh/products/cruiser-motorcycle-10</loc>`,
		`<loc>https://www.ghanachinatown.com/en/products/product-30</loc>`,
		`<loc>https://www.ghanachinatown.com/zh/products/product-30</loc>`,
		`<loc>https://www.ghanachinatown.com/en/products/42mm-two-color-pvc-pipe-cutter-and-kit-40</loc>`,
		`<loc>https://www.ghanachinatown.com/zh/products/42mm-two-color-pvc-pipe-cutter-and-kit-40</loc>`,
		`<xhtml:link rel="alternate" hreflang="en" href="https://www.ghanachinatown.com/en/products/cruiser-motorcycle-10"></xhtml:link>`,
		`<xhtml:link rel="alternate" hreflang="zh" href="https://www.ghanachinatown.com/zh/products/cruiser-motorcycle-10"></xhtml:link>`,
		`<xhtml:link rel="alternate" hreflang="x-default" href="https://www.ghanachinatown.com/en/products/cruiser-motorcycle-10"></xhtml:link>`,
		`<lastmod>2026-06-26T10:00:00Z</lastmod>`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("sitemap missing %q:\n%s", expected, body)
		}
	}
	for _, blocked := range []string{
		`hidden-shop`,
		`<loc>https://www.ghanachinatown.com/products/10</loc>`,
		`<loc>https://www.ghanachinatown.com/products/30</loc>`,
		`<loc>https://www.ghanachinatown.com/mall</loc>`,
		`<loc>https://www.ghanachinatown.com/mall?source=t6</loc>`,
		`<loc>https://www.ghanachinatown.com/merchant/join</loc>`,
		`<loc>https://www.ghanachinatown.com/shops/public-shop</loc>`,
		`<loc>https://www.ghanachinatown.com/products/cruiser-motorcycle-10</loc>`,
		`/products/11`,
		`/products/20`,
		`/shops/t6-mall`,
		`/admin`,
		`/login`,
		`/gc-control-7x`,
		`/cart`,
		`/checkout`,
		`/orders`,
	} {
		if strings.Contains(body, blocked) {
			t.Fatalf("sitemap contains blocked URL %q:\n%s", blocked, body)
		}
	}
	var parsed sitemapURLSet
	if err := xml.Unmarshal(sitemap, &parsed); err != nil {
		t.Fatalf("sitemap XML is invalid: %v\n%s", err, body)
	}
	if len(parsed.URLs) != 18 {
		t.Fatalf("sitemap URL count = %d, want 18:\n%s", len(parsed.URLs), body)
	}
}
