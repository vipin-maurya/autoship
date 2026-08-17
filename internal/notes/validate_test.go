package notes

import (
	"strings"
	"testing"
)

func TestValidate_RejectsOversize(t *testing.T) {
	err := Validate(strings.Repeat("a", MaxLen+1))
	if err == nil {
		t.Fatal("Validate = nil error, want one")
	}
	if !strings.Contains(err.Error(), "501") || !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %v, want it to name both the actual and the limit", err)
	}
}

func TestValidate_AcceptsAtLimit(t *testing.T) {
	if err := Validate(strings.Repeat("a", MaxLen)); err != nil {
		t.Errorf("Validate at the limit = %v, want nil", err)
	}
}

func TestValidate_CountsRunesNotBytes(t *testing.T) {
	// 500 multi-byte characters is 1500 bytes but still within Play's limit.
	if err := Validate(strings.Repeat("é", MaxLen)); err != nil {
		t.Errorf("Validate = %v, want nil (runes, not bytes)", err)
	}
}

func TestValidate_RejectsEmpty(t *testing.T) {
	for _, s := range []string{"", "   ", "\n\t"} {
		if err := Validate(s); err == nil {
			t.Errorf("Validate(%q) = nil error, want one", s)
		}
	}
}
