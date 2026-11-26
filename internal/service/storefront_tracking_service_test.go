package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"saas/internal/models"

	"gorm.io/gorm"
)

type fakeStorefrontTrackingRepository struct {
	setting *models.StorefrontTrackingSetting
}

func (r *fakeStorefrontTrackingRepository) Get(_ context.Context) (*models.StorefrontTrackingSetting, error) {
	if r.setting == nil {
		return nil, gorm.ErrRecordNotFound
	}
	cloned := *r.setting
	return &cloned, nil
}

func (r *fakeStorefrontTrackingRepository) Save(
	_ context.Context,
	setting models.StorefrontTrackingSetting,
) (*models.StorefrontTrackingSetting, error) {
	setting.ID = 1
	r.setting = &setting
	cloned := setting
	return &cloned, nil
}

func TestStorefrontTrackingServiceSavesAndFiltersPublicSettings(t *testing.T) {
	repo := &fakeStorefrontTrackingRepository{}
	trackingService := NewStorefrontTrackingService(repo)

	saved, err := trackingService.Save(context.Background(), StorefrontTrackingInput{
		MetaPixelEnabled:  true,
		MetaPixelID:       "123456789",
		CustomHeadEnabled: false,
		CustomHeadHTML:    `<meta name="verification" content="abc">`,
	})
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if !saved.MetaPixelEnabled || saved.MetaPixelID != "123456789" {
		t.Fatalf("saved = %+v, want enabled pixel", saved)
	}

	public, err := trackingService.GetPublic(context.Background())
	if err != nil {
		t.Fatalf("GetPublic returned error: %v", err)
	}
	if !public.MetaPixelEnabled || public.MetaPixelID != "123456789" {
		t.Fatalf("public = %+v, want enabled pixel", public)
	}
	if public.CustomHeadEnabled || public.CustomHeadHTML != "" {
		t.Fatalf("public custom head = %+v, want disabled content omitted", public)
	}
}

func TestStorefrontTrackingServiceValidatesPixelAndCustomHead(t *testing.T) {
	cases := []StorefrontTrackingInput{
		{MetaPixelEnabled: true, MetaPixelID: ""},
		{MetaPixelID: "fbq('init', '123')"},
		{MetaPixelID: "1234"},
		{CustomHeadEnabled: true, CustomHeadHTML: `<iframe src="javascript:alert(1)"></iframe>`},
		{CustomHeadEnabled: true, CustomHeadHTML: `<script onload="alert(1)"></script>`},
		{CustomHeadEnabled: true, CustomHeadHTML: strings.Repeat("x", maxCustomHeadHTMLLength+1)},
	}

	for _, input := range cases {
		trackingService := NewStorefrontTrackingService(&fakeStorefrontTrackingRepository{})
		if _, err := trackingService.Save(context.Background(), input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("Save(%+v) error = %v, want ErrInvalidInput", input, err)
		}
	}
}

func TestStorefrontTrackingServiceReturnsEmptyDefaults(t *testing.T) {
	trackingService := NewStorefrontTrackingService(&fakeStorefrontTrackingRepository{})
	admin, err := trackingService.GetAdmin(context.Background())
	if err != nil {
		t.Fatalf("GetAdmin returned error: %v", err)
	}
	public, err := trackingService.GetPublic(context.Background())
	if err != nil {
		t.Fatalf("GetPublic returned error: %v", err)
	}
	if *admin != (StorefrontTrackingDTO{}) || *public != (StorefrontTrackingDTO{}) {
		t.Fatalf("admin = %+v, public = %+v, want empty defaults", admin, public)
	}
}
