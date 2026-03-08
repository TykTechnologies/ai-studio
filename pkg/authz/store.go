package authz

import (
	"context"
	_ "embed"
	"fmt"
	"strconv"

	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	"github.com/openfga/language/pkg/go/transformer"
	"github.com/openfga/openfga/pkg/server"
	"github.com/openfga/openfga/pkg/storage/memory"
	"github.com/rs/zerolog/log"
)

//go:embed model.fga
var modelDSL string

// Store wraps the embedded OpenFGA server and implements Authorizer.
type Store struct {
	server  *server.Server
	storeID string
	modelID string
}

// New creates a new embedded OpenFGA authorization store.
// It initializes an in-memory datastore, creates a store, and writes the authorization model.
func New(ctx context.Context) (*Store, error) {
	datastore := memory.New()

	srv, err := server.NewServerWithOpts(
		server.WithDatastore(datastore),
	)
	if err != nil {
		datastore.Close()
		return nil, fmt.Errorf("authz: failed to create openfga server: %w", err)
	}

	// Create a store.
	storeResp, err := srv.CreateStore(ctx, &openfgav1.CreateStoreRequest{
		Name: "ai-studio",
	})
	if err != nil {
		srv.Close()
		return nil, fmt.Errorf("authz: failed to create store: %w", err)
	}

	// Parse the FGA model DSL and write it.
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
		Msg("OpenFGA authorization store initialized")

	return &Store{
		server:  srv,
		storeID: storeResp.GetId(),
		modelID: modelResp.GetAuthorizationModelId(),
	}, nil
}

// Enabled returns true — the Store is always an active authorizer.
func (s *Store) Enabled() bool { return true }

// Check returns true if the user has the given relation on the object.
func (s *Store) Check(ctx context.Context, userID uint, relation string, objectType string, objectID uint) (bool, error) {
	return s.CheckStr(ctx, userID, relation, objectType, strconv.FormatUint(uint64(objectID), 10))
}

// CheckStr is like Check but accepts a string object ID.
func (s *Store) CheckStr(ctx context.Context, userID uint, relation string, objectType string, objectID string) (bool, error) {
	resp, err := s.server.Check(ctx, &openfgav1.CheckRequest{
		StoreId:              s.storeID,
		AuthorizationModelId: s.modelID,
		TupleKey: &openfgav1.CheckRequestTupleKey{
			User:     UserStr(userID),
			Relation: relation,
			Object:   objectType + ":" + objectID,
		},
	})
	if err != nil {
		return false, fmt.Errorf("authz: check failed: %w", err)
	}
	return resp.GetAllowed(), nil
}

// ListObjectsStr returns raw object strings where the user has the given relation.
// Use this for types with non-numeric IDs (e.g. plugin_resource with composite keys).
func (s *Store) ListObjectsStr(ctx context.Context, userID uint, relation string, objectType string) ([]string, error) {
	resp, err := s.server.ListObjects(ctx, &openfgav1.ListObjectsRequest{
		StoreId:              s.storeID,
		AuthorizationModelId: s.modelID,
		Type:                 objectType,
		Relation:             relation,
		User:                 UserStr(userID),
	})
	if err != nil {
		return nil, fmt.Errorf("authz: list objects failed: %w", err)
	}
	return resp.GetObjects(), nil
}

// ListObjects returns numeric object IDs where the user has the given relation.
// Returns an error if any object has a non-numeric ID — use ListObjectsStr for those types.
func (s *Store) ListObjects(ctx context.Context, userID uint, relation string, objectType string) ([]uint, error) {
	objects, err := s.ListObjectsStr(ctx, userID, relation, objectType)
	if err != nil {
		return nil, err
	}

	ids := make([]uint, 0, len(objects))
	for _, obj := range objects {
		id, err := ParseObjectID(obj)
		if err != nil {
			return nil, fmt.Errorf("authz: non-numeric object ID in ListObjects for type %q: %w (use ListObjectsStr instead)", objectType, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// WriteTuples writes relationship tuples to the store in batches.
func (s *Store) WriteTuples(ctx context.Context, writes []Tuple) error {
	return s.WriteTuplesAndDelete(ctx, writes, nil)
}

// DeleteTuples removes relationship tuples from the store in batches.
func (s *Store) DeleteTuples(ctx context.Context, deletes []Tuple) error {
	return s.WriteTuplesAndDelete(ctx, nil, deletes)
}

// WriteTuplesAndDelete atomically writes and deletes tuples.
// Batches are split to respect the OpenFGA per-call limit.
func (s *Store) WriteTuplesAndDelete(ctx context.Context, writes []Tuple, deletes []Tuple) error {
	// Process in batches. Each batch can have up to maxTuplesPerWrite total (writes + deletes).
	wi, di := 0, 0
	for wi < len(writes) || di < len(deletes) {
		req := &openfgav1.WriteRequest{
			StoreId:              s.storeID,
			AuthorizationModelId: s.modelID,
		}

		remaining := maxTuplesPerWrite

		// Add writes for this batch.
		if wi < len(writes) {
			end := wi + remaining
			if end > len(writes) {
				end = len(writes)
			}
			batch := writes[wi:end]
			tupleKeys := make([]*openfgav1.TupleKey, len(batch))
			for i, t := range batch {
				tupleKeys[i] = &openfgav1.TupleKey{
					User:     t.User,
					Relation: t.Relation,
					Object:   t.Object,
				}
			}
			req.Writes = &openfgav1.WriteRequestWrites{TupleKeys: tupleKeys}
			remaining -= len(batch)
			wi = end
		}

		// Add deletes for this batch.
		if di < len(deletes) && remaining > 0 {
			end := di + remaining
			if end > len(deletes) {
				end = len(deletes)
			}
			batch := deletes[di:end]
			tupleKeys := make([]*openfgav1.TupleKeyWithoutCondition, len(batch))
			for i, t := range batch {
				tupleKeys[i] = &openfgav1.TupleKeyWithoutCondition{
					User:     t.User,
					Relation: t.Relation,
					Object:   t.Object,
				}
			}
			req.Deletes = &openfgav1.WriteRequestDeletes{TupleKeys: tupleKeys}
			di = end
		}

		if _, err := s.server.Write(ctx, req); err != nil {
			return fmt.Errorf("authz: write failed: %w", err)
		}
	}
	return nil
}

// Close shuts down the embedded OpenFGA server.
func (s *Store) Close() {
	if s.server != nil {
		s.server.Close()
	}
}

// Verify Store implements Authorizer at compile time.
var _ Authorizer = (*Store)(nil)
