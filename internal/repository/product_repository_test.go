package repository

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"saas/internal/models"

	"github.com/glebarez/sqlite"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

func TestIsOrderItemReferenceForeignKeyViolation(t *testing.T) {
	referencedSKUError := &pgconn.PgError{
		Code:           "23503",
		ConstraintName: "fk_order_items_sku",
	}
	if !isOrderItemReferenceForeignKeyViolation(referencedSKUError) {
		t.Fatal("expected order item SKU foreign key violation to be recognized")
	}
	if isOrderItemReferenceForeignKeyViolation(&pgconn.PgError{Code: "23503", ConstraintName: "another_constraint"}) {
		t.Fatal("unrelated foreign key violation was recognized")
	}
	if !isOrderItemReferenceForeignKeyViolation(&pgconn.PgError{
		Code:           "23503",
		ConstraintName: "legacy_order_items_sku_id_fkey",
		TableName:      "order_items",
	}) {
		t.Fatal("legacy order_items SKU foreign key violation was not recognized")
	}
	if isOrderItemReferenceForeignKeyViolation(errors.New("SQLSTATE 23503")) {
		t.Fatal("plain database error text should not be recognized as a typed foreign key violation")
	}
}

func TestIsDuplicateSKUCodeError(t *testing.T) {
	if !isDuplicateSKUCodeError(&pgconn.PgError{
		Code:           "23505",
		ConstraintName: "idx_skus_erp_sku_code_unique",
	}) {
		t.Fatal("expected SKU code unique index violation to be recognized")
	}
	if !isDuplicateSKUCodeError(errors.New("constraint failed: UNIQUE constraint failed: skus.erp_sku_code")) {
		t.Fatal("expected SQLite SKU code unique violation to be recognized")
	}
	if isDuplicateSKUCodeError(&pgconn.PgError{Code: "23505", ConstraintName: "idx_tenants_app_key"}) {
		t.Fatal("unrelated unique constraint violation was recognized as a SKU conflict")
	}
}

func TestIsProductCategoryForeignKeyViolation(t *testing.T) {
	if !isProductCategoryForeignKeyViolation(&pgconn.PgError{
		Code:           "23503",
		ConstraintName: "fk_spus_platform_category",
	}) {
		t.Fatal("expected product platform category FK violation to be recognized")
	}
	if isProductCategoryForeignKeyViolation(&pgconn.PgError{Code: "23503", ConstraintName: "fk_spus_tenant"}) {
		t.Fatal("unrelated product foreign key violation was recognized")
	}
}

func TestAvailableOrderItemReferencesUsesOnlyExistingColumns(t *testing.T) {
	references := availableOrderItemReferences([]string{
		"id",
		"order_id",
		"product_id",
		"product_name",
		"price",
		"quantity",
	})
	if len(references) != 1 || references[0] != orderItemReferenceProductID {
		t.Fatalf("references = %v, want only product_id", references)
	}
	for _, reference := range references {
		if reference == orderItemReferenceSKUId {
			t.Fatal("sku_id reference must not be selected when the column does not exist")
		}
	}
	filter, values, ok := orderItemReferenceFilter(references[0], 42, []uint{7}, []string{"SKU-7"})
	if !ok || filter != "product_id = ?" || values != uint(42) {
		t.Fatalf("filter = %q values = %v ok = %v, want product_id filter", filter, values, ok)
	}
	if strings.Contains(filter, "sku_id") {
		t.Fatalf("filter = %q, must not query missing sku_id", filter)
	}
}

func TestAvailableOrderItemReferencesSupportsModelAndLegacyColumns(t *testing.T) {
	references := availableOrderItemReferences([]string{
		"sku_id",
		"spu_id",
		"sku_code",
		"erp_sku_code",
	})
	want := []orderItemReferenceKind{
		orderItemReferenceSKUId,
		orderItemReferenceSPUID,
		orderItemReferenceSKUCode,
		orderItemReferenceERPSKUCode,
	}
	if len(references) != len(want) {
		t.Fatalf("references = %v, want %v", references, want)
	}
	for index := range want {
		if references[index] != want[index] {
			t.Fatalf("references = %v, want %v", references, want)
		}
	}
}

func TestProductRepositoryCreateAtomicRollsBackInjectedFailures(t *testing.T) {
	tests := []struct {
		name        string
		triggerSQL  string
		translation ProductTranslationMutation
	}{
		{
			name: "SPU write",
			triggerSQL: `CREATE TRIGGER fail_product_spu BEFORE INSERT ON spus
				WHEN NEW.name = 'Atomic Product' BEGIN SELECT RAISE(FAIL, 'forced spu failure'); END`,
			translation: productRepositoryTestTranslation("中文商品"),
		},
		{
			name: "second SKU write",
			triggerSQL: `CREATE TRIGGER fail_product_second_sku BEFORE INSERT ON skus
				WHEN NEW.name = 'Second SKU' BEGIN SELECT RAISE(FAIL, 'forced second sku failure'); END`,
			translation: productRepositoryTestTranslation("中文商品"),
		},
		{
			name: "translation write",
			triggerSQL: `CREATE TRIGGER fail_product_translation BEFORE INSERT ON content_translations
				WHEN NEW.value_text = 'FAIL TRANSLATION' BEGIN SELECT RAISE(FAIL, 'forced translation failure'); END`,
			translation: productRepositoryTestTranslation("FAIL TRANSLATION"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newAtomicProductRepositoryTestDB(t)
			repo := NewProductRepository(db)
			before := captureAtomicProductSnapshot(t, db)
			if err := db.Exec(tt.triggerSQL).Error; err != nil {
				t.Fatalf("create failure trigger: %v", err)
			}
			product := productRepositoryTestProduct(1001, 11, "Atomic Product", "FIRST-CODE", "SECOND-CODE")
			if err := repo.CreateAtomic(context.Background(), 1001, product, tt.translation, nil); err == nil {
				t.Fatal("CreateAtomic returned nil error for injected failure")
			}
			after := captureAtomicProductSnapshot(t, db)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("database changed after injected %s failure\nbefore=%+v\nafter=%+v", tt.name, before, after)
			}
		})
	}
}

func TestProductRepositoryCreateAtomicRejectsInvalidCategoryAndRollsBackSKUConflict(t *testing.T) {
	db := newAtomicProductRepositoryTestDB(t)
	repo := NewProductRepository(db)
	ctx := context.Background()

	invalid := productRepositoryTestProduct(1001, 10, "Invalid Category", "INVALID-CODE")
	if err := repo.CreateAtomic(ctx, 1001, invalid, productRepositoryTestTranslation("无效"), nil); !errors.Is(err, ErrProductCategoryMissing) {
		t.Fatalf("invalid category error = %v, want ErrProductCategoryMissing", err)
	}
	assertAtomicProductCounts(t, db, 0, 0, 0)

	seed := productRepositoryTestProduct(1001, 11, "Existing Product", "TAKEN-CODE")
	if err := repo.CreateAtomic(ctx, 1001, seed, productRepositoryTestTranslation("已有商品"), nil); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	before := captureAtomicProductSnapshot(t, db)
	conflict := productRepositoryTestProduct(1001, 11, "Conflicting Product", "TAKEN-CODE")
	if err := repo.CreateAtomic(ctx, 1001, conflict, productRepositoryTestTranslation("冲突商品"), nil); !errors.Is(err, ErrDuplicateSKUCode) {
		t.Fatalf("duplicate SKU error = %v, want ErrDuplicateSKUCode", err)
	}
	after := captureAtomicProductSnapshot(t, db)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("database changed after SKU conflict\nbefore=%+v\nafter=%+v", before, after)
	}
}

func TestProductRepositoryCreateManyAtomicRollsBackWholeImportBatch(t *testing.T) {
	db := newAtomicProductRepositoryTestDB(t)
	repo := NewProductRepository(db)
	products := []models.SPU{
		*productRepositoryTestProduct(1001, 11, "Import Product One", "IMPORT-CODE-1"),
		*productRepositoryTestProduct(1001, 11, "Import Product Two", "IMPORT-CODE-2"),
	}
	products[1].SKUs[0].Name = "FAIL IMPORT SKU"
	if err := db.Exec(`CREATE TRIGGER fail_import_second_product_sku BEFORE INSERT ON skus
		WHEN NEW.name = 'FAIL IMPORT SKU' BEGIN SELECT RAISE(FAIL, 'forced import sku failure'); END`).Error; err != nil {
		t.Fatalf("create import failure trigger: %v", err)
	}

	if err := repo.CreateManyAtomic(context.Background(), 1001, products, nil); err == nil {
		t.Fatal("CreateManyAtomic returned nil error for second product failure")
	}
	assertAtomicProductCounts(t, db, 0, 0, 0)
}

func TestProductRepositoryUpdateAtomicRollsBackSKUAndTranslationFailures(t *testing.T) {
	tests := []struct {
		name       string
		triggerSQL string
		build      func(models.SPU) (*models.SPU, ProductTranslationMutation, SKUCodeGenerator)
	}{
		{
			name: "SKU update after SPU update",
			triggerSQL: `CREATE TRIGGER fail_product_sku_update BEFORE UPDATE ON skus
				WHEN NEW.name = 'FAIL SKU' BEGIN SELECT RAISE(FAIL, 'forced sku update failure'); END`,
			build: func(existing models.SPU) (*models.SPU, ProductTranslationMutation, SKUCodeGenerator) {
				updated := productRepositoryUpdatedProduct(existing, "Updated English")
				updated.SKUs[0].Name = "FAIL SKU"
				return updated, productRepositoryTestTranslation("新中文"), nil
			},
		},
		{
			name: "translation after removed SKU",
			triggerSQL: `CREATE TRIGGER fail_product_translation_remove BEFORE INSERT ON content_translations
				WHEN NEW.value_text = 'FAIL TRANSLATION' BEGIN SELECT RAISE(FAIL, 'forced translation failure'); END`,
			build: func(existing models.SPU) (*models.SPU, ProductTranslationMutation, SKUCodeGenerator) {
				updated := productRepositoryUpdatedProduct(existing, "Updated English")
				updated.SKUs = updated.SKUs[:1]
				return updated, productRepositoryTestTranslation("FAIL TRANSLATION"), nil
			},
		},
		{
			name: "translation after added SKU",
			triggerSQL: `CREATE TRIGGER fail_product_translation_add BEFORE INSERT ON content_translations
				WHEN NEW.value_text = 'FAIL TRANSLATION' BEGIN SELECT RAISE(FAIL, 'forced translation failure'); END`,
			build: func(existing models.SPU) (*models.SPU, ProductTranslationMutation, SKUCodeGenerator) {
				updated := productRepositoryUpdatedProduct(existing, "Updated English")
				updated.SKUs = append(updated.SKUs, models.SKU{
					Name: "New SKU", RetailPrice: 3000, SellableStock: 3, IsActive: true,
				})
				generator := func(_ SKUCodeLookup) error {
					updated.SKUs[len(updated.SKUs)-1].ERPSKUCode = "NEW-CODE"
					return nil
				}
				return updated, productRepositoryTestTranslation("FAIL TRANSLATION"), generator
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newAtomicProductRepositoryTestDB(t)
			repo := NewProductRepository(db)
			existing := seedAtomicProductRepositoryProduct(t, repo)
			before := captureAtomicProductSnapshot(t, db)
			if err := db.Exec(tt.triggerSQL).Error; err != nil {
				t.Fatalf("create failure trigger: %v", err)
			}
			updated, translation, generator := tt.build(existing)
			if err := repo.UpdateAtomic(context.Background(), 1001, updated, translation, generator); err == nil {
				t.Fatal("UpdateAtomic returned nil error for injected failure")
			}
			after := captureAtomicProductSnapshot(t, db)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("database changed after injected %s failure\nbefore=%+v\nafter=%+v", tt.name, before, after)
			}
		})
	}
}

func TestProductRepositoryUpdateAtomicClearsTranslationsAndEnforcesTenant(t *testing.T) {
	db := newAtomicProductRepositoryTestDB(t)
	repo := NewProductRepository(db)
	existing := seedAtomicProductRepositoryProduct(t, repo)
	ctx := context.Background()

	updated := productRepositoryUpdatedProduct(existing, "English Updated")
	if err := repo.UpdateAtomic(ctx, 1001, updated, ProductTranslationMutation{Provided: true}, nil); err != nil {
		t.Fatalf("clear translations: %v", err)
	}
	var saved models.SPU
	if err := db.First(&saved, existing.ID).Error; err != nil {
		t.Fatalf("load updated SPU: %v", err)
	}
	if saved.Name != "English Updated" {
		t.Fatalf("English name = %q, want English Updated", saved.Name)
	}
	assertAtomicProductCounts(t, db, 1, 2, 0)

	before := captureAtomicProductSnapshot(t, db)
	crossTenant := productRepositoryUpdatedProduct(existing, "Cross Tenant")
	crossTenant.TenantID = 1002
	if err := repo.UpdateAtomic(ctx, 1002, crossTenant, productRepositoryTestTranslation("越权"), nil); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-tenant update error = %v, want record not found", err)
	}
	after := captureAtomicProductSnapshot(t, db)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("tenant B product state changed after tenant A write\nbefore=%+v\nafter=%+v", before, after)
	}
}

func TestProductRepositoryUpdateAtomicPreservesCodesAndAppliesSKUDiff(t *testing.T) {
	db := newAtomicProductRepositoryTestDB(t)
	repo := NewProductRepository(db)
	existing := seedAtomicProductRepositoryProduct(t, repo)
	updated := productRepositoryUpdatedProduct(existing, "Diff Updated")
	updated.SKUs = []models.SKU{
		updated.SKUs[0],
		{Name: "New SKU", ERPSKUCode: "CLIENT-CODE", RetailPrice: 3000, SellableStock: 3, IsActive: true},
	}
	updated.SKUs[0].ERPSKUCode = "CLIENT-CHANGED-CODE"
	generator := func(lookup SKUCodeLookup) error {
		existingCodes, err := lookup([]string{"NEW-SERVER-CODE"})
		if err != nil {
			return err
		}
		if len(existingCodes) != 0 {
			t.Fatalf("new SKU code unexpectedly occupied: %v", existingCodes)
		}
		updated.SKUs[1].ERPSKUCode = "NEW-SERVER-CODE"
		return nil
	}
	if err := repo.UpdateAtomic(context.Background(), 1001, updated, productRepositoryTestTranslation("差异更新"), generator); err != nil {
		t.Fatalf("UpdateAtomic SKU diff: %v", err)
	}

	var skus []models.SKU
	if err := db.Where("product_id = ?", existing.ID).Order("id ASC").Find(&skus).Error; err != nil {
		t.Fatalf("load SKU diff: %v", err)
	}
	if len(skus) != 3 {
		t.Fatalf("SKUs = %+v, want two historical rows and one new row", skus)
	}
	if skus[0].ID != existing.SKUs[0].ID || skus[0].ERPSKUCode != "OLD-CODE-1" || !skus[0].IsActive {
		t.Fatalf("preserved SKU = %+v, want original ID/code active", skus[0])
	}
	if skus[1].ID != existing.SKUs[1].ID || skus[1].ERPSKUCode != "OLD-CODE-2" || skus[1].IsActive {
		t.Fatalf("removed SKU = %+v, want original row inactive", skus[1])
	}
	if skus[2].ID == 0 || skus[2].ERPSKUCode != "NEW-SERVER-CODE" || !skus[2].IsActive {
		t.Fatalf("new SKU = %+v, want server code active", skus[2])
	}
}

type atomicProductSnapshot struct {
	Products     []models.SPU
	SKUs         []models.SKU
	Translations []models.ContentTranslation
}

func newAtomicProductRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	name := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Tenant{},
		&models.PlatformCategory{},
		&models.Category{},
		&models.SPU{},
		&models.SKU{},
		&models.ContentTranslation{},
	); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	if err := db.Exec("CREATE UNIQUE INDEX idx_skus_erp_sku_code_unique ON skus(erp_sku_code) WHERE erp_sku_code <> ''").Error; err != nil {
		t.Fatalf("create SKU code unique index: %v", err)
	}
	tenants := []models.Tenant{
		{BaseModel: models.BaseModel{ID: 1001}, Name: "Tenant A", Type: models.TenantTypeThirdParty, Status: models.TenantStatusActive, AppKey: "tenant-a"},
		{BaseModel: models.BaseModel{ID: 1002}, Name: "Tenant B", Type: models.TenantTypeThirdParty, Status: models.TenantStatusActive, AppKey: "tenant-b"},
	}
	if err := db.Create(&tenants).Error; err != nil {
		t.Fatalf("seed tenants: %v", err)
	}
	platformCategories := []models.PlatformCategory{
		{BaseModel: models.BaseModel{ID: 10}, Name: "Root", Path: "Root", Level: 1, Status: models.PlatformCategoryStatusActive, IsLeaf: false},
		{BaseModel: models.BaseModel{ID: 11}, ParentID: productRepositoryUintPtr(10), Name: "Leaf", Path: "Root/Leaf", Level: 2, Status: models.PlatformCategoryStatusActive, IsLeaf: true},
	}
	if err := db.Create(&platformCategories).Error; err != nil {
		t.Fatalf("seed platform categories: %v", err)
	}
	shopCategories := []models.Category{
		{BaseModel: models.BaseModel{ID: 21}, TenantID: 1001, Name: "Tenant A Category", IsActive: true},
		{BaseModel: models.BaseModel{ID: 22}, TenantID: 1002, Name: "Tenant B Category", IsActive: true},
	}
	if err := db.Create(&shopCategories).Error; err != nil {
		t.Fatalf("seed shop categories: %v", err)
	}
	return db
}

func productRepositoryTestProduct(tenantID uint, platformCategoryID uint, name string, codes ...string) *models.SPU {
	categoryID := uint(21)
	if tenantID == 1002 {
		categoryID = 22
	}
	product := &models.SPU{
		TenantID: tenantID, PlatformCategoryID: platformCategoryID, CategoryID: &categoryID,
		Name: name, Description: "Old description", Status: models.SPUStatusOffShelf,
	}
	for index, code := range codes {
		product.SKUs = append(product.SKUs, models.SKU{
			Name: []string{"First SKU", "Second SKU"}[index], ERPSKUCode: code,
			RetailPrice: int64(1000 + index), SellableStock: int64(10 + index), IsActive: true,
		})
	}
	return product
}

func productRepositoryTestTranslation(name string) ProductTranslationMutation {
	return ProductTranslationMutation{
		Provided: true, Name: name, Description: "中文描述", DetailHTML: "<p>中文详情</p>",
		Material: "材料", Dimensions: "尺寸", Weight: "重量",
	}
}

func seedAtomicProductRepositoryProduct(t *testing.T, repo *ProductRepository) models.SPU {
	t.Helper()
	product := productRepositoryTestProduct(1001, 11, "Old English", "OLD-CODE-1", "OLD-CODE-2")
	if err := repo.CreateAtomic(context.Background(), 1001, product, productRepositoryTestTranslation("旧中文"), nil); err != nil {
		t.Fatalf("seed atomic product: %v", err)
	}
	return *product
}

func productRepositoryUpdatedProduct(existing models.SPU, name string) *models.SPU {
	updated := existing
	updated.Name = name
	updated.Description = "New description"
	updated.SKUs = append([]models.SKU(nil), existing.SKUs...)
	for i := range updated.SKUs {
		updated.SKUs[i].Name += " Updated"
		updated.SKUs[i].RetailPrice += 100
	}
	return &updated
}

func captureAtomicProductSnapshot(t *testing.T, db *gorm.DB) atomicProductSnapshot {
	t.Helper()
	snapshot := atomicProductSnapshot{}
	if err := db.Order("id ASC").Find(&snapshot.Products).Error; err != nil {
		t.Fatalf("snapshot products: %v", err)
	}
	if err := db.Order("id ASC").Find(&snapshot.SKUs).Error; err != nil {
		t.Fatalf("snapshot SKUs: %v", err)
	}
	if err := db.Order("id ASC").Find(&snapshot.Translations).Error; err != nil {
		t.Fatalf("snapshot translations: %v", err)
	}
	return snapshot
}

func assertAtomicProductCounts(t *testing.T, db *gorm.DB, products int64, skus int64, translations int64) {
	t.Helper()
	for _, check := range []struct {
		model any
		want  int64
		name  string
	}{
		{model: &models.SPU{}, want: products, name: "products"},
		{model: &models.SKU{}, want: skus, name: "SKUs"},
		{model: &models.ContentTranslation{}, want: translations, name: "translations"},
	} {
		var count int64
		if err := db.Model(check.model).Count(&count).Error; err != nil {
			t.Fatalf("count %s: %v", check.name, err)
		}
		if count != check.want {
			t.Fatalf("%s count = %d, want %d", check.name, count, check.want)
		}
	}
}

func productRepositoryUintPtr(value uint) *uint { return &value }
