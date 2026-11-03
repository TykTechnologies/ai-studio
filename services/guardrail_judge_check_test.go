package services

import (
	"errors"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/require"
)

func judgeTestService(t *testing.T) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.LLM{}, &models.Filter{}))
	return &Service{DB: db}
}

func judgeLLM(t *testing.T, s *Service, name string, vendor models.Vendor, active bool, ns string) *models.LLM {
	t.Helper()
	l := &models.LLM{Name: name, Vendor: vendor, Active: true, Namespace: ns, APIKey: "k", DefaultModel: "m"}
	require.NoError(t, s.DB.Create(l).Error)
	if !active {
		require.NoError(t, s.DB.Model(l).Update("active", false).Error)
	}
	return l
}

func judgeSpec(ns string, judge map[string]any) FilterSpec {
	spec := guardrailSpec(map[string]any{"on_detect": guardrails.ActionBlock, "judge": judge})
	spec.Namespace = ns
	return spec
}

// The judge LLM is checked where the filter is saved: it must exist, be
// active, serve chat and reach every edge the filter runs on.
func TestFilterSave_ChecksTheJudge(t *testing.T) {
	s := judgeTestService(t)
	global := judgeLLM(t, s, "global judge", models.OPENAI, true, "")
	inactive := judgeLLM(t, s, "inactive", models.OPENAI, false, "")
	decisions := judgeLLM(t, s, "decisions", models.SYSTEMONE, true, "")
	euJudge := judgeLLM(t, s, "eu judge", models.OPENAI, true, "eu")

	on := func(id uint) map[string]any { return map[string]any{"mode": "record", "llm_id": id} }
	refused := func(t *testing.T, err error, want string) {
		t.Helper()
		if !errors.Is(err, ErrInvalidFilterSpec) || !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want ErrInvalidFilterSpec containing %q", err, want)
		}
	}

	_, err := s.CreateFilterFromSpec(judgeSpec("", on(9999)))
	refused(t, err, "does not exist")
	_, err = s.CreateFilterFromSpec(judgeSpec("", on(inactive.ID)))
	refused(t, err, "not active")
	_, err = s.CreateFilterFromSpec(judgeSpec("", on(decisions.ID)))
	refused(t, err, "does not serve chat")

	// A judge that is off is not checked (its LLM may be kept for later).
	_, err = s.CreateFilterFromSpec(judgeSpec("", map[string]any{"mode": "off", "llm_id": 9999}))
	require.NoError(t, err)

	f, err := s.CreateFilterFromSpec(judgeSpec("", on(global.ID)))
	require.NoError(t, err)
	// A namespaced judge on a new filter: attached to no LLM yet.
	nsFilter, err := s.CreateFilterFromSpec(judgeSpec("", on(euJudge.ID)))
	require.NoError(t, err)

	// Attached to a global LLM, a global filter runs on every edge: a judge
	// in one namespace does not reach them.
	globalLLM := judgeLLM(t, s, "gpt global", models.OPENAI, true, "")
	require.NoError(t, s.DB.Model(globalLLM).Association("Filters").Append(nsFilter))
	_, err = s.UpdateFilterFromSpec(nsFilter.ID, judgeSpec("", on(euJudge.ID)))
	refused(t, err, "runs on every edge")
	// The same filter scoped to eu runs on eu edges only: the eu judge does.
	_, err = s.UpdateFilterFromSpec(nsFilter.ID, judgeSpec("eu", on(euJudge.ID)))
	require.NoError(t, err)
	// ... and a filter scoped to us does not.
	_, err = s.UpdateFilterFromSpec(nsFilter.ID, judgeSpec("us", on(euJudge.ID)))
	refused(t, err, `namespace "us"`)

	// Attached to an eu LLM, a global filter runs on eu edges.
	euLLM := judgeLLM(t, s, "gpt eu", models.OPENAI, true, "eu")
	require.NoError(t, s.DB.Model(euLLM).Association("Filters").Append(f))
	_, err = s.UpdateFilterFromSpec(f.ID, judgeSpec("", on(euJudge.ID)))
	require.NoError(t, err)
	// A us LLM too: the eu judge does not reach the us edges.
	usLLM := judgeLLM(t, s, "gpt us", models.OPENAI, true, "us")
	require.NoError(t, s.DB.Model(usLLM).Association("Filters").Append(f))
	_, err = s.UpdateFilterFromSpec(f.ID, judgeSpec("", on(euJudge.ID)))
	refused(t, err, `namespace "us"`)
	// A global judge reaches every edge.
	_, err = s.UpdateFilterFromSpec(f.ID, judgeSpec("", on(global.ID)))
	require.NoError(t, err)
}

func TestEdgeNamespacesFor(t *testing.T) {
	for _, c := range []struct {
		llm, filter, edges string
		runs               bool
	}{
		{"", "", "", true},
		{"", "eu", "eu", true},
		{"eu", "", "eu", true},
		{"eu", "eu", "eu", true},
		{"eu", "us", "", false},
	} {
		edges, runs := edgeNamespacesFor(c.llm, c.filter)
		if edges != c.edges || runs != c.runs {
			t.Errorf("edgeNamespacesFor(%q, %q) = %q, %v; want %q, %v", c.llm, c.filter, edges, runs, c.edges, c.runs)
		}
	}
}

// The judge reads the traffic of every LLM the filter guards: its privacy
// score must be at least theirs (midsommar-clean-9u1j.46).
func TestFilterSave_JudgePrivacyScore(t *testing.T) {
	s := judgeTestService(t)
	judge := judgeLLM(t, s, "judge", models.OPENAI, true, "")
	require.NoError(t, s.DB.Model(judge).Update("privacy_score", 50).Error)
	on := map[string]any{"mode": "record", "llm_id": judge.ID}

	f, err := s.CreateFilterFromSpec(judgeSpec("", on))
	require.NoError(t, err, "a new filter guards no LLM yet")

	low := judgeLLM(t, s, "public", models.OPENAI, true, "")
	require.NoError(t, s.DB.Model(low).Update("privacy_score", 30).Error)
	require.NoError(t, s.DB.Model(low).Association("Filters").Append(f))
	_, err = s.UpdateFilterFromSpec(f.ID, judgeSpec("", on))
	require.NoError(t, err, "judge 50 may read an LLM scored 30")

	high := judgeLLM(t, s, "on-prem", models.OPENAI, true, "")
	require.NoError(t, s.DB.Model(high).Update("privacy_score", 90).Error)
	require.NoError(t, s.DB.Model(high).Association("Filters").Append(f))
	_, err = s.UpdateFilterFromSpec(f.ID, judgeSpec("", on))
	if !errors.Is(err, ErrInvalidFilterSpec) || !strings.Contains(err.Error(), "privacy score 50") || !strings.Contains(err.Error(), `"on-prem"`) {
		t.Errorf("err = %v, want the privacy score refusal naming on-prem", err)
	}
	// The judge off: no judge reads anything.
	_, err = s.UpdateFilterFromSpec(f.ID, judgeSpec("", map[string]any{"mode": "off", "llm_id": judge.ID}))
	require.NoError(t, err)
}
