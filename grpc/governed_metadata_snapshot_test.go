package grpc

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/services/governed_metadata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSnapshotReader returns fixed records and applies a simple visibility rule:
// keys prefixed "gw_" are gateway-visible, everything else is not.
type fakeSnapshotReader struct {
	recs map[string]map[string]*models.ObjectMetadata
}

func (f *fakeSnapshotReader) ListObjectMetadata(objectType string, ids []string) (map[string]*models.ObjectMetadata, error) {
	return f.recs[objectType], nil
}

func (f *fakeSnapshotReader) VisibleValues(objectType string, rec *models.ObjectMetadata, vis governed_metadata.Visibility) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range rec.Values {
		if vis != governed_metadata.VisibilityGateway || len(k) > 3 && k[:3] == "gw_" {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func TestGovernedMetadataJSON_ForSnapshot(t *testing.T) {
	t.Run("no reader yields empty string so CE snapshots are unchanged", func(t *testing.T) {
		s := &ControlServer{}
		assert.Nil(t, s.loadGovernedMetadata(models.GovernedObjectTypeLLM))
		assert.Equal(t, "", s.governedMetadataJSON(models.GovernedObjectTypeLLM, &models.ObjectMetadata{Values: models.JSONMap{"gw_x": "y"}}))
	})

	t.Run("only gateway-visible fields are serialised", func(t *testing.T) {
		s := &ControlServer{}
		s.SetGovernedMetadataReader(&fakeSnapshotReader{recs: map[string]map[string]*models.ObjectMetadata{
			models.GovernedObjectTypeLLM: {
				"1": {ObjectType: "llm", ObjectID: "1", Values: models.JSONMap{"gw_data_classification": "confidential", "business_owner": 7}},
				"2": {ObjectType: "llm", ObjectID: "2", Values: models.JSONMap{"business_owner": 7}},
			},
		}})
		recs := s.loadGovernedMetadata(models.GovernedObjectTypeLLM)
		require.Len(t, recs, 2)
		assert.JSONEq(t, `{"gw_data_classification":"confidential"}`, s.governedMetadataJSON(models.GovernedObjectTypeLLM, recs["1"]))
		assert.Equal(t, "", s.governedMetadataJSON(models.GovernedObjectTypeLLM, recs["2"]), "owner-only metadata never reaches the gateway")
		assert.Equal(t, "", s.governedMetadataJSON(models.GovernedObjectTypeLLM, nil))
	})
}

func TestSnapshotChecksum_GovernedMetadataIsContent(t *testing.T) {
	base := func() *pb.ConfigurationSnapshot {
		return &pb.ConfigurationSnapshot{
			Llms:        []*pb.LLMConfig{{Id: 1, Name: "gpt", Vendor: "openai"}},
			Tools:       []*pb.ToolConfig{{Id: 1, Name: "weather"}},
			Datasources: []*pb.DatasourceConfig{{Id: 1, Name: "docs"}},
		}
	}
	baseline, err := ComputeSnapshotChecksum(base())
	require.NoError(t, err)

	withLLM := base()
	withLLM.Llms[0].GovernedMetadata = `{"data_classification":"restricted"}`
	c1, err := ComputeSnapshotChecksum(withLLM)
	require.NoError(t, err)
	assert.NotEqual(t, baseline, c1, "gateway-visible LLM metadata must trigger a resync")

	withTool := base()
	withTool.Tools[0].GovernedMetadata = `{"data_classification":"internal"}`
	c2, err := ComputeSnapshotChecksum(withTool)
	require.NoError(t, err)
	assert.NotEqual(t, baseline, c2)

	withDS := base()
	withDS.Datasources[0].GovernedMetadata = `{"regulatory_applicability":["gdpr"]}`
	c3, err := ComputeSnapshotChecksum(withDS)
	require.NoError(t, err)
	assert.NotEqual(t, baseline, c3)

	again, err := ComputeSnapshotChecksum(withLLM)
	require.NoError(t, err)
	assert.Equal(t, c1, again, "checksum is deterministic")
}
