package database

import (
	"os"
	"strings"
	"testing"
	"time"

	"saas/internal/models"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPrepareAnonymousReservationSchemaPreservesLegacyOTPData(t *testing.T) {
	if os.Getenv("RESERVATION_TEST_ALLOW_ISOLATED") != "1" {
		t.Skip("set RESERVATION_TEST_ALLOW_ISOLATED=1 with an isolated reservation_test PostgreSQL database")
	}
	dsn := strings.TrimSpace(os.Getenv("RESERVATION_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Fatal("RESERVATION_TEST_POSTGRES_DSN is required")
	}
	adminDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open isolated PostgreSQL: %v", err)
	}
	var databaseName string
	if err := adminDB.Raw("SELECT current_database()").Scan(&databaseName).Error; err != nil {
		t.Fatalf("read current database: %v", err)
	}
	if databaseName != "reservation_test" {
		t.Fatalf("refusing non-isolated database %q", databaseName)
	}
	schema := "reservation_migration_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := `"` + schema + `"`
	if err := adminDB.Exec("CREATE SCHEMA " + quotedSchema).Error; err != nil {
		t.Fatalf("create isolated schema: %v", err)
	}
	t.Cleanup(func() {
		_ = adminDB.Exec("DROP SCHEMA " + quotedSchema + " CASCADE").Error
	})
	db, err := gorm.Open(postgres.Open(dsn+" search_path="+schema), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open schema-scoped PostgreSQL: %v", err)
	}
	if err := db.AutoMigrate(&models.SPU{}, &models.ReservationProduct{}); err != nil {
		t.Fatalf("create reservation product target: %v", err)
	}
	product := &models.ReservationProduct{
		Slug: "legacy-migration-product", TitleEN: "Legacy product",
		GalleryJSON: []byte(`[]`), Status: models.ReservationProductStatusOpen, CreatedBy: 1,
	}
	if err := db.Create(product).Error; err != nil {
		t.Fatalf("insert reservation product target: %v", err)
	}
	legacySchema := `
CREATE TABLE product_reservations (
	id bigserial PRIMARY KEY,
	reservation_product_id bigint NOT NULL,
	visitor_id char(36) NOT NULL,
	contact_type varchar(16) NOT NULL,
	contact_hash char(64) NOT NULL,
	contact_encrypted bytea NOT NULL,
	contact_encryption_version integer NOT NULL,
	phone_country_code varchar(8) NOT NULL,
	phone_last4 char(4) NOT NULL,
	verification_status varchar(16) NOT NULL,
	status varchar(16) NOT NULL,
	provider varchar(32) NOT NULL,
	provider_reference varchar(128),
	verification_challenge_id varchar(64) NOT NULL,
	otp_request_idempotency_key char(36) NOT NULL,
	otp_verify_idempotency_key char(36),
	cancel_idempotency_key char(36),
	verification_requested_at timestamptz NOT NULL,
	verified_at timestamptz,
	cancelled_at timestamptz,
	turnstile_verified_at timestamptz,
	verification_attempt_count integer NOT NULL DEFAULT 0,
	manage_token_hash char(64),
	source_type varchar(32) NOT NULL,
	source_id varchar(128),
	created_at timestamptz NOT NULL,
	updated_at timestamptz NOT NULL,
	CONSTRAINT chk_product_reservations_contact_type CHECK (contact_type IN ('sms','whatsapp')),
	CONSTRAINT chk_product_reservations_encryption_version CHECK (contact_encryption_version > 0),
	CONSTRAINT chk_product_reservations_verification_status CHECK (verification_status IN ('pending','verified','expired','failed','cancelled')),
	CONSTRAINT chk_product_reservations_status CHECK (status IN ('active','cancelled')),
	CONSTRAINT chk_product_reservations_state CHECK (
		(verification_status = 'verified' AND status = 'active' AND verified_at IS NOT NULL AND cancelled_at IS NULL AND manage_token_hash IS NOT NULL)
		OR (verification_status = 'cancelled' AND status = 'cancelled' AND verified_at IS NOT NULL AND cancelled_at IS NOT NULL AND manage_token_hash IS NOT NULL)
		OR (verification_status IN ('pending','expired','failed') AND status = 'active' AND verified_at IS NULL AND cancelled_at IS NULL AND manage_token_hash IS NULL)
	)
)`
	if err := db.Exec(legacySchema).Error; err != nil {
		t.Fatalf("create legacy reservation schema: %v", err)
	}
	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	if err := db.Exec(`
INSERT INTO product_reservations (
	reservation_product_id, visitor_id, contact_type, contact_hash, contact_encrypted,
	contact_encryption_version, phone_country_code, phone_last4, verification_status,
	status, provider, verification_challenge_id, otp_request_idempotency_key,
	verification_requested_at, verification_attempt_count, source_type, created_at, updated_at
) VALUES (?, ?, 'sms', ?, ?, 2, '+233', '4567', 'pending', 'active', 'twilio_verify', ?, ?, ?, 0, 'reservation_dialog', ?, ?)`,
		product.ID,
		"11111111-1111-4111-8111-111111111111",
		strings.Repeat("a", 64),
		[]byte("legacy-ciphertext"),
		"legacy-challenge",
		"22222222-2222-4222-8222-222222222222",
		now, now, now,
	).Error; err != nil {
		t.Fatalf("insert legacy reservation: %v", err)
	}

	if err := prepareAnonymousReservationSchema(db); err != nil {
		t.Fatalf("prepare anonymous schema: %v", err)
	}
	if err := db.AutoMigrate(&models.ProductReservation{}, &models.ReservationAttemptEvent{}); err != nil {
		t.Fatalf("AutoMigrate upgraded reservation schema: %v", err)
	}
	var upgraded models.ProductReservation
	if err := db.First(&upgraded).Error; err != nil {
		t.Fatalf("load upgraded reservation: %v", err)
	}
	if upgraded.IdentityType != models.ReservationIdentityPhoneVerified ||
		upgraded.ReservationCode == "" || upgraded.Quantity != 1 || upgraded.RiskFlagged {
		t.Fatalf("upgraded legacy reservation = %+v", upgraded)
	}
	for _, column := range []string{
		"visitor_id", "contact_type", "contact_hash", "contact_encrypted",
		"contact_encryption_version", "phone_country_code", "phone_last4",
		"provider", "verification_challenge_id", "otp_request_idempotency_key",
	} {
		var nullable string
		if err := db.Raw(`
SELECT is_nullable
FROM information_schema.columns
WHERE table_schema = current_schema() AND table_name = 'product_reservations' AND column_name = ?`,
			column,
		).Scan(&nullable).Error; err != nil || nullable != "YES" {
			t.Fatalf("column %s nullable = %q/%v", column, nullable, err)
		}
	}
	if !db.Migrator().HasIndex(&models.ProductReservation{}, "uidx_product_reservations_active_identity") ||
		!db.Migrator().HasConstraint(&models.ProductReservation{}, "chk_product_reservations_state") {
		t.Fatal("upgraded schema omitted anonymous active uniqueness or state constraint")
	}
}
