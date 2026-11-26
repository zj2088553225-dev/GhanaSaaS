package service

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"saas/internal/models"
	"saas/internal/pkg/categorysearch"
	"saas/internal/repository"

	"gorm.io/gorm"
)

type fakePlatformCategoryRepository struct {
	mu                        sync.Mutex
	nextID                    uint
	categories                map[uint]models.PlatformCategory
	productRefs               map[uint]int64
	searchIndexes             map[string]models.PlatformCategorySearchIndex
	translations              *fakeTranslationReader
	lastActiveOnly            bool
	lastSearchLimit           int
	importCategoryErr         error
	importCategoryFailAfter   int
	importTranslationErr      error
	importSearchIndexErr      error
	importDescendantUpdateErr error
	mutationCategoryErr       error
	mutationTranslationErr    error
	mutationSearchIndexErr    error
	mutationDescendantErr     error
}

func newFakePlatformCategoryRepository() *fakePlatformCategoryRepository {
	return &fakePlatformCategoryRepository{
		nextID: 4,
		categories: map[uint]models.PlatformCategory{
			1: {
				BaseModel: models.BaseModel{ID: 1},
				Name:      "食品饮料",
				Path:      "食品饮料",
				Level:     1,
				SortOrder: 1,
				Status:    models.PlatformCategoryStatusActive,
				IsLeaf:    false,
			},
			2: {
				BaseModel: models.BaseModel{ID: 2},
				ParentID:  uintPtr(1),
				Name:      "饮料",
				Path:      "食品饮料/饮料",
				Level:     2,
				SortOrder: 1,
				Status:    models.PlatformCategoryStatusActive,
				IsLeaf:    true,
			},
			3: {
				BaseModel: models.BaseModel{ID: 3},
				Name:      "日用百货",
				Path:      "日用百货",
				Level:     1,
				SortOrder: 2,
				Status:    models.PlatformCategoryStatusActive,
				IsLeaf:    true,
			},
		},
		productRefs:   make(map[uint]int64),
		searchIndexes: make(map[string]models.PlatformCategorySearchIndex),
	}
}

func (r *fakePlatformCategoryRepository) CreateAtomic(
	_ context.Context,
	mutation repository.PlatformCategoryMutation,
) (*models.PlatformCategory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	rollback := r.beginMutationLocked()
	committed := false
	defer func() {
		if !committed {
			rollback()
		}
	}()

	if mutation.ParentID != nil {
		if _, ok := r.categories[*mutation.ParentID]; !ok {
			return nil, gorm.ErrRecordNotFound
		}
	}
	if r.mutationNameExistsLocked(0, mutation.ParentID, mutation.Name) {
		return nil, repository.ErrPlatformCategoryDuplicate
	}
	category := models.PlatformCategory{
		BaseModel: models.BaseModel{ID: r.nextID},
		ParentID:  cloneUintPtr(mutation.ParentID),
		Name:      mutation.Name,
		SortOrder: mutation.SortOrder,
		Status:    mutation.Status,
		IsLeaf:    true,
	}
	r.nextID++
	if category.ParentID == nil {
		category.Path = category.Name
		category.Level = 1
	} else {
		parent := r.categories[*category.ParentID]
		category.Path = parent.Path + "/" + category.Name
		category.Level = parent.Level + 1
		parent.IsLeaf = false
		r.categories[parent.ID] = parent
	}
	r.categories[category.ID] = category
	if r.mutationCategoryErr != nil {
		return nil, r.mutationCategoryErr
	}
	r.saveMutationTranslationLocked(category.ID, mutation)
	if r.mutationTranslationErr != nil {
		return nil, r.mutationTranslationErr
	}
	if err := r.rebuildSearchIndexesLocked([]uint{category.ID}, r.mutationSearchIndexErr); err != nil {
		return nil, err
	}
	committed = true
	result := category
	return &result, nil
}

func (r *fakePlatformCategoryRepository) FindByID(_ context.Context, id uint) (*models.PlatformCategory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	category, ok := r.categories[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	cloned := category
	return &cloned, nil
}

func (r *fakePlatformCategoryRepository) List(_ context.Context, activeOnly bool) ([]models.PlatformCategory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.lastActiveOnly = activeOnly
	items := make([]models.PlatformCategory, 0, len(r.categories))
	for _, category := range r.categories {
		if activeOnly && category.Status != models.PlatformCategoryStatusActive {
			continue
		}
		items = append(items, category)
	}
	sort.Slice(items, func(i int, j int) bool {
		if items[i].Level == items[j].Level {
			if items[i].SortOrder == items[j].SortOrder {
				return items[i].ID < items[j].ID
			}
			return items[i].SortOrder < items[j].SortOrder
		}
		return items[i].Level < items[j].Level
	})
	return items, nil
}

func (r *fakePlatformCategoryRepository) ListByMaxLevel(_ context.Context, activeOnly bool, maxLevel int) ([]models.PlatformCategory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.lastActiveOnly = activeOnly
	items := make([]models.PlatformCategory, 0, len(r.categories))
	for _, category := range r.categories {
		if activeOnly && category.Status != models.PlatformCategoryStatusActive {
			continue
		}
		if maxLevel > 0 && category.Level > maxLevel {
			continue
		}
		items = append(items, category)
	}
	sort.Slice(items, func(i int, j int) bool {
		if items[i].Level == items[j].Level {
			if items[i].SortOrder == items[j].SortOrder {
				return items[i].ID < items[j].ID
			}
			return items[i].SortOrder < items[j].SortOrder
		}
		return items[i].Level < items[j].Level
	})
	return items, nil
}

func (r *fakePlatformCategoryRepository) ListChildren(
	_ context.Context,
	parentID *uint,
	activeOnly bool,
) ([]models.PlatformCategory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.lastActiveOnly = activeOnly
	items := make([]models.PlatformCategory, 0)
	for _, category := range r.categories {
		if activeOnly && category.Status != models.PlatformCategoryStatusActive {
			continue
		}
		if parentID == nil {
			if category.ParentID != nil {
				continue
			}
		} else if category.ParentID == nil || *category.ParentID != *parentID {
			continue
		}
		items = append(items, category)
	}
	sort.Slice(items, func(i int, j int) bool {
		if items[i].SortOrder == items[j].SortOrder {
			return items[i].ID < items[j].ID
		}
		return items[i].SortOrder < items[j].SortOrder
	})
	return items, nil
}

func (r *fakePlatformCategoryRepository) AttachBilingualDisplay(
	_ context.Context,
	categories []models.PlatformCategory,
) ([]models.PlatformCategory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	allCategories := make([]models.PlatformCategory, 0, len(r.categories))
	for _, category := range r.categories {
		allCategories = append(allCategories, category)
	}
	scope := make(map[uint]struct{}, len(allCategories))
	for _, category := range allCategories {
		scope[category.ID] = struct{}{}
	}
	zhNames := make(map[uint]string)
	if r.translations != nil {
		for _, row := range r.translations.rows {
			if row.EntityType == TranslationEntityPlatformCategory &&
				row.FieldKey == TranslationFieldName &&
				row.Locale == "zh" &&
				strings.TrimSpace(row.ValueText) != "" {
				zhNames[row.EntityID] = strings.TrimSpace(row.ValueText)
			}
		}
	}
	fallback := make(map[uint]models.PlatformCategorySearchIndex)
	for _, index := range fakeBuildPlatformCategorySearchIndexes(allCategories, zhNames, scope) {
		if index.Locale == "zh" {
			fallback[index.CategoryID] = index
		}
	}
	for i := range categories {
		index, ok := r.searchIndexes[fakePlatformCategorySearchIndexKey(categories[i].ID, "zh")]
		if !ok {
			index = fallback[categories[i].ID]
		}
		if strings.TrimSpace(index.Name) != "" {
			categories[i].ZhName = index.Name
		}
		if strings.TrimSpace(index.Path) != "" {
			categories[i].ZhPath = index.Path
		}
	}
	return categories, nil
}

func (r *fakePlatformCategoryRepository) Search(
	_ context.Context,
	keyword string,
	limit int,
	activeOnly bool,
	requestedLocale string,
) ([]models.PlatformCategory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.lastActiveOnly = activeOnly
	r.lastSearchLimit = limit
	query, _ := categorysearch.Parse(keyword)
	if len(r.searchIndexes) > 0 {
		return r.searchIndexesLocked(query.Tokens, limit, activeOnly, requestedLocale), nil
	}
	items := make([]models.PlatformCategory, 0)
	for _, category := range r.categories {
		if activeOnly && category.Status != models.PlatformCategoryStatusActive {
			continue
		}
		haystack := strings.ToLower(category.Name + " " + category.Path)
		if !platformCategoryFakeSearchContainsAll(haystack, query.Tokens) {
			continue
		}
		items = append(items, category)
	}
	sort.Slice(items, func(i int, j int) bool {
		if items[i].IsLeaf != items[j].IsLeaf {
			return items[i].IsLeaf
		}
		if items[i].Level != items[j].Level {
			return items[i].Level < items[j].Level
		}
		return items[i].ID < items[j].ID
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *fakePlatformCategoryRepository) searchIndexesLocked(
	tokens []string,
	limit int,
	activeOnly bool,
	requestedLocale string,
) []models.PlatformCategory {
	localeValue := strings.ToLower(strings.TrimSpace(requestedLocale))
	if localeValue != "en" && localeValue != "zh" {
		localeValue = "all"
	}
	matchedIDs := make([]uint, 0, limit)
	seen := make(map[uint]struct{})
	indexes := make([]models.PlatformCategorySearchIndex, 0, len(r.searchIndexes))
	for _, index := range r.searchIndexes {
		indexes = append(indexes, index)
	}
	sort.Slice(indexes, func(i int, j int) bool {
		if indexes[i].Level != indexes[j].Level {
			return indexes[i].Level < indexes[j].Level
		}
		if indexes[i].SortOrder != indexes[j].SortOrder {
			return indexes[i].SortOrder < indexes[j].SortOrder
		}
		return indexes[i].CategoryID < indexes[j].CategoryID
	})
	for _, index := range indexes {
		if localeValue != "all" && index.Locale != localeValue {
			continue
		}
		if activeOnly && index.Status != models.PlatformCategoryStatusActive {
			continue
		}
		haystack := strings.ToLower(index.SearchText)
		if !platformCategoryFakeSearchContainsAll(haystack, tokens) {
			continue
		}
		if _, ok := seen[index.CategoryID]; ok {
			continue
		}
		seen[index.CategoryID] = struct{}{}
		matchedIDs = append(matchedIDs, index.CategoryID)
		if len(matchedIDs) >= limit {
			break
		}
	}
	for _, id := range append([]uint(nil), matchedIDs...) {
		for {
			category, ok := r.categories[id]
			if !ok || category.ParentID == nil {
				break
			}
			parentID := *category.ParentID
			if _, exists := seen[parentID]; !exists {
				seen[parentID] = struct{}{}
				matchedIDs = append(matchedIDs, parentID)
			}
			id = parentID
		}
	}
	items := make([]models.PlatformCategory, 0, len(matchedIDs))
	for _, id := range matchedIDs {
		if category, ok := r.categories[id]; ok {
			items = append(items, category)
		}
	}
	sort.Slice(items, func(i int, j int) bool {
		if items[i].Level != items[j].Level {
			return items[i].Level < items[j].Level
		}
		if items[i].SortOrder != items[j].SortOrder {
			return items[i].SortOrder < items[j].SortOrder
		}
		return items[i].ID < items[j].ID
	})
	return items
}

func platformCategoryFakeSearchContainsAll(haystack string, tokens []string) bool {
	if len(tokens) == 0 {
		return false
	}
	for _, token := range tokens {
		if !strings.Contains(haystack, token) {
			return false
		}
	}
	return true
}

func (r *fakePlatformCategoryRepository) UpdateAtomic(
	_ context.Context,
	id uint,
	mutation repository.PlatformCategoryMutation,
) (*models.PlatformCategory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	rollback := r.beginMutationLocked()
	committed := false
	defer func() {
		if !committed {
			rollback()
		}
	}()

	current, ok := r.categories[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	if mutation.ParentID != nil {
		if _, ok := r.categories[*mutation.ParentID]; !ok {
			return nil, gorm.ErrRecordNotFound
		}
		byID := cloneFakePlatformCategories(r.categories)
		if fakePlatformCategoryImportWouldCreateCycle(id, mutation.ParentID, byID) {
			return nil, repository.ErrPlatformCategoryMutationCycle
		}
	}
	if r.mutationNameExistsLocked(id, mutation.ParentID, mutation.Name) {
		return nil, repository.ErrPlatformCategoryDuplicate
	}

	oldPath := current.Path
	oldLevel := current.Level
	oldParentID := cloneUintPtr(current.ParentID)
	current.ParentID = cloneUintPtr(mutation.ParentID)
	current.Name = mutation.Name
	current.SortOrder = mutation.SortOrder
	current.Status = mutation.Status
	if current.ParentID == nil {
		current.Path = current.Name
		current.Level = 1
	} else {
		parent := r.categories[*current.ParentID]
		current.Path = parent.Path + "/" + current.Name
		current.Level = parent.Level + 1
	}
	r.categories[id] = current
	if r.mutationCategoryErr != nil {
		return nil, r.mutationCategoryErr
	}
	levelDelta := current.Level - oldLevel
	for descendantID, descendant := range r.categories {
		if strings.HasPrefix(descendant.Path, oldPath+"/") {
			descendant.Path = current.Path + strings.TrimPrefix(descendant.Path, oldPath)
			descendant.Level += levelDelta
			r.categories[descendantID] = descendant
			if r.mutationDescendantErr != nil {
				return nil, r.mutationDescendantErr
			}
		}
	}
	r.updateLeafLocked(oldParentID)
	r.updateLeafLocked(current.ParentID)
	r.saveMutationTranslationLocked(id, mutation)
	if r.mutationTranslationErr != nil {
		return nil, r.mutationTranslationErr
	}
	if err := r.rebuildSearchIndexesLocked([]uint{id}, r.mutationSearchIndexErr); err != nil {
		return nil, err
	}
	committed = true
	result := r.categories[id]
	return &result, nil
}

func (r *fakePlatformCategoryRepository) beginMutationLocked() func() {
	categories := cloneFakePlatformCategories(r.categories)
	indexes := cloneFakePlatformCategorySearchIndexes(r.searchIndexes)
	nextID := r.nextID
	var translations []models.ContentTranslation
	if r.translations != nil {
		translations = append([]models.ContentTranslation(nil), r.translations.rows...)
	}
	return func() {
		r.categories = categories
		r.searchIndexes = indexes
		r.nextID = nextID
		if r.translations != nil {
			r.translations.rows = translations
		}
	}
}

func (r *fakePlatformCategoryRepository) mutationNameExistsLocked(currentID uint, parentID *uint, name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	for id, category := range r.categories {
		if id == currentID || strings.ToLower(strings.TrimSpace(category.Name)) != name {
			continue
		}
		if sameTestPlatformCategoryParent(category.ParentID, parentID) {
			return true
		}
	}
	return false
}

func (r *fakePlatformCategoryRepository) saveMutationTranslationLocked(
	categoryID uint,
	mutation repository.PlatformCategoryMutation,
) {
	if r.translations == nil || !mutation.ZhProvided {
		return
	}
	next := r.translations.rows[:0]
	for _, row := range r.translations.rows {
		if row.EntityType == TranslationEntityPlatformCategory && row.EntityID == categoryID &&
			row.FieldKey == TranslationFieldName && row.Locale == "zh" {
			continue
		}
		next = append(next, row)
	}
	r.translations.rows = next
	if value := strings.TrimSpace(mutation.ZhName); value != "" {
		r.translations.rows = append(r.translations.rows, models.ContentTranslation{
			EntityType: TranslationEntityPlatformCategory,
			EntityID:   categoryID,
			FieldKey:   TranslationFieldName,
			Locale:     "zh",
			ValueText:  value,
			Status:     models.ContentTranslationStatusPublished,
		})
	}
}

func (r *fakePlatformCategoryRepository) DeleteTree(_ context.Context, category *models.PlatformCategory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.categories[category.ID]; !ok {
		return gorm.ErrRecordNotFound
	}
	subtreeIDs := make(map[uint]struct{})
	queue := []uint{category.ID}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		subtreeIDs[id] = struct{}{}
		for _, candidate := range r.categories {
			if candidate.ParentID != nil && *candidate.ParentID == id {
				queue = append(queue, candidate.ID)
			}
		}
	}
	for id := range subtreeIDs {
		if r.productRefs[id] > 0 {
			return repository.ErrPlatformCategoryTreeHasProducts
		}
	}
	for id := range subtreeIDs {
		delete(r.searchIndexes, fakePlatformCategorySearchIndexKey(id, "en"))
		delete(r.searchIndexes, fakePlatformCategorySearchIndexKey(id, "zh"))
	}
	if r.translations != nil {
		filteredRows := make([]models.ContentTranslation, 0, len(r.translations.rows))
		for _, row := range r.translations.rows {
			if row.EntityType == TranslationEntityPlatformCategory {
				if _, ok := subtreeIDs[row.EntityID]; ok {
					continue
				}
			}
			filteredRows = append(filteredRows, row)
		}
		r.translations.rows = filteredRows
	}
	for id := range subtreeIDs {
		delete(r.categories, id)
	}
	r.updateLeafLocked(category.ParentID)
	return nil
}

func (r *fakePlatformCategoryRepository) ListSubtreeIDs(_ context.Context, rootID uint) ([]uint, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	categories := make([]models.PlatformCategory, 0, len(r.categories))
	for _, category := range r.categories {
		categories = append(categories, category)
	}
	return fakePlatformCategorySubtreeIDs(categories, rootID), nil
}

func (r *fakePlatformCategoryRepository) Import(ctx context.Context, operations []repository.PlatformCategoryImportOperation) (repository.PlatformCategoryImportResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	snapshotCategories := cloneFakePlatformCategories(r.categories)
	snapshotSearchIndexes := cloneFakePlatformCategorySearchIndexes(r.searchIndexes)
	snapshotNextID := r.nextID
	var snapshotTranslations []models.ContentTranslation
	snapshotBulkCalls := 0
	if r.translations != nil {
		snapshotTranslations = append([]models.ContentTranslation(nil), r.translations.rows...)
		snapshotBulkCalls = r.translations.bulkCalls
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		r.categories = snapshotCategories
		r.searchIndexes = snapshotSearchIndexes
		r.nextID = snapshotNextID
		if r.translations != nil {
			r.translations.rows = snapshotTranslations
			r.translations.bulkCalls = snapshotBulkCalls
		}
	}()

	result := repository.PlatformCategoryImportResult{}
	categoryWrites := 0
	byPath := make(map[string]models.PlatformCategory, len(r.categories)+len(operations))
	byID := make(map[uint]models.PlatformCategory, len(r.categories))
	for _, category := range r.categories {
		byPath[strings.ToLower(category.Path)] = category
		byID[category.ID] = category
	}
	for _, operation := range operations {
		key := strings.ToLower(operation.Path)
		var parentID *uint
		if operation.ParentPath != "" {
			parent, ok := byPath[strings.ToLower(operation.ParentPath)]
			if !ok {
				return result, gorm.ErrRecordNotFound
			}
			parentValue := parent.ID
			parentID = &parentValue
		}
		category, exists := byPath[key]
		if operation.ID != nil && *operation.ID > 0 {
			if byIDCategory, ok := byID[*operation.ID]; ok {
				category = byIDCategory
				exists = true
			}
		}
		if exists {
			if fakePlatformCategoryImportWouldCreateCycle(category.ID, parentID, byID) {
				return result, repository.ErrPlatformCategoryImportCycle
			}
			if target, occupied := byPath[key]; occupied && target.ID != category.ID {
				return result, gorm.ErrDuplicatedKey
			}
			oldPath := category.Path
			oldLevel := category.Level
			changed := !sameTestPlatformCategoryParent(category.ParentID, parentID) ||
				category.Level != operation.Level ||
				category.Name != operation.Name ||
				category.Path != operation.Path
			category.ParentID = parentID
			category.Name = operation.Name
			category.Path = operation.Path
			category.Level = operation.Level
			if operation.Explicit {
				if category.SortOrder != operation.SortOrder || category.Status != operation.Status {
					changed = true
				}
				category.SortOrder = operation.SortOrder
				category.Status = operation.Status
			}
			if changed {
				if oldPath != category.Path {
					levelDelta := category.Level - oldLevel
					for id, descendant := range r.categories {
						if strings.HasPrefix(descendant.Path, oldPath+"/") {
							delete(byPath, strings.ToLower(descendant.Path))
							descendant.Path = category.Path + strings.TrimPrefix(descendant.Path, oldPath)
							descendant.Level += levelDelta
							r.categories[id] = descendant
							byPath[strings.ToLower(descendant.Path)] = descendant
							byID[id] = descendant
							if r.importDescendantUpdateErr != nil {
								return result, r.importDescendantUpdateErr
							}
						}
					}
					delete(byPath, strings.ToLower(oldPath))
				}
				r.categories[category.ID] = category
				byPath[key] = category
				byID[category.ID] = category
				if operation.Explicit {
					result.Updated++
				}
				categoryWrites++
				if r.importCategoryErr != nil && categoryWrites >= fakeImportFailureThreshold(r.importCategoryFailAfter) {
					return result, r.importCategoryErr
				}
			} else if operation.Explicit {
				result.Unchanged++
			}
			continue
		}
		newCategory := models.PlatformCategory{
			BaseModel: models.BaseModel{ID: r.nextID},
			ParentID:  parentID, Name: operation.Name, Path: operation.Path, Level: operation.Level,
			SortOrder: operation.SortOrder, Status: operation.Status, IsLeaf: true,
		}
		r.nextID++
		r.categories[newCategory.ID] = newCategory
		byPath[key] = newCategory
		byID[newCategory.ID] = newCategory
		result.Created++
		categoryWrites++
		if r.importCategoryErr != nil && categoryWrites >= fakeImportFailureThreshold(r.importCategoryFailAfter) {
			return result, r.importCategoryErr
		}
	}
	for id, category := range r.categories {
		category.IsLeaf = true
		for _, child := range r.categories {
			if child.ParentID != nil && *child.ParentID == id {
				category.IsLeaf = false
				break
			}
		}
		r.categories[id] = category
	}

	translationValues := make(map[uint]string)
	for _, operation := range operations {
		if !operation.ZhProvided {
			continue
		}
		var category models.PlatformCategory
		var exists bool
		if operation.ID != nil && *operation.ID > 0 {
			category, exists = byID[*operation.ID]
		}
		if !exists {
			category, exists = byPath[strings.ToLower(operation.Path)]
		}
		if !exists {
			return result, gorm.ErrRecordNotFound
		}
		translationValues[category.ID] = operation.ZhName
	}
	if r.translations != nil && len(translationValues) > 0 {
		r.translations.bulkCalls++
		written := 0
		for categoryID, value := range translationValues {
			if err := r.translations.SaveTranslationsForEntity(
				ctx,
				TranslationEntityPlatformCategory,
				categoryID,
				"zh",
				map[string]TranslationValueInput{TranslationFieldName: {ValueText: value}},
			); err != nil {
				return result, err
			}
			written++
			if r.importTranslationErr != nil && written >= 1 {
				return result, r.importTranslationErr
			}
		}
	} else if r.importTranslationErr != nil {
		return result, r.importTranslationErr
	}
	if err := r.rebuildSearchIndexesLocked(nil, r.importSearchIndexErr); err != nil {
		return result, err
	}
	committed = true
	return result, nil
}

func cloneFakePlatformCategories(source map[uint]models.PlatformCategory) map[uint]models.PlatformCategory {
	cloned := make(map[uint]models.PlatformCategory, len(source))
	for id, category := range source {
		if category.ParentID != nil {
			parentID := *category.ParentID
			category.ParentID = &parentID
		}
		cloned[id] = category
	}
	return cloned
}

func cloneFakePlatformCategorySearchIndexes(source map[string]models.PlatformCategorySearchIndex) map[string]models.PlatformCategorySearchIndex {
	cloned := make(map[string]models.PlatformCategorySearchIndex, len(source))
	for key, index := range source {
		if index.ParentID != nil {
			parentID := *index.ParentID
			index.ParentID = &parentID
		}
		cloned[key] = index
	}
	return cloned
}

func fakePlatformCategoryImportWouldCreateCycle(
	categoryID uint,
	parentID *uint,
	byID map[uint]models.PlatformCategory,
) bool {
	if parentID == nil {
		return false
	}
	currentID := *parentID
	visited := make(map[uint]struct{})
	for currentID != 0 {
		if currentID == categoryID {
			return true
		}
		if _, exists := visited[currentID]; exists {
			return true
		}
		visited[currentID] = struct{}{}
		current, exists := byID[currentID]
		if !exists || current.ParentID == nil {
			return false
		}
		currentID = *current.ParentID
	}
	return false
}

func fakeImportFailureThreshold(value int) int {
	if value <= 0 {
		return 1
	}
	return value
}

func (r *fakePlatformCategoryRepository) RebuildSearchIndex(_ context.Context, rootIDs []uint) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rebuildSearchIndexesLocked(rootIDs, nil)
}

func (r *fakePlatformCategoryRepository) MaintainSearchIndex(
	_ context.Context,
	options repository.PlatformCategorySearchMaintenanceOptions,
) (repository.PlatformCategorySearchMaintenanceSummary, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var rootIDs []uint
	if options.CategoryID != nil {
		rootIDs = []uint{*options.CategoryID}
	}
	if err := r.rebuildSearchIndexesLocked(rootIDs, nil); err != nil {
		return repository.PlatformCategorySearchMaintenanceSummary{}, err
	}
	categories := int64(len(r.categories))
	return repository.PlatformCategorySearchMaintenanceSummary{
		Categories:   categories,
		ExpectedRows: categories * 2,
		Unchanged:    categories * 2,
		Verified:     options.Verify,
	}, nil
}

func (r *fakePlatformCategoryRepository) rebuildSearchIndexesLocked(rootIDs []uint, failErr error) error {
	categories := make([]models.PlatformCategory, 0, len(r.categories))
	for _, category := range r.categories {
		categories = append(categories, category)
	}
	scope := make(map[uint]struct{})
	if len(rootIDs) == 0 {
		for _, category := range categories {
			scope[category.ID] = struct{}{}
		}
		r.searchIndexes = make(map[string]models.PlatformCategorySearchIndex, len(categories)*2)
	} else {
		for _, rootID := range rootIDs {
			for _, id := range fakePlatformCategorySubtreeIDs(categories, rootID) {
				scope[id] = struct{}{}
			}
		}
	}
	zhNames := make(map[uint]string)
	if r.translations != nil {
		for _, row := range r.translations.rows {
			if row.EntityType == TranslationEntityPlatformCategory &&
				row.FieldKey == TranslationFieldName &&
				row.Locale == "zh" &&
				strings.TrimSpace(row.ValueText) != "" {
				zhNames[row.EntityID] = strings.TrimSpace(row.ValueText)
			}
		}
	}
	for rowIndex, index := range fakeBuildPlatformCategorySearchIndexes(categories, zhNames, scope) {
		r.searchIndexes[fakePlatformCategorySearchIndexKey(index.CategoryID, index.Locale)] = index
		if failErr != nil && rowIndex == 0 {
			return failErr
		}
	}
	return nil
}

func (r *fakePlatformCategoryRepository) DeleteSearchIndexes(_ context.Context, categoryIDs []uint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, categoryID := range categoryIDs {
		delete(r.searchIndexes, fakePlatformCategorySearchIndexKey(categoryID, "en"))
		delete(r.searchIndexes, fakePlatformCategorySearchIndexKey(categoryID, "zh"))
	}
	return nil
}

func fakePlatformCategorySubtreeIDs(categories []models.PlatformCategory, rootID uint) []uint {
	childrenByParent := make(map[uint][]uint)
	exists := false
	for _, category := range categories {
		if category.ID == rootID {
			exists = true
		}
		if category.ParentID != nil {
			childrenByParent[*category.ParentID] = append(childrenByParent[*category.ParentID], category.ID)
		}
	}
	if !exists {
		return nil
	}
	ids := make([]uint, 0)
	queue := []uint{rootID}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		ids = append(ids, id)
		queue = append(queue, childrenByParent[id]...)
	}
	return ids
}

func fakeBuildPlatformCategorySearchIndexes(
	categories []models.PlatformCategory,
	zhNames map[uint]string,
	scope map[uint]struct{},
) []models.PlatformCategorySearchIndex {
	byID := make(map[uint]models.PlatformCategory, len(categories))
	for _, category := range categories {
		byID[category.ID] = category
	}
	var zhPath func(id uint) string
	zhPath = func(id uint) string {
		category := byID[id]
		name := strings.TrimSpace(zhNames[id])
		if name == "" {
			name = strings.TrimSpace(category.Name)
		}
		if category.ParentID == nil {
			return name
		}
		parentPath := zhPath(*category.ParentID)
		if parentPath == "" {
			return name
		}
		return parentPath + " > " + name
	}
	rows := make([]models.PlatformCategorySearchIndex, 0, len(scope)*2)
	for _, category := range categories {
		if _, ok := scope[category.ID]; !ok {
			continue
		}
		enPath := strings.ReplaceAll(category.Path, "/", " > ")
		zhName := strings.TrimSpace(zhNames[category.ID])
		if zhName == "" {
			zhName = strings.TrimSpace(category.Name)
		}
		zhPathValue := zhPath(category.ID)
		rows = append(rows,
			models.PlatformCategorySearchIndex{
				CategoryID: category.ID,
				Locale:     "en",
				Name:       category.Name,
				Path:       enPath,
				SearchText: category.Name + " " + enPath + " " + category.Path,
				ParentID:   category.ParentID,
				Level:      category.Level,
				Status:     category.Status,
				SortOrder:  category.SortOrder,
			},
			models.PlatformCategorySearchIndex{
				CategoryID: category.ID,
				Locale:     "zh",
				Name:       zhName,
				Path:       zhPathValue,
				SearchText: zhName + " " + zhPathValue + " " + strings.ReplaceAll(zhPathValue, " > ", "/") + " " + category.Name + " " + enPath + " " + category.Path,
				ParentID:   category.ParentID,
				Level:      category.Level,
				Status:     category.Status,
				SortOrder:  category.SortOrder,
			},
		)
	}
	return rows
}

func fakePlatformCategorySearchIndexKey(categoryID uint, locale string) string {
	return locale + ":" + strconv.FormatUint(uint64(categoryID), 10)
}

func sameTestPlatformCategoryParent(left, right *uint) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func (r *fakePlatformCategoryRepository) updateLeafLocked(parentID *uint) {
	if parentID == nil {
		return
	}
	parent := r.categories[*parentID]
	parent.IsLeaf = true
	for _, category := range r.categories {
		if category.ParentID != nil && *category.ParentID == *parentID {
			parent.IsLeaf = false
			break
		}
	}
	r.categories[parent.ID] = parent
}

func TestPlatformCategoryServiceCreateAndUniqueness(t *testing.T) {
	repo := newFakePlatformCategoryRepository()
	service := NewPlatformCategoryService(repo)
	ctx := context.Background()

	if _, err := service.Create(ctx, PlatformCategoryInput{ParentID: uintPtr(1), Name: "饮料"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("Create duplicate under same parent error = %v, want ErrConflict", err)
	} else if err.Error() != "同级类目已存在" {
		t.Fatalf("duplicate error = %q, want 同级类目已存在", err.Error())
	}
	created, err := service.Create(ctx, PlatformCategoryInput{ParentID: uintPtr(3), Name: "饮料"})
	if err != nil {
		t.Fatalf("Create same name under different parent returned error: %v", err)
	}
	if created.Path != "日用百货/饮料" || created.Level != 2 {
		t.Fatalf("created category = %+v, want path and level under parent", created)
	}
}

func TestPlatformCategoryServiceCreateRootAndChildPaths(t *testing.T) {
	repo := newFakePlatformCategoryRepository()
	service := NewPlatformCategoryService(repo)
	ctx := context.Background()

	root, err := service.Create(ctx, PlatformCategoryInput{Name: "女装"})
	if err != nil {
		t.Fatalf("Create root returned error: %v", err)
	}
	if root.ParentID != nil || root.Level != 1 || root.Path != "女装" {
		t.Fatalf("root = %+v, want nil parent, level 1, path 女装", root)
	}

	child, err := service.Create(ctx, PlatformCategoryInput{ParentID: &root.ID, Name: "连衣裙"})
	if err != nil {
		t.Fatalf("Create child returned error: %v", err)
	}
	if child.ParentID == nil || *child.ParentID != root.ID || child.Level != 2 || child.Path != "女装/连衣裙" {
		t.Fatalf("child = %+v, want parent %d and path 女装/连衣裙", child, root.ID)
	}
}

func TestPlatformCategoryServiceTreatsZeroParentAsRoot(t *testing.T) {
	repo := newFakePlatformCategoryRepository()
	service := NewPlatformCategoryService(repo)
	ctx := context.Background()
	zero := uint(0)

	created, err := service.Create(ctx, PlatformCategoryInput{ParentID: &zero, Name: "美妆"})
	if err != nil {
		t.Fatalf("Create with zero parent returned error: %v", err)
	}
	if created.ParentID != nil || created.Level != 1 || created.Path != "美妆" {
		t.Fatalf("created = %+v, want root category", created)
	}
}

func TestPlatformCategoryServiceListChildrenOnlyReturnsRequestedLevel(t *testing.T) {
	repo := newFakePlatformCategoryRepository()
	service := NewPlatformCategoryService(repo)
	ctx := context.Background()

	roots, err := service.ListChildren(ctx, nil, false)
	if err != nil {
		t.Fatalf("ListChildren roots returned error: %v", err)
	}
	if len(roots) != 2 || roots[0].ID != 1 || roots[1].ID != 3 {
		t.Fatalf("roots = %+v, want only IDs 1 and 3", roots)
	}

	children, err := service.ListChildren(ctx, uintPtr(1), false)
	if err != nil {
		t.Fatalf("ListChildren parent returned error: %v", err)
	}
	if len(children) != 1 || children[0].ID != 2 {
		t.Fatalf("children = %+v, want only direct child ID 2", children)
	}
}

func TestPlatformCategoryServiceListWithLocaleAppliesNamesAndLocalizedPath(t *testing.T) {
	repo := newFakePlatformCategoryRepository()
	service := NewPlatformCategoryService(repo, PlatformCategoryOptions{Translations: &fakeTranslationReader{
		rows: []models.ContentTranslation{
			{EntityType: TranslationEntityPlatformCategory, EntityID: 1, FieldKey: TranslationFieldName, Locale: "zh", ValueText: "食品"},
			{EntityType: TranslationEntityPlatformCategory, EntityID: 2, FieldKey: TranslationFieldName, Locale: "en", ValueText: "Drinks"},
			{EntityType: TranslationEntityPlatformCategory, EntityID: 3, FieldKey: TranslationFieldName, Locale: "zh", ValueText: "百货"},
		},
	}})

	categories, err := service.ListWithLocale(context.Background(), true, "zh")
	if err != nil {
		t.Fatalf("ListWithLocale returned error: %v", err)
	}
	byID := make(map[uint]models.PlatformCategory, len(categories))
	for _, category := range categories {
		byID[category.ID] = category
	}
	if byID[1].Name != "食品" || byID[1].Path != "食品" {
		t.Fatalf("root category = %+v, want zh name/path", byID[1])
	}
	if byID[2].Name != "Drinks" || byID[2].Path != "食品/Drinks" {
		t.Fatalf("child category = %+v, want parent zh and child en fallback path", byID[2])
	}
	if byID[3].Name != "百货" || byID[3].Path != "百货" {
		t.Fatalf("sibling category = %+v, want zh path", byID[3])
	}
}

func TestPlatformCategoryServiceListWithLocaleMaxLevelLimitsDepthAndLocalizes(t *testing.T) {
	repo := newFakePlatformCategoryRepository()
	repo.categories = map[uint]models.PlatformCategory{
		1: {
			BaseModel: models.BaseModel{ID: 1},
			Name:      "Home Appliances",
			Path:      "Home Appliances",
			Level:     1,
			SortOrder: 1,
			Status:    models.PlatformCategoryStatusActive,
			IsLeaf:    false,
		},
		2: {
			BaseModel: models.BaseModel{ID: 2},
			ParentID:  uintPtr(1),
			Name:      "Kitchen Appliances",
			Path:      "Home Appliances/Kitchen Appliances",
			Level:     2,
			SortOrder: 1,
			Status:    models.PlatformCategoryStatusActive,
			IsLeaf:    false,
		},
		3: {
			BaseModel: models.BaseModel{ID: 3},
			ParentID:  uintPtr(2),
			Name:      "Rice Cookers",
			Path:      "Home Appliances/Kitchen Appliances/Rice Cookers",
			Level:     3,
			SortOrder: 1,
			Status:    models.PlatformCategoryStatusActive,
			IsLeaf:    true,
		},
	}
	service := NewPlatformCategoryService(repo, PlatformCategoryOptions{Translations: &fakeTranslationReader{
		rows: []models.ContentTranslation{
			{EntityType: TranslationEntityPlatformCategory, EntityID: 1, FieldKey: TranslationFieldName, Locale: "zh", ValueText: "家用电器"},
			{EntityType: TranslationEntityPlatformCategory, EntityID: 2, FieldKey: TranslationFieldName, Locale: "zh", ValueText: "厨房电器"},
			{EntityType: TranslationEntityPlatformCategory, EntityID: 3, FieldKey: TranslationFieldName, Locale: "zh", ValueText: "电饭煲"},
		},
	}})

	categories, err := service.ListWithLocaleMaxLevel(context.Background(), true, "zh", 2)
	if err != nil {
		t.Fatalf("ListWithLocaleMaxLevel returned error: %v", err)
	}
	if len(categories) != 2 {
		t.Fatalf("categories length = %d, want only level 1 and 2", len(categories))
	}
	byID := make(map[uint]models.PlatformCategory, len(categories))
	for _, category := range categories {
		if category.Level > 2 {
			t.Fatalf("category = %+v, want max level 2", category)
		}
		byID[category.ID] = category
	}
	if byID[1].Name != "家用电器" || byID[1].Path != "家用电器" {
		t.Fatalf("root category = %+v, want zh localized root", byID[1])
	}
	if byID[2].Name != "厨房电器" || byID[2].Path != "家用电器/厨房电器" {
		t.Fatalf("child category = %+v, want zh localized child path", byID[2])
	}
	if _, exists := byID[3]; exists {
		t.Fatal("level 3 category was returned by max level 2 list")
	}
}

func TestPlatformCategoryServiceListChildrenWithLocalePreservesLazyScope(t *testing.T) {
	repo := newFakePlatformCategoryRepository()
	service := NewPlatformCategoryService(repo, PlatformCategoryOptions{Translations: &fakeTranslationReader{
		rows: []models.ContentTranslation{
			{EntityType: TranslationEntityPlatformCategory, EntityID: 1, FieldKey: TranslationFieldName, Locale: "zh", ValueText: "食品"},
			{EntityType: TranslationEntityPlatformCategory, EntityID: 2, FieldKey: TranslationFieldName, Locale: "zh", ValueText: "饮品"},
		},
	}})

	children, err := service.ListChildrenWithLocale(context.Background(), uintPtr(1), true, "zh")
	if err != nil {
		t.Fatalf("ListChildrenWithLocale returned error: %v", err)
	}
	if len(children) != 1 || children[0].ID != 2 {
		t.Fatalf("children = %+v, want only child category 2", children)
	}
	if children[0].Name != "饮品" || children[0].Path != "食品/饮品" {
		t.Fatalf("localized child = %+v, want localized path", children[0])
	}
}

func TestPlatformCategoryServiceSavesTranslationNameAndClearsToFallback(t *testing.T) {
	repo := newFakePlatformCategoryRepository()
	translations := &fakeTranslationReader{}
	repo.translations = translations
	service := NewPlatformCategoryService(repo, PlatformCategoryOptions{
		Translations:      translations,
		TranslationEditor: translations,
	})
	ctx := context.Background()

	updated, err := service.Update(ctx, 2, PlatformCategoryInput{
		ParentID: uintPtr(1),
		Name:     "Drinks",
		Status:   models.PlatformCategoryStatusActive,
		Translations: &PlatformCategoryTranslationInput{
			Name: "饮品",
		},
	})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	translation, err := service.TranslationFields(ctx, updated.ID, "zh")
	if err != nil {
		t.Fatalf("TranslationFields returned error: %v", err)
	}
	if translation.Name != "饮品" {
		t.Fatalf("translation = %+v, want zh name", translation)
	}

	categories, err := service.ListWithLocale(ctx, true, "zh")
	if err != nil {
		t.Fatalf("ListWithLocale returned error: %v", err)
	}
	byID := make(map[uint]models.PlatformCategory, len(categories))
	for _, category := range categories {
		byID[category.ID] = category
	}
	if byID[2].Name != "饮品" || byID[2].Path != "食品饮料/饮品" {
		t.Fatalf("localized category = %+v, want zh name and localized path", byID[2])
	}

	if _, err := service.Update(ctx, 2, PlatformCategoryInput{
		ParentID:     uintPtr(1),
		Name:         "Drinks",
		Status:       models.PlatformCategoryStatusActive,
		Translations: &PlatformCategoryTranslationInput{Name: ""},
	}); err != nil {
		t.Fatalf("Update clearing translation returned error: %v", err)
	}
	categories, err = service.ListWithLocale(ctx, true, "zh")
	if err != nil {
		t.Fatalf("ListWithLocale after clear returned error: %v", err)
	}
	byID = make(map[uint]models.PlatformCategory, len(categories))
	for _, category := range categories {
		byID[category.ID] = category
	}
	if byID[2].Name != "Drinks" || byID[2].Path != "食品饮料/Drinks" {
		t.Fatalf("cleared localized category = %+v, want main table fallback", byID[2])
	}
}

func TestPlatformCategoryServiceFakeRepositoryRollsBackAtomicMutationFailure(t *testing.T) {
	repo := newFakePlatformCategoryRepository()
	translations := &fakeTranslationReader{rows: []models.ContentTranslation{
		{
			EntityType: TranslationEntityPlatformCategory,
			EntityID:   2,
			FieldKey:   TranslationFieldName,
			Locale:     "zh",
			ValueText:  "饮品",
			Status:     models.ContentTranslationStatusPublished,
		},
	}}
	repo.translations = translations
	if err := repo.RebuildSearchIndex(context.Background(), nil); err != nil {
		t.Fatalf("seed search indexes: %v", err)
	}
	beforeCategories := cloneFakePlatformCategories(repo.categories)
	beforeIndexes := cloneFakePlatformCategorySearchIndexes(repo.searchIndexes)
	beforeTranslations := append([]models.ContentTranslation(nil), translations.rows...)
	repo.mutationSearchIndexErr = errors.New("forced fake index failure")
	service := NewPlatformCategoryService(repo, PlatformCategoryOptions{Translations: translations})

	if _, err := service.Update(context.Background(), 2, PlatformCategoryInput{
		ParentID: uintPtr(3),
		Name:     "Drinks",
		Status:   models.PlatformCategoryStatusDisabled,
		Translations: &PlatformCategoryTranslationInput{
			Name: "新饮品",
		},
	}); err == nil {
		t.Fatal("Update returned nil error for injected fake index failure")
	}
	if !reflect.DeepEqual(repo.categories, beforeCategories) {
		t.Fatalf("fake categories changed after rollback\nbefore=%+v\nafter=%+v", beforeCategories, repo.categories)
	}
	if !reflect.DeepEqual(repo.searchIndexes, beforeIndexes) {
		t.Fatalf("fake indexes changed after rollback\nbefore=%+v\nafter=%+v", beforeIndexes, repo.searchIndexes)
	}
	if !reflect.DeepEqual(translations.rows, beforeTranslations) {
		t.Fatalf("fake translations changed after rollback\nbefore=%+v\nafter=%+v", beforeTranslations, translations.rows)
	}
}

func TestPlatformCategoryServiceSearchMatchesNameAndPathAndLimitsResults(t *testing.T) {
	repo := newFakePlatformCategoryRepository()
	repo.categories[4] = models.PlatformCategory{
		BaseModel: models.BaseModel{ID: 4},
		ParentID:  uintPtr(2),
		Name:      "碳酸饮料",
		Path:      "食品饮料/饮料/碳酸饮料",
		Level:     3,
		SortOrder: 1,
		Status:    models.PlatformCategoryStatusActive,
		IsLeaf:    true,
	}
	repo.nextID = 5
	service := NewPlatformCategoryService(repo)
	ctx := context.Background()

	byName, err := service.Search(ctx, "碳酸", 50, true, "all")
	if err != nil {
		t.Fatalf("Search by name returned error: %v", err)
	}
	if len(byName) != 1 || byName[0].ID != 4 {
		t.Fatalf("name results = %+v, want category 4", byName)
	}

	byPath, err := service.Search(ctx, "食品饮料/饮料", 50, true, "all")
	if err != nil {
		t.Fatalf("Search by path returned error: %v", err)
	}
	if len(byPath) != 3 {
		t.Fatalf("tokenized path results length = %d, want 3", len(byPath))
	}

	limited, err := service.Search(ctx, "饮料", 1, true, "all")
	if err != nil {
		t.Fatalf("Search with limit returned error: %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("limited results length = %d, want 1", len(limited))
	}
	if !limited[0].IsLeaf {
		t.Fatalf("first limited result IsLeaf = false, want leaf result prioritized")
	}

	if _, err := service.Search(ctx, "饮料", 1000, true, "all"); err != nil {
		t.Fatalf("Search with oversized limit returned error: %v", err)
	}
	if repo.lastSearchLimit != PlatformCategorySearchMaxLimit {
		t.Fatalf("repository search limit = %d, want max %d", repo.lastSearchLimit, PlatformCategorySearchMaxLimit)
	}
}

func TestPlatformCategoryServiceRebuildSearchIndexSupportsBilingualNameAndPath(t *testing.T) {
	repo := newEmptyFakePlatformCategoryRepository()
	translations := &fakeTranslationReader{}
	repo.translations = translations
	service := NewPlatformCategoryService(repo, PlatformCategoryOptions{
		Translations:      translations,
		TranslationEditor: translations,
	})
	ctx := context.Background()
	content := platformCategoryImportWorkbook(t, platformCategoryBilingualPathImportHeaders, [][]string{
		{"Home Appliances", "家用电器", "100", "active"},
		{"Home Appliances > Kitchen Appliances", "家用电器 > 厨房电器", "110", "active"},
		{"Electronics", "电子产品", "200", "active"},
		{"Electronics > Mobile Accessories", "电子产品 > 手机配件", "210", "active"},
	})
	if _, _, err := service.ConfirmImport(ctx, bytes.NewReader(content)); err != nil {
		t.Fatalf("ConfirmImport returned error: %v", err)
	}

	tests := []struct {
		name    string
		keyword string
		locale  string
		wantID  uint
	}{
		{name: "English name", keyword: "Home Appliances", locale: "en", wantID: 1},
		{name: "English path", keyword: "Electronics > Mobile Accessories", locale: "en", wantID: 4},
		{name: "Chinese name", keyword: "手机配件", locale: "zh", wantID: 4},
		{name: "Chinese path", keyword: "电子产品 > 手机配件", locale: "zh", wantID: 4},
		{name: "All locales", keyword: "厨房电器", locale: "all", wantID: 3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			results, err := service.Search(ctx, test.keyword, 100, true, test.locale)
			if err != nil {
				t.Fatalf("Search returned error: %v", err)
			}
			if !platformCategoryResultsContainID(results, test.wantID) {
				t.Fatalf("results = %+v, want category %d", results, test.wantID)
			}
		})
	}

	results, err := service.Search(ctx, "手机配件", 100, true, "zh")
	if err != nil {
		t.Fatalf("Search child returned error: %v", err)
	}
	if !platformCategoryResultsContainID(results, 2) || !platformCategoryResultsContainID(results, 4) {
		t.Fatalf("child match results = %+v, want parent and child chain", results)
	}
	byID := platformCategoryByID(results)
	if byID[4].ZhName != "手机配件" || byID[4].ZhPath != "电子产品 > 手机配件" {
		t.Fatalf("search result display = %+v, want zh name/path from index", byID[4])
	}
	if len(repo.searchIndexes) != 8 {
		t.Fatalf("search index rows = %d, want 8 for four categories and two locales", len(repo.searchIndexes))
	}

	if _, err := service.RebuildSearchIndex(ctx); err != nil {
		t.Fatalf("RebuildSearchIndex returned error: %v", err)
	}
	if len(repo.searchIndexes) != 8 {
		t.Fatalf("search index rows after repeated rebuild = %d, want no duplicates", len(repo.searchIndexes))
	}
}

func TestPlatformCategoryServiceListBilingualDisplayFallsBackWhenIndexMissing(t *testing.T) {
	repo := newFakePlatformCategoryRepository()
	translations := &fakeTranslationReader{
		rows: []models.ContentTranslation{
			{EntityType: TranslationEntityPlatformCategory, EntityID: 1, FieldKey: TranslationFieldName, Locale: "zh", ValueText: "食品"},
			{EntityType: TranslationEntityPlatformCategory, EntityID: 2, FieldKey: TranslationFieldName, Locale: "zh", ValueText: "饮品"},
		},
	}
	repo.translations = translations
	repo.searchIndexes = map[string]models.PlatformCategorySearchIndex{}
	service := NewPlatformCategoryService(repo, PlatformCategoryOptions{Translations: translations})

	categories, err := service.List(context.Background(), true)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	byID := platformCategoryByID(categories)
	if byID[1].ZhName != "食品" || byID[1].ZhPath != "食品" {
		t.Fatalf("root fallback display = %+v, want zh translation path", byID[1])
	}
	if byID[2].ZhName != "饮品" || byID[2].ZhPath != "食品 > 饮品" {
		t.Fatalf("child fallback display = %+v, want zh translation path", byID[2])
	}
	if byID[3].ZhName != "日用百货" || byID[3].ZhPath != "日用百货" {
		t.Fatalf("missing translation fallback display = %+v, want main table fallback", byID[3])
	}
}

func TestPlatformCategoryServiceSearchIndexFallbackAndUpdates(t *testing.T) {
	repo := newEmptyFakePlatformCategoryRepository()
	translations := &fakeTranslationReader{}
	repo.translations = translations
	service := NewPlatformCategoryService(repo, PlatformCategoryOptions{
		Translations:      translations,
		TranslationEditor: translations,
	})
	ctx := context.Background()
	root, err := service.Create(ctx, PlatformCategoryInput{
		Name:   "Electronics",
		Status: models.PlatformCategoryStatusActive,
	})
	if err != nil {
		t.Fatalf("Create root returned error: %v", err)
	}
	child, err := service.Create(ctx, PlatformCategoryInput{
		ParentID: &root.ID,
		Name:     "Mobile Accessories",
		Status:   models.PlatformCategoryStatusActive,
		Translations: &PlatformCategoryTranslationInput{
			Name: "手机配件",
		},
	})
	if err != nil {
		t.Fatalf("Create child returned error: %v", err)
	}

	results, err := service.Search(ctx, "Mobile Accessories", 100, true, "zh")
	if err != nil {
		t.Fatalf("Search fallback returned error: %v", err)
	}
	if !platformCategoryResultsContainID(results, child.ID) {
		t.Fatalf("fallback zh results = %+v, want English fallback category", results)
	}

	if _, err := service.Update(ctx, child.ID, PlatformCategoryInput{
		ParentID: &root.ID,
		Name:     "Phone Cases",
		Status:   models.PlatformCategoryStatusActive,
		Translations: &PlatformCategoryTranslationInput{
			Name: "手机壳",
		},
	}); err != nil {
		t.Fatalf("Update child returned error: %v", err)
	}
	results, err = service.Search(ctx, "手机壳", 100, true, "zh")
	if err != nil {
		t.Fatalf("Search updated zh returned error: %v", err)
	}
	if !platformCategoryResultsContainID(results, child.ID) {
		t.Fatalf("updated zh results = %+v, want child", results)
	}
	results, err = service.Search(ctx, "Phone Cases", 100, true, "en")
	if err != nil {
		t.Fatalf("Search updated en returned error: %v", err)
	}
	if !platformCategoryResultsContainID(results, child.ID) {
		t.Fatalf("updated en results = %+v, want child", results)
	}
}

func TestPlatformCategoryServiceSearchIndexHandlesLargeRebuildPath(t *testing.T) {
	repo := newEmptyFakePlatformCategoryRepository()
	translations := &fakeTranslationReader{}
	repo.translations = translations
	service := NewPlatformCategoryService(repo, PlatformCategoryOptions{
		Translations:      translations,
		TranslationEditor: translations,
	})
	rows := make([][]string, 0, 2000)
	for index := 1; index <= 2000; index++ {
		rows = append(rows, []string{
			"Bulk Root > Bulk Child " + strconv.Itoa(index),
			"批量根 > 批量子类目" + strconv.Itoa(index),
			strconv.Itoa(index),
			"active",
		})
	}
	content := platformCategoryImportWorkbook(t, platformCategoryBilingualPathImportHeaders, rows)
	if _, _, err := service.ConfirmImport(context.Background(), bytes.NewReader(content)); err != nil {
		t.Fatalf("ConfirmImport returned error: %v", err)
	}
	if len(repo.searchIndexes) != len(repo.categories)*2 {
		t.Fatalf("search indexes = %d categories=%d, want two locales per category", len(repo.searchIndexes), len(repo.categories))
	}
	results, err := service.Search(context.Background(), "批量子类目1999", 100, true, "zh")
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("large zh search returned no results")
	}
}

func TestPlatformCategoryServiceLazyListDoesNotReturnFullLargeTree(t *testing.T) {
	repo := newFakePlatformCategoryRepository()
	for id := uint(4); id <= 9004; id++ {
		repo.categories[id] = models.PlatformCategory{
			BaseModel: models.BaseModel{ID: id},
			ParentID:  uintPtr(1),
			Name:      "导入类目",
			Path:      "食品饮料/导入类目",
			Level:     2,
			Status:    models.PlatformCategoryStatusActive,
			IsLeaf:    true,
		}
	}
	service := NewPlatformCategoryService(repo)

	roots, err := service.ListChildren(context.Background(), nil, false)
	if err != nil {
		t.Fatalf("ListChildren roots returned error: %v", err)
	}
	if len(roots) != 2 {
		t.Fatalf("roots length = %d, want 2 without 9000+ descendants", len(roots))
	}
	if roots[0].IsLeaf {
		t.Fatalf("root IsLeaf = true, want has_children derivable from false leaf flag")
	}
}

func TestPlatformCategoryServiceDuplicateRootNameMessage(t *testing.T) {
	repo := newFakePlatformCategoryRepository()
	service := NewPlatformCategoryService(repo)
	ctx := context.Background()

	if _, err := service.Create(ctx, PlatformCategoryInput{Name: "女装"}); err != nil {
		t.Fatalf("Create root returned error: %v", err)
	}
	if _, err := service.Create(ctx, PlatformCategoryInput{Name: "女装"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("Create duplicate root error = %v, want ErrConflict", err)
	} else if err.Error() != "同级类目已存在" {
		t.Fatalf("duplicate root error = %q, want 同级类目已存在", err.Error())
	}
}

func TestPlatformCategoryServiceMaintainsMostSpecificStateOnCreateAndDelete(t *testing.T) {
	repo := newFakePlatformCategoryRepository()
	service := NewPlatformCategoryService(repo)
	ctx := context.Background()

	parent, err := service.Create(ctx, PlatformCategoryInput{Name: "家用电器"})
	if err != nil {
		t.Fatalf("Create parent returned error: %v", err)
	}
	if !parent.IsLeaf {
		t.Fatalf("new category IsLeaf = false, want true before children are added")
	}

	child, err := service.Create(ctx, PlatformCategoryInput{ParentID: &parent.ID, Name: "厨房电器"})
	if err != nil {
		t.Fatalf("Create child returned error: %v", err)
	}
	if !child.IsLeaf {
		t.Fatalf("child IsLeaf = false, want true")
	}

	parentAfterChild, err := service.FindByID(ctx, parent.ID)
	if err != nil {
		t.Fatalf("FindByID parent returned error: %v", err)
	}
	if parentAfterChild.IsLeaf {
		t.Fatalf("parent IsLeaf = true, want false after child is added")
	}

	if err := service.Delete(ctx, child.ID); err != nil {
		t.Fatalf("Delete child returned error: %v", err)
	}
	parentAfterDelete, err := service.FindByID(ctx, parent.ID)
	if err != nil {
		t.Fatalf("FindByID parent after delete returned error: %v", err)
	}
	if !parentAfterDelete.IsLeaf {
		t.Fatalf("parent IsLeaf = false, want true after last child is deleted")
	}
}

func TestPlatformCategoryServiceUpdateDeletedCategoryReturnsRefreshMessage(t *testing.T) {
	repo := newFakePlatformCategoryRepository()
	service := NewPlatformCategoryService(repo)
	ctx := context.Background()

	created, err := service.Create(ctx, PlatformCategoryInput{Name: "女装"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if err := service.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if _, err := service.Update(ctx, created.ID, PlatformCategoryInput{Name: "女装"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update deleted category error = %v, want ErrNotFound", err)
	} else if err.Error() != "类目不存在，请刷新后重试" {
		t.Fatalf("not found error = %q, want 类目不存在，请刷新后重试", err.Error())
	}
}

func TestPlatformCategoryServiceDeletesEmptyCategoryTree(t *testing.T) {
	repo := newFakePlatformCategoryRepository()
	service := NewPlatformCategoryService(repo)

	if err := service.Delete(context.Background(), 1); err != nil {
		t.Fatalf("Delete empty parent tree returned error: %v", err)
	}
	if _, exists := repo.categories[1]; exists {
		t.Fatal("root category still exists after tree deletion")
	}
	if _, exists := repo.categories[2]; exists {
		t.Fatal("child category still exists after tree deletion")
	}
}

func TestPlatformCategoryServiceDeletesUnusedLeafCategory(t *testing.T) {
	repo := newFakePlatformCategoryRepository()
	service := NewPlatformCategoryService(repo)

	if err := service.Delete(context.Background(), 2); err != nil {
		t.Fatalf("Delete unused leaf category returned error: %v", err)
	}
	if _, exists := repo.categories[2]; exists {
		t.Fatal("leaf category still exists after delete")
	}
	if repo.categories[1].IsLeaf != true {
		t.Fatalf("parent is_leaf = %v, want true after deleting only child", repo.categories[1].IsLeaf)
	}
}

func TestPlatformCategoryServiceDeleteCleansSearchIndexesTranslationsAndSearchResults(t *testing.T) {
	repo := newFakePlatformCategoryRepository()
	translations := &fakeTranslationReader{
		rows: []models.ContentTranslation{
			{EntityType: TranslationEntityPlatformCategory, EntityID: 1, FieldKey: TranslationFieldName, Locale: "zh", ValueText: "食品"},
			{EntityType: TranslationEntityPlatformCategory, EntityID: 2, FieldKey: TranslationFieldName, Locale: "zh", ValueText: "饮品"},
			{EntityType: TranslationEntityProduct, EntityID: 2, FieldKey: TranslationFieldName, Locale: "zh", ValueText: "不是平台类目"},
		},
	}
	repo.translations = translations
	service := NewPlatformCategoryService(repo, PlatformCategoryOptions{Translations: translations})
	ctx := context.Background()

	if _, err := service.RebuildSearchIndex(ctx); err != nil {
		t.Fatalf("RebuildSearchIndex returned error: %v", err)
	}
	if _, ok := repo.searchIndexes[fakePlatformCategorySearchIndexKey(1, "en")]; !ok {
		t.Fatal("root search index missing before delete")
	}
	if _, ok := repo.searchIndexes[fakePlatformCategorySearchIndexKey(2, "zh")]; !ok {
		t.Fatal("child zh search index missing before delete")
	}

	if err := service.Delete(ctx, 1); err != nil {
		t.Fatalf("Delete parent category returned error: %v", err)
	}
	for _, id := range []uint{1, 2} {
		if _, ok := repo.categories[id]; ok {
			t.Fatalf("category %d still exists after delete", id)
		}
		if _, ok := repo.searchIndexes[fakePlatformCategorySearchIndexKey(id, "en")]; ok {
			t.Fatalf("category %d en search index remained after delete", id)
		}
		if _, ok := repo.searchIndexes[fakePlatformCategorySearchIndexKey(id, "zh")]; ok {
			t.Fatalf("category %d zh search index remained after delete", id)
		}
	}
	for _, row := range translations.rows {
		if row.EntityType == TranslationEntityPlatformCategory && (row.EntityID == 1 || row.EntityID == 2) {
			t.Fatalf("platform category translation remained after delete: %+v", row)
		}
	}
	if len(translations.rows) != 1 || translations.rows[0].EntityType != TranslationEntityProduct {
		t.Fatalf("unexpected remaining translations = %+v, want unrelated product translation only", translations.rows)
	}
	results, err := service.Search(ctx, "食品", 100, true, "all")
	if err != nil {
		t.Fatalf("Search after delete returned error: %v", err)
	}
	if platformCategoryResultsContainID(results, 1) || platformCategoryResultsContainID(results, 2) {
		t.Fatalf("deleted categories were still searchable: %+v", results)
	}
}

func TestPlatformCategoryServiceDeletesImportedEmptyCategoryTree(t *testing.T) {
	repo := newFakePlatformCategoryRepository()
	service := NewPlatformCategoryService(repo)
	ctx := context.Background()
	content := platformCategoryImportWorkbook(t, platformCategoryPathImportHeaders, [][]string{
		{"导入类目/二级类目/三级类目", "10", "enabled"},
	})
	if _, _, err := service.ConfirmImport(ctx, bytes.NewReader(content)); err != nil {
		t.Fatalf("ConfirmImport returned error: %v", err)
	}

	var rootID uint
	for id, category := range repo.categories {
		if category.Path == "导入类目" {
			rootID = id
			break
		}
	}
	if rootID == 0 {
		t.Fatal("imported root category was not created")
	}
	if err := service.Delete(ctx, rootID); err != nil {
		t.Fatalf("Delete imported empty tree returned error: %v", err)
	}
	for _, category := range repo.categories {
		if category.Path == "导入类目" || strings.HasPrefix(category.Path, "导入类目/") {
			t.Fatalf("imported category remained after tree deletion: %+v", category)
		}
	}
}

func TestPlatformCategoryServiceRejectsDeleteWhenCurrentCategoryHasProduct(t *testing.T) {
	repo := newFakePlatformCategoryRepository()
	service := NewPlatformCategoryService(repo)
	repo.productRefs[2] = 1
	err := service.Delete(context.Background(), 2)
	if !errors.Is(err, ErrPlatformCategoryHasProducts) {
		t.Fatalf("Delete category referenced by product error = %v, want ErrPlatformCategoryHasProducts", err)
	}
	if err.Error() != "该类目或子类目已被商品使用，不能删除" {
		t.Fatalf("error = %q, want product binding message", err.Error())
	}
	if _, exists := repo.categories[2]; !exists {
		t.Fatal("category was deleted despite product reference")
	}
	if repo.productRefs[2] != 1 {
		t.Fatal("product reference was modified")
	}
}

func TestPlatformCategoryServiceRejectsDeleteWhenDescendantHasProduct(t *testing.T) {
	repo := newFakePlatformCategoryRepository()
	service := NewPlatformCategoryService(repo)
	repo.productRefs[2] = 1

	err := service.Delete(context.Background(), 1)
	if !errors.Is(err, ErrPlatformCategoryHasProducts) {
		t.Fatalf("Delete parent with referenced descendant error = %v, want ErrPlatformCategoryHasProducts", err)
	}
	if _, rootExists := repo.categories[1]; !rootExists {
		t.Fatal("root category was deleted despite descendant product reference")
	}
	if _, childExists := repo.categories[2]; !childExists {
		t.Fatal("child category was deleted despite product reference")
	}
}

func TestPlatformCategoryServiceDeleteTreeMaintainsExternalParentLeafState(t *testing.T) {
	repo := newFakePlatformCategoryRepository()
	service := NewPlatformCategoryService(repo)
	ctx := context.Background()

	first, err := service.Create(ctx, PlatformCategoryInput{ParentID: uintPtr(3), Name: "清洁用品"})
	if err != nil {
		t.Fatalf("Create first child returned error: %v", err)
	}
	grandchild, err := service.Create(ctx, PlatformCategoryInput{ParentID: &first.ID, Name: "清洁剂"})
	if err != nil {
		t.Fatalf("Create grandchild returned error: %v", err)
	}
	second, err := service.Create(ctx, PlatformCategoryInput{ParentID: uintPtr(3), Name: "收纳用品"})
	if err != nil {
		t.Fatalf("Create second child returned error: %v", err)
	}

	if err := service.Delete(ctx, first.ID); err != nil {
		t.Fatalf("Delete first subtree returned error: %v", err)
	}
	if _, exists := repo.categories[grandchild.ID]; exists {
		t.Fatal("grandchild still exists after subtree deletion")
	}
	parent := repo.categories[3]
	if parent.IsLeaf {
		t.Fatal("parent became leaf while another child still exists")
	}

	if err := service.Delete(ctx, second.ID); err != nil {
		t.Fatalf("Delete last child returned error: %v", err)
	}
	parent = repo.categories[3]
	if !parent.IsLeaf {
		t.Fatal("parent did not become leaf after last child deletion")
	}
}

func TestPlatformCategoryServiceDeleteMissingCategoryReturnsNotFound(t *testing.T) {
	service := NewPlatformCategoryService(newFakePlatformCategoryRepository())
	err := service.Delete(context.Background(), 9999)
	if !errors.Is(err, ErrPlatformCategoryNotFound) {
		t.Fatalf("Delete missing category error = %v, want ErrPlatformCategoryNotFound", err)
	}
}

func TestPlatformCategoryServiceUpdatesPathAndRejectsDescendantParent(t *testing.T) {
	repo := newFakePlatformCategoryRepository()
	service := NewPlatformCategoryService(repo)
	ctx := context.Background()
	child, err := service.Create(ctx, PlatformCategoryInput{ParentID: uintPtr(2), Name: "碳酸饮料"})
	if err != nil {
		t.Fatalf("Create child returned error: %v", err)
	}

	if _, err := service.Update(ctx, 1, PlatformCategoryInput{ParentID: &child.ID, Name: "食品饮料"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Update parent to descendant error = %v, want ErrInvalidInput", err)
	}

	updated, err := service.Update(ctx, 2, PlatformCategoryInput{ParentID: uintPtr(3), Name: "饮料"})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if updated.Path != "日用百货/饮料" {
		t.Fatalf("updated path = %q, want 日用百货/饮料", updated.Path)
	}
	descendant, err := service.FindByID(ctx, child.ID)
	if err != nil {
		t.Fatalf("FindByID descendant returned error: %v", err)
	}
	if descendant.Path != "日用百货/饮料/碳酸饮料" {
		t.Fatalf("descendant path = %q, want moved path", descendant.Path)
	}
}

func platformCategoryResultsContainID(categories []models.PlatformCategory, id uint) bool {
	for _, category := range categories {
		if category.ID == id {
			return true
		}
	}
	return false
}
