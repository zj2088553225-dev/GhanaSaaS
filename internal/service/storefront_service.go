package service

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"net/url"
	"strings"
	"time"

	"saas/internal/models"
	"saas/internal/pkg/locale"
	"saas/internal/repository"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type CategoryTreeDTO struct {
	ID       uint              `json:"id"`
	ParentID *uint             `json:"parent_id"`
	Name     string            `json:"name"`
	Sort     int               `json:"sort"`
	Icon     string            `json:"icon"`
	Children []CategoryTreeDTO `json:"children"`
}

type SKUDTO struct {
	ID            uint           `json:"id"`
	ProductID     uint           `json:"product_id"`
	Name          string         `json:"name"`
	ERPSKUCode    string         `json:"erp_sku_code"`
	ImageURL      string         `json:"image_url"`
	Specs         datatypes.JSON `json:"specs"`
	RetailPrice   int64          `json:"retail_price"`
	OriginalPrice int64          `json:"original_price"`
	SellableStock int64          `json:"sellable_stock"`
	Stock         int64          `json:"stock"`
	IsActive      bool           `json:"is_active"`
}

type ProductListDTO struct {
	ID                 uint             `json:"id"`
	CategoryID         *uint            `json:"category_id"`
	PlatformCategoryID uint             `json:"platform_category_id"`
	Name               string           `json:"name"`
	Description        string           `json:"description"`
	MainImage          string           `json:"main_image"`
	SliderImages       datatypes.JSON   `json:"slider_images"`
	SpecTemplate       datatypes.JSON   `json:"spec_template"`
	Status             models.SPUStatus `json:"status"`
	MinPrice           int64            `json:"min_price"`
	MaxPrice           int64            `json:"max_price"`
	OriginalPrice      int64            `json:"original_price"`
}

type ProductDetailDTO struct {
	ID                 uint                 `json:"id"`
	CategoryID         *uint                `json:"category_id"`
	PlatformCategoryID uint                 `json:"platform_category_id"`
	PlatformCategory   *PlatformCategoryDTO `json:"platform_category,omitempty"`
	Source             string               `json:"source"`
	ShopName           string               `json:"shop_name,omitempty"`
	ShopSlug           string               `json:"shop_slug,omitempty"`
	Name               string               `json:"name"`
	Description        string               `json:"description"`
	DetailHTML         string               `json:"detail_html"`
	MainImage          string               `json:"main_image"`
	VideoURL           string               `json:"video_url"`
	Material           string               `json:"material"`
	Dimensions         string               `json:"dimensions"`
	Weight             string               `json:"weight"`
	SliderImages       datatypes.JSON       `json:"slider_images"`
	Images             []string             `json:"images"`
	SpecTemplate       datatypes.JSON       `json:"spec_template"`
	Specs              datatypes.JSON       `json:"specs"`
	Status             models.SPUStatus     `json:"status"`
	MinPrice           int64                `json:"min_price"`
	MaxPrice           int64                `json:"max_price"`
	OriginalPrice      int64                `json:"original_price"`
	TotalStock         int64                `json:"total_stock"`
	SKUs               []SKUDTO             `json:"skus"`
}

type PlatformCategoryDTO struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type ProductPageDTO struct {
	Items    []ProductListDTO `json:"items"`
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
}

type PublicShopProfileDTO struct {
	TenantID      uint   `json:"tenant_id"`
	Slug          string `json:"slug"`
	ShopName      string `json:"shop_name"`
	LogoURL       string `json:"logo_url"`
	BannerURL     string `json:"banner_url"`
	Description   string `json:"description"`
	Phone         string `json:"phone"`
	Whatsapp      string `json:"whatsapp"`
	Address       string `json:"address"`
	BusinessHours string `json:"business_hours"`
}

type PublicShopPageDTO struct {
	Items    []PublicShopProfileDTO `json:"items"`
	Total    int64                  `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
}

type StorefrontProductQuery struct {
	CategoryID         *uint
	PlatformCategoryID *uint
	Keyword            string
	Locale             string
	Source             string
	IsSelfOperated     bool
	Sort               string
	Page               int
	PageSize           int
}

type StorefrontShopQuery struct {
	Locale   string
	Page     int
	PageSize int
}

type storefrontRepository interface {
	ListActiveCategories(ctx context.Context) ([]models.Category, error)
	ListProducts(ctx context.Context, query repository.ProductQuery) ([]models.SPU, int64, error)
	FindProductByID(ctx context.Context, id uint) (*models.SPU, error)
	FindPublicShopProfileByTenantID(ctx context.Context, tenantID uint) (*models.ShopProfile, error)
	FindPlatformCategoryByID(ctx context.Context, id uint) (*models.PlatformCategory, error)
	ListPlatformCategories(ctx context.Context, activeOnly bool) ([]models.PlatformCategory, error)
	ListPlatformCategoryIDsByRoot(ctx context.Context, id uint) ([]uint, error)
	FindPublicShopProfileBySlug(ctx context.Context, slug string) (*models.ShopProfile, error)
	ListPublicShopProfiles(ctx context.Context, query repository.PublicShopProfileQuery) ([]models.ShopProfile, int64, error)
	ListSitemapShops(ctx context.Context) ([]repository.SitemapShopURL, error)
	ListSitemapProducts(ctx context.Context) ([]repository.SitemapProductURL, error)
}

type StorefrontService struct {
	repo                 storefrontRepository
	translations         TranslationReader
	priceVisible         bool
	platformMallTenantID uint
}

type StorefrontOptions struct {
	PriceVisible         bool
	PlatformMallTenantID uint
	Translations         TranslationReader
}

func NewStorefrontService(repo storefrontRepository, options ...StorefrontOptions) *StorefrontService {
	s := &StorefrontService{
		repo:         repo,
		priceVisible: true,
	}
	if len(options) > 0 {
		s.priceVisible = options[0].PriceVisible
		s.platformMallTenantID = options[0].PlatformMallTenantID
		s.translations = options[0].Translations
	}
	return s
}

func (s *StorefrontService) CategoryTree(ctx context.Context) ([]CategoryTreeDTO, error) {
	return s.CategoryTreeWithLocale(ctx, locale.DefaultLocale)
}

func (s *StorefrontService) CategoryTreeWithLocale(ctx context.Context, requestedLocale string) ([]CategoryTreeDTO, error) {
	normalizedLocale := locale.NormalizeLocale(requestedLocale)
	categories, err := s.repo.ListActiveCategories(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.applyCategoryTranslations(ctx, categories, normalizedLocale); err != nil {
		return nil, err
	}
	return buildCategoryTree(categories), nil
}

func (s *StorefrontService) Products(ctx context.Context, query StorefrontProductQuery) (*ProductPageDTO, error) {
	query.Locale = locale.NormalizeLocale(query.Locale)
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > 100 {
		query.PageSize = 100
	}

	productQuery := repository.ProductQuery{
		CategoryID:     query.CategoryID,
		Keyword:        query.Keyword,
		Source:         normalizeProductSource(query.Source, query.IsSelfOperated),
		IsSelfOperated: query.IsSelfOperated,
		Sort:           normalizeProductSort(query.Sort, s.priceVisible),
		Page:           query.Page,
		PageSize:       query.PageSize,
	}
	if err := s.applyPlatformCategoryScope(ctx, &productQuery, query.PlatformCategoryID); err != nil {
		return nil, err
	}

	products, total, err := s.repo.ListProducts(ctx, productQuery)
	if err != nil {
		return nil, err
	}

	translations, err := s.productTranslations(ctx, products, query.Locale)
	if err != nil {
		return nil, err
	}
	items := make([]ProductListDTO, 0, len(products))
	for _, product := range products {
		dto := toProductListDTO(product, s.priceVisible)
		applyProductListTranslations(&dto, translations[product.ID])
		items = append(items, dto)
	}

	return &ProductPageDTO{
		Items:    items,
		Total:    total,
		Page:     query.Page,
		PageSize: query.PageSize,
	}, nil
}

func (s *StorefrontService) applyPlatformCategoryScope(ctx context.Context, productQuery *repository.ProductQuery, platformCategoryID *uint) error {
	if platformCategoryID == nil {
		return nil
	}

	ids, err := s.repo.ListPlatformCategoryIDsByRoot(ctx, *platformCategoryID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		productQuery.PlatformCategoryID = platformCategoryID
		return nil
	}
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		productQuery.PlatformCategoryID = platformCategoryID
		return nil
	}

	productQuery.PlatformCategoryIDs = ids
	return nil
}

func (s *StorefrontService) ProductDetail(ctx context.Context, id uint) (*ProductDetailDTO, error) {
	return s.ProductDetailWithLocale(ctx, id, locale.DefaultLocale)
}

func (s *StorefrontService) ProductDetailWithLocale(ctx context.Context, id uint, requestedLocale string) (*ProductDetailDTO, error) {
	normalizedLocale := locale.NormalizeLocale(requestedLocale)
	product, err := s.repo.FindProductByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	var platformCategory *models.PlatformCategory
	if product.PlatformCategoryID != 0 {
		platformCategory, err = s.repo.FindPlatformCategoryByID(ctx, product.PlatformCategoryID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		if platformCategory != nil {
			platformCategory, err = s.localizedPlatformCategory(ctx, *platformCategory, normalizedLocale)
			if err != nil {
				return nil, err
			}
		}
	}

	var shopProfile *models.ShopProfile
	if !models.IsPlatformMallTenantWithID(product.Tenant, s.platformMallTenantID) {
		shopProfile, err = s.repo.FindPublicShopProfileByTenantID(ctx, product.TenantID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		if shopProfile != nil {
			if err := s.applyShopProfileTranslations(ctx, shopProfile, normalizedLocale); err != nil {
				return nil, err
			}
		}
	}

	dto := toProductDetailDTO(*product, platformCategory, shopProfile, s.priceVisible, s.platformMallTenantID)
	if err := s.applyProductDetailTranslations(ctx, &dto, product.ID, normalizedLocale); err != nil {
		return nil, err
	}
	return &dto, nil
}

func (s *StorefrontService) PublicShopProfile(ctx context.Context, slug string) (*PublicShopProfileDTO, error) {
	return s.PublicShopProfileWithLocale(ctx, slug, locale.DefaultLocale)
}

func (s *StorefrontService) PublicShopProfileWithLocale(ctx context.Context, slug string, requestedLocale string) (*PublicShopProfileDTO, error) {
	normalizedLocale := locale.NormalizeLocale(requestedLocale)
	profile, err := s.publicShopProfileBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}

	if err := s.applyShopProfileTranslations(ctx, profile, normalizedLocale); err != nil {
		return nil, err
	}
	dto := toPublicShopProfileDTO(*profile)
	return &dto, nil
}

func (s *StorefrontService) PublicShops(ctx context.Context, query StorefrontShopQuery) (*PublicShopPageDTO, error) {
	query.Locale = locale.NormalizeLocale(query.Locale)
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > 100 {
		query.PageSize = 100
	}

	profiles, total, err := s.repo.ListPublicShopProfiles(ctx, repository.PublicShopProfileQuery{
		Page:     query.Page,
		PageSize: query.PageSize,
	})
	if err != nil {
		return nil, err
	}

	translations, err := s.shopProfileTranslations(ctx, profiles, query.Locale)
	if err != nil {
		return nil, err
	}
	items := make([]PublicShopProfileDTO, 0, len(profiles))
	for _, profile := range profiles {
		applyShopProfileTranslation(&profile, translations[profile.ID])
		items = append(items, toPublicShopProfileDTO(profile))
	}
	return &PublicShopPageDTO{
		Items:    items,
		Total:    total,
		Page:     query.Page,
		PageSize: query.PageSize,
	}, nil
}

type sitemapURLSet struct {
	XMLName    xml.Name     `xml:"urlset"`
	XMLNS      string       `xml:"xmlns,attr"`
	XMLNSXHTML string       `xml:"xmlns:xhtml,attr"`
	URLs       []sitemapURL `xml:"url"`
}

type sitemapURL struct {
	Loc        string             `xml:"loc"`
	Alternates []sitemapAlternate `xml:"xhtml:link,omitempty"`
	LastMod    string             `xml:"lastmod,omitempty"`
	ChangeFreq string             `xml:"changefreq"`
	Priority   string             `xml:"priority"`
}

type sitemapAlternate struct {
	Rel      string `xml:"rel,attr"`
	Hreflang string `xml:"hreflang,attr"`
	Href     string `xml:"href,attr"`
}

func (s *StorefrontService) SitemapXML(ctx context.Context, baseURL string) ([]byte, error) {
	shops, err := s.repo.ListSitemapShops(ctx)
	if err != nil {
		return nil, err
	}
	products, err := s.repo.ListSitemapProducts(ctx)
	if err != nil {
		return nil, err
	}

	baseURL = normalizedSitemapBaseURL(baseURL)
	urls := make([]sitemapURL, 0, 10+len(shops)*2+len(products)*2)
	urls = append(urls,
		localizedSitemapURLs(baseURL, "", "", "daily", "1.0")...,
	)
	urls = append(urls,
		localizedSitemapURLs(baseURL, "/mall", "", "daily", "0.9")...,
	)
	urls = append(urls,
		localizedSitemapURLs(baseURL, "/mall?source=t6", "", "daily", "0.8")...,
	)
	urls = append(urls,
		localizedSitemapURLs(baseURL, "/mall?source=merchant", "", "daily", "0.8")...,
	)
	urls = append(urls,
		localizedSitemapURLs(baseURL, "/merchant/join", "", "monthly", "0.7")...,
	)
	for _, shop := range shops {
		slug := strings.TrimSpace(shop.Slug)
		if slug == "" {
			continue
		}
		urls = append(urls, localizedSitemapURLs(baseURL, "/shops/"+url.PathEscape(slug), sitemapLastMod(shop.UpdatedAt), "weekly", "0.8")...)
	}
	for _, product := range products {
		if product.ID == 0 {
			continue
		}
		urls = append(urls, localizedSitemapURLs(baseURL, "/products/"+url.PathEscape(BuildProductSEOPathSegment(product.Name, product.ID)), sitemapLastMod(product.UpdatedAt), "weekly", "0.7")...)
	}

	output, err := xml.MarshalIndent(sitemapURLSet{
		XMLNS:      "http://www.sitemaps.org/schemas/sitemap/0.9",
		XMLNSXHTML: "http://www.w3.org/1999/xhtml",
		URLs:       urls,
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), output...), nil
}

func localizedSitemapURLs(baseURL string, storefrontPath string, lastMod string, changeFreq string, priority string) []sitemapURL {
	enURL := localizedSitemapURL(baseURL, "en", storefrontPath)
	zhURL := localizedSitemapURL(baseURL, "zh", storefrontPath)
	alternates := []sitemapAlternate{
		{Rel: "alternate", Hreflang: "en", Href: enURL},
		{Rel: "alternate", Hreflang: "zh", Href: zhURL},
		{Rel: "alternate", Hreflang: "x-default", Href: enURL},
	}
	return []sitemapURL{
		{Loc: enURL, Alternates: alternates, LastMod: lastMod, ChangeFreq: changeFreq, Priority: priority},
		{Loc: zhURL, Alternates: alternates, LastMod: lastMod, ChangeFreq: changeFreq, Priority: priority},
	}
}

func localizedSitemapURL(baseURL string, localeValue string, storefrontPath string) string {
	path := strings.TrimSpace(storefrontPath)
	if path == "" || path == "/" {
		return baseURL + "/" + localeValue
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return baseURL + "/" + localeValue + path
}

func (s *StorefrontService) ShopProducts(ctx context.Context, slug string, query StorefrontProductQuery) (*ProductPageDTO, error) {
	query.Locale = locale.NormalizeLocale(query.Locale)
	profile, err := s.publicShopProfileBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}

	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > 100 {
		query.PageSize = 100
	}

	tenantID := profile.TenantID
	productQuery := repository.ProductQuery{
		CategoryID: query.CategoryID,
		TenantID:   &tenantID,
		Keyword:    query.Keyword,
		Sort:       normalizeProductSort(query.Sort, s.priceVisible),
		Page:       query.Page,
		PageSize:   query.PageSize,
	}
	if err := s.applyPlatformCategoryScope(ctx, &productQuery, query.PlatformCategoryID); err != nil {
		return nil, err
	}

	products, total, err := s.repo.ListProducts(ctx, productQuery)
	if err != nil {
		return nil, err
	}

	translations, err := s.productTranslations(ctx, products, query.Locale)
	if err != nil {
		return nil, err
	}
	items := make([]ProductListDTO, 0, len(products))
	for _, product := range products {
		dto := toProductListDTO(product, s.priceVisible)
		applyProductListTranslations(&dto, translations[product.ID])
		items = append(items, dto)
	}

	return &ProductPageDTO{
		Items:    items,
		Total:    total,
		Page:     query.Page,
		PageSize: query.PageSize,
	}, nil
}

func (s *StorefrontService) publicShopProfileBySlug(ctx context.Context, slug string) (*models.ShopProfile, error) {
	slug = strings.TrimSpace(slug)
	if !isValidShopSlug(slug) {
		return nil, ErrNotFound
	}

	profile, err := s.repo.FindPublicShopProfileBySlug(ctx, slug)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return profile, nil
}

func toPublicShopProfileDTO(profile models.ShopProfile) PublicShopProfileDTO {
	return PublicShopProfileDTO{
		TenantID:      profile.TenantID,
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
}

func buildCategoryTree(categories []models.Category) []CategoryTreeDTO {
	childrenByParent := make(map[uint][]models.Category)
	roots := make([]models.Category, 0)

	for _, category := range categories {
		if category.ParentID == nil {
			roots = append(roots, category)
			continue
		}
		childrenByParent[*category.ParentID] = append(childrenByParent[*category.ParentID], category)
	}

	result := make([]CategoryTreeDTO, 0, len(roots))
	for _, category := range roots {
		result = append(result, buildCategoryNode(category, childrenByParent))
	}
	return result
}

func normalizeProductSort(sort string, priceVisible bool) string {
	normalized := strings.TrimSpace(strings.ToLower(sort))
	switch normalized {
	case "hot":
		return normalized
	case "price_asc", "price_desc":
		if priceVisible {
			return normalized
		}
		return "latest"
	default:
		return "latest"
	}
}

func normalizeProductSource(source string, isSelfOperated bool) repository.ProductSource {
	switch strings.TrimSpace(strings.ToLower(source)) {
	case string(repository.ProductSourceT6):
		return repository.ProductSourceT6
	case string(repository.ProductSourceMerchant):
		return repository.ProductSourceMerchant
	default:
		if isSelfOperated {
			return repository.ProductSourceT6
		}
		return repository.ProductSourceAll
	}
}

func normalizedSitemapBaseURL(baseURL string) string {
	value := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if value == "" {
		return "https://www.ghanachinatown.com"
	}
	return value
}

func sitemapLastMod(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func buildCategoryNode(category models.Category, childrenByParent map[uint][]models.Category) CategoryTreeDTO {
	node := CategoryTreeDTO{
		ID:       category.ID,
		ParentID: category.ParentID,
		Name:     category.Name,
		Sort:     category.Sort,
		Icon:     category.Icon,
		Children: []CategoryTreeDTO{},
	}

	for _, child := range childrenByParent[category.ID] {
		node.Children = append(node.Children, buildCategoryNode(child, childrenByParent))
	}

	return node
}

func toProductListDTO(product models.SPU, priceVisible bool) ProductListDTO {
	minPrice := int64(0)
	maxPrice := int64(0)
	originalPrice := int64(0)
	if priceVisible {
		minPrice = minRetailPrice(product.SKUs)
		maxPrice = maxRetailPrice(product.SKUs)
		originalPrice = minOriginalPrice(product.SKUs)
	}

	return ProductListDTO{
		ID:                 product.ID,
		CategoryID:         product.CategoryID,
		PlatformCategoryID: product.PlatformCategoryID,
		Name:               product.Name,
		Description:        product.Description,
		MainImage:          product.MainImage,
		SliderImages:       product.SliderImages,
		SpecTemplate:       product.SpecTemplate,
		Status:             product.Status,
		MinPrice:           minPrice,
		MaxPrice:           maxPrice,
		OriginalPrice:      originalPrice,
	}
}

func toProductDetailDTO(product models.SPU, platformCategory *models.PlatformCategory, shopProfile *models.ShopProfile, priceVisible bool, platformMallTenantID uint) ProductDetailDTO {
	skus := make([]SKUDTO, 0, len(product.SKUs))
	for _, sku := range product.SKUs {
		retailPrice := int64(0)
		originalPrice := int64(0)
		if priceVisible {
			retailPrice = sku.RetailPrice
			originalPrice = sku.OriginalPrice
		}
		skus = append(skus, SKUDTO{
			ID:            sku.ID,
			ProductID:     sku.ProductID,
			Name:          sku.Name,
			ERPSKUCode:    sku.ERPSKUCode,
			ImageURL:      sku.ImageURL,
			Specs:         sku.Specs,
			RetailPrice:   retailPrice,
			OriginalPrice: originalPrice,
			SellableStock: sku.SellableStock,
			Stock:         sku.SellableStock,
			IsActive:      sku.IsActive,
		})
	}

	source := string(repository.ProductSourceMerchant)
	shopName := ""
	shopSlug := ""
	if models.IsPlatformMallTenantWithID(product.Tenant, platformMallTenantID) {
		source = string(repository.ProductSourceT6)
		shopName = "T6 Mall"
	} else if shopProfile != nil {
		shopName = shopProfile.ShopName
		shopSlug = shopProfile.Slug
	}

	dto := ProductDetailDTO{
		ID:                 product.ID,
		CategoryID:         product.CategoryID,
		PlatformCategoryID: product.PlatformCategoryID,
		Source:             source,
		ShopName:           shopName,
		ShopSlug:           shopSlug,
		Name:               product.Name,
		Description:        product.Description,
		DetailHTML:         product.DetailHTML,
		MainImage:          product.MainImage,
		VideoURL:           product.VideoURL,
		Material:           product.Material,
		Dimensions:         product.Dimensions,
		Weight:             product.Weight,
		SliderImages:       product.SliderImages,
		Images:             productImages(product.MainImage, product.SliderImages),
		SpecTemplate:       product.SpecTemplate,
		Specs:              product.SpecTemplate,
		Status:             product.Status,
		MinPrice:           hiddenAwareMinRetailPrice(product.SKUs, priceVisible),
		MaxPrice:           hiddenAwareMaxRetailPrice(product.SKUs, priceVisible),
		OriginalPrice:      hiddenAwareMinOriginalPrice(product.SKUs, priceVisible),
		TotalStock:         totalStock(product.SKUs),
		SKUs:               skus,
	}
	if platformCategory != nil {
		dto.PlatformCategory = &PlatformCategoryDTO{
			ID:   platformCategory.ID,
			Name: platformCategory.Name,
			Path: platformCategory.Path,
		}
	}
	return dto
}

func minRetailPrice(skus []models.SKU) int64 {
	if len(skus) == 0 {
		return 0
	}

	minPrice := skus[0].RetailPrice
	for _, sku := range skus[1:] {
		if sku.RetailPrice < minPrice {
			minPrice = sku.RetailPrice
		}
	}
	return minPrice
}

func hiddenAwareMinRetailPrice(skus []models.SKU, priceVisible bool) int64 {
	if !priceVisible {
		return 0
	}
	return minRetailPrice(skus)
}

func maxRetailPrice(skus []models.SKU) int64 {
	if len(skus) == 0 {
		return 0
	}

	maxPrice := skus[0].RetailPrice
	for _, sku := range skus[1:] {
		if sku.RetailPrice > maxPrice {
			maxPrice = sku.RetailPrice
		}
	}
	return maxPrice
}

func hiddenAwareMaxRetailPrice(skus []models.SKU, priceVisible bool) int64 {
	if !priceVisible {
		return 0
	}
	return maxRetailPrice(skus)
}

func minOriginalPrice(skus []models.SKU) int64 {
	minPrice := int64(0)
	for _, sku := range skus {
		if sku.OriginalPrice <= 0 {
			continue
		}
		if minPrice == 0 || sku.OriginalPrice < minPrice {
			minPrice = sku.OriginalPrice
		}
	}
	return minPrice
}

func hiddenAwareMinOriginalPrice(skus []models.SKU, priceVisible bool) int64 {
	if !priceVisible {
		return 0
	}
	return minOriginalPrice(skus)
}

func totalStock(skus []models.SKU) int64 {
	total := int64(0)
	for _, sku := range skus {
		if sku.SellableStock > 0 {
			total += sku.SellableStock
		}
	}
	return total
}

func productImages(mainImage string, sliderImages datatypes.JSON) []string {
	seen := make(map[string]struct{})
	images := make([]string, 0)
	addImage := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		images = append(images, value)
	}

	addImage(mainImage)
	var parsed []string
	if len(sliderImages) > 0 && json.Unmarshal(sliderImages, &parsed) == nil {
		for _, image := range parsed {
			addImage(image)
		}
	}
	return images
}
