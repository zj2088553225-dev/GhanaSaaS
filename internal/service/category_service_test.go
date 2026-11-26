package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"

	"saas/internal/models"

	"gorm.io/gorm"
)

type fakeCategoryRepository struct {
	mu          sync.Mutex
	nextID      uint
	categories  map[uint]models.Category
	productRefs map[uint]int64
}

func newFakeCategoryRepository() *fakeCategoryRepository {
	return &fakeCategoryRepository{
		nextID: 1,
		categories: map[uint]models.Category{
			1: {BaseModel: models.BaseModel{ID: 1}, TenantID: 1001, Name: "饮料", Sort: 1, IsActive: true},
			2: {BaseModel: models.BaseModel{ID: 2}, TenantID: 1002, Name: "饮料", Sort: 1, IsActive: true},
			3: {BaseModel: models.BaseModel{ID: 3}, TenantID: 1002, Name: "零食", Sort: 2, IsActive: true},
		},
		productRefs: make(map[uint]int64),
	}
}

func (r *fakeCategoryRepository) Create(_ context.Context, category *models.Category) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, err := r.findByNameLocked(category.TenantID, category.Name); err == nil {
		return gorm.ErrDuplicatedKey
	}
	if category.ID == 0 {
		category.ID = r.nextID
		r.nextID++
		for {
			if _, exists := r.categories[category.ID]; !exists {
				break
			}
			category.ID = r.nextID
			r.nextID++
		}
	}
	r.categories[category.ID] = *category
	return nil
}

func (r *fakeCategoryRepository) FindByIDWithTenant(_ context.Context, tenantID uint, id uint) (*models.Category, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	category, ok := r.categories[id]
	if !ok || category.TenantID != tenantID {
		return nil, gorm.ErrRecordNotFound
	}
	cloned := category
	return &cloned, nil
}

func (r *fakeCategoryRepository) FindByNameWithTenant(_ context.Context, tenantID uint, name string) (*models.Category, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.findByNameLocked(tenantID, name)
}

func (r *fakeCategoryRepository) ListByTenant(_ context.Context, tenantID uint) ([]models.Category, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	items := make([]models.Category, 0)
	for _, category := range r.categories {
		if category.TenantID == tenantID {
			items = append(items, category)
		}
	}
	sort.Slice(items, func(i int, j int) bool {
		if items[i].Sort == items[j].Sort {
			return items[i].ID < items[j].ID
		}
		return items[i].Sort < items[j].Sort
	})
	return items, nil
}

func (r *fakeCategoryRepository) UpdateWithTenant(_ context.Context, tenantID uint, category *models.Category) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.categories[category.ID]
	if !ok || existing.TenantID != tenantID {
		return gorm.ErrRecordNotFound
	}
	existing.Name = category.Name
	existing.ParentID = category.ParentID
	existing.Sort = category.Sort
	existing.Icon = category.Icon
	existing.IsActive = category.IsActive
	existing.TenantID = tenantID
	r.categories[category.ID] = existing
	return nil
}

func (r *fakeCategoryRepository) CountChildrenWithTenant(_ context.Context, tenantID uint, id uint) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var count int64
	for _, category := range r.categories {
		if category.TenantID == tenantID && category.ParentID != nil && *category.ParentID == id {
			count++
		}
	}
	return count, nil
}

func (r *fakeCategoryRepository) CountProductsWithTenant(_ context.Context, tenantID uint, id uint) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	category, ok := r.categories[id]
	if !ok || category.TenantID != tenantID {
		return 0, nil
	}
	return r.productRefs[id], nil
}

func (r *fakeCategoryRepository) DeleteWithTenant(_ context.Context, tenantID uint, id uint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	category, ok := r.categories[id]
	if !ok || category.TenantID != tenantID {
		return gorm.ErrRecordNotFound
	}
	delete(r.categories, id)
	return nil
}

func (r *fakeCategoryRepository) findByNameLocked(tenantID uint, name string) (*models.Category, error) {
	normalized := strings.ToLower(strings.TrimSpace(name))
	for _, category := range r.categories {
		if category.TenantID == tenantID && strings.ToLower(strings.TrimSpace(category.Name)) == normalized {
			cloned := category
			return &cloned, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func TestCategoryServiceListIsTenantScoped(t *testing.T) {
	service := NewCategoryService(newFakeCategoryRepository())

	categories, err := service.List(context.Background(), 1001)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(categories) != 1 {
		t.Fatalf("categories = %+v, want one category for tenant 1001", categories)
	}
	if categories[0].TenantID != 1001 || categories[0].Name != "饮料" {
		t.Fatalf("category = %+v, want tenant 1001 饮料", categories[0])
	}
}

func TestCategoryServiceCreateUsesJWTTenantAndIgnoresBodyTenantID(t *testing.T) {
	repo := newFakeCategoryRepository()
	service := NewCategoryService(repo)
	category := &models.Category{
		TenantID: 9999,
		Name:     "果汁",
		IsActive: true,
	}

	if err := service.Create(context.Background(), 1001, category); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if category.TenantID != 1001 {
		t.Fatalf("TenantID = %d, want JWT tenant 1001", category.TenantID)
	}
	stored, err := repo.FindByIDWithTenant(context.Background(), 1001, category.ID)
	if err != nil {
		t.Fatalf("FindByIDWithTenant returned error: %v", err)
	}
	if stored.TenantID != 1001 {
		t.Fatalf("stored category = %+v, want tenant 1001", stored)
	}
	if _, err := repo.FindByIDWithTenant(context.Background(), 9999, category.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("FindByIDWithTenant with body tenant error = %v, want record not found", err)
	}
}

func TestCategoryServiceAllowsSameNameAcrossTenantsAndRejectsDuplicateInTenant(t *testing.T) {
	service := NewCategoryService(newFakeCategoryRepository())

	if err := service.Create(context.Background(), 1001, &models.Category{Name: "饮料", IsActive: true}); !errors.Is(err, ErrConflict) {
		t.Fatalf("Create duplicate in same tenant error = %v, want ErrConflict", err)
	}
	if err := service.Create(context.Background(), 1003, &models.Category{Name: "饮料", IsActive: true}); err != nil {
		t.Fatalf("Create same name in different tenant returned error: %v", err)
	}
}

func TestCategoryServiceRejectsParentFromOtherTenant(t *testing.T) {
	service := NewCategoryService(newFakeCategoryRepository())
	parentID := uint(2)

	err := service.Create(context.Background(), 1001, &models.Category{
		Name:     "跨租户子分类",
		ParentID: &parentID,
		IsActive: true,
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Create with other tenant parent error = %v, want ErrNotFound", err)
	}
}

func TestCategoryServiceRejectsDeleteWhenReferencedByProduct(t *testing.T) {
	repo := newFakeCategoryRepository()
	repo.productRefs[1] = 1
	service := NewCategoryService(repo)

	err := service.Delete(context.Background(), 1001, 1)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("Delete referenced category error = %v, want ErrConflict", err)
	}
}
