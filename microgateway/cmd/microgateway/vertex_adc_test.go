package main

import (
	"testing"

	vertexVendor "github.com/TykTechnologies/midsommar/v2/vendors/vertex"
)

// The edge reads the host-credential opt-in through the same vendor code,
// under the same variable name, as AI Studio.
func TestVertexADCSettingSharedWithStudio(t *testing.T) {
	if vertexVendor.ADCSetting != "AI_STUDIO_VERTEX_ADC" {
		t.Fatalf("setting renamed to %q: update the edge docs", vertexVendor.ADCSetting)
	}
	t.Setenv(vertexVendor.ADCSetting, "true")
	if !vertexVendor.ADCEnabled() {
		t.Fatal("AI_STUDIO_VERTEX_ADC=true not seen on the edge")
	}
	t.Setenv(vertexVendor.ADCSetting, "")
	if vertexVendor.ADCEnabled() {
		t.Fatal("ADC must be off by default")
	}
}
