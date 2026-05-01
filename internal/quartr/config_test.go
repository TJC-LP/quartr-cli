package quartr

import (
	"testing"
	"time"
)

func TestEffectiveTimeoutDefaultsInvalidValues(t *testing.T) {
	for _, input := range []string{"", "bad", "-1s"} {
		if got := EffectiveTimeout(input); got != 30*time.Second {
			t.Fatalf("EffectiveTimeout(%q): expected 30s, got %s", input, got)
		}
	}
}

func TestMaskKey(t *testing.T) {
	tests := map[string]string{
		"":           "",
		"short":      "*****",
		"abcd1234":   "********",
		"abcd1234ef": "abcd**34ef",
	}
	for input, want := range tests {
		if got := MaskKey(input); got != want {
			t.Fatalf("MaskKey(%q): expected %q, got %q", input, want, got)
		}
	}
}
