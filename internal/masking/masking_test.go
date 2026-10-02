package masking

import (
	"strings"
	"testing"
)

func TestApplyRule(t *testing.T) {
	// Null rule
	val, err := ApplyRule("secret", "null")
	if err != nil || val != nil {
		t.Fatalf("expected nil for null rule, got %v", val)
	}

	// Constant rule
	val, err = ApplyRule("secret", "constant.dummy_value")
	if err != nil || val != "dummy_value" {
		t.Fatalf("expected dummy_value, got %v", val)
	}

	// Mask keep first last
	masked := MaskKeepFirstLast("13812345678", 3, 4)
	if !strings.HasPrefix(masked, "138") || !strings.HasSuffix(masked, "5678") {
		t.Errorf("expected masked phone, got %s", masked)
	}

	// Faker email
	email, err := ApplyRule("admin@corp.com", "faker.email")
	if err != nil || !strings.Contains(email.(string), "@") {
		t.Errorf("expected valid fake email, got %v", email)
	}
}

func TestDeterministicHashConsistency(t *testing.T) {
	// Same input MUST produce exact same output across different tables / calls
	h1 := DeterministicHash("user_1001@example.com", DefaultFixedSalt)
	h2 := DeterministicHash("user_1001@example.com", DefaultFixedSalt)

	if h1 != h2 {
		t.Fatalf("deterministic hash failed: %v != %v", h1, h2)
	}

	// Different input should produce different output
	h3 := DeterministicHash("user_1002@example.com", DefaultFixedSalt)
	if h1 == h3 {
		t.Fatalf("hash collision between distinct inputs: %v == %v", h1, h3)
	}

	// Integer hash consistency
	id1 := DeterministicHash(int64(987654), DefaultFixedSalt)
	id2 := DeterministicHash(int64(987654), DefaultFixedSalt)
	if id1 != id2 {
		t.Fatalf("integer deterministic hash failed: %v != %v", id1, id2)
	}
}
