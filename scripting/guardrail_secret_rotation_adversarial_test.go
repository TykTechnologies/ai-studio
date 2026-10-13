package scripting

// Hub-side half of midsommar-clean-01t (tests agent): rotating a $SECRET/
// value in the secrets store changes a guardrail's resolved connection
// without changing the filter row, which keeps the reference. The
// prepared-filter cache resolves references when it builds an entry and
// keeps it for preparedTTL, so the accepted semantics (lead and security,
// 2026-10-13) are: the rotation reaches the provider once the entry
// expires, without a filter edit, and no later than preparedTTL.

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// keyFake records the api_key each provider instance was built with and
// which key answered each call.
type keyFake struct {
	key string
	log *[]string
	mu  *sync.Mutex
}

func (f *keyFake) Capabilities() guardrails.Capabilities { return guardrails.Capabilities{} }

func (f *keyFake) Classify(context.Context, guardrails.Input) (guardrails.Verdict, error) {
	f.mu.Lock()
	*f.log = append(*f.log, f.key)
	f.mu.Unlock()
	return guardrails.Verdict{}, nil
}

func TestAdversarial_SecretRotationReachesProviderWithoutFilterEdit(t *testing.T) {
	now := time.Now()
	preparedNow = func() time.Time { return now }
	t.Cleanup(func() { preparedNow = time.Now })

	t.Setenv("TYK_AI_SECRET_KEY", "rotation-test-key")
	// A database of its own per run, so -count=N does not find the
	// rotated value of the run before.
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:rotation_%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&secrets.Secret{}); err != nil {
		t.Fatal(err)
	}
	secrets.SetDBRef(db)
	t.Cleanup(func() { secrets.SetDBRef(nil) })

	secret := &secrets.Secret{VarName: "GR_ROTATION_KEY", Value: "lk-OLDOLDOLDOLDOLD"}
	if err := secrets.CreateSecret(db, secret); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var used []string
	name := "rotation_fake"
	guardrails.RegisterSpec(guardrails.Spec{
		Name: name, DisplayName: name,
		Detectors:        []guardrails.DetectorSpec{{Name: "secrets", Category: "secrets"}},
		ConnectionFields: []guardrails.ConnectionField{{Name: "api_key", Secret: true}},
	})
	guardrails.RegisterFactory(name, func(cfg guardrails.Config) (guardrails.Provider, error) {
		return &keyFake{key: cfg.Connection["api_key"], log: &used, mu: &mu}, nil
	})
	f := &models.Filter{Name: name, Kind: models.FilterKindGuardrail, Config: models.JSONMap{
		"provider":   name,
		"detectors":  []any{map[string]any{"name": "secrets"}},
		"on_detect":  guardrails.ActionLog,
		"connection": map[string]any{"api_key": "$SECRET/GR_ROTATION_KEY"},
	}}
	f.ID, f.UpdatedAt = 31337, time.Now()
	freshVerdictCache(t, time.Minute)

	runRequest(t, f, chatBody(t, chatMsg{Role: "user", Content: "first"}), app(1))

	// Rotate in the store: same length, the filter row untouched.
	stored, err := secrets.GetSecretByVarName(db, "GR_ROTATION_KEY", false)
	if err != nil {
		t.Fatal(err)
	}
	stored.Value = "lk-NEWNEWNEWNEWNEW"
	if err := secrets.UpdateSecret(db, stored, true); err != nil {
		t.Fatal(err)
	}

	// Within the window the prepared entry, and so the old key, is still in
	// use; once it has expired the next request resolves the new value.
	now = now.Add(preparedTTL / 2)
	runRequest(t, f, chatBody(t, chatMsg{Role: "user", Content: "second"}), app(1))
	now = now.Add(preparedTTL)
	runRequest(t, f, chatBody(t, chatMsg{Role: "user", Content: "third"}), app(1))

	mu.Lock()
	defer mu.Unlock()
	want := []string{"lk-OLDOLDOLDOLDOLD", "lk-OLDOLDOLDOLDOLD", "lk-NEWNEWNEWNEWNEW"}
	if len(used) != len(want) {
		t.Fatalf("provider keys used = %v, want %v", used, want)
	}
	for i := range want {
		if used[i] != want[i] {
			t.Errorf("call %d used %q, want %q (keys %v)", i, used[i], want[i], used)
		}
	}
}
