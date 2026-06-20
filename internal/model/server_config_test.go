package model

import "testing"

func TestServerConfigLifecycleState(t *testing.T) {
	for _, state := range []string{ServerConfigStateActive, ServerConfigStateDeleted, ServerConfigStatePurged} {
		if !IsValidServerConfigState(state) {
			t.Fatalf("expected state %q to be valid", state)
		}
	}
	if IsValidServerConfigState("unknown") {
		t.Fatal("unexpected unknown state to be valid")
	}

	if (ServerConfig{State: ServerConfigStateActive}).IsDeleted() {
		t.Fatal("active config must not be treated as deleted")
	}
	if !(ServerConfig{State: ServerConfigStateDeleted}).IsDeleted() {
		t.Fatal("deleted config must be treated as deleted")
	}
	if !(ServerConfig{State: ServerConfigStatePurged}).IsDeleted() {
		t.Fatal("purged config must be treated as deleted")
	}
}
