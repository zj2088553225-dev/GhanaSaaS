package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestReservationDatabaseConstraintsAndOpenWindow(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open(fmt.Sprintf("file:model-reservation-%s?mode=memory&cache=shared", t.Name())),
		&gorm.Config{DisableForeignKeyConstraintWhenMigrating: true, Logger: logger.Default.LogMode(logger.Silent)},
	)
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	if err := db.AutoMigrate(&SPU{}, &ReservationProduct{}, &ReservationInterest{}, &ProductReservation{}, &ReservationAttemptEvent{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	start := time.Date(2026, time.July, 17, 0, 0, 0, 0, time.FixedZone("test", 8*60*60))
	end := start.Add(48 * time.Hour)
	product := &ReservationProduct{
		Slug: "future-phone", TitleEN: "Future Phone", GalleryJSON: datatypes.JSON(`[]`),
		Status: ReservationProductStatusOpen, ReservationStartAt: &start, ReservationEndAt: &end, CreatedBy: 1,
	}
	if err := db.Create(product).Error; err != nil {
		t.Fatalf("create reservation product: %v", err)
	}
	if product.ReservationStartAt == nil || product.ReservationStartAt.Location() != time.UTC {
		t.Fatalf("start time was not normalized to UTC: %v", product.ReservationStartAt)
	}
	if product.IsOpenAt(start.Add(-time.Minute)) || !product.IsOpenAt(start.Add(time.Hour)) || product.IsOpenAt(end) {
		t.Fatalf("unexpected open-window behavior for %+v", product)
	}

	duplicate := *product
	duplicate.ID = 0
	if err := db.Create(&duplicate).Error; err == nil {
		t.Fatal("database accepted duplicate reservation product slug")
	}
	invalidStatus := &ReservationProduct{Slug: "invalid-status", TitleEN: "Invalid", GalleryJSON: datatypes.JSON(`[]`), Status: "selling", CreatedBy: 1}
	if err := db.Create(invalidStatus).Error; err == nil {
		t.Fatal("database accepted invalid reservation product status")
	}
	invalidOverride := ReservationHeatLevel("viral")
	invalidHeat := &ReservationProduct{Slug: "invalid-heat", TitleEN: "Invalid", GalleryJSON: datatypes.JSON(`[]`), Status: ReservationProductStatusDraft, HeatLevelOverride: &invalidOverride, CreatedBy: 1}
	if err := db.Create(invalidHeat).Error; err == nil {
		t.Fatal("database accepted invalid heat override")
	}

	interest := &ReservationInterest{
		ReservationProductID: product.ID, VisitorID: "11111111-1111-4111-8111-111111111111",
		IdempotencyKey: "22222222-2222-4222-8222-222222222222", SourceType: ReservationSourceList,
	}
	if err := db.Create(interest).Error; err != nil {
		t.Fatalf("create interest: %v", err)
	}
	duplicateVisitor := *interest
	duplicateVisitor.ID = 0
	duplicateVisitor.IdempotencyKey = "33333333-3333-4333-8333-333333333333"
	if err := db.Create(&duplicateVisitor).Error; err == nil {
		t.Fatal("database accepted duplicate product + visitor interest")
	}
	duplicateKey := *interest
	duplicateKey.ID = 0
	duplicateKey.VisitorID = "44444444-4444-4444-8444-444444444444"
	if err := db.Create(&duplicateKey).Error; err == nil {
		t.Fatal("database accepted duplicate interest idempotency key")
	}
	invalidSource := &ReservationInterest{
		ReservationProductID: product.ID, VisitorID: "55555555-5555-4555-8555-555555555555",
		IdempotencyKey: "66666666-6666-4666-8666-666666666666", SourceType: "product_page",
	}
	if err := db.Create(invalidSource).Error; err == nil {
		t.Fatal("database accepted invalid reservation interest source")
	}

	reservation := &ProductReservation{
		ReservationProductID: product.ID, VisitorID: "77777777-7777-4777-8777-777777777777",
		ContactType: ReservationContactSMS, ContactHash: strings.Repeat("a", 64), ContactEncrypted: []byte("ciphertext"),
		ContactEncryptionVersion: 1, PhoneCountryCode: "+233", PhoneLast4: "4567",
		VerificationStatus: ReservationVerificationPending, Status: ProductReservationActive,
		Provider: "mock", VerificationChallengeID: "88888888-8888-4888-8888-888888888888",
		OTPRequestIdempotencyKey: "99999999-9999-4999-8999-999999999999",
		VerificationRequestedAt:  time.Now().UTC(), SourceType: ReservationSourceDetail,
	}
	if err := db.Create(reservation).Error; err != nil {
		t.Fatalf("create product reservation: %v", err)
	}
	invalidVerifiedState := *reservation
	invalidVerifiedState.ID = 0
	invalidVerifiedState.ContactHash = strings.Repeat("d", 64)
	invalidVerifiedState.VerificationChallengeID = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	invalidVerifiedState.OTPRequestIdempotencyKey = "ffffffff-ffff-4fff-8fff-ffffffffffff"
	invalidVerifiedState.VerificationStatus = ReservationVerificationVerified
	if err := db.Create(&invalidVerifiedState).Error; err == nil {
		t.Fatal("database accepted verified state without timestamp/manage token")
	}
	invalidPendingState := *reservation
	invalidPendingState.ID = 0
	invalidPendingState.ContactHash = strings.Repeat("e", 64)
	invalidPendingState.VerificationChallengeID = "12121212-1212-4212-8212-121212121212"
	invalidPendingState.OTPRequestIdempotencyKey = "13131313-1313-4313-8313-131313131313"
	invalidPendingState.Status = ProductReservationCancelled
	if err := db.Create(&invalidPendingState).Error; err == nil {
		t.Fatal("database accepted pending+cancelled state")
	}
	duplicateContact := *reservation
	duplicateContact.ID = 0
	duplicateContact.VerificationChallengeID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	duplicateContact.OTPRequestIdempotencyKey = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	if err := db.Create(&duplicateContact).Error; err == nil {
		t.Fatal("database accepted duplicate product + contact hash")
	}
	invalidContact := *reservation
	invalidContact.ID = 0
	invalidContact.ContactHash = strings.Repeat("b", 64)
	invalidContact.ContactType = "email"
	invalidContact.VerificationChallengeID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	invalidContact.OTPRequestIdempotencyKey = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	if err := db.Create(&invalidContact).Error; err == nil {
		t.Fatal("database accepted invalid reservation contact type")
	}
	encoded, err := json.Marshal(reservation)
	if err != nil {
		t.Fatalf("marshal reservation: %v", err)
	}
	for _, forbidden := range []string{"contact_hash", "contact_encrypted", "phone_country_code", "phone_last4", "verification_status", "verification_challenge_id", "otp_request_idempotency_key", "turnstile_verified_at", "ciphertext"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("reservation JSON exposed %q: %s", forbidden, encoded)
		}
	}

	productID := product.ID
	reason := ReservationReasonRateLimited
	attempt := &ReservationAttemptEvent{
		ReservationProductID: &productID, IPRiskKey: stringPointerModel(strings.Repeat("c", 64)),
		Action: ReservationAttemptOTPRequest, Result: ReservationAttemptBlocked, ReasonCode: &reason,
	}
	if err := db.Create(attempt).Error; err != nil {
		t.Fatalf("create attempt event: %v", err)
	}
	invalidAttempt := *attempt
	invalidAttempt.ID = 0
	invalidAttempt.Action = "login"
	if err := db.Create(&invalidAttempt).Error; err == nil {
		t.Fatal("database accepted invalid attempt action")
	}
}

func stringPointerModel(value string) *string { return &value }
