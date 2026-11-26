package service

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"saas/internal/models"

	"gorm.io/gorm"
)

const (
	maxMerchantLeadContactNameLen  = 120
	maxMerchantLeadShopNameLen     = 160
	maxMerchantLeadWhatsappLen     = 64
	maxMerchantLeadPhoneLen        = 64
	maxMerchantLeadCategoryLen     = 120
	maxMerchantLeadCityLen         = 120
	maxMerchantLeadProductCountLen = 64
	maxMerchantLeadMessageLen      = 1000
)

type MerchantLeadInput struct {
	ContactName    string `json:"contact_name"`
	ShopName       string `json:"shop_name"`
	Whatsapp       string `json:"whatsapp"`
	Phone          string `json:"phone"`
	Category       string `json:"category"`
	HasOfflineShop bool   `json:"has_offline_shop"`
	City           string `json:"city"`
	ProductCount   string `json:"product_count"`
	Message        string `json:"message"`
}

type merchantLeadRepository interface {
	Create(ctx context.Context, lead *models.MerchantLead) error
	List(ctx context.Context) ([]models.MerchantLead, error)
	UpdateStatus(ctx context.Context, id uint, status models.MerchantLeadStatus) (*models.MerchantLead, error)
}

type MerchantLeadService struct {
	repo merchantLeadRepository
}

func NewMerchantLeadService(repo merchantLeadRepository) *MerchantLeadService {
	return &MerchantLeadService{repo: repo}
}

func (s *MerchantLeadService) Create(ctx context.Context, input MerchantLeadInput) (*models.MerchantLead, error) {
	contactName, err := normalizeMerchantLeadText(input.ContactName, maxMerchantLeadContactNameLen, true)
	if err != nil {
		return nil, err
	}
	shopName, err := normalizeMerchantLeadText(input.ShopName, maxMerchantLeadShopNameLen, true)
	if err != nil {
		return nil, err
	}
	whatsapp, err := normalizeMerchantLeadText(input.Whatsapp, maxMerchantLeadWhatsappLen, true)
	if err != nil {
		return nil, err
	}
	phone, err := normalizeMerchantLeadText(input.Phone, maxMerchantLeadPhoneLen, false)
	if err != nil {
		return nil, err
	}
	category, err := normalizeMerchantLeadText(input.Category, maxMerchantLeadCategoryLen, true)
	if err != nil {
		return nil, err
	}
	city, err := normalizeMerchantLeadText(input.City, maxMerchantLeadCityLen, false)
	if err != nil {
		return nil, err
	}
	productCount, err := normalizeMerchantLeadText(input.ProductCount, maxMerchantLeadProductCountLen, false)
	if err != nil {
		return nil, err
	}
	message, err := normalizeMerchantLeadText(input.Message, maxMerchantLeadMessageLen, false)
	if err != nil {
		return nil, err
	}

	lead := &models.MerchantLead{
		ContactName:    contactName,
		ShopName:       shopName,
		Whatsapp:       whatsapp,
		Phone:          phone,
		Category:       category,
		HasOfflineShop: input.HasOfflineShop,
		City:           city,
		ProductCount:   productCount,
		Message:        message,
		Status:         models.MerchantLeadStatusNew,
	}
	if err := s.repo.Create(ctx, lead); err != nil {
		return nil, err
	}
	return lead, nil
}

func (s *MerchantLeadService) List(ctx context.Context) ([]models.MerchantLead, error) {
	return s.repo.List(ctx)
}

func (s *MerchantLeadService) UpdateStatus(ctx context.Context, id uint, status models.MerchantLeadStatus) (*models.MerchantLead, error) {
	if id == 0 || !ValidMerchantLeadStatus(status) {
		return nil, ErrInvalidInput
	}
	lead, err := s.repo.UpdateStatus(ctx, id, status)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return lead, nil
}

func ValidMerchantLeadStatus(status models.MerchantLeadStatus) bool {
	switch status {
	case models.MerchantLeadStatusNew,
		models.MerchantLeadStatusContacted,
		models.MerchantLeadStatusApproved,
		models.MerchantLeadStatusRejected:
		return true
	default:
		return false
	}
}

func normalizeMerchantLeadText(value string, maxLen int, required bool) (string, error) {
	value = strings.TrimSpace(value)
	if required && value == "" {
		return "", ErrInvalidInput
	}
	if utf8.RuneCountInString(value) > maxLen {
		return "", ErrInvalidInput
	}
	return value, nil
}
