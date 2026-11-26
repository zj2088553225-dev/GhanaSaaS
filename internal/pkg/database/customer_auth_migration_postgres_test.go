package database

import (
	"os"
	"strings"
	"testing"

	"saas/internal/models"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCustomerEmailAuthMigrationIsRepeatableAndPreservesLegacyRows(t *testing.T) {
	if os.Getenv("PLATFORM_CATEGORY_SEARCH_TEST_ALLOW_ISOLATED") != "1" {
		t.Skip("isolated PostgreSQL integration environment is not configured")
	}
	dsn := strings.TrimSpace(os.Getenv("PLATFORM_CATEGORY_SEARCH_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Fatal("PLATFORM_CATEGORY_SEARCH_TEST_POSTGRES_DSN is required")
	}
	adminDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open isolated PostgreSQL: %v", err)
	}
	var databaseName string
	if err := adminDB.Raw("SELECT current_database()").Scan(&databaseName).Error; err != nil {
		t.Fatalf("read isolated database name: %v", err)
	}
	if databaseName != "platform_category_search_test" {
		t.Fatalf("refusing non-isolated database %q", databaseName)
	}

	schema := "customer_auth_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := `"` + schema + `"`
	if err := adminDB.Exec("CREATE SCHEMA " + quotedSchema).Error; err != nil {
		t.Fatalf("create isolated schema: %v", err)
	}
	t.Cleanup(func() {
		_ = adminDB.Exec("DROP SCHEMA " + quotedSchema + " CASCADE").Error
		if sqlDB, openErr := adminDB.DB(); openErr == nil {
			_ = sqlDB.Close()
		}
	})

	db, err := gorm.Open(
		postgres.Open(dsn+" search_path="+schema+",public"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)},
	)
	if err != nil {
		t.Fatalf("open schema-scoped PostgreSQL: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE customers (
			id bigserial PRIMARY KEY,
			created_at timestamptz NOT NULL DEFAULT NOW(),
			updated_at timestamptz NOT NULL DEFAULT NOW(),
			phone varchar(32),
			email varchar(255),
			password_hash varchar(255) NOT NULL,
			nickname varchar(120),
			avatar_url varchar(500),
			status varchar(32) NOT NULL,
			last_login_at timestamptz
		)
	`).Error; err != nil {
		t.Fatalf("create legacy customer table: %v", err)
	}
	if err := db.Exec("CREATE UNIQUE INDEX idx_customers_phone ON customers (phone)").Error; err != nil {
		t.Fatalf("create legacy phone index: %v", err)
	}
	if err := db.Exec("CREATE UNIQUE INDEX idx_customers_email ON customers (email)").Error; err != nil {
		t.Fatalf("create legacy email index: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO customers (phone, email, password_hash, status)
		VALUES
			('233500000001', NULL, 'legacy-hash-1', 'active'),
			(NULL, 'Buyer@Example.COM', 'legacy-hash-2', 'active'),
			(NULL, 'buyer@example.com', 'legacy-hash-3', 'disabled')
	`).Error; err != nil {
		t.Fatalf("seed legacy customers: %v", err)
	}

	for run := 1; run <= 2; run++ {
		if err := prepareCustomerEmailAuthSchema(db); err != nil {
			t.Fatalf("prepare migration run %d: %v", run, err)
		}
	}
	if err := db.AutoMigrate(&models.Customer{}, &models.PasswordResetToken{}); err != nil {
		t.Fatalf("auto migrate customer auth schema: %v", err)
	}

	var rowCount, normalizedCount, placeholderCount int64
	if err := db.Table("customers").Count(&rowCount).Error; err != nil {
		t.Fatalf("count preserved customers: %v", err)
	}
	if err := db.Table("customers").
		Where("email = LOWER(BTRIM(email)) AND email IS NOT NULL").
		Count(&normalizedCount).Error; err != nil {
		t.Fatalf("count normalized customers: %v", err)
	}
	if err := db.Table("customers").
		Where("email LIKE ?", "%@users.invalid").
		Count(&placeholderCount).Error; err != nil {
		t.Fatalf("count legacy placeholders: %v", err)
	}
	if rowCount != 3 || normalizedCount != 3 || placeholderCount != 2 {
		t.Fatalf(
			"migration rows/normalized/placeholders = %d/%d/%d, want 3/3/2",
			rowCount,
			normalizedCount,
			placeholderCount,
		)
	}

	var nullable string
	if err := db.Raw(`
		SELECT is_nullable
		FROM information_schema.columns
		WHERE table_schema = current_schema()
			AND table_name = 'customers'
			AND column_name = 'email'
	`).Scan(&nullable).Error; err != nil {
		t.Fatalf("inspect customer email nullability: %v", err)
	}
	if nullable != "NO" {
		t.Fatalf("customers.email is_nullable = %q, want NO", nullable)
	}
	if !db.Migrator().HasTable(&models.PasswordResetToken{}) {
		t.Fatal("password_reset_tokens table was not created")
	}
}
