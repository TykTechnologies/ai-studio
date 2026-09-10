package plugin_sdk

import (
	"context"
	"encoding/json"
	"testing"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test plugins ---

type plainUIPlugin struct {
	BasePlugin
	lastMethod string
}

func (p *plainUIPlugin) GetAsset(string) ([]byte, string, error)    { return nil, "", nil }
func (p *plainUIPlugin) ListAssets(string) ([]*pb.AssetInfo, error) { return nil, nil }
func (p *plainUIPlugin) GetManifest() ([]byte, error)               { return []byte("{}"), nil }
func (p *plainUIPlugin) HandleRPC(method string, payload []byte) ([]byte, error) {
	p.lastMethod = method
	return []byte(`{"via":"HandleRPC"}`), nil
}

type userAwarePlugin struct {
	plainUIPlugin
	lastUser *PortalUserContext
}

func (p *userAwarePlugin) HandleRPCWithUser(method string, payload []byte, userCtx *PortalUserContext) ([]byte, error) {
	p.lastMethod = method
	p.lastUser = userCtx
	return []byte(`{"via":"HandleRPCWithUser"}`), nil
}

func TestWrapperCall_UserAwareRPCHandler(t *testing.T) {
	userCtx := &pb.PortalUserContext{UserId: 7, Email: "admin@test.com", Name: "Admin", IsAdmin: true, Groups: []string{"ops"}}

	t.Run("routes to HandleRPCWithUser when the host supplies a user", func(t *testing.T) {
		plugin := &userAwarePlugin{}
		w := newPluginServerWrapper(plugin, RuntimeStudio, nil)

		resp, err := w.Call(context.Background(), &pb.CallRequest{Method: "admin_stats", Payload: "{}", UserContext: userCtx})
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.JSONEq(t, `{"via":"HandleRPCWithUser"}`, resp.Data)
		require.NotNil(t, plugin.lastUser)
		assert.Equal(t, uint32(7), plugin.lastUser.UserID)
		assert.Equal(t, "admin@test.com", plugin.lastUser.Email)
		assert.True(t, plugin.lastUser.IsAdmin)
		assert.Equal(t, []string{"ops"}, plugin.lastUser.Groups)
		assert.NotNil(t, plugin.lastUser.Metadata)
	})

	t.Run("falls back to HandleRPC when no user is supplied", func(t *testing.T) {
		plugin := &userAwarePlugin{}
		w := newPluginServerWrapper(plugin, RuntimeStudio, nil)

		resp, err := w.Call(context.Background(), &pb.CallRequest{Method: "admin_stats", Payload: "{}"})
		require.NoError(t, err)
		assert.JSONEq(t, `{"via":"HandleRPC"}`, resp.Data)
		assert.Nil(t, plugin.lastUser)
	})

	t.Run("plain UIProvider plugins are unaffected", func(t *testing.T) {
		plugin := &plainUIPlugin{}
		w := newPluginServerWrapper(plugin, RuntimeStudio, nil)

		resp, err := w.Call(context.Background(), &pb.CallRequest{Method: "stats", Payload: "{}", UserContext: userCtx})
		require.NoError(t, err)
		assert.JSONEq(t, `{"via":"HandleRPC"}`, resp.Data)
		assert.Equal(t, "stats", plugin.lastMethod)
	})
}

func TestParseSubmissionEnvelope(t *testing.T) {
	t.Run("full envelope", func(t *testing.T) {
		raw := `{"source":"submission","submission_id":12,"submitter":{"id":3,"email":"a@b.c","name":"A"},
		  "reviewer":{"id":1,"email":"admin@b.c","name":"Admin"},"final_privacy_score":40,"assigned_catalogues":[1,2],
		  "resource_payload":{"name":"Triage","purpose":"route"}}`
		env, err := ParseSubmissionEnvelope([]byte(raw))
		require.NoError(t, err)
		assert.Equal(t, uint32(12), env.SubmissionID)
		assert.Equal(t, "a@b.c", env.Submitter.Email)
		assert.Equal(t, uint32(1), env.Reviewer.ID)
		assert.Equal(t, 40, env.FinalPrivacyScore)
		assert.Equal(t, []uint32{1, 2}, env.AssignedCatalogues)
		assert.Equal(t, "Triage", env.ResourcePayload["name"])
	})

	t.Run("bare payload (legacy contract)", func(t *testing.T) {
		env, err := ParseSubmissionEnvelope([]byte(`{"name":"Bare","purpose":"x"}`))
		require.NoError(t, err)
		assert.Equal(t, uint32(0), env.SubmissionID)
		assert.Equal(t, "Bare", env.ResourcePayload["name"])
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := ParseSubmissionEnvelope([]byte(`nope`))
		assert.Error(t, err)
	})

	t.Run("round-trips through JSON", func(t *testing.T) {
		in := SubmissionEnvelope{Source: "submission", SubmissionID: 5, ResourcePayload: map[string]interface{}{"name": "x"}}
		b, err := json.Marshal(in)
		require.NoError(t, err)
		out, err := ParseSubmissionEnvelope(b)
		require.NoError(t, err)
		assert.Equal(t, in.SubmissionID, out.SubmissionID)
	})
}

// mockStudioServices records RegisterResourceTypes calls; other methods are
// inherited from the embedded nil interface and must not be called.
type mockStudioServices struct {
	StudioServices
	regs              []ResourceTypeRegistration
	deactivateMissing bool
	calls             int
}

func (m *mockStudioServices) RegisterResourceTypes(ctx context.Context, regs []ResourceTypeRegistration, deactivateMissing bool) (uint32, uint32, error) {
	m.calls++
	m.regs = regs
	m.deactivateMissing = deactivateMissing
	return uint32(len(regs)), 0, nil
}

type mockBrokerWithStudio struct {
	mockServiceBrokerForTest
	studio StudioServices
}

func (m *mockBrokerWithStudio) Studio() StudioServices { return m.studio }

func TestSyncResourceTypes(t *testing.T) {
	t.Run("forwards registrations with deactivate_missing", func(t *testing.T) {
		studio := &mockStudioServices{}
		ctx := Context{Runtime: RuntimeStudio, Services: &mockBrokerWithStudio{studio: studio}, Context: context.Background()}

		err := SyncResourceTypes(ctx, []*ResourceTypeRegistration{
			{Slug: "agent", Name: "Agent", SupportsSubmissions: true, SubmissionSchema: `{"type":"object"}`},
			nil,
			{Slug: "prompt", Name: "Prompt"},
		})
		require.NoError(t, err)
		assert.Equal(t, 1, studio.calls)
		assert.True(t, studio.deactivateMissing)
		require.Len(t, studio.regs, 2, "nil entries are skipped")
		assert.Equal(t, "agent", studio.regs[0].Slug)
		assert.Equal(t, `{"type":"object"}`, studio.regs[0].SubmissionSchema)
	})

	t.Run("refuses outside the Studio runtime", func(t *testing.T) {
		ctx := Context{Runtime: RuntimeGateway, Services: &mockBrokerWithStudio{studio: &mockStudioServices{}}}
		assert.Error(t, SyncResourceTypes(ctx, nil))
	})

	t.Run("errors when studio services are unavailable", func(t *testing.T) {
		ctx := Context{Runtime: RuntimeStudio, Services: &mockServiceBrokerForTest{}}
		assert.Error(t, SyncResourceTypes(ctx, nil))
	})
}
