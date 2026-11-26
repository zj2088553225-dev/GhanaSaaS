package analytics

import (
	"context"
	"errors"
	"testing"

	"saas/internal/models"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type fakeMarketingContentPageLoader struct {
	page  *models.SitePage
	err   error
	calls int
}

func (loader *fakeMarketingContentPageLoader) FindByKey(_ context.Context, pageKey string) (*models.SitePage, error) {
	loader.calls++
	if pageKey != "home" {
		return nil, errors.New("unexpected page key")
	}
	return loader.page, loader.err
}

func TestMarketingContentResolverPublishedPriorityDraftFallbackAndRealCMSNodes(t *testing.T) {
	const (
		heroKey      = "11111111-1111-4111-8111-111111111111"
		slideKey     = "22222222-2222-4222-8222-222222222222"
		promoKey     = "33333333-3333-4333-8333-333333333333"
		imageKey     = "44444444-4444-4444-8444-444444444444"
		draftKey     = "55555555-5555-4555-8555-555555555555"
		categoryKey  = "66666666-6666-4666-8666-666666666666"
		contactKey   = "77777777-7777-4777-8777-777777777777"
		duplicateKey = "88888888-8888-4888-8888-888888888888"
		linkKey      = "99999999-9999-4999-8999-999999999999"
	)
	page := &models.SitePage{
		PageKey: "home",
		PublishedContent: datatypes.JSON(`{"blocks":[
            {"type":"hero","tracking_key":"` + heroKey + `","title":"English Hero","backgroundImage":"/hero.webp","buttonLink":"/products/lamp-196"},
            {"type":"hero_carousel","tracking_key":"99999999-9999-4999-8999-999999999999","slides":[{"tracking_key":"` + slideKey + `","title":"Slide A","image":"/slide.webp","link":"https://example.com/deal"}]},
            {"type":"promo_banner","tracking_key":"` + promoKey + `","title":"Promo","image":"/promo.webp","buttonLink":"/mall?category_id=42"},
            {"type":"image","tracking_key":"` + imageKey + `","alt":"Image alternative","image":"/image.webp"},
            {"type":"category_nav","tracking_key":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","items":[{"tracking_key":"` + categoryKey + `","name":"Phones","image":"/phone.webp","link":"/mall?category_id=9"}]},
            {"type":"contact","tracking_key":"` + contactKey + `","title":"Contact us","phone":"+233000","whatsapp":"233111"},
            {"type":"footer_links","tracking_key":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","items":[{"tracking_key":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb","title":"Marketplace","links":[{"tracking_key":"` + linkKey + `","name":"Shop all","link":"/mall"}]}]},
            {"type":"promo_banner","tracking_key":"` + duplicateKey + `","title":"First published"},
            {"type":"promo_banner","tracking_key":"` + duplicateKey + `","title":"Second published"}
        ]}`),
		DraftContent: datatypes.JSON(`{"blocks":[
            {"type":"hero","tracking_key":"` + heroKey + `","title":"Draft must not override"},
            {"type":"image_text","tracking_key":"` + draftKey + `","title":"Draft only","image":"/draft.webp","buttonLink":"tel:+233555"}
        ]}`),
	}
	originalPublished := append([]byte(nil), page.PublishedContent...)
	originalDraft := append([]byte(nil), page.DraftContent...)
	loader := &fakeMarketingContentPageLoader{page: page}
	resolver := NewMarketingContentResolver(loader)

	metadata, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if loader.calls != 1 {
		t.Fatalf("CMS load calls = %d, want 1", loader.calls)
	}
	if len(metadata) != 9 {
		t.Fatalf("metadata count = %d: %+v", len(metadata), metadata)
	}
	hero := metadata[heroKey]
	if hero.Title != "English Hero" || !hero.Published || hero.ContentType != ContentTypeHero || hero.ImageURL != "/hero.webp" || hero.TargetType != ContentTargetProduct || hero.TargetValue != "lamp-196" {
		t.Fatalf("published hero metadata = %+v", hero)
	}
	slide := metadata[slideKey]
	if slide.ContentType != ContentTypeHeroSlide || slide.ImageURL != "/slide.webp" || slide.TargetType != ContentTargetExternal {
		t.Fatalf("hero slide metadata = %+v", slide)
	}
	promo := metadata[promoKey]
	if promo.ContentType != ContentTypePromoBanner || promo.TargetType != ContentTargetCategory || promo.TargetValue != "42" {
		t.Fatalf("promo metadata = %+v", promo)
	}
	image := metadata[imageKey]
	if image.Title != "Image alternative" || image.ContentType != ContentTypeImage || image.TargetType != ContentTargetNone {
		t.Fatalf("image metadata = %+v", image)
	}
	draft := metadata[draftKey]
	if draft.Published || draft.ContentType != ContentTypeImageText || draft.TargetType != ContentTargetPhone || draft.TargetValue != "+233555" {
		t.Fatalf("draft fallback metadata = %+v", draft)
	}
	category := metadata[categoryKey]
	if category.ContentType != ContentTypeCategoryItem || category.Title != "Phones" || category.TargetType != ContentTargetCategory || category.TargetValue != "9" {
		t.Fatalf("category item metadata = %+v", category)
	}
	contact := metadata[contactKey]
	if contact.ContentType != ContentTypeContact || contact.TargetType != ContentTargetWhatsApp || contact.TargetValue != "233111" {
		t.Fatalf("contact metadata = %+v", contact)
	}
	if metadata[duplicateKey].Title != "First published" {
		t.Fatalf("duplicate tracking key was not stable-first: %+v", metadata[duplicateKey])
	}
	if link := metadata[linkKey]; link.ContentType != ContentTypeLink || link.Title != "Shop all" || link.TargetType != ContentTargetInternal || link.TargetValue != "/mall" {
		t.Fatalf("CMS link metadata = %+v", link)
	}
	if string(page.PublishedContent) != string(originalPublished) || string(page.DraftContent) != string(originalDraft) {
		t.Fatal("resolver mutated CMS JSON")
	}
}

func TestMarketingContentResolverSkipsMalformedNodesDoesNotGenerateKeysAndNormalizesUnknown(t *testing.T) {
	const (
		validKey   = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
		unknownKey = "bbbbbbbb-cccc-4ddd-8eee-ffffffffffff"
	)
	loader := &fakeMarketingContentPageLoader{page: &models.SitePage{
		PageKey: "home",
		PublishedContent: datatypes.JSON(`{"blocks":[
            "malformed",
            {"type":"hero_carousel","tracking_key":"11111111-1111-4111-8111-111111111111","slides":[null,"bad",{"tracking_key":7},{"tracking_key":"not-a-uuid"},{"tracking_key":"` + validKey + `"}]},
            {"type":"future_clickable","tracking_key":"` + unknownKey + `","label":"Future"},
            {"type":"promo_banner","title":"Missing key"},
            {"type":"footer_links","tracking_key":"22222222-2222-4222-8222-222222222222","items":["bad",{"tracking_key":"33333333-3333-4333-8333-333333333333","links":[null,{"tracking_key":"bad"}]}]}
        ]}`),
	}}
	metadata, err := NewMarketingContentResolver(loader).Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(metadata) != 2 {
		t.Fatalf("malformed metadata = %+v", metadata)
	}
	if slide := metadata[validKey]; slide.ContentType != ContentTypeHeroSlide || slide.Title != "Hero slide" {
		t.Fatalf("slide fallback = %+v", slide)
	}
	if unknown := metadata[unknownKey]; unknown.ContentType != ContentTypeOther || unknown.Title != "Future" {
		t.Fatalf("unknown type metadata = %+v", unknown)
	}
	if loader.calls != 1 {
		t.Fatalf("loader calls = %d", loader.calls)
	}
}

func TestMarketingContentResolverMissingOrMalformedCMSIsUnavailableWithoutWrites(t *testing.T) {
	for _, test := range []struct {
		name   string
		loader *fakeMarketingContentPageLoader
	}{
		{name: "missing", loader: &fakeMarketingContentPageLoader{err: gorm.ErrRecordNotFound}},
		{name: "malformed", loader: &fakeMarketingContentPageLoader{page: &models.SitePage{PublishedContent: datatypes.JSON(`{"blocks":`)}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			metadata, err := NewMarketingContentResolver(test.loader).Resolve(context.Background())
			if err != nil || len(metadata) != 0 || test.loader.calls != 1 {
				t.Fatalf("Resolve = %+v err=%v calls=%d", metadata, err, test.loader.calls)
			}
		})
	}
}
