package service

import "testing"

func TestValidateStrongPassword(t *testing.T) {
	for _, password := range []string{"shortA1!", "alllowercase123!", "ALLUPPERCASE123!", "NoDigitsHere!", "NoSpecial1234", "Has Space A1!"} {
		if ValidateStrongPassword(password, 12) {
			t.Fatalf("expected password to be rejected: %q", password)
		}
	}
	if !ValidateStrongPassword("StrongPass123!", 12) {
		t.Fatal("expected strong password to be accepted")
	}
}

func TestValidateRegistrationEmail(t *testing.T) {
	allowed := []string{"gmail.com", "example.cn"}
	if !ValidateRegistrationEmail("User@GMAIL.com", allowed) {
		t.Fatal("expected normalized allowed email")
	}
	for _, value := range []string{"plain-user", "user@unknown.test", "@gmail.com", "user @gmail.com"} {
		if ValidateRegistrationEmail(value, allowed) {
			t.Fatalf("expected email to be rejected: %q", value)
		}
	}
}
