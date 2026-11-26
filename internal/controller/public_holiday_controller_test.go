package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"saas/internal/middleware"
	"saas/internal/models"
	"saas/internal/pkg/auth"
	"saas/internal/repository"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type fakePublicHolidayService struct {
	calendarQuery service.HolidayCalendarQuery
	createInput   service.PublicHolidayInput
	updateInput   service.PublicHolidayInput
	updateID      uint64
	findID        uint64
	deleteID      uint64
	listCalls     int
	createCalls   int
}

func (fake *fakePublicHolidayService) Calendar(_ context.Context, query service.HolidayCalendarQuery) (*service.HolidayCalendarResponse, error) {
	fake.calendarQuery = query
	return &service.HolidayCalendarResponse{
		CountryCode: "US", CountryName: "United States", Timezone: service.HolidayBusinessTimezone,
		Year: 2026, Month: 7, Today: "2026-07-16", Items: []service.HolidayCalendarItem{},
	}, nil
}

func (fake *fakePublicHolidayService) List(_ context.Context, _ service.PublicHolidayAdminQuery) ([]service.PublicHolidayAdminDTO, error) {
	fake.listCalls++
	return []service.PublicHolidayAdminDTO{}, nil
}

func (fake *fakePublicHolidayService) FindByID(_ context.Context, id uint64) (*service.PublicHolidayAdminDTO, error) {
	fake.findID = id
	return &service.PublicHolidayAdminDTO{ID: id}, nil
}

func (fake *fakePublicHolidayService) Create(_ context.Context, input service.PublicHolidayInput) (*service.PublicHolidayAdminDTO, error) {
	fake.createCalls++
	fake.createInput = input
	return &service.PublicHolidayAdminDTO{ID: 9, CountryCode: "US", HolidayDate: input.HolidayDate}, nil
}

func (fake *fakePublicHolidayService) Update(_ context.Context, id uint64, input service.PublicHolidayInput) (*service.PublicHolidayAdminDTO, error) {
	fake.updateID = id
	fake.updateInput = input
	return &service.PublicHolidayAdminDTO{ID: id, CountryCode: "US", HolidayDate: input.HolidayDate}, nil
}

func (fake *fakePublicHolidayService) Delete(_ context.Context, id uint64) error {
	fake.deleteID = id
	return nil
}

func TestPublicHolidayCalendarReturnsTopLevelAccraContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakePublicHolidayService{}
	router := gin.New()
	NewPublicHolidayController(fake).RegisterPublicRoutes(router.Group("/api/v1"))

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/storefront/holidays", nil)
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if payload["country_code"] != "US" || payload["timezone"] != "Africa/Accra" || payload["today"] != "2026-07-16" {
		t.Fatalf("calendar payload = %+v", payload)
	}
	if _, wrapped := payload["data"]; wrapped {
		t.Fatalf("public holiday response must not be wrapped: %+v", payload)
	}
	if fake.calendarQuery.CountryCode != "" || fake.calendarQuery.Year != 0 || fake.calendarQuery.Month != 0 || fake.calendarQuery.Locale != "" {
		t.Fatalf("controller replaced service defaults: %+v", fake.calendarQuery)
	}
}

func TestPublicHolidaySuperRoutesAllowCRUDAndForbidTenantAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakePublicHolidayService{}
	router := gin.New()
	group := router.Group("/api/v1/super")
	group.Use(requireControllerTestAuth("secret"))
	group.Use(middleware.RequireRole(models.UserRoleSuperAdmin))
	NewPublicHolidayController(fake).RegisterSuperRoutes(group)

	superToken := holidayControllerToken(t, models.UserRoleSuperAdmin, nil)
	payload := `{"country_code":"US","holiday_date":"2026-07-03","name_en":"Holiday","holiday_type":"observed","day_type":"public_holiday","is_official":true,"observed_for_date":"2026-07-04"}`
	cases := []struct {
		method string
		path   string
		body   string
		status int
	}{
		{http.MethodGet, "/api/v1/super/holidays", "", http.StatusOK},
		{http.MethodPost, "/api/v1/super/holidays", payload, http.StatusCreated},
		{http.MethodGet, "/api/v1/super/holidays/9", "", http.StatusOK},
		{http.MethodPut, "/api/v1/super/holidays/9", payload, http.StatusOK},
		{http.MethodDelete, "/api/v1/super/holidays/9", "", http.StatusNoContent},
	}
	for _, testCase := range cases {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(testCase.method, testCase.path, strings.NewReader(testCase.body))
		request.Header.Set("Authorization", "Bearer "+superToken)
		if testCase.body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		router.ServeHTTP(response, request)
		if response.Code != testCase.status {
			t.Fatalf("%s %s status = %d, want %d, body = %s", testCase.method, testCase.path, response.Code, testCase.status, response.Body.String())
		}
	}
	if fake.listCalls != 1 || fake.createCalls != 1 || fake.findID != 9 || fake.updateID != 9 || fake.deleteID != 9 {
		t.Fatalf("CRUD calls = %+v", fake)
	}
	if fake.createInput.ObservedForDate == nil || *fake.createInput.ObservedForDate != "2026-07-04" {
		t.Fatalf("observed input = %+v", fake.createInput)
	}

	tenantID := uint(88)
	tenantToken := holidayControllerToken(t, models.UserRoleTenantAdmin, &tenantID)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/super/holidays", strings.NewReader(payload))
	request.Header.Set("Authorization", "Bearer "+tenantToken)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("tenant admin status = %d, want forbidden", response.Code)
	}
	if fake.createCalls != 1 {
		t.Fatalf("tenant admin reached Create, calls = %d", fake.createCalls)
	}
}

func TestPublicHolidayAPIShowsImported2026USMarkersAndLocalizedNames(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(
		sqlite.Open(fmt.Sprintf("file:controller-holiday-import-%s?mode=memory&cache=shared", t.Name())),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)},
	)
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	if err := db.AutoMigrate(&models.PublicHoliday{}); err != nil {
		t.Fatalf("migrate PublicHoliday: %v", err)
	}
	data, err := os.ReadFile("../../deploy/holiday-data/us-2026.example.json")
	if err != nil {
		t.Fatalf("read example holiday data: %v", err)
	}
	if _, err := service.NewHolidayImportService(db).ImportJSON(t.Context(), data, service.HolidayImportOptions{CountryCode: "US", Apply: true}); err != nil {
		t.Fatalf("apply example holidays: %v", err)
	}
	holidayService := service.NewPublicHolidayService(
		repository.NewPublicHolidayRepository(db),
		service.PublicHolidayServiceOptions{Now: func() time.Time {
			return time.Date(2026, time.July, 2, 12, 0, 0, 0, time.UTC)
		}},
	)
	router := gin.New()
	NewPublicHolidayController(holidayService).RegisterPublicRoutes(router.Group("/api/v1"))

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/storefront/holidays?country=US&year=2026&month=7&locale=zh", nil)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var calendar service.HolidayCalendarResponse
	if err := json.Unmarshal(response.Body.Bytes(), &calendar); err != nil {
		t.Fatalf("decode calendar: %v", err)
	}
	if calendar.CountryCode != "US" || calendar.Timezone != service.HolidayBusinessTimezone || calendar.Today != "2026-07-02" || len(calendar.Items) != 2 {
		t.Fatalf("calendar = %+v", calendar)
	}
	if calendar.Items[0].HolidayType != models.HolidayTypeObserved || calendar.Items[0].Name != "美国独立日" || calendar.Items[0].ObservedForDate == nil {
		t.Fatalf("observed marker = %+v", calendar.Items[0])
	}
	if calendar.Items[1].HolidayType != models.HolidayTypeFederal || calendar.Items[1].NameEN != "Independence Day" {
		t.Fatalf("federal marker = %+v", calendar.Items[1])
	}
}

func holidayControllerToken(t *testing.T, role models.UserRole, tenantID *uint) string {
	t.Helper()
	user := &models.User{BaseModel: models.BaseModel{ID: 1}, TenantID: tenantID, Role: role}
	token, err := auth.GenerateToken("secret", time.Hour, user)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	return token
}
