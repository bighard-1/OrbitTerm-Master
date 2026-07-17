package identity

import "testing"

func TestCanonicalUsernameTrimsAndFoldsCase(t *testing.T) {
	if got, want := CanonicalUsername("  Orbit.User@Example.COM\n"), "orbit.user@example.com"; got != want {
		t.Fatalf("CanonicalUsername() = %q, want %q", got, want)
	}
}
