package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"saas/internal/models"
	"saas/internal/repository"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestReservationWorkflowRequestVerifyCancelAndReactivateSameRecord(t *testing.T) {
	db := newReservationServiceTestDB(t)
	clock := newWorkflowTestClock(reservationTestNow)
	workflow := newReservationWorkflowForTest(t, db, clock, map[string]bool{"sms": true})
	productService := newReservationTestService(db)
	product := createOpenReservationProduct(t, productService, "verified-workflow")
	visitorID := "11111111-1111-4111-8111-111111111111"
	requestKey := "22222222-2222-4222-8222-222222222222"

	requested, err := workflow.RequestOTP(t.Context(), product.ID, ReservationOTPRequestInput{
		VisitorID: visitorID, IdempotencyKey: requestKey, ContactType: models.ReservationContactSMS,
		PhoneCountry: "GH", PhoneNumber: "024 123 4567", TurnstileToken: "test-turnstile-request-one",
		SourceType: models.ReservationSourceDetail, IPRiskKey: strings.Repeat("a", 64), RemoteIP: "203.0.113.10",
	})
	if err != nil || requested.Status != "pending" || len(requested.ChallengeID) != 43 || requested.ResendAfterSeconds != 60 {
		t.Fatalf("RequestOTP = %+v/%v", requested, err)
	}
	var stored models.ProductReservation
	if err := db.Where("reservation_product_id = ?", product.ID).Take(&stored).Error; err != nil {
		t.Fatalf("load pending reservation: %v", err)
	}
	if stored.ContactHash == "+233241234567" || len(stored.ContactHash) != 64 || len(stored.ContactEncrypted) == 0 || stored.PhoneLast4 != "4567" || stored.PhoneCountryCode != "+233" {
		t.Fatalf("stored contact fields = %+v", stored)
	}
	plaintext, err := workflow.protector.Decrypt(stored.ContactEncrypted, stored.ContactEncryptionVersion)
	if err != nil || plaintext != "+233241234567" {
		t.Fatalf("decrypt stored contact = %q/%v", plaintext, err)
	}
	if stored.TurnstileVerifiedAt == nil || stored.ProviderReference == nil || strings.Contains(*stored.ProviderReference, "+233") {
		t.Fatalf("pending provider/Turnstile fields = %+v", stored)
	}

	idempotent, err := workflow.RequestOTP(t.Context(), product.ID, ReservationOTPRequestInput{
		VisitorID: visitorID, IdempotencyKey: requestKey, ContactType: models.ReservationContactSMS,
		PhoneCountry: "GH", PhoneNumber: "0241234567", TurnstileToken: "test-turnstile-request-one",
		SourceType: models.ReservationSourceDetail, IPRiskKey: strings.Repeat("a", 64),
	})
	if err != nil || idempotent.ChallengeID != requested.ChallengeID {
		t.Fatalf("idempotent RequestOTP = %+v/%v", idempotent, err)
	}
	cooldown, err := workflow.RequestOTP(t.Context(), product.ID, ReservationOTPRequestInput{
		VisitorID: visitorID, IdempotencyKey: uuid.NewString(), ContactType: models.ReservationContactSMS,
		PhoneCountry: "GH", PhoneNumber: "0241234567", TurnstileToken: "test-turnstile-cooldown",
		SourceType: models.ReservationSourceDetail, IPRiskKey: strings.Repeat("a", 64),
	})
	if err != nil || cooldown.Status != "pending" || cooldown.ChallengeID != requested.ChallengeID || cooldown.ResendAfterSeconds != 60 {
		t.Fatalf("opaque resend cooldown response = %+v/%v", cooldown, err)
	}

	invalid, err := workflow.VerifyOTP(t.Context(), product.ID, ReservationOTPVerifyInput{
		ChallengeID: requested.ChallengeID, Code: "000000", IdempotencyKey: uuid.NewString(), IPRiskKey: strings.Repeat("a", 64),
	})
	if !errors.Is(err, ErrReservationInvalidCode) || invalid.Reserved {
		t.Fatalf("invalid VerifyOTP = %+v/%v", invalid, err)
	}
	verifyKey := uuid.NewString()
	verified, err := workflow.VerifyOTP(t.Context(), product.ID, ReservationOTPVerifyInput{
		ChallengeID: requested.ChallengeID, Code: mockOTPCode, IdempotencyKey: verifyKey, IPRiskKey: strings.Repeat("a", 64),
	})
	if err != nil || !verified.Reserved || verified.Status != "verified" || verified.ManageToken == "" {
		t.Fatalf("approved VerifyOTP = %+v/%v", verified, err)
	}
	repeatedVerify, err := workflow.VerifyOTP(t.Context(), product.ID, ReservationOTPVerifyInput{
		ChallengeID: requested.ChallengeID, Code: mockOTPCode, IdempotencyKey: verifyKey, IPRiskKey: strings.Repeat("a", 64),
	})
	if err != nil || !repeatedVerify.Reserved || repeatedVerify.ManageToken != verified.ManageToken {
		t.Fatalf("repeated VerifyOTP = %+v/%v", repeatedVerify, err)
	}
	if _, err := workflow.VerifyOTP(t.Context(), product.ID, ReservationOTPVerifyInput{
		ChallengeID: requested.ChallengeID, Code: mockOTPCode, IdempotencyKey: uuid.NewString(), IPRiskKey: strings.Repeat("a", 64),
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("different-key replay error = %v", err)
	}

	if err := db.Where("id = ?", stored.ID).Take(&stored).Error; err != nil {
		t.Fatalf("reload verified reservation: %v", err)
	}
	if stored.VerificationStatus != models.ReservationVerificationVerified || stored.Status != models.ProductReservationActive || stored.VerifiedAt == nil || stored.ManageTokenHash == nil || *stored.ManageTokenHash == verified.ManageToken {
		t.Fatalf("verified persistence = %+v", stored)
	}
	cancelKey := uuid.NewString()
	cancelled, err := workflow.Cancel(t.Context(), product.ID, ReservationCancelInput{
		ManageToken: verified.ManageToken, IdempotencyKey: cancelKey, IPRiskKey: strings.Repeat("a", 64),
	})
	if err != nil || !cancelled.Cancelled {
		t.Fatalf("Cancel = %+v/%v", cancelled, err)
	}
	repeatedCancel, err := workflow.Cancel(t.Context(), product.ID, ReservationCancelInput{
		ManageToken: verified.ManageToken, IdempotencyKey: cancelKey, IPRiskKey: strings.Repeat("a", 64),
	})
	if err != nil || !repeatedCancel.Cancelled {
		t.Fatalf("repeat Cancel = %+v/%v", repeatedCancel, err)
	}
	if err := db.Where("id = ?", stored.ID).Take(&stored).Error; err != nil || stored.Status != models.ProductReservationCancelled {
		t.Fatalf("cancelled persistence = %+v/%v", stored, err)
	}
	cancelledAdmin, err := productService.AdminProduct(t.Context(), product.ID)
	cancelledAnalytics, analyticsErr := productService.ProductAnalytics(t.Context(), product.ID)
	if err != nil || analyticsErr != nil || cancelledAdmin.ExactVerifiedCount != 0 || cancelledAnalytics.VerifiedReservations != 1 || cancelledAnalytics.CancelledReservations != 1 || cancelledAnalytics.VerificationRate != 1 {
		t.Fatalf("cancelled active/funnel metrics = admin:%+v analytics:%+v errors:%v/%v", cancelledAdmin, cancelledAnalytics, err, analyticsErr)
	}

	clock.Advance(61 * time.Second)
	reactivatedRequest, err := workflow.RequestOTP(t.Context(), product.ID, ReservationOTPRequestInput{
		VisitorID: visitorID, IdempotencyKey: uuid.NewString(), ContactType: models.ReservationContactSMS,
		PhoneCountry: "GH", PhoneNumber: "0241234567", TurnstileToken: "test-turnstile-reactivate",
		SourceType: models.ReservationSourceDialog, IPRiskKey: strings.Repeat("a", 64),
	})
	if err != nil {
		t.Fatalf("reactivation RequestOTP: %v", err)
	}
	var pendingReactivation models.ProductReservation
	if err := db.Where("reservation_product_id = ?", product.ID).Take(&pendingReactivation).Error; err != nil || pendingReactivation.ID != stored.ID || pendingReactivation.Status != models.ProductReservationActive || pendingReactivation.VerificationStatus != models.ReservationVerificationPending || pendingReactivation.VerifiedAt != nil || pendingReactivation.CancelledAt != nil || pendingReactivation.ManageTokenHash != nil {
		t.Fatalf("pending reactivation must reuse and clear cancelled row = %+v/%v", pendingReactivation, err)
	}
	reactivated, err := workflow.VerifyOTP(t.Context(), product.ID, ReservationOTPVerifyInput{
		ChallengeID: reactivatedRequest.ChallengeID, Code: mockOTPCode, IdempotencyKey: uuid.NewString(), IPRiskKey: strings.Repeat("a", 64),
	})
	if err != nil || !reactivated.Reserved || reactivated.ManageToken == "" {
		t.Fatalf("reactivation VerifyOTP = %+v/%v", reactivated, err)
	}
	if reactivated.ManageToken == verified.ManageToken {
		t.Fatal("reactivation reused the cancelled manage token")
	}
	if _, err := workflow.Cancel(t.Context(), product.ID, ReservationCancelInput{
		ManageToken: verified.ManageToken, IdempotencyKey: uuid.NewString(), IPRiskKey: strings.Repeat("a", 64),
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old manage token after reactivation error = %v", err)
	}
	var rows int64
	if err := db.Model(&models.ProductReservation{}).Where("reservation_product_id = ?", product.ID).Count(&rows).Error; err != nil || rows != 1 {
		t.Fatalf("reservation rows after reactivation = %d/%v", rows, err)
	}

	public, err := productService.PublicProduct(t.Context(), product.Slug, "en", visitorID)
	if err != nil || !public.ViewerState.VerifiedReserved {
		t.Fatalf("verified viewer state = %+v/%v", public, err)
	}
	admin, err := productService.AdminProduct(t.Context(), product.ID)
	if err != nil || admin.ExactVerifiedCount != 1 {
		t.Fatalf("admin exact verified count = %+v/%v", admin, err)
	}
	analytics, err := productService.ProductAnalytics(t.Context(), product.ID)
	if err != nil || analytics.ReservationSubmissions != 1 || analytics.VerifiedReservations != 1 || analytics.CancelledReservations != 0 || analytics.VerificationRate != 1 || len(analytics.Contacts) != 1 || analytics.Contacts[0].MaskedPhone != "+233 *** *** 4567" {
		t.Fatalf("verified analytics = %+v/%v", analytics, err)
	}

	var attempts []models.ReservationAttemptEvent
	if err := db.Where("reservation_product_id = ?", product.ID).Find(&attempts).Error; err != nil {
		t.Fatalf("load attempt audit: %v", err)
	}
	if len(attempts) < 8 {
		t.Fatalf("attempt events = %d, want workflow audit", len(attempts))
	}
	for _, attempt := range attempts {
		if attempt.IPRiskKey != nil && strings.Contains(*attempt.IPRiskKey, "203.0.113") {
			t.Fatalf("attempt persisted raw IP: %+v", attempt)
		}
	}
}

func TestReservationWorkflowExpiryMaxAttemptsChannelsAndTurnstile(t *testing.T) {
	db := newReservationServiceTestDB(t)
	clock := newWorkflowTestClock(reservationTestNow)
	workflow := newReservationWorkflowForTest(t, db, clock, map[string]bool{"sms": true})
	product := createOpenReservationProduct(t, newReservationTestService(db), "workflow-errors")
	base := ReservationOTPRequestInput{
		VisitorID: uuid.NewString(), IdempotencyKey: uuid.NewString(), ContactType: models.ReservationContactSMS,
		PhoneCountry: "GH", PhoneNumber: "0241234567", TurnstileToken: "test-turnstile-error-base",
		SourceType: models.ReservationSourceDetail, IPRiskKey: strings.Repeat("b", 64),
	}
	whatsapp := base
	whatsapp.ContactType = models.ReservationContactWhatsApp
	whatsapp.IdempotencyKey = uuid.NewString()
	whatsapp.TurnstileToken = "test-turnstile-whatsapp"
	if _, err := workflow.RequestOTP(t.Context(), product.ID, whatsapp); !errors.Is(err, ErrReservationChannelDisabled) {
		t.Fatalf("disabled WhatsApp error = %v", err)
	}
	invalidTurnstile := base
	invalidTurnstile.IdempotencyKey = uuid.NewString()
	invalidTurnstile.TurnstileToken = "invalid-token"
	if _, err := workflow.RequestOTP(t.Context(), product.ID, invalidTurnstile); !errors.Is(err, ErrTurnstileRejected) {
		t.Fatalf("invalid Turnstile error = %v", err)
	}

	requested, err := workflow.RequestOTP(t.Context(), product.ID, base)
	if err != nil {
		t.Fatalf("RequestOTP for max attempts: %v", err)
	}
	for attempt := 1; attempt <= workflow.maxVerifyAttempts; attempt++ {
		_, err := workflow.VerifyOTP(t.Context(), product.ID, ReservationOTPVerifyInput{
			ChallengeID: requested.ChallengeID, Code: "000000", IdempotencyKey: uuid.NewString(), IPRiskKey: strings.Repeat("b", 64),
		})
		if attempt < workflow.maxVerifyAttempts && !errors.Is(err, ErrReservationInvalidCode) {
			t.Fatalf("attempt %d error = %v", attempt, err)
		}
		if attempt == workflow.maxVerifyAttempts && !errors.Is(err, ErrReservationMaxAttempts) {
			t.Fatalf("max attempt error = %v", err)
		}
	}
	var failed models.ProductReservation
	if err := db.Where("verification_challenge_id = ?", requested.ChallengeID).Take(&failed).Error; err != nil || failed.VerificationStatus != models.ReservationVerificationFailed || failed.VerificationAttemptCount != workflow.maxVerifyAttempts {
		t.Fatalf("failed reservation = %+v/%v", failed, err)
	}

	clock.Advance(61 * time.Second)
	expiryInput := base
	expiryInput.VisitorID = uuid.NewString()
	expiryInput.IdempotencyKey = uuid.NewString()
	expiryInput.PhoneNumber = "0201234567"
	expiryInput.TurnstileToken = "test-turnstile-expiry"
	expiring, err := workflow.RequestOTP(t.Context(), product.ID, expiryInput)
	if err != nil {
		t.Fatalf("RequestOTP for expiry: %v", err)
	}
	clock.Advance(workflow.challengeTTL)
	if _, err := workflow.VerifyOTP(t.Context(), product.ID, ReservationOTPVerifyInput{
		ChallengeID: expiring.ChallengeID, Code: mockOTPCode, IdempotencyKey: uuid.NewString(), IPRiskKey: strings.Repeat("b", 64),
	}); !errors.Is(err, ErrReservationChallengeExpired) {
		t.Fatalf("expired challenge error = %v", err)
	}
}

func TestReservationWorkflowConcurrentVerificationReturnsManageTokenOnce(t *testing.T) {
	db := newReservationServiceTestDB(t)
	clock := newWorkflowTestClock(reservationTestNow)
	workflow := newReservationWorkflowForTest(t, db, clock, map[string]bool{"sms": true})
	product := createOpenReservationProduct(t, newReservationTestService(db), "concurrent-verification")
	requested, err := workflow.RequestOTP(t.Context(), product.ID, ReservationOTPRequestInput{
		VisitorID: uuid.NewString(), IdempotencyKey: uuid.NewString(), ContactType: models.ReservationContactSMS,
		PhoneCountry: "GH", PhoneNumber: "0241234567", TurnstileToken: "test-turnstile-concurrent",
		SourceType: models.ReservationSourceDetail, IPRiskKey: strings.Repeat("c", 64),
	})
	if err != nil {
		t.Fatalf("RequestOTP: %v", err)
	}
	const workers = 12
	start := make(chan struct{})
	var tokens atomic.Int64
	errorsFound := make(chan error, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			result, err := workflow.VerifyOTP(t.Context(), product.ID, ReservationOTPVerifyInput{
				ChallengeID: requested.ChallengeID, Code: mockOTPCode, IdempotencyKey: uuid.NewString(), IPRiskKey: strings.Repeat("c", 64),
			})
			if err != nil {
				errorsFound <- err
				return
			}
			if !result.Reserved {
				errorsFound <- errors.New("verification did not reserve")
			}
			if result.ManageToken != "" {
				tokens.Add(1)
			}
		}()
	}
	close(start)
	wait.Wait()
	close(errorsFound)
	conflicts := 0
	for err := range errorsFound {
		if errors.Is(err, ErrConflict) {
			conflicts++
			continue
		}
		t.Errorf("concurrent verification: %v", err)
	}
	if tokens.Load() != 1 {
		t.Fatalf("manage tokens returned = %d, want 1", tokens.Load())
	}
	if conflicts != workers-1 {
		t.Fatalf("concurrent conflicts = %d, want %d", conflicts, workers-1)
	}
}

func TestReservationWorkflowConcurrentRequestsCallProviderOncePerContact(t *testing.T) {
	db := newReservationServiceTestDB(t)
	clock := newWorkflowTestClock(reservationTestNow)
	provider := &countingOTPProvider{delegate: NewMockOTPProvider()}
	workflow := newReservationWorkflowWithProviderForTest(t, db, clock, provider, highReservationRateLimits())
	product := createOpenReservationProduct(t, newReservationTestService(db), "concurrent-request")
	start := make(chan struct{})
	results := make(chan ReservationOTPRequestResult, 2)
	errorsFound := make(chan error, 2)
	var wait sync.WaitGroup
	for index, number := range []string{"024 123 4567", "0241234567"} {
		wait.Add(1)
		go func(index int, number string) {
			defer wait.Done()
			<-start
			result, err := workflow.RequestOTP(t.Context(), product.ID, ReservationOTPRequestInput{
				VisitorID: uuid.NewString(), IdempotencyKey: uuid.NewString(), ContactType: models.ReservationContactSMS,
				PhoneCountry: "GH", PhoneNumber: number, TurnstileToken: "test-turnstile-concurrent-request-" + string(rune('a'+index)),
				SourceType: models.ReservationSourceDetail, IPRiskKey: strings.Repeat("f", 64),
			})
			if err != nil {
				errorsFound <- err
				return
			}
			results <- result
		}(index, number)
	}
	close(start)
	wait.Wait()
	close(results)
	close(errorsFound)
	for err := range errorsFound {
		t.Fatalf("concurrent RequestOTP: %v", err)
	}
	var challenge string
	for result := range results {
		if result.Status != "pending" || result.ChallengeID == "" {
			t.Fatalf("concurrent request result = %+v", result)
		}
		if challenge == "" {
			challenge = result.ChallengeID
		} else if result.ChallengeID != challenge {
			t.Fatalf("concurrent requests returned different challenges")
		}
	}
	if provider.starts.Load() != 1 {
		t.Fatalf("provider starts = %d, want 1", provider.starts.Load())
	}
	var count int64
	if err := db.Model(&models.ProductReservation{}).Where("reservation_product_id = ?", product.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("reservation rows = %d/%v", count, err)
	}
}

func TestReservationWorkflowTurnstileTokenCannotCrossProductBoundary(t *testing.T) {
	db := newReservationServiceTestDB(t)
	clock := newWorkflowTestClock(reservationTestNow)
	provider := &countingOTPProvider{delegate: NewMockOTPProvider()}
	workflow := newReservationWorkflowWithProviderForTest(t, db, clock, provider, highReservationRateLimits())
	productService := newReservationTestService(db)
	firstProduct := createOpenReservationProduct(t, productService, "turnstile-product-one")
	secondProduct := createOpenReservationProduct(t, productService, "turnstile-product-two")
	input := ReservationOTPRequestInput{
		VisitorID: uuid.NewString(), IdempotencyKey: uuid.NewString(), ContactType: models.ReservationContactSMS,
		PhoneCountry: "GH", PhoneNumber: "0241234567", TurnstileToken: "test-turnstile-cross-product",
		SourceType: models.ReservationSourceDetail, IPRiskKey: strings.Repeat("7", 64),
	}
	if _, err := workflow.RequestOTP(t.Context(), firstProduct.ID, input); err != nil {
		t.Fatalf("first product RequestOTP: %v", err)
	}
	if _, err := workflow.RequestOTP(t.Context(), secondProduct.ID, input); !errors.Is(err, ErrTurnstileRejected) {
		t.Fatalf("cross-product token replay error = %v", err)
	}
	if provider.starts.Load() != 1 {
		t.Fatalf("cross-product replay provider starts = %d, want 1", provider.starts.Load())
	}
}

func TestReservationWorkflowArchiveBlocksPendingVerification(t *testing.T) {
	db := newReservationServiceTestDB(t)
	clock := newWorkflowTestClock(reservationTestNow)
	provider := &countingOTPProvider{delegate: NewMockOTPProvider()}
	workflow := newReservationWorkflowWithProviderForTest(t, db, clock, provider, highReservationRateLimits())
	productService := newReservationTestService(db)
	product := createOpenReservationProduct(t, productService, "archive-before-verify")
	requested, err := workflow.RequestOTP(t.Context(), product.ID, ReservationOTPRequestInput{
		VisitorID: uuid.NewString(), IdempotencyKey: uuid.NewString(), ContactType: models.ReservationContactSMS,
		PhoneCountry: "GH", PhoneNumber: "0241234567", TurnstileToken: "test-turnstile-archive-before-verify",
		SourceType: models.ReservationSourceDetail, IPRiskKey: strings.Repeat("5", 64),
	})
	if err != nil {
		t.Fatalf("RequestOTP: %v", err)
	}
	if err := productService.ArchiveProduct(t.Context(), product.ID); err != nil {
		t.Fatalf("archive product: %v", err)
	}
	if _, err := workflow.VerifyOTP(t.Context(), product.ID, ReservationOTPVerifyInput{
		ChallengeID: requested.ChallengeID, Code: mockOTPCode, IdempotencyKey: uuid.NewString(), IPRiskKey: strings.Repeat("5", 64),
	}); !errors.Is(err, ErrReservationProductUnavailable) {
		t.Fatalf("archived product verify error = %v", err)
	}
	if provider.checks.Load() != 0 {
		t.Fatalf("archived product called provider check %d times", provider.checks.Load())
	}
	var stored models.ProductReservation
	if err := db.Where("verification_challenge_id = ?", requested.ChallengeID).Take(&stored).Error; err != nil || stored.VerificationStatus != models.ReservationVerificationPending {
		t.Fatalf("archived pending reservation changed = %+v/%v", stored, err)
	}
}

func TestReservationWorkflowContactVisitorIPAndGlobalLimitsRunBeforeProvider(t *testing.T) {
	tests := []struct {
		name   string
		limits func(*ReservationRateLimits)
		second func(ReservationOTPRequestInput) ReservationOTPRequestInput
	}{
		{
			name: "contact", limits: func(value *ReservationRateLimits) { value.OTPRequestContactPer30Min = 1 },
			second: func(input ReservationOTPRequestInput) ReservationOTPRequestInput { return input },
		},
		{
			name: "visitor", limits: func(value *ReservationRateLimits) { value.OTPRequestVisitorPerHour = 1 },
			second: func(input ReservationOTPRequestInput) ReservationOTPRequestInput {
				input.PhoneNumber = "0201234567"
				return input
			},
		},
		{
			name: "IP", limits: func(value *ReservationRateLimits) { value.OTPRequestIPPerHour = 1 },
			second: func(input ReservationOTPRequestInput) ReservationOTPRequestInput {
				input.PhoneNumber = "0201234567"
				input.VisitorID = uuid.NewString()
				return input
			},
		},
		{
			name: "global", limits: func(value *ReservationRateLimits) { value.OTPRequestGlobalPerMinute = 1 },
			second: func(input ReservationOTPRequestInput) ReservationOTPRequestInput {
				input.PhoneNumber = "0201234567"
				input.VisitorID = uuid.NewString()
				input.IPRiskKey = strings.Repeat("e", 64)
				return input
			},
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			db := newReservationServiceTestDB(t)
			clock := newWorkflowTestClock(reservationTestNow)
			provider := &countingOTPProvider{delegate: NewMockOTPProvider()}
			limits := highReservationRateLimits()
			testCase.limits(&limits)
			workflow := newReservationWorkflowWithProviderForTest(t, db, clock, provider, limits)
			product := createOpenReservationProduct(t, newReservationTestService(db), "limit-"+strings.ToLower(testCase.name))
			first := ReservationOTPRequestInput{
				VisitorID: uuid.NewString(), IdempotencyKey: uuid.NewString(), ContactType: models.ReservationContactSMS,
				PhoneCountry: "GH", PhoneNumber: "0241234567", TurnstileToken: "test-turnstile-limit-first",
				SourceType: models.ReservationSourceDetail, IPRiskKey: strings.Repeat("d", 64),
			}
			if _, err := workflow.RequestOTP(t.Context(), product.ID, first); err != nil {
				t.Fatalf("first RequestOTP: %v", err)
			}
			if testCase.name == "contact" {
				clock.Advance(61 * time.Second)
			} else {
				clock.Advance(2 * time.Second)
			}
			second := testCase.second(first)
			second.IdempotencyKey = uuid.NewString()
			second.TurnstileToken = "test-turnstile-limit-second"
			result, err := workflow.RequestOTP(t.Context(), product.ID, second)
			if testCase.name == "contact" {
				if err != nil || result.Status != "pending" || result.ChallengeID == "" {
					t.Fatalf("opaque contact-limit response = %+v/%v", result, err)
				}
			} else if !errors.Is(err, ErrRateLimited) {
				t.Fatalf("second RequestOTP error = %v", err)
			}
			if provider.starts.Load() != 1 {
				t.Fatalf("provider calls = %d, want 1", provider.starts.Load())
			}
		})
	}
}

func TestReservationWorkflowPendingAndNewContactsShareResponseFloor(t *testing.T) {
	db := newReservationServiceTestDB(t)
	clock := newWorkflowTestClock(reservationTestNow)
	workflow := newReservationWorkflowForTest(t, db, clock, map[string]bool{"sms": true})
	workflow.minimumResponseTime = 25 * time.Millisecond
	product := createOpenReservationProduct(t, newReservationTestService(db), "response-floor")
	request := func(number, token string) time.Duration {
		started := time.Now()
		result, err := workflow.RequestOTP(t.Context(), product.ID, ReservationOTPRequestInput{
			VisitorID: uuid.NewString(), IdempotencyKey: uuid.NewString(), ContactType: models.ReservationContactSMS,
			PhoneCountry: "GH", PhoneNumber: number, TurnstileToken: token,
			SourceType: models.ReservationSourceDetail, IPRiskKey: strings.Repeat("6", 64),
		})
		if err != nil || result.Status != "pending" || len(result.ChallengeID) != 43 {
			t.Fatalf("response-floor RequestOTP = %+v/%v", result, err)
		}
		return time.Since(started)
	}
	newElapsed := request("0241234567", "test-turnstile-response-floor-new")
	pendingElapsed := request("0241234567", "test-turnstile-response-floor-pending")
	if newElapsed < 20*time.Millisecond || pendingElapsed < 20*time.Millisecond {
		t.Fatalf("response floor not applied: new=%s pending=%s", newElapsed, pendingElapsed)
	}
}

type workflowTestClock struct{ nanos atomic.Int64 }

func newWorkflowTestClock(value time.Time) *workflowTestClock {
	clock := &workflowTestClock{}
	clock.nanos.Store(value.UnixNano())
	return clock
}

func (c *workflowTestClock) Now() time.Time              { return time.Unix(0, c.nanos.Load()).UTC() }
func (c *workflowTestClock) Advance(value time.Duration) { c.nanos.Add(int64(value)) }

type countingOTPProvider struct {
	delegate OTPProvider
	starts   atomic.Int64
	checks   atomic.Int64
}

func (p *countingOTPProvider) Name() string { return p.delegate.Name() }
func (p *countingOTPProvider) StartVerification(ctx context.Context, e164 string, channel models.ReservationContactType, locale string) (string, error) {
	p.starts.Add(1)
	return p.delegate.StartVerification(ctx, e164, channel, locale)
}
func (p *countingOTPProvider) CheckVerification(ctx context.Context, e164, code string) (OTPVerificationResult, error) {
	p.checks.Add(1)
	return p.delegate.CheckVerification(ctx, e164, code)
}

func newReservationWorkflowForTest(t *testing.T, db *gorm.DB, clock *workflowTestClock, channels map[string]bool) *ReservationWorkflowService {
	t.Helper()
	return newReservationWorkflowForTestWith(t, db, clock, NewMockOTPProvider(), highReservationRateLimits(), channels)
}

func newReservationWorkflowWithProviderForTest(t *testing.T, db *gorm.DB, clock *workflowTestClock, provider OTPProvider, limits ReservationRateLimits) *ReservationWorkflowService {
	t.Helper()
	return newReservationWorkflowForTestWith(t, db, clock, provider, limits, map[string]bool{"sms": true})
}

func newReservationWorkflowForTestWith(t *testing.T, db *gorm.DB, clock *workflowTestClock, provider OTPProvider, limits ReservationRateLimits, channels map[string]bool) *ReservationWorkflowService {
	t.Helper()
	protector, err := NewReservationContactProtector(
		"workflow-contact-hash-secret-0123456789",
		[]byte("abcdef0123456789abcdef0123456789"),
	)
	if err != nil {
		t.Fatalf("NewReservationContactProtector: %v", err)
	}
	workflow, err := NewReservationWorkflowService(
		repository.NewReservationWorkflowRepository(db),
		ReservationWorkflowOptions{
			Now: clock.Now, Protector: protector, Turnstile: NewMockTurnstileVerifier(false, clock.Now), OTPProvider: provider,
			Limiter: NewReservationMemoryLimiter(clock.Now), RateLimits: limits, EnabledChannels: channels,
			ChallengeTTL: 10 * time.Minute, ResendCooldown: 60 * time.Second, MaxVerifyAttempts: 5,
		},
	)
	if err != nil {
		t.Fatalf("NewReservationWorkflowService: %v", err)
	}
	return workflow
}

func highReservationRateLimits() ReservationRateLimits {
	return ReservationRateLimits{
		InterestVisitorPerMinute: 1000, InterestIPPerMinute: 1000, InterestGlobalPerMinute: 10000,
		OTPRequestContactPer30Min: 1000, OTPRequestContactPerDay: 1000, OTPRequestVisitorPerHour: 1000,
		OTPRequestIPPerHour: 1000, OTPRequestGlobalPerMinute: 1000,
		OTPVerifyVisitorPer10Min: 1000, OTPVerifyIPPer10Min: 1000, OTPVerifyGlobalPerMinute: 10000,
		CancelIPPerMinute: 1000, CancelGlobalPerMinute: 10000,
	}
}
