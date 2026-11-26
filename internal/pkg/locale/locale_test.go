package locale

import "testing"

func TestNormalizeLocale(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: "en"},
		{name: "en", input: "en", want: "en"},
		{name: "zh", input: "zh", want: "zh"},
		{name: "trim and lowercase", input: " ZH ", want: "zh"},
		{name: "zh-cn is not supported in phase 2", input: "zh-CN", want: "en"},
		{name: "bad locale", input: "bad", want: "en"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeLocale(tt.input); got != tt.want {
				t.Fatalf("NormalizeLocale(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsSupportedLocale(t *testing.T) {
	if !IsSupportedLocale("en") || !IsSupportedLocale("zh") {
		t.Fatal("en and zh should be supported")
	}
	if IsSupportedLocale("zh-CN") || IsSupportedLocale("fr") || IsSupportedLocale("") {
		t.Fatal("unsupported locales should be rejected")
	}
}
