package password

import "testing"

func TestHashAndVerify(t *testing.T) {
	h, err := Hash("StrongPass!2026")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if !Verify("StrongPass!2026", h) {
		t.Fatal("expected valid password to verify")
	}
	if Verify("WrongPass!2026", h) {
		t.Fatal("expected wrong password to fail verification")
	}
	if _, err := Hash("short"); err == nil {
		t.Fatal("expected short password to be rejected")
	}
}
