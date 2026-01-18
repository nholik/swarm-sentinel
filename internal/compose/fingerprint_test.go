package compose

import "testing"

func TestFingerprint_Stable(t *testing.T) {
	body := []byte("version: '3.9'\nservices:\n  web:\n    image: nginx\n")

	first, err := Fingerprint(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := Fingerprint(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first != second {
		t.Fatalf("expected stable fingerprint")
	}
}

func TestFingerprint_DifferentInputs(t *testing.T) {
	first, err := Fingerprint([]byte("compose: one\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := Fingerprint([]byte("compose: two\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first == second {
		t.Fatalf("expected different fingerprints")
	}
}

func TestFingerprint_RejectsEmpty(t *testing.T) {
	if _, err := Fingerprint(nil); err == nil {
		t.Fatalf("expected error for empty body")
	}
}
