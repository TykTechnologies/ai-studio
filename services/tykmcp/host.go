package tykmcp

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// HostConnection is the Tyk Dashboard connection the host application AI
// Studio is embedded in provides (pkg/studio Options.HostTykConnection).
//
// Studio keeps one connection for it under a stable key, so every replica
// upserts the same row. The host is trusted: Studio probes the Dashboard as
// usual and activates the connection without an administrator. URL, OrgID,
// Mode and GatewayURL are the host's and read-only in Studio's
// administration; the rest of the connection (name, sync interval,
// governance, MDCB) is Studio's.
type HostConnection struct {
	// Name names the connection when Studio creates it; administrators may
	// rename it. Empty means "Tyk Dashboard".
	Name string
	// URL is the Dashboard's API base URL as Studio reaches it. It may be
	// on an internal address: the host vouches for it.
	URL string
	// OrgID is the Dashboard organisation Studio works in.
	OrgID string
	// Mode is the trust mode: models.TykConnectionModeCatalogue (the
	// default), Broker or Full. Studio runs at the lower of Mode and what the
	// probe verifies.
	Mode string
	// GatewayURL is the public gateway base URL clients reach MCP proxies on.
	GatewayURL string
	// Token returns the Dashboard API key Studio presents. Studio calls it
	// for every Dashboard request and never stores the result, so key
	// rotation is the host's business. It is called from several goroutines
	// at once and should not block. Required.
	Token func() string
}

// DefaultHostConnectionName names the host-managed connection when the host
// gives no name.
const DefaultHostConnectionName = "Tyk Dashboard"

// ErrHostManaged is returned for a change to what the host application
// provides on the host-managed connection.
var ErrHostManaged = errors.New("this connection is provided by the application AI Studio is embedded in; change it there")

// Validate checks the host's settings without contacting the Dashboard.
func (h *HostConnection) Validate() error {
	if h.Token == nil {
		return errors.New("tyk connection: Token is required")
	}
	u, err := url.Parse(strings.TrimSpace(h.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("tyk connection: URL %q must be an absolute http(s) URL", h.URL)
	}
	if u.User != nil {
		return errors.New("tyk connection: URL must not contain credentials")
	}
	if g := strings.TrimSpace(h.GatewayURL); g != "" {
		gu, err := url.Parse(g)
		if err != nil || (gu.Scheme != "http" && gu.Scheme != "https") || gu.Host == "" {
			return fmt.Errorf("tyk connection: GatewayURL %q must be an absolute http(s) URL", h.GatewayURL)
		}
	}
	if h.Mode != "" && models.TykModeRank(h.Mode) < 0 {
		return fmt.Errorf("tyk connection: Mode %q must be one of %s", h.Mode, strings.Join(models.TykConnectionModes, ", "))
	}
	if len(strings.TrimSpace(h.Name)) > 200 {
		return errors.New("tyk connection: Name is at most 200 characters")
	}
	return nil
}
