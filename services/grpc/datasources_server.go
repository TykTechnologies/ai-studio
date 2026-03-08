package grpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/v2/data_session"
	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/rs/zerolog/log"
	"github.com/tmc/langchaingo/schema"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

// DatasourcesServer implements the AIStudioManagementService for datasources management operations
type DatasourcesServer struct {
	pb.UnimplementedAIStudioManagementServiceServer
	service *services.Service
}

// NewDatasourcesServer creates a new datasources management gRPC server
func NewDatasourcesServer(service *services.Service) *DatasourcesServer {
	return &DatasourcesServer{
		service: service,
	}
}

// ListDatasources returns a list of datasources with filtering and pagination
func (s *DatasourcesServer) ListDatasources(ctx context.Context, req *pb.ListDatasourcesRequest) (*pb.ListDatasourcesResponse, error) {
	// Convert gRPC request parameters to service parameters
	page := int(req.GetPage())
	if page <= 0 {
		page = 1
	}
	limit := int(req.GetLimit())
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Handle is_active parameter
	var isActive *bool
	if req.IsActive != nil {
		value := req.GetIsActive()
		isActive = &value
	}

	// Handle user_id parameter
	var userID *uint
	if req.GetUserId() != "" {
		// Parse user_id string to uint
		if id, err := strconv.ParseUint(req.GetUserId(), 10, 32); err == nil {
			value := uint(id)
			userID = &value
		} else {
			log.Warn().Str("user_id", req.GetUserId()).Msg("Invalid user_id format in ListDatasources request")
		}
	}

	// Call enhanced service method with filtering
	datasources, totalCount, _, err := s.service.GetAllDatasourcesWithFilters(limit, page, false, isActive, userID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list datasources via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to list datasources: %v", err)
	}

	// Convert service response to gRPC protobuf
	pbDatasources := make([]*pb.DatasourceInfo, len(datasources))
	for i, datasource := range datasources {
		pbDatasources[i] = convertDatasourceToPB(&datasource)
	}

	log.Debug().
		Int("datasource_count", len(datasources)).
		Int64("total_count", totalCount).
		Interface("is_active", isActive).
		Interface("user_id", userID).
		Msg("Listed datasources with filtering via gRPC")

	return &pb.ListDatasourcesResponse{
		Datasources: pbDatasources,
		TotalCount:  totalCount,
	}, nil
}

// GetDatasource returns details for a specific datasource
func (s *DatasourcesServer) GetDatasource(ctx context.Context, req *pb.GetDatasourceRequest) (*pb.GetDatasourceResponse, error) {
	datasourceID := req.GetDatasourceId()
	if datasourceID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "datasource_id is required")
	}

	// Call existing service method
	datasource, err := s.service.GetDatasourceByID(ctx, uint(datasourceID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "datasource not found: %d", datasourceID)
		}
		log.Error().Err(err).Uint32("datasource_id", datasourceID).Msg("Failed to get datasource via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to get datasource: %v", err)
	}

	log.Debug().
		Uint32("datasource_id", datasourceID).
		Str("datasource_name", datasource.Name).
		Msg("Retrieved datasource via gRPC")

	return &pb.GetDatasourceResponse{
		Datasource: convertDatasourceToPB(datasource),
	}, nil
}

// CreateDatasource creates a new datasource
func (s *DatasourcesServer) CreateDatasource(ctx context.Context, req *pb.CreateDatasourceRequest) (*pb.CreateDatasourceResponse, error) {
	// Validate required fields
	if req.GetName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "name is required")
	}

	// Call existing service method
	datasource, err := s.service.CreateDatasource(
		req.GetName(),
		req.GetShortDescription(),
		req.GetLongDescription(),
		req.GetIcon(),
		req.GetUrl(),
		int(req.GetPrivacyScore()),
		uint(req.GetUserId()),
		req.GetTagNames(),
		req.GetDbConnString(),
		req.GetDbSourceType(),
		req.GetDbConnApiKey(),
		req.GetDbName(),
		req.GetEmbedVendor(),
		req.GetEmbedUrl(),
		req.GetEmbedApiKey(),
		req.GetEmbedModel(),
		req.GetActive(),
	)
	if err != nil {
		log.Error().Err(err).Str("name", req.GetName()).Msg("Failed to create datasource via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to create datasource: %v", err)
	}

	log.Info().
		Uint("datasource_id", datasource.ID).
		Str("datasource_name", datasource.Name).
		Msg("Created datasource via gRPC")

	return &pb.CreateDatasourceResponse{
		Datasource: convertDatasourceToPB(datasource),
	}, nil
}

// SearchDatasources searches for datasources by query
func (s *DatasourcesServer) SearchDatasources(ctx context.Context, req *pb.SearchDatasourcesRequest) (*pb.SearchDatasourcesResponse, error) {
	query := req.GetQuery()
	if query == "" {
		return nil, status.Errorf(codes.InvalidArgument, "search query is required")
	}

	// Call existing service method
	datasources, err := s.service.SearchDatasources(ctx, query)
	if err != nil {
		log.Error().Err(err).Str("query", query).Msg("Failed to search datasources via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to search datasources: %v", err)
	}

	// Convert service response to gRPC protobuf
	pbDatasources := make([]*pb.DatasourceInfo, len(datasources))
	for i, datasource := range datasources {
		pbDatasources[i] = convertDatasourceToPB(&datasource)
	}

	log.Debug().
		Str("query", query).
		Int("result_count", len(datasources)).
		Msg("Searched datasources via gRPC")

	return &pb.SearchDatasourcesResponse{
		Datasources: pbDatasources,
	}, nil
}

// UpdateDatasource updates an existing datasource
func (s *DatasourcesServer) UpdateDatasource(ctx context.Context, req *pb.UpdateDatasourceRequest) (*pb.UpdateDatasourceResponse, error) {
	datasourceID := req.GetDatasourceId()
	if datasourceID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "datasource_id is required")
	}

	// Validate required fields
	if req.GetName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "name is required")
	}

	// Call existing service method
	datasource, err := s.service.UpdateDatasource(
		uint(datasourceID),
		req.GetName(),
		req.GetShortDescription(),
		req.GetLongDescription(),
		req.GetIcon(),
		req.GetUrl(),
		int(req.GetPrivacyScore()),
		req.GetDbConnString(),
		req.GetDbSourceType(),
		req.GetDbConnApiKey(),
		req.GetDbName(),
		req.GetEmbedVendor(),
		req.GetEmbedUrl(),
		req.GetEmbedApiKey(),
		req.GetEmbedModel(),
		req.GetActive(),
		req.GetTagNames(),
		uint(req.GetUserId()),
	)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "datasource not found: %d", datasourceID)
		}
		log.Error().Err(err).
			Uint32("datasource_id", datasourceID).
			Str("name", req.GetName()).
			Msg("Failed to update datasource via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to update datasource: %v", err)
	}

	log.Info().
		Uint32("datasource_id", datasourceID).
		Str("datasource_name", datasource.Name).
		Msg("Updated datasource via gRPC")

	return &pb.UpdateDatasourceResponse{
		Datasource: convertDatasourceToPB(datasource),
	}, nil
}

// DeleteDatasource deletes a datasource
func (s *DatasourcesServer) DeleteDatasource(ctx context.Context, req *pb.DeleteDatasourceRequest) (*pb.DeleteDatasourceResponse, error) {
	datasourceID := req.GetDatasourceId()
	if datasourceID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "datasource_id is required")
	}

	// Call existing service method
	err := s.service.DeleteDatasource(uint(datasourceID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "datasource not found: %d", datasourceID)
		}
		log.Error().Err(err).
			Uint32("datasource_id", datasourceID).
			Msg("Failed to delete datasource via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to delete datasource: %v", err)
	}

	log.Info().
		Uint32("datasource_id", datasourceID).
		Msg("Deleted datasource via gRPC")

	return &pb.DeleteDatasourceResponse{
		Success: true,
		Message: "Datasource deleted successfully",
	}, nil
}

// CloneDatasource clones an existing datasource with all configuration
func (s *DatasourcesServer) CloneDatasource(ctx context.Context, req *pb.CloneDatasourceRequest) (*pb.CloneDatasourceResponse, error) {
	sourceDatasourceID := req.GetSourceDatasourceId()
	if sourceDatasourceID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "source_datasource_id is required")
	}

	// Call service layer to clone
	datasource, err := s.service.CloneDatasource(uint(sourceDatasourceID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "source datasource not found: %d", sourceDatasourceID)
		}
		log.Error().Err(err).
			Uint32("source_id", sourceDatasourceID).
			Msg("Failed to clone datasource via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to clone datasource: %v", err)
	}

	log.Info().
		Uint32("source_datasource_id", sourceDatasourceID).
		Uint("cloned_datasource_id", datasource.ID).
		Str("cloned_datasource_name", datasource.Name).
		Msg("Cloned datasource via gRPC")

	return &pb.CloneDatasourceResponse{
		Datasource: convertDatasourceToPB(datasource),
	}, nil
}

// ProcessDatasourceEmbeddings processes embeddings for a datasource
func (s *DatasourcesServer) ProcessDatasourceEmbeddings(ctx context.Context, req *pb.ProcessEmbeddingsRequest) (*pb.ProcessEmbeddingsResponse, error) {
	datasourceID := req.GetDatasourceId()
	if datasourceID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "datasource_id is required")
	}

	// Get datasource with files to verify it exists and has content
	datasource, err := s.service.GetDatasourceByID(ctx, uint(datasourceID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "datasource not found: %d", datasourceID)
		}
		return nil, status.Errorf(codes.Internal, "failed to get datasource: %v", err)
	}

	// Initialize sources map for DataSession
	sources := make(map[uint]*models.Datasource)
	sources[datasource.ID] = datasource

	// Create new DataSession
	ds := data_session.NewDataSession(sources)

	// Process embeddings in a goroutine (same pattern as REST API)
	go func() {
		err := ds.ProcessRAGForDatasource(uint(datasourceID), s.service.DB)
		if err != nil {
			log.Error().Err(err).
				Uint32("datasource_id", datasourceID).
				Msg("Error processing embeddings for datasource via gRPC")
			return
		}
		log.Info().
			Uint32("datasource_id", datasourceID).
			Msg("Successfully processed embeddings for datasource via gRPC")

		// Update LastProcessedOn for all files in the datasource
		for _, file := range datasource.Files {
			file.LastProcessedOn = time.Now()
			err = file.Update(s.service.DB)
			if err != nil {
				log.Error().Err(err).
					Uint("file_id", file.ID).
					Msg("Error updating LastProcessedOn for file")
			}
		}
	}()

	// Generate job ID for tracking
	jobID := fmt.Sprintf("embed-%d-%d", datasourceID, time.Now().Unix())

	log.Info().
		Uint32("datasource_id", datasourceID).
		Str("datasource_name", datasource.Name).
		Str("job_id", jobID).
		Msg("Started real embedding processing for datasource via gRPC")

	return &pb.ProcessEmbeddingsResponse{
		Success: true,
		Message: "Embedding processing started successfully",
		JobId:   jobID,
	}, nil
}

// convertDatasourceToPB converts a models.Datasource to protobuf DatasourceInfo
func convertDatasourceToPB(datasource *models.Datasource) *pb.DatasourceInfo {
	// Convert tags (no description field in Tag model)
	pbTags := make([]*pb.TagInfo, len(datasource.Tags))
	for i, tag := range datasource.Tags {
		pbTags[i] = &pb.TagInfo{
			Id:        uint32(tag.ID),
			Name:      tag.Name,
			CreatedAt: timestamppb.New(tag.CreatedAt),
			UpdatedAt: timestamppb.New(tag.UpdatedAt),
		}
	}

	// Convert metadata (JSONMap is already map[string]interface{})
	metadata := make(map[string]string)
	if datasource.Metadata != nil && len(datasource.Metadata) > 0 {
		// Convert interface{} values to strings for proto
		for k, v := range datasource.Metadata {
			if str, ok := v.(string); ok {
				metadata[k] = str
			} else {
				// Convert non-string values to JSON strings
				if jsonBytes, err := json.Marshal(v); err == nil {
					metadata[k] = string(jsonBytes)
				}
			}
		}
	}

	return &pb.DatasourceInfo{
		Id:               uint32(datasource.ID),
		Name:             datasource.Name,
		ShortDescription: datasource.ShortDescription,
		LongDescription:  datasource.LongDescription,
		Icon:             datasource.Icon,
		Url:              datasource.Url,
		PrivacyScore:     int32(datasource.PrivacyScore),
		UserId:           uint32(datasource.UserID),
		Tags:             pbTags,
		DbSourceType:     datasource.DBSourceType,
		DbName:           datasource.DBName,
		EmbedVendor:      string(datasource.EmbedVendor),
		EmbedModel:       datasource.EmbedModel,
		Active:           datasource.Active,
		HasDbConnApiKey:  datasource.DBConnAPIKey != "",
		HasEmbedApiKey:   datasource.EmbedAPIKey != "",
		CreatedAt:        timestamppb.New(datasource.CreatedAt),
		UpdatedAt:        timestamppb.New(datasource.UpdatedAt),
		Metadata:         metadata, // Plugin-stored data
	}
}

// === RAG/Embedding Operation Handlers ===

// GenerateEmbedding generates embeddings for text using the datasource's embedder configuration
func (s *DatasourcesServer) GenerateEmbedding(ctx context.Context, req *pb.GenerateEmbeddingRequest) (*pb.GenerateEmbeddingResponse, error) {
	// Validate datasource exists
	datasource, err := s.service.GetDatasourceByID(ctx, uint(req.DatasourceId))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "datasource not found: %v", err)
		}
		return nil, status.Errorf(codes.Internal, "failed to get datasource: %v", err)
	}

	// Check if datasource has embedder configured
	if datasource.EmbedVendor == "" || datasource.EmbedModel == "" {
		return &pb.GenerateEmbeddingResponse{
			Success:      false,
			ErrorMessage: "datasource does not have embedder configured",
		}, nil
	}

	// Create data session with this datasource
	datasources := map[uint]*models.Datasource{
		datasource.ID: datasource,
	}
	dataSession := data_session.NewDataSession(datasources)

	// Generate embeddings
	embeddings, err := dataSession.CreateEmbedding(datasource.ID, req.Texts)
	if err != nil {
		log.Error().
			Err(err).
			Str("embed_vendor", string(datasource.EmbedVendor)).
			Str("embed_model", datasource.EmbedModel).
			Str("embed_url", datasource.EmbedUrl).
			Msg("Failed to generate embeddings")
		return &pb.GenerateEmbeddingResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("failed to generate embeddings with %s/%s: %v", datasource.EmbedVendor, datasource.EmbedModel, err),
		}, nil
	}

	// Convert to protobuf format
	vectors := make([]*pb.EmbeddingVector, len(embeddings))
	for i, emb := range embeddings {
		vectors[i] = &pb.EmbeddingVector{
			Values: emb,
		}
	}

	return &pb.GenerateEmbeddingResponse{
		Success: true,
		Vectors: vectors,
	}, nil
}

// StoreDocuments stores pre-vectorized documents in the datasource's vector store
// This method uses pre-computed embeddings and bypasses the embedder to allow custom chunking
func (s *DatasourcesServer) StoreDocuments(ctx context.Context, req *pb.StoreDocumentsRequest) (*pb.StoreDocumentsResponse, error) {
	// Validate datasource exists
	datasource, err := s.service.GetDatasourceByID(ctx, uint(req.DatasourceId))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "datasource not found: %v", err)
		}
		return nil, status.Errorf(codes.Internal, "failed to get datasource: %v", err)
	}

	// Check if datasource has vector store configured
	if datasource.DBSourceType == "" || datasource.DBName == "" {
		return &pb.StoreDocumentsResponse{
			Success:      false,
			ErrorMessage: "datasource does not have vector store configured",
		}, nil
	}

	// Create data session with this datasource
	datasources := map[uint]*models.Datasource{
		datasource.ID: datasource,
	}
	dataSession := data_session.NewDataSession(datasources)

	// Extract data from proto documents
	contents := make([]string, len(req.Documents))
	vectors := make([][]float32, len(req.Documents))
	metadatas := make([]map[string]any, len(req.Documents))

	for i, doc := range req.Documents {
		contents[i] = doc.Content
		vectors[i] = doc.Embedding

		// Convert map[string]string to map[string]any
		metadata := make(map[string]any)
		for k, v := range doc.Metadata {
			metadata[k] = v
		}
		metadatas[i] = metadata
	}

	// Store documents with pre-computed vectors
	// This uses vendor-specific APIs to bypass the embedder
	err = dataSession.StoreDocumentsWithVectors(datasource.ID, contents, vectors, metadatas)
	if err != nil {
		log.Error().Err(err).Msg("Failed to store documents with pre-computed vectors")
		return &pb.StoreDocumentsResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &pb.StoreDocumentsResponse{
		Success:     true,
		StoredCount: int32(len(req.Documents)),
	}, nil
}

// ProcessAndStoreDocuments generates embeddings and stores documents in one step
func (s *DatasourcesServer) ProcessAndStoreDocuments(ctx context.Context, req *pb.ProcessAndStoreRequest) (*pb.ProcessAndStoreResponse, error) {
	// Validate datasource exists
	datasource, err := s.service.GetDatasourceByID(ctx, uint(req.DatasourceId))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "datasource not found: %v", err)
		}
		return nil, status.Errorf(codes.Internal, "failed to get datasource: %v", err)
	}

	// Check if datasource has both embedder and vector store configured
	if datasource.EmbedVendor == "" || datasource.EmbedModel == "" {
		return &pb.ProcessAndStoreResponse{
			Success:      false,
			ErrorMessage: "datasource does not have embedder configured",
		}, nil
	}
	if datasource.DBSourceType == "" || datasource.DBName == "" {
		return &pb.ProcessAndStoreResponse{
			Success:      false,
			ErrorMessage: "datasource does not have vector store configured",
		}, nil
	}

	// Create data session with this datasource
	datasources := map[uint]*models.Datasource{
		datasource.ID: datasource,
	}
	dataSession := data_session.NewDataSession(datasources)

	// Convert to documents (embeddings will be generated by StoreEmbedding)
	docs := make([]schema.Document, len(req.Chunks))
	for i, chunk := range req.Chunks {
		metadata := make(map[string]any)
		for k, v := range chunk.Metadata {
			metadata[k] = v
		}
		docs[i] = schema.Document{
			PageContent: chunk.Content,
			Metadata:    metadata,
			Score:       0,
		}
	}

	// Store documents (StoreEmbedding will automatically generate embeddings)
	err = dataSession.StoreEmbedding(datasource.ID, docs)
	if err != nil {
		log.Error().Err(err).Msg("Failed to store documents")
		return &pb.ProcessAndStoreResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &pb.ProcessAndStoreResponse{
		Success:        true,
		ProcessedCount: int32(len(docs)),
	}, nil
}

// QueryDatasourceByVector performs similarity search using a pre-computed embedding vector
func (s *DatasourcesServer) QueryDatasourceByVector(ctx context.Context, req *pb.QueryByVectorRequest) (*pb.QueryDatasourceResponse, error) {
	// Validate datasource exists
	datasource, err := s.service.GetDatasourceByID(ctx, uint(req.DatasourceId))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "datasource not found: %v", err)
		}
		return nil, status.Errorf(codes.Internal, "failed to get datasource: %v", err)
	}

	// Check if datasource has vector store configured
	if datasource.DBSourceType == "" || datasource.DBName == "" {
		return &pb.QueryDatasourceResponse{
			Success:      false,
			ErrorMessage: "datasource does not have vector store configured",
		}, nil
	}

	// Create data session with this datasource
	datasources := map[uint]*models.Datasource{
		datasource.ID: datasource,
	}
	dataSession := data_session.NewDataSession(datasources)

	// Perform vector similarity search
	maxResults := int(req.MaxResults)
	if maxResults <= 0 {
		maxResults = 10
	}

	docs, err := dataSession.SearchByVector(datasource.ID, req.Embedding, maxResults)
	if err != nil {
		log.Error().Err(err).Msg("Failed to search by vector")
		return &pb.QueryDatasourceResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	// Filter by similarity threshold if specified
	var filteredDocs []schema.Document
	if req.SimilarityThreshold > 0 {
		for _, doc := range docs {
			if doc.Score >= float32(req.SimilarityThreshold) {
				filteredDocs = append(filteredDocs, doc)
			}
		}
	} else {
		filteredDocs = docs
	}

	// Convert to protobuf results
	pbResults := make([]*pb.DatasourceResult, len(filteredDocs))
	for i, doc := range filteredDocs {
		// Convert metadata map[string]any to map[string]string
		metadata := make(map[string]string)
		for k, v := range doc.Metadata {
			if str, ok := v.(string); ok {
				metadata[k] = str
			} else {
				// Convert non-string values to JSON strings
				if jsonBytes, err := json.Marshal(v); err == nil {
					metadata[k] = string(jsonBytes)
				}
			}
		}

		pbResults[i] = &pb.DatasourceResult{
			Content:         doc.PageContent,
			SimilarityScore: float64(doc.Score),
			Metadata:        metadata,
		}
	}

	return &pb.QueryDatasourceResponse{
		Success: true,
		Results: pbResults,
	}, nil
}

// DeleteDocumentsByMetadata deletes documents by metadata filter
func (s *DatasourcesServer) DeleteDocumentsByMetadata(ctx context.Context, req *pb.DeleteDocumentsByMetadataRequest) (*pb.DeleteDocumentsByMetadataResponse, error) {
	datasourceID := req.GetDatasourceId()
	if datasourceID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "datasource_id is required")
	}

	if len(req.GetMetadataFilter()) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "metadata_filter cannot be empty")
	}

	// Get datasource
	datasource, err := s.service.GetDatasourceByID(ctx, uint(datasourceID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "datasource not found: %d", datasourceID)
		}
		return nil, status.Errorf(codes.Internal, "failed to get datasource: %v", err)
	}

	// Create DataSession
	sources := make(map[uint]*models.Datasource)
	sources[datasource.ID] = datasource
	ds := data_session.NewDataSession(sources)

	// Set filter mode (default to AND)
	filterMode := req.GetFilterMode()
	if filterMode == "" {
		filterMode = "AND"
	}

	// Delete documents
	count, err := ds.DeleteDocumentsByMetadata(
		uint(datasourceID),
		req.GetMetadataFilter(),
		filterMode,
		req.GetDryRun(),
	)
	if err != nil {
		log.Error().Err(err).
			Uint32("datasource_id", datasourceID).
			Interface("filter", req.GetMetadataFilter()).
			Msg("Failed to delete documents by metadata")
		return nil, status.Errorf(codes.Internal, "failed to delete documents: %v", err)
	}

	message := fmt.Sprintf("Deleted %d document(s)", count)
	if req.GetDryRun() {
		message = fmt.Sprintf("Would delete %d document(s) (dry run)", count)
	}

	log.Info().
		Uint32("datasource_id", datasourceID).
		Int("count", count).
		Bool("dry_run", req.GetDryRun()).
		Msg("Deleted documents by metadata")

	return &pb.DeleteDocumentsByMetadataResponse{
		Success:      true,
		DeletedCount: int32(count),
		Message:      message,
	}, nil
}

// QueryByMetadataOnly queries documents by metadata only
func (s *DatasourcesServer) QueryByMetadataOnly(ctx context.Context, req *pb.QueryByMetadataOnlyRequest) (*pb.QueryByMetadataOnlyResponse, error) {
	datasourceID := req.GetDatasourceId()
	if datasourceID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "datasource_id is required")
	}

	if len(req.GetMetadataFilter()) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "metadata_filter cannot be empty")
	}

	// Get datasource
	datasource, err := s.service.GetDatasourceByID(ctx, uint(datasourceID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "datasource not found: %d", datasourceID)
		}
		return nil, status.Errorf(codes.Internal, "failed to get datasource: %v", err)
	}

	// Create DataSession
	sources := make(map[uint]*models.Datasource)
	sources[datasource.ID] = datasource
	ds := data_session.NewDataSession(sources)

	// Set defaults
	limit := req.GetLimit()
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	offset := req.GetOffset()
	if offset < 0 {
		offset = 0
	}

	filterMode := req.GetFilterMode()
	if filterMode == "" {
		filterMode = "AND"
	}

	// Query documents
	docs, totalCount, err := ds.QueryByMetadataOnly(
		uint(datasourceID),
		req.GetMetadataFilter(),
		filterMode,
		int(limit),
		int(offset),
	)
	if err != nil {
		log.Error().Err(err).
			Uint32("datasource_id", datasourceID).
			Interface("filter", req.GetMetadataFilter()).
			Msg("Failed to query by metadata")
		return nil, status.Errorf(codes.Internal, "failed to query by metadata: %v", err)
	}

	// Convert to protobuf results (reuse existing conversion pattern)
	results := make([]*pb.DatasourceResult, len(docs))
	for i, doc := range docs {
		// Convert metadata map[string]any to map[string]string
		metadata := make(map[string]string)
		for k, v := range doc.Metadata {
			if str, ok := v.(string); ok {
				metadata[k] = str
			} else {
				// Convert non-string values to JSON strings
				if jsonBytes, err := json.Marshal(v); err == nil {
					metadata[k] = string(jsonBytes)
				}
			}
		}

		results[i] = &pb.DatasourceResult{
			Content:         doc.PageContent,
			SimilarityScore: 0.0, // N/A for metadata-only query
			Metadata:        metadata,
		}
	}

	log.Info().
		Uint32("datasource_id", datasourceID).
		Int("result_count", len(docs)).
		Int("total_count", totalCount).
		Msg("Queried documents by metadata")

	return &pb.QueryByMetadataOnlyResponse{
		Results:    results,
		TotalCount: int32(totalCount),
	}, nil
}

// ListNamespaces lists all namespaces in vector store
func (s *DatasourcesServer) ListNamespaces(ctx context.Context, req *pb.ListNamespacesRequest) (*pb.ListNamespacesResponse, error) {
	datasourceID := req.GetDatasourceId()
	if datasourceID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "datasource_id is required")
	}

	// Get datasource
	datasource, err := s.service.GetDatasourceByID(ctx, uint(datasourceID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "datasource not found: %d", datasourceID)
		}
		return nil, status.Errorf(codes.Internal, "failed to get datasource: %v", err)
	}

	// Create DataSession
	sources := make(map[uint]*models.Datasource)
	sources[datasource.ID] = datasource
	ds := data_session.NewDataSession(sources)

	// List namespaces
	namespaces, err := ds.ListNamespaces(uint(datasourceID))
	if err != nil {
		log.Error().Err(err).
			Uint32("datasource_id", datasourceID).
			Msg("Failed to list namespaces")
		return nil, status.Errorf(codes.Internal, "failed to list namespaces: %v", err)
	}

	// Convert to protobuf
	pbNamespaces := make([]*pb.NamespaceInfo, len(namespaces))
	for i, ns := range namespaces {
		pbNamespaces[i] = &pb.NamespaceInfo{
			Name:          ns.Name,
			DocumentCount: int32(ns.DocumentCount),
		}
	}

	log.Info().
		Uint32("datasource_id", datasourceID).
		Int("namespace_count", len(namespaces)).
		Msg("Listed namespaces")

	return &pb.ListNamespacesResponse{
		Namespaces: pbNamespaces,
	}, nil
}

// DeleteNamespace deletes an entire namespace
func (s *DatasourcesServer) DeleteNamespace(ctx context.Context, req *pb.DeleteNamespaceRequest) (*pb.DeleteNamespaceResponse, error) {
	datasourceID := req.GetDatasourceId()
	if datasourceID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "datasource_id is required")
	}

	if req.GetNamespace() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "namespace is required")
	}

	if !req.GetConfirm() {
		return nil, status.Errorf(codes.InvalidArgument, "confirm must be true to delete namespace (safety check)")
	}

	// Get datasource
	datasource, err := s.service.GetDatasourceByID(ctx, uint(datasourceID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "datasource not found: %d", datasourceID)
		}
		return nil, status.Errorf(codes.Internal, "failed to get datasource: %v", err)
	}

	// Create DataSession
	sources := make(map[uint]*models.Datasource)
	sources[datasource.ID] = datasource
	ds := data_session.NewDataSession(sources)

	// Delete namespace
	err = ds.DeleteNamespace(uint(datasourceID), req.GetNamespace())
	if err != nil {
		log.Error().Err(err).
			Uint32("datasource_id", datasourceID).
			Str("namespace", req.GetNamespace()).
			Msg("Failed to delete namespace")
		return nil, status.Errorf(codes.Internal, "failed to delete namespace: %v", err)
	}

	log.Warn().
		Uint32("datasource_id", datasourceID).
		Str("namespace", req.GetNamespace()).
		Msg("Deleted namespace")

	return &pb.DeleteNamespaceResponse{
		Success: true,
		Message: fmt.Sprintf("Namespace '%s' deleted successfully", req.GetNamespace()),
	}, nil
}