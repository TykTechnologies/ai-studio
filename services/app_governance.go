package services

import (
	"errors"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/clause"
)

// AppGovernanceFlagsKey is the App metadata key holding governance flags
// raised by plugins (an asset catalog marking an App "ownerless" or
// "review_lapsed", say).
const AppGovernanceFlagsKey = "governance_flags"

// ErrReservedAppMetadataKey is returned when a caller tries to write the
// governance flags through the generic metadata paths.
var ErrReservedAppMetadataKey = errors.New("metadata key " + AppGovernanceFlagsKey + " is reserved for governance plugins (SetAppGovernanceState)")

// keepGovernanceFlags returns incoming metadata carrying the App's existing
// governance flags, whatever the caller sent: only SetAppGovernanceState
// writes them, so an App edit (from a form loaded before a flag was raised,
// or an API client that omits metadata) cannot erase or forge them.
func keepGovernanceFlags(incoming, existing map[string]interface{}) map[string]interface{} {
	flags, had := existing[AppGovernanceFlagsKey]
	_, sent := incoming[AppGovernanceFlagsKey]
	if !had && !sent {
		return incoming
	}
	out := make(map[string]interface{}, len(incoming)+1)
	for k, v := range incoming {
		if k != AppGovernanceFlagsKey {
			out[k] = v
		}
	}
	if had {
		out[AppGovernanceFlagsKey] = flags
	}
	return out
}

// AppGovernanceFlag is one flag on an App.
type AppGovernanceFlag struct {
	Value  string    `json:"value"`
	Reason string    `json:"reason,omitempty"`
	SetBy  string    `json:"set_by,omitempty"`
	At     time.Time `json:"at"`
}

// SetAppGovernanceState changes an App's active state and governance flags
// for a governance plugin. active nil leaves the state alone; a flag with an
// empty value is cleared. It returns the App and what changed (for the audit
// trail): {"is_active": {old, new}, "flag:<name>": {old, new}}.
func (s *Service) SetAppGovernanceState(appID uint, active *bool, flags map[string]string, reason, setBy string) (*models.App, map[string]interface{}, error) {
	diff := map[string]interface{}{}
	if len(flags) > 0 {
		err := s.DB.Transaction(func(tx *gorm.DB) error {
			var app models.App
			query := tx
			if s.DB.Dialector.Name() == "postgres" {
				query = query.Clauses(clause.Locking{Strength: "UPDATE"})
			} else if err := tx.Exec("UPDATE apps SET id = id WHERE id = ?", appID).Error; err != nil {
				// SQLite: take the write lock before reading. A transaction
				// that reads and then writes fails at once ("database is
				// locked", no busy wait) if another connection wrote between.
				return err
			}
			if err := query.First(&app, appID).Error; err != nil {
				return err
			}
			if app.Metadata == nil {
				app.Metadata = map[string]interface{}{}
			}
			current, _ := app.Metadata[AppGovernanceFlagsKey].(map[string]interface{})
			if current == nil {
				current = map[string]interface{}{}
			}
			for name, value := range flags {
				old := ""
				if prev, ok := current[name].(map[string]interface{}); ok {
					old, _ = prev["value"].(string)
				}
				if value == "" {
					if _, ok := current[name]; !ok {
						continue
					}
					delete(current, name)
				} else {
					current[name] = AppGovernanceFlag{Value: value, Reason: reason, SetBy: setBy, At: time.Now().UTC()}
				}
				if old != value {
					diff["flag:"+name] = map[string]interface{}{"old": old, "new": value}
				}
			}
			if len(current) == 0 {
				delete(app.Metadata, AppGovernanceFlagsKey)
			} else {
				app.Metadata[AppGovernanceFlagsKey] = current
			}
			// A struct update so the JSON serializer applies (a map argument to
			// Update bypasses it).
			return tx.Model(&app).Select("metadata").Updates(&app).Error
		})
		if err != nil {
			return nil, nil, err
		}
	}
	app, err := s.GetAppByID(appID)
	if err != nil {
		return nil, nil, err
	}
	if active != nil && app.IsActive != *active {
		diff["is_active"] = map[string]interface{}{"old": app.IsActive, "new": *active}
		if app, err = s.SetAppActive(appID, *active, 0); err != nil {
			return nil, nil, err
		}
	} else if len(diff) > 0 && s.SystemEvents != nil {
		s.SystemEvents.EmitAppUpdated(app, app.ID, 0)
	}
	return app, diff, nil
}
