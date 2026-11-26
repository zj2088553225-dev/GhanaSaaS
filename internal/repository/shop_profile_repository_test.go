package repository

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"saas/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestShopProfileRepositoryCreateAtomicRollsBackTranslationFailure(t *testing.T) {
	db := newShopProfileRepositoryTestDB(t)
	repo := NewShopProfileRepository(db)
	before := captureShopProfileSnapshot(t, db)
	if err := db.Exec(`CREATE TRIGGER fail_new_shop_translation BEFORE INSERT ON content_translations
		WHEN NEW.value_text = 'FAIL' BEGIN SELECT RAISE(FAIL, 'forced translation failure'); END`).Error; err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	if _, err := repo.SaveAtomic(context.Background(), 1001, shopProfileRepositoryTestProfile("tenant-a", "English Shop"),
		shopProfileRepositoryTestTranslation("FAIL")); err == nil {
		t.Fatal("SaveAtomic returned nil error for translation failure")
	}
	after := captureShopProfileSnapshot(t, db)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("database changed after new profile translation failure\nbefore=%+v\nafter=%+v", before, after)
	}
}

func TestShopProfileRepositoryUpdateAtomicRollsBackInjectedFailures(t *testing.T) {
	tests := []struct {
		name        string
		triggerSQL  string
		translation ShopProfileTranslationMutation
	}{
		{
			name: "profile update",
			triggerSQL: `CREATE TRIGGER fail_shop_profile_update BEFORE UPDATE ON shop_profiles
				WHEN NEW.shop_name = 'Updated Shop' BEGIN SELECT RAISE(FAIL, 'forced profile failure'); END`,
			translation: shopProfileRepositoryTestTranslation("新中文店铺"),
		},
		{
			name: "translation upsert",
			triggerSQL: `CREATE TRIGGER fail_shop_translation_update BEFORE INSERT ON content_translations
				WHEN NEW.value_text = 'FAIL' BEGIN SELECT RAISE(FAIL, 'forced translation failure'); END`,
			translation: shopProfileRepositoryTestTranslation("FAIL"),
		},
		{
			name: "translation delete",
			triggerSQL: `CREATE TRIGGER fail_shop_translation_delete BEFORE DELETE ON content_translations
				WHEN OLD.entity_type = 'shop_profile' BEGIN SELECT RAISE(FAIL, 'forced translation delete failure'); END`,
			translation: ShopProfileTranslationMutation{Provided: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newShopProfileRepositoryTestDB(t)
			repo := NewShopProfileRepository(db)
			seedShopProfileRepositoryProfile(t, repo, 1001, "tenant-a")
			before := captureShopProfileSnapshot(t, db)
			if err := db.Exec(tt.triggerSQL).Error; err != nil {
				t.Fatalf("create failure trigger: %v", err)
			}
			updated := shopProfileRepositoryTestProfile("tenant-a-updated", "Updated Shop")
			updated.LogoURL = "new-logo.png"
			updated.BannerURL = "new-banner.png"
			updated.Phone = "999"
			updated.Whatsapp = "888"
			updated.IsPublic = false
			if _, err := repo.SaveAtomic(context.Background(), 1001, updated, tt.translation); err == nil {
				t.Fatal("SaveAtomic returned nil error for injected failure")
			}
			after := captureShopProfileSnapshot(t, db)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("database changed after injected %s failure\nbefore=%+v\nafter=%+v", tt.name, before, after)
			}
		})
	}
}

func TestShopProfileRepositorySlugConflictAndTenantScopeAreAtomic(t *testing.T) {
	db := newShopProfileRepositoryTestDB(t)
	repo := NewShopProfileRepository(db)
	profileA := seedShopProfileRepositoryProfile(t, repo, 1001, "tenant-a")
	profileB := seedShopProfileRepositoryProfile(t, repo, 2002, "tenant-b")
	before := captureShopProfileSnapshot(t, db)

	if _, err := repo.SaveAtomic(context.Background(), 1001,
		shopProfileRepositoryTestProfile("tenant-b", "Conflicting Shop"),
		shopProfileRepositoryTestTranslation("冲突店铺")); !errors.Is(err, ErrDuplicateShopProfileSlug) {
		t.Fatalf("slug conflict error = %v, want ErrDuplicateShopProfileSlug", err)
	}
	afterConflict := captureShopProfileSnapshot(t, db)
	if !reflect.DeepEqual(afterConflict, before) {
		t.Fatalf("database changed after slug conflict\nbefore=%+v\nafter=%+v", before, afterConflict)
	}

	requestWithForeignTenant := shopProfileRepositoryTestProfile("tenant-a", "Tenant A Updated")
	requestWithForeignTenant.TenantID = 2002
	if saved, err := repo.SaveAtomic(context.Background(), 1001, requestWithForeignTenant,
		shopProfileRepositoryTestTranslation("租户A更新")); err != nil {
		t.Fatalf("tenant-scoped update: %v", err)
	} else if saved.ID != profileA.ID || saved.TenantID != 1001 {
		t.Fatalf("saved profile = %+v, want tenant A profile", saved)
	}
	var unchangedB models.ShopProfile
	if err := db.First(&unchangedB, profileB.ID).Error; err != nil {
		t.Fatalf("load tenant B: %v", err)
	}
	if unchangedB.Slug != "tenant-b" || unchangedB.ShopName != "English Shop" {
		t.Fatalf("tenant B changed after tenant A update: %+v", unchangedB)
	}
}

func TestShopProfileRepositoryClearTranslationsKeepsEnglishStructure(t *testing.T) {
	db := newShopProfileRepositoryTestDB(t)
	repo := NewShopProfileRepository(db)
	seedShopProfileRepositoryProfile(t, repo, 1001, "tenant-a")
	updated := shopProfileRepositoryTestProfile("tenant-a-new", "English Fallback")
	updated.LogoURL = "logo-new.png"
	updated.BannerURL = "banner-new.png"
	updated.Phone = "123"
	updated.Whatsapp = "456"
	updated.IsPublic = false

	saved, err := repo.SaveAtomic(context.Background(), 1001, updated, ShopProfileTranslationMutation{Provided: true})
	if err != nil {
		t.Fatalf("clear translations: %v", err)
	}
	if saved.ShopName != "English Fallback" || saved.Slug != "tenant-a-new" || saved.LogoURL != "logo-new.png" || saved.IsPublic {
		t.Fatalf("English/structure fields = %+v", saved)
	}
	var count int64
	if err := db.Model(&models.ContentTranslation{}).
		Where("entity_type = ? AND entity_id = ?", "shop_profile", saved.ID).Count(&count).Error; err != nil {
		t.Fatalf("count translations: %v", err)
	}
	if count != 0 {
		t.Fatalf("translation count = %d, want 0", count)
	}
}

type shopProfileSnapshot struct {
	Profiles     []models.ShopProfile
	Translations []models.ContentTranslation
}

func newShopProfileRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	name := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	if err := db.AutoMigrate(&models.Tenant{}, &models.ShopProfile{}, &models.ContentTranslation{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	tenants := []models.Tenant{
		{BaseModel: models.BaseModel{ID: 1001}, Name: "Tenant A", Type: models.TenantTypeThirdParty, Status: models.TenantStatusActive, AppKey: "shop-a"},
		{BaseModel: models.BaseModel{ID: 2002}, Name: "Tenant B", Type: models.TenantTypeThirdParty, Status: models.TenantStatusActive, AppKey: "shop-b"},
	}
	if err := db.Create(&tenants).Error; err != nil {
		t.Fatalf("seed tenants: %v", err)
	}
	return db
}

func shopProfileRepositoryTestProfile(slug string, name string) *models.ShopProfile {
	return &models.ShopProfile{
		Slug: slug, ShopName: name, LogoURL: "logo.png", BannerURL: "banner.png",
		Description: "English description", Phone: "111", Whatsapp: "222",
		Address: "Accra", BusinessHours: "09:00-18:00", IsPublic: true,
	}
}

func shopProfileRepositoryTestTranslation(name string) ShopProfileTranslationMutation {
	return ShopProfileTranslationMutation{
		Provided: true, ShopName: name, Description: "中文简介",
		Address: "中文地址", BusinessHours: "中文营业时间",
	}
}

func seedShopProfileRepositoryProfile(t *testing.T, repo *ShopProfileRepository, tenantID uint, slug string) *models.ShopProfile {
	t.Helper()
	saved, err := repo.SaveAtomic(context.Background(), tenantID,
		shopProfileRepositoryTestProfile(slug, "English Shop"), shopProfileRepositoryTestTranslation("中文店铺"))
	if err != nil {
		t.Fatalf("seed shop profile: %v", err)
	}
	return saved
}

func captureShopProfileSnapshot(t *testing.T, db *gorm.DB) shopProfileSnapshot {
	t.Helper()
	snapshot := shopProfileSnapshot{}
	if err := db.Order("id ASC").Find(&snapshot.Profiles).Error; err != nil {
		t.Fatalf("snapshot profiles: %v", err)
	}
	if err := db.Order("id ASC").Find(&snapshot.Translations).Error; err != nil {
		t.Fatalf("snapshot translations: %v", err)
	}
	return snapshot
}
