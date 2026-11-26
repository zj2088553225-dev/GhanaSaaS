package database

import (
	"strings"

	"saas/internal/models"

	"gorm.io/gorm"
)

// prepareAnonymousReservationSchema upgrades the existing OTP-only table in one
// PostgreSQL transaction. Existing rows are preserved and classified as
// phone_verified. AutoMigrate runs immediately afterwards to install the new
// checks and indexes from the model definition.
func prepareAnonymousReservationSchema(db *gorm.DB) error {
	if db.Dialector.Name() != "postgres" || !db.Migrator().HasTable("product_reservations") {
		return nil
	}
	if db.Migrator().HasColumn(&models.ProductReservation{}, "identity_type") {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		statements := []string{
			"ALTER TABLE product_reservations ADD COLUMN identity_type varchar(20)",
			"ALTER TABLE product_reservations ADD COLUMN reservation_code varchar(48)",
			"ALTER TABLE product_reservations ADD COLUMN browser_hash char(64)",
			"ALTER TABLE product_reservations ADD COLUMN ip_hash char(64)",
			"ALTER TABLE product_reservations ADD COLUMN active_identity_key char(64)",
			"ALTER TABLE product_reservations ADD COLUMN quantity integer DEFAULT 1",
			"ALTER TABLE product_reservations ADD COLUMN note varchar(500)",
			"ALTER TABLE product_reservations ADD COLUMN expires_at timestamptz",
			"ALTER TABLE product_reservations ADD COLUMN risk_flagged boolean DEFAULT false",
			"ALTER TABLE product_reservations ADD COLUMN risk_flagged_at timestamptz",
			"UPDATE product_reservations SET identity_type = 'phone_verified', quantity = COALESCE(quantity, 1), risk_flagged = COALESCE(risk_flagged, false)",
			"ALTER TABLE product_reservations ALTER COLUMN visitor_id DROP NOT NULL",
			"ALTER TABLE product_reservations ALTER COLUMN contact_type DROP NOT NULL",
			"ALTER TABLE product_reservations ALTER COLUMN contact_hash DROP NOT NULL",
			"ALTER TABLE product_reservations ALTER COLUMN contact_encrypted DROP NOT NULL",
			"ALTER TABLE product_reservations ALTER COLUMN contact_encryption_version DROP NOT NULL",
			"ALTER TABLE product_reservations ALTER COLUMN phone_country_code DROP NOT NULL",
			"ALTER TABLE product_reservations ALTER COLUMN phone_last4 DROP NOT NULL",
			"ALTER TABLE product_reservations ALTER COLUMN provider DROP NOT NULL",
			"ALTER TABLE product_reservations ALTER COLUMN verification_challenge_id DROP NOT NULL",
			"ALTER TABLE product_reservations ALTER COLUMN otp_request_idempotency_key DROP NOT NULL",
			"ALTER TABLE product_reservations DROP CONSTRAINT IF EXISTS chk_product_reservations_contact_type",
			"ALTER TABLE product_reservations DROP CONSTRAINT IF EXISTS chk_product_reservations_encryption_version",
			"ALTER TABLE product_reservations DROP CONSTRAINT IF EXISTS chk_product_reservations_verification_status",
			"ALTER TABLE product_reservations DROP CONSTRAINT IF EXISTS chk_product_reservations_status",
			"ALTER TABLE product_reservations DROP CONSTRAINT IF EXISTS chk_product_reservations_state",
		}
		for _, statement := range statements {
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		if tx.Migrator().HasTable("reservation_attempt_events") {
			for _, statement := range []string{
				"ALTER TABLE reservation_attempt_events DROP CONSTRAINT IF EXISTS chk_reservation_attempt_events_action",
				"ALTER TABLE reservation_attempt_events DROP CONSTRAINT IF EXISTS chk_reservation_attempt_events_reason",
			} {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
		}

		var rows []struct {
			ID              uint64
			ReservationCode string
		}
		if err := tx.Table("product_reservations").Select("id, reservation_code").Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			if strings.TrimSpace(row.ReservationCode) != "" {
				continue
			}
			code, err := models.NewReservationCode()
			if err != nil {
				return err
			}
			if err := tx.Table("product_reservations").Where("id = ?", row.ID).Update("reservation_code", code).Error; err != nil {
				return err
			}
		}
		for _, statement := range []string{
			"ALTER TABLE product_reservations ALTER COLUMN identity_type SET DEFAULT 'phone_verified'",
			"ALTER TABLE product_reservations ALTER COLUMN identity_type SET NOT NULL",
			"ALTER TABLE product_reservations ALTER COLUMN reservation_code SET NOT NULL",
			"ALTER TABLE product_reservations ALTER COLUMN quantity SET NOT NULL",
			"ALTER TABLE product_reservations ALTER COLUMN risk_flagged SET NOT NULL",
		} {
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
