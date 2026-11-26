package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"saas/internal/models"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type fakeControllerStorefrontEventRepository struct {
	productTenant   uint
	created         *models.StorefrontEvent
	createdMany     []*models.StorefrontEvent
	createErr       error
	createManyErr   error
	batchAccepted   int
	batchDuplicates int
}

func (repo *fakeControllerStorefrontEventRepository) FindExistingEventIDs(_ context.Context, _ []string) (map[string]struct{}, error) {
	return map[string]struct{}{}, nil
}

func (repo *fakeControllerStorefrontEventRepository) CreateMany(_ context.Context, events []*models.StorefrontEvent) (int, int, error) {
	if repo.createManyErr != nil {
		return 0, 0, repo.createManyErr
	}
	repo.createdMany = make([]*models.StorefrontEvent, len(events))
	for index, event := range events {
		clone := *event
		repo.createdMany[index] = &clone
	}
	if repo.batchAccepted != 0 || repo.batchDuplicates != 0 {
		return repo.batchAccepted, repo.batchDuplicates, nil
	}
	return len(events), 0, nil
}

func (repo *fakeControllerStorefrontEventRepository) Create(_ context.Context, event *models.StorefrontEvent) error {
	if repo.createErr != nil {
		return repo.createErr
	}
	clone := *event
	repo.created = &clone
	return nil
}

func (repo *fakeControllerStorefrontEventRepository) FindPublicProductTenantID(_ context.Context, productID uint) (uint, error) {
	if productID != 42 || repo.productTenant == 0 {
		return 0, gorm.ErrRecordNotFound
	}
	return repo.productTenant, nil
}

func (repo *fakeControllerStorefrontEventRepository) FindPublicShopTenantID(_ context.Context, _ string) (uint, error) {
	return 0, gorm.ErrRecordNotFound
}

func (repo *fakeControllerStorefrontEventRepository) FindPlatformMallTenantID(_ context.Context) (*uint, error) {
	return nil, nil
}

func storefrontEventRequestBody(overrides map[string]any) string {
	body := map[string]any{
		"event_id":    "00000000-0000-4000-8000-000000000001",
		"event_name":  "product_click",
		"visitor_id":  "11111111-1111-4111-8111-111111111111",
		"session_id":  "22222222-2222-4222-8222-222222222222",
		"product_id":  42,
		"source_type": "mall",
		"source_id":   "all",
		"page_type":   "mall",
		"page_path":   "/en/mall",
		"locale":      "en",
		"device_type": "desktop",
	}
	for key, value := range overrides {
		body[key] = value
	}
	encoded, _ := json.Marshal(body)
	return string(encoded)
}

func newStorefrontEventTestRouter(repo *fakeControllerStorefrontEventRepository, authDisabled bool) *gin.Engine {
	router := gin.New()
	group := router.Group("/api/v1")
	if authDisabled {
		NewCustomerAuthController(nil, "secret", false).RegisterRoutes(group)
	}
	NewStorefrontEventController(service.NewStorefrontEventService(repo)).RegisterRoutes(group)
	return router
}

func postStorefrontEvent(router http.Handler, body string) *httptest.ResponseRecorder {
	return postStorefrontEventWithUserAgent(router, body, "")
}

func postStorefrontEventWithUserAgent(router http.Handler, body string, userAgent string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/storefront/events", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", userAgent)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestStorefrontEventClassifiesOnlyObviousBotUserAgents(t *testing.T) {
	tests := []struct {
		name      string
		userAgent string
		want      models.StorefrontTrafficType
	}{
		{name: "browser", userAgent: "Mozilla/5.0 Chrome/126 Safari/537.36", want: models.StorefrontTrafficHuman},
		{name: "empty", userAgent: "", want: models.StorefrontTrafficHuman},
		{name: "curl", userAgent: "curl/8.9.1", want: models.StorefrontTrafficHuman},
		{name: "google bot", userAgent: "Mozilla/5.0 (compatible; Googlebot/2.1)", want: models.StorefrontTrafficBot},
		{name: "crawler", userAgent: "ExampleCrawler/1.0", want: models.StorefrontTrafficBot},
		{name: "headless chrome", userAgent: "HeadlessChrome/126.0", want: models.StorefrontTrafficBot},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := classifyStorefrontTraffic(test.userAgent); got != test.want {
				t.Fatalf("classifyStorefrontTraffic(%q) = %q, want %q", test.userAgent, got, test.want)
			}
		})
	}
}

func TestStorefrontEventTrafficTypeComesFromUserAgentNotBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeControllerStorefrontEventRepository{productTenant: 7}
	router := newStorefrontEventTestRouter(repo, false)
	response := postStorefrontEventWithUserAgent(
		router,
		storefrontEventRequestBody(map[string]any{"traffic_type": "bot"}),
		"Mozilla/5.0 Chrome/126 Safari/537.36",
	)
	if response.Code != http.StatusAccepted || repo.created == nil || repo.created.TrafficType != models.StorefrontTrafficHuman {
		t.Fatalf("normal UA response/event = %d/%+v, want human", response.Code, repo.created)
	}
	response = postStorefrontEventWithUserAgent(
		router,
		storefrontEventRequestBody(map[string]any{"event_id": "00000000-0000-4000-8000-000000000002", "traffic_type": "human"}),
		"Googlebot/2.1",
	)
	if response.Code != http.StatusAccepted || repo.created == nil || repo.created.TrafficType != models.StorefrontTrafficBot {
		t.Fatalf("bot UA response/event = %d/%+v, want bot", response.Code, repo.created)
	}
}

func postStorefrontEventBatch(router http.Handler, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/storefront/events/batch", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func storefrontEventBatchRequestBody(count int, mutate func(int, map[string]any)) string {
	events := make([]map[string]any, count)
	for index := range events {
		var event map[string]any
		_ = json.Unmarshal([]byte(storefrontEventRequestBody(nil)), &event)
		event["event_id"] = fmt.Sprintf("00000000-0000-4000-8000-%012x", index+1)
		if mutate != nil {
			mutate(index, event)
		}
		events[index] = event
	}
	encoded, _ := json.Marshal(map[string]any{"events": events})
	return string(encoded)
}

func TestStorefrontEventBodyTenantCannotOverrideProductTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeControllerStorefrontEventRepository{productTenant: 7}
	router := newStorefrontEventTestRouter(repo, false)

	response := postStorefrontEvent(router, storefrontEventRequestBody(map[string]any{"tenant_id": 999}))
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusAccepted, response.Body.String())
	}
	if repo.created == nil || repo.created.TenantID == nil || *repo.created.TenantID != 7 {
		t.Fatalf("created tenant = %v, want product tenant 7", repo.created)
	}
}

func TestStorefrontEventRejectsInvalidEventName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeControllerStorefrontEventRepository{productTenant: 7}
	router := newStorefrontEventTestRouter(repo, false)

	response := postStorefrontEvent(router, storefrontEventRequestBody(map[string]any{"event_name": "purchase"}))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
	if repo.created != nil {
		t.Fatal("invalid event was inserted")
	}
}

func TestStorefrontEventRejectsMissingEventID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeControllerStorefrontEventRepository{productTenant: 7}
	response := postStorefrontEvent(newStorefrontEventTestRouter(repo, false), storefrontEventRequestBody(map[string]any{"event_id": ""}))
	if response.Code != http.StatusBadRequest || repo.created != nil {
		t.Fatalf("missing event_id response/event = %d/%+v, want 400/no event", response.Code, repo.created)
	}
}

func TestStorefrontEventRejectsOversizedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeControllerStorefrontEventRepository{productTenant: 7}
	router := newStorefrontEventTestRouter(repo, false)
	body := `{"padding":"` + strings.Repeat("x", int(maxStorefrontEventBodyBytes)) + `"}`

	response := postStorefrontEvent(router, body)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
}

func TestStorefrontEventRejectsMalformedAndInvalidUTF8JSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name string
		body string
	}{
		{name: "empty", body: ""},
		{name: "whitespace", body: " \t\r\n"},
		{name: "malformed", body: `{"event_id":`},
		{name: "trailing JSON", body: storefrontEventRequestBody(nil) + `{}`},
		{name: "invalid UTF-8", body: strings.Replace(storefrontEventRequestBody(nil), "/en/mall", "/en/\xff", 1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &fakeControllerStorefrontEventRepository{productTenant: 7}
			response := postStorefrontEvent(newStorefrontEventTestRouter(repo, false), test.body)
			if response.Code != http.StatusBadRequest || repo.created != nil {
				t.Fatalf("response/event = %d/%+v, want 400/no event: %s", response.Code, repo.created, response.Body.String())
			}
		})
	}
}

func TestMalformedStorefrontEventConsumesRequestLimitBeforeDecode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeControllerStorefrontEventRepository{productTenant: 7}
	controller := NewStorefrontEventController(service.NewStorefrontEventService(repo))
	router := gin.New()
	controller.RegisterRoutes(router.Group("/api/v1"))

	response := postStorefrontEvent(router, `{"event_id":`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("first malformed response = %d, want 400", response.Code)
	}
	if controller.limiter.requestCount != 1 || controller.limiter.eventCount != 0 {
		t.Fatalf("malformed limiter counts = requests %d/events %d, want 1/0", controller.limiter.requestCount, controller.limiter.eventCount)
	}

	controller.limiter.requestCount = storefrontEventGlobalRequestLimit
	response = postStorefrontEvent(router, `{"still":"malformed"`)
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") != "60" {
		t.Fatalf("limited malformed response = %d headers=%v, want 429/Retry-After", response.Code, response.Header())
	}
}

func TestStorefrontEventDatabaseErrorDoesNotLeakDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secretDatabaseError := errors.New(`insert into storefront_events failed: constraint "idx_storefront_events_product_occurred"`)
	repo := &fakeControllerStorefrontEventRepository{productTenant: 7, createErr: secretDatabaseError}
	router := newStorefrontEventTestRouter(repo, false)

	response := postStorefrontEvent(router, storefrontEventRequestBody(nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusInternalServerError, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "storefront_events") || strings.Contains(response.Body.String(), "constraint") || strings.Contains(response.Body.String(), "insert") {
		t.Fatalf("response leaked database details: %s", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "Internal server error") {
		t.Fatalf("response = %s, want generic internal error", response.Body.String())
	}
}

func TestStorefrontTrackingWorksWhenStorefrontAuthDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeControllerStorefrontEventRepository{productTenant: 7}
	router := newStorefrontEventTestRouter(repo, true)

	response := postStorefrontEvent(router, storefrontEventRequestBody(nil))
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d with storefront auth disabled, body = %s", response.Code, http.StatusAccepted, response.Body.String())
	}
	if repo.created == nil {
		t.Fatal("event was not inserted with storefront auth disabled")
	}
}

func TestStorefrontEventRateLimiterNormalizesVisitorAndEnforcesLimit(t *testing.T) {
	limiter := newStorefrontEventRateLimiter()
	now := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)
	for index := 0; index < storefrontEventVisitorLimit; index++ {
		visitorID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
		if index%2 == 1 {
			visitorID = "  AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA  "
		}
		if !limiter.Allow(visitorID, now) {
			t.Fatalf("request %d was rejected before visitor limit", index+1)
		}
	}
	if limiter.Allow("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", now) {
		t.Fatal("request above visitor limit was accepted")
	}
	if !limiter.Allow("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", now.Add(storefrontEventRateLimitWindow)) {
		t.Fatal("visitor remained limited after the rate limit window")
	}
}

func TestStorefrontEventBatchEndpointAcceptsOneAndFiftyEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, count := range []int{1, service.MaxStorefrontEventBatchSize} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			repo := &fakeControllerStorefrontEventRepository{productTenant: 7}
			router := newStorefrontEventTestRouter(repo, false)
			response := postStorefrontEventBatch(router, storefrontEventBatchRequestBody(count, func(index int, event map[string]any) {
				event["visitor_id"] = fmt.Sprintf("%08x-1111-4111-8111-%012x", index+1, index+1)
			}))
			if response.Code != http.StatusAccepted {
				t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusAccepted, response.Body.String())
			}
			if len(repo.createdMany) != count || !strings.Contains(response.Body.String(), fmt.Sprintf(`"accepted":%d`, count)) || !strings.Contains(response.Body.String(), `"rejected":0`) {
				t.Fatalf("created/response = %d/%s, want %d accepted", len(repo.createdMany), response.Body.String(), count)
			}
		})
	}
}

func TestStorefrontEventBatchResponseReportsDuplicates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeControllerStorefrontEventRepository{productTenant: 7, batchAccepted: 1, batchDuplicates: 1}
	response := postStorefrontEventBatch(newStorefrontEventTestRouter(repo, false), storefrontEventBatchRequestBody(2, nil))
	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"accepted":1`) ||
		!strings.Contains(response.Body.String(), `"duplicates":1`) || !strings.Contains(response.Body.String(), `"rejected":0`) {
		t.Fatalf("batch idempotency response = %d %s", response.Code, response.Body.String())
	}
}

func TestStorefrontEventBatchRejectsInvalidSizesAndMiddleEventAtomically(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name string
		body string
	}{
		{name: "empty", body: `{"events":[]}`},
		{name: "too many", body: storefrontEventBatchRequestBody(service.MaxStorefrontEventBatchSize+1, nil)},
		{name: "invalid middle", body: storefrontEventBatchRequestBody(3, func(index int, event map[string]any) {
			if index == 1 {
				event["event_name"] = "purchase"
			}
		})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &fakeControllerStorefrontEventRepository{productTenant: 7}
			response := postStorefrontEventBatch(newStorefrontEventTestRouter(repo, false), test.body)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body = %s", response.Code, response.Body.String())
			}
			if len(repo.createdMany) != 0 {
				t.Fatalf("invalid batch inserted %d events", len(repo.createdMany))
			}
		})
	}
}

func TestStorefrontEventBatchIgnoresBodyTenantAndWorksWithAuthDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeControllerStorefrontEventRepository{productTenant: 7}
	body := storefrontEventBatchRequestBody(1, func(_ int, event map[string]any) { event["tenant_id"] = 999 })
	response := postStorefrontEventBatch(newStorefrontEventTestRouter(repo, true), body)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202, body = %s", response.Code, response.Body.String())
	}
	if len(repo.createdMany) != 1 || repo.createdMany[0].TenantID == nil || *repo.createdMany[0].TenantID != 7 {
		t.Fatalf("created tenant = %+v, want derived tenant 7", repo.createdMany)
	}
}

func TestStorefrontEventBatchRejectsOversizedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeControllerStorefrontEventRepository{productTenant: 7}
	body := `{"events":[],"padding":"` + strings.Repeat("x", int(maxStorefrontEventBatchBodyBytes)) + `"}`
	response := postStorefrontEventBatch(newStorefrontEventTestRouter(repo, false), body)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", response.Code, response.Body.String())
	}
}

const storefrontEventClientBatchByteBudgetForTest = 48 * 1024

func maximumUnicodeStorefrontEventInput(index int) service.StorefrontEventInput {
	referrerPrefix := "https://example.com/"
	return service.StorefrontEventInput{
		EventID:    fmt.Sprintf("%08x-aaaa-4aaa-8aaa-%012x", index, index),
		EventName:  models.StorefrontEventPageView,
		VisitorID:  "11111111-1111-4111-8111-111111111111",
		SessionID:  "22222222-2222-4222-8222-222222222222",
		EntityType: models.StorefrontEntityStorefront,
		EntityID:   fmt.Sprintf("unicode-%d", index),
		SourceType: models.StorefrontSourceDirect,
		SourceID:   fmt.Sprintf("unicode-%d", index),
		PageType:   models.StorefrontPageOther,
		PagePath:   "/" + strings.Repeat("界", 511),
		Locale:     "zh",
		Referrer:   referrerPrefix + strings.Repeat("界", 1024-utf8.RuneCountInString(referrerPrefix)),
		DeviceType: models.StorefrontDeviceDesktop,
	}
}

func marshalStorefrontEventBatchForTest(t *testing.T, events []service.StorefrontEventInput) []byte {
	t.Helper()
	body, err := json.Marshal(service.StorefrontEventBatchInput{Events: events})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func splitStorefrontEventBatchByBytesForTest(t *testing.T, events []service.StorefrontEventInput) [][]service.StorefrontEventInput {
	t.Helper()
	var batches [][]service.StorefrontEventInput
	var current []service.StorefrontEventInput
	for _, event := range events {
		candidate := append(append([]service.StorefrontEventInput{}, current...), event)
		if len(candidate) > 20 || len(marshalStorefrontEventBatchForTest(t, candidate)) > storefrontEventClientBatchByteBudgetForTest {
			if len(current) > 0 {
				batches = append(batches, current)
			}
			current = []service.StorefrontEventInput{event}
			continue
		}
		current = candidate
	}
	if len(current) > 0 {
		batches = append(batches, current)
	}
	return batches
}

// This test exercises the HTTP/controller/service contract with a fake
// repository. PostgreSQL varchar persistence is covered separately by the
// opt-in PostgreSQL integration test in the service package.
func TestStorefrontEventUnicodeBatchFitsControllerServiceContractWhenByteSplit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	events := make([]service.StorefrontEventInput, 20)
	expectedIDs := make([]string, len(events))
	for index := range events {
		events[index] = maximumUnicodeStorefrontEventInput(index + 1)
		expectedIDs[index] = events[index].EventID
		if utf8.RuneCountInString(events[index].PagePath) != 512 || utf8.RuneCountInString(events[index].Referrer) != 1024 {
			t.Fatalf("event %d does not meet maximum rune contract", index)
		}
	}

	oversizedBody := marshalStorefrontEventBatchForTest(t, events)
	if int64(len(oversizedBody)) <= maxStorefrontEventBatchBodyBytes {
		t.Fatalf("unsplit Unicode body = %d bytes, want > %d", len(oversizedBody), maxStorefrontEventBatchBodyBytes)
	}
	repo := &fakeControllerStorefrontEventRepository{}
	router := newStorefrontEventTestRouter(repo, false)
	response := postStorefrontEventBatch(router, string(oversizedBody))
	if response.Code != http.StatusBadRequest || len(repo.createdMany) != 0 {
		t.Fatalf("unsplit response/rows = %d/%d, want 400/0", response.Code, len(repo.createdMany))
	}

	batches := splitStorefrontEventBatchByBytesForTest(t, events)
	if len(batches) <= 1 {
		t.Fatalf("byte-safe batch count = %d, want > 1", len(batches))
	}
	var normalizedIDs []string
	for index, batch := range batches {
		body := marshalStorefrontEventBatchForTest(t, batch)
		if len(body) > storefrontEventClientBatchByteBudgetForTest {
			t.Fatalf("batch %d = %d bytes, want <= %d", index, len(body), storefrontEventClientBatchByteBudgetForTest)
		}
		response = postStorefrontEventBatch(router, string(body))
		if response.Code != http.StatusAccepted || len(repo.createdMany) != len(batch) {
			t.Fatalf("batch %d response/rows = %d/%d, want 202/%d: %s", index, response.Code, len(repo.createdMany), len(batch), response.Body.String())
		}
		for _, event := range repo.createdMany {
			normalizedIDs = append(normalizedIDs, event.EventID)
			if strings.Contains(event.PagePath, "%E7") || strings.Contains(event.Referrer, "%E7") {
				t.Fatalf("batch %d retained percent-expanded Unicode: %q/%q", index, event.PagePath, event.Referrer)
			}
		}
	}
	if !slices.Equal(normalizedIDs, expectedIDs) {
		t.Fatalf("normalized IDs = %v, want %v", normalizedIDs, expectedIDs)
	}
}

func TestStorefrontEventBatchRejectsFinalUnicodeCharacterOverflowBeforeRepository(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeControllerStorefrontEventRepository{}
	body := storefrontEventBatchRequestBody(20, func(index int, event map[string]any) {
		event["event_name"] = string(models.StorefrontEventPageView)
		event["product_id"] = nil
		event["entity_type"] = string(models.StorefrontEntityStorefront)
		event["entity_id"] = nil
		event["source_type"] = string(models.StorefrontSourceDirect)
		event["source_id"] = nil
		event["page_type"] = string(models.StorefrontPageOther)
		if index == 19 {
			event["page_path"] = "/" + strings.Repeat("界", 512)
		}
	})
	response := postStorefrontEventBatch(newStorefrontEventTestRouter(repo, false), body)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", response.Code, response.Body.String())
	}
	if len(repo.createdMany) != 0 {
		t.Fatalf("over-limit batch reached fake repository with %d events", len(repo.createdMany))
	}
}

func TestStorefrontEventBatchDatabaseErrorDoesNotLeakDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := errors.New(`insert into storefront_events failed: constraint "batch_secret"`)
	repo := &fakeControllerStorefrontEventRepository{productTenant: 7, createManyErr: secret}
	response := postStorefrontEventBatch(newStorefrontEventTestRouter(repo, false), storefrontEventBatchRequestBody(1, nil))
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "storefront_events") || strings.Contains(response.Body.String(), "constraint") {
		t.Fatalf("batch error response leaked details or wrong status: %d %s", response.Code, response.Body.String())
	}
}

func TestStorefrontEventRateLimiterAccountsForBatchAtomically(t *testing.T) {
	limiter := newStorefrontEventRateLimiter()
	now := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	visitors := make([]string, storefrontEventVisitorLimit)
	for index := range visitors {
		visitors[index] = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	}
	if !limiter.AllowMany(visitors, now) {
		t.Fatal("batch at visitor event limit was rejected")
	}
	if limiter.AllowMany([]string{"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}, now) {
		t.Fatal("batch above visitor event limit was accepted")
	}
}

func TestStorefrontEventRateLimiterEnforcesGlobalRequestAndEventVolumeLimits(t *testing.T) {
	now := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	requestLimiter := newStorefrontEventRateLimiter()
	for index := 0; index < storefrontEventGlobalRequestLimit; index++ {
		visitor := fmt.Sprintf("request-visitor-%d", index)
		if !requestLimiter.Allow(visitor, now) {
			t.Fatalf("global request %d rejected before limit", index+1)
		}
	}
	if requestLimiter.Allow("request-over-limit", now) {
		t.Fatal("request above global request limit was accepted")
	}

	eventLimiter := newStorefrontEventRateLimiter()
	batch := make([]string, service.MaxStorefrontEventBatchSize)
	batchNumber := 0
	for acceptedEvents := 0; acceptedEvents < storefrontEventGlobalEventLimit; acceptedEvents += len(batch) {
		for index := range batch {
			batch[index] = fmt.Sprintf("volume-%d-%d", batchNumber, index)
		}
		if !eventLimiter.AllowMany(batch, now) {
			t.Fatalf("event batch %d rejected before volume limit", batchNumber+1)
		}
		batchNumber++
	}
	if eventLimiter.Allow("volume-over-limit", now) {
		t.Fatal("event above global event-volume limit was accepted")
	}
	if len(eventLimiter.visitors) != storefrontEventGlobalEventLimit {
		t.Fatalf("visitor limiter map = %d, want bounded maximum %d", len(eventLimiter.visitors), storefrontEventGlobalEventLimit)
	}
	if !eventLimiter.Allow("new-window", now.Add(storefrontEventRateLimitWindow)) {
		t.Fatal("new rate-limit window did not accept an event")
	}
	if len(eventLimiter.visitors) != 1 {
		t.Fatalf("visitor limiter map after reset = %d, want 1", len(eventLimiter.visitors))
	}
}

func TestStorefrontEventRateLimiterIsConcurrentAndAtomic(t *testing.T) {
	limiter := newStorefrontEventRateLimiter()
	now := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	const workers = 600
	accepted := make(chan bool, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			accepted <- limiter.Allow(fmt.Sprintf("concurrent-%d", index), now)
		}(index)
	}
	wait.Wait()
	close(accepted)
	acceptedCount := 0
	for value := range accepted {
		if value {
			acceptedCount++
		}
	}
	if acceptedCount != workers || limiter.requestCount != workers || limiter.eventCount != workers || len(limiter.visitors) != workers {
		t.Fatalf("concurrent limiter = accepted %d/requests %d/events %d/visitors %d, want %d each",
			acceptedCount, limiter.requestCount, limiter.eventCount, len(limiter.visitors), workers)
	}
}
