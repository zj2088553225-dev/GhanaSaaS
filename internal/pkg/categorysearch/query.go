package categorysearch

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaxQueryRunes = 256
	MaxTokenRunes = 64
	MaxTokens     = 8
)

var ErrInvalidQuery = errors.New("invalid platform category search query")

type Query struct {
	Normalized string
	Tokens     []string
}

func Parse(value string) (Query, error) {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) > MaxQueryRunes {
		return Query{}, ErrInvalidQuery
	}

	var normalized strings.Builder
	normalized.Grow(len(value))
	previousSeparator := true
	for _, current := range strings.ToLower(value) {
		if categorySearchSeparator(current) {
			if !previousSeparator {
				normalized.WriteByte(' ')
				previousSeparator = true
			}
			continue
		}
		normalized.WriteRune(current)
		previousSeparator = false
	}

	parts := strings.Fields(normalized.String())
	if len(parts) == 0 {
		return Query{}, nil
	}
	tokens := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, token := range parts {
		if utf8.RuneCountInString(token) > MaxTokenRunes {
			return Query{}, ErrInvalidQuery
		}
		if _, exists := seen[token]; exists {
			continue
		}
		if len(tokens) >= MaxTokens {
			return Query{}, ErrInvalidQuery
		}
		seen[token] = struct{}{}
		tokens = append(tokens, token)
	}
	return Query{Normalized: strings.Join(tokens, " "), Tokens: tokens}, nil
}

func NormalizeCategorySearchQuery(value string) (string, error) {
	query, err := Parse(value)
	return query.Normalized, err
}

func TokenizeCategorySearchQuery(value string) ([]string, error) {
	query, err := Parse(value)
	return query.Tokens, err
}

func EscapeLikePattern(value string) string {
	return strings.NewReplacer(
		`\`, `\\`,
		`%`, `\%`,
		`_`, `\_`,
	).Replace(value)
}

func categorySearchSeparator(value rune) bool {
	if unicode.IsSpace(value) || unicode.IsControl(value) {
		return true
	}
	switch value {
	case '>', '<', '/', '\\', ',', '，', ';', '；', '(', ')', '（', '）',
		'[', ']', '【', '】', '{', '}', '、', '。', '.', '!', '！', '?', '？',
		':', '：', '"', '\'', '“', '”', '‘', '’', '|', '｜', '《', '》', '·',
		'-', '－', '—', '–', '+', '=', '&', '＆':
		return true
	default:
		return false
	}
}
