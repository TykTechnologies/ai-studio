// Package openfga provides the OpenFGA-backed implementation of authz.Authorizer.
package openfga

import (
	"context"
	_ "embed"
	"fmt"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/authz"
	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	"github.com/openfga/language/pkg/go/transformer"
	"github.com/openfga/openfga/pkg/server"
	"github.com/openfga/openfga/pkg/storage/memory"
	"github.com/rs/zerolog/log"
)

//go:embed model.fga
var modelDSL string

// maxBatchSize is the backend limit for relationships in a single write call.
const maxBatchSize = 100

// SubjectGroupMembers returns an OpenFGA userset identifier representing all
// members of a group. This uses the "#member" syntax specific to OpenFGA's
// relationship model and is needed for group-based sharing (e.g. granting
// all members of a group viewer access to an app).
func SubjectGroupMembers(id uint) string {
	return authz.SubjectGroup(id) + "#member"
}

// Store is the production Authorizer backed by an embedded relationship engine.
type Store struct {
	server  *server.Server
	storeID string
	modelID string
}

// New creates a new embedded authorization store.
// It initializes an in-memory datastore, creates a store, and writes the authorization model.
func New(ctx context.Context) (*Store, error) {
	datastore := memory.New()

	srv, err := server.NewServerWithOpts(
		server.WithDatastore(datastore),
	)
	if err != nil {
		datastore.Close()
		return nil, fmt.Errorf("authz: failed to create server: %w", err)
	}

	// Create a store.
	storeResp, err := srv.CreateStore(ctx, &openfgav1.CreateStoreRequest{
		Name: "ai-studio",
	})
	if err != nil {
		srv.Close()
		return nil, fmt.Errorf("authz: failed to create store: %w", err)
	}

	// Parse the authorization model DSL and write it.
	model, err := transformer.TransformDSLToProto(modelDSL)
	if err != nil {
		srv.Close()
		return nil, fmt.Errorf("authz: failed to parse model DSL: %w", err)
	}

	modelResp, err := srv.WriteAuthorizationModel(ctx, &openfgav1.WriteAuthorizationModelRequest{
		StoreId:         storeResp.GetId(),
		TypeDefinitions: model.GetTypeDefinitions(),
		SchemaVersion:   model.GetSchemaVersion(),
		Conditions:      model.GetConditions(),
	})
	if err != nil {
		srv.Close()
		return nil, fmt.Errorf("authz: failed to write authorization model: %w", err)
	}

	log.Info().
		Str("store_id", storeResp.GetId()).
		Str("model_id", modelResp.GetAuthorizationModelId()).
		Msg("authz: authorization store initialized")

	return &Store{
		server:  srv,
		storeID: storeResp.GetId(),
		modelID: modelResp.GetAuthorizationModelId(),
	}, nil
}

// Enabled returns true — the Store is always an active authorizer.
func (s *Store) Enabled() bool { return true }

// Check returns true if the user has the given relation on the resource.
func (s *Store) Check(ctx context.Context, userID uint, relation string, resourceType string, resourceID uint) (bool, error) {
	return s.CheckByName(ctx, userID, relation, resourceType, strconv.FormatUint(uint64(resourceID), 10))
}

// CheckByName is like Check but accepts a string resource ID.
func (s *Store) CheckByName(ctx context.Context, userID uint, relation string, resourceType string, resourceID string) (bool, error) {
	resp, err := s.server.Check(ctx, &openfgav1.CheckRequest{
		StoreId:              s.storeID,
		AuthorizationModelId: s.modelID,
		TupleKey: &openfgav1.CheckRequestTupleKey{
			User:     authz.SubjectUser(userID),
			Relation: relation,
			Object:   resourceType + ":" + resourceID,
		},
	})
	if err != nil {
		return false, fmt.Errorf("authz: check failed: %w", err)
	}
	return resp.GetAllowed(), nil
}

// ListResourcesByName returns raw resource strings where the user has the given relation.
// Results are bounded by the server's configured max (default 1000).
// Use this for types with non-numeric IDs (e.g. plugin_resource with composite keys).
func (s *Store) ListResourcesByName(ctx context.Context, userID uint, relation string, resourceType string) ([]string, error) {
	resp, err := s.server.ListObjects(ctx, &openfgav1.ListObjectsRequest{
		StoreId:              s.storeID,
		AuthorizationModelId: s.modelID,
		Type:                 resourceType,
		Relation:             relation,
		User:                 authz.SubjectUser(userID),
	})
	if err != nil {
		return nil, fmt.Errorf("authz: list resources failed: %w", err)
	}
	return resp.GetObjects(), nil
}

// ListResources returns numeric resource IDs where the user has the given relation.
// Results are bounded by the server's configured max (default 1000).
// Returns an error if any resource has a non-numeric ID — use ListResourcesByName for those types.
func (s *Store) ListResources(ctx context.Context, userID uint, relation string, resourceType string) ([]uint, error) {
	resources, err := s.ListResourcesByName(ctx, userID, relation, resourceType)
	if err != nil {
		return nil, err
	}
	return parseNumericIDs(resourceType, resources)
}

func parseNumericIDs(resourceType string, resources []string) ([]uint, error) {
	ids := make([]uint, 0, len(resources))
	for _, res := range resources {
		id, err := authz.ParseResourceNumericID(res)
		if err != nil {
			return nil, fmt.Errorf("authz: non-numeric resource ID in ListResources for type %q: %w (use ListResourcesByName instead)", resourceType, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// Grant writes relationship grants to the authorization store.
func (s *Store) Grant(ctx context.Context, grants []authz.Relationship) error {
	return s.GrantAndRevoke(ctx, grants, nil)
}

// Revoke removes relationship grants from the authorization store.
func (s *Store) Revoke(ctx context.Context, revocations []authz.Relationship) error {
	return s.GrantAndRevoke(ctx, nil, revocations)
}

// GrantAndRevoke atomically writes and removes relationships.
// Batches are split to respect the per-call limit.
func (s *Store) GrantAndRevoke(ctx context.Context, grants []authz.Relationship, revocations []authz.Relationship) error {
	gi, ri := 0, 0
	for gi < len(grants) || ri < len(revocations) {
		req := &openfgav1.WriteRequest{
			StoreId:              s.storeID,
			AuthorizationModelId: s.modelID,
		}

		remaining := maxBatchSize

		// Add grants for this batch.
		if gi < len(grants) {
			end := gi + remaining
			if end > len(grants) {
				end = len(grants)
			}
			batch := grants[gi:end]
			writes := make([]*openfgav1.TupleKey, len(batch))
			for i, rel := range batch {
				writes[i] = &openfgav1.TupleKey{
					User:     rel.Subject,
					Relation: rel.Relation,
					Object:   rel.Resource,
				}
			}
			req.Writes = &openfgav1.WriteRequestWrites{TupleKeys: writes}
			remaining -= len(batch)
			gi = end
		}

		// Add revocations for this batch.
		if ri < len(revocations) && remaining > 0 {
			end := ri + remaining
			if end > len(revocations) {
				end = len(revocations)
			}
			batch := revocations[ri:end]
			deletes := make([]*openfgav1.TupleKeyWithoutCondition, len(batch))
			for i, rel := range batch {
				deletes[i] = &openfgav1.TupleKeyWithoutCondition{
					User:     rel.Subject,
					Relation: rel.Relation,
					Object:   rel.Resource,
				}
			}
			req.Deletes = &openfgav1.WriteRequestDeletes{TupleKeys: deletes}
			ri = end
		}

		if _, err := s.server.Write(ctx, req); err != nil {
			return fmt.Errorf("authz: write failed: %w", err)
		}
	}
	return nil
}

// Close shuts down the embedded authorization server.
func (s *Store) Close() {
	if s.server != nil {
		s.server.Close()
	}
}

// Verify Store implements Authorizer at compile time.
var _ authz.Authorizer = (*Store)(nil)
