package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"saas/internal/models"
	"saas/internal/pkg/locale"
	"saas/internal/pkg/trackingkey"
	"saas/internal/repository"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	SitePageHomeKey = "home"

	defaultSitePageTitle  = "平台主页"
	maxSitePageTitleLen   = 200
	maxSitePageBlocks     = 50
	maxBlockTitleLen      = 160
	maxBlockSubtitleLen   = 240
	maxBlockTextLen       = 4000
	maxBlockURLLen        = 512
	maxBlockButtonTextLen = 80
	maxBlockItems         = 24
	maxGridLimit          = 24
	defaultGridLimit      = 8
	defaultMapHeight      = 320
	minMapHeight          = 200
	maxMapHeight          = 800
)

var allowedSitePageBlockTypes = map[string]struct{}{
	"hero":          {},
	"hero_banner":   {},
	"hero_carousel": {},
	"category_nav":  {},
	"map_section":   {},
	"image_text":    {},
	"feature_grid":  {},
	"product_floor": {},
	"product_grid":  {},
	"shop_grid":     {},
	"promo_banner":  {},
	"footer_links":  {},
	"contact":       {},
	"text":          {},
	"image":         {},
}

type SitePageContent struct {
	Blocks []map[string]any `json:"blocks"`
}

type SitePageDraftInput struct {
	Title        string
	Content      map[string]any
	Translations *SitePageDraftTranslationsInput
}

type SitePageDraftTranslationsInput struct {
	Zh *SitePageLocaleDraftInput
}

type SitePageLocaleDraftInput struct {
	DraftContent map[string]any
}

type SitePageAdminDTO struct {
	PageKey          string                `json:"page_key"`
	Title            string                `json:"title"`
	DraftContent     SitePageContent       `json:"draft_content"`
	PublishedContent *SitePageContent      `json:"published_content,omitempty"`
	Translations     SitePageTranslations  `json:"translations"`
	Status           models.SitePageStatus `json:"status"`
	PublishedAt      *time.Time            `json:"published_at,omitempty"`
}

type SitePageTranslations struct {
	Zh SitePageLocaleTranslation `json:"zh"`
}

type SitePageLocaleTranslation struct {
	DraftContent     *SitePageContent `json:"draft_content,omitempty"`
	PublishedContent *SitePageContent `json:"published_content,omitempty"`
}

type PublicSitePageDTO struct {
	PageKey     string          `json:"page_key"`
	Title       string          `json:"title"`
	Content     SitePageContent `json:"content"`
	PublishedAt *time.Time      `json:"published_at,omitempty"`
}

type sitePageRepository interface {
	FindByKey(ctx context.Context, pageKey string) (*models.SitePage, error)
	SaveDraftAtomic(ctx context.Context, pageKey string, title string, content datatypes.JSON, userID uint, translation repository.SitePageTranslationMutation) (*models.SitePage, error)
	PublishAtomic(ctx context.Context, pageKey string, userID uint, buildTranslation repository.SitePagePublishTranslationBuilder) (*models.SitePage, error)
}

type SitePageService struct {
	repo         sitePageRepository
	translations TranslationEditor
}

type SitePageOptions struct {
	Translations TranslationEditor
}

func NewSitePageService(repo sitePageRepository, options ...SitePageOptions) *SitePageService {
	service := &SitePageService{repo: repo}
	if len(options) > 0 {
		service.translations = options[0].Translations
	}
	return service
}

func (s *SitePageService) GetHomeAdmin(ctx context.Context) (*SitePageAdminDTO, error) {
	page, err := s.repo.FindByKey(ctx, SitePageHomeKey)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &SitePageAdminDTO{
			PageKey:      SitePageHomeKey,
			Title:        defaultSitePageTitle,
			DraftContent: emptySitePageContent(),
			Status:       models.SitePageStatusDraft,
		}, nil
	}
	if err != nil {
		return nil, err
	}
	return s.toSitePageAdminDTO(ctx, page)
}

func (s *SitePageService) SaveHomeDraft(ctx context.Context, userID uint, input SitePageDraftInput) (*SitePageAdminDTO, error) {
	if userID == 0 {
		return nil, ErrUnauthorized
	}

	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = defaultSitePageTitle
	}
	if utf8.RuneCountInString(title) > maxSitePageTitleLen {
		return nil, ErrInvalidInput
	}

	content, err := normalizeSitePageContent(input.Content)
	if err != nil {
		return nil, err
	}
	contentJSON, err := marshalSitePageContent(content)
	if err != nil {
		return nil, err
	}

	translation, err := sitePageDraftTranslationMutation(content, input.Translations)
	if err != nil {
		return nil, err
	}
	page, err := s.repo.SaveDraftAtomic(ctx, SitePageHomeKey, title, contentJSON, userID, translation)
	if err != nil {
		return nil, err
	}
	return s.toSitePageAdminDTO(ctx, page)
}

func (s *SitePageService) PublishHome(ctx context.Context, userID uint) (*SitePageAdminDTO, error) {
	if userID == 0 {
		return nil, ErrUnauthorized
	}

	page, err := s.repo.PublishAtomic(ctx, SitePageHomeKey, userID, buildSitePagePublishedTranslation)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.toSitePageAdminDTO(ctx, page)
}

func (s *SitePageService) PublicHome(ctx context.Context) (*PublicSitePageDTO, error) {
	return s.PublicHomeWithLocale(ctx, locale.DefaultLocale)
}

func (s *SitePageService) PublicHomeWithLocale(ctx context.Context, requestedLocale string) (*PublicSitePageDTO, error) {
	localeValue := locale.NormalizeLocale(requestedLocale)
	page, err := s.repo.FindByKey(ctx, SitePageHomeKey)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if isEmptyJSON(page.PublishedContent) {
		return nil, ErrNotFound
	}

	content, err := unmarshalSitePageContent(page.PublishedContent)
	if err != nil {
		return nil, ErrNotFound
	}
	if localized, ok, err := s.sitePageTranslationContent(ctx, page.ID, TranslationFieldPublishedContent, localeValue); err != nil {
		return nil, err
	} else if ok {
		content = mergeLocalizedSitePageContent(content, localized)
	}
	return &PublicSitePageDTO{
		PageKey:     page.PageKey,
		Title:       page.Title,
		Content:     content,
		PublishedAt: page.PublishedAt,
	}, nil
}

func sitePageDraftTranslationMutation(
	base SitePageContent,
	translations *SitePageDraftTranslationsInput,
) (repository.SitePageTranslationMutation, error) {
	if translations == nil || translations.Zh == nil {
		return repository.SitePageTranslationMutation{}, nil
	}
	mutation := repository.SitePageTranslationMutation{Provided: true}
	if translations.Zh.DraftContent == nil {
		return mutation, nil
	}
	content, err := normalizeSitePageContent(translations.Zh.DraftContent)
	if err != nil {
		return mutation, err
	}
	localized := sitePageTranslationContentFromBase(base, content, false)
	if !hasSitePageTranslationText(localized) {
		return mutation, nil
	}
	data, err := marshalSitePageContent(localized)
	if err != nil {
		return mutation, err
	}
	mutation.Content = data
	return mutation, nil
}

func buildSitePagePublishedTranslation(
	englishDraft datatypes.JSON,
	zhDraft datatypes.JSON,
) (datatypes.JSON, bool, error) {
	if isEmptyJSON(zhDraft) {
		return nil, false, nil
	}
	base, err := unmarshalSitePageContent(englishDraft)
	if err != nil {
		return nil, false, err
	}
	localizedDraft, err := unmarshalSitePageContent(zhDraft)
	if err != nil {
		return nil, false, err
	}
	localized := sitePageTranslationContentFromBase(base, localizedDraft, false)
	if !hasSitePageTranslationText(localized) {
		return nil, false, nil
	}
	data, err := marshalSitePageContent(localized)
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

func (s *SitePageService) sitePageTranslationContent(ctx context.Context, pageID uint, fieldKey string, requestedLocale string) (SitePageContent, bool, error) {
	if s.translations == nil || pageID == 0 || locale.NormalizeLocale(requestedLocale) == locale.DefaultLocale {
		return SitePageContent{}, false, nil
	}
	rows, err := s.translations.GetTranslationsForEntity(ctx, TranslationEntitySitePage, pageID, locale.NormalizeLocale(requestedLocale))
	if err != nil {
		return SitePageContent{}, false, err
	}
	content, ok := sitePageContentFromTranslations(rows, fieldKey)
	return content, ok, nil
}

func normalizeSitePageContent(raw map[string]any) (SitePageContent, error) {
	if raw == nil {
		return SitePageContent{}, ErrInvalidInput
	}

	blocksValue, ok := raw["blocks"]
	if !ok {
		return SitePageContent{}, ErrInvalidInput
	}
	blocksRaw, ok := blocksValue.([]any)
	if !ok {
		return SitePageContent{}, ErrInvalidInput
	}
	if len(blocksRaw) > maxSitePageBlocks {
		return SitePageContent{}, ErrInvalidInput
	}

	blocks := make([]map[string]any, 0, len(blocksRaw))
	for _, rawBlock := range blocksRaw {
		block, err := normalizeSitePageBlock(rawBlock)
		if err != nil {
			return SitePageContent{}, err
		}
		blocks = append(blocks, block)
	}
	content := SitePageContent{Blocks: blocks}
	if err := ensureUniqueSitePageTrackingKeys(&content); err != nil {
		return SitePageContent{}, err
	}
	return content, nil
}

// ensureUniqueSitePageTrackingKeys preserves the first valid key and repairs
// later duplicates across the entire CMS tree. A second normalization pass is
// idempotent because every generated replacement is persisted with its node.
func ensureUniqueSitePageTrackingKeys(content *SitePageContent) error {
	seen := make(map[string]struct{})
	for _, block := range content.Blocks {
		if err := ensureUniqueSitePageNodeTrackingKey(block, seen); err != nil {
			return err
		}
	}
	return nil
}

func ensureUniqueSitePageNodeTrackingKey(node map[string]any, seen map[string]struct{}) error {
	trackingKey, _ := node["tracking_key"].(string)
	trackingKey = trackingkey.Normalize(trackingKey)
	if !trackingkey.IsValid(trackingKey) {
		var err error
		trackingKey, err = trackingkey.New()
		if err != nil {
			return err
		}
	}
	if _, duplicate := seen[trackingKey]; duplicate {
		var err error
		trackingKey, err = trackingkey.New()
		if err != nil {
			return err
		}
	}
	node["tracking_key"] = trackingKey
	seen[trackingKey] = struct{}{}

	for _, field := range []string{"slides", "items", "links"} {
		for _, child := range sitePageMapList(node[field]) {
			if err := ensureUniqueSitePageNodeTrackingKey(child, seen); err != nil {
				return err
			}
		}
	}
	return nil
}

func normalizeSitePageBlock(raw any) (map[string]any, error) {
	rawBlock, ok := raw.(map[string]any)
	if !ok {
		return nil, ErrInvalidInput
	}

	blockType, err := stringField(rawBlock, "type", 32)
	if err != nil || blockType == "" {
		return nil, ErrInvalidInput
	}
	if _, ok := allowedSitePageBlockTypes[blockType]; !ok {
		return nil, ErrInvalidInput
	}

	trackingKey, err := normalizeSitePageTrackingKey(rawBlock)
	if err != nil {
		return nil, err
	}
	block := map[string]any{"type": blockType, "tracking_key": trackingKey}
	switch blockType {
	case "hero":
		if err := copyStringField(block, rawBlock, "title", maxBlockTitleLen); err != nil {
			return nil, err
		}
		if err := copyStringField(block, rawBlock, "subtitle", maxBlockSubtitleLen); err != nil {
			return nil, err
		}
		if err := copyURLField(block, rawBlock, "backgroundImage"); err != nil {
			return nil, err
		}
		if err := copyStringField(block, rawBlock, "buttonText", maxBlockButtonTextLen); err != nil {
			return nil, err
		}
		if err := copyURLField(block, rawBlock, "buttonLink"); err != nil {
			return nil, err
		}
	case "hero_banner":
		if err := copyStringField(block, rawBlock, "title", maxBlockTitleLen); err != nil {
			return nil, err
		}
		if err := copyStringField(block, rawBlock, "subtitle", maxBlockSubtitleLen); err != nil {
			return nil, err
		}
		if err := copyURLField(block, rawBlock, "image_url"); err != nil {
			return nil, err
		}
		if err := copyURLField(block, rawBlock, "link_url"); err != nil {
			return nil, err
		}
		if err := copyStringField(block, rawBlock, "button_text", maxBlockButtonTextLen); err != nil {
			return nil, err
		}
	case "hero_carousel":
		if err := copyStringField(block, rawBlock, "title", maxBlockTitleLen); err != nil {
			return nil, err
		}
		autoplay, err := boolField(rawBlock, "autoplay", true)
		if err != nil {
			return nil, err
		}
		block["autoplay"] = autoplay
		interval, err := intFieldWithRange(rawBlock, "interval", 4000, 1000, 30000)
		if err != nil {
			return nil, err
		}
		block["interval"] = interval
		height, err := stringField(rawBlock, "height", 32)
		if err != nil {
			return nil, err
		}
		if height == "" {
			height = "large"
		}
		if height != "large" && height != "medium" && height != "small" {
			return nil, ErrInvalidInput
		}
		block["height"] = height
		fit, err := stringField(rawBlock, "fit", 16)
		if err != nil {
			return nil, err
		}
		if fit == "" {
			fit = "cover"
		}
		if fit != "cover" && fit != "contain" {
			return nil, ErrInvalidInput
		}
		block["fit"] = fit
		slides, err := heroCarouselSlides(rawBlock)
		if err != nil {
			return nil, err
		}
		block["slides"] = slides
	case "category_nav":
		if err := copyStringField(block, rawBlock, "title", maxBlockTitleLen); err != nil {
			return nil, err
		}
		if err := copyStringField(block, rawBlock, "subtitle", maxBlockSubtitleLen); err != nil {
			return nil, err
		}
		if err := copyUintField(block, rawBlock, "platform_category_id"); err != nil {
			return nil, err
		}
		if err := copyStringField(block, rawBlock, "platform_category_path", maxBlockTitleLen); err != nil {
			return nil, err
		}
		items, err := categoryNavItems(rawBlock)
		if err != nil {
			return nil, err
		}
		block["items"] = items
	case "map_section":
		enabled, err := boolField(rawBlock, "enabled", true)
		if err != nil {
			return nil, err
		}
		block["enabled"] = enabled
		for _, field := range []string{"title_zh", "title_en"} {
			if err := copyStringField(block, rawBlock, field, maxBlockTitleLen); err != nil {
				return nil, err
			}
		}
		for _, field := range []string{"subtitle_zh", "subtitle_en"} {
			if err := copyStringField(block, rawBlock, field, maxBlockSubtitleLen); err != nil {
				return nil, err
			}
		}
		if err := copyStringField(block, rawBlock, "place_query", maxBlockURLLen); err != nil {
			return nil, err
		}
		if err := copyStringField(block, rawBlock, "place_id", maxBlockSubtitleLen); err != nil {
			return nil, err
		}
		mapHeight, err := intFieldWithRange(rawBlock, "map_height", defaultMapHeight, minMapHeight, maxMapHeight)
		if err != nil {
			return nil, err
		}
		block["map_height"] = mapHeight
		if err := copyURLField(block, rawBlock, "external_map_url"); err != nil {
			return nil, err
		}
		sortOrder, err := intFieldWithRange(rawBlock, "sort_order", 0, 0, maxSitePageBlocks-1)
		if err != nil {
			return nil, err
		}
		block["sort_order"] = sortOrder
	case "text", "contact":
		if err := copyStringField(block, rawBlock, "title", maxBlockTitleLen); err != nil {
			return nil, err
		}
		if err := copyStringField(block, rawBlock, "content", maxBlockTextLen); err != nil {
			return nil, err
		}
		if blockType == "contact" {
			if err := copyStringField(block, rawBlock, "phone", maxBlockSubtitleLen); err != nil {
				return nil, err
			}
			if err := copyStringField(block, rawBlock, "whatsapp", maxBlockSubtitleLen); err != nil {
				return nil, err
			}
			if err := copyStringField(block, rawBlock, "address", maxBlockSubtitleLen); err != nil {
				return nil, err
			}
		}
	case "image":
		if err := copyStringField(block, rawBlock, "title", maxBlockTitleLen); err != nil {
			return nil, err
		}
		if err := copyURLField(block, rawBlock, "image"); err != nil {
			return nil, err
		}
		if err := copyStringField(block, rawBlock, "alt", maxBlockSubtitleLen); err != nil {
			return nil, err
		}
	case "image_text":
		if err := copyStringField(block, rawBlock, "title", maxBlockTitleLen); err != nil {
			return nil, err
		}
		if err := copyStringField(block, rawBlock, "content", maxBlockTextLen); err != nil {
			return nil, err
		}
		if err := copyURLField(block, rawBlock, "image"); err != nil {
			return nil, err
		}
		if err := copyStringField(block, rawBlock, "buttonText", maxBlockButtonTextLen); err != nil {
			return nil, err
		}
		if err := copyURLField(block, rawBlock, "buttonLink"); err != nil {
			return nil, err
		}
		layout, err := stringField(rawBlock, "layout", 32)
		if err != nil {
			return nil, err
		}
		if layout == "" {
			layout = "image_left"
		}
		if layout != "image_left" && layout != "image_right" {
			return nil, ErrInvalidInput
		}
		block["layout"] = layout
	case "feature_grid":
		if err := copyStringField(block, rawBlock, "title", maxBlockTitleLen); err != nil {
			return nil, err
		}
		if err := copyStringField(block, rawBlock, "subtitle", maxBlockSubtitleLen); err != nil {
			return nil, err
		}
		items, err := featureGridItems(rawBlock)
		if err != nil {
			return nil, err
		}
		block["items"] = items
	case "product_grid":
		if err := copyStringField(block, rawBlock, "title", maxBlockTitleLen); err != nil {
			return nil, err
		}
		if err := copyStringField(block, rawBlock, "subtitle", maxBlockSubtitleLen); err != nil {
			return nil, err
		}
		source, err := stringField(rawBlock, "source", 32)
		if err != nil {
			return nil, err
		}
		if source == "" {
			source = "latest"
		}
		if source != "latest" && source != "featured" && source != "hot" && source != "category" {
			return nil, ErrInvalidInput
		}
		block["source"] = source
		if err := copyUintField(block, rawBlock, "platform_category_id"); err != nil {
			return nil, err
		}
		if err := copyStringField(block, rawBlock, "platform_category_path", maxBlockTitleLen); err != nil {
			return nil, err
		}
		limit, err := intField(rawBlock, "limit", defaultGridLimit)
		if err != nil {
			return nil, err
		}
		block["limit"] = limit
	case "product_floor":
		if err := copyStringField(block, rawBlock, "title", maxBlockTitleLen); err != nil {
			return nil, err
		}
		if err := copyStringField(block, rawBlock, "subtitle", maxBlockSubtitleLen); err != nil {
			return nil, err
		}
		source, err := stringField(rawBlock, "source", 32)
		if err != nil {
			return nil, err
		}
		if source == "" {
			source = "all"
		}
		if source == "featured" {
			source = "t6"
		}
		if source == "latest" || source == "hot" || source == "category" {
			source = "all"
		}
		if source != "all" && source != "t6" && source != "merchant" {
			return nil, ErrInvalidInput
		}
		block["source"] = source
		if err := copyUintField(block, rawBlock, "platform_category_id"); err != nil {
			return nil, err
		}
		if err := copyStringField(block, rawBlock, "platform_category_path", maxBlockTitleLen); err != nil {
			return nil, err
		}
		sourceType, err := stringField(rawBlock, "source_type", 32)
		if err != nil {
			return nil, err
		}
		if sourceType == "" {
			if _, hasCategory := block["platform_category_id"]; hasCategory {
				sourceType = "category"
			} else {
				sourceType = "latest"
			}
		}
		if sourceType != "popular" && sourceType != "latest" &&
			sourceType != "category" && sourceType != "manual" {
			return nil, ErrInvalidInput
		}
		block["source_type"] = sourceType
		limit, err := intField(rawBlock, "limit", 10)
		if err != nil {
			return nil, err
		}
		block["limit"] = limit
		columns, err := intFieldWithRange(rawBlock, "columns", 4, 2, 6)
		if err != nil {
			return nil, err
		}
		block["columns"] = columns
		sort, err := stringField(rawBlock, "sort", 32)
		if err != nil {
			return nil, err
		}
		if sort == "" {
			sort = "latest"
		}
		if sort != "latest" && sort != "hot" {
			return nil, ErrInvalidInput
		}
		block["sort"] = sort
		enabled, err := boolField(rawBlock, "enabled", true)
		if err != nil {
			return nil, err
		}
		block["enabled"] = enabled
		showMoreLink, err := boolField(rawBlock, "show_more_link", true)
		if err != nil {
			return nil, err
		}
		block["show_more_link"] = showMoreLink
	case "shop_grid":
		if err := copyStringField(block, rawBlock, "title", maxBlockTitleLen); err != nil {
			return nil, err
		}
		if err := copyStringField(block, rawBlock, "subtitle", maxBlockSubtitleLen); err != nil {
			return nil, err
		}
		limit, err := intField(rawBlock, "limit", defaultGridLimit)
		if err != nil {
			return nil, err
		}
		block["limit"] = limit
	case "promo_banner":
		if err := copyStringField(block, rawBlock, "title", maxBlockTitleLen); err != nil {
			return nil, err
		}
		if err := copyStringField(block, rawBlock, "subtitle", maxBlockSubtitleLen); err != nil {
			return nil, err
		}
		if err := copyURLField(block, rawBlock, "image"); err != nil {
			return nil, err
		}
		if err := copyStringField(block, rawBlock, "buttonText", maxBlockButtonTextLen); err != nil {
			return nil, err
		}
		if err := copyURLField(block, rawBlock, "buttonLink"); err != nil {
			return nil, err
		}
		if err := copyURLField(block, rawBlock, "image_url"); err != nil {
			return nil, err
		}
		if err := copyURLField(block, rawBlock, "link_url"); err != nil {
			return nil, err
		}
	case "footer_links":
		if err := copyStringField(block, rawBlock, "title", maxBlockTitleLen); err != nil {
			return nil, err
		}
		groups, err := footerLinkGroups(rawBlock)
		if err != nil {
			return nil, err
		}
		block["items"] = groups
	}

	return block, nil
}

func categoryNavItems(src map[string]any) ([]map[string]any, error) {
	rawItems, err := blockItemsField(src)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(rawItems))
	for _, rawItem := range rawItems {
		item, ok := rawItem.(map[string]any)
		if !ok {
			return nil, ErrInvalidInput
		}
		trackingKey, err := normalizeSitePageTrackingKey(item)
		if err != nil {
			return nil, err
		}
		normalized := map[string]any{"tracking_key": trackingKey}
		if err := copyStringField(normalized, item, "name", maxBlockTitleLen); err != nil {
			return nil, err
		}
		if err := copyURLField(normalized, item, "image"); err != nil {
			return nil, err
		}
		if err := copyURLField(normalized, item, "link"); err != nil {
			return nil, err
		}
		items = append(items, normalized)
	}
	return items, nil
}

func featureGridItems(src map[string]any) ([]map[string]any, error) {
	rawItems, err := blockItemsField(src)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(rawItems))
	for _, rawItem := range rawItems {
		item, ok := rawItem.(map[string]any)
		if !ok {
			return nil, ErrInvalidInput
		}
		trackingKey, err := normalizeSitePageTrackingKey(item)
		if err != nil {
			return nil, err
		}
		normalized := map[string]any{"tracking_key": trackingKey}
		if err := copyStringField(normalized, item, "title", maxBlockTitleLen); err != nil {
			return nil, err
		}
		if err := copyStringField(normalized, item, "description", maxBlockTextLen); err != nil {
			return nil, err
		}
		if err := copyStringField(normalized, item, "icon", maxBlockButtonTextLen); err != nil {
			return nil, err
		}
		items = append(items, normalized)
	}
	return items, nil
}

func heroCarouselSlides(src map[string]any) ([]map[string]any, error) {
	value, ok := src["slides"]
	if !ok || value == nil {
		return []map[string]any{}, nil
	}
	rawSlides, ok := value.([]any)
	if !ok || len(rawSlides) > maxBlockItems {
		return nil, ErrInvalidInput
	}

	slides := make([]map[string]any, 0, len(rawSlides))
	for _, rawSlide := range rawSlides {
		slide, ok := rawSlide.(map[string]any)
		if !ok {
			return nil, ErrInvalidInput
		}
		trackingKey, err := normalizeSitePageTrackingKey(slide)
		if err != nil {
			return nil, err
		}
		normalized := map[string]any{"tracking_key": trackingKey}
		if err := copyURLField(normalized, slide, "image"); err != nil {
			return nil, err
		}
		if err := copyURLField(normalized, slide, "link"); err != nil {
			return nil, err
		}
		if err := copyStringField(normalized, slide, "title", maxBlockTitleLen); err != nil {
			return nil, err
		}
		if err := copyStringField(normalized, slide, "subtitle", maxBlockSubtitleLen); err != nil {
			return nil, err
		}
		if err := copyStringField(normalized, slide, "buttonText", maxBlockButtonTextLen); err != nil {
			return nil, err
		}
		enabled, err := boolField(slide, "enabled", true)
		if err != nil {
			return nil, err
		}
		normalized["enabled"] = enabled
		slides = append(slides, normalized)
	}
	return slides, nil
}

func footerLinkGroups(src map[string]any) ([]map[string]any, error) {
	rawGroups, err := blockItemsField(src)
	if err != nil {
		return nil, err
	}
	groups := make([]map[string]any, 0, len(rawGroups))
	for _, rawGroup := range rawGroups {
		group, ok := rawGroup.(map[string]any)
		if !ok {
			return nil, ErrInvalidInput
		}
		trackingKey, err := normalizeSitePageTrackingKey(group)
		if err != nil {
			return nil, err
		}
		normalized := map[string]any{"tracking_key": trackingKey}
		if err := copyStringField(normalized, group, "title", maxBlockTitleLen); err != nil {
			return nil, err
		}
		rawLinks, ok := group["links"]
		if !ok || rawLinks == nil {
			normalized["links"] = []map[string]any{}
			groups = append(groups, normalized)
			continue
		}
		links, ok := rawLinks.([]any)
		if !ok || len(links) > maxBlockItems {
			return nil, ErrInvalidInput
		}
		normalizedLinks := make([]map[string]any, 0, len(links))
		for _, rawLink := range links {
			link, ok := rawLink.(map[string]any)
			if !ok {
				return nil, ErrInvalidInput
			}
			trackingKey, err := normalizeSitePageTrackingKey(link)
			if err != nil {
				return nil, err
			}
			normalizedLink := map[string]any{"tracking_key": trackingKey}
			if err := copyStringField(normalizedLink, link, "name", maxBlockTitleLen); err != nil {
				return nil, err
			}
			if err := copyURLField(normalizedLink, link, "link"); err != nil {
				return nil, err
			}
			normalizedLinks = append(normalizedLinks, normalizedLink)
		}
		normalized["links"] = normalizedLinks
		groups = append(groups, normalized)
	}
	return groups, nil
}

func blockItemsField(src map[string]any) ([]any, error) {
	value, ok := src["items"]
	if !ok || value == nil {
		return []any{}, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, ErrInvalidInput
	}
	if len(items) > maxBlockItems {
		return nil, ErrInvalidInput
	}
	return items, nil
}

func copyUintField(dst map[string]any, src map[string]any, field string) error {
	value, err := uintField(src, field)
	if err != nil {
		return err
	}
	if value > 0 {
		dst[field] = value
	}
	return nil
}

func copyStringField(dst map[string]any, src map[string]any, field string, maxLen int) error {
	value, err := stringField(src, field, maxLen)
	if err != nil {
		return err
	}
	dst[field] = value
	return nil
}

func copyURLField(dst map[string]any, src map[string]any, field string) error {
	value, err := stringField(src, field, maxBlockURLLen)
	if err != nil {
		return err
	}
	if hasUnsafeURLProtocol(value) {
		return ErrInvalidInput
	}
	dst[field] = value
	return nil
}

func stringField(src map[string]any, field string, maxLen int) (string, error) {
	value, ok := src[field]
	if !ok || value == nil {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", ErrInvalidInput
	}
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) > maxLen {
		return "", ErrInvalidInput
	}
	return text, nil
}

func uintField(src map[string]any, field string) (uint, error) {
	value, ok := src[field]
	if !ok || value == nil {
		return 0, nil
	}

	var parsed uint64
	switch typed := value.(type) {
	case float64:
		if math.Trunc(typed) != typed || typed < 0 {
			return 0, ErrInvalidInput
		}
		parsed = uint64(typed)
	case int:
		if typed < 0 {
			return 0, ErrInvalidInput
		}
		parsed = uint64(typed)
	case uint:
		parsed = uint64(typed)
	default:
		return 0, ErrInvalidInput
	}
	if parsed > uint64(^uint(0)) {
		return 0, ErrInvalidInput
	}
	return uint(parsed), nil
}

func intField(src map[string]any, field string, fallback int) (int, error) {
	return intFieldWithRange(src, field, fallback, 1, maxGridLimit)
}

func intFieldWithRange(src map[string]any, field string, fallback int, min int, max int) (int, error) {
	value, ok := src[field]
	if !ok || value == nil {
		return fallback, nil
	}

	var parsed int
	switch typed := value.(type) {
	case float64:
		if math.Trunc(typed) != typed {
			return 0, ErrInvalidInput
		}
		parsed = int(typed)
	case int:
		parsed = typed
	default:
		return 0, ErrInvalidInput
	}
	if parsed < min || parsed > max {
		return 0, ErrInvalidInput
	}
	return parsed, nil
}

func boolField(src map[string]any, field string, fallback bool) (bool, error) {
	value, ok := src[field]
	if !ok || value == nil {
		return fallback, nil
	}
	parsed, ok := value.(bool)
	if !ok {
		return false, ErrInvalidInput
	}
	return parsed, nil
}

func hasUnsafeURLProtocol(value string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return false
	}

	colonIndex := strings.Index(trimmed, ":")
	if colonIndex < 0 {
		return false
	}
	for _, marker := range []string{"/", "?", "#"} {
		if markerIndex := strings.Index(trimmed, marker); markerIndex >= 0 && markerIndex < colonIndex {
			return false
		}
	}

	switch trimmed[:colonIndex] {
	case "javascript", "vbscript", "data":
		return true
	default:
		return false
	}
}

func (s *SitePageService) toSitePageAdminDTO(ctx context.Context, page *models.SitePage) (*SitePageAdminDTO, error) {
	draftContent, err := unmarshalSitePageContent(page.DraftContent)
	if err != nil {
		return nil, err
	}

	var publishedContent *SitePageContent
	if !isEmptyJSON(page.PublishedContent) {
		content, err := unmarshalSitePageContent(page.PublishedContent)
		if err != nil {
			return nil, err
		}
		publishedContent = &content
	}

	dto := &SitePageAdminDTO{
		PageKey:          page.PageKey,
		Title:            page.Title,
		DraftContent:     draftContent,
		PublishedContent: publishedContent,
		Status:           page.Status,
		PublishedAt:      page.PublishedAt,
	}

	if s.translations != nil && page.ID > 0 {
		rows, err := s.translations.GetTranslationsForEntity(ctx, TranslationEntitySitePage, page.ID, "zh")
		if err != nil {
			return nil, err
		}
		if content, ok := sitePageContentFromTranslations(rows, TranslationFieldDraftContent); ok {
			dto.Translations.Zh.DraftContent = &content
		}
		if content, ok := sitePageContentFromTranslations(rows, TranslationFieldPublishedContent); ok {
			dto.Translations.Zh.PublishedContent = &content
		}
	}

	return dto, nil
}

func marshalSitePageContent(content SitePageContent) (datatypes.JSON, error) {
	data, err := json.Marshal(content)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(data), nil
}

func unmarshalSitePageContent(data datatypes.JSON) (SitePageContent, error) {
	if isEmptyJSON(data) {
		return emptySitePageContent(), nil
	}
	var content SitePageContent
	if err := json.Unmarshal(data, &content); err != nil {
		return SitePageContent{}, err
	}
	if content.Blocks == nil {
		content.Blocks = []map[string]any{}
	}
	ensureLegacySitePageTrackingKeys(&content)
	return content, nil
}

func normalizeSitePageTrackingKey(node map[string]any) (string, error) {
	value, ok := node["tracking_key"]
	if ok && value != nil {
		trackingKey, ok := value.(string)
		if !ok {
			return "", ErrInvalidInput
		}
		trackingKey = strings.TrimSpace(trackingKey)
		if trackingKey != "" {
			if !trackingkey.IsValid(trackingKey) {
				return "", ErrInvalidInput
			}
			return trackingkey.Normalize(trackingKey), nil
		}
	}
	return trackingkey.New()
}

func ensureLegacySitePageTrackingKeys(content *SitePageContent) {
	for index, block := range content.Blocks {
		ensureLegacySitePageNodeTrackingKey(block, fmt.Sprintf("home.blocks.%d", index))
	}
}

func ensureLegacySitePageNodeTrackingKey(node map[string]any, path string) {
	trackingKey, _ := node["tracking_key"].(string)
	trackingKey = strings.TrimSpace(trackingKey)
	if !trackingkey.IsValid(trackingKey) {
		node["tracking_key"] = legacySitePageTrackingKey(path)
	} else {
		node["tracking_key"] = trackingkey.Normalize(trackingKey)
	}

	for _, field := range []string{"slides", "items", "links"} {
		for index, child := range sitePageMapList(node[field]) {
			ensureLegacySitePageNodeTrackingKey(child, fmt.Sprintf("%s.%s.%d", path, field, index))
		}
	}
}

func legacySitePageTrackingKey(path string) string {
	value := sha256.Sum256([]byte("ghana-chinatown.site-page.tracking-key:" + path))
	value[6] = (value[6] & 0x0f) | 0x50
	value[8] = (value[8] & 0x3f) | 0x80
	var uuid [16]byte
	copy(uuid[:], value[:16])
	return trackingkey.Format(uuid)
}

func emptySitePageContent() SitePageContent {
	return SitePageContent{Blocks: []map[string]any{}}
}

func isEmptyJSON(data datatypes.JSON) bool {
	trimmed := strings.TrimSpace(string(data))
	return trimmed == "" || trimmed == "null"
}
