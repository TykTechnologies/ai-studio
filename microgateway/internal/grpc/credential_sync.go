package grpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

// The edge's side of the SyncCredentials RPC (full credential sync,
// Enterprise): SimpleEdgeClient is the services.CredentialSyncSource the
// synced credential list syncs from, on the connection the edge holds to
// its current control plane.

var (
	_ services.CredentialSyncSource    = (*SimpleEdgeClient)(nil)
	_ services.CredentialSyncRegistrar = (*SimpleEdgeClient)(nil)
)

// SyncCredentialsOnContact has list follow this client: a registration a
// control plane accepts (at start, after a reconnect, after a move to
// another control endpoint) forgets that a control plane did not offer the
// sync, and every contact (the registration and each acknowledged
// heartbeat) asks for a sync. It adds to the client's hooks, so the
// contact recording of RecordControlContactsIn keeps working.
func (c *SimpleEdgeClient) SyncCredentialsOnContact(list *services.CredentialSync) {
	c.AddRegisteredHook(list.Reset)
	c.AddControlContactHook(func(time.Time) { list.Kick() })
}

// SyncCredentials streams the credentials changed since the watermark (0:
// all of them) from the control plane and hands each page to onPage, in
// order. What the control plane answers is mapped for the sync:
//
//   - Unimplemented (an older or a Community control plane):
//     services.ErrCredentialSyncUnavailable, so the edge stays on demand;
//   - FailedPrecondition "register first" (the control plane does not know
//     this edge): services.ErrCredentialSyncNotRegistered, so the edge
//     registers again and asks again;
//   - FailedPrecondition about the hash version:
//     services.ErrCredentialHashVersion;
//   - anything else, or a stream that ends without its last page: an error
//     the sync logs and retries on the next heartbeat.
func (c *SimpleEdgeClient) SyncCredentials(ctx context.Context, since uint64, onPage func(*services.CredentialSyncPage) error) error {
	client := c.GetGRPCClient()
	if client == nil {
		return errors.New("credential sync: not connected to a control plane")
	}
	stream, err := client.SyncCredentials(c.createAuthContext(ctx), &pb.CredentialSyncRequest{
		EdgeId:        c.config.HubSpoke.EdgeID,
		EdgeNamespace: c.config.HubSpoke.EdgeNamespace,
		Since:         since,
		HashVersion:   services.CredentialHashVersion(),
	})
	if err != nil {
		return credentialSyncError(err)
	}
	for {
		page, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			// The sync checks that the last page arrived.
			return nil
		}
		if err != nil {
			return credentialSyncError(err)
		}
		if err := onPage(credentialSyncPage(page)); err != nil {
			return err
		}
		if page.GetLast() {
			return nil
		}
	}
}

// Reregister registers the edge again on its current connection, as on a
// reconnect: the control plane answered that it does not know the edge.
func (c *SimpleEdgeClient) Reregister() error {
	return c.registerWithControl()
}

// credentialSyncError maps the control plane's status for the sync.
func credentialSyncError(err error) error {
	st, ok := status.FromError(err)
	if !ok {
		return fmt.Errorf("credential sync: %w", err)
	}
	switch {
	case st.Code() == codes.Unimplemented:
		return fmt.Errorf("%w (%s)", services.ErrCredentialSyncUnavailable, st.Message())
	case st.Code() == codes.FailedPrecondition && strings.Contains(st.Message(), "register first"):
		return fmt.Errorf("%w (%s)", services.ErrCredentialSyncNotRegistered, st.Message())
	case st.Code() == codes.FailedPrecondition && strings.Contains(st.Message(), "hash version"):
		return fmt.Errorf("%w (%s)", services.ErrCredentialHashVersion, st.Message())
	}
	return fmt.Errorf("credential sync: %s: %s", st.Code(), st.Message())
}

// credentialSyncPage converts a page from the wire.
func credentialSyncPage(p *pb.CredentialSyncPage) *services.CredentialSyncPage {
	page := &services.CredentialSyncPage{
		Apps:        p.GetApps(),
		Full:        p.GetFull(),
		Last:        p.GetLast(),
		Watermark:   p.GetWatermark(),
		HashVersion: p.GetHashVersion(),
	}
	page.Credentials = make([]services.SyncedCredentialUpdate, 0, len(p.GetCredentials()))
	for _, c := range p.GetCredentials() {
		u := services.SyncedCredentialUpdate{
			CredentialID: uint(c.GetCredentialId()),
			SecretHash:   c.GetSecretHash(),
			AppID:        uint(c.GetAppId()),
			Active:       c.GetActive(),
			Deleted:      c.GetDeleted(),
		}
		if ts := c.GetUpdatedAt(); ts != nil {
			u.UpdatedAt = ts.AsTime()
		}
		page.Credentials = append(page.Credentials, u)
	}
	return page
}
