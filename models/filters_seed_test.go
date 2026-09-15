package models

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGetOrCreateDefaultFilters(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_seed_filters=1"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Filter{}); err != nil {
		t.Fatal(err)
	}

	if err := GetOrCreateDefaultFilters(db); err != nil {
		t.Fatalf("GetOrCreateDefaultFilters() error = %v", err)
	}
	var filters []Filter
	db.Find(&filters)
	if len(filters) != len(DefaultFilters()) {
		t.Fatalf("seeded %d filters, want %d", len(filters), len(DefaultFilters()))
	}
	for _, f := range filters {
		if f.Kind != FilterKindGuardrail {
			t.Errorf("%s: kind = %q", f.Name, f.Kind)
		}
		// Stored normalised: the direction-dependent defaults are written out.
		if f.Config["fail_mode"] == nil || f.Config["scope"] == nil || f.Config["block_message"] == nil {
			t.Errorf("%s: config not normalised: %v", f.Name, f.Config)
		}
		want := guardrails.FailClosed
		if f.ResponseFilter {
			want = guardrails.FailOpen
		}
		if f.Config["fail_mode"] != want {
			t.Errorf("%s: fail_mode = %v, want %s", f.Name, f.Config["fail_mode"], want)
		}
	}

	// Idempotent, and a renamed or soft-deleted filter is not recreated.
	db.Model(&Filter{}).Where("name = ?", DefaultFilters()[0].Name).Update("description", "edited")
	db.Where("name = ?", DefaultFilters()[1].Name).Delete(&Filter{})
	if err := GetOrCreateDefaultFilters(db); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.Unscoped().Model(&Filter{}).Count(&count)
	if count != int64(len(DefaultFilters())) {
		t.Errorf("second run created rows: %d total including soft-deleted, want %d", count, len(DefaultFilters()))
	}
	var edited Filter
	db.Where("name = ?", DefaultFilters()[0].Name).First(&edited)
	if edited.Description != "edited" {
		t.Error("second run overwrote an edited filter")
	}
}
