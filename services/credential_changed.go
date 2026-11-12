package services

import (
	"errors"
	"sync/atomic"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/credhash"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// credential.changed (eventbridge.CredentialChangedTopic): when an App
// credential is created, regenerated, activated, deactivated or deleted, or
// its App is activated or deactivated, Studio tells the edges at once, so
// they drop what they cached for the key instead of serving it until their
// cache TTL or their next credential sync (features/ActiveActive.md, Phase
// 4 item 4). The event is DirDown: the cluster relay carries it to every
// replica, and each replica to the edges it holds. It is an accelerator;
// validation and the sync stay what decides.

// edgeCredentialHasher makes the hash an edge holds for a credential:
// pkg/credhash under MICROGATEWAY_ENCRYPTION_KEY, the hash SyncCredentials
// sends and an edge's token cache is keyed by. Nil when the key is not set
// or too short: no event is raised then.
var edgeCredentialHasher atomic.Pointer[credhash.Hasher]

// SetEdgeCredentialHasher sets the hasher from MICROGATEWAY_ENCRYPTION_KEY
// (pkg/studio calls it at start). A key that cannot make hashes leaves
// credential.changed events off.
func SetEdgeCredentialHasher(encryptionKey string) {
	h, err := credhash.New(encryptionKey)
	if err != nil {
		edgeCredentialHasher.Store(nil)
		return
	}
	edgeCredentialHasher.Store(h)
}

// credentialChange describes cred for credential.changed, as it is now, or
// as it was for a key being replaced or deleted. ok is false when no event
// can be raised: no edge hasher, or a secret this replica cannot read (the
// edges' hash is of the secret).
func (s *Service) credentialChange(cred *models.Credential, deleted bool) (eventbridge.CredentialChanged, bool) {
	h := edgeCredentialHasher.Load()
	if h == nil || cred == nil || cred.Secret == "" || cred.Secret == secrets.RefusedCredential {
		return eventbridge.CredentialChanged{}, false
	}
	change := eventbridge.CredentialChanged{
		CredentialID: cred.ID,
		Hash:         h.Hash(cred.Secret),
		Active:       cred.Active && !deleted,
		Deleted:      deleted,
	}
	var app models.App
	switch err := s.DB.Select("id", "namespace", "is_active").Where("credential_id = ?", cred.ID).Take(&app).Error; {
	case err == nil:
		change.AppID, change.Namespace = app.ID, app.Namespace
		change.Active = change.Active && app.IsActive
	case !errors.Is(err, gorm.ErrRecordNotFound):
		logger.Log.Warn().Err(err).Uint("credential_id", cred.ID).Msg("credential.changed: reading the credential's App failed; the event names no App")
	}
	return change, true
}

// publishCredentialChanged raises credential.changed for each change.
func (s *Service) publishCredentialChanged(changes ...eventbridge.CredentialChanged) {
	if s.EventBus == nil {
		return
	}
	for _, c := range changes {
		if err := eventbridge.PublishDown(s.EventBus, "control", eventbridge.CredentialChangedTopic, c); err != nil {
			logger.Log.Warn().Err(err).Uint("credential_id", c.CredentialID).Msg("credential.changed: the event could not be raised; edges learn of the change at their next sync or cache expiry")
		}
	}
}

// credentialChanged raises credential.changed for cred as it is now.
func (s *Service) credentialChanged(cred *models.Credential) {
	if change, ok := s.credentialChange(cred, false); ok {
		s.publishCredentialChanged(change)
	}
}

// appCredentialChanged raises credential.changed for app's credential, as
// it is now (after the App or its credential changed).
func (s *Service) appCredentialChanged(app *models.App) {
	if app == nil || app.CredentialID == 0 {
		return
	}
	cred, err := s.GetCredentialByID(app.CredentialID)
	if err != nil {
		return
	}
	s.credentialChanged(cred)
}
