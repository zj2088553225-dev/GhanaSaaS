package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"saas/internal/models"
	"saas/internal/pkg/locale"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrInvalidContentTranslation = errors.New("invalid content translation")

type ContentTranslationRepository struct {
	db           *gorm.DB
	mutationHook ContentTranslationMutationHook
}

type ContentTranslationMutation struct {
	EntityType string
	EntityID   uint
	FieldKey   string
	Locale     string
}

type ContentTranslationMutationHook struct {
	Before func(context.Context, *gorm.DB, []ContentTranslationMutation) error
	After  func(context.Context, *gorm.DB, []ContentTranslationMutation) error
}

func NewContentTranslationRepository(db *gorm.DB) *ContentTranslationRepository {
	return &ContentTranslationRepository{db: db}
}

func (r *ContentTranslationRepository) SetMutationHook(hook ContentTranslationMutationHook) {
	r.mutationHook = hook
}

func (r *ContentTranslationRepository) Upsert(ctx context.Context, translation *models.ContentTranslation) (*models.ContentTranslation, error) {
	if translation == nil {
		return nil, ErrInvalidContentTranslation
	}
	normalized, err := normalizeContentTranslation(*translation)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	if normalized.CreatedAt.IsZero() {
		normalized.CreatedAt = now
	}
	normalized.UpdatedAt = now

	var saved models.ContentTranslation
	mutation := contentTranslationMutation(normalized)
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := r.beforeMutations(ctx, tx, []ContentTranslationMutation{mutation}); err != nil {
			return err
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "entity_type"},
				{Name: "entity_id"},
				{Name: "field_key"},
				{Name: "locale"},
			},
			DoUpdates: clause.Assignments(map[string]any{
				"value_text": normalized.ValueText,
				"value_html": normalized.ValueHTML,
				"value_json": normalized.ValueJSON,
				"status":     normalized.Status,
				"updated_at": now,
			}),
		}).Create(&normalized).Error; err != nil {
			return err
		}
		if err := tx.Where("entity_type = ? AND entity_id = ? AND field_key = ? AND locale = ?",
			normalized.EntityType, normalized.EntityID, normalized.FieldKey, normalized.Locale).
			First(&saved).Error; err != nil {
			return err
		}
		return r.afterMutations(ctx, tx, []ContentTranslationMutation{mutation})
	})
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

func (r *ContentTranslationRepository) BatchUpsert(ctx context.Context, translations []models.ContentTranslation) error {
	if len(translations) == 0 {
		return nil
	}
	now := time.Now()
	rows := make([]models.ContentTranslation, 0, len(translations))
	for _, translation := range translations {
		normalized, err := normalizeContentTranslation(translation)
		if err != nil {
			return err
		}
		if normalized.CreatedAt.IsZero() {
			normalized.CreatedAt = now
		}
		normalized.UpdatedAt = now
		rows = append(rows, normalized)
	}
	mutations := make([]ContentTranslationMutation, 0, len(rows))
	for _, row := range rows {
		mutations = append(mutations, contentTranslationMutation(row))
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := r.beforeMutations(ctx, tx, mutations); err != nil {
			return err
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "entity_type"},
				{Name: "entity_id"},
				{Name: "field_key"},
				{Name: "locale"},
			},
			DoUpdates: clause.Assignments(map[string]any{
				"value_text": gorm.Expr("excluded.value_text"),
				"value_html": gorm.Expr("excluded.value_html"),
				"value_json": gorm.Expr("excluded.value_json"),
				"status":     gorm.Expr("excluded.status"),
				"updated_at": now,
			}),
		}).CreateInBatches(rows, 500).Error; err != nil {
			return err
		}
		return r.afterMutations(ctx, tx, mutations)
	})
}

func (r *ContentTranslationRepository) Delete(ctx context.Context, entityType string, entityID uint, fieldKey string, requestedLocale string) error {
	entityType = strings.TrimSpace(entityType)
	fieldKey = strings.TrimSpace(fieldKey)
	localeValue := locale.NormalizeLocale(requestedLocale)
	if entityType == "" || entityID == 0 || fieldKey == "" {
		return nil
	}
	mutation := ContentTranslationMutation{EntityType: entityType, EntityID: entityID, FieldKey: fieldKey, Locale: localeValue}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := r.beforeMutations(ctx, tx, []ContentTranslationMutation{mutation}); err != nil {
			return err
		}
		if err := tx.Where("entity_type = ? AND entity_id = ? AND field_key = ? AND locale = ?",
			entityType, entityID, fieldKey, localeValue).
			Delete(&models.ContentTranslation{}).Error; err != nil {
			return err
		}
		return r.afterMutations(ctx, tx, []ContentTranslationMutation{mutation})
	})
}

func contentTranslationMutation(translation models.ContentTranslation) ContentTranslationMutation {
	return ContentTranslationMutation{
		EntityType: translation.EntityType,
		EntityID:   translation.EntityID,
		FieldKey:   translation.FieldKey,
		Locale:     translation.Locale,
	}
}

func (r *ContentTranslationRepository) beforeMutations(
	ctx context.Context,
	tx *gorm.DB,
	mutations []ContentTranslationMutation,
) error {
	if r.mutationHook.Before == nil {
		return nil
	}
	return r.mutationHook.Before(ctx, tx, mutations)
}

func (r *ContentTranslationRepository) afterMutations(
	ctx context.Context,
	tx *gorm.DB,
	mutations []ContentTranslationMutation,
) error {
	if r.mutationHook.After == nil {
		return nil
	}
	return r.mutationHook.After(ctx, tx, mutations)
}

func (r *ContentTranslationRepository) GetEntityTranslations(ctx context.Context, entityType string, entityID uint, requestedLocale string) (map[string]models.ContentTranslation, error) {
	result := make(map[string]models.ContentTranslation)
	entityType = strings.TrimSpace(entityType)
	localeValue := locale.NormalizeLocale(requestedLocale)
	if entityType == "" || entityID == 0 {
		return result, nil
	}

	var rows []models.ContentTranslation
	err := r.db.WithContext(ctx).
		Where("entity_type = ? AND entity_id = ? AND locale = ? AND status = ?",
			entityType, entityID, localeValue, models.ContentTranslationStatusPublished).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if contentTranslationHasValue(row) {
			result[row.FieldKey] = row
		}
	}
	return result, nil
}

func (r *ContentTranslationRepository) GetTranslation(ctx context.Context, entityType string, entityID uint, fieldKey string, requestedLocale string) (*models.ContentTranslation, bool, error) {
	entityType = strings.TrimSpace(entityType)
	fieldKey = strings.TrimSpace(fieldKey)
	if entityType == "" || entityID == 0 || fieldKey == "" {
		return nil, false, nil
	}

	localePriority := fallbackLocales(requestedLocale)
	var rows []models.ContentTranslation
	err := r.db.WithContext(ctx).
		Where("entity_type = ? AND entity_id = ? AND field_key = ? AND locale IN ? AND status = ?",
			entityType, entityID, fieldKey, localePriority, models.ContentTranslationStatusPublished).
		Find(&rows).Error
	if err != nil {
		return nil, false, err
	}
	if len(rows) == 0 {
		return nil, false, nil
	}

	for _, localeValue := range localePriority {
		for i := range rows {
			if rows[i].Locale == localeValue && contentTranslationHasValue(rows[i]) {
				return &rows[i], true, nil
			}
		}
	}
	return nil, false, nil
}

func (r *ContentTranslationRepository) GetTranslations(ctx context.Context, entityType string, entityIDs []uint, requestedLocale string) (map[uint]map[string]models.ContentTranslation, error) {
	result := make(map[uint]map[string]models.ContentTranslation)
	entityType = strings.TrimSpace(entityType)
	ids := uniquePositiveUint(entityIDs)
	if entityType == "" || len(ids) == 0 {
		return result, nil
	}

	localePriority := fallbackLocales(requestedLocale)
	priorityByLocale := make(map[string]int, len(localePriority))
	for index, localeValue := range localePriority {
		priorityByLocale[localeValue] = index
	}

	var rows []models.ContentTranslation
	err := r.db.WithContext(ctx).
		Where("entity_type = ? AND entity_id IN ? AND locale IN ? AND status = ?",
			entityType, ids, localePriority, models.ContentTranslationStatusPublished).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	selectedPriority := make(map[uint]map[string]int)
	for _, row := range rows {
		if !contentTranslationHasValue(row) {
			continue
		}
		priority, ok := priorityByLocale[row.Locale]
		if !ok {
			continue
		}
		if result[row.EntityID] == nil {
			result[row.EntityID] = make(map[string]models.ContentTranslation)
			selectedPriority[row.EntityID] = make(map[string]int)
		}
		currentPriority, exists := selectedPriority[row.EntityID][row.FieldKey]
		if !exists || priority < currentPriority {
			result[row.EntityID][row.FieldKey] = row
			selectedPriority[row.EntityID][row.FieldKey] = priority
		}
	}

	return result, nil
}

func contentTranslationHasValue(translation models.ContentTranslation) bool {
	if strings.TrimSpace(translation.ValueText) != "" {
		return true
	}
	if strings.TrimSpace(translation.ValueHTML) != "" {
		return true
	}
	valueJSON := strings.TrimSpace(string(translation.ValueJSON))
	return valueJSON != "" && valueJSON != "null"
}

func normalizeContentTranslation(translation models.ContentTranslation) (models.ContentTranslation, error) {
	translation.EntityType = strings.TrimSpace(translation.EntityType)
	translation.FieldKey = strings.TrimSpace(translation.FieldKey)
	translation.Locale = locale.NormalizeLocale(translation.Locale)
	switch translation.Status {
	case "":
		translation.Status = models.ContentTranslationStatusPublished
	case models.ContentTranslationStatusDraft,
		models.ContentTranslationStatusPublished,
		models.ContentTranslationStatusRejected,
		models.ContentTranslationStatusUnpublished:
	default:
		return translation, ErrInvalidContentTranslation
	}
	if translation.EntityType == "" || translation.EntityID == 0 || translation.FieldKey == "" {
		return translation, ErrInvalidContentTranslation
	}
	return translation, nil
}

func fallbackLocales(requestedLocale string) []string {
	normalized := locale.NormalizeLocale(requestedLocale)
	if normalized == locale.DefaultLocale {
		return []string{locale.DefaultLocale}
	}
	return []string{normalized, locale.DefaultLocale}
}

func uniquePositiveUint(values []uint) []uint {
	seen := make(map[uint]struct{}, len(values))
	result := make([]uint, 0, len(values))
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
