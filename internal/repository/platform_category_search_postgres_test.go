package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"saas/internal/models"
	"saas/internal/pkg/categorysearch"
	searchdatabase "saas/internal/pkg/database"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	platformCategorySearchPostgresAllowEnvironment = "PLATFORM_CATEGORY_SEARCH_TEST_ALLOW_ISOLATED"
	platformCategorySearchPostgresDSNEnvironment   = "PLATFORM_CATEGORY_SEARCH_TEST_POSTGRES_DSN"
	platformCategorySearchPostgresDatabase         = "platform_category_search_test"
)

type platformCategorySearchPostgresFixture struct {
	Electronics uint
	Mobile      uint
	Accessories uint
	Smartphones uint
	PhoneCases  uint
	Cameras     uint
	Disabled    uint
	Percent     uint
	Underscore  uint
	Home        uint
}

func TestPlatformCategorySearchPostgresContract(t *testing.T) {
	db := openPlatformCategorySearchPostgresDB(t, true)
	if err := searchdatabase.EnsurePlatformCategorySearchInfrastructure(db); err != nil {
		t.Fatalf("create PostgreSQL search infrastructure: %v", err)
	}
	assertPlatformCategorySearchPostgresInfrastructure(t, db)

	fixture := seedPlatformCategorySearchPostgresHistory(t, db)
	repo := NewPlatformCategoryRepository(db)
	ctx := t.Context()

	var rowsBefore int64
	if err := db.Model(&models.PlatformCategorySearchIndex{}).Count(&rowsBefore).Error; err != nil || rowsBefore != 0 {
		t.Fatalf("historical index rows before rebuild = %d, %v", rowsBefore, err)
	}
	dryRun, err := repo.MaintainSearchIndex(ctx, PlatformCategorySearchMaintenanceOptions{BatchSize: 3})
	if err != nil {
		t.Fatalf("dry-run historical audit: %v", err)
	}
	if dryRun.Categories != 10 || dryRun.ExpectedRows != 20 || dryRun.Created != 20 || dryRun.Updated != 0 {
		t.Fatalf("dry-run summary = %+v", dryRun)
	}
	var rowsAfterDryRun int64
	if err := db.Model(&models.PlatformCategorySearchIndex{}).Count(&rowsAfterDryRun).Error; err != nil || rowsAfterDryRun != 0 {
		t.Fatalf("dry-run changed database: rows=%d err=%v", rowsAfterDryRun, err)
	}
	if _, err := repo.MaintainSearchIndex(ctx, PlatformCategorySearchMaintenanceOptions{Verify: true}); !errors.Is(err, ErrPlatformCategorySearchVerification) {
		t.Fatalf("dry-run verify error = %v, want verification failure", err)
	}

	firstApply, err := repo.MaintainSearchIndex(ctx, PlatformCategorySearchMaintenanceOptions{
		Apply: true, Verify: true, BatchSize: 4,
	})
	if err != nil {
		t.Fatalf("apply historical rebuild: %v", err)
	}
	if firstApply.Created != 20 || firstApply.Updated != 0 || !firstApply.Verified {
		t.Fatalf("first apply summary = %+v", firstApply)
	}
	secondApply, err := repo.MaintainSearchIndex(ctx, PlatformCategorySearchMaintenanceOptions{
		Apply: true, Verify: true, BatchSize: 5,
	})
	if err != nil {
		t.Fatalf("repeat historical rebuild: %v", err)
	}
	if secondApply.Created != 0 || secondApply.Updated != 0 || secondApply.Unchanged != 20 || !secondApply.Verified {
		t.Fatalf("repeat apply summary = %+v", secondApply)
	}
	assertPlatformCategorySearchPostgresTwoRowsPerCategory(t, db, 10)
	assertPlatformCategorySearchPostgresDerivedRows(t, db, fixture)

	t.Run("multilingual tokenized search", func(t *testing.T) {
		assertPlatformCategorySearchContains(t, repo, "手机", "zh", true, fixture.Mobile)
		assertPlatformCategorySearchContains(t, repo, "手", "zh", true, fixture.Mobile)
		assertPlatformCategorySearchContains(t, repo, "智能手机", "zh", true, fixture.Smartphones)
		assertPlatformCategorySearchContains(t, repo, "电子产品 > 手机", "zh", true, fixture.Mobile)
		assertPlatformCategorySearchContains(t, repo, "电子产品 / 手机", "zh", true, fixture.Mobile)
		assertPlatformCategorySearchContains(t, repo, "Electronics Mobile", "en", true, fixture.Mobile)
		assertPlatformCategorySearchContains(t, repo, "Mobile", "zh", true, fixture.Mobile)
		assertPlatformCategorySearchContains(t, repo, "手机 Mobile", "zh", true, fixture.Mobile)
		assertPlatformCategorySearchContains(t, repo, "手机　Mobile，", "all", true, fixture.Mobile)
		assertPlatformCategorySearchDoesNotContain(t, repo, "手机", "en", true, fixture.Mobile)
		assertPlatformCategorySearchDoesNotContain(t, repo, "手机 unrelated", "all", true, fixture.Mobile)
		assertPlatformCategorySearchDoesNotContain(t, repo, "legacy", "all", true, fixture.Disabled)
		assertPlatformCategorySearchContains(t, repo, "legacy", "all", false, fixture.Disabled)

		percentMatches := platformCategorySearchMatchedIDs(t, db, "100%", 100, true, "all")
		assertPlatformCategorySearchIDSetContains(t, percentMatches, fixture.Percent)
		if platformCategorySearchIDsContain(percentMatches, fixture.Underscore) {
			t.Fatalf("literal percent query matched underscore category: %v", percentMatches)
		}
		underscoreMatches := platformCategorySearchMatchedIDs(t, db, "code_name", 100, true, "all")
		assertPlatformCategorySearchIDSetContains(t, underscoreMatches, fixture.Underscore)
		if matches := platformCategorySearchMatchedIDs(t, db, "codexname", 100, true, "all"); platformCategorySearchIDsContain(matches, fixture.Underscore) {
			t.Fatalf("escaped underscore behaved as wildcard: %v", matches)
		}
		if results, err := repo.Search(ctx, `\\`, 100, true, "all"); err != nil || len(results) != 0 {
			t.Fatalf("separator-only query = %+v, %v", results, err)
		}

		allMatches := platformCategorySearchMatchedIDs(t, db, "Mobile", 100, true, "all")
		if !platformCategorySearchIDsUnique(allMatches) {
			t.Fatalf("locale=all returned duplicate category IDs: %v", allMatches)
		}
		if len(allMatches) == 0 || allMatches[0] != fixture.Mobile {
			t.Fatalf("exact-name ranking = %v, want Mobile first", allMatches)
		}
		repeated := platformCategorySearchMatchedIDs(t, db, "Mobile", 100, true, "all")
		if !reflect.DeepEqual(allMatches, repeated) {
			t.Fatalf("search ordering is unstable: first=%v second=%v", allMatches, repeated)
		}
		limited := platformCategorySearchMatchedIDs(t, db, "phone", 1, true, "all")
		if len(limited) != 1 {
			t.Fatalf("limit=1 returned %d IDs: %v", len(limited), limited)
		}

		results, err := repo.Search(ctx, "手机配件", 100, true, "zh")
		if err != nil {
			t.Fatalf("search Chinese path display: %v", err)
		}
		results, err = repo.AttachBilingualDisplay(ctx, results)
		if err != nil {
			t.Fatalf("attach Chinese path display: %v", err)
		}
		accessories := platformCategorySearchResultByID(results, fixture.Accessories)
		if accessories == nil || accessories.ZhPath != "电子产品 > 手机 > 手机配件" {
			t.Fatalf("Chinese result path = %+v", accessories)
		}
		fallback, err := repo.AttachBilingualDisplay(ctx, []models.PlatformCategory{platformCategorySearchLoadCategory(t, db, fixture.PhoneCases)})
		if err != nil || len(fallback) != 1 || fallback[0].ZhName != "Phone Cases" || fallback[0].ZhPath != "电子产品 > 手机 > Phone Cases" {
			t.Fatalf("Chinese fallback display = %+v, %v", fallback, err)
		}
	})

	t.Run("verification repairs stale and invalid rows", func(t *testing.T) {
		if err := db.Model(&models.PlatformCategorySearchIndex{}).
			Where("category_id = ? AND locale = ?", fixture.Accessories, "zh").
			Updates(map[string]any{"path": "过期路径", "search_text": "过期内容"}).Error; err != nil {
			t.Fatalf("make stale row: %v", err)
		}
		if _, err := repo.MaintainSearchIndex(ctx, PlatformCategorySearchMaintenanceOptions{Verify: true}); !errors.Is(err, ErrPlatformCategorySearchVerification) {
			t.Fatalf("stale verify error = %v", err)
		}
		repaired, err := repo.MaintainSearchIndex(ctx, PlatformCategorySearchMaintenanceOptions{Apply: true, Verify: true})
		if err != nil || repaired.Updated != 1 || repaired.Created != 0 {
			t.Fatalf("repair summary = %+v, %v", repaired, err)
		}

		invalid := models.PlatformCategorySearchIndex{
			CategoryID: fixture.Mobile, Locale: "fr", Name: "invalide", Path: "invalide", SearchText: "invalide",
			Level: 2, Status: models.PlatformCategoryStatusActive,
		}
		if err := db.Create(&invalid).Error; err != nil {
			t.Fatalf("create invalid locale row: %v", err)
		}
		cleaned, err := repo.MaintainSearchIndex(ctx, PlatformCategorySearchMaintenanceOptions{Apply: true, Verify: true})
		if err != nil || cleaned.InvalidRows != 1 {
			t.Fatalf("clean invalid locale summary = %+v, %v", cleaned, err)
		}
	})

	t.Run("translation lifecycle and subtree synchronization", func(t *testing.T) {
		translationRepo := NewContentTranslationRepository(db)
		translationRepo.SetMutationHook(ContentTranslationMutationHook{
			Before: repo.BeforeContentTranslationMutations,
			After:  repo.AfterContentTranslationMutations,
		})
		if _, err := translationRepo.Upsert(ctx, &models.ContentTranslation{
			EntityType: "platform_category", EntityID: fixture.Electronics, FieldKey: "name", Locale: "zh",
			ValueText: "数码产品", Status: models.ContentTranslationStatusPublished,
		}); err != nil {
			t.Fatalf("update published parent translation: %v", err)
		}
		assertPlatformCategorySearchIndexPath(t, db, fixture.Accessories, "zh", "数码产品 > 手机 > 手机配件")

		if _, err := translationRepo.Upsert(ctx, &models.ContentTranslation{
			EntityType: "platform_category", EntityID: fixture.Cameras, FieldKey: "name", Locale: "zh",
			ValueText: "相机", Status: models.ContentTranslationStatusPublished,
		}); err != nil {
			t.Fatalf("publish draft translation: %v", err)
		}
		assertPlatformCategorySearchIndexName(t, db, fixture.Cameras, "zh", "相机")

		if _, err := translationRepo.Upsert(ctx, &models.ContentTranslation{
			EntityType: "platform_category", EntityID: fixture.Mobile, FieldKey: "name", Locale: "zh",
			ValueText: "手机", Status: models.ContentTranslationStatusUnpublished,
		}); err != nil {
			t.Fatalf("unpublish translation: %v", err)
		}
		assertPlatformCategorySearchIndexName(t, db, fixture.Mobile, "zh", "Mobile")
		assertPlatformCategorySearchIndexPath(t, db, fixture.Accessories, "zh", "数码产品 > Mobile > 手机配件")

		if _, err := translationRepo.Upsert(ctx, &models.ContentTranslation{
			EntityType: "platform_category", EntityID: fixture.Mobile, FieldKey: "name", Locale: "zh",
			ValueText: "手机终端", Status: models.ContentTranslationStatusPublished,
		}); err != nil {
			t.Fatalf("republish translation: %v", err)
		}
		assertPlatformCategorySearchIndexPath(t, db, fixture.Accessories, "zh", "数码产品 > 手机终端 > 手机配件")

		if err := translationRepo.Delete(ctx, "platform_category", fixture.Electronics, "name", "zh"); err != nil {
			t.Fatalf("delete parent translation: %v", err)
		}
		assertPlatformCategorySearchIndexPath(t, db, fixture.Accessories, "zh", "Electronics > 手机终端 > 手机配件")
	})

	t.Run("category mutations rebuild subtrees", func(t *testing.T) {
		if _, err := repo.UpdateAtomic(ctx, fixture.Smartphones, PlatformCategoryMutation{
			ParentID: uintPointer(fixture.Home), Name: "Smart Smartphones", SortOrder: 17,
			Status: models.PlatformCategoryStatusDisabled,
		}); err != nil {
			t.Fatalf("move and disable category: %v", err)
		}
		assertPlatformCategorySearchIndexPath(t, db, fixture.Smartphones, "en", "Home > Smart Smartphones")
		assertPlatformCategorySearchIndexPath(t, db, fixture.Smartphones, "zh", "家居 > 智能手机")
		var index models.PlatformCategorySearchIndex
		if err := db.Where("category_id = ? AND locale = ?", fixture.Smartphones, "en").Take(&index).Error; err != nil {
			t.Fatalf("load moved index: %v", err)
		}
		if index.Status != models.PlatformCategoryStatusDisabled || index.SortOrder != 17 || index.Level != 2 {
			t.Fatalf("moved index metadata = %+v", index)
		}

		created, err := repo.CreateAtomic(ctx, PlatformCategoryMutation{
			ParentID: uintPointer(fixture.Home), Name: "Temporary", ZhName: "临时类目", ZhProvided: true,
			Status: models.PlatformCategoryStatusActive,
		})
		if err != nil {
			t.Fatalf("create synchronized category: %v", err)
		}
		assertPlatformCategorySearchIndexName(t, db, created.ID, "zh", "临时类目")
		if err := repo.DeleteTree(ctx, created); err != nil {
			t.Fatalf("delete synchronized category: %v", err)
		}
		var categoryRows, indexRows, translationRows int64
		_ = db.Model(&models.PlatformCategory{}).Where("id = ?", created.ID).Count(&categoryRows).Error
		_ = db.Model(&models.PlatformCategorySearchIndex{}).Where("category_id = ?", created.ID).Count(&indexRows).Error
		_ = db.Model(&models.ContentTranslation{}).Where("entity_type = ? AND entity_id = ?", "platform_category", created.ID).Count(&translationRows).Error
		if categoryRows != 0 || indexRows != 0 || translationRows != 0 {
			t.Fatalf("deleted category remnants: category=%d index=%d translation=%d", categoryRows, indexRows, translationRows)
		}
	})

	t.Run("orphan cleanup", func(t *testing.T) {
		const orphanCategoryID = uint(99999999)
		if err := db.Exec("ALTER TABLE platform_category_search_indexes DISABLE TRIGGER ALL").Error; err != nil {
			t.Fatalf("disable test-only FK trigger: %v", err)
		}
		insertErr := db.Exec(`
			INSERT INTO platform_category_search_indexes
				(created_at, updated_at, category_id, locale, name, path, search_text, level, status, sort_order)
			VALUES (NOW(), NOW(), ?, 'en', 'orphan', 'orphan', 'orphan', 1, 'active', 0)
		`, orphanCategoryID).Error
		enableErr := db.Exec("ALTER TABLE platform_category_search_indexes ENABLE TRIGGER ALL").Error
		if insertErr != nil || enableErr != nil {
			t.Fatalf("seed orphan row: insert=%v enable=%v", insertErr, enableErr)
		}
		cleaned, err := repo.MaintainSearchIndex(ctx, PlatformCategorySearchMaintenanceOptions{Apply: true, Verify: true})
		if err != nil || cleaned.DeletedOrphans != 1 {
			t.Fatalf("orphan cleanup summary = %+v, %v", cleaned, err)
		}
	})

	t.Run("concurrent maintenance and update remain unique", func(t *testing.T) {
		lockTx := db.WithContext(ctx).Begin()
		if lockTx.Error != nil {
			t.Fatalf("begin lock transaction: %v", lockTx.Error)
		}
		if err := lockPlatformCategorySearchMaintenance(lockTx, false); err != nil {
			_ = lockTx.Rollback().Error
			t.Fatalf("hold maintenance lock: %v", err)
		}
		tryResult := make(chan error, 1)
		go func() {
			_, lockErr := repo.MaintainSearchIndex(context.Background(), PlatformCategorySearchMaintenanceOptions{Apply: true, TryLock: true})
			tryResult <- lockErr
		}()
		select {
		case lockErr := <-tryResult:
			if !errors.Is(lockErr, ErrPlatformCategorySearchRebuildInProgress) {
				t.Fatalf("concurrent try-lock error = %v", lockErr)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("try-lock rebuild did not return promptly")
		}
		if err := lockTx.Rollback().Error; err != nil {
			t.Fatalf("release maintenance lock: %v", err)
		}

		concurrentContext, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		start := make(chan struct{})
		errorsChannel := make(chan error, 2)
		var wait sync.WaitGroup
		wait.Add(2)
		go func() {
			defer wait.Done()
			<-start
			_, rebuildErr := repo.MaintainSearchIndex(concurrentContext, PlatformCategorySearchMaintenanceOptions{Apply: true})
			errorsChannel <- rebuildErr
		}()
		go func() {
			defer wait.Done()
			<-start
			_, updateErr := repo.UpdateAtomic(concurrentContext, fixture.Mobile, PlatformCategoryMutation{
				ParentID: uintPointer(fixture.Electronics), Name: "Mobile", SortOrder: 23,
				Status: models.PlatformCategoryStatusActive,
			})
			errorsChannel <- updateErr
		}()
		close(start)
		wait.Wait()
		close(errorsChannel)
		for concurrentErr := range errorsChannel {
			if concurrentErr != nil {
				t.Fatalf("concurrent maintenance error: %v", concurrentErr)
			}
		}
		assertPlatformCategorySearchPostgresTwoRowsPerCategory(t, db, 10)
	})
}

func TestPlatformCategorySearchPostgresInfrastructureErrorsAreNotSwallowed(t *testing.T) {
	db := openPlatformCategorySearchPostgresDB(t, false)
	err := searchdatabase.EnsurePlatformCategorySearchInfrastructure(db)
	if err == nil || !strings.Contains(err.Error(), "create platform category search_text trigram index") {
		t.Fatalf("missing-table infrastructure error = %v", err)
	}
}

func TestPlatformCategorySearchPostgresUnexpectedDatabaseErrorsAreNotSwallowed(t *testing.T) {
	db := openPlatformCategorySearchPostgresDB(t, true)
	if err := searchdatabase.EnsurePlatformCategorySearchInfrastructure(db); err != nil {
		t.Fatalf("create infrastructure: %v", err)
	}
	repo := NewPlatformCategoryRepository(db)
	if err := db.Migrator().DropTable(&models.PlatformCategorySearchIndex{}); err != nil {
		t.Fatalf("drop search table: %v", err)
	}
	if _, err := repo.Search(t.Context(), "mobile", 50, true, "all"); err == nil {
		t.Fatal("Search swallowed missing-table database error")
	}
	if _, err := repo.MaintainSearchIndex(t.Context(), PlatformCategorySearchMaintenanceOptions{}); err == nil {
		t.Fatal("MaintainSearchIndex swallowed missing-table database error")
	}
}

func TestPlatformCategorySearchPostgresThirtyThousandCategoryPlan(t *testing.T) {
	db := openPlatformCategorySearchPostgresDB(t, true)
	if err := searchdatabase.EnsurePlatformCategorySearchInfrastructure(db); err != nil {
		t.Fatalf("create infrastructure: %v", err)
	}
	repo := NewPlatformCategoryRepository(db)
	rootID, accessoryIDs := seedPlatformCategorySearchPostgresLargeHistory(t, db, 30000)

	started := time.Now()
	summary, err := repo.MaintainSearchIndex(t.Context(), PlatformCategorySearchMaintenanceOptions{
		Apply: true, Verify: true, BatchSize: 500,
	})
	if err != nil {
		t.Fatalf("rebuild 30k history: %v", err)
	}
	if summary.Categories != 30000 || summary.ExpectedRows != 60000 || summary.Created != 60000 || !summary.Verified {
		t.Fatalf("30k rebuild summary = %+v", summary)
	}
	var indexRows int64
	if err := db.Model(&models.PlatformCategorySearchIndex{}).Count(&indexRows).Error; err != nil || indexRows != 60000 {
		t.Fatalf("large fixture index rows = %d, %v", indexRows, err)
	}
	if err := db.Exec("ANALYZE platform_category_search_indexes").Error; err != nil {
		t.Fatalf("analyze search index table: %v", err)
	}
	t.Logf("30k/60k rebuild duration=%s missing_translations=%d fallback_zh_rows=%d", time.Since(started), summary.MissingTranslations, summary.FallbackZhRows)

	for _, test := range []struct {
		keyword string
		locale  string
	}{
		{keyword: "智能手机", locale: "zh"},
		{keyword: "mobile", locale: "all"},
		{keyword: "accessories", locale: "en"},
		{keyword: "phone accessories", locale: "all"},
	} {
		t.Run("plan "+test.keyword, func(t *testing.T) {
			plan := explainPlatformCategorySearchPostgres(t, db, test.keyword, 100, true, test.locale)
			if !platformCategorySearchPlanUsesIndex(plan, searchdatabase.PlatformCategorySearchTextTrigramIndex) {
				t.Fatalf("query plan for %q did not use search_text GIN index: %s", test.keyword, platformCategorySearchPlanSummary(plan))
			}
			if plan.ExecutionTime > 10000 {
				t.Fatalf("query plan for %q took %.3fms", test.keyword, plan.ExecutionTime)
			}
			t.Logf("EXPLAIN keyword=%q locale=%s %s", test.keyword, test.locale, platformCategorySearchPlanSummary(plan))
		})
	}

	shortPlan := explainPlatformCategorySearchPostgres(t, db, "手机", 100, true, "zh")
	if shortPlan.ExecutionTime > 10000 {
		t.Fatalf("short Chinese query took %.3fms", shortPlan.ExecutionTime)
	}
	t.Logf("EXPLAIN short keyword=%q (GIN not required) %s", "手机", platformCategorySearchPlanSummary(shortPlan))

	if matches := platformCategorySearchMatchedIDs(t, db, "phone accessories", 50, true, "all"); len(matches) != 50 || !platformCategorySearchIDsUnique(matches) {
		t.Fatalf("locale=all limit=50 dedupe = %d/%v", len(matches), matches)
	}
	if matches := platformCategorySearchMatchedIDs(t, db, "phone accessories", 100, true, "all"); len(matches) != 100 || !platformCategorySearchIDsUnique(matches) {
		t.Fatalf("locale=all limit=100 dedupe = %d", len(matches))
	}
	for _, locale := range []string{"en", "zh"} {
		if matches := platformCategorySearchMatchedIDs(t, db, "phone accessories", 100, true, locale); len(matches) != 100 {
			t.Fatalf("locale=%s matches = %d, want 100", locale, len(matches))
		}
	}

	requested := make([]models.PlatformCategory, 0, 100)
	for _, id := range accessoryIDs[:100] {
		requested = append(requested, platformCategorySearchLoadCategory(t, db, id))
	}
	if err := db.Where("category_id IN ? AND locale = ?", accessoryIDs[:100], "zh").Delete(&models.PlatformCategorySearchIndex{}).Error; err != nil {
		t.Fatalf("force local display fallback: %v", err)
	}
	recorder := newPlatformCategorySearchQueryRecorder()
	boundedRepo := NewPlatformCategoryRepository(db.Session(&gorm.Session{Logger: recorder}))
	localized, err := boundedRepo.AttachBilingualDisplay(t.Context(), requested)
	if err != nil {
		t.Fatalf("bounded 100-result fallback: %v", err)
	}
	if len(localized) != 100 || localized[0].ZhPath == "" {
		t.Fatalf("bounded fallback results = %d, first=%+v", len(localized), localized[0])
	}
	statements := recorder.Statements()
	if len(statements) != 3 {
		t.Fatalf("100-result fallback queries = %d, want fixed 3:\n%s", len(statements), strings.Join(statements, "\n---\n"))
	}
	joinedStatements := strings.Join(statements, "\n")
	if !strings.Contains(joinedStatements, "WITH RECURSIVE ancestors") || !strings.Contains(joinedStatements, "entity_id IN") {
		t.Fatalf("fallback did not use bounded ancestor/translation queries:\n%s", joinedStatements)
	}
	if strings.Contains(joinedStatements, `SELECT * FROM "platform_categories" ORDER BY`) {
		t.Fatalf("fallback loaded the full category table:\n%s", joinedStatements)
	}
	if !strings.Contains(localized[0].ZhPath, "电子产品") || rootID == 0 {
		t.Fatalf("localized fallback path/root = %q/%d", localized[0].ZhPath, rootID)
	}
}

func openPlatformCategorySearchPostgresDB(t *testing.T, migrate bool) *gorm.DB {
	t.Helper()
	if os.Getenv(platformCategorySearchPostgresAllowEnvironment) != "1" {
		t.Fatalf("%s=1 is required; this PostgreSQL integration suite must run and never skips", platformCategorySearchPostgresAllowEnvironment)
	}
	dsn := strings.TrimSpace(os.Getenv(platformCategorySearchPostgresDSNEnvironment))
	if dsn == "" {
		t.Fatalf("%s is required", platformCategorySearchPostgresDSNEnvironment)
	}
	adminDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open isolated PostgreSQL: %v", err)
	}
	var databaseName string
	if err := adminDB.Raw("SELECT current_database()").Scan(&databaseName).Error; err != nil {
		t.Fatalf("read isolated database name: %v", err)
	}
	if databaseName != platformCategorySearchPostgresDatabase {
		t.Fatalf("refusing non-isolated database %q; want %q", databaseName, platformCategorySearchPostgresDatabase)
	}

	schema := "category_search_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := `"` + schema + `"`
	if err := adminDB.Exec("CREATE SCHEMA " + quotedSchema).Error; err != nil {
		t.Fatalf("create isolated schema: %v", err)
	}
	schemaDB, err := gorm.Open(postgres.Open(dsn+" search_path="+schema+",public"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		_ = adminDB.Exec("DROP SCHEMA " + quotedSchema + " CASCADE").Error
		t.Fatalf("open schema-scoped PostgreSQL: %v", err)
	}
	sqlDB, err := schemaDB.DB()
	if err != nil {
		t.Fatalf("get PostgreSQL pool: %v", err)
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(20)
	t.Cleanup(func() {
		_ = sqlDB.Close()
		if err := adminDB.Exec("DROP SCHEMA " + quotedSchema + " CASCADE").Error; err != nil {
			t.Errorf("drop isolated schema: %v", err)
		}
		if adminSQL, openErr := adminDB.DB(); openErr == nil {
			_ = adminSQL.Close()
		}
	})
	if migrate {
		if err := schemaDB.AutoMigrate(
			&models.PlatformCategory{},
			&models.PlatformCategorySearchIndex{},
			&models.ContentTranslation{},
		); err != nil {
			t.Fatalf("migrate isolated category search schema: %v", err)
		}
		if err := schemaDB.Exec("CREATE TABLE spus (id bigserial PRIMARY KEY, platform_category_id bigint)").Error; err != nil {
			t.Fatalf("create isolated product reference table: %v", err)
		}
	}
	return schemaDB
}

func seedPlatformCategorySearchPostgresHistory(t *testing.T, db *gorm.DB) platformCategorySearchPostgresFixture {
	t.Helper()
	create := func(category *models.PlatformCategory) uint {
		t.Helper()
		if err := db.Create(category).Error; err != nil {
			t.Fatalf("create historical category %q: %v", category.Name, err)
		}
		return category.ID
	}
	electronics := &models.PlatformCategory{Name: "Electronics", Path: "Electronics", Level: 1, SortOrder: 1, Status: models.PlatformCategoryStatusActive, IsLeaf: false}
	fixture := platformCategorySearchPostgresFixture{Electronics: create(electronics)}
	mobile := &models.PlatformCategory{ParentID: uintPointer(fixture.Electronics), Name: "Mobile", Path: "Electronics/Mobile", Level: 2, SortOrder: 1, Status: models.PlatformCategoryStatusActive, IsLeaf: false}
	fixture.Mobile = create(mobile)
	fixture.Accessories = create(&models.PlatformCategory{ParentID: uintPointer(fixture.Mobile), Name: "Phone Accessories", Path: "Electronics/Mobile/Phone Accessories", Level: 3, SortOrder: 1, Status: models.PlatformCategoryStatusActive, IsLeaf: true})
	fixture.Smartphones = create(&models.PlatformCategory{ParentID: uintPointer(fixture.Mobile), Name: "Smart Smartphones", Path: "Electronics/Mobile/Smart Smartphones", Level: 3, SortOrder: 2, Status: models.PlatformCategoryStatusActive, IsLeaf: true})
	fixture.PhoneCases = create(&models.PlatformCategory{ParentID: uintPointer(fixture.Mobile), Name: "Phone Cases", Path: "Electronics/Mobile/Phone Cases", Level: 3, SortOrder: 3, Status: models.PlatformCategoryStatusActive, IsLeaf: true})
	fixture.Cameras = create(&models.PlatformCategory{ParentID: uintPointer(fixture.Electronics), Name: "Cameras", Path: "Electronics/Cameras", Level: 2, SortOrder: 4, Status: models.PlatformCategoryStatusActive, IsLeaf: true})
	fixture.Disabled = create(&models.PlatformCategory{ParentID: uintPointer(fixture.Electronics), Name: "Mobile Legacy", Path: "Electronics/Mobile Legacy", Level: 2, SortOrder: 5, Status: models.PlatformCategoryStatusDisabled, IsLeaf: true})
	fixture.Percent = create(&models.PlatformCategory{ParentID: uintPointer(fixture.Electronics), Name: "Discount 100% Phones", Path: "Electronics/Discount 100% Phones", Level: 2, SortOrder: 6, Status: models.PlatformCategoryStatusActive, IsLeaf: true})
	fixture.Underscore = create(&models.PlatformCategory{ParentID: uintPointer(fixture.Electronics), Name: "Code_Name", Path: "Electronics/Code_Name", Level: 2, SortOrder: 7, Status: models.PlatformCategoryStatusActive, IsLeaf: true})
	fixture.Home = create(&models.PlatformCategory{Name: "Home", Path: "Home", Level: 1, SortOrder: 2, Status: models.PlatformCategoryStatusActive, IsLeaf: true})

	translations := []models.ContentTranslation{
		{EntityType: "platform_category", EntityID: fixture.Electronics, FieldKey: "name", Locale: "zh", ValueText: "电子产品", Status: models.ContentTranslationStatusPublished},
		{EntityType: "platform_category", EntityID: fixture.Mobile, FieldKey: "name", Locale: "zh", ValueText: "手机", Status: models.ContentTranslationStatusPublished},
		{EntityType: "platform_category", EntityID: fixture.Accessories, FieldKey: "name", Locale: "zh", ValueText: "手机配件", Status: models.ContentTranslationStatusPublished},
		{EntityType: "platform_category", EntityID: fixture.Smartphones, FieldKey: "name", Locale: "zh", ValueText: "智能手机", Status: models.ContentTranslationStatusPublished},
		{EntityType: "platform_category", EntityID: fixture.PhoneCases, FieldKey: "name", Locale: "zh", ValueText: "被拒绝的手机壳", Status: models.ContentTranslationStatusRejected},
		{EntityType: "platform_category", EntityID: fixture.Cameras, FieldKey: "name", Locale: "zh", ValueText: "相机", Status: models.ContentTranslationStatusDraft},
		{EntityType: "platform_category", EntityID: fixture.Disabled, FieldKey: "name", Locale: "zh", ValueText: "老旧手机", Status: models.ContentTranslationStatusPublished},
		{EntityType: "platform_category", EntityID: fixture.Percent, FieldKey: "name", Locale: "zh", ValueText: "  ", Status: models.ContentTranslationStatusPublished},
		{EntityType: "platform_category", EntityID: fixture.Home, FieldKey: "name", Locale: "zh", ValueText: "家居", Status: models.ContentTranslationStatusPublished},
	}
	if err := db.Create(&translations).Error; err != nil {
		t.Fatalf("create historical translations: %v", err)
	}
	return fixture
}

func assertPlatformCategorySearchPostgresInfrastructure(t *testing.T, db *gorm.DB) {
	t.Helper()
	status, err := searchdatabase.CheckPlatformCategorySearchInfrastructure(db)
	if err != nil || !status.Ready() {
		t.Fatalf("search infrastructure = %+v, %v", status, err)
	}
	type indexDefinition struct {
		IndexName string
		IndexDef  string
	}
	var definitions []indexDefinition
	if err := db.Raw(`
		SELECT indexname AS index_name, indexdef AS index_def
		FROM pg_indexes
		WHERE schemaname = current_schema()
		  AND tablename = 'platform_category_search_indexes'
		  AND indexname IN (?, ?)
	`, searchdatabase.PlatformCategorySearchTextTrigramIndex, searchdatabase.PlatformCategorySearchOrderIndex).Scan(&definitions).Error; err != nil {
		t.Fatalf("load PostgreSQL index definitions: %v", err)
	}
	if len(definitions) != 2 {
		t.Fatalf("search index definitions = %+v", definitions)
	}
	joined := strings.ToLower(fmt.Sprintf("%+v", definitions))
	if !strings.Contains(joined, "using gin") || !strings.Contains(joined, "gin_trgm_ops") ||
		!strings.Contains(joined, "status, locale, level, sort_order, category_id") {
		t.Fatalf("unexpected search index definitions: %+v", definitions)
	}
}

func assertPlatformCategorySearchPostgresTwoRowsPerCategory(t *testing.T, db *gorm.DB, categories int64) {
	t.Helper()
	var total int64
	if err := db.Model(&models.PlatformCategorySearchIndex{}).Count(&total).Error; err != nil {
		t.Fatalf("count search index rows: %v", err)
	}
	if total != categories*2 {
		t.Fatalf("search index rows = %d, want %d", total, categories*2)
	}
	type invalidCount struct {
		CategoryID uint
		Rows       int64
		Locales    int64
	}
	var invalid []invalidCount
	if err := db.Raw(`
		SELECT category_id, COUNT(*) AS rows, COUNT(DISTINCT locale) AS locales
		FROM platform_category_search_indexes
		GROUP BY category_id
		HAVING COUNT(*) <> 2 OR COUNT(DISTINCT locale) <> 2
	`).Scan(&invalid).Error; err != nil {
		t.Fatalf("verify per-category row counts: %v", err)
	}
	if len(invalid) != 0 {
		t.Fatalf("invalid per-category rows: %+v", invalid)
	}
}

func assertPlatformCategorySearchPostgresDerivedRows(t *testing.T, db *gorm.DB, fixture platformCategorySearchPostgresFixture) {
	t.Helper()
	var en, zh, fallback models.PlatformCategorySearchIndex
	if err := db.Where("category_id = ? AND locale = ?", fixture.Accessories, "en").Take(&en).Error; err != nil {
		t.Fatalf("load English derived row: %v", err)
	}
	if err := db.Where("category_id = ? AND locale = ?", fixture.Accessories, "zh").Take(&zh).Error; err != nil {
		t.Fatalf("load Chinese derived row: %v", err)
	}
	if err := db.Where("category_id = ? AND locale = ?", fixture.Cameras, "zh").Take(&fallback).Error; err != nil {
		t.Fatalf("load fallback derived row: %v", err)
	}
	if en.Name != "Phone Accessories" || en.Path != "Electronics > Mobile > Phone Accessories" || en.Status != models.PlatformCategoryStatusActive || en.Level != 3 {
		t.Fatalf("English derived row = %+v", en)
	}
	if zh.Name != "手机配件" || zh.Path != "电子产品 > 手机 > 手机配件" ||
		!strings.Contains(zh.SearchText, "手机配件") || !strings.Contains(zh.SearchText, "Phone Accessories") ||
		!strings.Contains(zh.SearchText, "Electronics > Mobile > Phone Accessories") {
		t.Fatalf("Chinese derived row = %+v", zh)
	}
	if fallback.Name != "Cameras" || fallback.Path != "电子产品 > Cameras" {
		t.Fatalf("draft translation entered public index or fallback is wrong: %+v", fallback)
	}
}

func assertPlatformCategorySearchContains(t *testing.T, repo *PlatformCategoryRepository, keyword, locale string, activeOnly bool, categoryID uint) {
	t.Helper()
	results, err := repo.Search(t.Context(), keyword, 100, activeOnly, locale)
	if err != nil {
		t.Fatalf("Search(%q, %s): %v", keyword, locale, err)
	}
	if !platformCategorySearchResultsContain(results, categoryID) {
		t.Fatalf("Search(%q, %s) IDs=%v, want %d", keyword, locale, platformCategorySearchResultIDs(results), categoryID)
	}
}

func assertPlatformCategorySearchDoesNotContain(t *testing.T, repo *PlatformCategoryRepository, keyword, locale string, activeOnly bool, categoryID uint) {
	t.Helper()
	results, err := repo.Search(t.Context(), keyword, 100, activeOnly, locale)
	if err != nil {
		t.Fatalf("Search(%q, %s): %v", keyword, locale, err)
	}
	if platformCategorySearchResultsContain(results, categoryID) {
		t.Fatalf("Search(%q, %s) unexpectedly returned %d: %v", keyword, locale, categoryID, platformCategorySearchResultIDs(results))
	}
}

func platformCategorySearchMatchedIDs(t *testing.T, db *gorm.DB, keyword string, limit int, activeOnly bool, locale string) []uint {
	t.Helper()
	query, err := categorysearch.Parse(keyword)
	if err != nil {
		t.Fatalf("parse search query %q: %v", keyword, err)
	}
	ids, err := searchPlatformCategoryIDs(db, query, limit, activeOnly, normalizePlatformCategorySearchLocale(locale))
	if err != nil {
		t.Fatalf("search matched IDs for %q: %v", keyword, err)
	}
	return ids
}

func platformCategorySearchIDsUnique(ids []uint) bool {
	seen := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		if _, exists := seen[id]; exists {
			return false
		}
		seen[id] = struct{}{}
	}
	return true
}

func platformCategorySearchIDsContain(ids []uint, target uint) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func assertPlatformCategorySearchIDSetContains(t *testing.T, ids []uint, target uint) {
	t.Helper()
	if !platformCategorySearchIDsContain(ids, target) {
		t.Fatalf("IDs %v do not contain %d", ids, target)
	}
}

func platformCategorySearchResultsContain(results []models.PlatformCategory, categoryID uint) bool {
	return platformCategorySearchResultByID(results, categoryID) != nil
}

func platformCategorySearchResultByID(results []models.PlatformCategory, categoryID uint) *models.PlatformCategory {
	for index := range results {
		if results[index].ID == categoryID {
			return &results[index]
		}
	}
	return nil
}

func platformCategorySearchResultIDs(results []models.PlatformCategory) []uint {
	ids := make([]uint, 0, len(results))
	for _, result := range results {
		ids = append(ids, result.ID)
	}
	return ids
}

func platformCategorySearchLoadCategory(t *testing.T, db *gorm.DB, id uint) models.PlatformCategory {
	t.Helper()
	var category models.PlatformCategory
	if err := db.Take(&category, id).Error; err != nil {
		t.Fatalf("load category %d: %v", id, err)
	}
	return category
}

func assertPlatformCategorySearchIndexName(t *testing.T, db *gorm.DB, categoryID uint, locale, expected string) {
	t.Helper()
	var index models.PlatformCategorySearchIndex
	if err := db.Where("category_id = ? AND locale = ?", categoryID, locale).Take(&index).Error; err != nil {
		t.Fatalf("load search index name: %v", err)
	}
	if index.Name != expected {
		t.Fatalf("category %d locale %s name = %q, want %q", categoryID, locale, index.Name, expected)
	}
}

func assertPlatformCategorySearchIndexPath(t *testing.T, db *gorm.DB, categoryID uint, locale, expected string) {
	t.Helper()
	var index models.PlatformCategorySearchIndex
	if err := db.Where("category_id = ? AND locale = ?", categoryID, locale).Take(&index).Error; err != nil {
		t.Fatalf("load search index path: %v", err)
	}
	if index.Path != expected {
		t.Fatalf("category %d locale %s path = %q, want %q", categoryID, locale, index.Path, expected)
	}
}

func uintPointer(value uint) *uint {
	return &value
}

func seedPlatformCategorySearchPostgresLargeHistory(t *testing.T, db *gorm.DB, count int) (uint, []uint) {
	t.Helper()
	root := models.PlatformCategory{
		Name: "Electronics", Path: "Electronics", Level: 1, SortOrder: 1,
		Status: models.PlatformCategoryStatusActive, IsLeaf: false,
	}
	if err := db.Create(&root).Error; err != nil {
		t.Fatalf("create large fixture root: %v", err)
	}
	children := make([]models.PlatformCategory, 0, count-1)
	for index := 1; index < count; index++ {
		name := fmt.Sprintf("Catalog Item %05d", index)
		switch {
		case index == 1:
			name = "Smartphones Mobile Showcase"
		case index >= 2 && index < 102:
			name = fmt.Sprintf("Phone Accessories %05d", index)
		}
		children = append(children, models.PlatformCategory{
			ParentID: uintPointer(root.ID), Name: name, Path: "Electronics/" + name,
			Level: 2, SortOrder: index, Status: models.PlatformCategoryStatusActive, IsLeaf: true,
		})
	}
	if err := db.CreateInBatches(&children, 1000).Error; err != nil {
		t.Fatalf("create %d category fixture: %v", count, err)
	}
	translations := []models.ContentTranslation{
		{EntityType: "platform_category", EntityID: root.ID, FieldKey: "name", Locale: "zh", ValueText: "电子产品", Status: models.ContentTranslationStatusPublished},
		{EntityType: "platform_category", EntityID: children[0].ID, FieldKey: "name", Locale: "zh", ValueText: "智能手机", Status: models.ContentTranslationStatusPublished},
	}
	accessoryIDs := make([]uint, 0, 100)
	for index := 1; index <= 100; index++ {
		accessoryIDs = append(accessoryIDs, children[index].ID)
		translations = append(translations, models.ContentTranslation{
			EntityType: "platform_category", EntityID: children[index].ID, FieldKey: "name", Locale: "zh",
			ValueText: fmt.Sprintf("手机配件%05d", index), Status: models.ContentTranslationStatusPublished,
		})
	}
	if err := db.CreateInBatches(&translations, 500).Error; err != nil {
		t.Fatalf("create large fixture translations: %v", err)
	}
	return root.ID, accessoryIDs
}

type platformCategorySearchExplain struct {
	Plan          map[string]any `json:"Plan"`
	PlanningTime  float64        `json:"Planning Time"`
	ExecutionTime float64        `json:"Execution Time"`
}

func explainPlatformCategorySearchPostgres(t *testing.T, db *gorm.DB, keyword string, limit int, activeOnly bool, locale string) platformCategorySearchExplain {
	t.Helper()
	query, err := categorysearch.Parse(keyword)
	if err != nil {
		t.Fatalf("parse explain query %q: %v", keyword, err)
	}
	statement, arguments := buildPlatformCategorySearchMatchQuery("postgres", query, limit, activeOnly, normalizePlatformCategorySearchLocale(locale))
	var raw string
	if err := db.Raw("EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+statement, arguments...).Row().Scan(&raw); err != nil {
		t.Fatalf("EXPLAIN %q: %v", keyword, err)
	}
	var plans []platformCategorySearchExplain
	if err := json.Unmarshal([]byte(raw), &plans); err != nil || len(plans) != 1 {
		t.Fatalf("decode EXPLAIN %q: plans=%d err=%v raw=%s", keyword, len(plans), err, raw)
	}
	return plans[0]
}

func platformCategorySearchPlanUsesIndex(plan platformCategorySearchExplain, indexName string) bool {
	var walk func(any) bool
	walk = func(value any) bool {
		switch typed := value.(type) {
		case map[string]any:
			if typed["Index Name"] == indexName {
				return true
			}
			for _, child := range typed {
				if walk(child) {
					return true
				}
			}
		case []any:
			for _, child := range typed {
				if walk(child) {
					return true
				}
			}
		}
		return false
	}
	return walk(plan.Plan)
}

func platformCategorySearchPlanSummary(plan platformCategorySearchExplain) string {
	nodeTypes := make([]string, 0)
	indexes := make([]string, 0)
	var sharedHits float64
	var sharedReads float64
	var walk func(any)
	walk = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			if node, ok := typed["Node Type"].(string); ok {
				nodeTypes = append(nodeTypes, node)
			}
			if index, ok := typed["Index Name"].(string); ok {
				indexes = append(indexes, index)
			}
			if hits, ok := typed["Shared Hit Blocks"].(float64); ok {
				sharedHits += hits
			}
			if reads, ok := typed["Shared Read Blocks"].(float64); ok {
				sharedReads += reads
			}
			for _, child := range typed {
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(plan.Plan)
	return fmt.Sprintf("planning=%.3fms execution=%.3fms shared_hits=%.0f shared_reads=%.0f nodes=%s indexes=%s",
		plan.PlanningTime, plan.ExecutionTime, sharedHits, sharedReads,
		strings.Join(uniqueStrings(nodeTypes), ","), strings.Join(uniqueStrings(indexes), ","))
}

func uniqueStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

type platformCategorySearchQueryRecorder struct {
	logger.Interface
	mu         sync.Mutex
	statements []string
}

func newPlatformCategorySearchQueryRecorder() *platformCategorySearchQueryRecorder {
	return &platformCategorySearchQueryRecorder{Interface: logger.Default.LogMode(logger.Silent)}
}

func (r *platformCategorySearchQueryRecorder) Trace(_ context.Context, _ time.Time, sql func() (string, int64), _ error) {
	statement, _ := sql()
	r.mu.Lock()
	r.statements = append(r.statements, statement)
	r.mu.Unlock()
}

func (r *platformCategorySearchQueryRecorder) Statements() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.statements...)
}
