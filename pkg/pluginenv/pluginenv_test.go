package pluginenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	goplugin "github.com/hashicorp/go-plugin"
)

// helperOutEnv names the file the helper process writes its environment to.
// When it is set, the test binary acts as the "plugin": it dumps its
// environment and exits (see TestMain).
const helperOutEnv = "PLUGINENV_HELPER_OUT"

func TestMain(m *testing.M) {
	if out := os.Getenv(helperOutEnv); out != "" {
		_ = os.WriteFile(out, []byte(strings.Join(os.Environ(), "\n")), 0o600)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// hostSecrets stand in for the host's own configuration.
var hostSecrets = map[string]string{
	"ENCRYPTION_KEY":    "host-encryption-key",
	"DATABASE_URL":      "postgres://user:pass@db/studio",
	"TYK_AI_SECRET_KEY": "host-secret",
	"OPENAI_API_KEY":    "sk-host",
}

func setHostEnv(t *testing.T) {
	t.Helper()
	for k, v := range hostSecrets {
		t.Setenv(k, v)
	}
	t.Setenv("TZ", "Europe/Berlin")
	t.Setenv("LC_ALL", "C.UTF-8")
	t.Setenv("HTTPS_PROXY", "http://proxy:3128")
	t.Setenv("GATEWAY_MODE", "edge")
	t.Setenv("GITHUB_RAG_KNOWN_HOSTS", "/etc/ssh/known_hosts")
	t.Setenv("PLUGIN_ONLY_SETTING", "passed")
}

func envMap(kvs []string) map[string]string {
	m := map[string]string{}
	for _, kv := range kvs {
		k, v, _ := strings.Cut(kv, "=")
		m[k] = v
	}
	return m
}

func assertPluginEnv(t *testing.T, got map[string]string) {
	t.Helper()
	for k := range hostSecrets {
		if _, ok := got[k]; ok {
			t.Errorf("plugin environment has %s", k)
		}
	}
	for k, want := range map[string]string{
		"TZ":                     "Europe/Berlin",
		"LC_ALL":                 "C.UTF-8",
		"HTTPS_PROXY":            "http://proxy:3128",
		"GATEWAY_MODE":           "edge",
		"GITHUB_RAG_KNOWN_HOSTS": "/etc/ssh/known_hosts", // prefix pass-through
		"PLUGIN_ONLY_SETTING":    "passed",               // exact pass-through
	} {
		if got[k] != want {
			t.Errorf("plugin environment %s = %q, want %q", k, got[k], want)
		}
	}
}

func TestEnviron_AllowlistAndPassthrough(t *testing.T) {
	setHostEnv(t)
	t.Setenv(EnvPassthrough, " GITHUB_RAG_* , PLUGIN_ONLY_SETTING ,")

	got := envMap(Environ())
	assertPluginEnv(t, got)
	if _, ok := got["PATH"]; !ok && os.Getenv("PATH") != "" {
		t.Error("plugin environment is missing PATH")
	}
}

func TestEnviron_NothingPassedWithoutPassthrough(t *testing.T) {
	setHostEnv(t)
	t.Setenv(EnvPassthrough, "")

	got := envMap(Environ())
	for _, k := range []string{"GITHUB_RAG_KNOWN_HOSTS", "PLUGIN_ONLY_SETTING", "ENCRYPTION_KEY"} {
		if _, ok := got[k]; ok {
			t.Errorf("plugin environment has %s without a pass-through", k)
		}
	}
}

// startHelper starts the test binary as a plugin through go-plugin and
// returns the environment it saw. The helper exits without a handshake, so
// Start fails; only the environment matters here.
func startHelper(t *testing.T, skipHostEnv bool) map[string]string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "env")
	t.Setenv(helperOutEnv, out)

	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := Command(self)
	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig:  goplugin.HandshakeConfig{ProtocolVersion: 1, MagicCookieKey: "PLUGINENV_TEST", MagicCookieValue: "x"},
		Plugins:          map[string]goplugin.Plugin{},
		Cmd:              cmd,
		SkipHostEnv:      skipHostEnv,
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Logger:           hclog.NewNullLogger(),
		StartTimeout:     10 * time.Second,
	})
	defer client.Kill()
	_, _ = client.Start()

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("helper did not write its environment: %v", err)
	}
	return envMap(strings.Split(string(data), "\n"))
}

// A plugin started the way the hosts start it (Command + SkipHostEnv) sees
// the allowlist and the pass-through, and none of the host's secrets.
func TestPluginProcess_GetsOnlyAllowedEnvironment(t *testing.T) {
	setHostEnv(t)
	t.Setenv(EnvPassthrough, "GITHUB_RAG_*,PLUGIN_ONLY_SETTING,"+helperOutEnv)

	got := startHelper(t, true)
	assertPluginEnv(t, got)
	if got["PLUGINENV_TEST"] != "x" {
		t.Errorf("plugin environment lost go-plugin's handshake cookie: %q", got["PLUGINENV_TEST"])
	}
}

// Without SkipHostEnv go-plugin appends the host environment, so the
// allowlisted Cmd alone is not enough. This pins why the hosts set both.
func TestPluginProcess_WithoutSkipHostEnvInheritsHost(t *testing.T) {
	setHostEnv(t)
	t.Setenv(EnvPassthrough, helperOutEnv)

	got := startHelper(t, false)
	if got["ENCRYPTION_KEY"] != hostSecrets["ENCRYPTION_KEY"] {
		t.Fatalf("expected go-plugin to pass the host environment without SkipHostEnv; got ENCRYPTION_KEY=%q", got["ENCRYPTION_KEY"])
	}
}

// Every plugin process the hosts start must use Command and SkipHostEnv.
func TestPluginHosts_UseCommandAndSkipHostEnv(t *testing.T) {
	for _, path := range []string{
		"../../services/ai_studio_plugin_manager.go",
		"../../microgateway/plugins/manager.go",
	} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		s := string(src)
		cmds := strings.Count(s, "Cmd:")
		if cmds == 0 {
			t.Fatalf("%s starts no plugin process; update this test", path)
		}
		if n := strings.Count(s, "pluginenv.Command("); n != cmds {
			t.Errorf("%s: %d plugin Cmd fields, %d use pluginenv.Command", path, cmds, n)
		}
		if n := strings.Count(s, "SkipHostEnv:      true,"); n != cmds {
			t.Errorf("%s: %d plugin Cmd fields, %d set SkipHostEnv", path, cmds, n)
		}
	}
}
