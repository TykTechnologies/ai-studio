package api

import (
	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"net/http/httptest"

	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Secret plugin configuration fields used to be masked in the edit form only:
// the plugin detail page and every API response carrying the configuration
// returned them in clear to anyone who may read the plugin
// (midsommar-clean-6bg5.9). They are redacted on every read now, and a write
// that sends the marker back keeps the stored value.

const redactionSecretMark = "S3CR3T-"

// rate-limiter-like schema: a top-level password, a writeOnly token and a
// nested password, as the shipped plugins declare them.
const redactionAPISchema = `{"type":"object","properties":{
	"redis_addr":{"type":"string"},
	"redis_password":{"type":"string","format":"password"},
	"vault_token":{"type":"string","writeOnly":true},
	"secret_encryption_key":{"type":"string","format":"password"},
	"backend":{"type":"object","properties":{"redis":{"type":"object","properties":{
		"host":{"type":"string"},"password":{"type":"string","format":"password"}}}}}
}}`

func redactionTestConfig() map[string]interface{} {
	return map[string]interface{}{
		"redis_addr":            "redis:6379",
		"redis_password":        redactionSecretMark + "redis",
		"vault_token":           redactionSecretMark + "vault",
		"secret_encryption_key": redactionSecretMark + "enc",
		"backend": map[string]interface{}{"redis": map[string]interface{}{
			"host": "cache:6379", "password": redactionSecretMark + "nested"}},
	}
}

// seedSecretPlugin stores a plugin with secrets in its configuration and the
// schema that marks them, attached to an LLM with a per-LLM override that
// holds a secret too.
func seedSecretPlugin(t *testing.T, db *gorm.DB, withSchema bool) (*models.Plugin, *models.LLM) {
	t.Helper()
	plugin := &models.Plugin{Name: "rate-limiter", Command: "file:///nonexistent/rate-limiter",
		HookType: models.HookTypePostAuth, HookTypes: []string{models.HookTypePostAuth, models.HookTypeStudioUI, models.HookTypeAgent},
		IsActive: true, Config: redactionTestConfig(),
		Manifest: map[string]interface{}{"id": "com.example.rate-limiter", "name": "rate-limiter"}}
	require.NoError(t, db.Create(plugin).Error)
	if withSchema {
		require.NoError(t, (&models.PluginConfigSchema{}).Upsert(db, plugin.Command, redactionAPISchema))
	}
	llm := &models.LLM{Name: "gpt", Vendor: models.OPENAI, APIEndpoint: "https://api.openai.com/v1", Active: true}
	require.NoError(t, db.Create(llm).Error)
	require.NoError(t, db.Create(&models.LLMPlugin{LLMID: llm.ID, PluginID: plugin.ID, IsActive: true,
		ConfigOverride: map[string]interface{}{"redis_addr": "override:6379", "redis_password": redactionSecretMark + "override"}}).Error)
	return plugin, llm
}

func storedPluginConfig(t *testing.T, db *gorm.DB, id uint) map[string]interface{} {
	t.Helper()
	var p models.Plugin
	require.NoError(t, db.First(&p, id).Error)
	return p.Config
}

// seedEmbedders stores the objects that embed the plugin in their own
// responses: an App and an agent built on the plugin, whose own configuration
// follows the plugin's schema and holds a secret too.
func seedEmbedders(t *testing.T, db *gorm.DB, plugin *models.Plugin, llm *models.LLM) (*models.App, *models.AgentConfig) {
	t.Helper()
	cred := &models.Credential{KeyID: "agent-app-key", Secret: "agent-app-credential", Active: true}
	require.NoError(t, db.Create(cred).Error)
	app := &models.App{Name: "agent-app", UserID: 1, CredentialID: cred.ID}
	require.NoError(t, db.Create(app).Error)
	require.NoError(t, db.Exec("INSERT INTO app_llms (app_id, llm_id) VALUES (?, ?)", app.ID, llm.ID).Error)
	agent := &models.AgentConfig{Name: "Agent", Slug: "agent", PluginID: plugin.ID, AppID: app.ID, IsActive: true,
		Config: map[string]interface{}{"redis_addr": "agent:6379", "redis_password": redactionSecretMark + "agent"}}
	require.NoError(t, db.Create(agent).Error)
	require.NoError(t, db.Create(&models.InstalledPluginVersion{PluginID: plugin.ID, MarketplacePluginID: "com.example.rate-limiter", InstalledVersion: "1.0.0", InstallSource: "manual"}).Error)
	// Rows that embed the plugin through a relationship: its UI mounts
	// (admin and portal), its stored manifest and a resource type.
	for _, scope := range []string{"admin", "portal"} {
		require.NoError(t, db.Create(&models.UIRegistry{PluginID: plugin.ID, SlotType: "sidebar.section", RoutePattern: "/" + scope + "/plugins/rate-limiter",
			ComponentTag: "rate-limiter-" + scope, EntryPoint: "ui/main.js", IsActive: true, Scope: scope}).Error)
	}
	require.NoError(t, db.Create(&models.PluginResourceType{PluginID: plugin.ID, Slug: "limits", Name: "Limits", IsActive: true, SupportsSubmissions: true}).Error)
	require.NoError(t, db.Create(&models.PluginSchedule{PluginID: plugin.ID, ManifestScheduleID: "sync", Name: "Sync", CronExpr: "0 * * * *", Config: "{}"}).Error)
	return app, agent
}

// sweepMayFail lists the plugin read routes that answer 5xx in this test API
// because they need a running plugin or an OCI client. Neither serialises a
// plugin row.
var sweepMayFail = map[string]bool{
	"/api/v1/plugins/oci/cached":        true, // lists OCI cache entries
	"/api/v1/plugins/:id/config-schema": true, // asks the plugin for its schema
}

// No read route returns a secret. Called as an authenticated administrator
// (who holds plugins:read) on the real router, permission checks on.
//
// The routes known to carry a plugin's configuration must answer 200 and
// still carry its readable fields: a 401 or 404 with an empty body does not
// count as "no secret". Then every other GET route of the management and
// portal APIs is read from the router and searched too, so a route added
// later is covered without touching this test.
func TestPluginConfigRedaction_NoReadRouteReturnsASecret(t *testing.T) {
	for _, withSchema := range []bool{true, false} {
		t.Run(fmt.Sprintf("schema=%v", withSchema), func(t *testing.T) {
			a, admin, _ := setupAuthzAPI(t)
			db := a.service.DB
			// The test service has no manifest service; without one the
			// UI registry routes answer 500 and prove nothing.
			a.service.PluginManifestService = services.NewPluginManifestService(db, nil)
			plugin, llm := seedSecretPlugin(t, db, withSchema)
			_, agent := seedEmbedders(t, db, plugin, llm)
			var schedule models.PluginSchedule
			require.NoError(t, db.Where("plugin_id = ?", plugin.ID).First(&schedule).Error)
			get := func(path string) *httptest.ResponseRecorder {
				return apitest.PerformAuthRequest(a.router, http.MethodGet, path, nil, admin.APIKey)
			}

			carriers := map[string]string{
				"/api/v1/plugins": "redis:6379",
				fmt.Sprintf("/api/v1/plugins/%d", plugin.ID):                        "redis:6379",
				"/api/v1/plugins/ai-studio/manifests":                               "redis:6379",
				"/api/v1/llms":                                                      "redis:6379",
				fmt.Sprintf("/api/v1/llms/%d", llm.ID):                              "redis:6379",
				fmt.Sprintf("/api/v1/llms/%d/plugins", llm.ID):                      "redis:6379",
				fmt.Sprintf("/api/v1/llms/%d/plugins/%d/config", llm.ID, plugin.ID): "override:6379",
				"/api/v1/agents":                                                    "redis:6379",
				// The UI registries embed the plugin of each mount; the
				// frontend reads them on every admin and portal page.
				"/api/v1/plugins/ui-registry": "redis:6379",
				// The portal registry carries the mount but no configuration at all.
				"/common/plugins/portal-ui-registry": "rate-limiter-portal",
				// A schedule embeds its plugin.
				fmt.Sprintf("/api/v1/plugins/%d/schedules/%d", plugin.ID, schedule.ID): "redis:6379",
				fmt.Sprintf("/api/v1/agents/%d", agent.ID):                             "redis:6379",
			}
			for path, readable := range carriers {
				w := get(path)
				require.Equal(t, http.StatusOK, w.Code, "GET %s must answer: %s", path, w.Body.String())
				assert.Contains(t, w.Body.String(), readable, "GET %s carries the plugin's readable configuration", path)
				assert.NotContains(t, w.Body.String(), redactionSecretMark, "GET %s returns a secret", path)
			}
			// A schedule update answers with the schedule, plugin embedded.
			w := apitest.PerformAuthRequest(a.router, http.MethodPut, fmt.Sprintf("/api/v1/plugins/%d/schedules/%d", plugin.ID, schedule.ID),
				map[string]interface{}{"name": "Sync hourly"}, admin.APIKey)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.Contains(t, w.Body.String(), "Sync hourly")
			assert.NotContains(t, w.Body.String(), redactionSecretMark, "PUT schedule returns a secret")
			// The agent's own configuration follows the plugin's schema.
			for _, path := range []string{"/api/v1/agents", fmt.Sprintf("/api/v1/agents/%d", agent.ID)} {
				assert.Contains(t, get(path).Body.String(), "agent:6379", path)
			}

			fill := strings.NewReplacer(
				":pluginId", fmt.Sprint(plugin.ID), ":plugin_id", fmt.Sprint(plugin.ID),
				":type", models.HookTypePostAuth, ":hookType", models.HookTypePostAuth,
				":object_type", "llm", ":object_id", fmt.Sprint(llm.ID),
				":schedule_id", fmt.Sprint(schedule.ID),
			)
			called := 0
			for _, route := range a.router.Routes() {
				if route.Method != http.MethodGet || strings.Contains(route.Path, "*") {
					continue
				}
				if !strings.HasPrefix(route.Path, "/api/v1/") && !strings.HasPrefix(route.Path, "/common/") {
					continue
				}
				if strings.Contains(route.Path, "stream") || strings.Contains(route.Path, "/sse") || strings.Contains(route.Path, "export") {
					continue // long-lived or file responses
				}
				id := plugin.ID
				switch {
				case strings.Contains(route.Path, "/llms"):
					id = llm.ID
				case strings.Contains(route.Path, "/agents"):
					id = agent.ID
				}
				path := strings.ReplaceAll(fill.Replace(route.Path), ":id", fmt.Sprint(id))
				if strings.Contains(path, ":") {
					continue // a parameter this test has no object for
				}
				w := get(path)
				called++
				require.NotEqual(t, http.StatusUnauthorized, w.Code, "GET %s: the sweep must be authenticated", route.Path)
				// A 5xx body proves nothing: the UI registry leak hid behind
				// a 500 from a service this test API did not have. So no
				// plugin route may fail here unnoticed.
				if w.Code >= 500 && strings.Contains(route.Path, "plugin") && !sweepMayFail[route.Path] {
					t.Errorf("GET %s answered %d, so the sweep did not see its response: %s", route.Path, w.Code, w.Body.String())
				}
				assert.NotContains(t, w.Body.String(), redactionSecretMark, "GET %s (%d) returns a secret", route.Path, w.Code)
			}
			assert.Greater(t, called, 150, "the management and portal read routes were called")
		})
	}
}

// The agent create and update responses carry the embedded plugin and the
// agent's own configuration redacted, and an update that sends the marker
// back keeps the stored secret.
func TestPluginConfigRedaction_AgentWrites(t *testing.T) {
	a, admin, _ := setupAuthzAPI(t)
	db := a.service.DB
	plugin, llm := seedSecretPlugin(t, db, true)
	app, _ := seedEmbedders(t, db, plugin, llm)
	do := func(method, path string, body interface{}) *httptest.ResponseRecorder {
		return apitest.PerformAuthRequest(a.router, method, path, body, admin.APIKey)
	}

	w := do(http.MethodPost, "/api/v1/agents", map[string]any{"name": "Fresh agent", "plugin_id": plugin.ID, "app_id": app.ID,
		"config": map[string]any{"redis_addr": "fresh:6379", "redis_password": redactionSecretMark + "fresh"}})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), redactionSecretMark, "the create response echoes no secret")
	assert.Contains(t, w.Body.String(), "fresh:6379")
	assert.Contains(t, w.Body.String(), "redis:6379", "the embedded plugin's readable configuration")
	var created struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	path := fmt.Sprintf("/api/v1/agents/%d", created.Data.ID)

	w = do(http.MethodPut, path, map[string]any{"name": "Fresh agent", "plugin_id": plugin.ID, "app_id": app.ID,
		"config": map[string]any{"redis_addr": "moved:6379", "redis_password": services.REDACTED_VALUE}})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), redactionSecretMark, "the update response echoes no secret")
	var stored models.AgentConfig
	require.NoError(t, db.First(&stored, created.Data.ID).Error)
	assert.Equal(t, map[string]interface{}{"redis_addr": "moved:6379", "redis_password": redactionSecretMark + "fresh"}, stored.Config)

	// The marker outside a secret field is refused and nothing changes.
	w = do(http.MethodPut, path, map[string]any{"name": "Fresh agent", "plugin_id": plugin.ID, "app_id": app.ID,
		"config": map[string]any{"redis_addr": services.REDACTED_VALUE}})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	require.NoError(t, db.First(&stored, created.Data.ID).Error)
	assert.Equal(t, "moved:6379", stored.Config["redis_addr"])
}

func TestPluginConfigRedaction_ReadShape(t *testing.T) {
	a, db := setupTestAPI(t)
	plugin, llm := seedSecretPlugin(t, db, true)

	w := performRequest(a.router, http.MethodGet, fmt.Sprintf("/api/v1/plugins/%d", plugin.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got struct {
		Data struct {
			Attributes struct {
				Config map[string]interface{} `json:"config"`
			} `json:"attributes"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	cfg := got.Data.Attributes.Config
	assert.Equal(t, "redis:6379", cfg["redis_addr"])
	assert.Equal(t, services.REDACTED_VALUE, cfg["redis_password"])
	assert.Equal(t, services.REDACTED_VALUE, cfg["vault_token"])
	assert.Equal(t, services.REDACTED_VALUE, cfg["secret_encryption_key"])
	assert.Equal(t, map[string]interface{}{"host": "cache:6379", "password": services.REDACTED_VALUE},
		cfg["backend"].(map[string]interface{})["redis"])

	w = performRequest(a.router, http.MethodGet, fmt.Sprintf("/api/v1/llms/%d/plugins/%d/config", llm.ID, plugin.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"redis_password":"[redacted]"`)
	assert.Contains(t, w.Body.String(), `"redis_addr":"override:6379"`)

	// A reference is shown as the reference, and a plugin whose schema marks
	// nothing is returned unchanged.
	require.NoError(t, db.Model(&models.Plugin{}).Where("id = ?", plugin.ID).Update("config",
		models.JSONMap{"redis_addr": "r", "redis_password": "$SECRET/REDIS_PW"}).Error)
	w = performRequest(a.router, http.MethodGet, fmt.Sprintf("/api/v1/plugins/%d", plugin.ID), nil)
	assert.Contains(t, w.Body.String(), `"redis_password":"$SECRET/REDIS_PW"`)

	plain := &models.Plugin{Name: "plain", Command: "file:///nonexistent/plain", HookType: models.HookTypePostAuth,
		Config: map[string]interface{}{"url": "http://x", "password_hint": "kept", "n": float64(3)}}
	require.NoError(t, db.Create(plain).Error)
	require.NoError(t, (&models.PluginConfigSchema{}).Upsert(db, plain.Command,
		`{"type":"object","properties":{"url":{"type":"string"},"password_hint":{"type":"string"},"n":{"type":"integer"}}}`))
	w = performRequest(a.router, http.MethodGet, fmt.Sprintf("/api/v1/plugins/%d", plain.ID), nil)
	assert.Contains(t, w.Body.String(), `"password_hint":"kept"`)
}

// Saving the form as it was loaded keeps the secrets; a new value replaces
// one; an empty value clears one; and no write response echoes a secret.
func TestPluginConfigRedaction_WriteKeepsReplacesAndClears(t *testing.T) {
	a, db := setupTestAPI(t)
	plugin, llm := seedSecretPlugin(t, db, true)
	path := fmt.Sprintf("/api/v1/plugins/%d", plugin.ID)

	w := performRequest(a.router, http.MethodGet, path, nil)
	var got struct {
		Data struct {
			Attributes struct {
				Config map[string]interface{} `json:"config"`
			} `json:"attributes"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	form := got.Data.Attributes.Config

	// Saved unchanged, with a sibling edited.
	form["redis_addr"] = "moved:6379"
	w = performRequest(a.router, http.MethodPatch, path, map[string]any{"config": form, "load_immediately": false})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), redactionSecretMark, "the write response is redacted too")
	stored := storedPluginConfig(t, db, plugin.ID)
	want := redactionTestConfig()
	want["redis_addr"] = "moved:6379"
	assert.Equal(t, want, stored, "every secret kept, the marker never stored")

	// A new value and a cleared one.
	form["vault_token"] = "new-token-value"
	form["redis_password"] = ""
	w = performRequest(a.router, http.MethodPatch, path, map[string]any{"config": form, "load_immediately": false})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "new-token-value", "a secret just sent is not echoed")
	stored = storedPluginConfig(t, db, plugin.ID)
	assert.Equal(t, "new-token-value", stored["vault_token"])
	assert.Equal(t, "", stored["redis_password"])
	assert.Equal(t, redactionSecretMark+"enc", stored["secret_encryption_key"])
	assert.Equal(t, redactionSecretMark+"nested", stored["backend"].(map[string]interface{})["redis"].(map[string]interface{})["password"])

	// The per-LLM override: marker kept, response redacted.
	opath := fmt.Sprintf("/api/v1/llms/%d/plugins/%d/config", llm.ID, plugin.ID)
	w = performRequest(a.router, http.MethodPut, opath, map[string]any{"config_override": map[string]any{
		"redis_addr": "o2:6379", "redis_password": services.REDACTED_VALUE}})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), redactionSecretMark)
	var lp models.LLMPlugin
	require.NoError(t, lp.Get(db, llm.ID, plugin.ID))
	assert.Equal(t, map[string]interface{}{"redis_addr": "o2:6379", "redis_password": redactionSecretMark + "override"}, map[string]interface{}(lp.ConfigOverride))
}

// The marker must not become an oracle: it is refused outside a secret
// field, never stored as a credential, and the error never echoes a value.
func TestPluginConfigRedaction_MarkerMisuse(t *testing.T) {
	a, db := setupTestAPI(t)
	plugin, llm := seedSecretPlugin(t, db, true)
	path := fmt.Sprintf("/api/v1/plugins/%d", plugin.ID)
	before := storedPluginConfig(t, db, plugin.ID)

	for name, config := range map[string]map[string]any{
		"marker in a readable field":        {"redis_addr": services.REDACTED_VALUE, "redis_password": services.REDACTED_VALUE},
		"marker under a new key":            {"copy_of_password": services.REDACTED_VALUE},
		"marker in a readable nested field": {"backend": map[string]any{"redis": map[string]any{"host": services.REDACTED_VALUE}}},
	} {
		t.Run(name, func(t *testing.T) {
			w := performRequest(a.router, http.MethodPatch, path, map[string]any{"config": config, "load_immediately": false})
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			assert.NotContains(t, w.Body.String(), redactionSecretMark)
			assert.Equal(t, before, storedPluginConfig(t, db, plugin.ID), "nothing was stored")
		})
	}

	w := performRequest(a.router, http.MethodPut, fmt.Sprintf("/api/v1/llms/%d/plugins/%d/config", llm.ID, plugin.ID),
		map[string]any{"config_override": map[string]any{"redis_addr": services.REDACTED_VALUE}})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	// A marker for a secret that has no stored value stores nothing.
	require.NoError(t, db.Model(&models.Plugin{}).Where("id = ?", plugin.ID).Update("config", models.JSONMap{"redis_addr": "r"}).Error)
	w = performRequest(a.router, http.MethodPatch, path, map[string]any{"load_immediately": false,
		"config": map[string]any{"redis_addr": "r", "redis_password": services.REDACTED_VALUE}})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, map[string]interface{}{"redis_addr": "r"}, storedPluginConfig(t, db, plugin.ID))
}

// A stored row that embeds a plugin in its JSON carries the plugin's
// configuration to whoever the row is served to. Each such field is listed
// here with what keeps its configuration from leaving; a new one fails this
// test until it is looked at and added.
func TestPluginConfigRedaction_KnownPluginCarriers(t *testing.T) {
	known := map[string]string{
		"agent_config.go:Plugin":         "redactAgentWith",
		"llm.go:Plugins":                 "serializeLLM builds PluginResponse; RedactLLMCredentials drops config",
		"llm_plugin.go:Plugin":           "the LLM plugin routes build PluginResponse with a redactor",
		"plugin_manifest.go:Plugin":      "UIRegistry: redactRegistryPlugins; RegisteredPlugin is never served",
		"plugin_schedule.go:Plugin":      "redactSchedulePlugin",
		"marketplace.go:Plugin":          "InstalledPluginVersion is never served as a row",
		"plugin_resource_type.go:Plugin": "serializePluginResourceType emits plugin_id and plugin_name only",
	}
	field := regexp.MustCompile("^\\s+(\\w+)\\s+(?:\\*|\\[\\]\\*?)?Plugin\\s+`[^`]*json:\"([^\"]*)\"")
	files, err := filepath.Glob("../models/*.go")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	found := map[string]bool{}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		src, err := os.ReadFile(file)
		require.NoError(t, err)
		for _, line := range strings.Split(string(src), "\n") {
			m := field.FindStringSubmatch(line)
			if m == nil || m[2] == "-" {
				continue
			}
			key := filepath.Base(file) + ":" + m[1]
			found[key] = true
			assert.Contains(t, known, key, "models/%s embeds a plugin in its JSON (field %s): redact its configuration wherever the row is served, cover the route in the sweep above, then list it here", filepath.Base(file), m[1])
		}
	}
	for key := range known {
		assert.True(t, found[key], "%s is listed but no longer exists; remove it", key)
	}
}

// The UI registries are read by people who have no business with plugin
// configuration: every admin-console role loads the admin one, every portal
// user the portal one. Redaction follows the plugin's schema, so it cannot
// protect a secret under a key the schema does not mark. Those readers get
// the mount and the plugin's identity, and no configuration.
func TestPluginConfigRedaction_RegistryConfigNeedsPluginsRead(t *testing.T) {
	for _, withSchema := range []bool{true, false} {
		t.Run(fmt.Sprintf("schema=%v", withSchema), func(t *testing.T) {
			a, admin, member := setupAuthzAPI(t)
			db := a.service.DB
			a.service.PluginManifestService = services.NewPluginManifestService(db, nil)
			plugin, llm := seedSecretPlugin(t, db, withSchema)
			seedEmbedders(t, db, plugin, llm)
			// Secrets the schema does not mark and no key-name rule knows,
			// and a credential in the command line.
			cfg := redactionTestConfig()
			cfg["upstream"] = redactionSecretMark + "unmarked"
			cfg["dsn"] = "postgres://svc:" + redactionSecretMark + "dsn@db:5432/x"
			cfg["signing_key"] = redactionSecretMark + "signing"
			cfg["pat"] = redactionSecretMark + "pat"
			plugin.Config = cfg
			plugin.Command = "grpc://svc:" + redactionSecretMark + "cmd@plugins.internal:9000"
			require.NoError(t, db.Save(plugin).Error)
			if withSchema {
				require.NoError(t, (&models.PluginConfigSchema{}).Upsert(db, plugin.Command, redactionAPISchema))
			}

			type registryBody struct {
				Data []struct {
					ComponentTag string `json:"component_tag"`
					Plugin       *struct {
						ID      uint                   `json:"id"`
						Name    string                 `json:"name"`
						Config  map[string]interface{} `json:"config"`
						Command string                 `json:"command"`
					} `json:"plugin"`
				} `json:"data"`
			}
			decode := func(w *httptest.ResponseRecorder) registryBody {
				t.Helper()
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				var body registryBody
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
				require.Len(t, body.Data, 1, w.Body.String())
				require.NotNil(t, body.Data[0].Plugin)
				assert.Equal(t, "rate-limiter", body.Data[0].Plugin.Name, "the mount still names its plugin")
				return body
			}

			t.Run("portal user", func(t *testing.T) {
				w := apitest.PerformAuthRequest(a.router, http.MethodGet, "/common/plugins/portal-ui-registry", nil, member.APIKey)
				body := decode(w)
				assert.Equal(t, "rate-limiter-portal", body.Data[0].ComponentTag)
				assert.Nil(t, body.Data[0].Plugin.Config, "a portal user gets no plugin configuration")
				assert.Empty(t, body.Data[0].Plugin.Command, "nor its command")
				assert.NotContains(t, w.Body.String(), redactionSecretMark)
				assert.NotContains(t, w.Body.String(), "redis:6379")
			})

			t.Run("administrator on the portal route", func(t *testing.T) {
				w := apitest.PerformAuthRequest(a.router, http.MethodGet, "/common/plugins/portal-ui-registry", nil, admin.APIKey)
				body := decode(w)
				assert.Nil(t, body.Data[0].Plugin.Config, "the portal route never carries configuration")
				assert.Empty(t, body.Data[0].Plugin.Command, "nor the command")
				assert.NotContains(t, w.Body.String(), redactionSecretMark)
			})

			// The admin route under the real permission gate, with the
			// caller's grants set by the test.
			var grants authz.Grants
			engine := gin.New()
			engine.GET("/api/v1/plugins/ui-registry", func(c *gin.Context) {
				c.Set("user", member)
				g := grants
				authz.WithContext(c, authz.NewGrantsContext(member.ID, false, func() (authz.Grants, error) { return g, nil }))
			}, a.requireRoutePermission(), a.getUIRegistry)
			call := func(g authz.Grants) *httptest.ResponseRecorder {
				grants = g
				w := httptest.NewRecorder()
				engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/plugins/ui-registry", nil))
				return w
			}
			var entry models.UIRegistry
			require.NoError(t, db.Preload("Plugin").Where("scope = ?", "admin").First(&entry).Error)
			page := authz.Permission(services.RequiredPermissionOf(&entry))

			t.Run("Viewer without plugins:read", func(t *testing.T) {
				// Holds what the plugin's page needs and read on other
				// things, but not plugins:read.
				w := call(authz.GlobalGrants(authz.NewSet(page, authz.Read("llms"), authz.Read("apps"))))
				body := decode(w)
				assert.Equal(t, "rate-limiter-admin", body.Data[0].ComponentTag)
				assert.Nil(t, body.Data[0].Plugin.Config, "no plugins:read, no configuration")
				assert.Empty(t, body.Data[0].Plugin.Command, "and no command")
				assert.NotContains(t, w.Body.String(), redactionSecretMark, "not even a secret the schema does not mark")
				assert.NotContains(t, w.Body.String(), "redis:6379")
			})

			t.Run("holder of plugins:read", func(t *testing.T) {
				w := call(authz.GlobalGrants(authz.NewSet(page, authz.Read("plugins"))))
				body := decode(w)
				require.NotNil(t, body.Data[0].Plugin.Config)
				assert.Equal(t, "redis:6379", body.Data[0].Plugin.Config["redis_addr"])
				assert.Equal(t, services.REDACTED_VALUE, body.Data[0].Plugin.Config["redis_password"])
				assert.NotEmpty(t, body.Data[0].Plugin.Command, "a plugins:read holder sees the command, as on the plugin routes")
			})
		})
	}
}
