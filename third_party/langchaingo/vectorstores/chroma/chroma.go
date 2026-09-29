//go:build cgo

package chroma

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/embeddings"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/schema"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/vectorstores"
	chromav2 "github.com/amikos-tech/chroma-go/pkg/api/v2"
	chromaembeddings "github.com/amikos-tech/chroma-go/pkg/embeddings"
	"github.com/amikos-tech/chroma-go/pkg/embeddings/openai"
	"github.com/google/uuid"
)

var (
	ErrInvalidScoreThreshold    = errors.New("score threshold must be between 0 and 1")
	ErrUnexpectedResponseLength = errors.New("unexpected length of response")
	ErrNewClient                = errors.New("error creating collection")
	ErrAddDocument              = errors.New("error adding document")
	ErrRemoveCollection         = errors.New("error resetting collection")
	ErrUnsupportedOptions       = errors.New("unsupported options")
)

// Store is a wrapper around the chromaGo API and client.
type Store struct {
	client             chromav2.Client
	collection         chromav2.Collection
	distanceFunction   chromaembeddings.DistanceMetric
	chromaURL          string
	openaiAPIKey       string
	openaiOrganization string

	nameSpace    string
	nameSpaceKey string
	embedder     embeddings.Embedder
	includes     []chromav2.Include
}

var _ vectorstores.VectorStore = Store{}

// New creates an active client connection to the (specified, or default) collection in the Chroma server
// and returns the `Store` object needed by the other accessors.
func New(opts ...Option) (Store, error) {
	s, coErr := applyClientOptions(opts...)
	if coErr != nil {
		return s, coErr
	}

	// create the client connection and confirm that we can access the server with it
	chromaClient, err := chromav2.NewHTTPClient(chromav2.WithBaseURL(s.chromaURL))
	if err != nil {
		return s, err
	}

	if errHb := chromaClient.Heartbeat(context.Background()); errHb != nil {
		return s, errHb
	}
	s.client = chromaClient

	var embeddingFunction chromaembeddings.EmbeddingFunction
	if s.embedder != nil {
		// inject user's embedding function, if provided
		embeddingFunction = chromaGoEmbedder{Embedder: s.embedder}
	} else {
		// otherwise use standard langchaingo OpenAI embedding function
		var options []openai.Option
		if s.openaiOrganization != "" {
			options = append(options, openai.WithOpenAIOrganizationID(s.openaiOrganization))
		}
		embeddingFunction, err = openai.NewOpenAIEmbeddingFunction(s.openaiAPIKey, options...)
		if err != nil {
			return s, err
		}
	}

	// Build collection options - use GetOrCreateCollection which handles create-or-get logic
	createOpts := []chromav2.CreateCollectionOption{
		chromav2.WithEmbeddingFunctionCreate(embeddingFunction),
		chromav2.WithHNSWSpaceCreate(s.distanceFunction),
	}

	col, errCc := s.client.GetOrCreateCollection(context.Background(), s.nameSpace, createOpts...)
	if errCc != nil {
		return s, fmt.Errorf("%w: %w", ErrNewClient, errCc)
	}

	s.collection = col
	return s, nil
}

// AddDocuments adds the text and metadata from the documents to the Chroma collection associated with 'Store'.
// and returns the ids of the added documents.
func (s Store) AddDocuments(ctx context.Context,
	docs []schema.Document,
	options ...vectorstores.Option,
) ([]string, error) {
	opts := s.getOptions(options...)
	if opts.Embedder != nil || opts.ScoreThreshold != 0 || opts.Filters != nil {
		return nil, ErrUnsupportedOptions
	}

	nameSpace := s.getNameSpace(opts)
	if nameSpace != "" && s.nameSpaceKey == "" {
		return nil, fmt.Errorf("%w: nameSpace without nameSpaceKey", ErrUnsupportedOptions)
	}

	ids := make([]string, len(docs))
	texts := make([]string, len(docs))
	metadatas := make([]chromav2.DocumentMetadata, len(docs))
	for docIdx, doc := range docs {
		ids[docIdx] = uuid.New().String() // TODO (noodnik2): find & use something more meaningful
		texts[docIdx] = doc.PageContent
		mc := make(map[string]any, 0)
		maps.Copy(mc, doc.Metadata)
		if nameSpace != "" {
			mc[s.nameSpaceKey] = nameSpace
		}
		metadata, err := chromav2.NewDocumentMetadataFromMap(mc)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to create metadata: %w", ErrAddDocument, err)
		}
		metadatas[docIdx] = metadata
	}

	col := s.collection

	// Convert string IDs to DocumentID type
	docIDs := make([]chromav2.DocumentID, len(ids))
	for i, id := range ids {
		docIDs[i] = chromav2.DocumentID(id)
	}

	addOpts := []chromav2.CollectionAddOption{
		chromav2.WithIDs(docIDs...),
		chromav2.WithTexts(texts...),
		chromav2.WithMetadatas(metadatas...),
	}
	if addErr := col.Add(ctx, addOpts...); addErr != nil {
		return nil, fmt.Errorf("%w: %w", ErrAddDocument, addErr)
	}
	return ids, nil
}

func (s Store) SimilaritySearch(ctx context.Context, query string, numDocuments int,
	options ...vectorstores.Option,
) ([]schema.Document, error) {
	opts := s.getOptions(options...)

	if opts.Embedder != nil {
		// embedder is not used by this method, so shouldn't ever be specified
		return nil, fmt.Errorf("%w: Embedder", ErrUnsupportedOptions)
	}

	scoreThreshold, stErr := s.getScoreThreshold(opts)
	if stErr != nil {
		return nil, stErr
	}

	// Build query options for V2 API
	filter := s.getNamespacedFilter(opts)
	queryOpts := []chromav2.CollectionQueryOption{
		chromav2.WithQueryTexts(query),
		chromav2.WithNResults(numDocuments),
	}
	if filter != nil {
		// Convert map filter to Where clause - for now we'll pass it as-is and let Chroma handle it
		// TODO: Implement proper Where clause builder if needed
		queryOpts = append(queryOpts, chromav2.WithWhereQuery(convertMapToWhere(filter)))
	}

	qr, queryErr := s.collection.Query(ctx, queryOpts...)
	if queryErr != nil {
		return nil, queryErr
	}

	// V2 API returns groups - extract documents from query result
	docGroups := qr.GetDocumentsGroups()
	metadataGroups := qr.GetMetadatasGroups()
	distanceGroups := qr.GetDistancesGroups()

	if len(docGroups) != len(metadataGroups) || len(metadataGroups) != len(distanceGroups) {
		return nil, fmt.Errorf("%w: docGroups[%d], metadataGroups[%d], distanceGroups[%d]",
			ErrUnexpectedResponseLength, len(docGroups), len(metadataGroups), len(distanceGroups))
	}

	var sDocs []schema.Document
	for groupI := range docGroups {
		docs := docGroups[groupI]
		metadatas := metadataGroups[groupI]
		distances := distanceGroups[groupI]

		for docI := range docs {
			if docI < len(distances) {
				if score := 1.0 - float32(distances[docI]); score >= scoreThreshold {
					metadata := make(map[string]any)
					if docI < len(metadatas) {
						// Convert DocumentMetadata to map
						metadata = documentMetadataToMap(metadatas[docI])
					}
					sDocs = append(sDocs, schema.Document{
						Metadata:    metadata,
						PageContent: docs[docI].ContentString(),
						Score:       score,
					})
				}
			}
		}
	}

	return sDocs, nil
}

// documentMetadataToMap converts V2 DocumentMetadata to a plain map.
// We use type-specific getters instead of GetRaw because DocumentMetadataImpl.GetRaw
// returns a MetadataValue struct rather than the unwrapped primitive value.
func documentMetadataToMap(dm chromav2.DocumentMetadata) map[string]any {
	result := make(map[string]any)
	// Try to cast to impl to access Keys method
	if dmImpl, ok := dm.(*chromav2.DocumentMetadataImpl); ok {
		for _, key := range dmImpl.Keys() {
			// Use type-specific getters to extract actual values
			if str, ok := dmImpl.GetString(key); ok {
				result[key] = str
			} else if i, ok := dmImpl.GetInt(key); ok {
				result[key] = i
			} else if f, ok := dmImpl.GetFloat(key); ok {
				result[key] = f
			} else if b, ok := dmImpl.GetBool(key); ok {
				result[key] = b
			}
		}
	}
	return result
}

// convertMapToWhere converts a map filter to a WhereClause
// This builds Where clauses for simple filters with $eq, $in, $and operators
func convertMapToWhere(filter map[string]any) chromav2.WhereClause {
	// Check for $and operator
	if andClauses, ok := filter["$and"].([]map[string]any); ok {
		clauses := make([]chromav2.WhereClause, 0, len(andClauses))
		for _, clause := range andClauses {
			if wc := convertMapToWhere(clause); wc != nil {
				clauses = append(clauses, wc)
			}
		}
		if len(clauses) > 0 {
			return chromav2.And(clauses...)
		}
		return nil
	}

	// Build individual field filters
	var clauses []chromav2.WhereClause
	for field, value := range filter {
		if field == "$and" {
			continue
		}

		// Check if value is a filter operator map
		if opMap, ok := value.(map[string]any); ok {
			for op, opValue := range opMap {
				var clause chromav2.WhereClause
				switch op {
				case "$eq":
					clause = buildEqClause(field, opValue)
				case "$in":
					if inValues, ok := opValue.([]string); ok {
						clause = chromav2.InString(field, inValues...)
					}
				case "$gte":
					if floatVal, ok := opValue.(float64); ok {
						clause = chromav2.GteFloat(field, float32(floatVal))
					} else if intVal, ok := opValue.(int); ok {
						clause = chromav2.GteInt(field, intVal)
					}
				}
				if clause != nil {
					clauses = append(clauses, clause)
				}
			}
		} else {
			// Direct value - assume equality
			if clause := buildEqClause(field, value); clause != nil {
				clauses = append(clauses, clause)
			}
		}
	}

	if len(clauses) == 0 {
		return nil
	}
	if len(clauses) == 1 {
		return clauses[0]
	}
	return chromav2.And(clauses...)
}

// buildEqClause builds an equality clause based on value type
func buildEqClause(field string, value any) chromav2.WhereClause {
	switch v := value.(type) {
	case string:
		return chromav2.EqString(field, v)
	case int:
		return chromav2.EqInt(field, v)
	case int64:
		return chromav2.EqInt(field, int(v))
	case float32:
		return chromav2.EqFloat(field, v)
	case float64:
		return chromav2.EqFloat(field, float32(v))
	case bool:
		return chromav2.EqBool(field, v)
	default:
		// Fallback - return nil for unsupported types
		return nil
	}
}

func (s Store) RemoveCollection() error {
	if s.client == nil || s.collection == nil {
		return fmt.Errorf("%w: no collection", ErrRemoveCollection)
	}
	collectionName := s.collection.Name()
	errDc := s.client.DeleteCollection(context.Background(), collectionName)
	if errDc != nil {
		return fmt.Errorf("%w(%s): %w", ErrRemoveCollection, collectionName, errDc)
	}
	return nil
}

func (s Store) getOptions(options ...vectorstores.Option) vectorstores.Options {
	opts := vectorstores.Options{}
	for _, opt := range options {
		opt(&opts)
	}
	return opts
}

func (s Store) getScoreThreshold(opts vectorstores.Options) (float32, error) {
	if opts.ScoreThreshold < 0 || opts.ScoreThreshold > 1 {
		return 0, ErrInvalidScoreThreshold
	}
	return opts.ScoreThreshold, nil
}

func (s Store) getNameSpace(opts vectorstores.Options) string {
	if opts.NameSpace != "" {
		return opts.NameSpace
	}
	return s.nameSpace
}

func (s Store) getNamespacedFilter(opts vectorstores.Options) map[string]any {
	filter, _ := opts.Filters.(map[string]any)

	nameSpace := s.getNameSpace(opts)
	if nameSpace == "" || s.nameSpaceKey == "" {
		return filter
	}

	nameSpaceFilter := map[string]any{s.nameSpaceKey: nameSpace}
	if filter == nil {
		return nameSpaceFilter
	}

	return map[string]any{"$and": []map[string]any{nameSpaceFilter, filter}}
}

func safeIntToInt32(n int) int32 {
	return int32(max(0, n))
}
