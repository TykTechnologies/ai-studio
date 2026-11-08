package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services/entitlements"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// llmEntitlements runs LLM creates inside a transaction of its own, as the
// enterprise guard does, and refuses them when refuse is set.
type llmEntitlements struct {
	entitlements.Service
	guarded int
	refuse  string
}

func (r *llmEntitlements) GuardLLMCreate(tx *gorm.DB, fn func(*gorm.DB) error) error {
	r.guarded++
	if r.refuse != "" {
		return entitlements.NewExceededError(r.refuse)
	}
	return tx.Transaction(fn)
}

// Every admin LLM create, with or without a namespace, goes through the
// guard; a refusal is 409 QUOTA_EXCEEDED with the entitlements message and
// leaves no LLM behind.
func TestCreateLLMGuardedByLicence(t *testing.T) {
	a, db := setupTestAPI(t)
	ent := &llmEntitlements{Service: entitlements.NewCommunityService()}
	a.service.Entitlements = ent
	r := credentialRouter(a, authz.NewSet(authz.Write("llms"), authz.Publish("llms")))

	post := func(name, namespace string) *httptest.ResponseRecorder {
		body := llmRefBody(name, "sk-plain", nil)
		if namespace != "" {
			body["data"].(map[string]interface{})["attributes"].(map[string]interface{})["namespace"] = namespace
		}
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/llms", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}
	count := func() int64 {
		var n int64
		require.NoError(t, db.Model(&models.LLM{}).Count(&n).Error)
		return n
	}
	before := count()

	require.Equal(t, http.StatusCreated, post("Guarded LLM", "").Code)
	require.Equal(t, http.StatusCreated, post("Namespaced LLM", "tenant-a").Code)
	assert.Equal(t, before+2, count(), "creates inside the guard's transaction are committed")

	ent.refuse = "Your organisation has reached its licensed limit of 2 LLMs."
	for _, ns := range []string{"", "tenant-a"} {
		w := post("One too many "+ns, ns)
		require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
		code, detail := quotaError(t, w)
		assert.Equal(t, entitlements.CodeQuotaExceeded, code)
		assert.Equal(t, ent.refuse, detail)
	}
	assert.Equal(t, 4, ent.guarded)
	assert.Equal(t, before+2, count())
}
