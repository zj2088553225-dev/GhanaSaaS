package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"saas/internal/models"
	"saas/internal/pkg/categorysearch"
	"saas/internal/pkg/locale"
	"saas/internal/repository"

	"gorm.io/gorm"
)

type platformCategoryRepository interface {
	CreateAtomic(ctx context.Context, mutation repository.PlatformCategoryMutation) (*models.PlatformCategory, error)
	FindByID(ctx context.Context, id uint) (*models.PlatformCategory, error)
	List(ctx context.Context, activeOnly bool) ([]models.PlatformCategory, error)
	ListByMaxLevel(ctx context.Context, activeOnly bool, maxLevel int) ([]models.PlatformCategory, error)
	ListChildren(ctx context.Context, parentID *uint, activeOnly bool) ([]models.PlatformCategory, error)
	AttachBilingualDisplay(ctx context.Context, categories []models.PlatformCategory) ([]models.PlatformCategory, error)
	Search(ctx context.Context, keyword string, limit int, activeOnly bool, requestedLocale string) ([]models.PlatformCategory, error)
	UpdateAtomic(ctx context.Context, id uint, mutation repository.PlatformCategoryMutation) (*models.PlatformCategory, error)
	DeleteTree(ctx context.Context, category *models.PlatformCategory) error
	ListSubtreeIDs(ctx context.Context, rootID uint) ([]uint, error)
	Import(ctx context.Context, operations []repository.PlatformCategoryImportOperation) (repository.PlatformCategoryImportResult, error)
	RebuildSearchIndex(ctx context.Context, rootIDs []uint) error
	MaintainSearchIndex(ctx context.Context, options repository.PlatformCategorySearchMaintenanceOptions) (repository.PlatformCategorySearchMaintenanceSummary, error)
}

const (
	PlatformCategoryHasProductsMessageEN      = "This category or its child categories are used by products and cannot be deleted"
	PlatformCategorySearchDefaultLimit        = 50
	PlatformCategorySearchMaxLimit            = 100
	PlatformCategorySearchAdminRebuildTimeout = 2 * time.Minute
)

type PlatformCategoryInput struct {
	ParentID     *uint
	Name         string
	SortOrder    int
	Status       models.PlatformCategoryStatus
	Translations *PlatformCategoryTranslationInput
}

type PlatformCategoryService struct {
	repo              platformCategoryRepository
	translations      TranslationReader
	translationEditor TranslationEditor
}

type PlatformCategoryOptions struct {
	Translations      TranslationReader
	TranslationEditor TranslationEditor
}

func NewPlatformCategoryService(repo platformCategoryRepository, options ...PlatformCategoryOptions) *PlatformCategoryService {
	s := &PlatformCategoryService{repo: repo}
	if len(options) > 0 {
		s.translations = options[0].Translations
		s.translationEditor = options[0].TranslationEditor
		if s.translationEditor == nil {
			if editor, ok := options[0].Translations.(TranslationEditor); ok {
				s.translationEditor = editor
			}
		}
	}
	return s
}

func (s *PlatformCategoryService) Create(ctx context.Context, input PlatformCategoryInput) (*models.PlatformCategory, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, ErrInvalidInput
	}
	status := normalizePlatformCategoryStatus(input.Status)
	if status == "" {
		return nil, ErrInvalidInput
	}
	parentID := normalizePlatformCategoryParentID(input.ParentID)
	mutation := platformCategoryMutation(parentID, name, input)
	category, err := s.repo.CreateAtomic(ctx, mutation)
	if err != nil {
		return nil, mapPlatformCategoryMutationError(err)
	}
	return category, nil
}

func (s *PlatformCategoryService) FindByID(ctx context.Context, id uint) (*models.PlatformCategory, error) {
	if id == 0 {
		return nil, ErrInvalidInput
	}
	category, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPlatformCategoryNotFound
	}
	return category, err
}

func (s *PlatformCategoryService) FindByIDWithBilingualDisplay(ctx context.Context, id uint) (*models.PlatformCategory, error) {
	category, err := s.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	categories, err := s.repo.AttachBilingualDisplay(ctx, []models.PlatformCategory{*category})
	if err != nil {
		return nil, err
	}
	if len(categories) == 0 {
		return category, nil
	}
	return &categories[0], nil
}

func (s *PlatformCategoryService) TranslationFields(ctx context.Context, id uint, requestedLocale string) (PlatformCategoryTranslationInput, error) {
	if s.translationEditor == nil {
		return PlatformCategoryTranslationInput{}, nil
	}
	if _, err := s.FindByID(ctx, id); err != nil {
		return PlatformCategoryTranslationInput{}, err
	}
	rows, err := s.translationEditor.GetTranslationsForEntity(ctx, TranslationEntityPlatformCategory, id, requestedLocale)
	if err != nil {
		return PlatformCategoryTranslationInput{}, err
	}
	return platformCategoryTranslationInputFromRows(rows), nil
}

func (s *PlatformCategoryService) List(ctx context.Context, activeOnly bool) ([]models.PlatformCategory, error) {
	categories, err := s.repo.List(ctx, activeOnly)
	if err != nil {
		return nil, err
	}
	return s.repo.AttachBilingualDisplay(ctx, categories)
}

func (s *PlatformCategoryService) ListWithLocale(ctx context.Context, activeOnly bool, requestedLocale string) ([]models.PlatformCategory, error) {
	return s.ListWithLocaleMaxLevel(ctx, activeOnly, requestedLocale, 0)
}

func (s *PlatformCategoryService) ListWithLocaleMaxLevel(
	ctx context.Context,
	activeOnly bool,
	requestedLocale string,
	maxLevel int,
) ([]models.PlatformCategory, error) {
	normalizedLocale := locale.NormalizeLocale(requestedLocale)
	var (
		categories []models.PlatformCategory
		err        error
	)
	if maxLevel > 0 {
		categories, err = s.repo.ListByMaxLevel(ctx, activeOnly, maxLevel)
	} else {
		categories, err = s.repo.List(ctx, activeOnly)
	}
	if err != nil {
		return nil, err
	}
	return s.localizePlatformCategoryList(ctx, categories, normalizedLocale)
}

func (s *PlatformCategoryService) ListChildren(
	ctx context.Context,
	parentID *uint,
	activeOnly bool,
) ([]models.PlatformCategory, error) {
	categories, err := s.repo.ListChildren(ctx, normalizePlatformCategoryParentID(parentID), activeOnly)
	if err != nil {
		return nil, err
	}
	return s.repo.AttachBilingualDisplay(ctx, categories)
}

func (s *PlatformCategoryService) ListChildrenWithLocale(
	ctx context.Context,
	parentID *uint,
	activeOnly bool,
	requestedLocale string,
) ([]models.PlatformCategory, error) {
	normalizedParentID := normalizePlatformCategoryParentID(parentID)
	normalizedLocale := locale.NormalizeLocale(requestedLocale)
	children, err := s.repo.ListChildren(ctx, normalizedParentID, activeOnly)
	if err != nil {
		return nil, err
	}
	if s.translations == nil || len(children) == 0 {
		return children, nil
	}

	allCategories, err := s.repo.List(ctx, activeOnly)
	if err != nil {
		return nil, err
	}
	localized, err := s.localizePlatformCategoryList(ctx, allCategories, normalizedLocale)
	if err != nil {
		return nil, err
	}
	byID := make(map[uint]models.PlatformCategory, len(localized))
	for _, category := range localized {
		byID[category.ID] = category
	}
	for index := range children {
		if localizedChild, ok := byID[children[index].ID]; ok {
			localizedChild.HasChildren = children[index].HasChildren
			children[index] = localizedChild
		}
	}
	return children, nil
}

func (s *PlatformCategoryService) Search(
	ctx context.Context,
	keyword string,
	limit int,
	activeOnly bool,
	requestedLocale string,
) ([]models.PlatformCategory, error) {
	query, err := categorysearch.Parse(keyword)
	if err != nil {
		return nil, ErrInvalidInput
	}
	if len(query.Tokens) == 0 {
		return []models.PlatformCategory{}, nil
	}
	if limit <= 0 {
		limit = PlatformCategorySearchDefaultLimit
	}
	if limit > PlatformCategorySearchMaxLimit {
		limit = PlatformCategorySearchMaxLimit
	}
	categories, err := s.repo.Search(ctx, query.Normalized, limit, activeOnly, requestedLocale)
	if err != nil {
		return nil, err
	}
	return s.repo.AttachBilingualDisplay(ctx, categories)
}

func (s *PlatformCategoryService) Update(ctx context.Context, id uint, input PlatformCategoryInput) (*models.PlatformCategory, error) {
	if id == 0 {
		return nil, ErrInvalidInput
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, ErrInvalidInput
	}
	status := normalizePlatformCategoryStatus(input.Status)
	if status == "" {
		return nil, ErrInvalidInput
	}
	parentID := normalizePlatformCategoryParentID(input.ParentID)
	if parentID != nil && *parentID == id {
		return nil, ErrPlatformCategoryImportCycle
	}
	updated, err := s.repo.UpdateAtomic(ctx, id, platformCategoryMutation(parentID, name, input))
	if err != nil {
		return nil, mapPlatformCategoryMutationError(err)
	}
	return updated, nil
}

func (s *PlatformCategoryService) Delete(ctx context.Context, id uint) error {
	category, err := s.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteTree(ctx, category); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPlatformCategoryNotFound
		}
		if errors.Is(err, repository.ErrPlatformCategoryTreeHasProducts) {
			return ErrPlatformCategoryHasProducts
		}
		return err
	}
	return nil
}

func (s *PlatformCategoryService) RebuildSearchIndex(ctx context.Context) (repository.PlatformCategorySearchMaintenanceSummary, error) {
	rebuildContext, cancel := context.WithTimeout(ctx, PlatformCategorySearchAdminRebuildTimeout)
	defer cancel()
	summary, err := s.repo.MaintainSearchIndex(rebuildContext, repository.PlatformCategorySearchMaintenanceOptions{
		Apply:     true,
		Verify:    true,
		BatchSize: repository.PlatformCategorySearchRebuildDefaultBatchSize,
		TryLock:   true,
	})
	if errors.Is(err, repository.ErrPlatformCategorySearchRebuildInProgress) {
		return summary, ErrPlatformCategorySearchRebuildInProgress
	}
	return summary, err
}

func (s *PlatformCategoryService) MaintainSearchIndex(
	ctx context.Context,
	options repository.PlatformCategorySearchMaintenanceOptions,
) (repository.PlatformCategorySearchMaintenanceSummary, error) {
	return s.repo.MaintainSearchIndex(ctx, options)
}

func (s *PlatformCategoryService) localizePlatformCategoryList(
	ctx context.Context,
	categories []models.PlatformCategory,
	requestedLocale string,
) ([]models.PlatformCategory, error) {
	if s.translations == nil || len(categories) == 0 {
		return categories, nil
	}
	ids := make([]uint, 0, len(categories))
	for _, category := range categories {
		ids = append(ids, category.ID)
	}
	translations, err := s.translations.GetTranslations(ctx, TranslationEntityPlatformCategory, ids, requestedLocale)
	if err != nil {
		return nil, err
	}
	return localizePlatformCategories(categories, translations), nil
}

func platformCategoryMutation(
	parentID *uint,
	name string,
	input PlatformCategoryInput,
) repository.PlatformCategoryMutation {
	mutation := repository.PlatformCategoryMutation{
		ParentID:  cloneUintPtr(parentID),
		Name:      name,
		SortOrder: input.SortOrder,
		Status:    normalizePlatformCategoryStatus(input.Status),
	}
	if input.Translations != nil {
		mutation.ZhProvided = true
		mutation.ZhName = strings.TrimSpace(input.Translations.Name)
	}
	return mutation
}

func mapPlatformCategoryMutationError(err error) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return ErrPlatformCategoryNotFound
	case errors.Is(err, gorm.ErrDuplicatedKey), errors.Is(err, repository.ErrPlatformCategoryDuplicate):
		return ErrPlatformCategoryDuplicate
	case errors.Is(err, repository.ErrPlatformCategoryMutationCycle):
		return ErrPlatformCategoryImportCycle
	default:
		return err
	}
}

func normalizePlatformCategoryStatus(status models.PlatformCategoryStatus) models.PlatformCategoryStatus {
	if status == "" {
		return models.PlatformCategoryStatusActive
	}
	if status == models.PlatformCategoryStatusActive || status == models.PlatformCategoryStatusDisabled {
		return status
	}
	return ""
}

func normalizePlatformCategoryParentID(parentID *uint) *uint {
	if parentID == nil || *parentID == 0 {
		return nil
	}
	return parentID
}

func normalizePlatformCategoryPath(value string) string {
	parts := strings.Split(value, "/")
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			normalized = append(normalized, part)
		}
	}
	return strings.Join(normalized, "/")
}

func cloneUintPtr(value *uint) *uint {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
