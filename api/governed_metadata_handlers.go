package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/governed_metadata"
	"github.com/gin-gonic/gin"
)

// governedMetadata returns the governed metadata service (CE stub when not wired).
func (a *API) governedMetadata() governed_metadata.Service {
	return a.service.GovernedMetadata()
}

// governedMetadataPointer builds the JSON pointer used in 422 responses.
func governedMetadataPointer(field string) string {
	if field == "" {
		return "/data/attributes/governed_metadata"
	}
	return "/data/attributes/governed_metadata/" + field
}

// MetadataSchemaInput is the JSON:API body for schema create/update.
type MetadataSchemaInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Name        string                    `json:"name"`
			Slug        string                    `json:"slug"`
			Description string                    `json:"description"`
			AppliesTo   []string                  `json:"applies_to"`
			Fields      []models.MetadataFieldDef `json:"fields"`
			Enforcement string                    `json:"enforcement"`
			Active      *bool                     `json:"active"`
			Order       *int                      `json:"order"`
		} `json:"attributes"`
	} `json:"data"`
}

// MetadataVocabularyInput is the JSON:API body for vocabulary create/update.
type MetadataVocabularyInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Name        string                  `json:"name"`
			Slug        string                  `json:"slug"`
			Description string                  `json:"description"`
			Terms       []models.VocabularyTerm `json:"terms"`
		} `json:"attributes"`
	} `json:"data"`
}

// ObjectMetadataInput is the body for PUT /metadata/objects/:type/:id.
type ObjectMetadataInput struct {
	Values map[string]interface{} `json:"values"`
	Merge  bool                   `json:"merge"`
}

// ValidateMetadataInput is the body for POST /metadata/validate.
type ValidateMetadataInput struct {
	ObjectType string                 `json:"object_type"`
	Values     map[string]interface{} `json:"values"`
}

func simpleError(c *gin.Context, status int, title, detail string) {
	c.JSON(status, models.ErrorResponse{Errors: []struct {
		Title  string `json:"title"`
		Detail string `json:"detail"`
	}{{Title: title, Detail: detail}}})
}

// writeGovernedMetadataError maps service errors to HTTP responses.
// Returns true when a response was written.
func writeGovernedMetadataError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	var verr *governed_metadata.ValidationError
	var hrej *governed_metadata.HookRejectedError
	var coll *governed_metadata.SchemaKeyCollisionError
	var def *governed_metadata.SchemaDefinitionError
	switch {
	case errors.Is(err, governed_metadata.ErrEnterpriseFeature):
		simpleError(c, http.StatusForbidden, "Enterprise Feature", err.Error())
	case errors.Is(err, governed_metadata.ErrNotFound):
		simpleError(c, http.StatusNotFound, "Not Found", err.Error())
	case errors.Is(err, governed_metadata.ErrInvalidObjectType):
		simpleError(c, http.StatusBadRequest, "Bad Request", err.Error())
	case errors.Is(err, governed_metadata.ErrVocabularyInUse), errors.Is(err, governed_metadata.ErrReadOnlySchema):
		simpleError(c, http.StatusConflict, "Conflict", err.Error())
	case errors.As(err, &coll):
		simpleError(c, http.StatusConflict, "Field Key Collision", err.Error())
	case errors.As(err, &def):
		simpleError(c, http.StatusBadRequest, "Invalid Definition", err.Error())
	case errors.As(err, &verr):
		writeMetadataValidationResponse(c, verr.Result)
	case errors.As(err, &hrej):
		c.JSON(http.StatusUnprocessableEntity, MetadataValidationErrorResponse{Errors: []MetadataValidationError{{
			Title: "Metadata Change Rejected", Detail: hrej.Reason, Code: "hook_rejected",
		}}})
	default:
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
	}
	return true
}

// writeMetadataValidationResponse writes a 422 with one entry per hard error.
func writeMetadataValidationResponse(c *gin.Context, result *governed_metadata.ValidationResult) {
	resp := MetadataValidationErrorResponse{Errors: []MetadataValidationError{}}
	if result != nil {
		for _, issue := range result.Errors {
			e := MetadataValidationError{Title: "Metadata Validation Failed", Detail: issue.Message, Code: issue.Code}
			e.Source = &struct {
				Pointer string `json:"pointer"`
			}{Pointer: governedMetadataPointer(issue.Field)}
			resp.Errors = append(resp.Errors, e)
		}
	}
	if len(resp.Errors) == 0 {
		resp.Errors = append(resp.Errors, MetadataValidationError{Title: "Metadata Validation Failed", Detail: "governed metadata failed validation"})
	}
	c.JSON(http.StatusUnprocessableEntity, resp)
}

func parseIDParam(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		simpleError(c, http.StatusBadRequest, "Bad Request", "invalid id")
		return 0, false
	}
	return uint(id), true
}

// currentUserID returns the acting user's ID, or 0 when unauthenticated.
func currentUserID(c *gin.Context) uint {
	if u, ok := c.Get("user"); ok {
		if user, ok := u.(*models.User); ok && user != nil {
			return user.ID
		}
	}
	return 0
}

// @Summary Check governed metadata availability
// @Tags governed-metadata
// @Success 200 {object} map[string]bool
// @Router /api/v1/metadata/available [get]
func (a *API) isGovernedMetadataAvailable(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"available": governed_metadata.IsEnterpriseAvailable()})
}

// @Summary List object types that can carry governed metadata
// @Tags governed-metadata
// @Success 200 {object} map[string][]governed_metadata.ObjectTypeInfo
// @Router /api/v1/metadata/object-types [get]
func (a *API) listMetadataObjectTypes(c *gin.Context) {
	types, err := a.governedMetadata().ListObjectTypes()
	if writeGovernedMetadataError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": types})
}

// ---------------------------------------------------------------------------
// Schemas
// ---------------------------------------------------------------------------

func serializeMetadataSchema(s *models.MetadataSchema) gin.H {
	return gin.H{"id": s.ID, "type": "MetadataSchema", "attributes": s}
}

// @Summary List governed metadata schemas
// @Tags governed-metadata
// @Success 200 {object} map[string]interface{}
// @Failure 403 {object} models.ErrorResponse
// @Router /api/v1/metadata/schemas [get]
func (a *API) listMetadataSchemas(c *gin.Context) {
	if !governed_metadata.IsEnterpriseAvailable() {
		writeGovernedMetadataError(c, governed_metadata.ErrEnterpriseFeature)
		return
	}
	schemas, err := a.governedMetadata().ListSchemas()
	if writeGovernedMetadataError(c, err) {
		return
	}
	out := make([]gin.H, 0, len(schemas))
	for i := range schemas {
		out = append(out, serializeMetadataSchema(&schemas[i]))
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// @Summary Get a governed metadata schema
// @Tags governed-metadata
// @Param id path int true "Schema ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/metadata/schemas/{id} [get]
func (a *API) getMetadataSchema(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	schema, err := a.governedMetadata().GetSchema(id)
	if writeGovernedMetadataError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": serializeMetadataSchema(schema)})
}

func applySchemaInput(target *models.MetadataSchema, in *MetadataSchemaInput) {
	attrs := in.Data.Attributes
	target.Name = attrs.Name
	target.Slug = attrs.Slug
	target.Description = attrs.Description
	target.AppliesTo = attrs.AppliesTo
	target.Fields = attrs.Fields
	target.Enforcement = attrs.Enforcement
	if attrs.Active != nil {
		target.Active = *attrs.Active
	}
	if attrs.Order != nil {
		target.Order = *attrs.Order
	}
}

// @Summary Create a governed metadata schema
// @Tags governed-metadata
// @Param schema body MetadataSchemaInput true "Schema"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} models.ErrorResponse
// @Failure 409 {object} models.ErrorResponse
// @Router /api/v1/metadata/schemas [post]
func (a *API) createMetadataSchema(c *gin.Context) {
	var in MetadataSchemaInput
	if err := c.ShouldBindJSON(&in); err != nil {
		simpleError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	schema := &models.MetadataSchema{Active: true}
	applySchemaInput(schema, &in)
	schema.Source = models.MetadataSourceAdmin
	if err := a.governedMetadata().CreateSchema(schema); writeGovernedMetadataError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": serializeMetadataSchema(schema)})
}

// @Summary Update a governed metadata schema
// @Tags governed-metadata
// @Param id path int true "Schema ID"
// @Param schema body MetadataSchemaInput true "Schema"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/metadata/schemas/{id} [patch]
func (a *API) updateMetadataSchema(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	var in MetadataSchemaInput
	if err := c.ShouldBindJSON(&in); err != nil {
		simpleError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	svc := a.governedMetadata()
	schema, err := svc.GetSchema(id)
	if writeGovernedMetadataError(c, err) {
		return
	}
	// Absent structural attributes keep their current value (PATCH semantics).
	attrs := &in.Data.Attributes
	if attrs.Name == "" {
		attrs.Name = schema.Name
	}
	if attrs.Slug == "" {
		attrs.Slug = schema.Slug
	}
	if attrs.Description == "" {
		attrs.Description = schema.Description
	}
	if attrs.AppliesTo == nil {
		attrs.AppliesTo = schema.AppliesTo
	}
	if attrs.Fields == nil {
		attrs.Fields = schema.Fields
	}
	if attrs.Enforcement == "" {
		attrs.Enforcement = schema.Enforcement
	}
	applySchemaInput(schema, &in)
	if err := svc.UpdateSchema(schema); writeGovernedMetadataError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": serializeMetadataSchema(schema)})
}

// @Summary Delete a governed metadata schema
// @Tags governed-metadata
// @Param id path int true "Schema ID"
// @Success 204
// @Router /api/v1/metadata/schemas/{id} [delete]
func (a *API) deleteMetadataSchema(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	if err := a.governedMetadata().DeleteSchema(id); writeGovernedMetadataError(c, err) {
		return
	}
	c.Status(http.StatusNoContent)
}

// @Summary Resolve the merged schema for an object type
// @Tags governed-metadata
// @Param object_type query string true "Object type (llm, tool, datasource, plugin_resource:<id>:<slug>)"
// @Success 200 {object} governed_metadata.ResolvedSchema
// @Router /api/v1/metadata/schemas/resolve [get]
func (a *API) resolveMetadataSchema(c *gin.Context) {
	objectType := strings.TrimSpace(c.Query("object_type"))
	if objectType == "" {
		simpleError(c, http.StatusBadRequest, "Bad Request", "object_type is required")
		return
	}
	resolved, err := a.governedMetadata().ResolveSchema(objectType)
	if writeGovernedMetadataError(c, err) {
		return
	}
	c.JSON(http.StatusOK, resolved)
}

// ---------------------------------------------------------------------------
// Vocabularies
// ---------------------------------------------------------------------------

func serializeMetadataVocabulary(v *models.MetadataVocabulary) gin.H {
	return gin.H{"id": v.ID, "type": "MetadataVocabulary", "attributes": v}
}

// @Summary List controlled vocabularies
// @Tags governed-metadata
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/metadata/vocabularies [get]
func (a *API) listMetadataVocabularies(c *gin.Context) {
	if !governed_metadata.IsEnterpriseAvailable() {
		writeGovernedMetadataError(c, governed_metadata.ErrEnterpriseFeature)
		return
	}
	vocabs, err := a.governedMetadata().ListVocabularies()
	if writeGovernedMetadataError(c, err) {
		return
	}
	out := make([]gin.H, 0, len(vocabs))
	for i := range vocabs {
		out = append(out, serializeMetadataVocabulary(&vocabs[i]))
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// @Summary Get a controlled vocabulary
// @Tags governed-metadata
// @Param id path int true "Vocabulary ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/metadata/vocabularies/{id} [get]
func (a *API) getMetadataVocabulary(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	v, err := a.governedMetadata().GetVocabulary(id)
	if writeGovernedMetadataError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": serializeMetadataVocabulary(v)})
}

// @Summary Create a controlled vocabulary
// @Tags governed-metadata
// @Param vocabulary body MetadataVocabularyInput true "Vocabulary"
// @Success 201 {object} map[string]interface{}
// @Router /api/v1/metadata/vocabularies [post]
func (a *API) createMetadataVocabulary(c *gin.Context) {
	var in MetadataVocabularyInput
	if err := c.ShouldBindJSON(&in); err != nil {
		simpleError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	v := &models.MetadataVocabulary{
		Name: in.Data.Attributes.Name, Slug: in.Data.Attributes.Slug,
		Description: in.Data.Attributes.Description, Terms: in.Data.Attributes.Terms,
		Source: models.MetadataSourceAdmin,
	}
	if err := a.governedMetadata().CreateVocabulary(v); writeGovernedMetadataError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": serializeMetadataVocabulary(v)})
}

// @Summary Update a controlled vocabulary
// @Tags governed-metadata
// @Param id path int true "Vocabulary ID"
// @Param vocabulary body MetadataVocabularyInput true "Vocabulary"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/metadata/vocabularies/{id} [patch]
func (a *API) updateMetadataVocabulary(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	var in MetadataVocabularyInput
	if err := c.ShouldBindJSON(&in); err != nil {
		simpleError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	svc := a.governedMetadata()
	v, err := svc.GetVocabulary(id)
	if writeGovernedMetadataError(c, err) {
		return
	}
	if in.Data.Attributes.Name != "" {
		v.Name = in.Data.Attributes.Name
	}
	if in.Data.Attributes.Slug != "" {
		v.Slug = in.Data.Attributes.Slug
	}
	if in.Data.Attributes.Description != "" {
		v.Description = in.Data.Attributes.Description
	}
	if in.Data.Attributes.Terms != nil {
		v.Terms = in.Data.Attributes.Terms
	}
	if err := svc.UpdateVocabulary(v); writeGovernedMetadataError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": serializeMetadataVocabulary(v)})
}

// @Summary Delete a controlled vocabulary
// @Tags governed-metadata
// @Param id path int true "Vocabulary ID"
// @Success 204
// @Failure 409 {object} models.ErrorResponse "Referenced by a schema field"
// @Router /api/v1/metadata/vocabularies/{id} [delete]
func (a *API) deleteMetadataVocabulary(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	if err := a.governedMetadata().DeleteVocabulary(id); writeGovernedMetadataError(c, err) {
		return
	}
	c.Status(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Values
// ---------------------------------------------------------------------------

// @Summary Validate governed metadata values without saving
// @Tags governed-metadata
// @Param body body ValidateMetadataInput true "Values"
// @Success 200 {object} governed_metadata.ValidationResult
// @Router /api/v1/metadata/validate [post]
func (a *API) validateObjectMetadata(c *gin.Context) {
	var in ValidateMetadataInput
	if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.ObjectType) == "" {
		simpleError(c, http.StatusBadRequest, "Bad Request", "object_type and values are required")
		return
	}
	result, err := a.governedMetadata().Validate(in.ObjectType, in.Values)
	if writeGovernedMetadataError(c, err) {
		return
	}
	c.JSON(http.StatusOK, result)
}

func serializeObjectMetadata(rec *models.ObjectMetadata) gin.H {
	return gin.H{
		"object_type":        rec.ObjectType,
		"object_id":          rec.ObjectID,
		"values":             rec.Values,
		"validation_status":  rec.ValidationStatus,
		"validation_result":  rec.ValidationResult,
		"last_validated_at":  rec.LastValidatedAt,
		"updated_by_user_id": rec.UpdatedByUserID,
		"updated_by_source":  rec.UpdatedBySource,
		"updated_at":         rec.UpdatedAt,
	}
}

// @Summary Get governed metadata for an object
// @Tags governed-metadata
// @Param object_type path string true "Object type"
// @Param object_id path string true "Object ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/metadata/objects/{object_type}/{object_id} [get]
func (a *API) getObjectMetadata(c *gin.Context) {
	rec, err := a.governedMetadata().GetObjectMetadata(c.Param("object_type"), c.Param("object_id"))
	if writeGovernedMetadataError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": serializeObjectMetadata(rec)})
}

// @Summary Set governed metadata for an object
// @Tags governed-metadata
// @Param object_type path string true "Object type"
// @Param object_id path string true "Object ID"
// @Param body body ObjectMetadataInput true "Values"
// @Success 200 {object} map[string]interface{}
// @Failure 422 {object} MetadataValidationErrorResponse
// @Router /api/v1/metadata/objects/{object_type}/{object_id} [put]
func (a *API) setObjectMetadata(c *gin.Context) {
	var in ObjectMetadataInput
	if err := c.ShouldBindJSON(&in); err != nil {
		simpleError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	if in.Values == nil {
		in.Values = map[string]interface{}{}
	}
	rec, result, err := a.governedMetadata().SetObjectMetadata(c.Request.Context(), c.Param("object_type"), c.Param("object_id"), in.Values,
		governed_metadata.SetOptions{Merge: in.Merge, UserID: currentUserID(c), Source: models.MetadataSourceAdmin})
	if writeGovernedMetadataError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": serializeObjectMetadata(rec), "validation": result})
}

// @Summary Delete governed metadata for an object
// @Tags governed-metadata
// @Param object_type path string true "Object type"
// @Param object_id path string true "Object ID"
// @Success 204
// @Router /api/v1/metadata/objects/{object_type}/{object_id} [delete]
func (a *API) deleteObjectMetadata(c *gin.Context) {
	if !governed_metadata.IsEnterpriseAvailable() {
		writeGovernedMetadataError(c, governed_metadata.ErrEnterpriseFeature)
		return
	}
	err := a.governedMetadata().DeleteObjectMetadata(c.Request.Context(), c.Param("object_type"), c.Param("object_id"),
		governed_metadata.SetOptions{UserID: currentUserID(c), Source: models.MetadataSourceAdmin})
	if writeGovernedMetadataError(c, err) {
		return
	}
	c.Status(http.StatusNoContent)
}

// @Summary Get the governed metadata audit trail for an object
// @Tags governed-metadata
// @Param object_type path string true "Object type"
// @Param object_id path string true "Object ID"
// @Param limit query int false "Max entries (default 100)"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/metadata/objects/{object_type}/{object_id}/audit [get]
func (a *API) getObjectMetadataAudit(c *gin.Context) {
	if !governed_metadata.IsEnterpriseAvailable() {
		writeGovernedMetadataError(c, governed_metadata.ErrEnterpriseFeature)
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	audits, err := a.governedMetadata().ListAudit(c.Param("object_type"), c.Param("object_id"), limit)
	if writeGovernedMetadataError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": audits})
}

// @Summary Governed metadata compliance report
// @Tags governed-metadata
// @Param object_type query string false "Filter by object type"
// @Param status query string false "Filter by status (missing, invalid, expired, warnings, valid)"
// @Success 200 {object} governed_metadata.ComplianceReport
// @Router /api/v1/metadata/compliance [get]
func (a *API) getMetadataComplianceReport(c *gin.Context) {
	report, err := a.governedMetadata().ComplianceReport(governed_metadata.ComplianceFilter{
		ObjectType: strings.TrimSpace(c.Query("object_type")),
		Status:     strings.TrimSpace(c.Query("status")),
	})
	if writeGovernedMetadataError(c, err) {
		return
	}
	c.JSON(http.StatusOK, report)
}
