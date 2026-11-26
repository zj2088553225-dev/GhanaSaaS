package repository

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"saas/internal/models"

	"github.com/glebarez/sqlite"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

func TestPlatformCategorySubtreeIDsIncludesAllDescendantsOnly(t *testing.T) {
	categories := []models.PlatformCategory{
		{BaseModel: models.BaseModel{ID: 1}},
		{BaseModel: models.BaseModel{ID: 2}, ParentID: repositoryUintPtr(1)},
		{BaseModel: models.BaseModel{ID: 3}, ParentID: repositoryUintPtr(2)},
		{BaseModel: models.BaseModel{ID: 4}, ParentID: repositoryUintPtr(1)},
		{BaseModel: models.BaseModel{ID: 5}},
	}

	got := platformCategorySubtreeIDs(categories, 1)
	want := []uint{1, 2, 4, 3}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("subtree ids = %v, want %v", got, want)
	}
	if missing := platformCategorySubtreeIDs(categories, 999); missing != nil {
		t.Fatalf("missing subtree ids = %v, want nil", missing)
	}
}

func TestPlatformCategoryProductForeignKeyViolationIsRecognized(t *testing.T) {
	if !isPlatformCategoryProductForeignKeyViolation(&pgconn.PgError{
		Code:           "23503",
		ConstraintName: "fk_spus_platform_category",
		TableName:      "spus",
	}) {
		t.Fatal("expected product platform category FK violation to be recognized")
	}
	if !isPlatformCategoryProductForeignKeyViolation(errors.New("constraint failed: FOREIGN KEY constraint failed")) {
		t.Fatal("expected SQLite foreign key violation to be recognized")
	}
	if isPlatformCategoryProductForeignKeyViolation(&pgconn.PgError{Code: "23503", ConstraintName: "fk_other_table"}) {
		t.Fatal("unrelated PostgreSQL foreign key violation was recognized")
	}
	if isPlatformCategoryProductForeignKeyViolation(errors.New("unrelated error")) {
		t.Fatal("unrelated error was recognized")
	}
}

func TestPlatformCategoryRepositoryDeleteTreeCleansDependentRowsBeforeCategories(t *testing.T) {
	db := newPlatformCategoryRepositoryTestDB(t)
	repo := NewPlatformCategoryRepository(db)
	root := models.PlatformCategory{
		BaseModel: models.BaseModel{ID: 1},
		Name:      "Electronics",
		Path:      "Electronics",
		Level:     1,
		Status:    models.PlatformCategoryStatusActive,
		IsLeaf:    false,
	}
	child := models.PlatformCategory{
		BaseModel: models.BaseModel{ID: 2},
		ParentID:  repositoryUintPtr(1),
		Name:      "Mobile Accessories",
		Path:      "Electronics/Mobile Accessories",
		Level:     2,
		Status:    models.PlatformCategoryStatusActive,
		IsLeaf:    false,
	}
	grandchild := models.PlatformCategory{
		BaseModel: models.BaseModel{ID: 3},
		ParentID:  repositoryUintPtr(2),
		Name:      "Phone Cases",
		Path:      "Electronics/Mobile Accessories/Phone Cases",
		Level:     3,
		Status:    models.PlatformCategoryStatusActive,
		IsLeaf:    true,
	}
	if err := db.Create(&[]models.PlatformCategory{root, child, grandchild}).Error; err != nil {
		t.Fatalf("create categories: %v", err)
	}
	for _, category := range []models.PlatformCategory{root, child, grandchild} {
		createPlatformCategoryRepositoryTestSearchIndexes(t, db, category)
		if err := db.Create(&models.ContentTranslation{
			EntityType: "platform_category",
			EntityID:   category.ID,
			FieldKey:   "name",
			Locale:     "zh",
			ValueText:  "中文类目",
			Status:     models.ContentTranslationStatusPublished,
		}).Error; err != nil {
			t.Fatalf("create translation: %v", err)
		}
	}
	if err := db.Create(&models.ContentTranslation{
		EntityType: "product",
		EntityID:   2,
		FieldKey:   "name",
		Locale:     "zh",
		ValueText:  "商品翻译",
		Status:     models.ContentTranslationStatusPublished,
	}).Error; err != nil {
		t.Fatalf("create unrelated translation: %v", err)
	}

	if err := repo.DeleteTree(context.Background(), &root); err != nil {
		t.Fatalf("DeleteTree returned error: %v", err)
	}

	assertPlatformCategoryRepositoryTestCount(t, db, &models.PlatformCategory{}, "id IN ?", []uint{1, 2, 3}, 0)
	assertPlatformCategoryRepositoryTestCount(t, db, &models.PlatformCategorySearchIndex{}, "category_id IN ?", []uint{1, 2, 3}, 0)
	assertPlatformCategoryRepositoryTestCount(t, db, &models.ContentTranslation{}, "entity_type = ? AND entity_id IN ?", []any{"platform_category", []uint{1, 2, 3}}, 0)
	assertPlatformCategoryRepositoryTestCount(t, db, &models.ContentTranslation{}, "entity_type = ?", "product", 1)
}

func TestPlatformCategoryRepositoryDeleteTreeRejectsProductReferencesBeforeCleanup(t *testing.T) {
	db := newPlatformCategoryRepositoryTestDB(t)
	repo := NewPlatformCategoryRepository(db)
	root := models.PlatformCategory{
		BaseModel: models.BaseModel{ID: 1},
		Name:      "Electronics",
		Path:      "Electronics",
		Level:     1,
		Status:    models.PlatformCategoryStatusActive,
		IsLeaf:    false,
	}
	child := models.PlatformCategory{
		BaseModel: models.BaseModel{ID: 2},
		ParentID:  repositoryUintPtr(1),
		Name:      "Mobile Accessories",
		Path:      "Electronics/Mobile Accessories",
		Level:     2,
		Status:    models.PlatformCategoryStatusActive,
		IsLeaf:    true,
	}
	if err := db.Create(&[]models.PlatformCategory{root, child}).Error; err != nil {
		t.Fatalf("create categories: %v", err)
	}
	createPlatformCategoryRepositoryTestSearchIndexes(t, db, root)
	createPlatformCategoryRepositoryTestSearchIndexes(t, db, child)
	if err := db.Exec("INSERT INTO spus (id, platform_category_id) VALUES (?, ?)", 1, child.ID).Error; err != nil {
		t.Fatalf("insert product ref: %v", err)
	}

	err := repo.DeleteTree(context.Background(), &root)
	if !errors.Is(err, ErrPlatformCategoryTreeHasProducts) {
		t.Fatalf("DeleteTree error = %v, want ErrPlatformCategoryTreeHasProducts", err)
	}
	assertPlatformCategoryRepositoryTestCount(t, db, &models.PlatformCategory{}, "id IN ?", []uint{1, 2}, 2)
	assertPlatformCategoryRepositoryTestCount(t, db, &models.PlatformCategorySearchIndex{}, "category_id IN ?", []uint{1, 2}, 4)
}

func TestPlatformCategoryRepositoryImportMovesEntireSubtreeAndPreservesReferences(t *testing.T) {
	db := newPlatformCategoryRepositoryTestDB(t)
	repo := NewPlatformCategoryRepository(db)
	seedPlatformCategoryImportTree(t, db, repo)

	moveToRoot := PlatformCategoryImportOperation{
		ID:         repositoryUintPtr(2),
		Path:       "B",
		Name:       "B",
		ZhName:     "乙",
		ZhProvided: true,
		Level:      1,
		SortOrder:  20,
		Status:     models.PlatformCategoryStatusActive,
		Explicit:   true,
	}
	result, err := repo.Import(context.Background(), []PlatformCategoryImportOperation{moveToRoot})
	if err != nil {
		t.Fatalf("move subtree to root: %v", err)
	}
	if result.Updated != 1 {
		t.Fatalf("move result = %+v, want one updated category", result)
	}
	assertPlatformCategoryImportNode(t, db, 2, nil, "B", 1)
	assertPlatformCategoryImportNode(t, db, 3, repositoryUintPtr(2), "B/C", 2)
	assertPlatformCategoryImportProductReference(t, db, 3)
	assertPlatformCategoryImportIndex(t, db, 3, "en", "B > C", 2)
	assertPlatformCategoryImportIndex(t, db, 3, "zh", "乙 > 丙", 2)

	repeated, err := repo.Import(context.Background(), []PlatformCategoryImportOperation{moveToRoot})
	if err != nil {
		t.Fatalf("repeat subtree move: %v", err)
	}
	if repeated.Created != 0 || repeated.Updated != 0 || repeated.Unchanged != 1 {
		t.Fatalf("repeated result = %+v, want idempotent unchanged import", repeated)
	}

	moveDown := moveToRoot
	moveDown.Path = "D/B"
	moveDown.ParentPath = "D"
	moveDown.Level = 2
	if _, err := repo.Import(context.Background(), []PlatformCategoryImportOperation{moveDown}); err != nil {
		t.Fatalf("move subtree under D: %v", err)
	}
	assertPlatformCategoryImportNode(t, db, 2, repositoryUintPtr(4), "D/B", 2)
	assertPlatformCategoryImportNode(t, db, 3, repositoryUintPtr(2), "D/B/C", 3)
	assertPlatformCategoryImportProductReference(t, db, 3)
}

func TestPlatformCategoryRepositoryImportRejectsCyclesWithoutChanges(t *testing.T) {
	tests := []struct {
		name       string
		operations []PlatformCategoryImportOperation
	}{
		{
			name: "self parent",
			operations: []PlatformCategoryImportOperation{{
				ID: repositoryUintPtr(1), Path: "A/A", ParentPath: "A", Name: "A", Level: 2,
				Status: models.PlatformCategoryStatusActive, Explicit: true,
			}},
		},
		{
			name: "descendant parent",
			operations: []PlatformCategoryImportOperation{{
				ID: repositoryUintPtr(1), Path: "A/B/A", ParentPath: "A/B", Name: "A", Level: 3,
				Status: models.PlatformCategoryStatusActive, Explicit: true,
			}},
		},
		{
			name: "multi operation cycle",
			operations: []PlatformCategoryImportOperation{
				{
					ID: repositoryUintPtr(1), Path: "D/A", ParentPath: "D", Name: "A", Level: 2,
					Status: models.PlatformCategoryStatusActive, Explicit: true,
				},
				{
					ID: repositoryUintPtr(4), Path: "D/A/D", ParentPath: "D/A", Name: "D", Level: 3,
					Status: models.PlatformCategoryStatusActive, Explicit: true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newPlatformCategoryRepositoryTestDB(t)
			repo := NewPlatformCategoryRepository(db)
			seedPlatformCategoryImportTree(t, db, repo)
			before := capturePlatformCategoryImportSnapshot(t, db)

			_, err := repo.Import(context.Background(), tt.operations)
			if !errors.Is(err, ErrPlatformCategoryImportCycle) {
				t.Fatalf("Import error = %v, want ErrPlatformCategoryImportCycle", err)
			}
			after := capturePlatformCategoryImportSnapshot(t, db)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("database changed after rejected cycle\nbefore=%+v\nafter=%+v", before, after)
			}
		})
	}
}

func TestPlatformCategoryRepositoryImportRollsBackInjectedFailures(t *testing.T) {
	tests := []struct {
		name       string
		triggerSQL string
		operation  PlatformCategoryImportOperation
	}{
		{
			name: "category write",
			triggerSQL: `CREATE TRIGGER fail_category_import BEFORE INSERT ON platform_categories
				WHEN NEW.name = 'Y' BEGIN SELECT RAISE(FAIL, 'forced category failure'); END`,
			operation: PlatformCategoryImportOperation{
				Path: "Y", Name: "Y", ZhName: "新乙", ZhProvided: true, Level: 1,
				Status: models.PlatformCategoryStatusActive, Explicit: true,
			},
		},
		{
			name: "translation write",
			triggerSQL: `CREATE TRIGGER fail_translation_import BEFORE INSERT ON content_translations
				WHEN NEW.value_text = 'FAIL' BEGIN SELECT RAISE(FAIL, 'forced translation failure'); END`,
			operation: PlatformCategoryImportOperation{
				ID: repositoryUintPtr(2), Path: "A/B2", ParentPath: "A", Name: "B2",
				ZhName: "FAIL", ZhProvided: true, Level: 2,
				Status: models.PlatformCategoryStatusActive, Explicit: true,
			},
		},
		{
			name: "search index write",
			triggerSQL: `CREATE TRIGGER fail_search_index_import BEFORE INSERT ON platform_category_search_indexes
				WHEN NEW.category_id = 2 AND NEW.locale = 'en' BEGIN SELECT RAISE(FAIL, 'forced index failure'); END`,
			operation: PlatformCategoryImportOperation{
				ID: repositoryUintPtr(2), Path: "A/B2", ParentPath: "A", Name: "B2",
				ZhName: "乙二", ZhProvided: true, Level: 2,
				Status: models.PlatformCategoryStatusActive, Explicit: true,
			},
		},
		{
			name: "descendant update",
			triggerSQL: `CREATE TRIGGER fail_descendant_import BEFORE UPDATE ON platform_categories
				WHEN OLD.id = 3 BEGIN SELECT RAISE(FAIL, 'forced descendant failure'); END`,
			operation: PlatformCategoryImportOperation{
				ID: repositoryUintPtr(2), Path: "B", Name: "B", ZhName: "乙",
				ZhProvided: true, Level: 1,
				Status: models.PlatformCategoryStatusActive, Explicit: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newPlatformCategoryRepositoryTestDB(t)
			repo := NewPlatformCategoryRepository(db)
			seedPlatformCategoryImportTree(t, db, repo)
			before := capturePlatformCategoryImportSnapshot(t, db)
			if err := db.Exec(tt.triggerSQL).Error; err != nil {
				t.Fatalf("create failure trigger: %v", err)
			}

			operations := []PlatformCategoryImportOperation{tt.operation}
			if tt.name == "category write" {
				operations = append([]PlatformCategoryImportOperation{{
					Path: "X", Name: "X", ZhName: "新甲", ZhProvided: true, Level: 1,
					Status: models.PlatformCategoryStatusActive, Explicit: true,
				}}, operations...)
			}
			if _, err := repo.Import(context.Background(), operations); err == nil {
				t.Fatal("Import returned nil error for injected failure")
			}
			after := capturePlatformCategoryImportSnapshot(t, db)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("database changed after injected %s failure\nbefore=%+v\nafter=%+v", tt.name, before, after)
			}
		})
	}
}

func TestPlatformCategoryRepositoryCreateAtomicRollsBackInjectedFailures(t *testing.T) {
	tests := []struct {
		name       string
		triggerSQL string
		mutation   PlatformCategoryMutation
	}{
		{
			name: "category write",
			triggerSQL: `CREATE TRIGGER fail_category_create BEFORE INSERT ON platform_categories
				WHEN NEW.name = 'X' BEGIN SELECT RAISE(FAIL, 'forced category failure'); END`,
			mutation: PlatformCategoryMutation{
				ParentID: repositoryUintPtr(4), Name: "X", ZhName: "戊", ZhProvided: true,
				Status: models.PlatformCategoryStatusActive,
			},
		},
		{
			name: "translation write",
			triggerSQL: `CREATE TRIGGER fail_translation_create BEFORE INSERT ON content_translations
				WHEN NEW.value_text = 'FAIL' BEGIN SELECT RAISE(FAIL, 'forced translation failure'); END`,
			mutation: PlatformCategoryMutation{
				ParentID: repositoryUintPtr(4), Name: "X", ZhName: "FAIL", ZhProvided: true,
				Status: models.PlatformCategoryStatusActive,
			},
		},
		{
			name: "search index write",
			triggerSQL: `CREATE TRIGGER fail_search_index_create BEFORE INSERT ON platform_category_search_indexes
				WHEN NEW.category_id = 5 AND NEW.locale = 'en' BEGIN SELECT RAISE(FAIL, 'forced index failure'); END`,
			mutation: PlatformCategoryMutation{
				ParentID: repositoryUintPtr(4), Name: "X", ZhName: "戊", ZhProvided: true,
				Status: models.PlatformCategoryStatusActive,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newPlatformCategoryRepositoryTestDB(t)
			repo := NewPlatformCategoryRepository(db)
			seedPlatformCategoryImportTree(t, db, repo)
			before := capturePlatformCategoryImportSnapshot(t, db)
			if err := db.Exec(tt.triggerSQL).Error; err != nil {
				t.Fatalf("create failure trigger: %v", err)
			}

			if _, err := repo.CreateAtomic(context.Background(), tt.mutation); err == nil {
				t.Fatal("CreateAtomic returned nil error for injected failure")
			}
			after := capturePlatformCategoryImportSnapshot(t, db)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("database changed after injected %s failure\nbefore=%+v\nafter=%+v", tt.name, before, after)
			}
		})
	}
}

func TestPlatformCategoryRepositoryUpdateAtomicRollsBackInjectedFailures(t *testing.T) {
	tests := []struct {
		name       string
		triggerSQL string
		mutation   PlatformCategoryMutation
	}{
		{
			name: "translation after English path update",
			triggerSQL: `CREATE TRIGGER fail_translation_update BEFORE INSERT ON content_translations
				WHEN NEW.value_text = 'FAIL' BEGIN SELECT RAISE(FAIL, 'forced translation failure'); END`,
			mutation: PlatformCategoryMutation{
				ParentID: repositoryUintPtr(1), Name: "B2", ZhName: "FAIL", ZhProvided: true,
				SortOrder: 9, Status: models.PlatformCategoryStatusDisabled,
			},
		},
		{
			name: "search index after subtree move",
			triggerSQL: `CREATE TRIGGER fail_search_index_update BEFORE INSERT ON platform_category_search_indexes
				WHEN NEW.category_id = 2 AND NEW.locale = 'en' BEGIN SELECT RAISE(FAIL, 'forced index failure'); END`,
			mutation: PlatformCategoryMutation{
				ParentID: repositoryUintPtr(4), Name: "B", ZhName: "乙二", ZhProvided: true,
				SortOrder: 8, Status: models.PlatformCategoryStatusDisabled,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newPlatformCategoryRepositoryTestDB(t)
			repo := NewPlatformCategoryRepository(db)
			seedPlatformCategoryImportTree(t, db, repo)
			before := capturePlatformCategoryImportSnapshot(t, db)
			if err := db.Exec(tt.triggerSQL).Error; err != nil {
				t.Fatalf("create failure trigger: %v", err)
			}

			if _, err := repo.UpdateAtomic(context.Background(), 2, tt.mutation); err == nil {
				t.Fatal("UpdateAtomic returned nil error for injected failure")
			}
			after := capturePlatformCategoryImportSnapshot(t, db)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("database changed after injected %s failure\nbefore=%+v\nafter=%+v", tt.name, before, after)
			}
		})
	}
}

func TestPlatformCategoryRepositoryAtomicMutationsKeepTranslationsAndIndexesInSync(t *testing.T) {
	db := newPlatformCategoryRepositoryTestDB(t)
	repo := NewPlatformCategoryRepository(db)
	seedPlatformCategoryImportTree(t, db, repo)
	ctx := context.Background()

	created, err := repo.CreateAtomic(ctx, PlatformCategoryMutation{
		ParentID: repositoryUintPtr(4), Name: "E", ZhProvided: true,
		SortOrder: 5, Status: models.PlatformCategoryStatusActive,
	})
	if err != nil {
		t.Fatalf("CreateAtomic with English fallback: %v", err)
	}
	if created.Name != "E" || created.Path != "D/E" || created.Level != 2 {
		t.Fatalf("created category = %+v, want English main-table values", created)
	}
	assertPlatformCategoryRepositoryTestCount(t, db, &models.ContentTranslation{},
		"entity_type = ? AND entity_id = ?", []any{"platform_category", created.ID}, 0)
	assertPlatformCategoryImportIndex(t, db, created.ID, "en", "D > E", 2)
	assertPlatformCategoryImportIndex(t, db, created.ID, "zh", "丁 > E", 2)
	assertPlatformCategoryLeaf(t, db, 4, false)

	if _, err := repo.UpdateAtomic(ctx, 2, PlatformCategoryMutation{
		ParentID: repositoryUintPtr(1), Name: "B", ZhProvided: true,
		SortOrder: 7, Status: models.PlatformCategoryStatusDisabled,
	}); err != nil {
		t.Fatalf("UpdateAtomic clearing Chinese translation: %v", err)
	}
	assertPlatformCategoryRepositoryTestCount(t, db, &models.ContentTranslation{},
		"entity_type = ? AND entity_id = ? AND field_key = ? AND locale = ?",
		[]any{"platform_category", uint(2), "name", "zh"}, 0)
	assertPlatformCategoryIndexValues(t, db, 2, "zh", "B", "甲 > B", 7, models.PlatformCategoryStatusDisabled)
	assertPlatformCategoryIndexValues(t, db, 2, "en", "B", "A > B", 7, models.PlatformCategoryStatusDisabled)
}

func TestPlatformCategoryRepositoryUpdateAtomicRefreshesDescendantPathsAndBilingualIndexes(t *testing.T) {
	db := newPlatformCategoryRepositoryTestDB(t)
	repo := NewPlatformCategoryRepository(db)
	seedPlatformCategoryImportTree(t, db, repo)
	ctx := context.Background()

	if err := db.Where("entity_type = ? AND entity_id = ? AND field_key = ? AND locale = ?",
		"platform_category", 1, "name", "zh").Delete(&models.ContentTranslation{}).Error; err != nil {
		t.Fatalf("delete root translation: %v", err)
	}
	if err := repo.RebuildSearchIndex(ctx, nil); err != nil {
		t.Fatalf("rebuild fallback indexes: %v", err)
	}
	if _, err := repo.UpdateAtomic(ctx, 1, PlatformCategoryMutation{
		Name: "A2", SortOrder: 3, Status: models.PlatformCategoryStatusActive,
	}); err != nil {
		t.Fatalf("rename English ancestor: %v", err)
	}
	assertPlatformCategoryImportNode(t, db, 1, nil, "A2", 1)
	assertPlatformCategoryImportNode(t, db, 2, repositoryUintPtr(1), "A2/B", 2)
	assertPlatformCategoryImportNode(t, db, 3, repositoryUintPtr(2), "A2/B/C", 3)
	assertPlatformCategoryImportIndex(t, db, 3, "en", "A2 > B > C", 3)
	assertPlatformCategoryImportIndex(t, db, 3, "zh", "A2 > 乙 > 丙", 3)

	if _, err := repo.UpdateAtomic(ctx, 1, PlatformCategoryMutation{
		Name: "A2", ZhName: "新甲", ZhProvided: true,
		SortOrder: 3, Status: models.PlatformCategoryStatusActive,
	}); err != nil {
		t.Fatalf("rename Chinese ancestor: %v", err)
	}
	assertPlatformCategoryImportNode(t, db, 3, repositoryUintPtr(2), "A2/B/C", 3)
	assertPlatformCategoryImportIndex(t, db, 3, "en", "A2 > B > C", 3)
	assertPlatformCategoryImportIndex(t, db, 3, "zh", "新甲 > 乙 > 丙", 3)
}

func TestPlatformCategoryRepositoryUpdateAtomicMovesSubtreeAndRejectsCycles(t *testing.T) {
	db := newPlatformCategoryRepositoryTestDB(t)
	repo := NewPlatformCategoryRepository(db)
	seedPlatformCategoryImportTree(t, db, repo)
	ctx := context.Background()

	if _, err := repo.UpdateAtomic(ctx, 2, PlatformCategoryMutation{
		ParentID: repositoryUintPtr(4), Name: "B", SortOrder: 6,
		Status: models.PlatformCategoryStatusActive,
	}); err != nil {
		t.Fatalf("move subtree: %v", err)
	}
	assertPlatformCategoryImportNode(t, db, 2, repositoryUintPtr(4), "D/B", 2)
	assertPlatformCategoryImportNode(t, db, 3, repositoryUintPtr(2), "D/B/C", 3)
	assertPlatformCategoryLeaf(t, db, 1, true)
	assertPlatformCategoryLeaf(t, db, 4, false)
	assertPlatformCategoryImportIndex(t, db, 3, "en", "D > B > C", 3)
	assertPlatformCategoryImportIndex(t, db, 3, "zh", "丁 > 乙 > 丙", 3)

	for _, parentID := range []uint{2, 3} {
		before := capturePlatformCategoryImportSnapshot(t, db)
		_, err := repo.UpdateAtomic(ctx, 2, PlatformCategoryMutation{
			ParentID: repositoryUintPtr(parentID), Name: "B",
			Status: models.PlatformCategoryStatusActive,
		})
		if !errors.Is(err, ErrPlatformCategoryMutationCycle) {
			t.Fatalf("parent %d error = %v, want ErrPlatformCategoryMutationCycle", parentID, err)
		}
		after := capturePlatformCategoryImportSnapshot(t, db)
		if !reflect.DeepEqual(after, before) {
			t.Fatalf("database changed after rejected parent %d", parentID)
		}
	}
}

func newPlatformCategoryRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	if err := db.AutoMigrate(
		&models.PlatformCategory{},
		&models.PlatformCategorySearchIndex{},
		&models.ContentTranslation{},
	); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	if err := db.Exec("CREATE TABLE spus (id integer primary key, platform_category_id integer)").Error; err != nil {
		t.Fatalf("create spus table: %v", err)
	}
	return db
}

type platformCategoryImportProductReference struct {
	ID                 uint
	PlatformCategoryID uint
}

type platformCategoryImportSnapshot struct {
	Categories   []models.PlatformCategory
	Translations []models.ContentTranslation
	Indexes      []models.PlatformCategorySearchIndex
	Products     []platformCategoryImportProductReference
}

func seedPlatformCategoryImportTree(t *testing.T, db *gorm.DB, repo *PlatformCategoryRepository) {
	t.Helper()
	categories := []models.PlatformCategory{
		{BaseModel: models.BaseModel{ID: 1}, Name: "A", Path: "A", Level: 1, Status: models.PlatformCategoryStatusActive, IsLeaf: false},
		{BaseModel: models.BaseModel{ID: 2}, ParentID: repositoryUintPtr(1), Name: "B", Path: "A/B", Level: 2, Status: models.PlatformCategoryStatusActive, IsLeaf: false},
		{BaseModel: models.BaseModel{ID: 3}, ParentID: repositoryUintPtr(2), Name: "C", Path: "A/B/C", Level: 3, Status: models.PlatformCategoryStatusActive, IsLeaf: true},
		{BaseModel: models.BaseModel{ID: 4}, Name: "D", Path: "D", Level: 1, Status: models.PlatformCategoryStatusActive, IsLeaf: true},
	}
	if err := db.Create(&categories).Error; err != nil {
		t.Fatalf("create import categories: %v", err)
	}
	for categoryID, value := range map[uint]string{1: "甲", 2: "乙", 3: "丙", 4: "丁"} {
		if err := db.Create(&models.ContentTranslation{
			EntityType: "platform_category",
			EntityID:   categoryID,
			FieldKey:   "name",
			Locale:     "zh",
			ValueText:  value,
			Status:     models.ContentTranslationStatusPublished,
		}).Error; err != nil {
			t.Fatalf("create import translation: %v", err)
		}
	}
	if err := repo.RebuildSearchIndex(context.Background(), nil); err != nil {
		t.Fatalf("build initial search index: %v", err)
	}
	if err := db.Exec("INSERT INTO spus (id, platform_category_id) VALUES (?, ?)", 100, 3).Error; err != nil {
		t.Fatalf("create import product reference: %v", err)
	}
}

func capturePlatformCategoryImportSnapshot(t *testing.T, db *gorm.DB) platformCategoryImportSnapshot {
	t.Helper()
	snapshot := platformCategoryImportSnapshot{}
	if err := db.Order("id ASC").Find(&snapshot.Categories).Error; err != nil {
		t.Fatalf("snapshot categories: %v", err)
	}
	if err := db.Order("id ASC").Find(&snapshot.Translations).Error; err != nil {
		t.Fatalf("snapshot translations: %v", err)
	}
	if err := db.Order("category_id ASC, locale ASC").Find(&snapshot.Indexes).Error; err != nil {
		t.Fatalf("snapshot search indexes: %v", err)
	}
	if err := db.Table("spus").Select("id", "platform_category_id").Order("id ASC").Scan(&snapshot.Products).Error; err != nil {
		t.Fatalf("snapshot product references: %v", err)
	}
	return snapshot
}

func assertPlatformCategoryImportNode(t *testing.T, db *gorm.DB, id uint, parentID *uint, path string, level int) {
	t.Helper()
	var category models.PlatformCategory
	if err := db.First(&category, id).Error; err != nil {
		t.Fatalf("load category %d: %v", id, err)
	}
	if !samePlatformCategoryParent(category.ParentID, parentID) || category.Path != path || category.Level != level {
		t.Fatalf("category %d = %+v, want parent=%v path=%q level=%d", id, category, parentID, path, level)
	}
}

func assertPlatformCategoryImportProductReference(t *testing.T, db *gorm.DB, wantCategoryID uint) {
	t.Helper()
	var categoryID uint
	if err := db.Table("spus").Select("platform_category_id").Where("id = ?", 100).Scan(&categoryID).Error; err != nil {
		t.Fatalf("load product category reference: %v", err)
	}
	if categoryID != wantCategoryID {
		t.Fatalf("product platform_category_id = %d, want %d", categoryID, wantCategoryID)
	}
}

func assertPlatformCategoryImportIndex(t *testing.T, db *gorm.DB, categoryID uint, locale string, path string, level int) {
	t.Helper()
	var index models.PlatformCategorySearchIndex
	if err := db.Where("category_id = ? AND locale = ?", categoryID, locale).First(&index).Error; err != nil {
		t.Fatalf("load %s search index for category %d: %v", locale, categoryID, err)
	}
	if index.Path != path || index.Level != level {
		t.Fatalf("search index = %+v, want path=%q level=%d", index, path, level)
	}
}

func assertPlatformCategoryIndexValues(
	t *testing.T,
	db *gorm.DB,
	categoryID uint,
	locale string,
	name string,
	path string,
	sortOrder int,
	status models.PlatformCategoryStatus,
) {
	t.Helper()
	var index models.PlatformCategorySearchIndex
	if err := db.Where("category_id = ? AND locale = ?", categoryID, locale).First(&index).Error; err != nil {
		t.Fatalf("load %s search index for category %d: %v", locale, categoryID, err)
	}
	if index.Name != name || index.Path != path || index.SortOrder != sortOrder || index.Status != status {
		t.Fatalf("search index = %+v, want name=%q path=%q sort=%d status=%s",
			index, name, path, sortOrder, status)
	}
}

func assertPlatformCategoryLeaf(t *testing.T, db *gorm.DB, categoryID uint, want bool) {
	t.Helper()
	var category models.PlatformCategory
	if err := db.First(&category, categoryID).Error; err != nil {
		t.Fatalf("load category %d leaf state: %v", categoryID, err)
	}
	if category.IsLeaf != want {
		t.Fatalf("category %d is_leaf = %t, want %t", categoryID, category.IsLeaf, want)
	}
}

func createPlatformCategoryRepositoryTestSearchIndexes(t *testing.T, db *gorm.DB, category models.PlatformCategory) {
	t.Helper()
	for _, localeValue := range []string{"en", "zh"} {
		if err := db.Create(&models.PlatformCategorySearchIndex{
			CategoryID: category.ID,
			Locale:     localeValue,
			Name:       category.Name,
			Path:       category.Path,
			SearchText: category.Name + " " + category.Path,
			ParentID:   category.ParentID,
			Level:      category.Level,
			Status:     category.Status,
			SortOrder:  category.SortOrder,
		}).Error; err != nil {
			t.Fatalf("create search index: %v", err)
		}
	}
}

func assertPlatformCategoryRepositoryTestCount(t *testing.T, db *gorm.DB, model any, query string, args any, want int64) {
	t.Helper()
	var count int64
	var tx *gorm.DB
	if values, ok := args.([]any); ok {
		tx = db.Model(model).Where(query, values...)
	} else {
		tx = db.Model(model).Where(query, args)
	}
	if err := tx.Count(&count).Error; err != nil {
		t.Fatalf("count %s: %v", query, err)
	}
	if count != want {
		t.Fatalf("count %s = %d, want %d", query, count, want)
	}
}

func repositoryUintPtr(value uint) *uint {
	return &value
}
