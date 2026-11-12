package grpc

import (
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/rs/zerolog/log"
)

// Starting an edge without a control plane (features/ActiveActive.md, Phase
// 3). With EDGE_START_FROM_LAST_KNOWN_GOOD, an edge that finds no control
// plane within EDGE_STARTUP_CONTROL_TIMEOUT serves the configuration
// snapshot it last applied, which it keeps encrypted in its own database,
// and goes on trying to connect in the background. The first registration a
// control plane accepts brings the live configuration, as any reconnect
// does. Without the option an edge exits when its first connection fails,
// as it always has.

// Configuration sources, as /ready and microgateway_config_source report
// them.
const (
	ConfigSourceControl       = "control"
	ConfigSourceLastKnownGood = "last_known_good"
)

// StartWithLastKnownGood starts the edge. It connects to a control plane as
// Start does; when EDGE_START_FROM_LAST_KNOWN_GOOD is set and none accepts
// the edge within EDGE_STARTUP_CONTROL_TIMEOUT, it returns the snapshot
// stored in db for the caller to apply and serve, having made it the
// client's loaded configuration. The caller then calls StartReconnecting
// once it can apply the configuration a control plane will send.
//
// It returns nil, nil when connected. The error is the connection error
// when the option is off (exactly Start's), and otherwise one that also
// says why the stored snapshot cannot be used (a
// *services.LastKnownGoodRefusal): the edge must exit.
func (c *SimpleEdgeClient) StartWithLastKnownGood(db *gorm.DB) (*services.LastKnownGood, error) {
	hs := c.config.HubSpoke
	// The contact a registration is about to make is recorded
	// authenticated with the key of the database's snapshot store, so the
	// store is set up before the first registration, with the option on or
	// off; the service container sets it up too, later in the start.
	// Without the option a key the store cannot be made from is left for
	// the container to report, as before: Start's behaviour is unchanged.
	err := services.PersistAppliedSnapshots(db, c.config.Security.EncryptionKey)
	if !hs.StartFromLastKnownGood {
		return nil, c.Start()
	}
	if err != nil {
		return nil, err
	}
	connErr := c.StartWithin(hs.StartupControlTimeout)
	if connErr == nil {
		return nil, nil
	}
	// A failed start leaves its closed connection as the current one (an
	// edge without this option exits at that point). This edge may go on:
	// it has no connection until the reconnect loop makes one.
	c.setConnection(nil, "")

	lkg, err := services.LoadLastKnownGood(db, c.config.Security.EncryptionKey, hs.EdgeNamespace, hs.LastKnownGoodMaxAge, time.Now())
	if err != nil {
		return nil, fmt.Errorf("no control plane answered (%v) and the edge cannot start from its last-known-good configuration: %w", connErr, err)
	}
	services.KeepStoredSnapshot(db, lkg)
	c.UseLastKnownGood(lkg)
	log.Warn().
		Err(connErr).
		Strs("control_endpoints", c.endpoints).
		Dur("waited", hs.StartupControlTimeout).
		Str("config_source", ConfigSourceLastKnownGood).
		Str("snapshot_version", lkg.Version).
		Str("snapshot_checksum", lkg.Checksum).
		Time("applied_at", lkg.AppliedAt).
		Time("last_control_contact", lkg.LastContact).
		Msgf("No control plane answered: serving the last-known-good configuration, a snapshot of %s from %s (applied %s). Changes made on the control plane since, revoked keys included, do not apply until this edge reconnects",
			time.Since(lkg.LastContact).Round(time.Second), lkg.LastContact.UTC().Format(time.RFC3339), lkg.AppliedAt.UTC().Format(time.RFC3339))
	return lkg, nil
}

// StartWithin is Start, repeated with the reconnect backoff until a control
// plane accepts the edge or timeout has passed; the last attempt is made
// when it runs out. It returns the last attempt's error. A timeout of zero
// makes one attempt.
//
// A control plane that takes the connection and does not answer is given
// until the timeout runs out, and at least 5 s of it (or the whole timeout
// if that is shorter), so the last attempt is a real one: the start-up
// takes at most the timeout and that much again.
func (c *SimpleEdgeClient) StartWithin(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	if timeout > 0 {
		c.startupDeadline = deadline
		c.startupMinAttempt = min(controlConnectTimeout, timeout)
		defer func() { c.startupDeadline = time.Time{} }()
	}
	base := c.reconnectInterval
	for attempt := 1; ; attempt++ {
		err := c.Start()
		if err == nil {
			return nil
		}
		left := time.Until(deadline)
		if timeout <= 0 || left <= 0 || c.stopping() {
			return err
		}
		wait := c.calculateBackoffDelay(base, reconnectBackoffCap(base), 2.0, 0.1, attempt)
		if wait > left {
			wait = left
		}
		log.Warn().Err(err).
			Dur("retry_in", wait).
			Dur("giving_up_in", time.Until(deadline).Round(time.Second)).
			Msg("No control plane accepted this edge yet, trying again before falling back to last-known-good")
		select {
		case <-time.After(wait):
		case <-c.stopCh:
			return err
		}
	}
}

// UseLastKnownGood makes the stored snapshot the client's loaded
// configuration and marks the edge as serving last-known-good.
func (c *SimpleEdgeClient) UseLastKnownGood(lkg *services.LastKnownGood) {
	c.setLoadedConfig(lkg.Snapshot)
	c.lastContact.Store(lkg.LastContact.UnixNano())
	c.lastKnownGood.Store(true)
}

// StartReconnecting starts the reconnect loop for an edge that started
// without a control plane. It returns at once; the loop runs until a control
// plane accepts the edge or the client stops.
func (c *SimpleEdgeClient) StartReconnecting() {
	go c.attemptReconnection()
}

// SetControlContactHook has fn called, with the time, whenever a control
// plane accepts a registration or acknowledges a heartbeat. The edge
// records it (services.RecordControlContact) so a later start without a
// control plane knows how long it has been without one.
func (c *SimpleEdgeClient) SetControlContactHook(fn func(time.Time)) {
	if fn == nil {
		c.onContact.Store(nil)
		return
	}
	c.onContact.Store(&fn)
}

// SetRegisteredHook has fn called whenever a control plane accepts this
// edge's registration: at start, after a reconnect and after a move to
// another control endpoint. It is called before the control contact hook
// of the same registration.
func (c *SimpleEdgeClient) SetRegisteredHook(fn func()) {
	if fn == nil {
		c.onRegistered.Store(nil)
		return
	}
	c.onRegistered.Store(&fn)
}

// controlContact notes contact with a control plane now.
func (c *SimpleEdgeClient) controlContact() {
	now := time.Now()
	c.lastContact.Store(now.UnixNano())
	if fn := c.onContact.Load(); fn != nil {
		(*fn)(now)
	}
}

// ConfigSource reports where the configuration the edge runs came from
// (ConfigSourceControl or ConfigSourceLastKnownGood) and when the edge last
// had contact with a control plane (zero if it never had).
func (c *SimpleEdgeClient) ConfigSource() (source string, lastContact time.Time) {
	source = ConfigSourceControl
	if c.lastKnownGood.Load() {
		source = ConfigSourceLastKnownGood
	}
	if ns := c.lastContact.Load(); ns != 0 {
		lastContact = time.Unix(0, ns)
	}
	return source, lastContact
}
