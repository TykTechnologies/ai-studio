package services

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// The hub resolves a guardrail filter's $SECRET/ references into the
// snapshot it sends an edge, and the edge row keeps the hub's UpdatedAt.
// Rotating the secret on the hub changes the resolved value but not the
// filter's UpdatedAt, and API keys keep their length. The edge must still
// use the new value once the new snapshot is applied, not the one parsed
// for the old snapshot.
func TestAdversarial_RotatedSecretSameLengthReachesTheRunner(t *testing.T) {
	parsedConfigs.Reset()
	v := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	row := func(key string) *database.Filter {
		return &database.Filter{
			ID: 41, CreatedAt: v.Add(-time.Hour), UpdatedAt: v,
			Name: "lakera", Kind: models.FilterKindGuardrail,
			Config: `{"provider":"lakera","detectors":[{"name":"prompt_attack"}],"on_detect":"block","connection":{"api_key":"` + key + `"}}`,
		}
	}
	before := convertDatabaseFilterToModel(nil, row("lk-OLDOLDOLDOLDOLD"))
	after := convertDatabaseFilterToModel(nil, row("lk-NEWNEWNEWNEWNEW"))

	conn := func(f *models.Filter) string {
		c, _ := f.Config["connection"].(map[string]any)
		s, _ := c["api_key"].(string)
		return s
	}
	if conn(before) != "lk-OLDOLDOLDOLDOLD" {
		t.Fatalf("first snapshot parsed to %q", conn(before))
	}
	if got := conn(after); got != "lk-NEWNEWNEWNEWNEW" {
		t.Errorf("after the rotation the edge still uses %q", got)
	}
}
