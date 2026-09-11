package grpc

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/governed_metadata"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// GovernedMetadataServer exposes governed metadata (Enterprise) to plugins over
// the management API. Scope enforcement happens in the auth interceptor
// (metadata.read / metadata.write); this server only maps calls and errors.
type GovernedMetadataServer struct {
	service *services.Service
}

// NewGovernedMetadataServer creates the governed metadata management server.
func NewGovernedMetadataServer(service *services.Service) *GovernedMetadataServer {
	return &GovernedMetadataServer{service: service}
}

func (s *GovernedMetadataServer) svc() governed_metadata.Service {
	return s.service.GovernedMetadata()
}

// pluginSource attributes writes to the calling plugin when it is known.
func pluginSource(ctx context.Context) string {
	if plugin, ok := GetPluginFromContext(ctx); ok && plugin != nil {
		return models.MetadataSourcePlugin(plugin.ID)
	}
	return models.MetadataSourceSystem
}

// resolveObjectType expands "plugin_resource:self:<slug>" to the calling
// plugin's resource type so plugins never need to know their numeric ID.
func resolveObjectType(ctx context.Context, objectType string) (string, error) {
	if objectType == "" {
		return "", status.Error(codes.InvalidArgument, "object_type is required")
	}
	if !models.IsSelfObjectType(objectType) {
		return objectType, nil
	}
	plugin, ok := GetPluginFromContext(ctx)
	if !ok || plugin == nil {
		return "", status.Error(codes.InvalidArgument, "plugin_resource:self:<slug> requires an authenticated plugin context")
	}
	return models.ResolveSelfObjectType(objectType, plugin.ID), nil
}

// visibilityFromRequest maps the wire value to the service enum.
func visibilityFromRequest(v string) (governed_metadata.Visibility, error) {
	switch v {
	case "", "admin":
		return governed_metadata.VisibilityAdmin, nil
	case "portal":
		return governed_metadata.VisibilityPortal, nil
	case "gateway":
		return governed_metadata.VisibilityGateway, nil
	default:
		return governed_metadata.VisibilityAdmin, status.Errorf(codes.InvalidArgument, "visibility must be admin, portal or gateway (got %q)", v)
	}
}

// mapGovernedMetadataError converts service errors into gRPC status errors.
func mapGovernedMetadataError(err error) error {
	var def *governed_metadata.SchemaDefinitionError
	var hrej *governed_metadata.HookRejectedError
	switch {
	case err == nil:
		return nil
	case errors.Is(err, governed_metadata.ErrEnterpriseFeature):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, governed_metadata.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, governed_metadata.ErrInvalidObjectType):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.As(err, &def):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.As(err, &hrej):
		return status.Error(codes.PermissionDenied, err.Error())
	default:
		return status.Errorf(codes.Internal, "governed metadata: %v", err)
	}
}

func marshalJSON(v interface{}) string {
	if v == nil {
		return ""
	}
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(data)
}

// GetObjectMetadata returns the stored governed metadata for an object.
// visibility="portal"|"gateway" narrows values_json to the fields flagged for
// that audience; "portal" also fills display_json (labels resolved) so a
// plugin's end-user page can render the values exactly as the built-in
// portal does.
func (s *GovernedMetadataServer) GetObjectMetadata(ctx context.Context, req *pb.GetObjectMetadataRequest) (*pb.GetObjectMetadataResponse, error) {
	if req.GetObjectType() == "" || req.GetObjectId() == "" {
		return nil, status.Error(codes.InvalidArgument, "object_type and object_id are required")
	}
	objectType, err := resolveObjectType(ctx, req.GetObjectType())
	if err != nil {
		return nil, err
	}
	vis, err := visibilityFromRequest(req.GetVisibility())
	if err != nil {
		return nil, err
	}
	rec, err := s.svc().GetObjectMetadata(objectType, req.GetObjectId())
	if errors.Is(err, governed_metadata.ErrNotFound) {
		return &pb.GetObjectMetadataResponse{Found: false, ValuesJson: "{}"}, nil
	}
	if err != nil {
		return nil, mapGovernedMetadataError(err)
	}
	values := rec.Values
	if vis != governed_metadata.VisibilityAdmin {
		values = s.svc().VisibleValues(objectType, rec, vis)
		if values == nil {
			values = map[string]interface{}{}
		}
	}
	resp := &pb.GetObjectMetadataResponse{
		Found:                true,
		ValuesJson:           marshalJSON(values),
		ValidationStatus:     rec.ValidationStatus,
		ValidationResultJson: marshalJSON(rec.ValidationResult),
	}
	if vis == governed_metadata.VisibilityPortal {
		display := s.svc().DisplayValues(objectType, rec)
		if display == nil {
			display = []governed_metadata.DisplayField{}
		}
		resp.DisplayJson = marshalJSON(display)
	}
	if !rec.UpdatedAt.IsZero() {
		resp.UpdatedAt = timestamppb.New(rec.UpdatedAt)
	}
	return resp, nil
}

// DeleteObjectMetadata removes the governed metadata of an object, e.g. when a
// plugin deletes the resource instance it belongs to. Absent records succeed.
func (s *GovernedMetadataServer) DeleteObjectMetadata(ctx context.Context, req *pb.DeleteObjectMetadataRequest) (*pb.DeleteObjectMetadataResponse, error) {
	if req.GetObjectType() == "" || req.GetObjectId() == "" {
		return nil, status.Error(codes.InvalidArgument, "object_type and object_id are required")
	}
	objectType, err := resolveObjectType(ctx, req.GetObjectType())
	if err != nil {
		return nil, err
	}
	err = s.svc().DeleteObjectMetadata(ctx, objectType, req.GetObjectId(), governed_metadata.SetOptions{Source: pluginSource(ctx)})
	if err != nil {
		log.Warn().Err(err).Str("object_type", objectType).Str("object_id", req.GetObjectId()).Msg("DeleteObjectMetadata via gRPC failed")
		return nil, mapGovernedMetadataError(err)
	}
	return &pb.DeleteObjectMetadataResponse{Success: true, Message: "governed metadata deleted"}, nil
}

// SetObjectMetadata validates and stores governed metadata on behalf of a plugin.
// Validation failures under an enforcing schema are returned in-band
// (success=false + validation_result_json) so plugins get the structured result.
func (s *GovernedMetadataServer) SetObjectMetadata(ctx context.Context, req *pb.SetObjectMetadataRequest) (*pb.SetObjectMetadataResponse, error) {
	if req.GetObjectType() == "" || req.GetObjectId() == "" {
		return nil, status.Error(codes.InvalidArgument, "object_type and object_id are required")
	}
	objectType, err := resolveObjectType(ctx, req.GetObjectType())
	if err != nil {
		return nil, err
	}
	values := map[string]interface{}{}
	if req.GetValuesJson() != "" {
		if err := json.Unmarshal([]byte(req.GetValuesJson()), &values); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "values_json must be a JSON object: %v", err)
		}
	}
	// The management PluginContext carries no user; writes are attributed to the plugin source.
	rec, result, err := s.svc().SetObjectMetadata(ctx, objectType, req.GetObjectId(), values,
		governed_metadata.SetOptions{Merge: req.GetMerge(), Source: pluginSource(ctx)})
	var verr *governed_metadata.ValidationError
	if errors.As(err, &verr) {
		return &pb.SetObjectMetadataResponse{
			Success:              false,
			Message:              err.Error(),
			ValidationResultJson: marshalJSON(verr.Result),
		}, nil
	}
	if err != nil {
		log.Warn().Err(err).Str("object_type", req.GetObjectType()).Str("object_id", req.GetObjectId()).Msg("SetObjectMetadata via gRPC failed")
		return nil, mapGovernedMetadataError(err)
	}
	return &pb.SetObjectMetadataResponse{
		Success:              true,
		Message:              "governed metadata updated",
		ValuesJson:           marshalJSON(rec.Values),
		ValidationResultJson: marshalJSON(result),
	}, nil
}

// GetResolvedMetadataSchema returns the merged schema for an object type.
func (s *GovernedMetadataServer) GetResolvedMetadataSchema(ctx context.Context, req *pb.GetResolvedMetadataSchemaRequest) (*pb.GetResolvedMetadataSchemaResponse, error) {
	objectType, err := resolveObjectType(ctx, req.GetObjectType())
	if err != nil {
		return nil, err
	}
	resolved, err := s.svc().ResolveSchema(objectType)
	if err != nil {
		return nil, mapGovernedMetadataError(err)
	}
	vocabularies := resolved.Vocabularies
	if vocabularies == nil {
		vocabularies = map[string][]models.VocabularyTerm{}
	}
	return &pb.GetResolvedMetadataSchemaResponse{
		FieldsJson:       marshalJSON(resolved.Fields),
		JsonSchema:       marshalJSON(resolved.JSONSchema),
		Enforcement:      resolved.Enforcement,
		SchemaSlugs:      resolved.SchemaSlugs,
		VocabulariesJson: marshalJSON(vocabularies),
	}, nil
}

// ValidateObjectMetadata validates values without storing them.
func (s *GovernedMetadataServer) ValidateObjectMetadata(ctx context.Context, req *pb.ValidateObjectMetadataRequest) (*pb.ValidateObjectMetadataResponse, error) {
	objectType, err := resolveObjectType(ctx, req.GetObjectType())
	if err != nil {
		return nil, err
	}
	values := map[string]interface{}{}
	if req.GetValuesJson() != "" {
		if err := json.Unmarshal([]byte(req.GetValuesJson()), &values); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "values_json must be a JSON object: %v", err)
		}
	}
	var result *governed_metadata.ValidationResult
	if req.GetPublishing() {
		result, err = s.svc().ValidateForPublish(ctx, objectType, req.GetObjectId(), values)
	} else {
		result, err = s.svc().Validate(objectType, values)
	}
	if err != nil {
		return nil, mapGovernedMetadataError(err)
	}
	return &pb.ValidateObjectMetadataResponse{
		Valid:      result.Valid,
		Enforced:   result.Enforced,
		ResultJson: marshalJSON(result),
	}, nil
}
