package services

import (
	"fmt"
	"slices"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Alert event names (the guardrail alert action, features/GuardrailExceptions.md)
// are unique across every guardrail filter and guardrail exception, so the
// topic guardrail.alert.<event> a webhook subscribes to identifies one
// source. Checked where filters and exceptions are saved.

// checkAlertEventsFree refuses names already used by another filter (not
// exceptFilter) or another exception (not exceptException). Retired
// exceptions keep their names.
func checkAlertEventsFree(db *gorm.DB, names []string, exceptFilter, exceptException uint) error {
	if len(names) == 0 {
		return nil
	}
	var filters []models.Filter
	q := db.Select("id", "name", "kind", "config").Where("kind = ?", models.FilterKindGuardrail)
	if exceptFilter != 0 {
		q = q.Where("id <> ?", exceptFilter)
	}
	if err := q.Find(&filters).Error; err != nil {
		return err
	}
	for _, f := range filters {
		cfg, err := guardrails.ParseConfig(f.Config)
		if err != nil {
			continue
		}
		for _, used := range cfg.AlertEvents() {
			if slices.Contains(names, used) {
				return fmt.Errorf("%w: alert event %q is already used by filter %q; each alert needs its own name", guardrails.ErrInvalidConfig, used, f.Name)
			}
		}
	}
	var exceptions []models.GuardrailException
	eq := db.Select("id", "app_id", "alert_event").Where("alert_event IN ?", names)
	if exceptException != 0 {
		eq = eq.Where("id <> ?", exceptException)
	}
	if err := eq.Find(&exceptions).Error; err != nil {
		return err
	}
	if len(exceptions) > 0 {
		e := exceptions[0]
		return fmt.Errorf("%w: alert event %q is already used by guardrail exception #%d of app %d; each alert needs its own name", guardrails.ErrInvalidConfig, e.AlertEvent, e.ID, e.AppID)
	}
	return nil
}

// filterAlertEvents lists the alert events a stored guardrail config names.
func filterAlertEvents(kind string, config models.JSONMap) []string {
	if kind != models.FilterKindGuardrail {
		return nil
	}
	cfg, err := guardrails.ParseConfig(config)
	if err != nil {
		return nil
	}
	return cfg.AlertEvents()
}

// GuardrailAlertTopics lists the guardrail topics a webhook can subscribe
// to, sorted: the bus topic of every configured guardrail alert
// (guardrail.alert.<event>, from guardrail filters and exceptions) and the
// fixed outcome event topics (GuardrailOutcomeTopics). The webhook target
// editor offers them (Enterprise ListTopics).
func GuardrailAlertTopics(db *gorm.DB) ([]string, error) {
	set := map[string]bool{}
	var filters []models.Filter
	if err := db.Select("id", "kind", "config").Where("kind = ?", models.FilterKindGuardrail).Find(&filters).Error; err != nil {
		return nil, err
	}
	for _, f := range filters {
		for _, name := range filterAlertEvents(f.Kind, f.Config) {
			set[name] = true
		}
	}
	var names []string
	if err := db.Model(&models.GuardrailException{}).Where("alert_event <> '' AND status <> ?", models.GuardrailExceptionRetired).
		Distinct("alert_event").Pluck("alert_event", &names).Error; err != nil {
		return nil, err
	}
	for _, n := range names {
		set[n] = true
	}
	out := make([]string, 0, len(set)+len(GuardrailOutcomeTopics))
	for n := range set {
		out = append(out, guardrails.AlertTopicPrefix+n)
	}
	out = append(out, GuardrailOutcomeTopics...)
	slices.Sort(out)
	return out, nil
}
