package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"saas/internal/models"
	"saas/internal/repository"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type fakeSitePageRepository struct {
	pages          map[string]models.SitePage
	translations   *fakeTranslationReader
	pageWriteErr   error
	translationErr error
}

func newFakeSitePageRepository() *fakeSitePageRepository {
	return &fakeSitePageRepository{pages: make(map[string]models.SitePage)}
}

func (r *fakeSitePageRepository) FindByKey(_ context.Context, pageKey string) (*models.SitePage, error) {
	page, ok := r.pages[pageKey]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	cloned := page
	return &cloned, nil
}

func (r *fakeSitePageRepository) SaveDraftAtomic(
	_ context.Context,
	pageKey string,
	title string,
	content datatypes.JSON,
	userID uint,
	translation repository.SitePageTranslationMutation,
) (*models.SitePage, error) {
	rollback := r.beginSitePageMutation()
	committed := false
	defer func() {
		if !committed {
			rollback()
		}
	}()
	page := r.pages[pageKey]
	if page.ID == 0 {
		page.ID = uint(len(r.pages) + 1)
		page.PageKey = pageKey
		page.CreatedBy = &userID
	}
	page.Title = title
	page.DraftContent = append(datatypes.JSON(nil), content...)
	page.Status = models.SitePageStatusDraft
	page.UpdatedBy = &userID
	r.pages[pageKey] = page
	if r.pageWriteErr != nil {
		return nil, r.pageWriteErr
	}
	if err := r.saveSitePageTranslation(page.ID, TranslationFieldDraftContent, translation); err != nil {
		return nil, err
	}
	committed = true
	cloned := page
	return &cloned, nil
}

func (r *fakeSitePageRepository) PublishAtomic(
	_ context.Context,
	pageKey string,
	userID uint,
	buildTranslation repository.SitePagePublishTranslationBuilder,
) (*models.SitePage, error) {
	rollback := r.beginSitePageMutation()
	committed := false
	defer func() {
		if !committed {
			rollback()
		}
	}()
	page, ok := r.pages[pageKey]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	var zhDraft datatypes.JSON
	if r.translations != nil {
		for _, row := range r.translations.rows {
			if row.EntityType == TranslationEntitySitePage && row.EntityID == page.ID &&
				row.FieldKey == TranslationFieldDraftContent && row.Locale == "zh" {
				zhDraft = append(datatypes.JSON(nil), row.ValueJSON...)
			}
		}
	}
	publishedTranslation := repository.SitePageTranslationMutation{Provided: true}
	if buildTranslation != nil {
		localized, hasTranslation, err := buildTranslation(page.DraftContent, zhDraft)
		if err != nil {
			return nil, err
		}
		if hasTranslation {
			publishedTranslation.Content = localized
		}
	}
	page.PublishedContent = append(datatypes.JSON(nil), page.DraftContent...)
	page.Status = models.SitePageStatusPublished
	page.UpdatedBy = &userID
	publishedAt := time.Now()
	page.PublishedAt = &publishedAt
	r.pages[pageKey] = page
	if r.pageWriteErr != nil {
		return nil, r.pageWriteErr
	}
	if err := r.saveSitePageTranslation(page.ID, TranslationFieldPublishedContent, publishedTranslation); err != nil {
		return nil, err
	}
	committed = true
	cloned := page
	return &cloned, nil
}

func (r *fakeSitePageRepository) beginSitePageMutation() func() {
	pages := make(map[string]models.SitePage, len(r.pages))
	for key, page := range r.pages {
		page.DraftContent = append(datatypes.JSON(nil), page.DraftContent...)
		page.PublishedContent = append(datatypes.JSON(nil), page.PublishedContent...)
		pages[key] = page
	}
	var rows []models.ContentTranslation
	if r.translations != nil {
		rows = append([]models.ContentTranslation(nil), r.translations.rows...)
	}
	return func() {
		r.pages = pages
		if r.translations != nil {
			r.translations.rows = rows
		}
	}
}

func (r *fakeSitePageRepository) saveSitePageTranslation(
	pageID uint,
	fieldKey string,
	mutation repository.SitePageTranslationMutation,
) error {
	if mutation.Provided && r.translations != nil {
		if err := r.translations.SaveTranslationsForEntity(
			context.Background(), TranslationEntitySitePage, pageID, "zh",
			map[string]TranslationValueInput{fieldKey: {ValueJSON: mutation.Content}},
		); err != nil {
			return err
		}
	}
	return r.translationErr
}

func validHomePageInput(title string) SitePageDraftInput {
	return SitePageDraftInput{
		Title: title,
		Content: map[string]any{
			"blocks": []any{
				map[string]any{
					"type":            "hero",
					"title":           title,
					"subtitle":        "一站式本地商城服务",
					"backgroundImage": "/uploads/home.webp",
					"buttonText":      "开始购物",
					"buttonLink":      "/shops",
				},
			},
		},
	}
}

func TestSitePageServiceGetHomeAdminReturnsDefaultDraftWhenMissing(t *testing.T) {
	repo := newFakeSitePageRepository()
	service := NewSitePageService(repo)

	page, err := service.GetHomeAdmin(context.Background())
	if err != nil {
		t.Fatalf("GetHomeAdmin returned error: %v", err)
	}
	if page.PageKey != SitePageHomeKey {
		t.Fatalf("PageKey = %q, want %q", page.PageKey, SitePageHomeKey)
	}
	if page.Title != defaultSitePageTitle {
		t.Fatalf("Title = %q, want %q", page.Title, defaultSitePageTitle)
	}
	if page.Status != models.SitePageStatusDraft {
		t.Fatalf("Status = %q, want %q", page.Status, models.SitePageStatusDraft)
	}
	if len(page.DraftContent.Blocks) != 0 {
		t.Fatalf("DraftContent.Blocks len = %d, want 0", len(page.DraftContent.Blocks))
	}
	if page.PublishedContent != nil {
		t.Fatalf("PublishedContent = %+v, want nil", page.PublishedContent)
	}
	if _, err := service.PublicHome(context.Background()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("PublicHome error = %v, want ErrNotFound before publish", err)
	}
}

func TestSitePageTrackingKeysRemainStableAcrossEditsAndTranslations(t *testing.T) {
	repo := newFakeSitePageRepository()
	repo.translations = &fakeTranslationReader{}
	service := NewSitePageService(repo, SitePageOptions{Translations: repo.translations})
	firstKey := "11111111-1111-4111-8111-111111111111"
	secondKey := "22222222-2222-4222-8222-222222222222"
	localizedOverwrite := "99999999-9999-4999-8999-999999999999"

	input := SitePageDraftInput{
		Title: "Tracked Home",
		Content: map[string]any{"blocks": []any{
			map[string]any{"type": "hero", "tracking_key": firstKey, "title": "First", "buttonLink": "/mall"},
			map[string]any{"type": "promo_banner", "tracking_key": secondKey, "title": "Second", "buttonLink": "/shops"},
		}},
		Translations: &SitePageDraftTranslationsInput{Zh: &SitePageLocaleDraftInput{DraftContent: map[string]any{"blocks": []any{
			map[string]any{"type": "hero", "tracking_key": localizedOverwrite, "title": "第一"},
			map[string]any{"type": "promo_banner", "tracking_key": localizedOverwrite, "title": "第二"},
		}}}},
	}

	page, err := service.SaveHomeDraft(context.Background(), 1, input)
	if err != nil {
		t.Fatalf("SaveHomeDraft returned error: %v", err)
	}
	if page.DraftContent.Blocks[0]["tracking_key"] != firstKey || page.DraftContent.Blocks[1]["tracking_key"] != secondKey {
		t.Fatalf("english tracking keys = %+v", page.DraftContent.Blocks)
	}
	if page.Translations.Zh.DraftContent == nil ||
		page.Translations.Zh.DraftContent.Blocks[0]["tracking_key"] != firstKey ||
		page.Translations.Zh.DraftContent.Blocks[1]["tracking_key"] != secondKey {
		t.Fatalf("localized tracking keys did not inherit english keys: %+v", page.Translations.Zh.DraftContent)
	}

	input.Content = map[string]any{"blocks": []any{
		map[string]any{"type": "promo_banner", "tracking_key": secondKey, "title": "Renamed and moved", "buttonLink": "/mall"},
		map[string]any{"type": "hero", "tracking_key": firstKey, "title": "Also renamed", "buttonLink": "/shops"},
	}}
	input.Translations = nil
	page, err = service.SaveHomeDraft(context.Background(), 1, input)
	if err != nil {
		t.Fatalf("second SaveHomeDraft returned error: %v", err)
	}
	if page.DraftContent.Blocks[0]["tracking_key"] != secondKey || page.DraftContent.Blocks[1]["tracking_key"] != firstKey {
		t.Fatalf("tracking keys changed after title edit/reorder: %+v", page.DraftContent.Blocks)
	}
}

func TestSitePageServiceRepairsDuplicateTrackingKeysAcrossEntireTree(t *testing.T) {
	repo := newFakeSitePageRepository()
	repo.translations = &fakeTranslationReader{}
	service := NewSitePageService(repo, SitePageOptions{Translations: repo.translations})
	blockKey := "11111111-1111-4111-8111-111111111111"
	slideKey := "22222222-2222-4222-8222-222222222222"
	itemKey := "33333333-3333-4333-8333-333333333333"
	linkKey := "44444444-4444-4444-8444-444444444444"
	localizedKey := "99999999-9999-4999-8999-999999999999"

	input := SitePageDraftInput{
		Title: "Duplicate tracking keys",
		Content: map[string]any{"blocks": []any{
			map[string]any{"type": "hero", "tracking_key": blockKey, "title": "Hero"},
			map[string]any{"type": "hero_carousel", "tracking_key": blockKey, "title": "Carousel", "slides": []any{
				map[string]any{"tracking_key": slideKey, "title": "Slide one"},
				map[string]any{"tracking_key": slideKey, "title": "Slide two"},
			}},
			map[string]any{"type": "category_nav", "tracking_key": "55555555-5555-4555-8555-555555555555", "items": []any{
				map[string]any{"tracking_key": itemKey, "name": "Item one"},
				map[string]any{"tracking_key": itemKey, "name": "Item two"},
			}},
			map[string]any{"type": "footer_links", "tracking_key": "66666666-6666-4666-8666-666666666666", "items": []any{
				map[string]any{"tracking_key": "77777777-7777-4777-8777-777777777777", "title": "Group", "links": []any{
					map[string]any{"tracking_key": linkKey, "name": "Link one", "link": "/one"},
					map[string]any{"tracking_key": linkKey, "name": "Link two", "link": "/two"},
				}},
			}},
		}},
		Translations: &SitePageDraftTranslationsInput{Zh: &SitePageLocaleDraftInput{DraftContent: map[string]any{"blocks": []any{
			map[string]any{"type": "hero", "tracking_key": localizedKey, "title": "主图"},
			map[string]any{"type": "hero_carousel", "tracking_key": localizedKey, "title": "轮播", "slides": []any{
				map[string]any{"tracking_key": localizedKey, "title": "轮播一"},
				map[string]any{"tracking_key": localizedKey, "title": "轮播二"},
			}},
			map[string]any{"type": "category_nav", "tracking_key": localizedKey, "items": []any{
				map[string]any{"tracking_key": localizedKey, "name": "项目一"},
				map[string]any{"tracking_key": localizedKey, "name": "项目二"},
			}},
			map[string]any{"type": "footer_links", "tracking_key": localizedKey, "items": []any{
				map[string]any{"tracking_key": localizedKey, "title": "分组", "links": []any{
					map[string]any{"tracking_key": localizedKey, "name": "链接一"},
					map[string]any{"tracking_key": localizedKey, "name": "链接二"},
				}},
			}},
		}}}},
	}

	page, err := service.SaveHomeDraft(context.Background(), 1, input)
	if err != nil {
		t.Fatalf("SaveHomeDraft duplicate keys: %v", err)
	}
	keys := sitePageContentTrackingKeys(page.DraftContent)
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if !storefrontUUIDPattern.MatchString(key) {
			t.Fatalf("normalized tracking key = %q, want UUID", key)
		}
		if _, duplicate := seen[key]; duplicate {
			t.Fatalf("tracking key remained duplicated: %q in %v", key, keys)
		}
		seen[key] = struct{}{}
	}
	if keys[0] != blockKey || keys[2] != slideKey || keys[5] != itemKey || keys[9] != linkKey {
		t.Fatalf("first duplicate occurrences were not preserved: %v", keys)
	}
	if page.Translations.Zh.DraftContent == nil ||
		!reflect.DeepEqual(sitePageContentTrackingKeys(*page.Translations.Zh.DraftContent), keys) {
		t.Fatalf("localized tree did not inherit repaired English keys: %+v", page.Translations.Zh.DraftContent)
	}

	encoded, err := json.Marshal(map[string]any{"blocks": page.DraftContent.Blocks})
	if err != nil {
		t.Fatalf("marshal normalized draft: %v", err)
	}
	var normalizedInput map[string]any
	if err := json.Unmarshal(encoded, &normalizedInput); err != nil {
		t.Fatalf("unmarshal normalized draft: %v", err)
	}
	page, err = service.SaveHomeDraft(context.Background(), 1, SitePageDraftInput{Title: "Second pass", Content: normalizedInput})
	if err != nil {
		t.Fatalf("SaveHomeDraft second pass: %v", err)
	}
	if next := sitePageContentTrackingKeys(page.DraftContent); !reflect.DeepEqual(next, keys) {
		t.Fatalf("duplicate repair was not idempotent: first=%v second=%v", keys, next)
	}
}

func TestHistoricalSitePageTrackingKeysAreBackfilledDeterministically(t *testing.T) {
	legacy := datatypes.JSON(`{"blocks":[{"type":"hero_carousel","slides":[{"image":"/one.webp"}]}]}`)
	first, err := unmarshalSitePageContent(legacy)
	if err != nil {
		t.Fatalf("first unmarshal returned error: %v", err)
	}
	second, err := unmarshalSitePageContent(legacy)
	if err != nil {
		t.Fatalf("second unmarshal returned error: %v", err)
	}
	firstBlockKey, _ := first.Blocks[0]["tracking_key"].(string)
	secondBlockKey, _ := second.Blocks[0]["tracking_key"].(string)
	firstSlideKey, _ := sitePageMapList(first.Blocks[0]["slides"])[0]["tracking_key"].(string)
	secondSlideKey, _ := sitePageMapList(second.Blocks[0]["slides"])[0]["tracking_key"].(string)
	if !storefrontUUIDPattern.MatchString(firstBlockKey) || !storefrontUUIDPattern.MatchString(firstSlideKey) {
		t.Fatalf("backfilled keys are not UUIDs: block=%q slide=%q", firstBlockKey, firstSlideKey)
	}
	if firstBlockKey != secondBlockKey || firstSlideKey != secondSlideKey {
		t.Fatalf("historical keys are unstable: first=%q/%q second=%q/%q", firstBlockKey, firstSlideKey, secondBlockKey, secondSlideKey)
	}
}

func sitePageContentTrackingKeys(content SitePageContent) []string {
	keys := make([]string, 0)
	var collect func(map[string]any)
	collect = func(node map[string]any) {
		key, _ := node["tracking_key"].(string)
		keys = append(keys, key)
		for _, field := range []string{"slides", "items", "links"} {
			for _, child := range sitePageMapList(node[field]) {
				collect(child)
			}
		}
	}
	for _, block := range content.Blocks {
		collect(block)
	}
	return keys
}

func TestSitePageServiceGetHomeAdminReadsSavedDraft(t *testing.T) {
	repo := newFakeSitePageRepository()
	service := NewSitePageService(repo)

	if _, err := service.GetHomeAdmin(context.Background()); err != nil {
		t.Fatalf("initial GetHomeAdmin returned error: %v", err)
	}
	if _, err := service.SaveHomeDraft(context.Background(), 1, validHomePageInput("Saved Draft")); err != nil {
		t.Fatalf("SaveHomeDraft returned error: %v", err)
	}

	page, err := service.GetHomeAdmin(context.Background())
	if err != nil {
		t.Fatalf("GetHomeAdmin after save returned error: %v", err)
	}
	if page.Title != "Saved Draft" {
		t.Fatalf("Title = %q, want Saved Draft", page.Title)
	}
	if len(page.DraftContent.Blocks) != 1 {
		t.Fatalf("DraftContent.Blocks len = %d, want 1", len(page.DraftContent.Blocks))
	}
	if page.DraftContent.Blocks[0]["title"] != "Saved Draft" {
		t.Fatalf("draft block = %+v, want title Saved Draft", page.DraftContent.Blocks[0])
	}
	if _, err := service.PublicHome(context.Background()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("PublicHome error = %v, want ErrNotFound before publish", err)
	}
}

func TestSitePageServiceAcceptsEnhancedBlockTypes(t *testing.T) {
	repo := newFakeSitePageRepository()
	service := NewSitePageService(repo)

	input := SitePageDraftInput{
		Title: "Enhanced Home",
		Content: map[string]any{
			"blocks": []any{
				map[string]any{
					"type":            "hero",
					"title":           "Ghana Chinatown",
					"subtitle":        "一站式本地商城服务平台",
					"backgroundImage": "/uploads/home.webp",
					"buttonText":      "浏览商家",
					"buttonLink":      "/shops",
				},
				map[string]any{
					"type":  "category_nav",
					"title": "热门分类",
					"items": []any{
						map[string]any{"name": "推荐商家", "image": "/uploads/category.webp", "link": "/shops"},
					},
				},
				map[string]any{
					"type":       "image_text",
					"title":      "连接本地商家与消费者",
					"content":    "介绍内容",
					"image":      "/uploads/about.webp",
					"layout":     "image_right",
					"buttonText": "了解更多",
					"buttonLink": "/platform-home",
				},
				map[string]any{
					"type":     "feature_grid",
					"title":    "服务优势",
					"subtitle": "平台能力",
					"items": []any{
						map[string]any{"title": "商家展示", "description": "集中展示商家信息", "icon": "店"},
					},
				},
				map[string]any{"type": "product_grid", "title": "精选商品", "source": "featured", "limit": float64(8)},
				map[string]any{"type": "shop_grid", "title": "推荐商家", "limit": float64(6)},
				map[string]any{
					"type":       "promo_banner",
					"title":      "本地好物正在上新",
					"subtitle":   "发现更多平台活动",
					"image":      "/uploads/promo.webp",
					"buttonText": "立即查看",
					"buttonLink": "/shops",
				},
				map[string]any{
					"type":     "contact",
					"title":    "联系我们",
					"content":  "欢迎联系",
					"phone":    "+233 000 000",
					"whatsapp": "+233 000 001",
					"address":  "Accra",
				},
			},
		},
	}

	page, err := service.SaveHomeDraft(context.Background(), 1, input)
	if err != nil {
		t.Fatalf("SaveHomeDraft returned error: %v", err)
	}
	if len(page.DraftContent.Blocks) != 8 {
		t.Fatalf("DraftContent.Blocks len = %d, want 8", len(page.DraftContent.Blocks))
	}
	if page.DraftContent.Blocks[1]["type"] != "category_nav" {
		t.Fatalf("category block = %+v", page.DraftContent.Blocks[1])
	}

	if _, err := service.PublishHome(context.Background(), 1); err != nil {
		t.Fatalf("PublishHome returned error: %v", err)
	}
	publicPage, err := service.PublicHome(context.Background())
	if err != nil {
		t.Fatalf("PublicHome returned error: %v", err)
	}
	if len(publicPage.Content.Blocks) != 8 {
		t.Fatalf("public blocks len = %d, want 8", len(publicPage.Content.Blocks))
	}
	data, err := json.Marshal(publicPage)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	if jsonContainsKey(data, "draft_content") {
		t.Fatalf("public payload leaked draft_content: %s", string(data))
	}
}

func TestSitePageServiceAcceptsMapSectionWithoutPersistingAPIKey(t *testing.T) {
	repo := newFakeSitePageRepository()
	sitePageService := NewSitePageService(repo)
	input := SitePageDraftInput{
		Title: "Mapped Home",
		Content: map[string]any{
			"blocks": []any{
				map[string]any{
					"type":             "map_section",
					"enabled":          true,
					"title_zh":         "到访我们",
					"title_en":         "Visit Us",
					"subtitle_zh":      "查看地图",
					"subtitle_en":      "View the map",
					"place_query":      "Accra, Ghana",
					"place_id":         "ChIJ-Test-ID",
					"map_height":       float64(360),
					"external_map_url": "https://maps.google.com/?q=Accra",
					"sort_order":       float64(2),
					"api_key":          "must-not-be-persisted",
				},
			},
		},
	}

	page, err := sitePageService.SaveHomeDraft(context.Background(), 1, input)
	if err != nil {
		t.Fatalf("SaveHomeDraft returned error: %v", err)
	}
	if len(page.DraftContent.Blocks) != 1 {
		t.Fatalf("DraftContent.Blocks len = %d, want 1", len(page.DraftContent.Blocks))
	}
	block := page.DraftContent.Blocks[0]
	if block["type"] != "map_section" || block["enabled"] != true {
		t.Fatalf("map block type/enabled = %+v", block)
	}
	if block["title_zh"] != "到访我们" || block["title_en"] != "Visit Us" ||
		block["subtitle_zh"] != "查看地图" || block["subtitle_en"] != "View the map" {
		t.Fatalf("map localized text = %+v", block)
	}
	if block["place_query"] != "Accra, Ghana" || block["place_id"] != "ChIJ-Test-ID" {
		t.Fatalf("map place fields = %+v", block)
	}
	if block["map_height"] != float64(360) || block["sort_order"] != float64(2) {
		t.Fatalf("map numeric fields = %+v", block)
	}
	if block["external_map_url"] != "https://maps.google.com/?q=Accra" {
		t.Fatalf("map external URL = %v", block["external_map_url"])
	}
	if _, exists := block["api_key"]; exists {
		t.Fatalf("map block persisted an API key: %+v", block)
	}

	if _, err := sitePageService.PublishHome(context.Background(), 1); err != nil {
		t.Fatalf("PublishHome returned error: %v", err)
	}
	publicPage, err := sitePageService.PublicHomeWithLocale(context.Background(), "zh")
	if err != nil {
		t.Fatalf("PublicHomeWithLocale returned error: %v", err)
	}
	if publicPage.Content.Blocks[0]["place_query"] != "Accra, Ghana" ||
		publicPage.Content.Blocks[0]["title_zh"] != "到访我们" {
		t.Fatalf("published map block = %+v", publicPage.Content.Blocks[0])
	}
	if _, exists := publicPage.Content.Blocks[0]["api_key"]; exists {
		t.Fatalf("public map block leaked an API key: %+v", publicPage.Content.Blocks[0])
	}
}

func TestSitePageServiceRejectsInvalidMapSectionConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		update map[string]any
	}{
		{name: "height below range", update: map[string]any{"map_height": float64(199)}},
		{name: "height above range", update: map[string]any{"map_height": float64(801)}},
		{name: "sort order outside page", update: map[string]any{"sort_order": float64(50)}},
		{name: "unsafe external URL", update: map[string]any{"external_map_url": "javascript:alert(1)"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block := map[string]any{
				"type":        "map_section",
				"place_query": "Accra, Ghana",
			}
			for key, value := range tt.update {
				block[key] = value
			}
			input := SitePageDraftInput{
				Title:   "Invalid Map",
				Content: map[string]any{"blocks": []any{block}},
			}
			sitePageService := NewSitePageService(newFakeSitePageRepository())
			if _, err := sitePageService.SaveHomeDraft(context.Background(), 1, input); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("SaveHomeDraft error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestSitePageServiceAcceptsMallHomepageBlocks(t *testing.T) {
	repo := newFakeSitePageRepository()
	service := NewSitePageService(repo)

	input := SitePageDraftInput{
		Title: "Mall Home",
		Content: map[string]any{
			"blocks": []any{
				map[string]any{
					"type":     "hero_carousel",
					"title":    "首页轮播",
					"autoplay": true,
					"interval": float64(4500),
					"height":   "medium",
					"fit":      "cover",
					"slides": []any{
						map[string]any{
							"image":      "/uploads/banner-1.webp",
							"title":      "Legacy title",
							"subtitle":   "Legacy subtitle",
							"buttonText": "Shop now",
							"link":       "/mall",
							"enabled":    true,
						},
						map[string]any{
							"image":   "/uploads/banner-2.webp",
							"link":    "/mall?platform_category_id=5001",
							"enabled": false,
						},
					},
				},
				map[string]any{
					"type":        "hero_banner",
					"title":       "Tospino Style Mall",
					"subtitle":    "Daily deals and category picks",
					"image_url":   "/uploads/hero.webp",
					"link_url":    "/mall",
					"button_text": "Shop now",
				},
				map[string]any{
					"type":                   "product_floor",
					"title":                  "Phone Parts",
					"subtitle":               "Screens, batteries and accessories",
					"platform_category_id":   float64(5001),
					"platform_category_path": "Phone Parts",
					"limit":                  float64(10),
					"columns":                float64(5),
					"sort":                   "hot",
					"enabled":                true,
					"show_more_link":         false,
				},
				map[string]any{
					"type": "footer_links",
					"items": []any{
						map[string]any{
							"title": "Support",
							"links": []any{
								map[string]any{"name": "My Orders", "link": "/orders"},
							},
						},
					},
				},
			},
		},
	}

	page, err := service.SaveHomeDraft(context.Background(), 1, input)
	if err != nil {
		t.Fatalf("SaveHomeDraft returned error: %v", err)
	}
	if len(page.DraftContent.Blocks) != 4 {
		t.Fatalf("DraftContent.Blocks len = %d, want 4", len(page.DraftContent.Blocks))
	}

	carousel := page.DraftContent.Blocks[0]
	if carousel["type"] != "hero_carousel" || carousel["interval"] != float64(4500) || carousel["height"] != "medium" || carousel["fit"] != "cover" {
		t.Fatalf("hero carousel block = %+v", carousel)
	}
	carouselSlides, ok := carousel["slides"].([]any)
	if !ok || len(carouselSlides) != 2 {
		t.Fatalf("hero carousel slides = %+v, want 2 slides", carousel["slides"])
	}
	firstSlide, ok := carouselSlides[0].(map[string]any)
	if !ok {
		t.Fatalf("first carousel slide = %+v", carouselSlides[0])
	}
	if firstSlide["title"] != "Legacy title" || firstSlide["buttonText"] != "Shop now" || firstSlide["link"] != "/mall" {
		t.Fatalf("first carousel slide fields = %+v", firstSlide)
	}

	hero := page.DraftContent.Blocks[1]
	if hero["type"] != "hero_banner" || hero["image_url"] != "/uploads/hero.webp" || hero["button_text"] != "Shop now" {
		t.Fatalf("hero banner block = %+v", hero)
	}

	floor := page.DraftContent.Blocks[2]
	if floor["type"] != "product_floor" {
		t.Fatalf("floor type = %v, want product_floor", floor["type"])
	}
	if floor["platform_category_path"] != "Phone Parts" || floor["sort"] != "hot" {
		t.Fatalf("floor category/sort = %+v", floor)
	}
	if floor["source"] != "all" {
		t.Fatalf("floor source = %v, want all default", floor["source"])
	}
	if floor["source_type"] != "category" {
		t.Fatalf("legacy floor source_type = %v, want category compatibility", floor["source_type"])
	}
	if floor["platform_category_id"] != float64(5001) || floor["limit"] != float64(10) || floor["columns"] != float64(5) {
		t.Fatalf("floor numeric config = %+v", floor)
	}
	if floor["enabled"] != true || floor["show_more_link"] != false {
		t.Fatalf("floor flags = %+v", floor)
	}

	footer := page.DraftContent.Blocks[3]
	groups, ok := footer["items"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("footer groups = %+v, want one group", footer["items"])
	}
	group, ok := groups[0].(map[string]any)
	if !ok || group["title"] != "Support" {
		t.Fatalf("footer group = %+v", groups[0])
	}
	links, ok := group["links"].([]any)
	if !ok || len(links) != 1 {
		t.Fatalf("footer links = %+v, want one link", group["links"])
	}

	if _, err := service.PublishHome(context.Background(), 1); err != nil {
		t.Fatalf("PublishHome returned error: %v", err)
	}
	publicPage, err := service.PublicHome(context.Background())
	if err != nil {
		t.Fatalf("PublicHome returned error: %v", err)
	}
	if len(publicPage.Content.Blocks) != 4 || publicPage.Content.Blocks[0]["type"] != "hero_carousel" {
		t.Fatalf("public blocks = %+v", publicPage.Content.Blocks)
	}
	if publicPage.Content.Blocks[0]["fit"] != "cover" {
		t.Fatalf("published carousel fit = %v, want cover", publicPage.Content.Blocks[0]["fit"])
	}
}

func TestProductFloorSourceTypeNormalizationAndLegacyCompatibility(t *testing.T) {
	tests := []struct {
		name       string
		block      map[string]any
		wantSource string
		wantErr    bool
	}{
		{name: "legacy uncategorized stays latest", block: map[string]any{
			"type": "product_floor",
		}, wantSource: "latest"},
		{name: "legacy category stays category", block: map[string]any{
			"type": "product_floor", "platform_category_id": float64(7),
		}, wantSource: "category"},
		{name: "explicit popular", block: map[string]any{
			"type": "product_floor", "source_type": "popular",
		}, wantSource: "popular"},
		{name: "manual enum is preserved", block: map[string]any{
			"type": "product_floor", "source_type": "manual",
		}, wantSource: "manual"},
		{name: "invalid source", block: map[string]any{
			"type": "product_floor", "source_type": "invented",
		}, wantErr: true},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			normalized, err := normalizeSitePageBlock(testCase.block)
			if testCase.wantErr {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("normalizeSitePageBlock() error = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if normalized["source_type"] != testCase.wantSource {
				t.Fatalf("source_type = %v, want %q", normalized["source_type"], testCase.wantSource)
			}
		})
	}
}

func TestSitePageServiceDefaultsMissingHeroCarouselFitToCoverAndRejectsInvalidFit(t *testing.T) {
	repo := newFakeSitePageRepository()
	sitePageService := NewSitePageService(repo)

	input := SitePageDraftInput{
		Title: "Carousel Fit",
		Content: map[string]any{
			"blocks": []any{
				map[string]any{
					"type": "hero_carousel",
					"slides": []any{
						map[string]any{"image": "/uploads/banner.webp", "enabled": true},
					},
				},
			},
		},
	}
	page, err := sitePageService.SaveHomeDraft(context.Background(), 1, input)
	if err != nil {
		t.Fatalf("SaveHomeDraft returned error: %v", err)
	}
	if page.DraftContent.Blocks[0]["fit"] != "cover" {
		t.Fatalf("default fit = %v, want cover", page.DraftContent.Blocks[0]["fit"])
	}

	carousel := input.Content["blocks"].([]any)[0].(map[string]any)
	carousel["fit"] = "contain"
	page, err = sitePageService.SaveHomeDraft(context.Background(), 1, input)
	if err != nil {
		t.Fatalf("SaveHomeDraft contain returned error: %v", err)
	}
	if page.DraftContent.Blocks[0]["fit"] != "contain" {
		t.Fatalf("contain fit = %v, want contain", page.DraftContent.Blocks[0]["fit"])
	}

	carousel["fit"] = "stretch"
	if _, err := sitePageService.SaveHomeDraft(context.Background(), 1, input); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("SaveHomeDraft invalid fit error = %v, want ErrInvalidInput", err)
	}
}

func TestSitePageServiceRejectsUnsafeHeroCarouselSlideLink(t *testing.T) {
	service := NewSitePageService(newFakeSitePageRepository())
	input := SitePageDraftInput{
		Title: "Unsafe Carousel Link",
		Content: map[string]any{
			"blocks": []any{
				map[string]any{
					"type": "hero_carousel",
					"slides": []any{
						map[string]any{
							"image":   "/uploads/banner.webp",
							"link":    "javascript:alert(1)",
							"enabled": true,
						},
					},
				},
			},
		},
	}

	if _, err := service.SaveHomeDraft(context.Background(), 1, input); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("SaveHomeDraft error = %v, want ErrInvalidInput", err)
	}
}

func TestSitePageServiceSaveDraftDoesNotPublish(t *testing.T) {
	repo := newFakeSitePageRepository()
	service := NewSitePageService(repo)

	if _, err := service.SaveHomeDraft(context.Background(), 1, validHomePageInput("Draft Title")); err != nil {
		t.Fatalf("SaveHomeDraft returned error: %v", err)
	}
	if _, err := service.PublicHome(context.Background()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("PublicHome error = %v, want ErrNotFound before publish", err)
	}
}

func TestSitePageServicePublishExposesPublishedContent(t *testing.T) {
	repo := newFakeSitePageRepository()
	service := NewSitePageService(repo)

	if _, err := service.SaveHomeDraft(context.Background(), 1, validHomePageInput("Live Title")); err != nil {
		t.Fatalf("SaveHomeDraft returned error: %v", err)
	}
	if _, err := service.PublishHome(context.Background(), 1); err != nil {
		t.Fatalf("PublishHome returned error: %v", err)
	}

	publicPage, err := service.PublicHome(context.Background())
	if err != nil {
		t.Fatalf("PublicHome returned error: %v", err)
	}
	if publicPage.Content.Blocks[0]["title"] != "Live Title" {
		t.Fatalf("public block = %+v, want Live Title", publicPage.Content.Blocks[0])
	}

	data, err := json.Marshal(publicPage)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	if string(data) == "" || jsonContainsKey(data, "draft_content") {
		t.Fatalf("public payload leaked draft_content: %s", string(data))
	}
}

func TestSitePageServicePublicHomeUsesPublishedCMSLocaleTranslation(t *testing.T) {
	repo := newFakeSitePageRepository()
	translations := &fakeTranslationReader{}
	repo.translations = translations
	service := NewSitePageService(repo, SitePageOptions{Translations: translations})

	input := SitePageDraftInput{
		Title: "Localized Home",
		Content: map[string]any{
			"blocks": []any{
				map[string]any{
					"type":     "hero_carousel",
					"title":    "Homepage Deals",
					"autoplay": true,
					"interval": float64(4500),
					"height":   "medium",
					"fit":      "cover",
					"slides": []any{
						map[string]any{
							"image":      "/uploads/banner.webp",
							"title":      "English Slide",
							"subtitle":   "English subtitle",
							"buttonText": "Shop now",
							"link":       "/mall?source=t6",
							"enabled":    true,
						},
					},
				},
				map[string]any{
					"type":                 "product_floor",
					"title":                "T6 Picks",
					"subtitle":             "English subtitle",
					"source":               "t6",
					"platform_category_id": float64(88),
					"limit":                float64(10),
					"columns":              float64(5),
					"sort":                 "hot",
					"enabled":              true,
					"show_more_link":       true,
				},
			},
		},
		Translations: &SitePageDraftTranslationsInput{
			Zh: &SitePageLocaleDraftInput{
				DraftContent: map[string]any{
					"blocks": []any{
						map[string]any{
							"type":     "hero_carousel",
							"title":    "首页精选",
							"autoplay": false,
							"interval": float64(1000),
							"height":   "small",
							"fit":      "contain",
							"slides": []any{
								map[string]any{
									"image":      "/uploads/changed.webp",
									"title":      "中文轮播",
									"subtitle":   "",
									"buttonText": "立即查看",
									"link":       "/shops/changed",
									"enabled":    false,
								},
							},
						},
						map[string]any{
							"type":                 "product_floor",
							"title":                "T6 中文精选",
							"subtitle":             "",
							"source":               "merchant",
							"platform_category_id": float64(999),
							"limit":                float64(1),
							"columns":              float64(2),
							"sort":                 "latest",
							"enabled":              false,
							"show_more_link":       false,
						},
					},
				},
			},
		},
	}

	if _, err := service.SaveHomeDraft(context.Background(), 1, input); err != nil {
		t.Fatalf("SaveHomeDraft returned error: %v", err)
	}
	if _, err := service.PublishHome(context.Background(), 1); err != nil {
		t.Fatalf("PublishHome returned error: %v", err)
	}

	zhPage, err := service.PublicHomeWithLocale(context.Background(), "zh")
	if err != nil {
		t.Fatalf("PublicHomeWithLocale zh returned error: %v", err)
	}
	if zhPage.Content.Blocks[0]["title"] != "首页精选" {
		t.Fatalf("zh hero title = %v, want 首页精选", zhPage.Content.Blocks[0]["title"])
	}
	slides := zhPage.Content.Blocks[0]["slides"].([]map[string]any)
	if slides[0]["title"] != "中文轮播" || slides[0]["subtitle"] != "English subtitle" || slides[0]["buttonText"] != "立即查看" {
		t.Fatalf("zh slide text fields = %+v", slides[0])
	}
	if slides[0]["image"] != "/uploads/banner.webp" || slides[0]["link"] != "/mall?source=t6" || slides[0]["enabled"] != true {
		t.Fatalf("zh slide structure fields = %+v", slides[0])
	}

	floor := zhPage.Content.Blocks[1]
	if floor["title"] != "T6 中文精选" || floor["subtitle"] != "English subtitle" {
		t.Fatalf("zh floor text fields = %+v", floor)
	}
	if floor["source"] != "t6" || floor["platform_category_id"] != float64(88) || floor["limit"] != float64(10) || floor["columns"] != float64(5) || floor["sort"] != "hot" || floor["enabled"] != true {
		t.Fatalf("zh floor structure fields were not protected: %+v", floor)
	}

	enPage, err := service.PublicHomeWithLocale(context.Background(), "en")
	if err != nil {
		t.Fatalf("PublicHomeWithLocale en returned error: %v", err)
	}
	if enPage.Content.Blocks[0]["title"] != "Homepage Deals" {
		t.Fatalf("en hero title = %v, want Homepage Deals", enPage.Content.Blocks[0]["title"])
	}
	badLocalePage, err := service.PublicHomeWithLocale(context.Background(), "bad")
	if err != nil {
		t.Fatalf("PublicHomeWithLocale bad returned error: %v", err)
	}
	if badLocalePage.Content.Blocks[0]["title"] != "Homepage Deals" {
		t.Fatalf("bad locale title = %v, want English fallback", badLocalePage.Content.Blocks[0]["title"])
	}
}

func TestSitePageServiceClearsCMSLocaleTranslationAndFallsBackToPublishedEnglish(t *testing.T) {
	repo := newFakeSitePageRepository()
	translations := &fakeTranslationReader{}
	repo.translations = translations
	service := NewSitePageService(repo, SitePageOptions{Translations: translations})

	input := validHomePageInput("English Home")
	input.Translations = &SitePageDraftTranslationsInput{
		Zh: &SitePageLocaleDraftInput{
			DraftContent: map[string]any{
				"blocks": []any{
					map[string]any{
						"type":            "hero",
						"title":           "中文首页",
						"subtitle":        "",
						"backgroundImage": "/uploads/zh.webp",
						"buttonText":      "进入",
						"buttonLink":      "/changed",
					},
				},
			},
		},
	}
	if _, err := service.SaveHomeDraft(context.Background(), 1, input); err != nil {
		t.Fatalf("SaveHomeDraft zh returned error: %v", err)
	}
	if _, err := service.PublishHome(context.Background(), 1); err != nil {
		t.Fatalf("PublishHome zh returned error: %v", err)
	}
	page, err := service.PublicHomeWithLocale(context.Background(), "zh")
	if err != nil {
		t.Fatalf("PublicHomeWithLocale zh returned error: %v", err)
	}
	if page.Content.Blocks[0]["title"] != "中文首页" {
		t.Fatalf("zh title = %v, want 中文首页", page.Content.Blocks[0]["title"])
	}

	input.Translations.Zh.DraftContent = map[string]any{
		"blocks": []any{
			map[string]any{
				"type":            "hero",
				"title":           " ",
				"subtitle":        " ",
				"backgroundImage": "/uploads/zh.webp",
				"buttonText":      " ",
				"buttonLink":      "/changed",
			},
		},
	}
	if _, err := service.SaveHomeDraft(context.Background(), 1, input); err != nil {
		t.Fatalf("SaveHomeDraft clear zh returned error: %v", err)
	}
	if _, err := service.PublishHome(context.Background(), 1); err != nil {
		t.Fatalf("PublishHome clear zh returned error: %v", err)
	}
	page, err = service.PublicHomeWithLocale(context.Background(), "zh")
	if err != nil {
		t.Fatalf("PublicHomeWithLocale zh after clear returned error: %v", err)
	}
	if page.Content.Blocks[0]["title"] != "English Home" {
		t.Fatalf("zh title after clear = %v, want English fallback", page.Content.Blocks[0]["title"])
	}
}

func TestSitePageServiceFakeRepositoryRollsBackTranslationFailure(t *testing.T) {
	repo := newFakeSitePageRepository()
	translations := &fakeTranslationReader{}
	repo.translations = translations
	service := NewSitePageService(repo, SitePageOptions{Translations: translations})
	input := validHomePageInput("Old English")
	input.Translations = &SitePageDraftTranslationsInput{Zh: &SitePageLocaleDraftInput{DraftContent: map[string]any{
		"blocks": []any{map[string]any{"type": "hero", "title": "旧中文"}},
	}}}
	if _, err := service.SaveHomeDraft(context.Background(), 1, input); err != nil {
		t.Fatalf("seed translated draft: %v", err)
	}
	pageBefore := repo.pages[SitePageHomeKey]
	rowsBefore := append([]models.ContentTranslation(nil), translations.rows...)
	repo.translationErr = errors.New("forced fake translation failure")
	input = validHomePageInput("New English")
	input.Translations = &SitePageDraftTranslationsInput{Zh: &SitePageLocaleDraftInput{DraftContent: map[string]any{
		"blocks": []any{map[string]any{"type": "hero", "title": "新中文"}},
	}}}

	if _, err := service.SaveHomeDraft(context.Background(), 2, input); err == nil {
		t.Fatal("SaveHomeDraft returned nil error for translation failure")
	}
	if !reflect.DeepEqual(repo.pages[SitePageHomeKey], pageBefore) {
		t.Fatalf("page changed after rollback\nbefore=%+v\nafter=%+v", pageBefore, repo.pages[SitePageHomeKey])
	}
	if !reflect.DeepEqual(translations.rows, rowsBefore) {
		t.Fatalf("translations changed after rollback\nbefore=%+v\nafter=%+v", rowsBefore, translations.rows)
	}
}

func TestSitePageServiceDraftUpdateDoesNotChangePublishedContent(t *testing.T) {
	repo := newFakeSitePageRepository()
	service := NewSitePageService(repo)

	if _, err := service.SaveHomeDraft(context.Background(), 1, validHomePageInput("Published Title")); err != nil {
		t.Fatalf("SaveHomeDraft returned error: %v", err)
	}
	if _, err := service.PublishHome(context.Background(), 1); err != nil {
		t.Fatalf("PublishHome returned error: %v", err)
	}
	if _, err := service.SaveHomeDraft(context.Background(), 1, validHomePageInput("Unpublished Draft")); err != nil {
		t.Fatalf("SaveHomeDraft update returned error: %v", err)
	}

	publicPage, err := service.PublicHome(context.Background())
	if err != nil {
		t.Fatalf("PublicHome returned error: %v", err)
	}
	if publicPage.Content.Blocks[0]["title"] != "Published Title" {
		t.Fatalf("public block = %+v, want previous published title", publicPage.Content.Blocks[0])
	}
}

func TestSitePageServiceRejectsInvalidBlockType(t *testing.T) {
	service := NewSitePageService(newFakeSitePageRepository())
	input := validHomePageInput("Invalid")
	input.Content["blocks"] = []any{map[string]any{"type": "html", "content": "<script>alert(1)</script>"}}

	if _, err := service.SaveHomeDraft(context.Background(), 1, input); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("SaveHomeDraft error = %v, want ErrInvalidInput", err)
	}
}

func TestSitePageServiceRejectsJavascriptURL(t *testing.T) {
	service := NewSitePageService(newFakeSitePageRepository())
	input := validHomePageInput("Unsafe")
	input.Content["blocks"] = []any{map[string]any{
		"type":       "hero",
		"title":      "Unsafe",
		"buttonLink": "javascript:alert(1)",
	}}

	if _, err := service.SaveHomeDraft(context.Background(), 1, input); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("SaveHomeDraft error = %v, want ErrInvalidInput", err)
	}
}

func TestSitePageServiceRejectsJavascriptURLInNestedItems(t *testing.T) {
	service := NewSitePageService(newFakeSitePageRepository())
	input := SitePageDraftInput{
		Title: "Unsafe Nested Link",
		Content: map[string]any{
			"blocks": []any{
				map[string]any{
					"type":  "category_nav",
					"title": "分类",
					"items": []any{
						map[string]any{"name": "Unsafe", "link": "javascript:alert(1)"},
					},
				},
			},
		},
	}

	if _, err := service.SaveHomeDraft(context.Background(), 1, input); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("SaveHomeDraft error = %v, want ErrInvalidInput", err)
	}
}

func jsonContainsKey(data []byte, key string) bool {
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return false
	}
	_, ok := payload[key]
	return ok
}
