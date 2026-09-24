package services

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every embedder change emits a system event (edges re-sync on them, and
// webhooks deliver them), with the key redacted; embedders created for a
// datasource's legacy fields announce themselves too.
func TestEmbedderEvents(t *testing.T) {
	s := setupEmbedderService(t)
	bus := eventbridge.NewBus()
	s.SystemEvents = NewSystemEventEmitter(bus, "test")

	collector := NewTestEventCollector()
	collector.Expect(4)
	for _, topic := range []string{TopicEmbedderCreated, TopicEmbedderUpdated, TopicEmbedderDeleted} {
		sub := bus.Subscribe(topic, collector.Handler())
		defer bus.Unsubscribe(sub)
	}

	e, err := s.CreateEmbedder(standalone("evented", 50), 7)
	require.NoError(t, err)
	next := *e
	next.APIKey = RedactedEmbedderKey
	next.Description = "changed"
	_, err = s.UpdateEmbedder(e.ID, &next, 7)
	require.NoError(t, err)
	require.NoError(t, s.DeleteEmbedder(e.ID, 7))
	createLegacyDS(t, s, "Docs", 10, legacy("ollama", "http://o", "", "nomic"))

	require.True(t, collector.WaitWithTimeout(2*time.Second))
	var topics []string
	for _, ev := range collector.GetEvents() {
		topics = append(topics, ev.Topic)
		payload := parseEventPayload(t, ev)
		assert.Equal(t, "embedder", payload.ObjectType)
		raw, _ := json.Marshal(payload.Object)
		assert.NotContains(t, string(raw), "sk-1", "the key never travels in an event")
	}
	assert.ElementsMatch(t, []string{TopicEmbedderCreated, TopicEmbedderUpdated, TopicEmbedderDeleted, TopicEmbedderCreated}, topics)
}

// Cloning a datasource shares its embedder (and so its vectors' model).
func TestCloneDatasource_SharesEmbedder(t *testing.T) {
	s := setupEmbedderService(t)
	src := createLegacyDS(t, s, "Source", 40, legacy("openai", "https://x", "sk", "m"))
	clone, err := s.CloneDatasource(src.ID)
	require.NoError(t, err)
	got, err := s.GetDatasourceByID(clone.ID)
	require.NoError(t, err)
	require.NotNil(t, got.EmbedderID)
	assert.Equal(t, *src.EmbedderID, *got.EmbedderID)
	assert.Equal(t, "sk", got.FlattenedEmbed().APIKey)

	var n int64
	require.NoError(t, s.DB.Model(&models.Embedder{}).Count(&n).Error)
	assert.EqualValues(t, 1, n, "no new embedder for a clone")
}

// A portal submission describes its embedder with the legacy fields; the
// approved datasource is linked to a matching embedder, and a second
// submission with the same settings reuses it.
func TestSubmissionApproval_LinksEmbedder(t *testing.T) {
	s := setupEmbedderService(t)
	user := createSubmissionTestUser(t, s, "owner@test.com")
	admin := createSubmissionTestAdmin(t, s, "admin@test.com")

	approve := func(name string) *models.Datasource {
		sub, err := s.CreateSubmission(user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
			models.JSONMap{"name": name, "db_source_type": "pgvector", "embed_vendor": "openai",
				"embed_url": "https://api.openai.com/v1", "embed_api_key": "sk-sub", "embed_model": "text-embedding-3-small"},
			nil, 5, "", "c@test.com", "", "", nil, "", "")
		require.NoError(t, err)
		approved, err := s.ApproveSubmission(sub.ID, admin.ID, 5, nil, "")
		require.NoError(t, err)
		ds, err := s.GetDatasourceByID(*approved.ResourceID)
		require.NoError(t, err)
		return ds
	}
	first := approve("First")
	require.NotNil(t, first.EmbedderID)
	assert.Equal(t, "text-embedding-3-small", first.FlattenedEmbed().Model)
	assert.Equal(t, "sk-sub", first.FlattenedEmbed().APIKey)

	second := approve("Second")
	assert.Equal(t, *first.EmbedderID, *second.EmbedderID)
}

// What a plugin hook returns decides the embedder: edited legacy fields move
// the datasource, a changed embedder_id links that one, and an object with
// neither keeps the embedder it had.
func TestApplyHookEmbedEdits_Cases(t *testing.T) {
	s := setupEmbedderService(t)
	ds := createLegacyDS(t, s, "Docs", 10, legacy("openai", "https://e", "sk", "m1"))
	original := ds.Embedder
	other, err := s.CreateEmbedder(standalone("other", 90), 1)
	require.NoError(t, err)

	t.Run("neither legacy fields nor embedder_id keeps the embedder", func(t *testing.T) {
		var bare models.Datasource
		require.NoError(t, json.Unmarshal([]byte(`{"name":"Docs","privacy_score":10}`), &bare))
		require.NoError(t, s.applyHookEmbedEdits(s.DB, original, &bare, 1))
		require.NotNil(t, bare.EmbedderID)
		assert.Equal(t, original.ID, *bare.EmbedderID)
	})

	t.Run("embedder_id alone links that embedder", func(t *testing.T) {
		var only models.Datasource
		require.NoError(t, json.Unmarshal([]byte(`{"name":"Docs","privacy_score":10,"embedder_id":`+jsonUint(other.ID)+`}`), &only))
		require.NoError(t, s.applyHookEmbedEdits(s.DB, original, &only, 1))
		assert.Equal(t, other.ID, *only.EmbedderID)
	})

	t.Run("embedder_id 0 unlinks", func(t *testing.T) {
		var unlink models.Datasource
		require.NoError(t, json.Unmarshal([]byte(`{"name":"Docs","privacy_score":10,"embedder_id":0}`), &unlink))
		require.NoError(t, s.applyHookEmbedEdits(s.DB, original, &unlink, 1))
		assert.Nil(t, unlink.EmbedderID)
	})

	t.Run("an embedder_id that does not exist is refused", func(t *testing.T) {
		var missing models.Datasource
		require.NoError(t, json.Unmarshal([]byte(`{"name":"Docs","privacy_score":10,"embedder_id":999}`), &missing))
		assert.ErrorIs(t, s.applyHookEmbedEdits(s.DB, original, &missing, 1), ErrEmbedderInvalid)
	})
}

func jsonUint(v uint) string { b, _ := json.Marshal(v); return string(b) }

// Documented behaviours of the legacy fields that differ from what a reader
// might expect; pinned so a change to them is deliberate.
func TestLegacyEmbedFields_DocumentedEdges(t *testing.T) {
	s := setupEmbedderService(t)

	t.Run("a model without a vendor on a datasource that has no embedder is ignored", func(t *testing.T) {
		bare := createLegacyDS(t, s, "Bare", 10, EmbedderInput{})
		got, err := updateLegacyDS(t, s, bare, 10, legacy("", "", RedactedEmbedderKey, "m-only"))
		require.NoError(t, err)
		assert.Nil(t, got.EmbedderID)
	})

	t.Run("an empty key clears it on a new embedder, leaving the shared one intact", func(t *testing.T) {
		a := createLegacyDS(t, s, "A", 10, legacy("openai", "https://x", "sk", "m"))
		b := createLegacyDS(t, s, "B", 10, legacy("openai", "https://x", "sk", "m"))
		got, err := updateLegacyDS(t, s, a, 10, legacy("", "", "", ""))
		require.NoError(t, err)
		assert.NotEqual(t, *b.EmbedderID, *got.EmbedderID)
		assert.Equal(t, "", got.FlattenedEmbed().APIKey)
		bNow, err := s.GetDatasourceByID(b.ID)
		require.NoError(t, err)
		assert.Equal(t, "sk", bNow.FlattenedEmbed().APIKey)
	})
}

// An embedder linked to an LLM scoped to a namespace only serves that
// namespace: its credentials must not reach other namespaces' edges.
func TestEmbedderNamespaces(t *testing.T) {
	s := setupEmbedderService(t)
	eu := &models.LLM{Name: "EU OpenAI", Vendor: models.OPENAI, PrivacyScore: 90, Active: true, Namespace: "eu"}
	require.NoError(t, s.DB.Create(eu).Error)
	global := &models.LLM{Name: "Global OpenAI", Vendor: models.OPENAI, PrivacyScore: 90, Active: true}
	require.NoError(t, s.DB.Create(global).Error)
	euEmb, err := s.CreateEmbedder(&models.Embedder{Name: "eu-linked", LLMID: &eu.ID, ModelName: "m"}, 1)
	require.NoError(t, err)
	globalEmb, err := s.CreateEmbedder(&models.Embedder{Name: "global-linked", LLMID: &global.ID, ModelName: "m"}, 1)
	require.NoError(t, err)

	create := func(name, ns string, embedderID uint) (*models.Datasource, error) {
		return s.CreateDatasource(name, "", "", "", "", 10, 1, nil, "", "pgvector", "", "db",
			EmbedderInput{EmbedderID: &embedderID}, true, ns)
	}

	_, err = create("Global DS", "", euEmb.ID)
	assert.ErrorIs(t, err, ErrEmbedderInvalid, "a global datasource reaches every edge")
	_, err = create("US DS", "us", euEmb.ID)
	assert.ErrorIs(t, err, ErrEmbedderInvalid)
	euDS, err := create("EU DS", "eu", euEmb.ID)
	require.NoError(t, err)
	_, err = create("Anywhere", "us", globalEmb.ID)
	require.NoError(t, err, "a global LLM serves every namespace")

	t.Run("moving the datasource out of the namespace is refused", func(t *testing.T) {
		_, err := s.UpdateDatasource(euDS.ID, euDS.Name, "", "", "", "", 10, "", "pgvector", "[redacted]", "db",
			EmbedderInput{EmbedderID: &euEmb.ID}, true, nil, 1, "us")
		assert.ErrorIs(t, err, ErrEmbedderInvalid)
	})

	t.Run("relinking a used embedder to a namespaced LLM is refused", func(t *testing.T) {
		next := *globalEmb
		next.LLMID = &eu.ID
		next.LLM = nil
		_, err := s.UpdateEmbedder(globalEmb.ID, &next, 1)
		var nsErr *EmbedderNamespaceError
		require.True(t, errors.As(err, &nsErr), "%v", err)
		assert.Equal(t, "Anywhere", nsErr.Consumers[0].Name)
	})

	t.Run("moving the LLM to a namespace its consumers are not in is refused", func(t *testing.T) {
		var nsErr *EmbedderNamespaceError
		assert.True(t, errors.As(s.CheckLLMUpdateForEmbedders(global, global.Vendor, 90, "eu"), &nsErr))
		assert.NoError(t, s.CheckLLMUpdateForEmbedders(eu, eu.Vendor, 90, "eu"), "staying put is fine")
		assert.NoError(t, s.CheckLLMUpdateForEmbedders(eu, eu.Vendor, 90, ""), "becoming global is fine")
	})

	t.Run("routers follow the same rule", func(t *testing.T) {
		assert.ErrorIs(t, s.CheckEmbedderForNamespace(&euEmb.ID, ""), ErrEmbedderInvalid)
		assert.NoError(t, s.CheckEmbedderForNamespace(&euEmb.ID, "eu"))
		assert.NoError(t, s.CheckEmbedderForNamespace(nil, ""))
		missing := uint(999)
		assert.ErrorIs(t, s.CheckEmbedderForNamespace(&missing, ""), ErrEmbedderInvalid)
	})
}
