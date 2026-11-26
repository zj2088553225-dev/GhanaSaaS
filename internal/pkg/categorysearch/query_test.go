package categorysearch

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseCategorySearchQuery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		input      string
		normalized string
		tokens     []string
	}{
		{name: "mixed", input: "  手机\u3000Mobile  ", normalized: "手机 mobile", tokens: []string{"手机", "mobile"}},
		{name: "angle path", input: "电子产品 > 手机", normalized: "电子产品 手机", tokens: []string{"电子产品", "手机"}},
		{name: "slash path", input: "电子产品 / 手机", normalized: "电子产品 手机", tokens: []string{"电子产品", "手机"}},
		{name: "backslash path", input: `电子产品\手机`, normalized: "电子产品 手机", tokens: []string{"电子产品", "手机"}},
		{name: "punctuation", input: "手机，Mobile；（Accessories）", normalized: "手机 mobile accessories", tokens: []string{"手机", "mobile", "accessories"}},
		{name: "unicode whitespace", input: "phone\u00a0\taccessories", normalized: "phone accessories", tokens: []string{"phone", "accessories"}},
		{name: "deduplicate", input: "Mobile mobile 手机 手机", normalized: "mobile 手机", tokens: []string{"mobile", "手机"}},
		{name: "like literals retained", input: "100% code_name", normalized: "100% code_name", tokens: []string{"100%", "code_name"}},
		{name: "punctuation only", input: " > / \\ ，；（） ", normalized: "", tokens: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			query, err := Parse(test.input)
			if err != nil {
				t.Fatalf("Parse returned error: %v", err)
			}
			if query.Normalized != test.normalized {
				t.Fatalf("normalized = %q, want %q", query.Normalized, test.normalized)
			}
			if !reflect.DeepEqual(query.Tokens, test.tokens) {
				t.Fatalf("tokens = %#v, want %#v", query.Tokens, test.tokens)
			}
		})
	}
}

func TestCategorySearchQueryLimits(t *testing.T) {
	t.Parallel()
	for name, input := range map[string]string{
		"total query":  strings.Repeat("手", MaxQueryRunes+1),
		"single token": strings.Repeat("a", MaxTokenRunes+1),
		"token count":  "one two three four five six seven eight nine",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := Parse(input); !errors.Is(err, ErrInvalidQuery) {
				t.Fatalf("Parse error = %v, want ErrInvalidQuery", err)
			}
		})
	}
}

func TestCategorySearchQueryHelpers(t *testing.T) {
	t.Parallel()
	normalized, err := NormalizeCategorySearchQuery(" 手机 / MOBILE ")
	if err != nil || normalized != "手机 mobile" {
		t.Fatalf("NormalizeCategorySearchQuery = %q, %v", normalized, err)
	}
	tokens, err := TokenizeCategorySearchQuery("手机 Mobile")
	if err != nil || !reflect.DeepEqual(tokens, []string{"手机", "mobile"}) {
		t.Fatalf("TokenizeCategorySearchQuery = %#v, %v", tokens, err)
	}
	if escaped := EscapeLikePattern(`100%_\sale`); escaped != `100\%\_\\sale` {
		t.Fatalf("EscapeLikePattern = %q", escaped)
	}
}
