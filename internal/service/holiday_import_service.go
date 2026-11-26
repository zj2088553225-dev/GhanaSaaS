package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"saas/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrHolidayImportValidation = errors.New("holiday import validation failed")
	ErrHolidayImportConflict   = errors.New("holiday import conflicts with existing data")
)

const (
	MaxHolidayImportFileBytes = 2 << 20
	maxHolidayImportRecords   = 5_000
)

type HolidayImportMode string

const (
	HolidayImportModeDryRun HolidayImportMode = "dry-run"
	HolidayImportModeApply  HolidayImportMode = "apply"
)

type HolidayImportStatus string

const (
	HolidayImportStatusCreated   HolidayImportStatus = "created"
	HolidayImportStatusUpdated   HolidayImportStatus = "updated"
	HolidayImportStatusUnchanged HolidayImportStatus = "unchanged"
	HolidayImportStatusConflict  HolidayImportStatus = "conflict"
	HolidayImportStatusRejected  HolidayImportStatus = "rejected"
)

type HolidayImportOptions struct {
	CountryCode string
	Apply       bool
	Overwrite   bool
}

type HolidayImportChange struct {
	Row           int                 `json:"row"`
	CountryCode   string              `json:"country_code,omitempty"`
	HolidayDate   string              `json:"holiday_date,omitempty"`
	NameEN        string              `json:"name_en,omitempty"`
	Status        HolidayImportStatus `json:"status"`
	ChangedFields []string            `json:"changed_fields,omitempty"`
	Reason        string              `json:"reason,omitempty"`
}

type HolidayImportReport struct {
	Mode      HolidayImportMode     `json:"mode"`
	Created   int                   `json:"created"`
	Updated   int                   `json:"updated"`
	Unchanged int                   `json:"unchanged"`
	Conflicts int                   `json:"conflicts"`
	Rejected  int                   `json:"rejected"`
	Changes   []HolidayImportChange `json:"changes"`
}

type HolidayImportService struct {
	db *gorm.DB
}

type holidayImportRecord struct {
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

type validatedHolidayImportRecord struct {
	Row     int
	Holiday models.PublicHoliday
}

func NewHolidayImportService(db *gorm.DB) *HolidayImportService {
	return &HolidayImportService{db: db}
}

func (s *HolidayImportService) ImportJSON(
	ctx context.Context,
	data []byte,
	options HolidayImportOptions,
) (HolidayImportReport, error) {
	report := HolidayImportReport{Mode: HolidayImportModeDryRun, Changes: make([]HolidayImportChange, 0)}
	if options.Apply {
		report.Mode = HolidayImportModeApply
	}
	if len(data) > MaxHolidayImportFileBytes {
		report.Rejected = 1
		report.Changes = append(report.Changes, HolidayImportChange{Status: HolidayImportStatusRejected, Reason: "file exceeds size limit"})
		return report, fmt.Errorf("%w: file exceeds %d bytes", ErrHolidayImportValidation, MaxHolidayImportFileBytes)
	}

	countryCode, ok := normalizeImportCountryCode(options.CountryCode)
	if !ok {
		report.Rejected = 1
		report.Changes = append(report.Changes, HolidayImportChange{Status: HolidayImportStatusRejected, Reason: "invalid country"})
		return report, fmt.Errorf("%w: invalid country", ErrHolidayImportValidation)
	}

	records, err := decodeHolidayImportFile(data)
	if err != nil {
		report.Rejected = 1
		report.Changes = append(report.Changes, HolidayImportChange{Status: HolidayImportStatusRejected, Reason: "invalid JSON file"})
		return report, fmt.Errorf("%w: invalid JSON file: %v", ErrHolidayImportValidation, err)
	}
	if len(records) == 0 {
		report.Rejected = 1
		report.Changes = append(report.Changes, HolidayImportChange{Status: HolidayImportStatusRejected, Reason: "file must contain at least one holiday"})
		return report, fmt.Errorf("%w: file must contain at least one holiday", ErrHolidayImportValidation)
	}
	if len(records) > maxHolidayImportRecords {
		report.Rejected = 1
		report.Changes = append(report.Changes, HolidayImportChange{Status: HolidayImportStatusRejected, Reason: "file contains too many holidays"})
		return report, fmt.Errorf("%w: file must not contain more than %d holidays", ErrHolidayImportValidation, maxHolidayImportRecords)
	}

	validated := make([]validatedHolidayImportRecord, 0, len(records))
	seen := make(map[string]int, len(records))
	validationErrors := make([]error, 0)
	for index, record := range records {
		row := index + 1
		holiday, validationErr := validateHolidayImportRecord(record, countryCode)
		if validationErr == nil {
			key := holidayImportBusinessKey(holiday)
			if previousRow, duplicate := seen[key]; duplicate {
				validationErr = fmt.Errorf("duplicate business key from row %d", previousRow)
			} else {
				seen[key] = row
			}
		}
		if validationErr != nil {
			report.Rejected++
			report.Changes = append(report.Changes, HolidayImportChange{
				Row: row, CountryCode: strings.TrimSpace(record.CountryCode), HolidayDate: strings.TrimSpace(record.HolidayDate),
				NameEN: strings.TrimSpace(record.NameEN), Status: HolidayImportStatusRejected, Reason: validationErr.Error(),
			})
			validationErrors = append(validationErrors, fmt.Errorf("row %d: %w", row, validationErr))
			continue
		}
		validated = append(validated, validatedHolidayImportRecord{Row: row, Holiday: *holiday})
	}
	if len(validationErrors) > 0 {
		return report, fmt.Errorf("%w: %v", ErrHolidayImportValidation, errors.Join(validationErrors...))
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for index := range validated {
			record := &validated[index]
			change, classifyErr := classifyHolidayImportChange(tx, record, options.Overwrite)
			if classifyErr != nil {
				return classifyErr
			}
			report.Changes = append(report.Changes, change)
			switch change.Status {
			case HolidayImportStatusCreated:
				report.Created++
			case HolidayImportStatusUpdated:
				report.Updated++
			case HolidayImportStatusUnchanged:
				report.Unchanged++
			case HolidayImportStatusConflict:
				report.Conflicts++
			}
		}
		if options.Apply && report.Conflicts > 0 {
			return fmt.Errorf("%w: %d existing row(s) differ; review the dry-run and use --overwrite explicitly", ErrHolidayImportConflict, report.Conflicts)
		}
		if !options.Apply {
			return nil
		}
		for index := range validated {
			change := report.Changes[index]
			if change.Status == HolidayImportStatusUnchanged {
				continue
			}
			if applyErr := upsertImportedHoliday(tx, &validated[index].Holiday); applyErr != nil {
				return applyErr
			}
		}
		return nil
	})
	if err != nil {
		return report, err
	}
	return report, nil
}

func decodeHolidayImportFile(data []byte) ([]holidayImportRecord, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var records []holidayImportRecord
	if err := decoder.Decode(&records); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("multiple JSON values are not allowed")
		}
		return nil, err
	}
	if records == nil {
		return nil, errors.New("top-level value must be an array")
	}
	return records, nil
}

func validateHolidayImportRecord(record holidayImportRecord, expectedCountry string) (*models.PublicHoliday, error) {
	country := strings.TrimSpace(record.CountryCode)
	if country != expectedCountry {
		return nil, errors.New("country_code does not match --country")
	}
	holidayDate, ok := parseHolidayCalendarDate(record.HolidayDate)
	if !ok {
		return nil, errors.New("holiday_date must use YYYY-MM-DD")
	}
	nameEN := strings.TrimSpace(record.NameEN)
	if nameEN == "" || utf8.RuneCountInString(nameEN) > 160 {
		return nil, errors.New("name_en is required and must not exceed 160 characters")
	}
	if !validHolidayType(record.HolidayType) {
		return nil, errors.New("holiday_type is not allowed")
	}
	if !validHolidayDayType(record.DayType) {
		return nil, errors.New("day_type is not allowed")
	}

	nameZH, ok := strictOptionalImportText(record.NameZH, 160)
	if !ok {
		return nil, errors.New("name_zh must not be empty or exceed 160 characters")
	}
	sourceName, ok := strictOptionalImportText(record.SourceName, 200)
	if !ok {
		return nil, errors.New("source_name must not be empty or exceed 200 characters")
	}
	sourceReference, ok := strictOptionalImportText(record.SourceReference, 2048)
	if !ok {
		return nil, errors.New("source_reference must not be empty or exceed 2048 characters")
	}
	if record.IsOfficial && (sourceName == nil || sourceReference == nil) {
		return nil, errors.New("official holidays require source_name and source_reference")
	}

	var observedForDate *time.Time
	if record.ObservedForDate != nil {
		parsed, valid := parseHolidayCalendarDate(*record.ObservedForDate)
		if !valid {
			return nil, errors.New("observed_for_date must use YYYY-MM-DD or null")
		}
		observedForDate = &parsed
	}

	return &models.PublicHoliday{
		CountryCode: country, HolidayDate: holidayDate, NameEN: nameEN, NameZH: nameZH,
		HolidayType: record.HolidayType, DayType: record.DayType, IsOfficial: record.IsOfficial,
		SourceName: sourceName, SourceReference: sourceReference, ObservedForDate: observedForDate,
	}, nil
}

func normalizeImportCountryCode(value string) (string, bool) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		value = DefaultHolidayCountryCode
	}
	if len(value) != 2 {
		return "", false
	}
	for _, character := range value {
		if character > unicode.MaxASCII || !unicode.IsLetter(character) {
			return "", false
		}
	}
	return value, true
}

func strictOptionalImportText(value *string, maxLength int) (*string, bool) {
	if value == nil {
		return nil, true
	}
	normalized := strings.TrimSpace(*value)
	if normalized == "" || utf8.RuneCountInString(normalized) > maxLength {
		return nil, false
	}
	return &normalized, true
}

func holidayImportBusinessKey(holiday *models.PublicHoliday) string {
	return holiday.CountryCode + "\x00" + holiday.HolidayDate.Format(HolidayCalendarDateLayout) + "\x00" + holiday.NameEN
}

func classifyHolidayImportChange(tx *gorm.DB, record *validatedHolidayImportRecord, overwrite bool) (HolidayImportChange, error) {
	holiday := &record.Holiday
	change := HolidayImportChange{
		Row: record.Row, CountryCode: holiday.CountryCode, HolidayDate: holiday.HolidayDate.Format(HolidayCalendarDateLayout),
		NameEN: holiday.NameEN, Status: HolidayImportStatusCreated,
	}
	var existing models.PublicHoliday
	err := tx.Where(
		"country_code = ? AND holiday_date = ? AND name_en = ?",
		holiday.CountryCode, holiday.HolidayDate, holiday.NameEN,
	).Take(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return change, nil
	}
	if err != nil {
		return HolidayImportChange{}, err
	}
	changedFields := importedHolidayChangedFields(&existing, holiday)
	if len(changedFields) == 0 {
		change.Status = HolidayImportStatusUnchanged
	} else if overwrite {
		change.Status = HolidayImportStatusUpdated
		change.ChangedFields = changedFields
	} else {
		change.Status = HolidayImportStatusConflict
		change.ChangedFields = changedFields
		change.Reason = "existing row differs; --overwrite is required to replace manual changes"
	}
	return change, nil
}

func importedHolidayChangedFields(existing *models.PublicHoliday, incoming *models.PublicHoliday) []string {
	fields := make([]string, 0, 7)
	if !optionalStringEqual(existing.NameZH, incoming.NameZH) {
		fields = append(fields, "name_zh")
	}
	if existing.HolidayType != incoming.HolidayType {
		fields = append(fields, "holiday_type")
	}
	if existing.DayType != incoming.DayType {
		fields = append(fields, "day_type")
	}
	if existing.IsOfficial != incoming.IsOfficial {
		fields = append(fields, "is_official")
	}
	if !optionalStringEqual(existing.SourceName, incoming.SourceName) {
		fields = append(fields, "source_name")
	}
	if !optionalStringEqual(existing.SourceReference, incoming.SourceReference) {
		fields = append(fields, "source_reference")
	}
	if !optionalDateEqual(existing.ObservedForDate, incoming.ObservedForDate) {
		fields = append(fields, "observed_for_date")
	}
	return fields
}

func optionalStringEqual(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func optionalDateEqual(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func upsertImportedHoliday(tx *gorm.DB, holiday *models.PublicHoliday) error {
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "country_code"}, {Name: "holiday_date"}, {Name: "name_en"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"name_zh", "holiday_type", "day_type", "is_official", "source_name", "source_reference", "observed_for_date", "updated_at",
		}),
	}).Create(holiday).Error
}
