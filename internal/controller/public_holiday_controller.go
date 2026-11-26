package controller

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"saas/internal/models"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

const maxPublicHolidayAdminBodyBytes = 64 << 10

type PublicHolidayController struct {
	service publicHolidayService
}

type publicHolidayService interface {
	Calendar(ctx context.Context, query service.HolidayCalendarQuery) (*service.HolidayCalendarResponse, error)
	List(ctx context.Context, query service.PublicHolidayAdminQuery) ([]service.PublicHolidayAdminDTO, error)
	FindByID(ctx context.Context, id uint64) (*service.PublicHolidayAdminDTO, error)
	Create(ctx context.Context, input service.PublicHolidayInput) (*service.PublicHolidayAdminDTO, error)
	Update(ctx context.Context, id uint64, input service.PublicHolidayInput) (*service.PublicHolidayAdminDTO, error)
	Delete(ctx context.Context, id uint64) error
}

type publicHolidayPayload struct {
	CountryCode     string                `json:"country_code"`
	HolidayDate     string                `json:"holiday_date"`
	NameEN          string                `json:"name_en"`
	NameZH          *string               `json:"name_zh"`
	HolidayType     models.HolidayType    `json:"holiday_type"`
	DayType         models.HolidayDayType `json:"day_type"`
	IsOfficial      bool                  `json:"is_official"`
	SourceName      *string               `json:"source_name"`
	SourceReference *string               `json:"source_reference"`
	ObservedForDate *string               `json:"observed_for_date"`
}

func NewPublicHolidayController(service publicHolidayService) *PublicHolidayController {
	return &PublicHolidayController{service: service}
}

func (ctl *PublicHolidayController) RegisterPublicRoutes(group *gin.RouterGroup) {
	group.GET("/storefront/holidays", ctl.Calendar)
}

func (ctl *PublicHolidayController) RegisterSuperRoutes(group *gin.RouterGroup) {
	holidays := group.Group("/holidays")
	holidays.GET("", ctl.List)
	holidays.POST("", ctl.Create)
	holidays.GET("/:id", ctl.GetByID)
	holidays.PUT("/:id", ctl.Update)
	holidays.DELETE("/:id", ctl.Delete)
}

func (ctl *PublicHolidayController) Calendar(c *gin.Context) {
	year, ok := parseOptionalHolidayQueryInt(c, "year")
	if !ok {
		return
	}
	month, ok := parseOptionalHolidayQueryInt(c, "month")
	if !ok {
		return
	}

	calendar, err := ctl.service.Calendar(c.Request.Context(), service.HolidayCalendarQuery{
		CountryCode: c.Query("country"),
		Year:        year,
		Month:       month,
		Locale:      c.Query("locale"),
	})
	if err != nil {
		writeError(c, err)
		return
	}

	// The public calendar contract is deliberately unwrapped so its timezone,
	// today, and selected month are explicit top-level response fields.
	c.JSON(http.StatusOK, calendar)
}

func (ctl *PublicHolidayController) List(c *gin.Context) {
	year, ok := parseOptionalHolidayQueryInt(c, "year")
	if !ok {
		return
	}
	month, ok := parseOptionalHolidayQueryInt(c, "month")
	if !ok {
		return
	}
	items, err := ctl.service.List(c.Request.Context(), service.PublicHolidayAdminQuery{
		CountryCode: c.Query("country"),
		Year:        year,
		Month:       month,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: items})
}

func (ctl *PublicHolidayController) GetByID(c *gin.Context) {
	id, ok := parseHolidayID(c)
	if !ok {
		return
	}
	item, err := ctl.service.FindByID(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: item})
}

func (ctl *PublicHolidayController) Create(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxPublicHolidayAdminBodyBytes)
	var payload publicHolidayPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: "invalid request"})
		return
	}
	item, err := ctl.service.Create(c.Request.Context(), payload.toInput())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, response{Data: item})
}

func (ctl *PublicHolidayController) Update(c *gin.Context) {
	id, ok := parseHolidayID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxPublicHolidayAdminBodyBytes)
	var payload publicHolidayPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: "invalid request"})
		return
	}
	item, err := ctl.service.Update(c.Request.Context(), id, payload.toInput())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: item})
}

func (ctl *PublicHolidayController) Delete(c *gin.Context) {
	id, ok := parseHolidayID(c)
	if !ok {
		return
	}
	if err := ctl.service.Delete(c.Request.Context(), id); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (payload publicHolidayPayload) toInput() service.PublicHolidayInput {
	return service.PublicHolidayInput{
		CountryCode:     payload.CountryCode,
		HolidayDate:     payload.HolidayDate,
		NameEN:          payload.NameEN,
		NameZH:          payload.NameZH,
		HolidayType:     payload.HolidayType,
		DayType:         payload.DayType,
		IsOfficial:      payload.IsOfficial,
		SourceName:      payload.SourceName,
		SourceReference: payload.SourceReference,
		ObservedForDate: payload.ObservedForDate,
	}
}

func parseOptionalHolidayQueryInt(c *gin.Context, name string) (int, bool) {
	value := strings.TrimSpace(c.Query(name))
	if value == "" {
		return 0, true
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		c.JSON(http.StatusBadRequest, response{Error: "invalid " + name})
		return 0, false
	}
	return parsed, true
}

func parseHolidayID(c *gin.Context) (uint64, bool) {
	value, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || value == 0 {
		c.JSON(http.StatusBadRequest, response{Error: "invalid id"})
		return 0, false
	}
	return value, true
}
