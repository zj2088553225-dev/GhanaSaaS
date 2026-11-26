package service

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"saas/internal/models"
	"saas/internal/repository"

	"gorm.io/gorm"
)

type fakeShopProfileRepository struct {
	mu              sync.Mutex
	nextID          uint
	profiles        map[uint]models.ShopProfile
	tenants         map[uint]models.Tenant
	translations    *fakeTranslationReader
	profileWriteErr error
	translationErr  error
}

func newFakeShopProfileRepository() *fakeShopProfileRepository {
	return &fakeShopProfileRepository{
		nextID:   1,
		profiles: make(map[uint]models.ShopProfile),
		tenants: map[uint]models.Tenant{
			1001: {BaseModel: models.BaseModel{ID: 1001}, Status: models.TenantStatusActive},
			2002: {BaseModel: models.BaseModel{ID: 2002}, Status: models.TenantStatusActive},
		},
	}
}

func (r *fakeShopProfileRepository) FindByTenant(_ context.Context, tenantID uint) (*models.ShopProfile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	profile, ok := r.profiles[tenantID]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	cloned := profile
	return &cloned, nil
}

func (r *fakeShopProfileRepository) SaveAtomic(
	_ context.Context,
	tenantID uint,
	profile *models.ShopProfile,
	translation repository.ShopProfileTranslationMutation,
) (*models.ShopProfile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	tenant, ok := r.tenants[tenantID]
	if !ok || tenant.Status != models.TenantStatusActive {
		return nil, repository.ErrShopProfileTenantUnavailable
	}
	profiles := make(map[uint]models.ShopProfile, len(r.profiles))
	for id, existing := range r.profiles {
		profiles[id] = existing
	}
	nextID := r.nextID
	var translationRows []models.ContentTranslation
	if r.translations != nil {
		translationRows = append([]models.ContentTranslation(nil), r.translations.rows...)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		r.profiles = profiles
		r.nextID = nextID
		if r.translations != nil {
			r.translations.rows = translationRows
		}
	}()

	for existingTenantID, existing := range r.profiles {
		if existingTenantID != tenantID && existing.Slug == profile.Slug {
			return nil, repository.ErrDuplicateShopProfileSlug
		}
	}

	saved := *profile
	saved.TenantID = tenantID
	if existing, ok := r.profiles[tenantID]; ok {
		saved.ID = existing.ID
	} else {
		saved.ID = r.nextID
		r.nextID++
	}
	r.profiles[tenantID] = saved
	if r.profileWriteErr != nil {
		return nil, r.profileWriteErr
	}
	if translation.Provided && r.translations != nil {
		if err := r.translations.SaveTranslationsForEntity(
			context.Background(), TranslationEntityShopProfile, saved.ID, "zh",
			map[string]TranslationValueInput{
				TranslationFieldShopName:      {ValueText: translation.ShopName},
				TranslationFieldDescription:   {ValueText: translation.Description},
				TranslationFieldAddress:       {ValueText: translation.Address},
				TranslationFieldBusinessHours: {ValueText: translation.BusinessHours},
			},
		); err != nil {
			return nil, err
		}
	}
	if r.translationErr != nil {
		return nil, r.translationErr
	}
	committed = true
	cloned := saved
	return &cloned, nil
}

func validShopProfileInput(slug string) ShopProfileInput {
	return ShopProfileInput{
		Slug:          slug,
		ShopName:      "Ghana Market",
		LogoURL:       "https://example.com/logo.png",
		BannerURL:     "https://example.com/banner.png",
		Description:   "Fresh goods",
		Phone:         "123456",
		Whatsapp:      "233000000",
		Address:       "Accra",
		BusinessHours: "Mon-Sat 09:00-18:00",
		IsPublic:      true,
	}
}

func TestShopProfileServiceUpdateIsScopedToTenant(t *testing.T) {
	repo := newFakeShopProfileRepository()
	service := NewShopProfileService(repo)
	ctx := context.Background()

	tenantB, err := service.UpdateForTenant(ctx, 2002, validShopProfileInput("tenant-b"))
	if err != nil {
		t.Fatalf("seed tenant B returned error: %v", err)
	}

	tenantA, err := service.UpdateForTenant(ctx, 1001, validShopProfileInput("tenant-a"))
	if err != nil {
		t.Fatalf("update tenant A returned error: %v", err)
	}
	if tenantA.TenantID != 1001 || tenantA.Slug != "tenant-a" {
		t.Fatalf("tenant A profile = %+v, want tenant 1001 slug tenant-a", tenantA)
	}

	unchanged, err := service.GetForTenant(ctx, 2002)
	if err != nil {
		t.Fatalf("get tenant B returned error: %v", err)
	}
	if unchanged.ID != tenantB.ID || unchanged.Slug != "tenant-b" {
		t.Fatalf("tenant B profile changed: %+v", unchanged)
	}
}

func TestShopProfileServiceSavesTranslationsAndStorefrontUsesThem(t *testing.T) {
	repo := newFakeShopProfileRepository()
	translations := &fakeTranslationReader{}
	repo.translations = translations
	service := NewShopProfileService(repo, ShopProfileOptions{Translations: translations})
	ctx := context.Background()

	input := validShopProfileInput("tenant-shop")
	input.Translations = &ShopProfileTranslationInput{
		ShopName:      "中文店铺",
		Description:   "中文简介",
		Address:       "阿克拉",
		BusinessHours: "上午9点至下午6点",
	}
	saved, err := service.UpdateForTenant(ctx, 1001, input)
	if err != nil {
		t.Fatalf("UpdateForTenant returned error: %v", err)
	}
	if saved.Translations.Zh.ShopName != "中文店铺" {
		t.Fatalf("saved translations = %+v, want zh shop name", saved.Translations.Zh)
	}

	storefront := NewStorefrontService(&fakeStorefrontRepository{
		profiles: []models.ShopProfile{{
			BaseModel:     models.BaseModel{ID: saved.ID},
			TenantID:      1001,
			Slug:          saved.Slug,
			ShopName:      saved.ShopName,
			Description:   saved.Description,
			Address:       saved.Address,
			BusinessHours: saved.BusinessHours,
			IsPublic:      true,
		}},
	}, StorefrontOptions{Translations: translations})
	profile, err := storefront.PublicShopProfileWithLocale(ctx, "tenant-shop", "zh")
	if err != nil {
		t.Fatalf("PublicShopProfileWithLocale returned error: %v", err)
	}
	if profile.ShopName != "中文店铺" || profile.Address != "阿克拉" {
		t.Fatalf("localized profile = %+v, want zh fields", profile)
	}

	clearInput := validShopProfileInput("tenant-shop")
	clearInput.Translations = &ShopProfileTranslationInput{ShopName: ""}
	cleared, err := service.UpdateForTenant(ctx, 1001, clearInput)
	if err != nil {
		t.Fatalf("UpdateForTenant clearing translation returned error: %v", err)
	}
	storefront = NewStorefrontService(&fakeStorefrontRepository{
		profiles: []models.ShopProfile{{
			BaseModel:     models.BaseModel{ID: cleared.ID},
			TenantID:      1001,
			Slug:          cleared.Slug,
			ShopName:      cleared.ShopName,
			Description:   cleared.Description,
			Address:       cleared.Address,
			BusinessHours: cleared.BusinessHours,
			IsPublic:      true,
		}},
	}, StorefrontOptions{Translations: translations})
	profile, err = storefront.PublicShopProfileWithLocale(ctx, "tenant-shop", "zh")
	if err != nil {
		t.Fatalf("PublicShopProfileWithLocale after clear returned error: %v", err)
	}
	if profile.ShopName != "Ghana Market" {
		t.Fatalf("cleared profile shop_name = %q, want main table fallback", profile.ShopName)
	}
}

func TestShopProfileServiceTranslationsAreScopedToCurrentTenantProfile(t *testing.T) {
	repo := newFakeShopProfileRepository()
	translations := &fakeTranslationReader{}
	repo.translations = translations
	service := NewShopProfileService(repo, ShopProfileOptions{Translations: translations})
	ctx := context.Background()

	tenantB, err := service.UpdateForTenant(ctx, 2002, validShopProfileInput("tenant-b"))
	if err != nil {
		t.Fatalf("seed tenant B returned error: %v", err)
	}
	input := validShopProfileInput("tenant-a")
	input.Translations = &ShopProfileTranslationInput{ShopName: "租户A店铺"}
	if _, err := service.UpdateForTenant(ctx, 1001, input); err != nil {
		t.Fatalf("update tenant A returned error: %v", err)
	}

	tenantBProfile, err := service.GetForTenant(ctx, 2002)
	if err != nil {
		t.Fatalf("get tenant B returned error: %v", err)
	}
	if tenantBProfile.ID != tenantB.ID || tenantBProfile.Translations.Zh.ShopName != "" {
		t.Fatalf("tenant B profile = %+v, should not receive tenant A translation", tenantBProfile)
	}
}

func TestShopProfileServiceFakeRepositoryRollsBackTranslationFailure(t *testing.T) {
	repo := newFakeShopProfileRepository()
	translations := &fakeTranslationReader{}
	repo.translations = translations
	service := NewShopProfileService(repo, ShopProfileOptions{Translations: translations})
	input := validShopProfileInput("tenant-a")
	input.Translations = &ShopProfileTranslationInput{ShopName: "旧中文"}
	if _, err := service.UpdateForTenant(context.Background(), 1001, input); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	profilesBefore := make(map[uint]models.ShopProfile, len(repo.profiles))
	for id, profile := range repo.profiles {
		profilesBefore[id] = profile
	}
	translationsBefore := append([]models.ContentTranslation(nil), translations.rows...)
	repo.translationErr = errors.New("forced fake translation failure")
	input = validShopProfileInput("tenant-a-updated")
	input.ShopName = "Updated English"
	input.Translations = &ShopProfileTranslationInput{ShopName: "新中文"}

	if _, err := service.UpdateForTenant(context.Background(), 1001, input); err == nil {
		t.Fatal("UpdateForTenant returned nil error for translation failure")
	}
	if !reflect.DeepEqual(repo.profiles, profilesBefore) {
		t.Fatalf("profiles changed after rollback\nbefore=%+v\nafter=%+v", profilesBefore, repo.profiles)
	}
	if !reflect.DeepEqual(translations.rows, translationsBefore) {
		t.Fatalf("translations changed after rollback\nbefore=%+v\nafter=%+v", translationsBefore, translations.rows)
	}
}

func TestShopProfileServiceRejectsDuplicateSlug(t *testing.T) {
	repo := newFakeShopProfileRepository()
	service := NewShopProfileService(repo)
	ctx := context.Background()

	if _, err := service.UpdateForTenant(ctx, 2002, validShopProfileInput("same-shop")); err != nil {
		t.Fatalf("seed tenant B returned error: %v", err)
	}
	if _, err := service.UpdateForTenant(ctx, 1001, validShopProfileInput("same-shop")); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate slug error = %v, want ErrConflict", err)
	}
}

func TestShopProfileServiceValidatesSlugAndRequiredName(t *testing.T) {
	service := NewShopProfileService(newFakeShopProfileRepository())
	ctx := context.Background()

	cases := []struct {
		name  string
		input ShopProfileInput
		want  error
	}{
		{name: "empty slug", input: validShopProfileInput(""), want: ErrInvalidInput},
		{name: "uppercase slug", input: validShopProfileInput("Ghana-Market"), want: ErrInvalidInput},
		{name: "trailing hyphen", input: validShopProfileInput("ghana-market-"), want: ErrInvalidInput},
		{name: "empty shop name", input: ShopProfileInput{Slug: "ghana-market"}, want: ErrInvalidInput},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := service.UpdateForTenant(ctx, 1001, tt.input); !errors.Is(err, tt.want) {
				t.Fatalf("UpdateForTenant error = %v, want %v", err, tt.want)
			}
		})
	}
}
