package service

import "testing"

func TestValidateAIAgentCapacityUnlimited(t *testing.T) {
	if err := validateAIAgentCapacity(100, 0); err != nil {
		t.Fatalf("expected zero limit to allow unlimited agents, got %v", err)
	}
}

func TestValidateAIAgentCapacityBelowLimit(t *testing.T) {
	if err := validateAIAgentCapacity(4, 5); err != nil {
		t.Fatalf("expected capacity below the limit, got %v", err)
	}
}

func TestValidateAIAgentCapacityReached(t *testing.T) {
	if err := validateAIAgentCapacity(5, 5); err == nil {
		t.Fatal("expected capacity at the configured limit to be rejected")
	}
}

func TestAgentInstallHooksSkipAppMetadataLimit(t *testing.T) {
	if !shouldCheckAppLimit(nil) {
		t.Fatal("expected regular app installs to enforce app store metadata limits")
	}
	if !shouldCheckAppLimit(&appInstallHooks{}) {
		t.Fatal("expected default hooks to enforce app store metadata limits")
	}
	if shouldCheckAppLimit(&appInstallHooks{SkipAppLimit: true}) {
		t.Fatal("expected AI agent installs to bypass app store metadata limits")
	}
}
