package service

import (
	"strings"
	"testing"
)

func TestSanitizeProductDetailHTML(t *testing.T) {
	input := `<script>alert(1)</script><p onclick="evil()">Hello</p><img src="/uploads/detail.webp" onerror="evil()"><a href="javascript:alert(1)">bad</a><img src="https://example.com/long.jpg">`
	got := sanitizeProductDetailHTML(input)

	for _, forbidden := range []string{"<script", "alert(1)", "onclick", "onerror", "javascript:"} {
		if strings.Contains(strings.ToLower(got), strings.ToLower(forbidden)) {
			t.Fatalf("sanitized HTML contains %q: %s", forbidden, got)
		}
	}
	for _, retained := range []string{"<p>Hello</p>", `/uploads/detail.webp`, `https://example.com/long.jpg`} {
		if !strings.Contains(got, retained) {
			t.Fatalf("sanitized HTML missing %q: %s", retained, got)
		}
	}
}
