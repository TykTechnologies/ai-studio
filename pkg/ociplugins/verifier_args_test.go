package ociplugins

import (
	"context"
	"strings"
	"testing"
)

// Issue #397: values passed as positional arguments to the cosign subprocess
// must not be interpretable as flags (argument injection).

func TestValidateCosignArg(t *testing.T) {
	valid := []string{
		"registry.example.com/repo@sha256:abc",
		"/tmp/policy.yaml",
		"relative/path.pub",
	}
	for _, v := range valid {
		if err := validateCosignArg("test", v); err != nil {
			t.Errorf("expected %q to be accepted, got: %v", v, err)
		}
	}

	invalid := []string{
		"",
		"-o=/etc/passwd",
		"--policy=evil",
		"-",
	}
	for _, v := range invalid {
		if err := validateCosignArg("test", v); err == nil {
			t.Errorf("expected %q to be rejected", v)
		}
	}
}

func TestVerifyWithPolicyRejectsFlagLikePolicyPath(t *testing.T) {
	v, err := NewSignatureVerifier(&OCIConfig{})
	if err != nil {
		t.Fatal(err)
	}

	ref := &OCIReference{Registry: "registry.example.com", Repository: "repo", Tag: "v1"}
	err = v.VerifyWithPolicy(context.Background(), ref, "--output=/tmp/x")
	if err == nil {
		t.Fatal("expected error for flag-like policy path")
	}
	if !strings.Contains(err.Error(), "argument") {
		t.Fatalf("expected argument-validation error, got: %v", err)
	}
}
