package service

import (
	"testing"

	"saas/internal/models"

	"gorm.io/datatypes"
)

func TestApplyTranslationFallbacksDoNotOverwriteOriginalValues(t *testing.T) {
	if got := ApplyTextTranslation("Original name", nil); got != "Original name" {
		t.Fatalf("ApplyTextTranslation nil = %q, want original", got)
	}
	if got := ApplyTextTranslation("Original name", &models.ContentTranslation{ValueText: "   "}); got != "Original name" {
		t.Fatalf("ApplyTextTranslation empty = %q, want original", got)
	}
	if got := ApplyTextTranslation("Original name", &models.ContentTranslation{ValueText: "中文名"}); got != "中文名" {
		t.Fatalf("ApplyTextTranslation translated = %q, want translation", got)
	}

	if got := ApplyHTMLTranslation("<p>Original</p>", &models.ContentTranslation{ValueHTML: "   "}); got != "<p>Original</p>" {
		t.Fatalf("ApplyHTMLTranslation empty = %q, want original", got)
	}
	if got := ApplyHTMLTranslation("<p>Original</p>", &models.ContentTranslation{ValueHTML: "<p>中文</p>"}); got != "<p>中文</p>" {
		t.Fatalf("ApplyHTMLTranslation translated = %q, want translation", got)
	}
}

func TestApplyJSONTranslationFallbacksDoNotOverwriteOriginalValues(t *testing.T) {
	original := datatypes.JSON(`{"name":"Size"}`)
	if got := ApplyJSONTranslation(original, nil); string(got) != string(original) {
		t.Fatalf("ApplyJSONTranslation nil = %q, want original", got)
	}
	if got := ApplyJSONTranslation(original, &models.ContentTranslation{ValueJSON: datatypes.JSON(`null`)}); string(got) != string(original) {
		t.Fatalf("ApplyJSONTranslation null = %q, want original", got)
	}
	if got := ApplyJSONTranslation(original, &models.ContentTranslation{ValueJSON: datatypes.JSON(`{"name":"尺寸"}`)}); string(got) != `{"name":"尺寸"}` {
		t.Fatalf("ApplyJSONTranslation translated = %q, want translation", got)
	}
}

func TestTranslationField(t *testing.T) {
	translations := map[string]models.ContentTranslation{
		"name": {ValueText: "Translated"},
	}
	if got := TranslationField(translations, "name"); got == nil || got.ValueText != "Translated" {
		t.Fatalf("TranslationField(name) = %+v, want translation", got)
	}
	if got := TranslationField(translations, "missing"); got != nil {
		t.Fatalf("TranslationField(missing) = %+v, want nil", got)
	}
}
