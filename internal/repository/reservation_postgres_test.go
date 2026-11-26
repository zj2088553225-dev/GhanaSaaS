package repository

import (
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"saas/internal/models"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestReservationPostgresConflictTargetsAndTransactions(t *testing.T) {
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
	schema := "reservation_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("database handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(20)
	if err := db.Exec("CREATE TABLE spus (id bigint PRIMARY KEY)").Error; err != nil {
		t.Fatalf("create linked-product target: %v", err)
	}
	if err := db.AutoMigrate(
		&models.ReservationProduct{}, &models.ReservationInterest{},
		&models.ProductReservation{}, &models.ReservationAttemptEvent{},
	); err != nil {
		t.Fatalf("migrate isolated reservation schema: %v", err)
	}
	for model, constraints := range map[any][]string{
		&models.ReservationInterest{}: {"uidx_reservation_interests_product_visitor", "uidx_reservation_interests_idempotency"},
		&models.ProductReservation{}: {
			"uidx_product_reservations_product_contact",
			"uidx_product_reservations_challenge",
			"uidx_product_reservations_active_identity",
			"uidx_product_reservations_code",
			"chk_product_reservations_state",
		},
	} {
		for _, constraint := range constraints {
			if !db.Migrator().HasConstraint(model, constraint) && !db.Migrator().HasIndex(model, constraint) {
				t.Fatalf("PostgreSQL migration omitted %s", constraint)
			}
		}
	}

	product := &models.ReservationProduct{
		Slug: "postgres-conflict-product", TitleEN: "PostgreSQL product", GalleryJSON: []byte(`[]`),
		Status: models.ReservationProductStatusOpen, CreatedBy: 1,
	}
	if err := db.Create(product).Error; err != nil {
		t.Fatalf("create reservation product: %v", err)
	}
	if err := db.Create(&models.ReservationInterest{
		ReservationProductID: product.ID + 9999, VisitorID: uuid.NewString(), IdempotencyKey: uuid.NewString(),
		SourceType: models.ReservationSourceDetail, CreatedAt: time.Now().UTC(),
	}).Error; err == nil {
		t.Fatal("PostgreSQL accepted interest with missing reservation product")
	}
	missingProductReservation := postgresPendingReservation(product.ID+9999, strings.Repeat("8", 64), uuid.NewString(), uuid.NewString(), time.Now().UTC())
	if err := db.Create(missingProductReservation).Error; err == nil {
		t.Fatal("PostgreSQL accepted reservation with missing reservation product")
	}
	repo := NewReservationWorkflowRepository(db)
	productRepo := NewReservationProductRepository(db)
	now := time.Now().UTC()
	contactHash := strings.Repeat("a", 64)
	const workers = 20
	startInterests := make(chan struct{})
	var interestCreated atomic.Int64
	interestErrors := make(chan error, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-startInterests
			created, createErr := productRepo.CreateInterest(t.Context(), &models.ReservationInterest{
				ReservationProductID: product.ID, VisitorID: "11111111-1111-4111-8111-111111111111",
				IdempotencyKey: uuid.NewString(), SourceType: models.ReservationSourceDetail, CreatedAt: now,
			}, nil)
			if createErr != nil {
				interestErrors <- createErr
			} else if created {
				interestCreated.Add(1)
			}
		}()
	}
	close(startInterests)
	wait.Wait()
	close(interestErrors)
	for concurrencyErr := range interestErrors {
		t.Fatalf("concurrent interest: %v", concurrencyErr)
	}
	if interestCreated.Load() != 1 {
		t.Fatalf("concurrent interest creates = %d, want 1", interestCreated.Load())
	}

	anonymousProduct := &models.ReservationProduct{
		Slug: "postgres-anonymous-conflict", TitleEN: "PostgreSQL anonymous product", GalleryJSON: []byte(`[]`),
		Status: models.ReservationProductStatusOpen, CreatedBy: 1,
	}
	if err := db.Create(anonymousProduct).Error; err != nil {
		t.Fatalf("create anonymous reservation product: %v", err)
	}
	anonymousRepo := NewAnonymousReservationRepository(db)
	activeKey := strings.Repeat("d", 64)
	browserHash := strings.Repeat("e", 64)
	ipHash := strings.Repeat("f", 64)
	startAnonymous := make(chan struct{})
	anonymousErrors := make(chan error, workers)
	var anonymousCreated atomic.Int64
	var anonymousDuplicates atomic.Int64
	wait = sync.WaitGroup{}
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-startAnonymous
			_, duplicate, createErr := anonymousRepo.Create(
				t.Context(),
				postgresAnonymousReservation(anonymousProduct.ID, activeKey, browserHash, ipHash, now),
				nil,
				AnonymousReservationLimits{
					BrowserPerHour: 100, BrowserPerDay: 100, IPPerHour: 100, IPPerDay: 100, IPProductPerDay: 100,
				},
				now,
			)
			if createErr != nil {
				anonymousErrors <- createErr
			} else if duplicate {
				anonymousDuplicates.Add(1)
			} else {
				anonymousCreated.Add(1)
			}
		}()
	}
	close(startAnonymous)
	wait.Wait()
	close(anonymousErrors)
	for concurrencyErr := range anonymousErrors {
		t.Fatalf("concurrent anonymous create: %v", concurrencyErr)
	}
	if anonymousCreated.Load() != 1 || anonymousDuplicates.Load() != workers-1 {
		t.Fatalf("concurrent anonymous creates = %d, duplicates = %d", anonymousCreated.Load(), anonymousDuplicates.Load())
	}
	var anonymousCount int64
	if err := db.Model(&models.ProductReservation{}).
		Where("active_identity_key = ?", activeKey).Count(&anonymousCount).Error; err != nil || anonymousCount != 1 {
		t.Fatalf("active anonymous reservation count = %d/%v", anonymousCount, err)
	}

	startRequests := make(chan struct{})
	requestErrors := make(chan error, workers)
	wait = sync.WaitGroup{}
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-startRequests
			_, upsertErr := repo.UpsertPending(t.Context(), postgresPendingReservation(product.ID, contactHash, uuid.NewString(), uuid.NewString(), now), nil)
			if upsertErr != nil {
				requestErrors <- upsertErr
			}
		}()
	}
	close(startRequests)
	wait.Wait()
	close(requestErrors)
	for concurrencyErr := range requestErrors {
		t.Fatalf("concurrent pending upsert: %v", concurrencyErr)
	}
	var pending models.ProductReservation
	if err := db.Where("reservation_product_id = ? AND contact_hash = ?", product.ID, contactHash).Take(&pending).Error; err != nil {
		t.Fatalf("load concurrent pending reservation: %v", err)
	}

	startVerify := make(chan struct{})
	verifyErrors := make(chan error, workers)
	var verifyWinners atomic.Int64
	wait = sync.WaitGroup{}
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-startVerify
			result, finalizeErr := repo.FinalizeVerification(
				t.Context(), product.ID, pending.VerificationChallengeID, uuid.NewString(), strings.Repeat(string(rune('a'+index%6)), 64), now.Add(2*time.Minute), nil,
			)
			if finalizeErr != nil {
				verifyErrors <- finalizeErr
			} else if result.Won {
				verifyWinners.Add(1)
			}
		}(index)
	}
	close(startVerify)
	wait.Wait()
	close(verifyErrors)
	for concurrencyErr := range verifyErrors {
		t.Fatalf("concurrent finalize: %v", concurrencyErr)
	}
	if verifyWinners.Load() != 1 {
		t.Fatalf("concurrent verification winners = %d, want 1", verifyWinners.Load())
	}
	var verified models.ProductReservation
	if err := db.Where("id = ?", pending.ID).Take(&verified).Error; err != nil || verified.ManageTokenHash == nil {
		t.Fatalf("load verified reservation = %+v/%v", verified, err)
	}
	manageHash := *verified.ManageTokenHash
	startCancelRace := make(chan struct{})
	var cancelled bool
	var cancelErr, duplicateFinalizeErr error
	wait = sync.WaitGroup{}
	wait.Add(2)
	go func() {
		defer wait.Done()
		<-startCancelRace
		_, duplicateFinalizeErr = repo.FinalizeVerification(
			t.Context(), product.ID, pending.VerificationChallengeID, uuid.NewString(), strings.Repeat("7", 64), now.Add(4*time.Minute), nil,
		)
	}()
	go func() {
		defer wait.Done()
		<-startCancelRace
		cancelled, cancelErr = repo.Cancel(t.Context(), product.ID, manageHash, uuid.NewString(), now.Add(4*time.Minute), nil)
	}()
	close(startCancelRace)
	wait.Wait()
	if cancelErr != nil || !cancelled || (duplicateFinalizeErr != nil && !errors.Is(duplicateFinalizeErr, gorm.ErrRecordNotFound)) {
		t.Fatalf("verify/cancel race = cancelled:%v cancelErr:%v finalizeErr:%v", cancelled, cancelErr, duplicateFinalizeErr)
	}
	var racedState models.ProductReservation
	if err := db.Where("id = ?", pending.ID).Take(&racedState).Error; err != nil || racedState.Status != models.ProductReservationCancelled || racedState.VerificationStatus != models.ReservationVerificationCancelled || racedState.CancelledAt == nil {
		t.Fatalf("verify/cancel race state = %+v/%v", racedState, err)
	}
	reactivated, err := repo.UpsertPending(t.Context(), postgresPendingReservation(product.ID, contactHash, uuid.NewString(), uuid.NewString(), now.Add(5*time.Minute)), nil)
	if err != nil || reactivated.ID != pending.ID || reactivated.VerifiedAt != nil || reactivated.CancelledAt != nil || reactivated.ManageTokenHash != nil || reactivated.Status != models.ProductReservationActive || reactivated.VerificationStatus != models.ReservationVerificationPending {
		t.Fatalf("reactivated pending row = %+v/%v", reactivated, err)
	}
	if _, err := repo.FinalizeVerification(t.Context(), product.ID, reactivated.VerificationChallengeID, uuid.NewString(), strings.Repeat("f", 64), now.Add(6*time.Minute), nil); err != nil {
		t.Fatalf("finalize reactivation: %v", err)
	}
	var count int64
	if err := db.Model(&models.ProductReservation{}).Where("reservation_product_id = ?", product.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("final reservation row count = %d/%v", count, err)
	}
	malicious := postgresPendingReservation(product.ID, strings.Repeat("9", 64), uuid.NewString(), uuid.NewString(), now)
	malicious.VerificationStatus = models.ReservationVerificationVerified
	if err := db.Create(malicious).Error; err == nil {
		t.Fatal("PostgreSQL accepted verified state without verified_at/manage token")
	}

	raceProduct := &models.ReservationProduct{
		Slug: "postgres-archive-race", TitleEN: "Archive race", GalleryJSON: []byte(`[]`),
		Status: models.ReservationProductStatusOpen, CreatedBy: 1,
	}
	if err := db.Create(raceProduct).Error; err != nil {
		t.Fatalf("create archive-race product: %v", err)
	}
	racePending, err := repo.UpsertPending(t.Context(), postgresPendingReservation(raceProduct.ID, strings.Repeat("6", 64), uuid.NewString(), uuid.NewString(), now), nil)
	if err != nil {
		t.Fatalf("create archive-race pending: %v", err)
	}
	startArchiveRace := make(chan struct{})
	var archiveErr, archiveFinalizeErr error
	wait = sync.WaitGroup{}
	wait.Add(2)
	go func() {
		defer wait.Done()
		<-startArchiveRace
		archiveErr = productRepo.Archive(t.Context(), raceProduct.ID)
	}()
	go func() {
		defer wait.Done()
		<-startArchiveRace
		_, archiveFinalizeErr = repo.FinalizeVerification(t.Context(), raceProduct.ID, racePending.VerificationChallengeID, uuid.NewString(), strings.Repeat("5", 64), now.Add(time.Minute), nil)
	}()
	close(startArchiveRace)
	wait.Wait()
	if archiveErr != nil || (archiveFinalizeErr != nil && !errors.Is(archiveFinalizeErr, ErrReservationProductNotOpen)) {
		t.Fatalf("archive/finalize race errors = archive:%v finalize:%v", archiveErr, archiveFinalizeErr)
	}
	var raceState models.ProductReservation
	if err := db.Where("id = ?", racePending.ID).Take(&raceState).Error; err != nil {
		t.Fatalf("load archive-race reservation: %v", err)
	}
	if archiveFinalizeErr == nil && raceState.VerificationStatus != models.ReservationVerificationVerified {
		t.Fatalf("winning finalize did not persist verified state: %+v", raceState)
	}
	if errors.Is(archiveFinalizeErr, ErrReservationProductNotOpen) && raceState.VerificationStatus != models.ReservationVerificationPending {
		t.Fatalf("winning archive allowed verification mutation: %+v", raceState)
	}
}

func postgresPendingReservation(productID uint64, contactHash, challengeID, requestKey string, now time.Time) *models.ProductReservation {
	return &models.ProductReservation{
		ReservationProductID: productID, VisitorID: uuid.NewString(), ContactType: models.ReservationContactSMS,
		ContactHash: contactHash, ContactEncrypted: []byte("ciphertext"), ContactEncryptionVersion: 1,
		PhoneCountryCode: "+233", PhoneLast4: "4567", VerificationStatus: models.ReservationVerificationPending,
		Status: models.ProductReservationActive, Provider: "mock", VerificationChallengeID: challengeID,
		OTPRequestIdempotencyKey: requestKey, VerificationRequestedAt: now,
		SourceType: models.ReservationSourceDetail, CreatedAt: now, UpdatedAt: now,
	}
}

func postgresAnonymousReservation(productID uint64, activeKey, browserHash, ipHash string, now time.Time) *models.ProductReservation {
	expiresAt := now.Add(24 * time.Hour)
	return &models.ProductReservation{
		IdentityType: models.ReservationIdentityAnonymous, ReservationProductID: productID,
		BrowserHash: &browserHash, IPHash: &ipHash, ActiveIdentityKey: &activeKey,
		VerificationStatus: models.ReservationVerificationNotRequired,
		Status:             models.ProductReservationPending,
		Quantity:           1, VerificationRequestedAt: now, ExpiresAt: &expiresAt,
		SourceType: models.ReservationSourceDirect, CreatedAt: now, UpdatedAt: now,
	}
}
