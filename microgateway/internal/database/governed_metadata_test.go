package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/datatypes"
)

func TestGovernedMetadataJSON(t *testing.T) {
	assert.Nil(t, GovernedMetadataJSON(""))
	assert.Equal(t, datatypes.JSON(`{"a":1}`), GovernedMetadataJSON(`{"a":1}`))
}

func TestAddGovernedMetadataToContext(t *testing.T) {
	t.Run("nil llm or empty metadata is a no-op", func(t *testing.T) {
		meta := map[string]interface{}{"edge_id": "e1"}
		AddGovernedMetadataToContext(meta, nil)
		AddGovernedMetadataToContext(meta, &LLM{})
		assert.Equal(t, map[string]interface{}{"edge_id": "e1"}, meta)
		AddGovernedMetadataToContext(nil, &LLM{GovernedMetadata: datatypes.JSON(`{"x":"y"}`)})
	})

	t.Run("exposes raw JSON plus flattened string values", func(t *testing.T) {
		llm := &LLM{GovernedMetadata: datatypes.JSON(`{"data_classification":"confidential","regulatory_applicability":["gdpr","hipaa"],"risk_score":3,"ratio":1.5,"approved":true,"nested":{"k":"v"}}`)}
		meta := map[string]interface{}{}
		AddGovernedMetadataToContext(meta, llm)

		assert.Equal(t, string(llm.GovernedMetadata), meta["governed_metadata"])
		assert.Equal(t, "confidential", meta["governed_metadata.data_classification"])
		assert.Equal(t, "gdpr,hipaa", meta["governed_metadata.regulatory_applicability"])
		assert.Equal(t, "3", meta["governed_metadata.risk_score"])
		assert.Equal(t, "1.5", meta["governed_metadata.ratio"])
		assert.Equal(t, "true", meta["governed_metadata.approved"])
		assert.JSONEq(t, `{"k":"v"}`, meta["governed_metadata.nested"].(string))
	})

	t.Run("malformed JSON still exposes the raw string", func(t *testing.T) {
		meta := map[string]interface{}{}
		AddGovernedMetadataToContext(meta, &LLM{GovernedMetadata: datatypes.JSON(`not json`)})
		assert.Equal(t, "not json", meta["governed_metadata"])
		assert.Len(t, meta, 1)
	})
}
