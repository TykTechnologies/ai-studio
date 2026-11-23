package api

import (
	"errors"
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
)

// Secret plugin configuration fields never leave the management API: every
// response that carries a plugin's configuration (or a per-LLM override of
// it) is built through a pluginRedactor, and every write that takes one goes
// through restorePluginConfig, which keeps the stored value where the client
// sends the redaction marker back. See services/plugin_config_redaction.go
// for which fields are secret.
//
// serializePlugin takes the redactor as an argument so a new call site
// cannot serialise a plugin without one.

// pluginRedactor holds the secret-field classifiers of the plugins about to
// be serialised, by command. A command it does not know (no cached schema)
// is classified by key name.
type pluginRedactor map[string]*services.PluginConfigSecrets

// byKeyName classifies every plugin by key name. For code paths without a
// database (unit tests of the serialiser).
var byKeyName = pluginRedactor{}

func (r pluginRedactor) secrets(command string) *services.PluginConfigSecrets {
	return r[command] // nil classifies by key name
}

// config is the plugin's configuration as the API returns it.
func (r pluginRedactor) config(plugin *models.Plugin) map[string]interface{} {
	return r.secrets(plugin.Command).RedactPluginConfig(plugin.Config)
}

// override is a per-LLM override of plugin's configuration as the API
// returns it: the same schema, the same secrets.
func (r pluginRedactor) override(plugin *models.Plugin, override map[string]interface{}) map[string]interface{} {
	return r.secrets(plugin.Command).RedactPluginConfig(override)
}

// pluginRedactorFor loads the classifiers for plugins in one query.
func (a *API) pluginRedactorFor(plugins ...*models.Plugin) pluginRedactor {
	if a == nil || a.service == nil || a.service.PluginService == nil {
		return byKeyName
	}
	seen := make(map[string]bool, len(plugins))
	commands := make([]string, 0, len(plugins))
	for _, p := range plugins {
		if p == nil || seen[p.Command] {
			continue
		}
		seen[p.Command] = true
		commands = append(commands, p.Command)
	}
	return pluginRedactor(a.service.PluginService.ConfigSecretsForCommands(commands))
}

// pluginRedactorForList is pluginRedactorFor for a slice of plugins.
func (a *API) pluginRedactorForList(plugins []models.Plugin) pluginRedactor {
	ptrs := make([]*models.Plugin, len(plugins))
	for i := range plugins {
		ptrs[i] = &plugins[i]
	}
	return a.pluginRedactorFor(ptrs...)
}

// redactRegistryPlugins replaces the plugin each UI registry entry embeds
// with a copy that is safe to serve. The entries are served as stored rows,
// so without this the embedded plugin carries its secrets to every caller of
// the registry routes (midsommar-clean-6bg5.9).
//
// withConfig says whether the caller may read plugin configuration (the
// mayReadPluginConfig rule: plugins:read). Such a caller gets the
// configuration with its secret fields redacted, as on the plugin routes.
// Anyone else gets neither the configuration nor the command: redaction
// follows the plugin's schema, so a secret under a key the schema does not
// mark, or of a plugin with no cached schema, would otherwise reach readers
// who have no use for the configuration; and a command line can hold a
// credential (a URL with userinfo).
func (a *API) redactRegistryPlugins(entries []models.UIRegistry, withConfig bool) {
	var red pluginRedactor
	if withConfig {
		plugins := make([]*models.Plugin, 0, len(entries))
		for i := range entries {
			if entries[i].Plugin != nil {
				plugins = append(plugins, entries[i].Plugin)
			}
		}
		if len(plugins) > 0 {
			red = a.pluginRedactorFor(plugins...)
		}
	}
	for i := range entries {
		if entries[i].Plugin == nil {
			continue
		}
		plugin := *entries[i].Plugin
		if withConfig {
			plugin.Config = red.config(&plugin)
		} else {
			plugin.Config = nil
			plugin.Command = ""
		}
		entries[i].Plugin = &plugin
	}
}

// redactSchedulePlugin does the same for the plugin a schedule embeds.
func (a *API) redactSchedulePlugin(schedule *models.PluginSchedule) {
	if schedule == nil || schedule.Plugin.ID == 0 {
		return
	}
	schedule.Plugin.Config = a.pluginRedactorFor(&schedule.Plugin).config(&schedule.Plugin)
}

// serializePlugin is serializePlugin with the plugin's own redactor.
func (a *API) serializePlugin(plugin *models.Plugin) PluginResponse {
	return serializePlugin(plugin, a.pluginRedactorFor(plugin))
}

// restorePluginConfig prepares a configuration a client sent for plugin
// (command) for storing: the redaction marker at a secret field becomes the
// value stored there (stored is the current configuration, nil on create).
// It answers 400 and returns false when the marker is used where no stored
// secret can be kept; the message names the field, never a value.
func (a *API) restorePluginConfig(c *gin.Context, command string, incoming, stored map[string]interface{}) (map[string]interface{}, bool) {
	if incoming == nil {
		return nil, true
	}
	red := a.pluginRedactorFor(&models.Plugin{Command: command})
	out, err := red.secrets(command).RestorePluginConfig(incoming, stored)
	if err != nil {
		detail := "invalid plugin configuration"
		var me *services.PluginConfigMarkerError
		if errors.As(err, &me) {
			detail = me.Error()
		}
		c.AbortWithStatusJSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: detail}},
		})
		return nil, false
	}
	return out, true
}

// pluginCommand is the command of plugin id, "" when it cannot be loaded
// (its configuration is then classified by key name).
func (a *API) pluginCommand(id uint) string {
	if a == nil || a.service == nil || a.service.PluginService == nil {
		return ""
	}
	plugin, err := a.service.PluginService.GetPlugin(id)
	if err != nil || plugin == nil {
		return ""
	}
	return plugin.Command
}

// redactPluginOverride is a per-LLM configuration override of plugin id as
// the API returns it.
func (a *API) redactPluginOverride(id uint, override map[string]interface{}) map[string]interface{} {
	plugin := &models.Plugin{Command: a.pluginCommand(id)}
	return a.pluginRedactorFor(plugin).override(plugin, override)
}
