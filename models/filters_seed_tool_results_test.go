package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

const toolResultsFilterName = "Prompt injection in tool results (quarantine)"

func seedTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Filter{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// A fresh install seeds the tool-result filter unattached: built-in
// injection heuristics, a matching user message only logged, a matching
// tool result quarantined.
func TestDefaultFilters_ToolResultsFilterOnFreshInstall(t *testing.T) {
	db := seedTestDB(t, "seed_tool_results_fresh")
	if err := GetOrCreateDefaultFilters(db); err != nil {
		t.Fatal(err)
	}
	var f Filter
	if err := db.Where("name = ?", toolResultsFilterName).First(&f).Error; err != nil {
		t.Fatalf("the tool-result filter was not seeded: %v", err)
	}
	if f.Kind != FilterKindGuardrail || f.ResponseFilter {
		t.Errorf("kind %q response %v, want a request guardrail", f.Kind, f.ResponseFilter)
	}
	cfg, err := guardrails.ParseConfig(f.Config)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider != guardrails.ProviderBuiltin || cfg.OnDetect != guardrails.ActionLog {
		t.Errorf("provider %q on_detect %q, want builtin and log", cfg.Provider, cfg.OnDetect)
	}
	if !cfg.ScansToolResults() {
		t.Error("the tool-result filter does not scan tool results")
	}
	if got := cfg.ActionFor(guardrails.KindToolResult); got != guardrails.ActionQuarantine {
		t.Errorf("tool-result action %q, want quarantine", got)
	}
	if got := cfg.ActionFor(guardrails.KindText); got != guardrails.ActionLog {
		t.Errorf("user-text action %q, want log", got)
	}
	// Stored normalised, like the others, so it survives the admin API and
	// the edge snapshot unchanged.
	if _, err := guardrails.Normalize(cfg, false); err != nil {
		t.Errorf("stored config does not normalise: %v", err)
	}
	// Unattached: no LLM, chat or tool references it (the join tables are not
	// even touched by seeding).
	for _, table := range []string{"llm_filters", "chat_filters", "tool_filters"} {
		if db.Migrator().HasTable(table) {
			var n int64
			db.Table(table).Where("filter_id = ?", f.ID).Count(&n)
			if n != 0 {
				t.Errorf("%s references the seeded filter", table)
			}
		}
	}
}

// An install from before v2.3 already has the four earlier seeded filters,
// possibly edited, renamed away or deleted. Starting v2.3 adds the new
// filter and leaves every existing row exactly as it was.
func TestDefaultFilters_ToolResultsFilterOnUpgrade(t *testing.T) {
	db := seedTestDB(t, "seed_tool_results_upgrade")

	// The v2.2 seed: the four filters as that release stored them
	// (normalised, without tool_results, quarantine or budget keys).
	v22 := map[string]JSONMap{
		"Credentials in prompts (block)":           {"provider": "builtin", "detectors": []any{map[string]any{"name": "secrets"}}, "on_detect": "block", "scope": "all_user", "fail_mode": "closed", "block_message": "Request blocked by policy"},
		"Personal data before the vendor (redact)": {"provider": "builtin", "detectors": []any{map[string]any{"name": "pii"}}, "on_detect": "redact", "scope": "all_user", "fail_mode": "closed", "block_message": "Request blocked by policy", "redaction": map[string]any{"style": "placeholder", "placeholder": "[REDACTED:{{type}}]"}},
		"Prompt injection heuristics (log)":        {"provider": "builtin", "detectors": []any{map[string]any{"name": "injection"}}, "on_detect": "log", "scope": "all_user", "fail_mode": "closed", "block_message": "Request blocked by policy"},
		"Credential leakage in responses (block)":  {"provider": "builtin", "detectors": []any{map[string]any{"name": "secrets"}, map[string]any{"name": "leak"}}, "on_detect": "block", "scope": "all_messages", "fail_mode": "open", "block_message": "Response blocked by policy"},
	}
	stamp := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	for name, cfg := range v22 {
		f := Filter{Name: name, Description: "v2.2", Kind: FilterKindGuardrail, Config: cfg, ResponseFilter: name == "Credential leakage in responses (block)"}
		if err := db.Create(&f).Error; err != nil {
			t.Fatal(err)
		}
		db.Model(&Filter{}).Where("id = ?", f.ID).UpdateColumn("updated_at", stamp)
	}
	// An admin edited one and deleted another.
	db.Model(&Filter{}).Where("name = ?", "Prompt injection heuristics (log)").UpdateColumns(map[string]any{"description": "ours now", "updated_at": stamp})
	db.Where("name = ?", "Personal data before the vendor (redact)").Delete(&Filter{})

	type snapshot struct {
		Description string
		Config      string
		UpdatedAt   time.Time
		Deleted     bool
	}
	read := func() map[string]snapshot {
		var rows []Filter
		db.Unscoped().Find(&rows)
		out := map[string]snapshot{}
		for _, r := range rows {
			b, _ := json.Marshal(r.Config)
			out[r.Name] = snapshot{r.Description, string(b), r.UpdatedAt.UTC(), r.DeletedAt.Valid}
		}
		return out
	}
	before := read()

	if err := GetOrCreateDefaultFilters(db); err != nil {
		t.Fatal(err)
	}
	after := read()

	if _, ok := after[toolResultsFilterName]; !ok {
		t.Fatal("upgrade did not add the tool-result filter")
	}
	for name, b := range before {
		a, ok := after[name]
		if !ok || a != b {
			t.Errorf("%s changed on upgrade:\n before %+v\n after  %+v", name, b, a)
		}
	}
	if len(after) != len(before)+1 {
		t.Errorf("%d rows after upgrade, want %d (the deleted filter is not resurrected)", len(after), len(before)+1)
	}

	// A second start adds nothing more.
	if err := GetOrCreateDefaultFilters(db); err != nil {
		t.Fatal(err)
	}
	if again := read(); len(again) != len(after) {
		t.Errorf("a second start created rows: %d, want %d", len(again), len(after))
	}

	// An admin who deleted the new filter does not get it back either.
	db.Where("name = ?", toolResultsFilterName).Delete(&Filter{})
	if err := GetOrCreateDefaultFilters(db); err != nil {
		t.Fatal(err)
	}
	var live int64
	db.Model(&Filter{}).Where("name = ?", toolResultsFilterName).Count(&live)
	if live != 0 {
		t.Error("a deleted tool-result filter was recreated")
	}
}
