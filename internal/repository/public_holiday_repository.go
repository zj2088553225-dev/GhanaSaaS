package repository

import (
	"context"
	"time"

	"saas/internal/models"

	"gorm.io/gorm"
)

type PublicHolidayRepository struct {
	db *gorm.DB
}

func NewPublicHolidayRepository(db *gorm.DB) *PublicHolidayRepository {
	return &PublicHolidayRepository{db: db}
}

func (r *PublicHolidayRepository) ListOfficialByDateRange(
	ctx context.Context,
	countryCode string,
	from time.Time,
	to time.Time,
) ([]models.PublicHoliday, error) {
	var holidays []models.PublicHoliday
	err := r.db.WithContext(ctx).
		Where("country_code = ? AND is_official = ? AND holiday_date >= ? AND holiday_date < ?", countryCode, true, from, to).
		Order("holiday_date ASC, CASE holiday_type WHEN 'observed' THEN 1 WHEN 'federal' THEN 2 ELSE 3 END ASC, id ASC").
		Find(&holidays).Error
	return holidays, err
}

func (r *PublicHolidayRepository) FindNextOfficial(
	ctx context.Context,
	countryCode string,
	from time.Time,
) (*models.PublicHoliday, error) {
	var holiday models.PublicHoliday
	err := r.db.WithContext(ctx).
		Where(
			"country_code = ? AND is_official = ? AND day_type = ? AND holiday_date >= ?",
			countryCode,
			true,
			models.HolidayDayTypePublicHoliday,
			from,
		).
		Order("holiday_date ASC, CASE holiday_type WHEN 'observed' THEN 1 WHEN 'federal' THEN 2 ELSE 3 END ASC, id ASC").
		First(&holiday).Error
	if err != nil {
		return nil, err
	}
	return &holiday, nil
}

func (r *PublicHolidayRepository) List(
	ctx context.Context,
	countryCode string,
	from *time.Time,
	to *time.Time,
) ([]models.PublicHoliday, error) {
	var holidays []models.PublicHoliday
	query := r.db.WithContext(ctx)
	if countryCode != "" {
		query = query.Where("country_code = ?", countryCode)
	}
	if from != nil {
		query = query.Where("holiday_date >= ?", *from)
	}
	if to != nil {
		query = query.Where("holiday_date < ?", *to)
	}
	err := query.Order("holiday_date ASC, country_code ASC, id ASC").Find(&holidays).Error
	return holidays, err
}

func (r *PublicHolidayRepository) FindByID(ctx context.Context, id uint64) (*models.PublicHoliday, error) {
	var holiday models.PublicHoliday
	if err := r.db.WithContext(ctx).First(&holiday, id).Error; err != nil {
		return nil, err
	}
	return &holiday, nil
}

func (r *PublicHolidayRepository) Create(ctx context.Context, holiday *models.PublicHoliday) error {
	return r.db.WithContext(ctx).Create(holiday).Error
}

func (r *PublicHolidayRepository) Update(ctx context.Context, holiday *models.PublicHoliday) error {
	result := r.db.WithContext(ctx).
		Model(&models.PublicHoliday{}).
		Where("id = ?", holiday.ID).
		Updates(map[string]any{
			"country_code":      holiday.CountryCode,
			"holiday_date":      holiday.HolidayDate,
			"name_en":           holiday.NameEN,
			"name_zh":           holiday.NameZH,
			"holiday_type":      holiday.HolidayType,
			"day_type":          holiday.DayType,
			"is_official":       holiday.IsOfficial,
			"source_name":       holiday.SourceName,
			"source_reference":  holiday.SourceReference,
			"observed_for_date": holiday.ObservedForDate,
			"updated_at":        time.Now(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *PublicHolidayRepository) Delete(ctx context.Context, id uint64) error {
	result := r.db.WithContext(ctx).Delete(&models.PublicHoliday{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
