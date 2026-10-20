package plugins

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/interfaces"
	"github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/sdk"
	"github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
	"github.com/TykTechnologies/midsommar/v2/pkg/pluginenv"
	"github.com/TykTechnologies/midsommar/v2/pkg/pluginlimits"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

// msgSizeHelperEnv makes the test binary serve one of the helper plugins
// below instead of running tests, so the plugin side is a real process
// started through go-plugin, as the gateway starts plugins.
const msgSizeHelperEnv = "MGW_MSG_SIZE_TEST_PLUGIN"

func TestMain(m *testing.M) {
	switch os.Getenv(msgSizeHelperEnv) {
	case "":
		os.Exit(m.Run())
	case "plugin_sdk":
		plugin_sdk.Serve(&upperSDKPlugin{BasePlugin: plugin_sdk.NewBasePlugin("upper-sdk", "1.0.0", "")})
	case "gatewayplugin_sdk":
		sdk.ServePlugin(&upperGatewayPlugin{})
	case "legacy":
		// A plugin built before 2.3: go-plugin's default server, 4 MiB.
		plugin.Serve(&plugin.ServeConfig{
			HandshakeConfig: HandshakeConfig,
			Plugins:         map[string]plugin.Plugin{"plugin": &legacyPluginGRPC{}},
			GRPCServer:      plugin.DefaultGRPCServer,
		})
	}
	os.Exit(0)
}

// upperSDKPlugin upper-cases what it is sent, so a round trip shows the
// plugin saw the whole message and the host got the whole answer.
type upperSDKPlugin struct {
	plugin_sdk.BasePlugin
}

func (p *upperSDKPlugin) HandlePostAuth(_ plugin_sdk.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
	return &pb.PluginResponse{Modified: true, Body: bytes.ToUpper(req.Request.Body)}, nil
}

func (p *upperSDKPlugin) OnBeforeWriteHeaders(_ plugin_sdk.Context, req *pb.HeadersRequest) (*pb.HeadersResponse, error) {
	return &pb.HeadersResponse{Modified: false, Headers: req.Headers}, nil
}

func (p *upperSDKPlugin) OnBeforeWrite(_ plugin_sdk.Context, req *pb.ResponseWriteRequest) (*pb.ResponseWriteResponse, error) {
	return &pb.ResponseWriteResponse{Modified: true, Body: bytes.ToUpper(req.Body), Headers: req.Headers}, nil
}

func (p *upperSDKPlugin) OnStreamComplete(_ plugin_sdk.Context, req *pb.StreamCompleteRequest) (*pb.StreamCompleteResponse, error) {
	if len(req.AccumulatedResponse) != len(req.RequestBody) {
		return &pb.StreamCompleteResponse{ErrorMessage: "lengths differ"}, nil
	}
	return &pb.StreamCompleteResponse{Handled: true}, nil
}

type upperGatewayPlugin struct{}

func (p *upperGatewayPlugin) Initialize(map[string]interface{}) error { return nil }
func (p *upperGatewayPlugin) GetHookType() interfaces.HookType        { return interfaces.HookTypePostAuth }
func (p *upperGatewayPlugin) GetName() string                         { return "upper-gateway" }
func (p *upperGatewayPlugin) GetVersion() string                      { return "1.0.0" }
func (p *upperGatewayPlugin) Shutdown() error                         { return nil }
func (p *upperGatewayPlugin) ProcessRequest(_ context.Context, req *interfaces.EnrichedRequest, _ *interfaces.PluginContext) (*interfaces.PluginResponse, error) {
	return &interfaces.PluginResponse{Modified: true, Body: bytes.ToUpper(req.PluginRequest.Body)}, nil
}

type legacyPluginGRPC struct {
	plugin.NetRPCUnsupportedPlugin
}

func (p *legacyPluginGRPC) GRPCServer(_ *plugin.GRPCBroker, s *grpc.Server) error {
	pb.RegisterPluginServiceServer(s, &legacyPluginServer{})
	return nil
}

func (p *legacyPluginGRPC) GRPCClient(context.Context, *plugin.GRPCBroker, *grpc.ClientConn) (interface{}, error) {
	return nil, nil
}

type legacyPluginServer struct {
	pb.UnimplementedPluginServiceServer
}

func (s *legacyPluginServer) ProcessPostAuth(context.Context, *pb.EnrichedRequest) (*pb.PluginResponse, error) {
	return &pb.PluginResponse{}, nil
}

func (s *legacyPluginServer) ProcessPreAuth(context.Context, *pb.PluginRequest) (*pb.PluginResponse, error) {
	return &pb.PluginResponse{}, nil
}

func (s *legacyPluginServer) Authenticate(context.Context, *pb.AuthRequest) (*pb.AuthResponse, error) {
	return &pb.AuthResponse{Authenticated: true, AppId: "1"}, nil
}

// startHelperPlugin starts the test binary as the named helper plugin and
// returns it as the gateway's manager would have loaded it.
func startHelperPlugin(t *testing.T, mode string, id uint) *LoadedPlugin {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// Started the way the gateway starts plugins: pluginenv's environment
	// and SkipHostEnv.
	cmd := pluginenv.Command(self)
	cmd.Env = append(cmd.Env, msgSizeHelperEnv+"="+mode, "PLUGIN_RUNTIME=gateway")
	pm := NewPluginManager(&mockPluginService{})
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig:  pm.handshakeConfig,
		Plugins:          pm.pluginMap,
		Cmd:              cmd,
		SkipHostEnv:      true,
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		Logger:           hclog.NewNullLogger(),
		StartTimeout:     30 * time.Second,
	})
	t.Cleanup(client.Kill)
	rpcClient, err := client.Client()
	if err != nil {
		t.Fatalf("starting %s helper plugin: %v", mode, err)
	}
	raw, err := rpcClient.Dispense("plugin")
	if err != nil {
		t.Fatal(err)
	}
	return &LoadedPlugin{ID: id, Name: mode + "-plugin", Client: client, GRPCClient: raw.(pb.PluginServiceClient), IsHealthy: true}
}

const llmWithPlugin = 7

// quietLogs turns logging off for a test; the chain logs every plugin call.
func quietLogs(t *testing.T) {
	prev := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.Disabled)
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })
}

func chainManager(lp *LoadedPlugin, hooks ...string) *PluginManager {
	svc := &mockPluginService{pluginsByLLM: map[uint][]PluginData{
		llmWithPlugin: {{ID: lp.ID, Name: lp.Name, HookType: hooks[0], HookTypes: hooks, IsActive: true}},
	}}
	return newTestManager(svc, map[uint]*LoadedPlugin{lp.ID: lp})
}

func postAuthRequest(body []byte) *interfaces.EnrichedRequest {
	pctx := &interfaces.PluginContext{RequestID: "req-1", LLMID: llmWithPlugin}
	return &interfaces.EnrichedRequest{
		PluginRequest: &interfaces.PluginRequest{Method: "POST", Path: "/llm/rest/x/v1/messages", Body: body, Context: pctx},
		AppID:         "1",
		Authenticated: true,
	}
}

// Plugins on the current SDKs take 20 MiB in each direction, through the
// gateway's post_auth chain and its response hooks.
func TestLargeMessagesRoundTripThroughCurrentSDKs(t *testing.T) {
	if testing.Short() {
		t.Skip("starts plugin processes")
	}
	quietLogs(t)
	body := bytes.Repeat([]byte("a"), 20<<20)
	want := bytes.Repeat([]byte("A"), 20<<20)

	t.Run("plugin_sdk", func(t *testing.T) {
		lp := startHelperPlugin(t, "plugin_sdk", 1)
		pm := chainManager(lp, "post_auth", "on_response")

		out, err := pm.ExecutePluginChain(llmWithPlugin, interfaces.HookTypePostAuth, postAuthRequest(body), &interfaces.PluginContext{LLMID: llmWithPlugin})
		if err != nil {
			t.Fatalf("20 MiB post_auth: %v", err)
		}
		if got := out.(*interfaces.EnrichedRequest).PluginRequest.Body; !bytes.Equal(got, want) {
			t.Fatalf("post_auth body: got %d bytes, want the upper-cased %d", len(got), len(want))
		}

		ctx := context.Background()
		wr, err := lp.GRPCClient.OnBeforeWrite(ctx, &pb.ResponseWriteRequest{Body: body, Context: &pb.PluginContext{LlmId: llmWithPlugin}})
		if err != nil {
			t.Fatalf("20 MiB OnBeforeWrite: %v", err)
		}
		if !wr.Modified || !bytes.Equal(wr.Body, want) {
			t.Fatalf("OnBeforeWrite body: got %d bytes, want the upper-cased %d", len(wr.Body), len(want))
		}

		sc, err := lp.GRPCClient.OnStreamComplete(ctx, &pb.StreamCompleteRequest{AccumulatedResponse: body, RequestBody: body, Context: &pb.PluginContext{LlmId: llmWithPlugin}})
		if err != nil {
			t.Fatalf("2 x 20 MiB OnStreamComplete: %v", err)
		}
		if !sc.Handled || sc.ErrorMessage != "" {
			t.Fatalf("OnStreamComplete = %+v", sc)
		}
	})

	t.Run("gatewayplugin_sdk", func(t *testing.T) {
		lp := startHelperPlugin(t, "gatewayplugin_sdk", 2)
		pm := chainManager(lp, "post_auth")
		out, err := pm.ExecutePluginChain(llmWithPlugin, interfaces.HookTypePostAuth, postAuthRequest(body), &interfaces.PluginContext{LLMID: llmWithPlugin})
		if err != nil {
			t.Fatalf("20 MiB post_auth: %v", err)
		}
		if got := out.(*interfaces.EnrichedRequest).PluginRequest.Body; !bytes.Equal(got, want) {
			t.Fatalf("post_auth body: got %d bytes, want the upper-cased %d", len(got), len(want))
		}
	})
}

// A plugin on an older SDK refuses a body over 4 MiB. The chain stops with a
// *TooLargeError naming the plugin (the hook answers 413 and does not forward
// the request); under the limit it still works.
func TestLegacyPluginRefusalIsTooLarge(t *testing.T) {
	if testing.Short() {
		t.Skip("starts plugin processes")
	}
	quietLogs(t)
	lp := startHelperPlugin(t, "legacy", 3)
	pm := chainManager(lp, "pre_auth", "post_auth", "auth")
	pctx := &interfaces.PluginContext{LLMID: llmWithPlugin}

	if _, err := pm.ExecutePluginChain(llmWithPlugin, interfaces.HookTypePostAuth, postAuthRequest(make([]byte, 1<<20)), pctx); err != nil {
		t.Fatalf("1 MiB post_auth to a legacy plugin: %v", err)
	}

	big := make([]byte, 5<<20)
	_, err := pm.ExecutePluginChain(llmWithPlugin, interfaces.HookTypePostAuth, postAuthRequest(big), pctx)
	tl, ok := pluginlimits.AsTooLargeError(err)
	if !ok {
		t.Fatalf("5 MiB post_auth: err = %v, want a TooLargeError", err)
	}
	if tl.Plugin != "legacy-plugin" || tl.Hook != "post_auth" || tl.Bytes < len(big) {
		t.Errorf("TooLargeError = %+v", tl)
	}

	_, err = pm.ExecutePluginChain(llmWithPlugin, interfaces.HookTypePreAuth, postAuthRequest(big).PluginRequest, pctx)
	if _, ok := pluginlimits.AsTooLargeError(err); !ok {
		t.Errorf("5 MiB pre_auth: err = %v, want a TooLargeError", err)
	}

	_, err = pm.CallAuth(lp, &interfaces.AuthRequest{Credential: "k", Request: postAuthRequest(big).PluginRequest}, pctx)
	if tl, ok := pluginlimits.AsTooLargeError(err); !ok || !strings.Contains(err.Error(), "legacy-plugin") {
		t.Errorf("5 MiB auth: err = %v (%+v), want a TooLargeError naming the plugin", err, tl)
	}
}

// The limit set in the gateway's environment reaches plugin processes
// (pluginenv passes it), here lowered to 2 MiB.
func TestPluginLimitEnvOverrideReachesPlugin(t *testing.T) {
	if testing.Short() {
		t.Skip("starts plugin processes")
	}
	quietLogs(t)
	t.Setenv(pluginlimits.EnvMaxMsgBytes, "2097152")
	lp := startHelperPlugin(t, "plugin_sdk", 4)
	pm := chainManager(lp, "post_auth")
	pctx := &interfaces.PluginContext{LLMID: llmWithPlugin}
	if _, err := pm.ExecutePluginChain(llmWithPlugin, interfaces.HookTypePostAuth, postAuthRequest(make([]byte, 1<<20)), pctx); err != nil {
		t.Fatalf("1 MiB under a 2 MiB limit: %v", err)
	}
	_, err := pm.ExecutePluginChain(llmWithPlugin, interfaces.HookTypePostAuth, postAuthRequest(make([]byte, 3<<20)), pctx)
	if _, ok := pluginlimits.AsTooLargeError(err); !ok {
		t.Fatalf("3 MiB over a 2 MiB limit: err = %v, want a TooLargeError", err)
	}
}
