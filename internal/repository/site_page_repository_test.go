package repository

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"saas/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func TestSitePageRepositorySaveDraftAtomicRollsBackInjectedFailures(t *testing.T) {
	tests := []struct {
		name        string
		triggerSQL  string
		translation SitePageTranslationMutation
	}{
		{
			name: "English draft update",
			triggerSQL: `CREATE TRIGGER fail_english_draft_update BEFORE UPDATE ON site_pages
				WHEN NEW.title = 'Updated Title' BEGIN SELECT RAISE(FAIL, 'forced draft failure'); END`,
			translation: sitePageRepositoryTestTranslation(`{"blocks":[{"type":"text","title":"新中文"}]}`),
		},
		{
			name: "Chinese draft upsert",
			triggerSQL: `CREATE TRIGGER fail_chinese_draft_upsert BEFORE INSERT ON content_translations
				WHEN NEW.field_key = 'draft_content' AND json_extract(NEW.value_json, '$.blocks[0].title') = 'FAIL'
				BEGIN SELECT RAISE(FAIL, 'forced translation failure'); END`,
			translation: sitePageRepositoryTestTranslation(`{"blocks":[{"type":"text","title":"FAIL"}]}`),
		},
		{
			name: "Chinese draft delete",
			triggerSQL: `CREATE TRIGGER fail_chinese_draft_delete BEFORE DELETE ON content_translations
				WHEN OLD.entity_type = 'site_page' AND OLD.field_key = 'draft_content'
				BEGIN SELECT RAISE(FAIL, 'forced translation delete failure'); END`,
			translation: SitePageTranslationMutation{Provided: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newSitePageRepositoryTestDB(t)
			repo := NewSitePageRepository(db)
			seedSitePageRepositoryDraft(t, repo)
			before := captureSitePageSnapshot(t, db)
			if err := db.Exec(tt.triggerSQL).Error; err != nil {
				t.Fatalf("create failure trigger: %v", err)
			}
			if _, err := repo.SaveDraftAtomic(context.Background(), "home", "Updated Title",
				datatypes.JSON(`{"blocks":[{"type":"text","title":"Updated English"}]}`), 2, tt.translation); err == nil {
				t.Fatal("SaveDraftAtomic returned nil error for injected failure")
			}
			after := captureSitePageSnapshot(t, db)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("database changed after injected %s failure\nbefore=%+v\nafter=%+v", tt.name, before, after)
			}
		})
	}
}

func TestSitePageRepositorySaveDraftAtomicRollsBackNewPageTranslationFailure(t *testing.T) {
	db := newSitePageRepositoryTestDB(t)
	repo := NewSitePageRepository(db)
	before := captureSitePageSnapshot(t, db)
	if err := db.Exec(`CREATE TRIGGER fail_new_page_translation BEFORE INSERT ON content_translations
		WHEN NEW.field_key = 'draft_content' BEGIN SELECT RAISE(FAIL, 'forced translation failure'); END`).Error; err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}
	if _, err := repo.SaveDraftAtomic(context.Background(), "home", "New Home",
		datatypes.JSON(`{"blocks":[]}`), 1,
		sitePageRepositoryTestTranslation(`{"blocks":[{"type":"text","title":"中文"}]}`)); err == nil {
		t.Fatal("SaveDraftAtomic returned nil error for new page translation failure")
	}
	after := captureSitePageSnapshot(t, db)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("database changed after new page translation failure\nbefore=%+v\nafter=%+v", before, after)
	}
}

func TestSitePageRepositoryPublishAtomicRollsBackTranslationFailureAndRetriesCleanly(t *testing.T) {
	db := newSitePageRepositoryTestDB(t)
	repo := NewSitePageRepository(db)
	seedSitePageRepositoryDraft(t, repo)
	if _, err := repo.PublishAtomic(context.Background(), "home", 1, sitePageRepositoryTestPublishBuilder); err != nil {
		t.Fatalf("seed publish: %v", err)
	}
	if _, err := repo.SaveDraftAtomic(context.Background(), "home", "Latest Title",
		datatypes.JSON(`{"blocks":[{"type":"text","title":"Latest English"}]}`), 2,
		sitePageRepositoryTestTranslation(`{"blocks":[{"type":"text","title":"最新中文"}]}`)); err != nil {
		t.Fatalf("save latest draft: %v", err)
	}
	before := captureSitePageSnapshot(t, db)
	if err := db.Exec(`CREATE TRIGGER fail_published_translation BEFORE INSERT ON content_translations
		WHEN NEW.field_key = 'published_content' BEGIN SELECT RAISE(FAIL, 'forced publish translation failure'); END`).Error; err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	if _, err := repo.PublishAtomic(context.Background(), "home", 3, sitePageRepositoryTestPublishBuilder); err == nil {
		t.Fatal("PublishAtomic returned nil error for translation failure")
	}
	afterFailure := captureSitePageSnapshot(t, db)
	if !reflect.DeepEqual(afterFailure, before) {
		t.Fatalf("database changed after publish translation failure\nbefore=%+v\nafter=%+v", before, afterFailure)
	}
	if err := db.Exec("DROP TRIGGER fail_published_translation").Error; err != nil {
		t.Fatalf("drop failure trigger: %v", err)
	}
	if _, err := repo.PublishAtomic(context.Background(), "home", 3, sitePageRepositoryTestPublishBuilder); err != nil {
		t.Fatalf("retry publish: %v", err)
	}
	var count int64
	if err := db.Model(&models.ContentTranslation{}).
		Where("entity_type = ? AND field_key = ? AND locale = ?", "site_page", "published_content", "zh").
		Count(&count).Error; err != nil {
		t.Fatalf("count published translations: %v", err)
	}
	if count != 1 {
		t.Fatalf("published translation count = %d, want 1", count)
	}
}

func TestSitePageRepositoryPublishAtomicUsesLatestDraftAndSupportsEnglishFallback(t *testing.T) {
	db := newSitePageRepositoryTestDB(t)
	repo := NewSitePageRepository(db)
	seedSitePageRepositoryDraft(t, repo)
	latestEnglish := datatypes.JSON(`{"blocks":[{"type":"text","title":"Latest English"}]}`)
	latestChinese := datatypes.JSON(`{"blocks":[{"type":"text","title":"最新中文"}]}`)
	if _, err := repo.SaveDraftAtomic(context.Background(), "home", "Latest", latestEnglish, 2,
		SitePageTranslationMutation{Provided: true, Content: latestChinese}); err != nil {
		t.Fatalf("save latest drafts: %v", err)
	}
	builderCalled := false
	page, err := repo.PublishAtomic(context.Background(), "home", 3, func(englishDraft datatypes.JSON, zhDraft datatypes.JSON) (datatypes.JSON, bool, error) {
		builderCalled = true
		if string(englishDraft) != string(latestEnglish) || string(zhDraft) != string(latestChinese) {
			t.Fatalf("publish drafts = %s / %s, want latest tx values", englishDraft, zhDraft)
		}
		return append(datatypes.JSON(nil), zhDraft...), true, nil
	})
	if err != nil {
		t.Fatalf("publish latest drafts: %v", err)
	}
	if !builderCalled || string(page.PublishedContent) != string(latestEnglish) || page.Status != models.SitePageStatusPublished || page.PublishedAt == nil {
		t.Fatalf("published page = %+v", page)
	}

	if _, err := repo.SaveDraftAtomic(context.Background(), "home", "English Only", latestEnglish, 4,
		SitePageTranslationMutation{Provided: true}); err != nil {
		t.Fatalf("clear Chinese draft: %v", err)
	}
	if _, err := repo.PublishAtomic(context.Background(), "home", 4, sitePageRepositoryTestPublishBuilder); err != nil {
		t.Fatalf("publish English fallback: %v", err)
	}
	var count int64
	if err := db.Model(&models.ContentTranslation{}).
		Where("entity_type = ? AND field_key = ? AND locale = ?", "site_page", "published_content", "zh").
		Count(&count).Error; err != nil {
		t.Fatalf("count fallback translation: %v", err)
	}
	if count != 0 {
		t.Fatalf("published translation count = %d, want English fallback with no row", count)
	}
}

type sitePageSnapshot struct {
	Pages        []models.SitePage
	Translations []models.ContentTranslation
}

func newSitePageRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	name := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.SitePage{}, &models.ContentTranslation{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return db
}

func seedSitePageRepositoryDraft(t *testing.T, repo *SitePageRepository) *models.SitePage {
	t.Helper()
	page, err := repo.SaveDraftAtomic(context.Background(), "home", "Old Title",
		datatypes.JSON(`{"blocks":[{"type":"text","title":"Old English"}]}`), 1,
		sitePageRepositoryTestTranslation(`{"blocks":[{"type":"text","title":"旧中文"}]}`))
	if err != nil {
		t.Fatalf("seed site page draft: %v", err)
	}
	return page
}

func sitePageRepositoryTestTranslation(content string) SitePageTranslationMutation {
	return SitePageTranslationMutation{Provided: true, Content: datatypes.JSON(content)}
}

func sitePageRepositoryTestPublishBuilder(_ datatypes.JSON, zhDraft datatypes.JSON) (datatypes.JSON, bool, error) {
	if len(zhDraft) == 0 {
		return nil, false, nil
	}
	return append(datatypes.JSON(nil), zhDraft...), true, nil
}

func captureSitePageSnapshot(t *testing.T, db *gorm.DB) sitePageSnapshot {
	t.Helper()
	snapshot := sitePageSnapshot{}
	if err := db.Order("id ASC").Find(&snapshot.Pages).Error; err != nil {
		t.Fatalf("snapshot pages: %v", err)
	}
	if err := db.Order("id ASC").Find(&snapshot.Translations).Error; err != nil {
		t.Fatalf("snapshot translations: %v", err)
	}
	return snapshot
}
