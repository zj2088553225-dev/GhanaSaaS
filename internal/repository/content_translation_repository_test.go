package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"saas/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func TestContentTranslationRepositoryUpsertAndGetTranslation(t *testing.T) {
	repo := newContentTranslationTestRepository(t)
	ctx := context.Background()

	saved, err := repo.Upsert(ctx, &models.ContentTranslation{
		EntityType: "product",
		EntityID:   10,
		FieldKey:   "name",
		Locale:     "zh",
		ValueText:  "中文商品名",
	})
	if err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}
	if saved.Locale != "zh" || saved.Status != models.ContentTranslationStatusPublished {
		t.Fatalf("saved translation = %+v, want normalized zh published", saved)
	}

	got, found, err := repo.GetTranslation(ctx, "product", 10, "name", "zh")
	if err != nil {
		t.Fatalf("GetTranslation returned error: %v", err)
	}
	if !found || got.ValueText != "中文商品名" {
		t.Fatalf("translation = %+v found = %v, want zh value", got, found)
	}
}

func TestContentTranslationRepositoryFallsBackToDefaultLocale(t *testing.T) {
	repo := newContentTranslationTestRepository(t)
	ctx := context.Background()

	if _, err := repo.Upsert(ctx, &models.ContentTranslation{
		EntityType: "shop",
		EntityID:   20,
		FieldKey:   "description",
		Locale:     "en",
		ValueText:  "English description",
	}); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}

	got, found, err := repo.GetTranslation(ctx, "shop", 20, "description", "zh")
	if err != nil {
		t.Fatalf("GetTranslation returned error: %v", err)
	}
	if !found || got.Locale != "en" || got.ValueText != "English description" {
		t.Fatalf("translation = %+v found = %v, want en fallback", got, found)
	}

	missing, found, err := repo.GetTranslation(ctx, "shop", 20, "address", "zh")
	if err != nil {
		t.Fatalf("GetTranslation missing returned error: %v", err)
	}
	if found || missing != nil {
		t.Fatalf("missing translation = %+v found = %v, want nil false", missing, found)
	}
}

func TestContentTranslationRepositorySkipsEmptyTranslationAndFallsBack(t *testing.T) {
	repo := newContentTranslationTestRepository(t)
	ctx := context.Background()

	rows := []models.ContentTranslation{
		{EntityType: "product", EntityID: 21, FieldKey: "detail_html", Locale: "zh", ValueHTML: " "},
		{EntityType: "product", EntityID: 21, FieldKey: "detail_html", Locale: "en", ValueHTML: "<p>English detail</p>"},
	}
	for index := range rows {
		if _, err := repo.Upsert(ctx, &rows[index]); err != nil {
			t.Fatalf("Upsert row %d returned error: %v", index, err)
		}
	}

	got, found, err := repo.GetTranslation(ctx, "product", 21, "detail_html", "zh")
	if err != nil {
		t.Fatalf("GetTranslation returned error: %v", err)
	}
	if !found || got.Locale != "en" || got.ValueHTML != "<p>English detail</p>" {
		t.Fatalf("translation = %+v found = %v, want non-empty en fallback", got, found)
	}
}

func TestContentTranslationRepositoryUpsertUsesUniqueKey(t *testing.T) {
	repo := newContentTranslationTestRepository(t)
	ctx := context.Background()

	for _, value := range []string{"First name", "Updated name"} {
		if _, err := repo.Upsert(ctx, &models.ContentTranslation{
			EntityType: "product",
			EntityID:   30,
			FieldKey:   "name",
			Locale:     "en",
			ValueText:  value,
		}); err != nil {
			t.Fatalf("Upsert(%q) returned error: %v", value, err)
		}
	}

	var count int64
	if err := repo.db.Model(&models.ContentTranslation{}).
		Where("entity_type = ? AND entity_id = ? AND field_key = ? AND locale = ?", "product", 30, "name", "en").
		Count(&count).Error; err != nil {
		t.Fatalf("Count returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("translation rows = %d, want 1", count)
	}
	got, found, err := repo.GetTranslation(ctx, "product", 30, "name", "en")
	if err != nil {
		t.Fatalf("GetTranslation returned error: %v", err)
	}
	if !found || got.ValueText != "Updated name" {
		t.Fatalf("translation = %+v found = %v, want updated value", got, found)
	}
}

func TestContentTranslationRepositoryBatchUpsertUsesUniqueKey(t *testing.T) {
	repo := newContentTranslationTestRepository(t)
	ctx := context.Background()

	if err := repo.BatchUpsert(ctx, []models.ContentTranslation{
		{EntityType: "platform_category", EntityID: 1, FieldKey: "name", Locale: "zh", ValueText: "家用电器"},
		{EntityType: "platform_category", EntityID: 2, FieldKey: "name", Locale: "zh", ValueText: "电子产品"},
	}); err != nil {
		t.Fatalf("BatchUpsert returned error: %v", err)
	}
	if err := repo.BatchUpsert(ctx, []models.ContentTranslation{
		{EntityType: "platform_category", EntityID: 1, FieldKey: "name", Locale: "zh", ValueText: "家电"},
	}); err != nil {
		t.Fatalf("second BatchUpsert returned error: %v", err)
	}

	var count int64
	if err := repo.db.Model(&models.ContentTranslation{}).
		Where("entity_type = ? AND field_key = ? AND locale = ?", "platform_category", "name", "zh").
		Count(&count).Error; err != nil {
		t.Fatalf("Count returned error: %v", err)
	}
	if count != 2 {
		t.Fatalf("translation rows = %d, want 2", count)
	}
	got, found, err := repo.GetTranslation(ctx, "platform_category", 1, "name", "zh")
	if err != nil {
		t.Fatalf("GetTranslation returned error: %v", err)
	}
	if !found || got.ValueText != "家电" {
		t.Fatalf("translation = %+v found=%v, want updated batch value", got, found)
	}
}

func TestContentTranslationRepositoryGetEntityTranslationsAndDelete(t *testing.T) {
	repo := newContentTranslationTestRepository(t)
	ctx := context.Background()

	rows := []models.ContentTranslation{
		{EntityType: "product", EntityID: 40, FieldKey: "name", Locale: "zh", ValueText: "中文名"},
		{EntityType: "product", EntityID: 40, FieldKey: "description", Locale: "zh", ValueText: "中文描述"},
		{EntityType: "product", EntityID: 40, FieldKey: "name", Locale: "en", ValueText: "English name"},
	}
	for index := range rows {
		if _, err := repo.Upsert(ctx, &rows[index]); err != nil {
			t.Fatalf("Upsert row %d returned error: %v", index, err)
		}
	}

	translations, err := repo.GetEntityTranslations(ctx, "product", 40, "zh")
	if err != nil {
		t.Fatalf("GetEntityTranslations returned error: %v", err)
	}
	if got := translations["name"].ValueText; got != "中文名" {
		t.Fatalf("zh name = %q, want exact zh", got)
	}
	if _, ok := translations["missing"]; ok {
		t.Fatal("unexpected missing field in entity translations")
	}

	if err := repo.Delete(ctx, "product", 40, "name", "zh"); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	translations, err = repo.GetEntityTranslations(ctx, "product", 40, "zh")
	if err != nil {
		t.Fatalf("GetEntityTranslations after delete returned error: %v", err)
	}
	if _, ok := translations["name"]; ok {
		t.Fatal("deleted zh name should not be returned")
	}

	fallback, found, err := repo.GetTranslation(ctx, "product", 40, "name", "zh")
	if err != nil {
		t.Fatalf("GetTranslation fallback returned error: %v", err)
	}
	if !found || fallback.Locale != "en" || fallback.ValueText != "English name" {
		t.Fatalf("fallback = %+v found = %v, want en fallback after zh delete", fallback, found)
	}
}

func TestContentTranslationRepositoryGetTranslationsBatchFallback(t *testing.T) {
	repo := newContentTranslationTestRepository(t)
	ctx := context.Background()
	rows := []models.ContentTranslation{
		{EntityType: "product", EntityID: 1, FieldKey: "name", Locale: "en", ValueText: "English One"},
		{EntityType: "product", EntityID: 1, FieldKey: "name", Locale: "zh", ValueText: "中文一"},
		{EntityType: "product", EntityID: 2, FieldKey: "name", Locale: "en", ValueText: "English Two"},
		{EntityType: "product", EntityID: 2, FieldKey: "spec_template", Locale: "zh", ValueJSON: datatypes.JSON(`{"name":"规格"}`)},
		{EntityType: "product", EntityID: 3, FieldKey: "name", Locale: "zh", ValueText: "Draft name", Status: models.ContentTranslationStatusDraft},
	}
	for index := range rows {
		if _, err := repo.Upsert(ctx, &rows[index]); err != nil {
			t.Fatalf("Upsert row %d returned error: %v", index, err)
		}
	}

	translations, err := repo.GetTranslations(ctx, "product", []uint{1, 2, 2, 3, 0}, "zh")
	if err != nil {
		t.Fatalf("GetTranslations returned error: %v", err)
	}
	if got := translations[1]["name"].ValueText; got != "中文一" {
		t.Fatalf("entity 1 name = %q, want requested zh", got)
	}
	if got := translations[2]["name"].ValueText; got != "English Two" {
		t.Fatalf("entity 2 name = %q, want en fallback", got)
	}
	if got := string(translations[2]["spec_template"].ValueJSON); got != `{"name":"规格"}` {
		t.Fatalf("entity 2 json = %q, want zh JSON", got)
	}
	if _, ok := translations[3]["name"]; ok {
		t.Fatal("draft translation should not be returned for public reads")
	}
}

func newContentTranslationTestRepository(t *testing.T) *ContentTranslationRepository {
	t.Helper()
	name := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.ContentTranslation{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return NewContentTranslationRepository(db)
}
