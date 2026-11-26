package database

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"saas/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestEnsureProductInvariantsCreatesIdempotentConstraints(t *testing.T) {
	db := newProductInvariantTestDB(t, false)
	if err := EnsureProductInvariants(db); err != nil {
		t.Fatalf("first EnsureProductInvariants: %v", err)
	}
	if err := EnsureProductInvariants(db); err != nil {
		t.Fatalf("second EnsureProductInvariants: %v", err)
	}
	if !db.Migrator().HasConstraint(&models.SPU{}, "PlatformCategory") {
		t.Fatal("platform category foreign key is missing")
	}
	if !db.Migrator().HasIndex(&models.SKU{}, SKUCodeUniqueIndexName) {
		t.Fatal("SKU code unique index is missing")
	}

	tenant := createProductInvariantTenant(t, db, "tenant-one")
	category := createProductInvariantCategory(t, db, "Category")
	product := createProductInvariantSPU(t, db, tenant.ID, category.ID, "Product One")
	if err := db.Create(&models.SKU{ProductID: product.ID, Name: "One", ERPSKUCode: "SKU-GLOBAL-1", RetailPrice: 100, IsActive: true}).Error; err != nil {
		t.Fatalf("create first SKU: %v", err)
	}
	if err := db.Delete(category).Error; err == nil {
		t.Fatal("database allowed deletion of a platform category used by a product")
	}

	invalidProduct := models.SPU{
		TenantID:           tenant.ID,
		PlatformCategoryID: category.ID + 999,
		Name:               "Invalid Category Product",
		Status:             models.SPUStatusOffShelf,
	}
	if err := db.Create(&invalidProduct).Error; err == nil {
		t.Fatal("database accepted a product with a missing platform category")
	}

	secondProduct := createProductInvariantSPU(t, db, tenant.ID, category.ID, "Product Two")
	if err := db.Create(&models.SKU{ProductID: secondProduct.ID, Name: "Two", ERPSKUCode: "SKU-GLOBAL-1", RetailPrice: 100, IsActive: true}).Error; err == nil {
		t.Fatal("database accepted a duplicated non-empty SKU code")
	}
	for _, productID := range []uint{product.ID, secondProduct.ID} {
		if err := db.Create(&models.SKU{ProductID: productID, Name: "Legacy Empty", ERPSKUCode: "", RetailPrice: 100, IsActive: true}).Error; err != nil {
			t.Fatalf("create legacy empty SKU code: %v", err)
		}
	}
}

func TestPreflightProductInvariantsReportsOrphansAndDuplicates(t *testing.T) {
	db := newProductInvariantTestDB(t, true)
	tenant := createProductInvariantTenant(t, db, "preflight-tenant")
	category := createProductInvariantCategory(t, db, "Valid Category")
	validProduct := createProductInvariantSPU(t, db, tenant.ID, category.ID, "Valid Product")
	orphanProduct := createProductInvariantSPU(t, db, tenant.ID, category.ID+999, "Orphan Product")

	for index, productID := range []uint{validProduct.ID, orphanProduct.ID} {
		if err := db.Create(&models.SKU{
			ProductID:   productID,
			Name:        fmt.Sprintf("Duplicate %d", index+1),
			ERPSKUCode:  "DUPLICATE-CODE",
			RetailPrice: 100,
			IsActive:    true,
		}).Error; err != nil {
			t.Fatalf("create duplicated SKU fixture: %v", err)
		}
	}

	err := PreflightProductInvariants(db)
	var preflightErr *ProductInvariantPreflightError
	if !errors.As(err, &preflightErr) {
		t.Fatalf("PreflightProductInvariants error = %v, want ProductInvariantPreflightError", err)
	}
	if preflightErr.OrphanProductCount != 1 || len(preflightErr.OrphanProductIDs) != 1 || preflightErr.OrphanProductIDs[0] != orphanProduct.ID {
		t.Fatalf("orphan preflight result = %+v, want SPU %d", preflightErr, orphanProduct.ID)
	}
	if preflightErr.DuplicateSKUCodeCount != 1 || len(preflightErr.DuplicateSKUCodes) != 1 || preflightErr.DuplicateSKUCodes[0] != "DUPLICATE-CODE" {
		t.Fatalf("duplicate preflight result = %+v, want DUPLICATE-CODE", preflightErr)
	}
	if !strings.Contains(err.Error(), "docs/product-invariant-preflight.sql") {
		t.Fatalf("preflight error is not actionable: %v", err)
	}
}

func newProductInvariantTestDB(t *testing.T, disableForeignKeys bool) *gorm.DB {
	t.Helper()
	name := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	db, err := gorm.Open(
		sqlite.Open("file:product-invariants-"+name+"?mode=memory&cache=shared&_pragma=foreign_keys(1)"),
		&gorm.Config{DisableForeignKeyConstraintWhenMigrating: disableForeignKeys},
	)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Tenant{}, &models.PlatformCategory{}, &models.SPU{}, &models.SKU{}); err != nil {
		t.Fatalf("migrate product invariant test database: %v", err)
	}
	return db
}

func createProductInvariantTenant(t *testing.T, db *gorm.DB, key string) *models.Tenant {
	t.Helper()
	tenant := &models.Tenant{
		Name:      key,
		Type:      models.TenantTypeThirdParty,
		Status:    models.TenantStatusActive,
		AppKey:    key,
		AppSecret: "test-secret",
	}
	if err := db.Create(tenant).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	return tenant
}

func createProductInvariantCategory(t *testing.T, db *gorm.DB, name string) *models.PlatformCategory {
	t.Helper()
	category := &models.PlatformCategory{
		Name:   name,
		Path:   name,
		Level:  1,
		Status: models.PlatformCategoryStatusActive,
		IsLeaf: true,
	}
	if err := db.Create(category).Error; err != nil {
		t.Fatalf("create platform category: %v", err)
	}
	return category
}

func createProductInvariantSPU(t *testing.T, db *gorm.DB, tenantID uint, platformCategoryID uint, name string) *models.SPU {
	t.Helper()
	product := &models.SPU{
		TenantID:           tenantID,
		PlatformCategoryID: platformCategoryID,
		Name:               name,
		Status:             models.SPUStatusOffShelf,
	}
	if err := db.Create(product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	return product
}
