package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"saas/internal/models"
	"saas/internal/pkg/locale"

	"gorm.io/gorm"
)

const (
	HolidayBusinessTimezone   = "Africa/Accra"
	DefaultHolidayCountryCode = "US"
	DefaultHolidayCountryName = "United States"
	HolidayCalendarDateLayout = "2006-01-02"
)

type publicHolidayRepository interface {
	ListOfficialByDateRange(ctx context.Context, countryCode string, from time.Time, to time.Time) ([]models.PublicHoliday, error)
	FindNextOfficial(ctx context.Context, countryCode string, from time.Time) (*models.PublicHoliday, error)
	List(ctx context.Context, countryCode string, from *time.Time, to *time.Time) ([]models.PublicHoliday, error)
	FindByID(ctx context.Context, id uint64) (*models.PublicHoliday, error)
	Create(ctx context.Context, holiday *models.PublicHoliday) error
	Update(ctx context.Context, holiday *models.PublicHoliday) error
	Delete(ctx context.Context, id uint64) error
}

type PublicHolidayService struct {
	repo publicHolidayRepository
	now  func() time.Time
}

type PublicHolidayServiceOptions struct {
	Now func() time.Time
}

type HolidayCalendarQuery struct {
	CountryCode string
	Year        int
	Month       int
	Locale      string
}

type HolidayCalendarItem struct {
	Date            string                `json:"date"`
	Name            string                `json:"name"`
	NameEN          string                `json:"name_en"`
	NameZH          *string               `json:"name_zh"`
	HolidayType     models.HolidayType    `json:"holiday_type"`
	DayType         models.HolidayDayType `json:"day_type"`
	ObservedForDate *string               `json:"observed_for_date"`
	IsOfficial      bool                  `json:"is_official"`
}

type HolidayCalendarResponse struct {
	CountryCode string                `json:"country_code"`
	CountryName string                `json:"country_name"`
	Timezone    string                `json:"timezone"`
	Year        int                   `json:"year"`
	Month       int                   `json:"month"`
	Today       string                `json:"today"`
	Items       []HolidayCalendarItem `json:"items"`
	NextHoliday *HolidayCalendarItem  `json:"next_holiday"`
}

type PublicHolidayInput struct {
	CountryCode     string
	HolidayDate     string
	NameEN          string
	NameZH          *string
	HolidayType     models.HolidayType
	DayType         models.HolidayDayType
	IsOfficial      bool
	SourceName      *string
	SourceReference *string
	ObservedForDate *string
}

type PublicHolidayAdminQuery struct {
	CountryCode string
	Year        int
	Month       int
}

type PublicHolidayAdminDTO struct {
	ID              uint64                `json:"id"`
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
	CreatedAt       time.Time             `json:"created_at"`
	UpdatedAt       time.Time             `json:"updated_at"`
}

func NewPublicHolidayService(repo publicHolidayRepository, options ...PublicHolidayServiceOptions) *PublicHolidayService {
	service := &PublicHolidayService{repo: repo, now: time.Now}
	if len(options) > 0 && options[0].Now != nil {
		service.now = options[0].Now
	}
	return service
}

func (s *PublicHolidayService) Calendar(ctx context.Context, query HolidayCalendarQuery) (*HolidayCalendarResponse, error) {
	countryCode, ok := normalizeHolidayCountryCode(query.CountryCode, true)
	if !ok {
		return nil, ErrInvalidInput
	}

	location, err := time.LoadLocation(HolidayBusinessTimezone)
	if err != nil {
		return nil, err
	}
	businessNow := s.now().In(location)
	year, month := query.Year, query.Month
	if year == 0 {
		year = businessNow.Year()
	}
	if month == 0 {
		month = int(businessNow.Month())
	}
	if year < 1 || year > 9999 || month < 1 || month > 12 {
		return nil, ErrInvalidInput
	}

	from := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, 0)
	holidays, err := s.repo.ListOfficialByDateRange(ctx, countryCode, from, to)
	if err != nil {
		return nil, err
	}

	normalizedLocale := locale.NormalizeLocale(query.Locale)
	items := make([]HolidayCalendarItem, 0, len(holidays))
	for i := range holidays {
		items = append(items, toHolidayCalendarItem(&holidays[i], normalizedLocale))
	}

	todayDate := time.Date(businessNow.Year(), businessNow.Month(), businessNow.Day(), 0, 0, 0, 0, time.UTC)
	next, err := s.repo.FindNextOfficial(ctx, countryCode, todayDate)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	var nextItem *HolidayCalendarItem
	if next != nil {
		item := toHolidayCalendarItem(next, normalizedLocale)
		nextItem = &item
	}

	return &HolidayCalendarResponse{
		CountryCode: countryCode,
		CountryName: holidayCountryName(countryCode),
		Timezone:    HolidayBusinessTimezone,
		Year:        year,
		Month:       month,
		Today:       todayDate.Format(HolidayCalendarDateLayout),
		Items:       items,
		NextHoliday: nextItem,
	}, nil
}

func (s *PublicHolidayService) List(ctx context.Context, query PublicHolidayAdminQuery) ([]PublicHolidayAdminDTO, error) {
	countryCode := ""
	if strings.TrimSpace(query.CountryCode) != "" {
		var ok bool
		countryCode, ok = normalizeHolidayCountryCode(query.CountryCode, false)
		if !ok {
			return nil, ErrInvalidInput
		}
	}
	if query.Year < 0 || query.Year > 9999 || query.Month < 0 || query.Month > 12 || (query.Month > 0 && query.Year == 0) {
		return nil, ErrInvalidInput
	}

	var from, to *time.Time
	if query.Year > 0 {
		month := time.January
		months := 12
		if query.Month > 0 {
			month = time.Month(query.Month)
			months = 1
		}
		start := time.Date(query.Year, month, 1, 0, 0, 0, 0, time.UTC)
		end := start.AddDate(0, months, 0)
		from, to = &start, &end
	}

	holidays, err := s.repo.List(ctx, countryCode, from, to)
	if err != nil {
		return nil, err
	}
	items := make([]PublicHolidayAdminDTO, 0, len(holidays))
	for i := range holidays {
		items = append(items, toPublicHolidayAdminDTO(&holidays[i]))
	}
	return items, nil
}

func (s *PublicHolidayService) FindByID(ctx context.Context, id uint64) (*PublicHolidayAdminDTO, error) {
	if id == 0 {
		return nil, ErrInvalidInput
	}
	holiday, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	dto := toPublicHolidayAdminDTO(holiday)
	return &dto, nil
}

func (s *PublicHolidayService) Create(ctx context.Context, input PublicHolidayInput) (*PublicHolidayAdminDTO, error) {
	holiday, err := publicHolidayFromInput(input)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, holiday); err != nil {
		return nil, mapPublicHolidayPersistenceError(err)
	}
	dto := toPublicHolidayAdminDTO(holiday)
	return &dto, nil
}

func (s *PublicHolidayService) Update(ctx context.Context, id uint64, input PublicHolidayInput) (*PublicHolidayAdminDTO, error) {
	if id == 0 {
		return nil, ErrInvalidInput
	}
	holiday, err := publicHolidayFromInput(input)
	if err != nil {
		return nil, err
	}
	holiday.ID = id
	if err := s.repo.Update(ctx, holiday); err != nil {
		return nil, mapPublicHolidayPersistenceError(err)
	}
	updated, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	dto := toPublicHolidayAdminDTO(updated)
	return &dto, nil
}

func (s *PublicHolidayService) Delete(ctx context.Context, id uint64) error {
	if id == 0 {
		return ErrInvalidInput
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return mapPublicHolidayPersistenceError(err)
	}
	return nil
}

func publicHolidayFromInput(input PublicHolidayInput) (*models.PublicHoliday, error) {
	countryCode, ok := normalizeHolidayCountryCode(input.CountryCode, true)
	if !ok {
		return nil, ErrInvalidInput
	}
	holidayDate, ok := parseHolidayCalendarDate(input.HolidayDate)
	if !ok {
		return nil, ErrInvalidInput
	}
	nameEN := strings.TrimSpace(input.NameEN)
	if nameEN == "" || utf8.RuneCountInString(nameEN) > 160 {
		return nil, ErrInvalidInput
	}
	holidayType := input.HolidayType
	if holidayType == "" {
		holidayType = models.HolidayTypeFederal
	}
	if !validHolidayType(holidayType) {
		return nil, ErrInvalidInput
	}
	dayType := input.DayType
	if dayType == "" {
		dayType = models.HolidayDayTypePublicHoliday
	}
	if !validHolidayDayType(dayType) {
		return nil, ErrInvalidInput
	}

	var observedForDate *time.Time
	if input.ObservedForDate != nil && strings.TrimSpace(*input.ObservedForDate) != "" {
		parsed, valid := parseHolidayCalendarDate(*input.ObservedForDate)
		if !valid {
			return nil, ErrInvalidInput
		}
		observedForDate = &parsed
	}
	nameZH, ok := normalizeOptionalHolidayText(input.NameZH, 160)
	if !ok {
		return nil, ErrInvalidInput
	}
	sourceName, ok := normalizeOptionalHolidayText(input.SourceName, 200)
	if !ok {
		return nil, ErrInvalidInput
	}
	sourceReference, ok := normalizeOptionalHolidayText(input.SourceReference, 2048)
	if !ok {
		return nil, ErrInvalidInput
	}
	if input.IsOfficial && (sourceName == nil || sourceReference == nil) {
		return nil, ErrInvalidInput
	}

	return &models.PublicHoliday{
		CountryCode:     countryCode,
		HolidayDate:     holidayDate,
		NameEN:          nameEN,
		NameZH:          nameZH,
		HolidayType:     holidayType,
		DayType:         dayType,
		IsOfficial:      input.IsOfficial,
		SourceName:      sourceName,
		SourceReference: sourceReference,
		ObservedForDate: observedForDate,
	}, nil
}

func parseHolidayCalendarDate(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if len(value) != len(HolidayCalendarDateLayout) {
		return time.Time{}, false
	}
	parsed, err := time.Parse(HolidayCalendarDateLayout, value)
	if err != nil || parsed.Format(HolidayCalendarDateLayout) != value {
		return time.Time{}, false
	}
	return parsed, true
}

func normalizeHolidayCountryCode(value string, useDefault bool) (string, bool) {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	if normalized == "" && useDefault {
		normalized = DefaultHolidayCountryCode
	}
	if len(normalized) != 2 {
		return "", false
	}
	for _, character := range normalized {
		if character > unicode.MaxASCII || !unicode.IsLetter(character) {
			return "", false
		}
	}
	return normalized, true
}

func normalizeOptionalHolidayText(value *string, maxLength int) (*string, bool) {
	if value == nil {
		return nil, true
	}
	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return nil, true
	}
	if maxLength > 0 && utf8.RuneCountInString(normalized) > maxLength {
		return nil, false
	}
	return &normalized, true
}

func validHolidayType(value models.HolidayType) bool {
	switch value {
	case models.HolidayTypeFederal, models.HolidayTypeObserved, models.HolidayTypeSpecial:
		return true
	default:
		return false
	}
}

func validHolidayDayType(value models.HolidayDayType) bool {
	return value == models.HolidayDayTypePublicHoliday || value == models.HolidayDayTypeWorkingDayOverride
}

func holidayCountryName(countryCode string) string {
	switch countryCode {
	case "US":
		return DefaultHolidayCountryName
	case "GH":
		return "Ghana"
	case "GB":
		return "United Kingdom"
	default:
		return countryCode
	}
}

func toHolidayCalendarItem(holiday *models.PublicHoliday, requestedLocale string) HolidayCalendarItem {
	item := HolidayCalendarItem{
		Date:        holiday.HolidayDate.Format(HolidayCalendarDateLayout),
		Name:        holiday.NameEN,
		NameEN:      holiday.NameEN,
		NameZH:      holiday.NameZH,
		HolidayType: holiday.HolidayType,
		DayType:     holiday.DayType,
		IsOfficial:  holiday.IsOfficial,
	}
	if requestedLocale == "zh" && holiday.NameZH != nil && strings.TrimSpace(*holiday.NameZH) != "" {
		item.Name = *holiday.NameZH
	}
	if holiday.ObservedForDate != nil {
		value := holiday.ObservedForDate.Format(HolidayCalendarDateLayout)
		item.ObservedForDate = &value
	}
	return item
}

func toPublicHolidayAdminDTO(holiday *models.PublicHoliday) PublicHolidayAdminDTO {
	dto := PublicHolidayAdminDTO{
		ID:              holiday.ID,
		CountryCode:     holiday.CountryCode,
		HolidayDate:     holiday.HolidayDate.Format(HolidayCalendarDateLayout),
		NameEN:          holiday.NameEN,
		NameZH:          holiday.NameZH,
		HolidayType:     holiday.HolidayType,
		DayType:         holiday.DayType,
		IsOfficial:      holiday.IsOfficial,
		SourceName:      holiday.SourceName,
		SourceReference: holiday.SourceReference,
		CreatedAt:       holiday.CreatedAt,
		UpdatedAt:       holiday.UpdatedAt,
	}
	if holiday.ObservedForDate != nil {
		value := holiday.ObservedForDate.Format(HolidayCalendarDateLayout)
		dto.ObservedForDate = &value
	}
	return dto
}

func mapPublicHolidayPersistenceError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrConflict
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unique constraint") || strings.Contains(message, "duplicate key") || strings.Contains(message, "unique failed") {
		return ErrConflict
	}
	return err
}
