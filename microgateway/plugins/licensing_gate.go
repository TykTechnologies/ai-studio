package plugins

import "errors"

// ErrLicensingNotConfigured is why a plugin is not started while the
// gateway's service broker has no licensing service yet.
var ErrLicensingNotConfigured = errors.New("the gateway's licensing service is not set yet; plugins are not started before it is")

// licensingGate is the part of the management server (the plugins' service
// broker) that says whether it can answer GetLicenseInfo yet.
type licensingGate interface {
	LicensingConfigured() bool
}

// checkLicensingConfigured refuses to start a plugin before the service
// broker it will be given can answer GetLicenseInfo. An Enterprise plugin
// asks as soon as its session is ready; one started earlier was told
// "community" and exited, and its LLMs answered 500 until the health check
// restarted it (2.3.0-rc4 finding M3). main wires licensing before anything
// loads plugins; this keeps a later change of that order from going unseen.
// A manager without a service broker gives plugins no license API at all,
// so there is nothing to wait for.
func (pm *PluginManager) checkLicensingConfigured() error {
	if gate, ok := pm.GetManagementServer().(licensingGate); ok && !gate.LicensingConfigured() {
		return ErrLicensingNotConfigured
	}
	return nil
}
