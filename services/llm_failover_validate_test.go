package services

import (
	"errors"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failoverLLM creates an LLM with the fields the validator inspects.
func failoverLLM(t *testing.T, s *Service, name string, active bool, privacy int, allowed []string, namespace string, failover *models.LLMFailover) *models.LLM {
	t.Helper()
	llm, err := s.CreateLLMWithNamespace(name, "key", "https://example.test", privacy, "", "", "",
		models.OPENAI, active, nil, "gpt-4o", allowed, nil, nil, namespace, false, nil, failover)
	require.NoError(t, err, "creating %s", name)
	return llm
}

func failoverTo(targets ...models.LLMFailoverTarget) *models.LLMFailover {
	return &models.LLMFailover{Targets: targets}
}

func assertFailoverErr(t *testing.T, err error, index int, field, contains string) {
	t.Helper()
	var verr *LLMFailoverValidationError
	if !assert.True(t, errors.As(err, &verr), "expected a failover validation error, got %v", err) {
		return
	}
	assert.Equal(t, index, verr.Index)
	assert.Equal(t, field, verr.Field)
	assert.Contains(t, err.Error(), contains)
}

func TestLLMFailover_ValidWaterfallPersistsThroughCreateAndUpdate(t *testing.T) {
	db := setupTestDB(t)
	s := NewService(db)

	backup := failoverLLM(t, s, "Backup", true, 50, []string{"^claude-.*$"}, "", nil)
	primary := failoverLLM(t, s, "Primary", true, 50, nil, "",
		failoverTo(models.LLMFailoverTarget{LLMID: backup.ID, Model: "claude-sonnet-4"}))

	loaded, err := s.GetLLMByID(primary.ID)
	require.NoError(t, err)
	require.Len(t, loaded.Failover.Targets, 1)
	assert.Equal(t, backup.ID, loaded.Failover.Targets[0].LLMID)

	// Update replaces the waterfall; nil clears it.
	second := failoverLLM(t, s, "Second", true, 50, nil, "", nil)
	_, err = s.UpdateLLM(primary.ID, primary.Name, "[redacted]", primary.APIEndpoint, primary.PrivacyScore, "", "", "",
		primary.Vendor, true, nil, primary.DefaultModel, nil, nil, nil, "", false, nil,
		failoverTo(models.LLMFailoverTarget{LLMID: second.ID, Model: "gpt-4o"}))
	require.NoError(t, err)
	loaded, err = s.GetLLMByID(primary.ID)
	require.NoError(t, err)
	assert.Equal(t, second.ID, loaded.Failover.Targets[0].LLMID)

	_, err = s.UpdateLLM(primary.ID, primary.Name, "[redacted]", primary.APIEndpoint, primary.PrivacyScore, "", "", "",
		primary.Vendor, true, nil, primary.DefaultModel, nil, nil, nil, "", false, nil, nil)
	require.NoError(t, err)
	loaded, err = s.GetLLMByID(primary.ID)
	require.NoError(t, err)
	assert.False(t, loaded.Failover.Enabled())
}

func TestLLMFailover_ValidationRules(t *testing.T) {
	db := setupTestDB(t)
	s := NewService(db)

	backup := failoverLLM(t, s, "Backup", true, 50, []string{"^claude-.*$"}, "", nil)
	inactive := failoverLLM(t, s, "Inactive", false, 50, nil, "", nil)
	lessPrivate := failoverLLM(t, s, "LessPrivate", true, 10, nil, "", nil)
	otherNS := failoverLLM(t, s, "OtherNS", true, 50, nil, "eu", nil)
	brokenPattern := failoverLLM(t, s, "Broken", true, 50, []string{"("}, "", nil)
	primary := failoverLLM(t, s, "Primary", true, 50, nil, "", nil)

	update := func(f *models.LLMFailover) error {
		_, err := s.UpdateLLM(primary.ID, primary.Name, "[redacted]", primary.APIEndpoint, primary.PrivacyScore, "", "", "",
			primary.Vendor, true, nil, primary.DefaultModel, nil, nil, nil, "", false, nil, f)
		return err
	}

	cases := []struct {
		name     string
		failover *models.LLMFailover
		index    int
		field    string
		contains string
	}{
		{"missing model", failoverTo(models.LLMFailoverTarget{LLMID: backup.ID}), 0, "model", "a model is required"},
		{"missing llm", failoverTo(models.LLMFailoverTarget{Model: "x"}), 0, "llm_id", "an LLM must be selected"},
		{"self reference", failoverTo(models.LLMFailoverTarget{LLMID: primary.ID, Model: "gpt-4o"}), 0, "llm_id", "cannot fail over to itself"},
		{"unknown llm", failoverTo(models.LLMFailoverTarget{LLMID: 9999, Model: "gpt-4o"}), 0, "llm_id", "does not exist"},
		{"inactive target", failoverTo(models.LLMFailoverTarget{LLMID: inactive.ID, Model: "gpt-4o"}), 0, "llm_id", "not active"},
		{"model outside allowlist", failoverTo(models.LLMFailoverTarget{LLMID: backup.ID, Model: "gpt-4o"}), 0, "model", "not in the allowed models"},
		{"broken allowlist pattern", failoverTo(models.LLMFailoverTarget{LLMID: brokenPattern.ID, Model: "gpt-4o"}), 0, "model", "invalid pattern"},
		{"lower privacy score", failoverTo(models.LLMFailoverTarget{LLMID: lessPrivate.ID, Model: "gpt-4o"}), 0, "llm_id", "lower privacy score"},
		{"other namespace", failoverTo(models.LLMFailoverTarget{LLMID: otherNS.ID, Model: "gpt-4o"}), 0, "llm_id", "namespace"},
		{"duplicate rung", failoverTo(
			models.LLMFailoverTarget{LLMID: backup.ID, Model: "claude-sonnet-4"},
			models.LLMFailoverTarget{LLMID: backup.ID, Model: "claude-sonnet-4"}), 1, "llm_id", "duplicate"},
		{"4xx trigger", &models.LLMFailover{
			Targets:  []models.LLMFailoverTarget{{LLMID: backup.ID, Model: "claude-sonnet-4"}},
			Triggers: &models.LLMFailoverTriggers{StatusCodes: []int{503, 403}},
		}, -1, "triggers.status_codes", "status 403 cannot trigger failover"},
		{"triggers without targets", &models.LLMFailover{Triggers: &models.LLMFailoverTriggers{StatusCodes: []int{503}}},
			-1, "targets", "no failover targets"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertFailoverErr(t, update(tc.failover), tc.index, tc.field, tc.contains)
		})
	}

	t.Run("too many rungs", func(t *testing.T) {
		var targets []models.LLMFailoverTarget
		for i := 0; i <= models.MaxFailoverTargets; i++ {
			targets = append(targets, models.LLMFailoverTarget{LLMID: backup.ID, Model: "claude-" + string(rune('a'+i))})
		}
		assertFailoverErr(t, update(&models.LLMFailover{Targets: targets}), -1, "targets", "at most")
	})

	t.Run("the second rung is reported by its own index", func(t *testing.T) {
		err := update(failoverTo(
			models.LLMFailoverTarget{LLMID: backup.ID, Model: "claude-sonnet-4"},
			models.LLMFailoverTarget{LLMID: inactive.ID, Model: "gpt-4o"},
		))
		assertFailoverErr(t, err, 1, "llm_id", "not active")
		assert.Contains(t, err.Error(), "failover target 2:")
	})

	t.Run("global target reachable from a namespaced primary", func(t *testing.T) {
		nsPrimary := failoverLLM(t, s, "NSPrimary", true, 50, nil, "eu", nil)
		_, err := s.UpdateLLM(nsPrimary.ID, nsPrimary.Name, "[redacted]", nsPrimary.APIEndpoint, 50, "", "", "",
			nsPrimary.Vendor, true, nil, nsPrimary.DefaultModel, nil, nil, nil, "eu", false, nil,
			failoverTo(
				models.LLMFailoverTarget{LLMID: backup.ID, Model: "claude-sonnet-4"},
				models.LLMFailoverTarget{LLMID: otherNS.ID, Model: "gpt-4o"},
			))
		assert.NoError(t, err)
	})
}

func TestLLMFailover_DeleteRefusedWhileReferenced(t *testing.T) {
	db := setupTestDB(t)
	s := NewService(db)

	backup := failoverLLM(t, s, "Backup", true, 50, nil, "", nil)
	primary := failoverLLM(t, s, "Primary", true, 50, nil, "",
		failoverTo(models.LLMFailoverTarget{LLMID: backup.ID, Model: "gpt-4o"}))

	err := s.DeleteLLM(backup.ID)
	assertFailoverErr(t, err, -1, "targets", "Primary")

	names, err := s.LLMsReferencingAsFailoverTarget(backup.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"Primary"}, names)

	// Unlink, then the delete goes through.
	_, err = s.UpdateLLM(primary.ID, primary.Name, "[redacted]", primary.APIEndpoint, 50, "", "", "",
		primary.Vendor, true, nil, primary.DefaultModel, nil, nil, nil, "", false, nil, nil)
	require.NoError(t, err)
	assert.NoError(t, s.DeleteLLM(backup.ID))
}
