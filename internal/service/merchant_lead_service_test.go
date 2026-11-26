package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"saas/internal/models"

	"gorm.io/gorm"
)

type fakeMerchantLeadRepository struct {
	createdLead   *models.MerchantLead
	leads         []models.MerchantLead
	updatedID     uint
	updatedStatus models.MerchantLeadStatus
	updateErr     error
}

func (r *fakeMerchantLeadRepository) Create(_ context.Context, lead *models.MerchantLead) error {
	r.createdLead = lead
	lead.ID = 1
	return nil
}

func (r *fakeMerchantLeadRepository) List(context.Context) ([]models.MerchantLead, error) {
	return r.leads, nil
}

func (r *fakeMerchantLeadRepository) UpdateStatus(_ context.Context, id uint, status models.MerchantLeadStatus) (*models.MerchantLead, error) {
	r.updatedID = id
	r.updatedStatus = status
	if r.updateErr != nil {
		return nil, r.updateErr
	}
	return &models.MerchantLead{
		BaseModel: models.BaseModel{ID: id},
		Status:    status,
	}, nil
}

func TestMerchantLeadCreateNormalizesAndStoresLead(t *testing.T) {
	repo := &fakeMerchantLeadRepository{}
	service := NewMerchantLeadService(repo)

	lead, err := service.Create(context.Background(), MerchantLeadInput{
		ContactName:    " Alice ",
		ShopName:       " ABC Store ",
		Whatsapp:       " +233500000000 ",
		Phone:          " 0300000000 ",
		Category:       " Electronics ",
		HasOfflineShop: true,
		City:           " Accra ",
		ProductCount:   " 100-500 ",
		Message:        " I want to join. ",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if repo.createdLead == nil {
		t.Fatal("Create did not store lead")
	}
	if lead.ContactName != "Alice" || lead.ShopName != "ABC Store" || lead.Category != "Electronics" {
		t.Fatalf("lead fields were not normalized: %+v", lead)
	}
	if lead.Status != models.MerchantLeadStatusNew {
		t.Fatalf("status = %q, want %q", lead.Status, models.MerchantLeadStatusNew)
	}
	if !lead.HasOfflineShop {
		t.Fatal("has_offline_shop was not preserved")
	}
}

func TestMerchantLeadCreateRejectsMissingRequiredFields(t *testing.T) {
	service := NewMerchantLeadService(&fakeMerchantLeadRepository{})

	cases := []MerchantLeadInput{
		{ShopName: "ABC", Whatsapp: "+233", Category: "Retail"},
		{ContactName: "Alice", Whatsapp: "+233", Category: "Retail"},
		{ContactName: "Alice", ShopName: "ABC", Category: "Retail"},
		{ContactName: "Alice", ShopName: "ABC", Whatsapp: "+233"},
	}
	for _, input := range cases {
		if _, err := service.Create(context.Background(), input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("Create(%+v) error = %v, want invalid input", input, err)
		}
	}
}

func TestMerchantLeadCreateRejectsOverlongFields(t *testing.T) {
	service := NewMerchantLeadService(&fakeMerchantLeadRepository{})
	input := MerchantLeadInput{
		ContactName: "Alice",
		ShopName:    "ABC Store",
		Whatsapp:    "+233500000000",
		Category:    "Retail",
		Message:     strings.Repeat("x", maxMerchantLeadMessageLen+1),
	}

	if _, err := service.Create(context.Background(), input); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Create returned error = %v, want invalid input", err)
	}
}

func TestMerchantLeadUpdateStatusAcceptsOnlyValidStatuses(t *testing.T) {
	repo := &fakeMerchantLeadRepository{}
	service := NewMerchantLeadService(repo)

	if _, err := service.UpdateStatus(context.Background(), 12, models.MerchantLeadStatusContacted); err != nil {
		t.Fatalf("UpdateStatus returned error: %v", err)
	}
	if repo.updatedID != 12 || repo.updatedStatus != models.MerchantLeadStatusContacted {
		t.Fatalf("updated id/status = %d/%q, want 12/contacted", repo.updatedID, repo.updatedStatus)
	}

	if _, err := service.UpdateStatus(context.Background(), 12, models.MerchantLeadStatus("done")); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("UpdateStatus invalid status error = %v, want invalid input", err)
	}
}

func TestMerchantLeadUpdateStatusMapsNotFound(t *testing.T) {
	service := NewMerchantLeadService(&fakeMerchantLeadRepository{updateErr: gorm.ErrRecordNotFound})

	if _, err := service.UpdateStatus(context.Background(), 12, models.MerchantLeadStatusRejected); !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateStatus error = %v, want not found", err)
	}
}
