package plugintest

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	mgmtpb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
)

// Governed Metadata fakes for TestManagementServer.
//
// A plugin that owns portal objects (an asset catalogue, an MCP registry, …)
// calls SetObjectMetadata / GetObjectMetadata / DeleteObjectMetadata /
// GetResolvedMetadataSchema / ValidateObjectMetadata on its save, render and
// delete paths. These fakes let a plugin test drive that contract without a
// Studio: configure a schema per object type with SetMetadataSchema, decide
// whether it enforces, and assert the records the plugin wrote.
//
// Object types are matched as the plugin sends them; "plugin_resource:self:<slug>"
// is resolved to "plugin_resource:<plugin id>:<slug>" using the plugin ID in the
// request context, exactly like Studio. When MetadataEnterprise is false every
// RPC fails with FailedPrecondition, mimicking Community Edition.

// MetadataField is one governed field of a fake schema.
type MetadataField struct {
	Key            string   `json:"key"`
	Label          string   `json:"label"`
	Type           string   `json:"type"` // string | text | vocabulary | multi_vocabulary | user | date | number | boolean | string_list
	Required       bool     `json:"required"`
	Severity       string   `json:"severity,omitempty"` // error (default) | warning
	VocabularySlug string   `json:"vocabulary_slug,omitempty"`
	Terms          []string `json:"-"` // allowed values for vocabulary fields
	PortalVisible  bool     `json:"portal_visible"`
	GatewayVisible bool     `json:"gateway_visible"`
	Order          int      `json:"order"`
}

// MetadataSchema is the fake resolved schema for one object type.
type MetadataSchema struct {
	Fields      []MetadataField
	Enforcement string // "advisory" (default) | "enforce"
	SchemaSlugs []string
}

// MetadataRecord is a stored governed metadata record.
type MetadataRecord struct {
	ObjectType string
	ObjectID   string
	Values     map[string]interface{}
	Source     string // "plugin:<id>"
	UpdatedAt  time.Time
}

type metadataState struct {
	mu         sync.RWMutex
	enterprise bool
	pluginID   uint32 // resolves plugin_resource:self:<slug> when the request context carries no plugin ID
	schemas    map[string]*MetadataSchema
	records    map[string]*MetadataRecord
}

func (s *TestManagementServer) metadata() *metadataState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.metadataState == nil {
		s.metadataState = &metadataState{enterprise: true, schemas: map[string]*MetadataSchema{}, records: map[string]*MetadataRecord{}}
	}
	return s.metadataState
}

// SetMetadataEnterprise toggles the Enterprise gate. false makes every governed
// metadata RPC fail with FailedPrecondition (Community Edition behaviour).
func (s *TestManagementServer) SetMetadataEnterprise(enabled bool) {
	m := s.metadata()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enterprise = enabled
}

// SetMetadataPluginID sets the plugin ID used to resolve "plugin_resource:self:<slug>"
// when a request context carries none. Studio takes the ID from the
// authenticated connection, so plugin contexts usually do not carry it; the
// E2E harness sets this from the "plugin_id" configuration value.
func (s *TestManagementServer) SetMetadataPluginID(id uint32) {
	m := s.metadata()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pluginID = id
}

// SetMetadataSchema installs the resolved schema for an object type. Use
// "plugin_resource:self:<slug>" to target the plugin's own type regardless of
// its numeric ID.
func (s *TestManagementServer) SetMetadataSchema(objectType string, schema MetadataSchema) {
	m := s.metadata()
	m.mu.Lock()
	defer m.mu.Unlock()
	if schema.Enforcement == "" {
		schema.Enforcement = "advisory"
	}
	m.schemas[objectType] = &schema
}

// GetMetadataRecord returns the stored record for an object, if any.
func (s *TestManagementServer) GetMetadataRecord(objectType, objectID string) (*MetadataRecord, bool) {
	m := s.metadata()
	m.mu.RLock()
	defer m.mu.RUnlock()
	rec, ok := m.records[objectType+"|"+objectID]
	if !ok {
		return nil, false
	}
	cp := *rec
	cp.Values = map[string]interface{}{}
	for k, v := range rec.Values {
		cp.Values[k] = v
	}
	return &cp, true
}

// MetadataRecords returns every stored record, ordered by object type and ID.
func (s *TestManagementServer) MetadataRecords() []MetadataRecord {
	m := s.metadata()
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]MetadataRecord, 0, len(m.records))
	for _, rec := range m.records {
		out = append(out, *rec)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ObjectType != out[j].ObjectType {
			return out[i].ObjectType < out[j].ObjectType
		}
		return out[i].ObjectID < out[j].ObjectID
	})
	return out
}

func (s *TestManagementServer) recordMetadataCall(method string, req interface{}) {
	s.mu.Lock()
	s.calls = append(s.calls, ServiceCall{Method: method, Request: req, Timestamp: time.Now()})
	s.mu.Unlock()
}

// resolveObjectType applies Studio's "self" resolution and the Enterprise gate.
func (m *metadataState) resolveObjectType(pc *mgmtpb.PluginContext, objectType string) (string, error) {
	if !m.enterprise {
		return "", status.Error(codes.FailedPrecondition, "governed metadata requires an Enterprise licence")
	}
	objectType = strings.TrimSpace(objectType)
	if objectType == "" {
		return "", status.Error(codes.InvalidArgument, "object_type is required")
	}
	if strings.HasPrefix(objectType, "plugin_resource:self:") {
		id := m.pluginID
		if pc != nil && pc.PluginId != 0 {
			id = pc.PluginId
		}
		if id == 0 {
			return "", status.Error(codes.InvalidArgument, "plugin_resource:self:<slug> requires an authenticated plugin context")
		}
		objectType = fmt.Sprintf("plugin_resource:%d:%s", id, strings.TrimPrefix(objectType, "plugin_resource:self:"))
	}
	return objectType, nil
}

// schemaFor finds the schema for a resolved object type, also matching a
// schema registered under the "self" form for the same slug.
func (m *metadataState) schemaFor(objectType string) *MetadataSchema {
	if sch, ok := m.schemas[objectType]; ok {
		return sch
	}
	if strings.HasPrefix(objectType, "plugin_resource:") {
		parts := strings.SplitN(objectType, ":", 3)
		if len(parts) == 3 {
			if sch, ok := m.schemas["plugin_resource:self:"+parts[2]]; ok {
				return sch
			}
		}
	}
	return nil
}

type metadataIssue struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type metadataResult struct {
	Valid    bool            `json:"valid"`
	Enforced bool            `json:"enforced"`
	Errors   []metadataIssue `json:"errors"`
	Warnings []metadataIssue `json:"warnings"`
}

func (m *metadataState) validate(objectType string, values map[string]interface{}) metadataResult {
	res := metadataResult{Valid: true, Errors: []metadataIssue{}, Warnings: []metadataIssue{}}
	sch := m.schemaFor(objectType)
	if sch == nil {
		return res
	}
	res.Enforced = sch.Enforcement == "enforce"
	known := map[string]MetadataField{}
	for _, f := range sch.Fields {
		known[f.Key] = f
		v, present := values[f.Key]
		empty := !present || v == nil || v == ""
		label := f.Label
		if label == "" {
			label = f.Key
		}
		if empty {
			if f.Required {
				issue := metadataIssue{Field: f.Key, Code: "required", Message: label + " is required"}
				if f.Severity == "warning" {
					res.Warnings = append(res.Warnings, issue)
				} else {
					res.Errors = append(res.Errors, issue)
				}
			}
			continue
		}
		if len(f.Terms) > 0 {
			sv, _ := v.(string)
			ok := false
			for _, t := range f.Terms {
				if t == sv {
					ok = true
				}
			}
			if !ok {
				res.Errors = append(res.Errors, metadataIssue{Field: f.Key, Code: "invalid_enum", Message: fmt.Sprintf("%s must be one of %s", label, strings.Join(f.Terms, ", "))})
			}
		}
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if _, ok := known[k]; !ok {
			res.Errors = append(res.Errors, metadataIssue{Field: k, Code: "unknown_field", Message: k + " is not a governed field"})
		}
	}
	res.Valid = len(res.Errors) == 0
	return res
}

func (m *metadataState) displayList(objectType string, values map[string]interface{}, visibility string) []map[string]interface{} {
	sch := m.schemaFor(objectType)
	items := []map[string]interface{}{}
	if sch == nil {
		return items
	}
	fields := append([]MetadataField(nil), sch.Fields...)
	sort.SliceStable(fields, func(i, j int) bool { return fields[i].Order < fields[j].Order })
	for _, f := range fields {
		if visibility == "portal" && !f.PortalVisible {
			continue
		}
		if visibility == "gateway" && !f.GatewayVisible {
			continue
		}
		v, ok := values[f.Key]
		if !ok {
			continue
		}
		label := f.Label
		if label == "" {
			label = f.Key
		}
		typ := f.Type
		if typ == "" {
			typ = "string"
		}
		items = append(items, map[string]interface{}{"key": f.Key, "label": label, "type": typ, "value": v})
	}
	return items
}

func mustJSON(v interface{}) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

// GetObjectMetadata implements the GetObjectMetadata RPC.
func (s *TestManagementServer) GetObjectMetadata(ctx context.Context, req *mgmtpb.GetObjectMetadataRequest) (*mgmtpb.GetObjectMetadataResponse, error) {
	s.recordMetadataCall("GetObjectMetadata", req)
	m := s.metadata()
	m.mu.RLock()
	defer m.mu.RUnlock()
	objectType, err := m.resolveObjectType(req.Context, req.ObjectType)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.ObjectId) == "" {
		return nil, status.Error(codes.InvalidArgument, "object_id is required")
	}
	rec, ok := m.records[objectType+"|"+req.ObjectId]
	if !ok {
		return &mgmtpb.GetObjectMetadataResponse{Found: false, ValuesJson: "{}", ValidationStatus: "unvalidated"}, nil
	}
	visibility := req.Visibility
	if visibility == "" {
		visibility = "admin"
	}
	values := rec.Values
	if visibility != "admin" {
		values = map[string]interface{}{}
		for _, item := range m.displayList(objectType, rec.Values, visibility) {
			values[item["key"].(string)] = item["value"]
		}
	}
	res := m.validate(objectType, rec.Values)
	resp := &mgmtpb.GetObjectMetadataResponse{
		Found:                true,
		ValuesJson:           mustJSON(values),
		ValidationStatus:     metadataStatus(res),
		ValidationResultJson: mustJSON(res),
	}
	if visibility == "portal" {
		resp.DisplayJson = mustJSON(m.displayList(objectType, rec.Values, "portal"))
	}
	return resp, nil
}

func metadataStatus(res metadataResult) string {
	switch {
	case len(res.Errors) > 0:
		return "invalid"
	case len(res.Warnings) > 0:
		return "warnings"
	default:
		return "valid"
	}
}

// SetObjectMetadata implements the SetObjectMetadata RPC.
func (s *TestManagementServer) SetObjectMetadata(ctx context.Context, req *mgmtpb.SetObjectMetadataRequest) (*mgmtpb.SetObjectMetadataResponse, error) {
	s.recordMetadataCall("SetObjectMetadata", req)
	m := s.metadata()
	m.mu.Lock()
	defer m.mu.Unlock()
	objectType, err := m.resolveObjectType(req.Context, req.ObjectType)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.ObjectId) == "" {
		return nil, status.Error(codes.InvalidArgument, "object_id is required")
	}
	values := map[string]interface{}{}
	if strings.TrimSpace(req.ValuesJson) != "" {
		if err := json.Unmarshal([]byte(req.ValuesJson), &values); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "values_json must be a JSON object: %v", err)
		}
	}
	key := objectType + "|" + req.ObjectId
	merged := map[string]interface{}{}
	if req.Merge {
		if existing, ok := m.records[key]; ok {
			for k, v := range existing.Values {
				merged[k] = v
			}
		}
	}
	for k, v := range values {
		merged[k] = v
	}
	res := m.validate(objectType, merged)
	if res.Enforced && !res.Valid {
		return &mgmtpb.SetObjectMetadataResponse{Success: false, Message: "governed metadata failed validation", ValidationResultJson: mustJSON(res)}, nil
	}
	source := "system"
	switch {
	case req.Context != nil && req.Context.PluginId != 0:
		source = fmt.Sprintf("plugin:%d", req.Context.PluginId)
	case m.pluginID != 0:
		source = fmt.Sprintf("plugin:%d", m.pluginID)
	}
	m.records[key] = &MetadataRecord{ObjectType: objectType, ObjectID: req.ObjectId, Values: merged, Source: source, UpdatedAt: time.Now()}
	return &mgmtpb.SetObjectMetadataResponse{Success: true, ValuesJson: mustJSON(merged), ValidationResultJson: mustJSON(res)}, nil
}

// DeleteObjectMetadata implements the DeleteObjectMetadata RPC. Absent records succeed.
func (s *TestManagementServer) DeleteObjectMetadata(ctx context.Context, req *mgmtpb.DeleteObjectMetadataRequest) (*mgmtpb.DeleteObjectMetadataResponse, error) {
	s.recordMetadataCall("DeleteObjectMetadata", req)
	m := s.metadata()
	m.mu.Lock()
	defer m.mu.Unlock()
	objectType, err := m.resolveObjectType(req.Context, req.ObjectType)
	if err != nil {
		return nil, err
	}
	delete(m.records, objectType+"|"+req.ObjectId)
	return &mgmtpb.DeleteObjectMetadataResponse{Success: true}, nil
}

// GetResolvedMetadataSchema implements the GetResolvedMetadataSchema RPC.
func (s *TestManagementServer) GetResolvedMetadataSchema(ctx context.Context, req *mgmtpb.GetResolvedMetadataSchemaRequest) (*mgmtpb.GetResolvedMetadataSchemaResponse, error) {
	s.recordMetadataCall("GetResolvedMetadataSchema", req)
	m := s.metadata()
	m.mu.RLock()
	defer m.mu.RUnlock()
	objectType, err := m.resolveObjectType(req.Context, req.ObjectType)
	if err != nil {
		return nil, err
	}
	sch := m.schemaFor(objectType)
	if sch == nil {
		return &mgmtpb.GetResolvedMetadataSchemaResponse{FieldsJson: "[]", JsonSchema: `{"type":"object","properties":{}}`, Enforcement: "advisory", VocabulariesJson: "{}"}, nil
	}
	fields := make([]map[string]interface{}, 0, len(sch.Fields))
	vocab := map[string]interface{}{}
	props := map[string]interface{}{}
	required := []string{}
	for _, f := range sch.Fields {
		typ := f.Type
		if typ == "" {
			typ = "string"
		}
		field := map[string]interface{}{
			"key": f.Key, "label": f.Label, "type": typ, "required": f.Required,
			"severity": orDefault(f.Severity, "error"), "portal_visible": f.PortalVisible,
			"gateway_visible": f.GatewayVisible, "order": f.Order,
		}
		prop := map[string]interface{}{"type": "string"}
		if len(f.Terms) > 0 {
			slug := f.VocabularySlug
			if slug == "" {
				slug = f.Key
			}
			field["type"] = "vocabulary"
			field["vocabulary_slug"] = slug
			terms := make([]map[string]interface{}, 0, len(f.Terms))
			for _, t := range f.Terms {
				terms = append(terms, map[string]interface{}{"value": t, "label": termLabel(t)})
			}
			vocab[slug] = terms
			prop["enum"] = f.Terms
		}
		props[f.Key] = prop
		if f.Required && f.Severity != "warning" {
			required = append(required, f.Key)
		}
		fields = append(fields, field)
	}
	schema := map[string]interface{}{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		schema["required"] = required
	}
	return &mgmtpb.GetResolvedMetadataSchemaResponse{
		FieldsJson:       mustJSON(fields),
		JsonSchema:       mustJSON(schema),
		Enforcement:      sch.Enforcement,
		SchemaSlugs:      sch.SchemaSlugs,
		VocabulariesJson: mustJSON(vocab),
	}, nil
}

// termLabel renders a vocabulary value as a human label ("high_risk" → "High risk").
func termLabel(v string) string {
	v = strings.ReplaceAll(strings.TrimSpace(v), "_", " ")
	if v == "" {
		return v
	}
	return strings.ToUpper(v[:1]) + v[1:]
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// ValidateObjectMetadata implements the ValidateObjectMetadata RPC.
func (s *TestManagementServer) ValidateObjectMetadata(ctx context.Context, req *mgmtpb.ValidateObjectMetadataRequest) (*mgmtpb.ValidateObjectMetadataResponse, error) {
	s.recordMetadataCall("ValidateObjectMetadata", req)
	m := s.metadata()
	m.mu.RLock()
	defer m.mu.RUnlock()
	objectType, err := m.resolveObjectType(req.Context, req.ObjectType)
	if err != nil {
		return nil, err
	}
	values := map[string]interface{}{}
	if strings.TrimSpace(req.ValuesJson) != "" {
		if err := json.Unmarshal([]byte(req.ValuesJson), &values); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "values_json must be a JSON object: %v", err)
		}
	}
	res := m.validate(objectType, values)
	return &mgmtpb.ValidateObjectMetadataResponse{Valid: res.Valid, Enforced: res.Enforced, ResultJson: mustJSON(res)}, nil
}
