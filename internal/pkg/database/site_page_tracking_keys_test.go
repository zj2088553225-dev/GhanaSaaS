package database

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"saas/internal/models"
	"saas/internal/pkg/trackingkey"

	"github.com/glebarez/sqlite"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func TestBackfillSitePageTrackingKeysPersistsSharedStableKeys(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.SitePage{}, &models.ContentTranslation{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	english := datatypes.JSON(`{"blocks":[{"type":"hero_carousel","title":"English","slides":[{"title":"Slide"}]}]}`)
	chinese := datatypes.JSON(`{"blocks":[{"type":"hero_carousel","title":"中文","slides":[{"title":"轮播"}]}]}`)
	page := models.SitePage{
		PageKey:          "home",
		Title:            "Home",
		DraftContent:     english,
		PublishedContent: english,
		Status:           models.SitePageStatusPublished,
	}
	if err := db.Create(&page).Error; err != nil {
		t.Fatalf("create page: %v", err)
	}
	translations := []models.ContentTranslation{
		{EntityType: sitePageTranslationEntityType, EntityID: page.ID, FieldKey: sitePageDraftContentField, Locale: "zh", ValueJSON: chinese, Status: models.ContentTranslationStatusPublished},
		{EntityType: sitePageTranslationEntityType, EntityID: page.ID, FieldKey: sitePagePublishedContentField, Locale: "zh", ValueJSON: chinese, Status: models.ContentTranslationStatusPublished},
	}
	if err := db.Create(&translations).Error; err != nil {
		t.Fatalf("create translations: %v", err)
	}

	if err := BackfillSitePageTrackingKeys(db); err != nil {
		t.Fatalf("BackfillSitePageTrackingKeys: %v", err)
	}
	if err := db.First(&page, page.ID).Error; err != nil {
		t.Fatalf("reload page: %v", err)
	}
	if err := db.Where("entity_type = ? AND entity_id = ?", sitePageTranslationEntityType, page.ID).
		Order("field_key ASC").Find(&translations).Error; err != nil {
		t.Fatalf("reload translations: %v", err)
	}

	draftBlockKey := sitePageTestTrackingKey(t, page.DraftContent, 0)
	publishedBlockKey := sitePageTestTrackingKey(t, page.PublishedContent, 0)
	draftSlideKey := sitePageTestNestedTrackingKey(t, page.DraftContent, 0, "slides", 0)
	publishedSlideKey := sitePageTestNestedTrackingKey(t, page.PublishedContent, 0, "slides", 0)
	if !trackingkey.IsValid(draftBlockKey) || !trackingkey.IsValid(draftSlideKey) {
		t.Fatalf("generated invalid keys: block=%q slide=%q", draftBlockKey, draftSlideKey)
	}
	if publishedBlockKey != draftBlockKey || publishedSlideKey != draftSlideKey {
		t.Fatalf("published keys differ from draft: %q/%q vs %q/%q", publishedBlockKey, publishedSlideKey, draftBlockKey, draftSlideKey)
	}
	for _, translation := range translations {
		baseBlockKey := draftBlockKey
		baseSlideKey := draftSlideKey
		if translation.FieldKey == sitePagePublishedContentField {
			baseBlockKey = publishedBlockKey
			baseSlideKey = publishedSlideKey
		}
		if got := sitePageTestTrackingKey(t, translation.ValueJSON, 0); got != baseBlockKey {
			t.Errorf("%s localized block key = %q, want %q", translation.FieldKey, got, baseBlockKey)
		}
		if got := sitePageTestNestedTrackingKey(t, translation.ValueJSON, 0, "slides", 0); got != baseSlideKey {
			t.Errorf("%s localized slide key = %q, want %q", translation.FieldKey, got, baseSlideKey)
		}
	}

	draftSnapshot := string(page.DraftContent)
	publishedSnapshot := string(page.PublishedContent)
	if err := BackfillSitePageTrackingKeys(db); err != nil {
		t.Fatalf("second BackfillSitePageTrackingKeys: %v", err)
	}
	if err := db.First(&page, page.ID).Error; err != nil {
		t.Fatalf("reload page after second backfill: %v", err)
	}
	if string(page.DraftContent) != draftSnapshot || string(page.PublishedContent) != publishedSnapshot {
		t.Fatal("second backfill regenerated stable tracking keys")
	}
}

func TestBackfillSitePageTrackingKeysRepairsHistoricalDuplicatesIdempotently(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.SitePage{}, &models.ContentTranslation{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	blockKey := "11111111-1111-4111-8111-111111111111"
	slideKey := "22222222-2222-4222-8222-222222222222"
	itemKey := "33333333-3333-4333-8333-333333333333"
	linkKey := "44444444-4444-4444-8444-444444444444"
	english := datatypes.JSON(fmt.Sprintf(`{"blocks":[
		{"type":"hero","tracking_key":%q,"title":"Hero"},
		{"type":"hero_carousel","tracking_key":%q,"slides":[
			{"tracking_key":%q,"title":"One"},{"tracking_key":%q,"title":"Two"}]},
		{"type":"category_nav","tracking_key":"55555555-5555-4555-8555-555555555555","items":[
			{"tracking_key":%q,"name":"One"},{"tracking_key":%q,"name":"Two"}]},
		{"type":"footer_links","tracking_key":"66666666-6666-4666-8666-666666666666","items":[
			{"tracking_key":"77777777-7777-4777-8777-777777777777","links":[
				{"tracking_key":%q,"name":"One"},{"tracking_key":%q,"name":"Two"}]}]}
	]}`, blockKey, blockKey, slideKey, slideKey, itemKey, itemKey, linkKey, linkKey))
	localized := datatypes.JSON(`{"blocks":[
		{"type":"hero","tracking_key":"99999999-9999-4999-8999-999999999999","title":"主图"},
		{"type":"hero_carousel","tracking_key":"99999999-9999-4999-8999-999999999999","slides":[
			{"tracking_key":"99999999-9999-4999-8999-999999999999","title":"一"},
			{"tracking_key":"99999999-9999-4999-8999-999999999999","title":"二"}]},
		{"type":"category_nav","tracking_key":"99999999-9999-4999-8999-999999999999","items":[
			{"tracking_key":"99999999-9999-4999-8999-999999999999","name":"一"},
			{"tracking_key":"99999999-9999-4999-8999-999999999999","name":"二"}]},
		{"type":"footer_links","tracking_key":"99999999-9999-4999-8999-999999999999","items":[
			{"tracking_key":"99999999-9999-4999-8999-999999999999","links":[
				{"tracking_key":"99999999-9999-4999-8999-999999999999","name":"一"},
				{"tracking_key":"99999999-9999-4999-8999-999999999999","name":"二"}]}]}
	]}`)
	page := models.SitePage{
		PageKey:          "home",
		Title:            "Home",
		DraftContent:     english,
		PublishedContent: english,
		Status:           models.SitePageStatusPublished,
	}
	if err := db.Create(&page).Error; err != nil {
		t.Fatalf("create page: %v", err)
	}
	translations := []models.ContentTranslation{
		{EntityType: sitePageTranslationEntityType, EntityID: page.ID, FieldKey: sitePageDraftContentField, Locale: "zh", ValueJSON: localized},
		{EntityType: sitePageTranslationEntityType, EntityID: page.ID, FieldKey: sitePagePublishedContentField, Locale: "zh", ValueJSON: localized},
	}
	if err := db.Create(&translations).Error; err != nil {
		t.Fatalf("create translations: %v", err)
	}

	if err := BackfillSitePageTrackingKeys(db); err != nil {
		t.Fatalf("BackfillSitePageTrackingKeys: %v", err)
	}
	if err := db.First(&page, page.ID).Error; err != nil {
		t.Fatalf("reload page: %v", err)
	}
	if err := db.Where("entity_type = ? AND entity_id = ?", sitePageTranslationEntityType, page.ID).
		Order("field_key ASC").Find(&translations).Error; err != nil {
		t.Fatalf("reload translations: %v", err)
	}

	draftKeys := sitePageTestAllTrackingKeys(t, page.DraftContent)
	publishedKeys := sitePageTestAllTrackingKeys(t, page.PublishedContent)
	if !sitePageTestUniqueTrackingKeys(draftKeys) {
		t.Fatalf("draft keys are not globally unique: %v", draftKeys)
	}
	if !sitePageTestUniqueTrackingKeys(publishedKeys) {
		t.Fatalf("published keys are not globally unique: %v", publishedKeys)
	}
	if !reflect.DeepEqual(publishedKeys, draftKeys) {
		t.Fatalf("published keys do not inherit repaired draft keys: published=%v draft=%v", publishedKeys, draftKeys)
	}
	if draftKeys[0] != blockKey || draftKeys[2] != slideKey || draftKeys[5] != itemKey || draftKeys[9] != linkKey {
		t.Fatalf("first duplicate occurrences were not preserved: %v", draftKeys)
	}
	for _, translation := range translations {
		if got := sitePageTestAllTrackingKeys(t, translation.ValueJSON); !reflect.DeepEqual(got, draftKeys) {
			t.Fatalf("%s localized keys=%v, want %v", translation.FieldKey, got, draftKeys)
		}
	}

	draftSnapshot := string(page.DraftContent)
	publishedSnapshot := string(page.PublishedContent)
	translationSnapshots := []string{string(translations[0].ValueJSON), string(translations[1].ValueJSON)}
	if err := BackfillSitePageTrackingKeys(db); err != nil {
		t.Fatalf("second BackfillSitePageTrackingKeys: %v", err)
	}
	if err := db.First(&page, page.ID).Error; err != nil {
		t.Fatalf("reload page after second backfill: %v", err)
	}
	if err := db.Where("entity_type = ? AND entity_id = ?", sitePageTranslationEntityType, page.ID).
		Order("field_key ASC").Find(&translations).Error; err != nil {
		t.Fatalf("reload translations after second backfill: %v", err)
	}
	if string(page.DraftContent) != draftSnapshot || string(page.PublishedContent) != publishedSnapshot ||
		string(translations[0].ValueJSON) != translationSnapshots[0] || string(translations[1].ValueJSON) != translationSnapshots[1] {
		t.Fatal("second backfill regenerated duplicate replacements")
	}
}

func sitePageTestTrackingKey(t *testing.T, data datatypes.JSON, blockIndex int) string {
	t.Helper()
	content := sitePageTestContent(t, data)
	blocks := content["blocks"].([]any)
	return blocks[blockIndex].(map[string]any)["tracking_key"].(string)
}

func sitePageTestNestedTrackingKey(t *testing.T, data datatypes.JSON, blockIndex int, field string, itemIndex int) string {
	t.Helper()
	content := sitePageTestContent(t, data)
	block := content["blocks"].([]any)[blockIndex].(map[string]any)
	return block[field].([]any)[itemIndex].(map[string]any)["tracking_key"].(string)
}

func sitePageTestContent(t *testing.T, data datatypes.JSON) map[string]any {
	t.Helper()
	var content map[string]any
	if err := json.Unmarshal(data, &content); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	return content
}

func sitePageTestAllTrackingKeys(t *testing.T, data datatypes.JSON) []string {
	t.Helper()
	content := sitePageTestContent(t, data)
	keys := make([]string, 0)
	var collect func(map[string]any)
	collect = func(node map[string]any) {
		key, _ := node["tracking_key"].(string)
		keys = append(keys, key)
		for _, field := range []string{"slides", "items", "links"} {
			for _, child := range sitePageTrackingNodeList(node[field]) {
				collect(child)
			}
		}
	}
	for _, block := range sitePageTrackingNodeList(content["blocks"]) {
		collect(block)
	}
	return keys
}

func sitePageTestUniqueTrackingKeys(keys []string) bool {
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if !trackingkey.IsValid(key) {
			return false
		}
		if _, duplicate := seen[key]; duplicate {
			return false
		}
		seen[key] = struct{}{}
	}
	return true
}
