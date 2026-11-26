package service

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"saas/internal/models"
	"saas/internal/repository"

	"gorm.io/gorm"
)

func TestAnonymousReservationCreateDuplicateExpireAndBrowserOwnership(t *testing.T) {
	db := newReservationServiceTestDB(t)
	now := reservationTestNow
	service := newAnonymousReservationServiceForTest(t, db, func() time.Time { return now }, defaultAnonymousTestLimits())
	product := createOpenReservationProduct(t, newReservationTestService(db), "anonymous-create")
	browserHash := strings.Repeat("a", 64)
	ipHash := strings.Repeat("b", 64)

	created, err := service.Create(t.Context(), anonymousCreateInput(product.ID, browserHash, ipHash, "test-turnstile-create"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ReservationCode == "" || created.Status != models.ProductReservationPending ||
		created.IdentityType != models.ReservationIdentityAnonymous || created.ExpiresAt == nil ||
		!created.ExpiresAt.Equal(now.Add(24*time.Hour)) {
		t.Fatalf("created reservation = %+v", created)
	}

	var stored models.ProductReservation
	if err := db.Where("reservation_code = ?", created.ReservationCode).Take(&stored).Error; err != nil {
		t.Fatalf("load stored reservation: %v", err)
	}
	if stored.BrowserHash == nil || *stored.BrowserHash != browserHash || stored.IPHash == nil || *stored.IPHash != ipHash {
		t.Fatalf("stored hashes = browser:%v ip:%v", stored.BrowserHash, stored.IPHash)
	}
	columns, err := db.Migrator().ColumnTypes(&models.ProductReservation{})
	if err != nil {
		t.Fatalf("ColumnTypes: %v", err)
	}
	for _, column := range columns {
		name := strings.ToLower(column.Name())
		if strings.Contains(name, "cookie") || strings.Contains(name, "raw_ip") || name == "ip_address" {
			t.Fatalf("anonymous reservation schema persists forbidden field %q", name)
		}
	}

	_, err = service.Create(t.Context(), anonymousCreateInput(product.ID, browserHash, ipHash, "test-turnstile-duplicate"))
	var duplicate *AnonymousReservationDuplicateError
	if !errors.As(err, &duplicate) || duplicate.Existing.ReservationCode != created.ReservationCode {
		t.Fatalf("duplicate error = %#v", err)
	}

	mine, err := service.Mine(t.Context(), browserHash, "en")
	if err != nil || len(mine) != 1 || mine[0].ReservationCode != created.ReservationCode {
		t.Fatalf("Mine = %+v/%v", mine, err)
	}
	otherMine, err := service.Mine(t.Context(), strings.Repeat("c", 64), "en")
	if err != nil || len(otherMine) != 0 {
		t.Fatalf("other browser Mine = %+v/%v", otherMine, err)
	}
	if _, err := service.Cancel(t.Context(), created.ReservationCode, strings.Repeat("c", 64), "en"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other browser Cancel error = %v", err)
	}

	now = now.Add(25 * time.Hour)
	recreated, err := service.Create(t.Context(), anonymousCreateInput(product.ID, browserHash, ipHash, "test-turnstile-after-expiry"))
	if err != nil || recreated.ReservationCode == created.ReservationCode {
		t.Fatalf("create after expiry = %+v/%v", recreated, err)
	}
	if err := db.Where("reservation_code = ?", created.ReservationCode).Take(&stored).Error; err != nil ||
		stored.Status != models.ProductReservationExpired || stored.ActiveIdentityKey != nil {
		t.Fatalf("expired stored reservation = %+v/%v", stored, err)
	}
	changed, err := service.ExpireDue(t.Context(), 100)
	if err != nil || changed != 0 {
		t.Fatalf("idempotent expiry = %d/%v", changed, err)
	}

	adminPage, err := service.AdminReservations(t.Context(), AnonymousReservationAdminQuery{Page: 1, PageSize: 20})
	if err != nil || adminPage.Total != 2 {
		t.Fatalf("AdminReservations = %+v/%v", adminPage, err)
	}
	var adminItem AnonymousReservationAdminDTO
	for _, item := range adminPage.Items {
		if item.ReservationCode == recreated.ReservationCode {
			adminItem = item
		}
	}
	if len(adminItem.BrowserHashPrefix) != 12 || len(adminItem.IPHashPrefix) != 12 ||
		adminItem.BrowserHashPrefix == browserHash || adminItem.IPHashPrefix == ipHash ||
		adminItem.BrowserReservationCount != 2 || adminItem.RecentIPReservationCount != 1 {
		t.Fatalf("safe admin identity summary = %+v", adminItem)
	}
	flagged, err := service.AdminSetRisk(t.Context(), recreated.ReservationCode, true)
	if err != nil || !flagged.RiskFlagged || flagged.RiskFlaggedAt == nil {
		t.Fatalf("AdminSetRisk = %+v/%v", flagged, err)
	}
	confirmed, err := service.AdminSetStatus(t.Context(), recreated.ReservationCode, "confirmed")
	if err != nil || confirmed.Status != models.ProductReservationConfirmed {
		t.Fatalf("AdminSetStatus confirmed = %+v/%v", confirmed, err)
	}
	rejected, err := service.AdminSetStatus(t.Context(), recreated.ReservationCode, "rejected")
	if err != nil || rejected.Status != models.ProductReservationRejected {
		t.Fatalf("AdminSetStatus rejected = %+v/%v", rejected, err)
	}
}

func TestAnonymousReservationTurnstileFailureDoesNotCreate(t *testing.T) {
	db := newReservationServiceTestDB(t)
	service := newAnonymousReservationServiceForTest(t, db, func() time.Time { return reservationTestNow }, defaultAnonymousTestLimits())
	product := createOpenReservationProduct(t, newReservationTestService(db), "anonymous-turnstile")

	_, err := service.Create(t.Context(), anonymousCreateInput(
		product.ID, strings.Repeat("a", 64), strings.Repeat("b", 64), "invalid-token",
	))
	if !errors.Is(err, ErrTurnstileRejected) {
		t.Fatalf("Turnstile failure error = %v", err)
	}
	var reservations int64
	if err := db.Model(&models.ProductReservation{}).Count(&reservations).Error; err != nil || reservations != 0 {
		t.Fatalf("reservations after failed Turnstile = %d/%v", reservations, err)
	}
	var blocked int64
	if err := db.Model(&models.ReservationAttemptEvent{}).
		Where("action = ? AND result = ?", models.ReservationAttemptCreate, models.ReservationAttemptBlocked).
		Count(&blocked).Error; err != nil || blocked != 1 {
		t.Fatalf("blocked Turnstile attempts = %d/%v", blocked, err)
	}
}

func TestAnonymousReservationConcurrentDuplicateCreatesExactlyOne(t *testing.T) {
	db := newReservationServiceTestDB(t)
	service := newAnonymousReservationServiceForTest(t, db, func() time.Time { return reservationTestNow }, defaultAnonymousTestLimits())
	product := createOpenReservationProduct(t, newReservationTestService(db), "anonymous-concurrent")
	const workers = 20
	start := make(chan struct{})
	var created atomic.Int64
	var duplicates atomic.Int64
	errorsFound := make(chan error, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			_, err := service.Create(t.Context(), anonymousCreateInput(
				product.ID,
				strings.Repeat("a", 64),
				strings.Repeat("b", 64),
				fmt.Sprintf("test-turnstile-concurrent-%d", index),
			))
			var duplicate *AnonymousReservationDuplicateError
			switch {
			case err == nil:
				created.Add(1)
			case errors.As(err, &duplicate):
				duplicates.Add(1)
			default:
				errorsFound <- err
			}
		}(index)
	}
	close(start)
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Fatalf("concurrent create error: %v", err)
	}
	if created.Load() != 1 || duplicates.Load() != workers-1 {
		t.Fatalf("concurrent outcomes = created:%d duplicate:%d", created.Load(), duplicates.Load())
	}
	var count int64
	if err := db.Model(&models.ProductReservation{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("concurrent reservation count = %d/%v", count, err)
	}
}

func TestAnonymousReservationRateLimits(t *testing.T) {
	tests := []struct {
		name       string
		limits     repository.AnonymousReservationLimits
		first      func(index int, productIDs []uint64) AnonymousReservationCreateInput
		blocked    func(productIDs []uint64) AnonymousReservationCreateInput
		wantScope  string
		advanceNow time.Duration
	}{
		{
			name: "browser hour",
			limits: repository.AnonymousReservationLimits{
				BrowserPerHour: 1, BrowserPerDay: 10, IPPerHour: 10, IPPerDay: 10, IPProductPerDay: 10,
			},
			first: func(_ int, products []uint64) AnonymousReservationCreateInput {
				return anonymousCreateInput(products[0], strings.Repeat("a", 64), strings.Repeat("b", 64), "test-turnstile-browser-hour-1")
			},
			blocked: func(products []uint64) AnonymousReservationCreateInput {
				return anonymousCreateInput(products[1], strings.Repeat("a", 64), strings.Repeat("c", 64), "test-turnstile-browser-hour-2")
			},
			wantScope: "browser",
		},
		{
			name: "browser day",
			limits: repository.AnonymousReservationLimits{
				BrowserPerHour: 10, BrowserPerDay: 1, IPPerHour: 10, IPPerDay: 10, IPProductPerDay: 10,
			},
			first: func(_ int, products []uint64) AnonymousReservationCreateInput {
				return anonymousCreateInput(products[0], strings.Repeat("a", 64), strings.Repeat("b", 64), "test-turnstile-browser-day-1")
			},
			blocked: func(products []uint64) AnonymousReservationCreateInput {
				return anonymousCreateInput(products[1], strings.Repeat("a", 64), strings.Repeat("c", 64), "test-turnstile-browser-day-2")
			},
			wantScope: "browser", advanceNow: 2 * time.Hour,
		},
		{
			name: "IP hour",
			limits: repository.AnonymousReservationLimits{
				BrowserPerHour: 10, BrowserPerDay: 10, IPPerHour: 1, IPPerDay: 10, IPProductPerDay: 10,
			},
			first: func(_ int, products []uint64) AnonymousReservationCreateInput {
				return anonymousCreateInput(products[0], strings.Repeat("a", 64), strings.Repeat("b", 64), "test-turnstile-ip-hour-1")
			},
			blocked: func(products []uint64) AnonymousReservationCreateInput {
				return anonymousCreateInput(products[1], strings.Repeat("c", 64), strings.Repeat("b", 64), "test-turnstile-ip-hour-2")
			},
			wantScope: "ip",
		},
		{
			name: "IP day",
			limits: repository.AnonymousReservationLimits{
				BrowserPerHour: 10, BrowserPerDay: 10, IPPerHour: 10, IPPerDay: 1, IPProductPerDay: 10,
			},
			first: func(_ int, products []uint64) AnonymousReservationCreateInput {
				return anonymousCreateInput(products[0], strings.Repeat("a", 64), strings.Repeat("b", 64), "test-turnstile-ip-day-1")
			},
			blocked: func(products []uint64) AnonymousReservationCreateInput {
				return anonymousCreateInput(products[1], strings.Repeat("c", 64), strings.Repeat("b", 64), "test-turnstile-ip-day-2")
			},
			wantScope: "ip", advanceNow: 2 * time.Hour,
		},
		{
			name: "IP product day",
			limits: repository.AnonymousReservationLimits{
				BrowserPerHour: 10, BrowserPerDay: 10, IPPerHour: 10, IPPerDay: 10, IPProductPerDay: 1,
			},
			first: func(_ int, products []uint64) AnonymousReservationCreateInput {
				return anonymousCreateInput(products[0], strings.Repeat("a", 64), strings.Repeat("b", 64), "test-turnstile-ip-product-1")
			},
			blocked: func(products []uint64) AnonymousReservationCreateInput {
				return anonymousCreateInput(products[0], strings.Repeat("c", 64), strings.Repeat("b", 64), "test-turnstile-ip-product-2")
			},
			wantScope: "ip",
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			db := newReservationServiceTestDB(t)
			now := reservationTestNow
			service := newAnonymousReservationServiceForTest(t, db, func() time.Time { return now }, testCase.limits)
			productService := newReservationTestService(db)
			products := []uint64{
				createOpenReservationProduct(t, productService, "rate-a").ID,
				createOpenReservationProduct(t, productService, "rate-b").ID,
			}
			if _, err := service.Create(t.Context(), testCase.first(0, products)); err != nil {
				t.Fatalf("first Create: %v", err)
			}
			now = now.Add(testCase.advanceNow)
			_, err := service.Create(t.Context(), testCase.blocked(products))
			var limit *AnonymousReservationRateLimitError
			if !errors.As(err, &limit) || limit.Scope != testCase.wantScope {
				t.Fatalf("rate limit error = %#v", err)
			}
			var count int64
			if err := db.Model(&models.ProductReservation{}).Count(&count).Error; err != nil || count != 1 {
				t.Fatalf("rate-limited reservation count = %d/%v", count, err)
			}
		})
	}
}

func newAnonymousReservationServiceForTest(
	t *testing.T,
	db *gorm.DB,
	now func() time.Time,
	limits repository.AnonymousReservationLimits,
) *AnonymousReservationService {
	t.Helper()
	service, err := NewAnonymousReservationService(
		repository.NewAnonymousReservationRepository(db),
		AnonymousReservationOptions{
			Now: now, Turnstile: NewMockTurnstileVerifier(false, now),
			TTL: 24 * time.Hour, RateLimits: limits,
		},
	)
	if err != nil {
		t.Fatalf("NewAnonymousReservationService: %v", err)
	}
	return service
}

func defaultAnonymousTestLimits() repository.AnonymousReservationLimits {
	return repository.AnonymousReservationLimits{
		BrowserPerHour: 100, BrowserPerDay: 100, IPPerHour: 100, IPPerDay: 100, IPProductPerDay: 100,
	}
}

func anonymousCreateInput(
	productID uint64,
	browserHash, ipHash, turnstileToken string,
) AnonymousReservationCreateInput {
	return AnonymousReservationCreateInput{
		ProductID: productID, Quantity: 1, TurnstileToken: turnstileToken,
		BrowserHash: browserHash, IPHash: ipHash, RemoteIP: "203.0.113.8", Locale: "en",
	}
}
