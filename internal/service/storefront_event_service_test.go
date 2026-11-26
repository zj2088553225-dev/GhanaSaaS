package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"saas/internal/models"

	"gorm.io/gorm"
)

const (
	testEventID   = "00000000-0000-4000-8000-000000000001"
	testVisitorID = "11111111-1111-4111-8111-111111111111"
	testSessionID = "22222222-2222-4222-8222-222222222222"
)

type fakeStorefrontEventRepository struct {
	mu               sync.Mutex
	productTenants   map[uint]uint
	shopTenants      map[string]uint
	platformTenant   *uint
	created          *models.StorefrontEvent
	createdMany      []*models.StorefrontEvent
	findProductErr   error
	createErr        error
	createManyErr    error
	batchAccepted    int
	batchDuplicates  int
	existingEventIDs map[string]struct{}
	throttles        map[string]time.Time
	createdEvents    []*models.StorefrontEvent
}

func (repo *fakeStorefrontEventRepository) FindExistingEventIDs(_ context.Context, eventIDs []string) (map[string]struct{}, error) {
	found := make(map[string]struct{})
	for _, eventID := range eventIDs {
		if _, exists := repo.existingEventIDs[eventID]; exists {
			found[eventID] = struct{}{}
		}
	}
	return found, nil
}

func (repo *fakeStorefrontEventRepository) CreateMany(_ context.Context, events []*models.StorefrontEvent) (int, int, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if repo.createManyErr != nil {
		return 0, 0, repo.createManyErr
	}
	repo.createdMany = make([]*models.StorefrontEvent, 0, len(events))
	duplicates := 0
	for _, event := range events {
		if !repo.acquireThrottle(event) {
			duplicates++
			continue
		}
		clone := *event
		repo.createdMany = append(repo.createdMany, &clone)
		repo.createdEvents = append(repo.createdEvents, &clone)
	}
	if repo.batchAccepted != 0 || repo.batchDuplicates != 0 {
		return repo.batchAccepted, repo.batchDuplicates, nil
	}
	return len(repo.createdMany), duplicates, nil
}

func (repo *fakeStorefrontEventRepository) Create(_ context.Context, event *models.StorefrontEvent) error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if repo.createErr != nil {
		return repo.createErr
	}
	if !repo.acquireThrottle(event) {
		return nil
	}
	clone := *event
	repo.created = &clone
	repo.createdEvents = append(repo.createdEvents, &clone)
	return nil
}

func (repo *fakeStorefrontEventRepository) acquireThrottle(event *models.StorefrontEvent) bool {
	if event.ThrottleKey == "" {
		return true
	}
	if repo.throttles == nil {
		repo.throttles = make(map[string]time.Time)
	}
	if expiresAt, exists := repo.throttles[event.ThrottleKey]; exists &&
		expiresAt.After(event.OccurredAt) {
		return false
	}
	repo.throttles[event.ThrottleKey] = event.ThrottleUntil
	return true
}

func (repo *fakeStorefrontEventRepository) FindPublicProductTenantID(_ context.Context, productID uint) (uint, error) {
	if repo.findProductErr != nil {
		return 0, repo.findProductErr
	}
	tenantID, ok := repo.productTenants[productID]
	if !ok {
		return 0, gorm.ErrRecordNotFound
	}
	return tenantID, nil
}

func (repo *fakeStorefrontEventRepository) FindPublicShopTenantID(_ context.Context, identifier string) (uint, error) {
	tenantID, ok := repo.shopTenants[identifier]
	if !ok {
		return 0, gorm.ErrRecordNotFound
	}
	return tenantID, nil
}

func (repo *fakeStorefrontEventRepository) FindPlatformMallTenantID(_ context.Context) (*uint, error) {
	return repo.platformTenant, nil
}

func validProductEventInput() StorefrontEventInput {
	productID := uint(42)
	return StorefrontEventInput{
		EventID:    testEventID,
		EventName:  models.StorefrontEventProductClick,
		VisitorID:  testVisitorID,
		SessionID:  testSessionID,
		ProductID:  &productID,
		SourceType: models.StorefrontSourceMall,
		SourceID:   "all",
		PageType:   models.StorefrontPageMall,
		PagePath:   "/en/mall",
		Locale:     "en",
		Referrer:   "https://example.com/en/?campaign=test#hero",
		DeviceType: models.StorefrontDeviceMobile,
	}
}

func TestStorefrontEventServiceTracksValidProductEventAndDerivesTenant(t *testing.T) {
	repo := &fakeStorefrontEventRepository{productTenants: map[uint]uint{42: 7}}
	fixedTime := time.Date(2026, 7, 14, 5, 6, 7, 0, time.FixedZone("test", 8*60*60))
	service := NewStorefrontEventService(repo, StorefrontEventOptions{Now: func() time.Time { return fixedTime }})

	if err := service.Track(context.Background(), validProductEventInput()); err != nil {
		t.Fatalf("Track returned error: %v", err)
	}
	if repo.created == nil {
		t.Fatal("event was not created")
	}
	if repo.created.TenantID == nil || *repo.created.TenantID != 7 {
		t.Fatalf("TenantID = %v, want 7", repo.created.TenantID)
	}
	if repo.created.ProductID == nil || *repo.created.ProductID != 42 {
		t.Fatalf("ProductID = %v, want 42", repo.created.ProductID)
	}
	if repo.created.EntityType != models.StorefrontEntityProduct || repo.created.EntityID == nil || *repo.created.EntityID != "42" {
		t.Fatalf("product entity = %q/%v, want product/42", repo.created.EntityType, repo.created.EntityID)
	}
	if repo.created.Referrer != "https://example.com/en/" {
		t.Fatalf("Referrer = %q, want query and fragment removed", repo.created.Referrer)
	}
	if !repo.created.OccurredAt.Equal(fixedTime.UTC()) || !repo.created.CreatedAt.Equal(fixedTime.UTC()) {
		t.Fatalf("server timestamps = %s/%s, want %s", repo.created.OccurredAt, repo.created.CreatedAt, fixedTime.UTC())
	}
}

func TestStorefrontEventServiceRejectsInvalidEventName(t *testing.T) {
	repo := &fakeStorefrontEventRepository{productTenants: map[uint]uint{42: 7}}
	service := NewStorefrontEventService(repo)
	input := validProductEventInput()
	input.EventName = "purchase"

	if err := service.Track(context.Background(), input); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Track error = %v, want ErrInvalidInput", err)
	}
	if repo.created != nil {
		t.Fatal("invalid event was created")
	}
}

func TestStorefrontEventServiceRequiresValidEventID(t *testing.T) {
	repo := &fakeStorefrontEventRepository{productTenants: map[uint]uint{42: 7}}
	tracker := NewStorefrontEventService(repo)
	for _, eventID := range []string{"", "not-a-uuid", strings.Repeat("a", maxStorefrontEventIDLength+1)} {
		input := validProductEventInput()
		input.EventID = eventID
		if err := tracker.Track(context.Background(), input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("Track event_id %q error = %v, want ErrInvalidInput", eventID, err)
		}
	}
	if repo.created != nil {
		t.Fatal("event with invalid event_id was created")
	}
}

func TestStorefrontEventServiceUsesServerTrafficClassification(t *testing.T) {
	repo := &fakeStorefrontEventRepository{productTenants: map[uint]uint{42: 7}}
	tracker := NewStorefrontEventService(repo)
	if err := tracker.Track(context.Background(), validProductEventInput(), models.StorefrontTrafficBot); err != nil {
		t.Fatalf("Track: %v", err)
	}
	if repo.created == nil || repo.created.TrafficType != models.StorefrontTrafficBot {
		t.Fatalf("TrafficType = %v, want bot", repo.created)
	}
}

func TestStorefrontPopularityThrottleIgnoresRotatedEventIDsWithinInterval(t *testing.T) {
	current := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	repo := &fakeStorefrontEventRepository{productTenants: map[uint]uint{42: 7}}
	tracker := NewStorefrontEventService(repo, StorefrontEventOptions{Now: func() time.Time { return current }})

	first := validProductEventInput()
	first.EventName = models.StorefrontEventProductView
	if err := tracker.Track(t.Context(), first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.EventID = "00000000-0000-4000-8000-000000000002"
	current = current.Add(29 * time.Second)
	if err := tracker.Track(t.Context(), second); err != nil {
		t.Fatal(err)
	}
	if len(repo.createdEvents) != 1 {
		t.Fatalf("events inside view throttle = %d, want 1", len(repo.createdEvents))
	}

	third := first
	third.EventID = "00000000-0000-4000-8000-000000000003"
	current = current.Add(2 * time.Second)
	if err := tracker.Track(t.Context(), third); err != nil {
		t.Fatal(err)
	}
	if len(repo.createdEvents) != 2 {
		t.Fatalf("events after view throttle = %d, want 2", len(repo.createdEvents))
	}
}

func TestStorefrontPopularityThrottleScopesEventProductAndTenant(t *testing.T) {
	current := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	repo := &fakeStorefrontEventRepository{
		productTenants: map[uint]uint{42: 7, 43: 7, 84: 9},
	}
	tracker := NewStorefrontEventService(repo, StorefrontEventOptions{Now: func() time.Time { return current }})

	inputs := []StorefrontEventInput{
		validProductEventInput(),
		validProductEventInput(),
		validProductEventInput(),
		validProductEventInput(),
	}
	inputs[0].EventName = models.StorefrontEventProductView
	inputs[1].EventName = models.StorefrontEventProductClick
	inputs[1].EventID = "00000000-0000-4000-8000-000000000002"
	product43 := uint(43)
	inputs[2].ProductID = &product43
	inputs[2].EventID = "00000000-0000-4000-8000-000000000003"
	product84 := uint(84)
	inputs[3].ProductID = &product84
	inputs[3].EventID = "00000000-0000-4000-8000-000000000004"

	for _, input := range inputs {
		if err := tracker.Track(t.Context(), input); err != nil {
			t.Fatal(err)
		}
	}
	if len(repo.createdEvents) != 4 {
		t.Fatalf("event/product/tenant scoped events = %d, want 4", len(repo.createdEvents))
	}
}

func TestStorefrontPopularityThrottleIsConcurrentSafe(t *testing.T) {
	fixedTime := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	repo := &fakeStorefrontEventRepository{productTenants: map[uint]uint{42: 7}}
	tracker := NewStorefrontEventService(repo, StorefrontEventOptions{Now: func() time.Time { return fixedTime }})

	const workers = 20
	var waitGroup sync.WaitGroup
	errs := make(chan error, workers)
	for index := 0; index < workers; index++ {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			input := validProductEventInput()
			input.EventName = models.StorefrontEventProductView
			input.EventID = fmt.Sprintf("00000000-0000-4000-8000-%012d", index+1)
			errs <- tracker.Track(context.Background(), input)
		}(index)
	}
	waitGroup.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	if len(repo.createdEvents) != 1 {
		t.Fatalf("concurrent duplicate events = %d, want 1", len(repo.createdEvents))
	}
}

func TestStorefrontEventServiceAcceptsExistingEventIDWithoutRevalidatingChangedProduct(t *testing.T) {
	repo := &fakeStorefrontEventRepository{
		productTenants:   map[uint]uint{},
		existingEventIDs: map[string]struct{}{testEventID: {}},
	}
	tracker := NewStorefrontEventService(repo)
	if err := tracker.Track(context.Background(), validProductEventInput()); err != nil {
		t.Fatalf("duplicate Track: %v", err)
	}
	if repo.created != nil {
		t.Fatal("duplicate event was sent to Create")
	}
}

func TestStorefrontEventServiceRejectsMissingOrNonPublicProduct(t *testing.T) {
	tests := []struct {
		name  string
		input StorefrontEventInput
	}{
		{name: "unknown product", input: validProductEventInput()},
		{name: "zero product id", input: func() StorefrontEventInput {
			input := validProductEventInput()
			zero := uint(0)
			input.ProductID = &zero
			return input
		}()},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &fakeStorefrontEventRepository{productTenants: map[uint]uint{}}
			service := NewStorefrontEventService(repo)
			if err := service.Track(context.Background(), test.input); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("Track error = %v, want ErrInvalidInput", err)
			}
			if repo.created != nil {
				t.Fatal("invalid/non-public product event was created")
			}
		})
	}
}

func TestStorefrontEventServiceNormalizesLocale(t *testing.T) {
	repo := &fakeStorefrontEventRepository{productTenants: map[uint]uint{42: 7}}
	service := NewStorefrontEventService(repo)
	input := validProductEventInput()
	input.Locale = " ZH "

	if err := service.Track(context.Background(), input); err != nil {
		t.Fatalf("Track returned error: %v", err)
	}
	if repo.created.Locale != "zh" {
		t.Fatalf("Locale = %q, want zh", repo.created.Locale)
	}
}

func TestStorefrontEventServiceRejectsOverlongStrings(t *testing.T) {
	repo := &fakeStorefrontEventRepository{productTenants: map[uint]uint{42: 7}}
	service := NewStorefrontEventService(repo)

	tests := []struct {
		name   string
		mutate func(*StorefrontEventInput)
	}{
		{name: "source id", mutate: func(input *StorefrontEventInput) { input.SourceID = strings.Repeat("a", maxStorefrontSourceIDLength+1) }},
		{name: "page path", mutate: func(input *StorefrontEventInput) {
			input.PagePath = "/" + strings.Repeat("a", maxStorefrontPagePathLength)
		}},
		{name: "referrer", mutate: func(input *StorefrontEventInput) {
			input.Referrer = "https://example.com/" + strings.Repeat("a", maxStorefrontReferrerLength)
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validProductEventInput()
			test.mutate(&input)
			if err := service.Track(context.Background(), input); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("Track error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestStorefrontEventServicePropagatesRepositoryErrorWithoutCreating(t *testing.T) {
	databaseError := errors.New("database unavailable")
	repo := &fakeStorefrontEventRepository{
		productTenants: map[uint]uint{},
		findProductErr: databaseError,
	}
	service := NewStorefrontEventService(repo)

	if err := service.Track(context.Background(), validProductEventInput()); !errors.Is(err, databaseError) {
		t.Fatalf("Track error = %v, want repository error", err)
	}
	if repo.created != nil {
		t.Fatal("event was created after repository failure")
	}
}

func TestStorefrontEventServiceTracksAtomicBatchWithPerEventTenantDerivation(t *testing.T) {
	repo := &fakeStorefrontEventRepository{productTenants: map[uint]uint{42: 7, 84: 9}}
	fixedTime := time.Date(2026, 7, 15, 1, 2, 3, 0, time.UTC)
	tracker := NewStorefrontEventService(repo, StorefrontEventOptions{Now: func() time.Time { return fixedTime }})
	first := validProductEventInput()
	second := validProductEventInput()
	secondProductID := uint(84)
	second.ProductID = &secondProductID
	second.VisitorID = "33333333-3333-4333-8333-333333333333"
	second.EventID = "00000000-0000-4000-8000-000000000002"

	result, err := tracker.TrackBatch(context.Background(), StorefrontEventBatchInput{Events: []StorefrontEventInput{first, second}})
	if err != nil {
		t.Fatalf("TrackBatch returned error: %v", err)
	}
	if result.Accepted != 2 || result.Duplicates != 0 || len(repo.createdMany) != 2 {
		t.Fatalf("result/created = %+v/%d, want 2 accepted and 0 duplicates", result, len(repo.createdMany))
	}
	if repo.createdMany[0].TenantID == nil || *repo.createdMany[0].TenantID != 7 ||
		repo.createdMany[1].TenantID == nil || *repo.createdMany[1].TenantID != 9 {
		t.Fatalf("derived tenants = %v/%v, want 7/9", repo.createdMany[0].TenantID, repo.createdMany[1].TenantID)
	}
	for _, event := range repo.createdMany {
		if !event.OccurredAt.Equal(fixedTime) || !event.CreatedAt.Equal(fixedTime) {
			t.Fatalf("batch timestamp = %s/%s, want %s", event.OccurredAt, event.CreatedAt, fixedTime)
		}
	}
}

func TestStorefrontEventServiceRejectsInvalidBatchAtomically(t *testing.T) {
	tests := []struct {
		name   string
		events []StorefrontEventInput
	}{
		{name: "empty"},
		{name: "too many", events: make([]StorefrontEventInput, MaxStorefrontEventBatchSize+1)},
		{name: "invalid middle", events: func() []StorefrontEventInput {
			inputs := []StorefrontEventInput{validProductEventInput(), validProductEventInput(), validProductEventInput()}
			inputs[1].EventName = "purchase"
			return inputs
		}()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &fakeStorefrontEventRepository{productTenants: map[uint]uint{42: 7}}
			tracker := NewStorefrontEventService(repo)
			result, err := tracker.TrackBatch(context.Background(), StorefrontEventBatchInput{Events: test.events})
			if result != (StorefrontEventBatchResult{}) || !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("TrackBatch = %+v/%v, want empty result/ErrInvalidInput", result, err)
			}
			if len(repo.createdMany) != 0 {
				t.Fatalf("invalid batch inserted %d events", len(repo.createdMany))
			}
		})
	}
}

func TestStorefrontEventServiceBatchRepositoryFailureReturnsNoAcceptedEvents(t *testing.T) {
	databaseError := errors.New("database unavailable")
	repo := &fakeStorefrontEventRepository{
		productTenants: map[uint]uint{42: 7},
		createManyErr:  databaseError,
	}
	tracker := NewStorefrontEventService(repo)
	result, err := tracker.TrackBatch(context.Background(), StorefrontEventBatchInput{Events: []StorefrontEventInput{validProductEventInput()}})
	if result != (StorefrontEventBatchResult{}) || !errors.Is(err, databaseError) {
		t.Fatalf("TrackBatch = %+v/%v, want empty result/database error", result, err)
	}
}

func TestStorefrontEventServiceReturnsBatchIdempotencyCounts(t *testing.T) {
	repo := &fakeStorefrontEventRepository{
		productTenants:  map[uint]uint{42: 7},
		batchAccepted:   1,
		batchDuplicates: 1,
	}
	tracker := NewStorefrontEventService(repo)
	first := validProductEventInput()
	second := validProductEventInput()
	second.EventID = "00000000-0000-4000-8000-000000000002"
	result, err := tracker.TrackBatch(context.Background(), StorefrontEventBatchInput{Events: []StorefrontEventInput{first, second}})
	if err != nil {
		t.Fatalf("TrackBatch: %v", err)
	}
	if result.Accepted != 1 || result.Duplicates != 1 {
		t.Fatalf("TrackBatch result = %+v, want 1 accepted/1 duplicate", result)
	}
}

func TestNormalizeStorefrontPagePathPreservesUnicodeAndReservedEscapes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "locale", input: "/en", want: "/en"},
		{name: "product", input: "/en/products/196", want: "/en/products/196"},
		{name: "query", input: "/en/mall?category=123", want: "/en/mall"},
		{name: "fragment", input: "/en/mall#top", want: "/en/mall"},
		{name: "raw Unicode", input: "/zh/商品/灯具?category=123#detail", want: "/zh/商品/灯具"},
		{name: "encoded Unicode", input: "/zh/%E5%95%86%E5%93%81", want: "/zh/商品"},
		{name: "reserved escapes", input: "/a%2fb/%3f/%23/%25/%252F", want: "/a%2Fb/%3F/%23/%25/%252F"},
		{name: "encoded unreserved", input: "/%41", want: "/A"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalizeStorefrontPagePath(test.input)
			if err != nil || got != test.want {
				t.Fatalf("normalizeStorefrontPagePath(%q) = %q/%v, want %q", test.input, got, err, test.want)
			}
			again, err := normalizeStorefrontPagePath(got)
			if err != nil || again != got {
				t.Fatalf("second normalization = %q/%v, want stable %q", again, err, got)
			}
		})
	}

	for _, invalid := range []string{"", "en/mall", "https://example.com/en", "//example.com/en", "/bad%escape"} {
		if got, err := normalizeStorefrontPagePath(invalid); !errors.Is(err, ErrInvalidInput) || got != "" {
			t.Fatalf("normalizeStorefrontPagePath(%q) = %q/%v, want ErrInvalidInput", invalid, got, err)
		}
	}
}

func TestNormalizeStorefrontPagePathUsesFinalUnicodeCharacterLimit(t *testing.T) {
	for _, character := range []string{"界", "😀"} {
		valid := "/" + strings.Repeat(character, maxStorefrontPagePathLength-1)
		got, err := normalizeStorefrontPagePath(valid)
		if err != nil || got != valid || utf8.RuneCountInString(got) != maxStorefrontPagePathLength {
			t.Fatalf("valid %q boundary normalized to %d runes/%v", character, utf8.RuneCountInString(got), err)
		}
		tooLong := valid + character
		if _, err := normalizeStorefrontPagePath(tooLong); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("%q path above %d runes error = %v, want ErrInvalidInput", character, maxStorefrontPagePathLength, err)
		}
	}

	encodedChinese := "/" + strings.Repeat("%E7%95%8C", maxStorefrontPagePathLength-1)
	got, err := normalizeStorefrontPagePath(encodedChinese)
	if err != nil || got != "/"+strings.Repeat("界", maxStorefrontPagePathLength-1) {
		t.Fatalf("encoded Unicode boundary normalized to %d runes/%v", utf8.RuneCountInString(got), err)
	}
}

func TestNormalizeStorefrontReferrerPreservesUnicodeAndStripsPrivateParts(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: ""},
		{name: "ASCII", input: "https://example.com/en/products/196?utm_source=x#detail", want: "https://example.com/en/products/196"},
		{name: "Unicode", input: "https://example.com/中文/商品?utm_source=test#detail", want: "https://example.com/中文/商品"},
		{name: "encoded Unicode", input: "https://example.com/%E4%B8%AD%E6%96%87?utm=x", want: "https://example.com/中文"},
		{name: "port", input: "http://example.com:8080/a%2Fb?x=1", want: "http://example.com:8080/a%2Fb"},
		{name: "reserved escapes", input: "https://example.com/%3F/%23/%25/%252F", want: "https://example.com/%3F/%23/%25/%252F"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalizeStorefrontReferrer(test.input)
			if err != nil || got != test.want {
				t.Fatalf("normalizeStorefrontReferrer(%q) = %q/%v, want %q", test.input, got, err, test.want)
			}
			again, err := normalizeStorefrontReferrer(got)
			if err != nil || again != got {
				t.Fatalf("second normalization = %q/%v, want stable %q", again, err, got)
			}
		})
	}

	for _, invalid := range []string{
		"ftp://example.com/path",
		"https:///path",
		"https://user:password@example.com/path",
		"https://example.com/bad%escape",
	} {
		if got, err := normalizeStorefrontReferrer(invalid); !errors.Is(err, ErrInvalidInput) || got != "" {
			t.Fatalf("normalizeStorefrontReferrer(%q) = %q/%v, want ErrInvalidInput", invalid, got, err)
		}
	}
}

func TestNormalizeStorefrontReferrerUsesFinalUnicodeCharacterLimit(t *testing.T) {
	prefix := "https://example.com/"
	for _, character := range []string{"界", "😀"} {
		valid := prefix + strings.Repeat(character, maxStorefrontReferrerLength-utf8.RuneCountInString(prefix))
		got, err := normalizeStorefrontReferrer(valid)
		if err != nil || got != valid || utf8.RuneCountInString(got) != maxStorefrontReferrerLength {
			t.Fatalf("valid %q referrer normalized to %d runes/%v", character, utf8.RuneCountInString(got), err)
		}
		if _, err := normalizeStorefrontReferrer(valid + character); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("%q referrer above %d runes error = %v, want ErrInvalidInput", character, maxStorefrontReferrerLength, err)
		}
	}
}

func TestStorefrontEventServiceNormalizesUnicodeBatchBeforeRepositoryWrite(t *testing.T) {
	inputs := make([]StorefrontEventInput, 20)
	for index := range inputs {
		inputs[index] = StorefrontEventInput{
			EventID:    fmt.Sprintf("%08x-aaaa-4aaa-8aaa-%012x", index+1, index+1),
			EventName:  models.StorefrontEventPageView,
			VisitorID:  testVisitorID,
			SessionID:  testSessionID,
			EntityType: models.StorefrontEntityStorefront,
			SourceType: models.StorefrontSourceDirect,
			PageType:   models.StorefrontPageOther,
			PagePath:   fmt.Sprintf("/en/page/%d", index),
			Locale:     "en",
			DeviceType: models.StorefrontDeviceDesktop,
		}
	}
	inputs[len(inputs)-1].PagePath = "/" + strings.Repeat("界", maxStorefrontPagePathLength-1)
	referrerPrefix := "https://example.com/"
	inputs[len(inputs)-1].Referrer = referrerPrefix + strings.Repeat("界", maxStorefrontReferrerLength-utf8.RuneCountInString(referrerPrefix))

	repo := &fakeStorefrontEventRepository{}
	tracker := NewStorefrontEventService(repo)
	result, err := tracker.TrackBatch(context.Background(), StorefrontEventBatchInput{Events: inputs})
	if err != nil || result.Accepted != len(inputs) || len(repo.createdMany) != len(inputs) {
		t.Fatalf("Unicode TrackBatch = %+v/%v, created %d", result, err, len(repo.createdMany))
	}
	last := repo.createdMany[len(repo.createdMany)-1]
	if last.PagePath != inputs[len(inputs)-1].PagePath || last.Referrer != inputs[len(inputs)-1].Referrer {
		t.Fatalf("Unicode persistence values = %q/%q", last.PagePath, last.Referrer)
	}

	invalidRepo := &fakeStorefrontEventRepository{}
	invalidTracker := NewStorefrontEventService(invalidRepo)
	inputs[len(inputs)-1].PagePath += "界"
	result, err = invalidTracker.TrackBatch(context.Background(), StorefrontEventBatchInput{Events: inputs})
	if result != (StorefrontEventBatchResult{}) || !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("over-limit TrackBatch = %+v/%v, want empty result/ErrInvalidInput", result, err)
	}
	if len(invalidRepo.createdMany) != 0 {
		t.Fatalf("over-limit batch reached repository with %d events", len(invalidRepo.createdMany))
	}
}
